// To run all tests with the race detector:
//
//	go test ./internal/platform/connect_sse/... -v -race
package connect_sse

import (
	"sync"
	"testing"

	"github.com/google/uuid"
)

// ── BroadcastToUserSession ────────────────────────────────────────────────────

func TestBroadcastToUserSession_DirectUUIDLookup(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn, ok := m.Register(userID, sessionUUID, uuid.New(), false)
	if !ok {
		t.Fatal("Register returned false")
	}
	m.BroadcastToUserSession(userID, sessionUUID, "hello", false)
	select {
	case got := <-conn.Send:
		if got != "hello" {
			t.Fatalf("want 'hello', got %q", got)
		}
	default:
		t.Fatal("event not delivered")
	}
}

func TestBroadcastToUserSession_WrongUUID(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn, ok := m.Register(userID, sessionUUID, uuid.New(), false)
	if !ok {
		t.Fatal("Register returned false")
	}
	m.BroadcastToUserSession(userID, uuid.New(), "should-not-arrive", false)
	select {
	case got := <-conn.Send:
		t.Fatalf("event delivered to wrong session: %q", got)
	default:
	}
}

func TestBroadcastToUserSession_UserNotRegistered(t *testing.T) {
	// User not in the map at all — must not panic.
	m := NewManager[string]()
	m.BroadcastToUserSession(uuid.New(), uuid.New(), "noop", false)
}

func TestBroadcastToUserSession_MultipleSessionsOnlyTargeted(t *testing.T) {
	userID, targetUUID, otherUUID := uuid.New(), uuid.New(), uuid.New()
	m := NewManager[string]()
	target, _ := m.Register(userID, targetUUID, uuid.New(), false)
	other, _ := m.Register(userID, otherUUID, uuid.New(), false)

	m.BroadcastToUserSession(userID, targetUUID, "targeted", false)

	select {
	case got := <-target.Send:
		if got != "targeted" {
			t.Fatalf("want 'targeted', got %q", got)
		}
	default:
		t.Fatal("target session did not receive event")
	}
	select {
	case got := <-other.Send:
		t.Fatalf("other session unexpectedly got event: %q", got)
	default:
	}
}

// Session-targeted events trigger real work per delivery (history sync), so a
// session with several tabs must receive the event on exactly one stream — the
// oldest one, which is the most likely to have finished key init and hydration.
func TestBroadcastToUserSession_MultipleConnsOneSession_OnlyOldestReceives(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	first, _ := m.Register(userID, sessionUUID, uuid.New(), false)
	second, _ := m.Register(userID, sessionUUID, uuid.New(), false)
	third, _ := m.Register(userID, sessionUUID, uuid.New(), false)

	m.BroadcastToUserSession(userID, sessionUUID, "targeted", false)

	select {
	case got := <-first.Send:
		if got != "targeted" {
			t.Fatalf("want 'targeted', got %q", got)
		}
	default:
		t.Fatal("oldest connection did not receive event")
	}
	for i, conn := range []*Conn[string]{second, third} {
		select {
		case got := <-conn.Send:
			t.Fatalf("connection %d unexpectedly got targeted event: %q", i, got)
		default:
		}
	}
}

// deliverToAllSessionStreams=true is the opt-in for session-scoped events that every tab
// must observe: all streams of the target session receive, other sessions do not.
func TestBroadcastToUserSession_AllSessionStreams_AllConnsReceive(t *testing.T) {
	userID, sessionUUID, otherSession := uuid.New(), uuid.New(), uuid.New()
	m := NewManager[string]()
	first, _ := m.Register(userID, sessionUUID, uuid.New(), false)
	second, _ := m.Register(userID, sessionUUID, uuid.New(), false)
	third, _ := m.Register(userID, sessionUUID, uuid.New(), false)
	other, _ := m.Register(userID, otherSession, uuid.New(), false)

	m.BroadcastToUserSession(userID, sessionUUID, "all", true)

	for i, conn := range []*Conn[string]{first, second, third} {
		select {
		case got := <-conn.Send:
			if got != "all" {
				t.Fatalf("conn[%d] want 'all', got %q", i, got)
			}
		default:
			t.Fatalf("conn[%d] did not receive session-wide event", i)
		}
	}
	select {
	case got := <-other.Send:
		t.Fatalf("other session unexpectedly got event: %q", got)
	default:
	}
}

func TestBroadcastToUserSession_BufferFull_EventDropped(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn, ok := m.Register(userID, sessionUUID, uuid.New(), false)
	if !ok {
		t.Fatal("Register returned false")
	}

	// Fill the buffer completely (channelBufferSize = 128)
	for range channelBufferSize {
		conn.Send <- "fill"
	}
	// One more — should be dropped without panic
	m.BroadcastToUserSession(userID, sessionUUID, "overflow", false)
	if len(conn.Send) != channelBufferSize {
		t.Fatalf("expected buffer size %d, got %d", channelBufferSize, len(conn.Send))
	}
}

// ── BroadcastToUser ───────────────────────────────────────────────────────────

func TestBroadcastToUser_AllSessionsReceive(t *testing.T) {
	userID := uuid.New()
	m := NewManager[string]()
	c1, _ := m.Register(userID, uuid.New(), uuid.New(), false)
	c2, _ := m.Register(userID, uuid.New(), uuid.New(), false)

	m.BroadcastToUser(userID, "broadcast")

	for _, c := range []*Conn[string]{c1, c2} {
		select {
		case got := <-c.Send:
			if got != "broadcast" {
				t.Fatalf("want 'broadcast', got %q", got)
			}
		default:
			t.Fatal("a session did not receive event")
		}
	}
}

func TestBroadcastToUser_AllConnectionsOfSessionReceive(t *testing.T) {
	userID, sessionA, sessionB := uuid.New(), uuid.New(), uuid.New()
	m := NewManager[string]()
	a1, _ := m.Register(userID, sessionA, uuid.New(), false)
	a2, _ := m.Register(userID, sessionA, uuid.New(), false)
	b1, _ := m.Register(userID, sessionB, uuid.New(), false)

	m.BroadcastToUser(userID, "broadcast")

	for i, c := range []*Conn[string]{a1, a2, b1} {
		select {
		case got := <-c.Send:
			if got != "broadcast" {
				t.Fatalf("conn[%d] want 'broadcast', got %q", i, got)
			}
		default:
			t.Fatalf("conn[%d] did not receive event", i)
		}
	}
}

func TestBroadcastToUser_UserNotRegistered(t *testing.T) {
	m := NewManager[string]()
	m.BroadcastToUser(uuid.New(), "noop") // must not panic
}

// ── BroadcastToUserExcept ─────────────────────────────────────────────────────

func TestBroadcastToUserExcept_ExcludedSessionSkipped(t *testing.T) {
	userID, excludeUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	excluded, _ := m.Register(userID, excludeUUID, uuid.New(), false)
	other, _ := m.Register(userID, uuid.New(), uuid.New(), false)

	m.BroadcastToUserExcept(userID, excludeUUID, "except")

	select {
	case got := <-other.Send:
		if got != "except" {
			t.Fatalf("want 'except', got %q", got)
		}
	default:
		t.Fatal("other session did not receive event")
	}
	select {
	case got := <-excluded.Send:
		t.Fatalf("excluded session received event: %q", got)
	default:
	}
}

func TestBroadcastToUserExcept_ExcludedSessionSkipsAllItsConnections(t *testing.T) {
	userID, excludeUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	excluded1, _ := m.Register(userID, excludeUUID, uuid.New(), false)
	excluded2, _ := m.Register(userID, excludeUUID, uuid.New(), false)
	other, _ := m.Register(userID, uuid.New(), uuid.New(), false)

	m.BroadcastToUserExcept(userID, excludeUUID, "except")

	select {
	case got := <-other.Send:
		if got != "except" {
			t.Fatalf("want 'except', got %q", got)
		}
	default:
		t.Fatal("other session did not receive event")
	}
	for i, conn := range []*Conn[string]{excluded1, excluded2} {
		select {
		case got := <-conn.Send:
			t.Fatalf("excluded conn[%d] received event: %q", i, got)
		default:
		}
	}
}

// ── IsSessionActive ───────────────────────────────────────────────────────────

func TestIsSessionActive_TrueAfterRegister(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	m.Register(userID, sessionUUID, uuid.New(), false)
	if !m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("expected session to be active")
	}
}

func TestIsSessionActive_FalseForUnknownUser(t *testing.T) {
	m := NewManager[string]()
	if m.IsSessionActive(uuid.New(), uuid.New()) {
		t.Fatal("expected inactive for unknown user")
	}
}

func TestIsSessionActive_FalseAfterUnregister(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn, _ := m.Register(userID, sessionUUID, uuid.New(), false)
	m.Unregister(conn)
	if m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("expected session to be inactive after unregister")
	}
}

func TestIsSessionActive_TrueWhileAnyConnRemains(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn1, _ := m.Register(userID, sessionUUID, uuid.New(), false)
	conn2, _ := m.Register(userID, sessionUUID, uuid.New(), false)

	m.Unregister(conn1)
	if !m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("session must stay active while a stream remains")
	}
	m.Unregister(conn2)
	if m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("session must be inactive after the last stream closes")
	}
}

// ── Register ──────────────────────────────────────────────────────────────────

func TestRegister_MaxSessionsPerUser_Rejected(t *testing.T) {
	userID := uuid.New()
	m := NewManager[string]()
	for i := range maxSessionsPerUser {
		_, ok := m.Register(userID, uuid.New(), uuid.New(), false)
		if !ok {
			t.Fatalf("Register should succeed for session %d", i+1)
		}
	}
	_, ok := m.Register(userID, uuid.New(), uuid.New(), false)
	if ok {
		t.Fatal("Register should be rejected when at max session count")
	}
}

func TestRegister_SameSession_ThreeConnectionsAllowed_FourthRejected(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()

	conns := make([]*Conn[string], 0, MaxConnsPerSession)
	for i := range MaxConnsPerSession {
		conn, ok := m.Register(userID, sessionUUID, uuid.New(), false)
		if !ok {
			t.Fatalf("Register should succeed for connection %d", i+1)
		}
		conns = append(conns, conn)
	}

	if _, ok := m.Register(userID, sessionUUID, uuid.New(), false); ok {
		t.Fatalf("Register should be rejected beyond %d connections per session", MaxConnsPerSession)
	}

	// All three streams stay active and reachable.
	m.BroadcastToUser(userID, "all")
	for i, conn := range conns {
		select {
		case got := <-conn.Send:
			if got != "all" {
				t.Fatalf("conn[%d] want 'all', got %q", i, got)
			}
		default:
			t.Fatalf("conn[%d] did not receive broadcast", i)
		}
	}
}

func TestRegister_SameConnectionID_ReplacesOld(t *testing.T) {
	userID, sessionUUID, connectionID := uuid.New(), uuid.New(), uuid.New()
	m := NewManager[string]()

	old, _ := m.Register(userID, sessionUUID, connectionID, false)
	// Register again with the same connection ID — should close old and succeed
	newConn, ok := m.Register(userID, sessionUUID, connectionID, false)
	if !ok {
		t.Fatal("Re-register of same connection ID should succeed")
	}
	// Old conn's Send channel should now be closed
	_, oldOpen := <-old.Send
	if oldOpen {
		t.Fatal("old conn Send channel should be closed after re-register")
	}
	// New conn should be active
	if !m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("new conn should be active")
	}
	_ = newConn
}

// Replacement must not consume an extra slot: re-registering one tab while the
// session is at the cap must still succeed and leave the count at the cap.
func TestRegister_SameConnectionID_ReplaceKeepsSessionAtCap(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()

	c1, ok1 := m.Register(userID, sessionUUID, uuid.New(), false)
	c2, ok2 := m.Register(userID, sessionUUID, uuid.New(), false)
	c3, ok3 := m.Register(userID, sessionUUID, uuid.New(), false)
	if !ok1 || !ok2 || !ok3 {
		t.Fatal("first three registrations must succeed")
	}

	if _, ok := m.Register(userID, sessionUUID, c2.ConnectionID, false); !ok {
		t.Fatal("re-registering an existing connection at cap must succeed")
	}
	if _, open := <-c2.Send; open {
		t.Fatal("replaced connection's Send channel should be closed")
	}
	if _, ok := m.Register(userID, sessionUUID, uuid.New(), false); ok {
		t.Fatal("a fourth distinct connection must still be rejected")
	}

	// c1 and c3 are untouched.
	for i, conn := range []*Conn[string]{c1, c3} {
		select {
		case <-conn.Send:
			t.Fatalf("conn[%d] should not have been closed", i)
		default:
		}
	}
}

func TestRegister_AfterUnregister_FreesSlot(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()

	first, _ := m.Register(userID, sessionUUID, uuid.New(), false)
	second, _ := m.Register(userID, sessionUUID, uuid.New(), false)
	if _, ok := m.Register(userID, sessionUUID, uuid.New(), false); !ok {
		t.Fatal("third registration must succeed")
	}
	if _, ok := m.Register(userID, sessionUUID, uuid.New(), false); ok {
		t.Fatal("fourth registration must be rejected")
	}

	m.Unregister(second)
	if _, ok := m.Register(userID, sessionUUID, uuid.New(), false); !ok {
		t.Fatal("register must succeed after a slot is freed")
	}
	_ = first
}

func TestRegister_SameConnectionIDAcrossSessions_Coexist(t *testing.T) {
	userID, connectionID := uuid.New(), uuid.New()
	m := NewManager[string]()

	connA, okA := m.Register(userID, uuid.New(), connectionID, false)
	connB, okB := m.Register(userID, uuid.New(), connectionID, false)
	if !okA || !okB {
		t.Fatal("the same connection ID must be independent across sessions")
	}
	if connA.ConnectionID != connectionID || connB.ConnectionID != connectionID {
		t.Fatal("ConnectionID should be propagated to both conns")
	}
}

// ── Unregister ────────────────────────────────────────────────────────────────

func TestUnregister_NilConn_NoPanic(t *testing.T) {
	m := NewManager[string]()
	m.Unregister(nil) // must not panic
}

func TestUnregister_RemovesFromMap(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn, _ := m.Register(userID, sessionUUID, uuid.New(), false)
	m.Unregister(conn)
	if m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("session should be inactive after unregister")
	}
}

func TestUnregister_ClosesChannel(t *testing.T) {
	userID := uuid.New()
	m := NewManager[string]()
	conn, _ := m.Register(userID, uuid.New(), uuid.New(), false)
	m.Unregister(conn)
	_, open := <-conn.Send
	if open {
		t.Fatal("Send channel should be closed after unregister")
	}
}

func TestUnregister_OneOfMultiple_LeavesSessionActive(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn1, _ := m.Register(userID, sessionUUID, uuid.New(), false)
	conn2, _ := m.Register(userID, sessionUUID, uuid.New(), false)

	m.Unregister(conn1)
	if !m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("session should remain active while another conn exists")
	}
	if _, open := <-conn1.Send; open {
		t.Fatal("unregistered conn's channel should be closed")
	}
	select {
	case <-conn2.Send:
		t.Fatal("remaining conn should not have been closed")
	default:
	}
}

// ── UnregisterSession ─────────────────────────────────────────────────────────

func TestUnregisterSession_RemovesTargetOnly(t *testing.T) {
	userID, s1, s2 := uuid.New(), uuid.New(), uuid.New()
	m := NewManager[string]()
	m.Register(userID, s1, uuid.New(), false)
	m.Register(userID, s2, uuid.New(), false)
	m.UnregisterSession(userID, s1)
	if m.IsSessionActive(userID, s1) {
		t.Fatal("s1 should be inactive")
	}
	if !m.IsSessionActive(userID, s2) {
		t.Fatal("s2 should still be active")
	}
}

func TestUnregisterSession_ClosesAllConnectionsOfSession(t *testing.T) {
	userID, s1, s2 := uuid.New(), uuid.New(), uuid.New()
	m := NewManager[string]()
	c1, _ := m.Register(userID, s1, uuid.New(), false)
	c2, _ := m.Register(userID, s1, uuid.New(), false)
	c3, _ := m.Register(userID, s2, uuid.New(), false)

	m.UnregisterSession(userID, s1)

	for i, conn := range []*Conn[string]{c1, c2} {
		if _, open := <-conn.Send; open {
			t.Fatalf("conn[%d] of the closed session should have its channel closed", i)
		}
	}
	if !m.IsSessionActive(userID, s2) {
		t.Fatal("other session should remain active")
	}
	select {
	case <-c3.Send:
		t.Fatal("other session's conn should not have been closed")
	default:
	}
}

// ── UnregisterUserConnections ─────────────────────────────────────────────────

func TestUnregisterUserConnections_ClearsAll(t *testing.T) {
	userID := uuid.New()
	s1, s2 := uuid.New(), uuid.New()
	m := NewManager[string]()
	m.Register(userID, s1, uuid.New(), false)
	m.Register(userID, s2, uuid.New(), false)
	m.UnregisterUserConnections(userID)
	if m.IsSessionActive(userID, s1) || m.IsSessionActive(userID, s2) {
		t.Fatal("all sessions should be inactive after UnregisterUserConnections")
	}
}

// ── Close idempotency ─────────────────────────────────────────────────────────

func TestConn_Close_Idempotent(t *testing.T) {
	conn := &Conn[string]{Send: make(chan string, 1)}
	// Must not panic on multiple calls
	conn.Close()
	conn.Close()
	conn.Close()
}

// ── Concurrency ───────────────────────────────────────────────────────────────

func TestManager_ConcurrentRegisterAndBroadcast(t *testing.T) {
	userID := uuid.New()
	m := NewManager[string]()

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			sID := uuid.New()
			conn, ok := m.Register(userID, sID, uuid.New(), false)
			if ok {
				m.BroadcastToUserSession(userID, sID, "concurrent", false)
				// drain to avoid goroutine leak
				select {
				case <-conn.Send:
				default:
				}
				m.Unregister(conn)
			}
		})
	}
	wg.Wait()
}

// Ten goroutines race for three slots on one session: exactly three may win.
func TestManager_ConcurrentRegisterSameSession_CapEnforced(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var conns []*Conn[string]
	for range 10 {
		wg.Go(func() {
			conn, ok := m.Register(userID, sessionUUID, uuid.New(), false)
			if ok {
				mu.Lock()
				conns = append(conns, conn)
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if len(conns) != MaxConnsPerSession {
		t.Fatalf("expected exactly %d successful registrations, got %d", MaxConnsPerSession, len(conns))
	}
	for _, conn := range conns {
		m.Unregister(conn)
	}
	if m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("session should be inactive after all conns unregister")
	}
}

// ── BroadcastToUser — additional edge cases ───────────────────────────────────

func TestBroadcastToUser_BufferFull_EventDropped(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn, ok := m.Register(userID, sessionUUID, uuid.New(), false)
	if !ok {
		t.Fatal("Register returned false")
	}
	// Fill buffer
	for range channelBufferSize {
		conn.Send <- "fill"
	}
	// Overflow — must not panic, event dropped
	m.BroadcastToUser(userID, "overflow")
	if len(conn.Send) != channelBufferSize {
		t.Fatalf("expected buffer size %d, got %d", channelBufferSize, len(conn.Send))
	}
}

// ── BroadcastToUserExcept — additional edge cases ─────────────────────────────

func TestBroadcastToUserExcept_UserNotRegistered_NoPanic(t *testing.T) {
	m := NewManager[string]()
	m.BroadcastToUserExcept(uuid.New(), uuid.New(), "noop") // must not panic
}

func TestBroadcastToUserExcept_OnlySessionIsExcluded_NothingDelivered(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn, _ := m.Register(userID, sessionUUID, uuid.New(), false)

	m.BroadcastToUserExcept(userID, sessionUUID, "excluded-only")

	select {
	case got := <-conn.Send:
		t.Fatalf("only session was excluded but received event: %q", got)
	default:
		// correct: nothing delivered
	}
}

// ── UnregisterSession — additional edge cases ─────────────────────────────────

func TestUnregisterSession_UserNotRegistered_NoPanic(t *testing.T) {
	m := NewManager[string]()
	m.UnregisterSession(uuid.New(), uuid.New()) // must not panic
}

func TestUnregisterSession_SessionNotFound_NoPanic(t *testing.T) {
	userID := uuid.New()
	m := NewManager[string]()
	m.Register(userID, uuid.New(), uuid.New(), false)
	m.UnregisterSession(userID, uuid.New()) // unknown session — must not panic, other sessions unaffected
}

// ── UnregisterUserConnections — additional edge cases ─────────────────────────

func TestUnregisterUserConnections_UserNotRegistered_NoPanic(t *testing.T) {
	m := NewManager[string]()
	m.UnregisterUserConnections(uuid.New()) // must not panic
}

func TestUnregisterUserConnections_ClosesAllChannels(t *testing.T) {
	userID := uuid.New()
	m := NewManager[string]()
	c1, _ := m.Register(userID, uuid.New(), uuid.New(), false)
	c2, _ := m.Register(userID, uuid.New(), uuid.New(), false)

	m.UnregisterUserConnections(userID)

	for i, c := range []*Conn[string]{c1, c2} {
		_, open := <-c.Send
		if open {
			t.Fatalf("conn[%d] Send channel should be closed", i)
		}
	}
}

func TestUnregisterUserConnections_ThenRegisterAgain(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	m.Register(userID, sessionUUID, uuid.New(), false)
	m.UnregisterUserConnections(userID)

	// Re-register on the same user — map slot should be reusable
	conn, ok := m.Register(userID, sessionUUID, uuid.New(), false)
	if !ok {
		t.Fatal("Re-register after UnregisterUserConnections should succeed")
	}
	if !m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("session should be active after re-register")
	}
	_ = conn
}

// ── Unregister — stale conn edge case ────────────────────────────────────────

func TestUnregister_StaleConn_DoesNotEvictNewRegistration(t *testing.T) {
	userID, sessionUUID, connectionID := uuid.New(), uuid.New(), uuid.New()
	m := NewManager[string]()

	oldConn, _ := m.Register(userID, sessionUUID, connectionID, false)
	// Re-register same connection ID — oldConn is closed, newConn takes over
	newConn, ok := m.Register(userID, sessionUUID, connectionID, false)
	if !ok {
		t.Fatal("re-register should succeed")
	}

	// Unregistering the old (now stale) conn must not remove the new registration
	m.Unregister(oldConn)
	if !m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("new registration should still be active after unregistering stale conn")
	}
	_ = newConn
}

// ── Register with uuid.Nil ────────────────────────────────────────────────────

func TestRegister_NilSessionUUID_AutoGenerates(t *testing.T) {
	userID := uuid.New()
	m := NewManager[string]()
	conn, ok := m.Register(userID, uuid.Nil, uuid.New(), false)
	if !ok {
		t.Fatal("Register with uuid.Nil session should succeed")
	}
	if conn.SessionUUID == uuid.Nil {
		t.Fatal("SessionUUID should be auto-generated, not uuid.Nil")
	}
	if !m.IsSessionActive(userID, conn.SessionUUID) {
		t.Fatal("auto-generated session should be active")
	}
}

func TestRegister_NilConnectionID_Rejected(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	if _, ok := m.Register(userID, sessionUUID, uuid.Nil, false); ok {
		t.Fatal("Register without a connection ID must be rejected")
	}
	if m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("a rejected registration must not leave an active session")
	}
}

// ── IsPrimary flag ────────────────────────────────────────────────────────────

func TestRegister_IsPrimary_PropagatedToConn(t *testing.T) {
	userID := uuid.New()
	m := NewManager[string]()

	primary, _ := m.Register(userID, uuid.New(), uuid.New(), true)
	secondary, _ := m.Register(userID, uuid.New(), uuid.New(), false)

	if !primary.IsPrimary {
		t.Fatal("primary conn should have IsPrimary=true")
	}
	if secondary.IsPrimary {
		t.Fatal("secondary conn should have IsPrimary=false")
	}
}

// ── Concurrency — BroadcastToUser from multiple goroutines ───────────────────

func TestManager_ConcurrentBroadcastToUser(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn, ok := m.Register(userID, sessionUUID, uuid.New(), false)
	if !ok {
		t.Fatal("Register returned false")
	}
	defer m.Unregister(conn)

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			m.BroadcastToUser(userID, "concurrent-broadcast")
		})
	}
	wg.Wait()
	// drain — no assertions on count since buffer may drop; just must not race or panic
	for len(conn.Send) > 0 {
		<-conn.Send
	}
}

func TestManager_ConcurrentUnregisterAndBroadcast(t *testing.T) {
	// Race between broadcast and unregister — must not panic or deadlock.
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn, ok := m.Register(userID, sessionUUID, uuid.New(), false)
	if !ok {
		t.Fatal("Register returned false")
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 50 {
			m.BroadcastToUser(userID, "racing")
		}
	}()
	go func() {
		defer wg.Done()
		m.Unregister(conn)
	}()
	wg.Wait()
}

func TestManager_HasUser(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()

	if m.HasUser(userID) {
		t.Fatal("expected HasUser to return false for unregistered user")
	}

	conn, ok := m.Register(userID, sessionUUID, uuid.New(), false)
	if !ok {
		t.Fatal("Register returned false")
	}

	if !m.HasUser(userID) {
		t.Fatal("expected HasUser to return true for registered user")
	}

	m.Unregister(conn)
	if m.HasUser(userID) {
		t.Fatal("expected HasUser to return false after unregistering")
	}
}

func TestManager_HasUserExcept(t *testing.T) {
	userID := uuid.New()
	sessionA, sessionB := uuid.New(), uuid.New()
	m := NewManager[string]()

	if m.HasUserExcept(userID, sessionA) {
		t.Fatal("expected false for unregistered user")
	}

	connA, _ := m.Register(userID, sessionA, uuid.New(), false)

	// User only has sessionA, so HasUserExcept sessionA should be false
	if m.HasUserExcept(userID, sessionA) {
		t.Fatal("expected false when only excluded session exists")
	}
	// HasUserExcept sessionB should be true because sessionA exists
	if !m.HasUserExcept(userID, sessionB) {
		t.Fatal("expected true when other session exists")
	}

	connB, _ := m.Register(userID, sessionB, uuid.New(), false)
	// Now both exist, so HasUserExcept sessionA should be true (sessionB is present)
	if !m.HasUserExcept(userID, sessionA) {
		t.Fatal("expected true because sessionB is still registered")
	}

	m.Unregister(connB)
	if m.HasUserExcept(userID, sessionA) {
		t.Fatal("expected false after sessionB unregistered")
	}
	m.Unregister(connA)
}
