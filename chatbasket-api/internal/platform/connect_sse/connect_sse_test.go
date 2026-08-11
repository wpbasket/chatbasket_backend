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
	conn, ok := m.Register(userID, sessionUUID, false)
	if !ok {
		t.Fatal("Register returned false")
	}
	m.BroadcastToUserSession(userID, sessionUUID, "hello")
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
	conn, ok := m.Register(userID, sessionUUID, false)
	if !ok {
		t.Fatal("Register returned false")
	}
	m.BroadcastToUserSession(userID, uuid.New(), "should-not-arrive")
	select {
	case got := <-conn.Send:
		t.Fatalf("event delivered to wrong session: %q", got)
	default:
	}
}

func TestBroadcastToUserSession_UserNotRegistered(t *testing.T) {
	// User not in the map at all — must not panic.
	m := NewManager[string]()
	m.BroadcastToUserSession(uuid.New(), uuid.New(), "noop")
}

func TestBroadcastToUserSession_MultipleSessionsOnlyTargeted(t *testing.T) {
	userID, targetUUID, otherUUID := uuid.New(), uuid.New(), uuid.New()
	m := NewManager[string]()
	target, _ := m.Register(userID, targetUUID, false)
	other, _ := m.Register(userID, otherUUID, false)

	m.BroadcastToUserSession(userID, targetUUID, "targeted")

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

func TestBroadcastToUserSession_BufferFull_EventDropped(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn, ok := m.Register(userID, sessionUUID, false)
	if !ok {
		t.Fatal("Register returned false")
	}

	// Fill the buffer completely (channelBufferSize = 128)
	for i := 0; i < channelBufferSize; i++ {
		conn.Send <- "fill"
	}
	// One more — should be dropped without panic
	m.BroadcastToUserSession(userID, sessionUUID, "overflow")
	if len(conn.Send) != channelBufferSize {
		t.Fatalf("expected buffer size %d, got %d", channelBufferSize, len(conn.Send))
	}
}

// ── BroadcastToUser ───────────────────────────────────────────────────────────

func TestBroadcastToUser_AllSessionsReceive(t *testing.T) {
	userID := uuid.New()
	m := NewManager[string]()
	c1, _ := m.Register(userID, uuid.New(), false)
	c2, _ := m.Register(userID, uuid.New(), false)

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

func TestBroadcastToUser_UserNotRegistered(t *testing.T) {
	m := NewManager[string]()
	m.BroadcastToUser(uuid.New(), "noop") // must not panic
}

// ── BroadcastToUserExcept ─────────────────────────────────────────────────────

func TestBroadcastToUserExcept_ExcludedSessionSkipped(t *testing.T) {
	userID, excludeUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	excluded, _ := m.Register(userID, excludeUUID, false)
	other, _ := m.Register(userID, uuid.New(), false)

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

// ── IsSessionActive ───────────────────────────────────────────────────────────

func TestIsSessionActive_TrueAfterRegister(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	m.Register(userID, sessionUUID, false)
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
	conn, _ := m.Register(userID, sessionUUID, false)
	m.Unregister(conn)
	if m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("expected session to be inactive after unregister")
	}
}

// ── Register ──────────────────────────────────────────────────────────────────

func TestRegister_MaxConnsPerUser_Rejected(t *testing.T) {
	userID := uuid.New()
	m := NewManager[string]()
	for i := 0; i < maxConnsPerUser; i++ {
		_, ok := m.Register(userID, uuid.New(), false)
		if !ok {
			t.Fatalf("Register should succeed for conn %d", i+1)
		}
	}
	_, ok := m.Register(userID, uuid.New(), false)
	if ok {
		t.Fatal("Register should be rejected when at max capacity")
	}
}

func TestRegister_SameSessionUUID_ReplacesOld(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()

	old, _ := m.Register(userID, sessionUUID, false)
	// Register again with the same session UUID — should close old and succeed
	newConn, ok := m.Register(userID, sessionUUID, false)
	if !ok {
		t.Fatal("Re-register of same session UUID should succeed")
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

// ── Unregister ────────────────────────────────────────────────────────────────

func TestUnregister_NilConn_NoPanic(t *testing.T) {
	m := NewManager[string]()
	m.Unregister(nil) // must not panic
}

func TestUnregister_RemovesFromMap(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn, _ := m.Register(userID, sessionUUID, false)
	m.Unregister(conn)
	if m.IsSessionActive(userID, sessionUUID) {
		t.Fatal("session should be inactive after unregister")
	}
}

func TestUnregister_ClosesChannel(t *testing.T) {
	userID := uuid.New()
	m := NewManager[string]()
	conn, _ := m.Register(userID, uuid.New(), false)
	m.Unregister(conn)
	_, open := <-conn.Send
	if open {
		t.Fatal("Send channel should be closed after unregister")
	}
}

// ── UnregisterSession ─────────────────────────────────────────────────────────

func TestUnregisterSession_RemovesTargetOnly(t *testing.T) {
	userID, s1, s2 := uuid.New(), uuid.New(), uuid.New()
	m := NewManager[string]()
	m.Register(userID, s1, false)
	m.Register(userID, s2, false)
	m.UnregisterSession(userID, s1)
	if m.IsSessionActive(userID, s1) {
		t.Fatal("s1 should be inactive")
	}
	if !m.IsSessionActive(userID, s2) {
		t.Fatal("s2 should still be active")
	}
}

// ── UnregisterUserConnections ─────────────────────────────────────────────────

func TestUnregisterUserConnections_ClearsAll(t *testing.T) {
	userID := uuid.New()
	s1, s2 := uuid.New(), uuid.New()
	m := NewManager[string]()
	m.Register(userID, s1, false)
	m.Register(userID, s2, false)
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
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sID := uuid.New()
			conn, ok := m.Register(userID, sID, false)
			if ok {
				m.BroadcastToUserSession(userID, sID, "concurrent")
				// drain to avoid goroutine leak
				select {
				case <-conn.Send:
				default:
				}
				m.Unregister(conn)
			}
		}()
	}
	wg.Wait()
}

// ── BroadcastToUser — additional edge cases ───────────────────────────────────

func TestBroadcastToUser_BufferFull_EventDropped(t *testing.T) {
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()
	conn, ok := m.Register(userID, sessionUUID, false)
	if !ok {
		t.Fatal("Register returned false")
	}
	// Fill buffer
	for i := 0; i < channelBufferSize; i++ {
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
	conn, _ := m.Register(userID, sessionUUID, false)

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
	m.Register(userID, uuid.New(), false)
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
	c1, _ := m.Register(userID, uuid.New(), false)
	c2, _ := m.Register(userID, uuid.New(), false)

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
	m.Register(userID, sessionUUID, false)
	m.UnregisterUserConnections(userID)

	// Re-register on the same user — map slot should be reusable
	conn, ok := m.Register(userID, sessionUUID, false)
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
	userID, sessionUUID := uuid.New(), uuid.New()
	m := NewManager[string]()

	oldConn, _ := m.Register(userID, sessionUUID, false)
	// Re-register same session — oldConn is closed, newConn takes over
	newConn, ok := m.Register(userID, sessionUUID, false)
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
	conn, ok := m.Register(userID, uuid.Nil, false)
	if !ok {
		t.Fatal("Register with uuid.Nil should succeed")
	}
	if conn.SessionUUID == uuid.Nil {
		t.Fatal("SessionUUID should be auto-generated, not uuid.Nil")
	}
	if !m.IsSessionActive(userID, conn.SessionUUID) {
		t.Fatal("auto-generated session should be active")
	}
}

// ── IsPrimary flag ────────────────────────────────────────────────────────────

func TestRegister_IsPrimary_PropagatedToConn(t *testing.T) {
	userID := uuid.New()
	m := NewManager[string]()

	primary, _ := m.Register(userID, uuid.New(), true)
	secondary, _ := m.Register(userID, uuid.New(), false)

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
	conn, ok := m.Register(userID, sessionUUID, false)
	if !ok {
		t.Fatal("Register returned false")
	}
	defer m.Unregister(conn)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.BroadcastToUser(userID, "concurrent-broadcast")
		}()
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
	conn, ok := m.Register(userID, sessionUUID, false)
	if !ok {
		t.Fatal("Register returned false")
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			m.BroadcastToUser(userID, "racing")
		}
	}()
	go func() {
		defer wg.Done()
		m.Unregister(conn)
	}()
	wg.Wait()
}
