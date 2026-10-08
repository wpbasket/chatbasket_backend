package personal_sse

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	rpc_personal_chatv1 "chatbasket-api/gen/proto/personal/personal_chat"
	rpc_personal_ssev1 "chatbasket-api/gen/proto/personal/personal_sse"
	"chatbasket-api/internal/modules/personal/personal_sse/internal/personal_sse_store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func getTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DatabaseURLTesting")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL_PG_TESTING")
	}
	if dsn == "" {
		t.Skip("DatabaseURLTesting/DATABASE_URL_PG_TESTING environment variable not set, skipping database test")
	}
	return dsn
}

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := getTestDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	// Migration 013 must be applied.
	_, err = pool.Exec(ctx, "SELECT 1 FROM personal_sse_outbox LIMIT 1")
	require.NoError(t, err, "personal_sse_outbox missing: apply migration 013")
	return pool
}

func bigTestEvent(t *testing.T) (*rpc_personal_ssev1.PersonalSseEvent, []byte) {
	t.Helper()
	replyID := uuid.New().String()
	msg := &rpc_personal_chatv1.Message{
		MessageId:        uuid.New().String(),
		ChatId:           uuid.New().String(),
		RecipientId:      uuid.New().String(),
		Content:          strings.Repeat("x", 65000),
		MessageType:      "text",
		ReplyToMessageId: &replyID,
	}
	evt := &rpc_personal_ssev1.PersonalSseEvent{
		Timestamp: timestamppb.Now(),
		Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
			ChatModule: &rpc_personal_chatv1.ChatSsePayload{
				Event: &rpc_personal_chatv1.ChatSsePayload_SendMessageSseEvent{
					SendMessageSseEvent: msg,
				},
			},
		},
	}
	raw, err := marshalEventBytes(evt)
	require.NoError(t, err)
	require.Greater(t, len(raw), 60000)
	return evt, raw
}

func smallTestEvent() *rpc_personal_ssev1.PersonalSseEvent {
	return &rpc_personal_ssev1.PersonalSseEvent{
		Timestamp: timestamppb.Now(),
		Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
			ChatModule: &rpc_personal_chatv1.ChatSsePayload{
				Event: &rpc_personal_chatv1.ChatSsePayload_AcknowledgeDeliverySseEvent{
					AcknowledgeDeliverySseEvent: &rpc_personal_chatv1.AcknowledgeDeliverySsePayload{
						ChatId:      uuid.New().String(),
						MessageIds:  []string{uuid.New().String()},
						DeliveredAt: timestamppb.Now(),
					},
				},
			},
		},
	}
}

func deleteOutboxRow(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `DELETE FROM personal_sse_outbox WHERE id = $1`, id)
	require.NoError(t, err)
}

func insertOutboxRow(t *testing.T, pool *pgxpool.Pool, ctx context.Context, id uuid.UUID, payload postgresSsePayload, eventBytes []byte) error {
	t.Helper()
	return personal_sse_store.New(pool).CreateSseOutboxRow(ctx, personal_sse_store.CreateSseOutboxRowParams{
		ID:               id,
		Command:          payload.Command,
		TargetUserID:     payload.TargetUserID,
		TargetSessionID:  payload.TargetSessionUUID,
		ExcludeSessionID: payload.ExcludeSessionUUID,
		Payload:          eventBytes,
		ExpiresAt:        time.Now().Add(outboxTTL),
	})
}

func fetchOutboxEvent(t *testing.T, pool *pgxpool.Pool, ctx context.Context, id uuid.UUID) ([]byte, error) {
	t.Helper()
	return personal_sse_store.New(pool).GetSseOutboxRow(ctx, id)
}

func TestIntegration_OutboxRoundTrip65k(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	want, raw := bigTestEvent(t)
	id := uuid.New()

	require.NoError(t, insertOutboxRow(t, pool, ctx, id, postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: uuid.New(),
	}, raw))
	t.Cleanup(func() { deleteOutboxRow(t, pool, id) })

	got, err := fetchOutboxEvent(t, pool, ctx, id)
	require.NoError(t, err)
	assert.Equal(t, raw, got, "stored bytes must be bit-exact")

	back, err := unmarshalEventBytes(got)
	require.NoError(t, err)
	assert.True(t, proto.Equal(want, back), "hydrated event must equal original")

	deleteOutboxRow(t, pool, id)
	_, err = fetchOutboxEvent(t, pool, ctx, id)
	assert.Error(t, err, "deleted row must be gone")
}

func TestIntegration_NotNullPayloadEnforced(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	err := personal_sse_store.New(pool).CreateSseOutboxRow(ctx, personal_sse_store.CreateSseOutboxRowParams{
		ID:           uuid.New(),
		Command:      cmdBroadcastToUser,
		TargetUserID: uuid.New(),
		Payload:      nil,
		ExpiresAt:    time.Now().Add(outboxTTL),
	})
	assert.Error(t, err, "NULL payload must violate NOT NULL")
}

func TestIntegration_SweepDeletesOnlyExpired(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	q := personal_sse_store.New(pool)
	expiredID := uuid.New()
	freshID := uuid.New()

	require.NoError(t, q.CreateSseOutboxRow(ctx, personal_sse_store.CreateSseOutboxRowParams{
		ID:           expiredID,
		Command:      cmdBroadcastToUser,
		TargetUserID: uuid.New(),
		Payload:      []byte("old"),
		ExpiresAt:    time.Now().Add(-time.Minute),
	}))
	require.NoError(t, q.CreateSseOutboxRow(ctx, personal_sse_store.CreateSseOutboxRowParams{
		ID:           freshID,
		Command:      cmdBroadcastToUser,
		TargetUserID: uuid.New(),
		Payload:      []byte("new"),
		ExpiresAt:    time.Now().Add(outboxTTL),
	}))
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM personal_sse_outbox WHERE id = ANY($1::uuid[])`, []uuid.UUID{expiredID, freshID})
	})

	_, err := q.DeleteExpiredSseOutboxBatch(ctx, 1000)
	require.NoError(t, err)
	_, err = q.GetSseOutboxRow(ctx, expiredID)
	assert.Error(t, err, "expired row must be swept")
	payload, err := q.GetSseOutboxRow(ctx, freshID)
	require.NoError(t, err, "fresh row must survive")
	assert.Equal(t, []byte("new"), payload)
}

func TestIntegration_SharedRowFanout(t *testing.T) {
	// Two nodes SELECT the same row independently; delete is idempotent.
	pool := integrationPool(t)
	ctx := context.Background()
	_, raw := bigTestEvent(t)
	id := uuid.New()
	require.NoError(t, insertOutboxRow(t, pool, ctx, id, postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: uuid.New(),
	}, raw))
	t.Cleanup(func() { deleteOutboxRow(t, pool, id) })

	first, err := fetchOutboxEvent(t, pool, ctx, id)
	require.NoError(t, err)
	second, err := fetchOutboxEvent(t, pool, ctx, id)
	require.NoError(t, err)
	assert.Equal(t, first, second, "both nodes must read identical bytes")

	deleteOutboxRow(t, pool, id)
	deleteOutboxRow(t, pool, id) // idempotent, must not error
	_, err = fetchOutboxEvent(t, pool, ctx, id)
	assert.Error(t, err)
}

func TestIntegration_MissingRowErrors(t *testing.T) {
	pool := integrationPool(t)
	_, err := fetchOutboxEvent(t, pool, context.Background(), uuid.New())
	assert.Error(t, err)
}

// listenConn holds one pool connection with LISTEN active. The caller
// publishes afterwards, so the notification cannot race the LISTEN.
func listenConn(t *testing.T, pool *pgxpool.Pool) *pgxpool.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	t.Cleanup(conn.Release)
	_, err = conn.Exec(ctx, "LISTEN "+sseNotificationChannel)
	require.NoError(t, err)
	return conn
}

func waitOne(t *testing.T, conn *pgxpool.Conn) postgresSsePayload {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	n, err := conn.Conn().WaitForNotification(ctx)
	require.NoError(t, err, "timed out waiting for NOTIFY")
	var p postgresSsePayload
	require.NoError(t, json.Unmarshal([]byte(n.Payload), &p))
	return p
}

func TestIntegration_RouteLargeEndToEnd(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	mgr := NewManager(pool)
	want, raw := bigTestEvent(t)
	require.True(t, shouldUseOutbox(postgresSsePayload{Command: cmdBroadcastToUser, TargetUserID: uuid.New()}, raw))

	target := uuid.New()
	conn := listenConn(t, pool)

	require.NoError(t, mgr.routeAndNotify(ctx, postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: target,
	}, raw))

	got := waitOne(t, conn)
	require.NotNil(t, got.OutboxID, "large event must arrive as pointer")
	t.Cleanup(func() { deleteOutboxRow(t, pool, *got.OutboxID) })
	assert.Empty(t, got.ProtoEventBase64)
	assert.Equal(t, target, got.TargetUserID)

	stored, err := fetchOutboxEvent(t, pool, ctx, *got.OutboxID)
	require.NoError(t, err)
	assert.Equal(t, raw, stored)
	back, err := unmarshalEventBytes(stored)
	require.NoError(t, err)
	assert.True(t, proto.Equal(want, back))
	deleteOutboxRow(t, pool, *got.OutboxID)
}

func TestIntegration_RouteSmallInline(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	mgr := NewManager(pool)
	want := smallTestEvent()
	raw, err := marshalEventBytes(want)
	require.NoError(t, err)
	require.False(t, shouldUseOutbox(postgresSsePayload{Command: cmdBroadcastToUser, TargetUserID: uuid.New()}, raw))

	target := uuid.New()
	conn := listenConn(t, pool)

	require.NoError(t, mgr.routeAndNotify(ctx, postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: target,
	}, raw))

	got := waitOne(t, conn)
	assert.Nil(t, got.OutboxID, "small event must stay inline")
	require.NotEmpty(t, got.ProtoEventBase64)
	back, err := unmarshalEvent(got.ProtoEventBase64)
	require.NoError(t, err)
	assert.True(t, proto.Equal(want, back))
}

func TestIntegration_WorkerDeliversToLocalSession(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	mgr := NewManager(pool)
	want, raw := bigTestEvent(t)

	userID := uuid.New()
	sessionID := uuid.New()
	conn, ok := mgr.Register(userID, sessionID, false)
	require.True(t, ok)
	defer mgr.Unregister(conn)

	id := uuid.New()
	require.NoError(t, insertOutboxRow(t, pool, ctx, id, postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: userID,
	}, raw))
	t.Cleanup(func() { deleteOutboxRow(t, pool, id) })

	// Through the queue, like the listener loop hands it over.
	require.True(t, mgr.enqueueOutbox(postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: userID,
		OutboxID:     &id,
	}))
	p := <-mgr.outboxQueue
	mgr.processOutboxPayload(ctx, p)

	select {
	case got := <-conn.Send:
		assert.True(t, proto.Equal(want, got))
	default:
		t.Fatal("local session did not receive worker-delivered event")
	}
}

// -------------------------------------------------------------
// Deep Edge-Case Integration Tests
// -------------------------------------------------------------

func TestIntegration_EdgeCase_PoisonPillCorruptedPayload(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	mgr := NewManager(pool)

	targetUser := uuid.New()
	sessID := uuid.New()
	conn, ok := mgr.Register(targetUser, sessID, false)
	require.True(t, ok)
	defer mgr.Unregister(conn)

	poisonID := uuid.New()
	err := insertOutboxRow(t, pool, ctx, poisonID, postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: targetUser,
	}, []byte("INVALID_CORRUPTED_NON_PROTOBUF_DATA"))
	require.NoError(t, err)
	defer deleteOutboxRow(t, pool, poisonID)

	assert.NotPanics(t, func() {
		mgr.processOutboxPayload(ctx, postgresSsePayload{
			Command:      cmdBroadcastToUser,
			TargetUserID: targetUser,
			OutboxID:     &poisonID,
		})
	})

	select {
	case <-conn.Send:
		t.Fatal("corrupt event should never be dispatched to client")
	default:
	}
}

func TestIntegration_EdgeCase_GhostOutboxID(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	mgr := NewManager(pool)

	targetUser := uuid.New()
	sessID := uuid.New()
	conn, ok := mgr.Register(targetUser, sessID, false)
	require.True(t, ok)
	defer mgr.Unregister(conn)

	ghostID := uuid.New()
	assert.NotPanics(t, func() {
		mgr.processOutboxPayload(ctx, postgresSsePayload{
			Command:      cmdBroadcastToUser,
			TargetUserID: targetUser,
			OutboxID:     &ghostID,
		})
	})

	select {
	case <-conn.Send:
		t.Fatal("ghost event should never be dispatched to client")
	default:
	}
}

func TestIntegration_EdgeCase_DisconnectRaceCondition(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	mgr := NewManager(pool)

	targetUser := uuid.New()
	sessID := uuid.New()
	conn, ok := mgr.Register(targetUser, sessID, false)
	require.True(t, ok)

	_, raw := bigTestEvent(t)
	outboxID := uuid.New()
	require.NoError(t, insertOutboxRow(t, pool, ctx, outboxID, postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: targetUser,
	}, raw))
	defer deleteOutboxRow(t, pool, outboxID)

	// User disconnects BEFORE worker runs
	mgr.Unregister(conn)

	mgr.processOutboxPayload(ctx, postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: targetUser,
		OutboxID:     &outboxID,
	})
}

func TestIntegration_EdgeCase_InterleavedLargeAndSmallEvents(t *testing.T) {
	pool := integrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mgr := NewManager(pool)

	var wg sync.WaitGroup
	for i := 0; i < outboxWorkerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.outboxWorker(ctx)
		}()
	}

	targetUser := uuid.New()
	sessID := uuid.New()
	conn, ok := mgr.Register(targetUser, sessID, false)
	require.True(t, ok)
	defer mgr.Unregister(conn)

	const eventPairs = 10
	largeIDs := make([]uuid.UUID, eventPairs)

	for i := 0; i < eventPairs; i++ {
		id := uuid.New()
		largeIDs[i] = id
		_, raw := bigTestEvent(t)
		require.NoError(t, insertOutboxRow(t, pool, ctx, id, postgresSsePayload{
			Command:      cmdBroadcastToUser,
			TargetUserID: targetUser,
		}, raw))
	}
	defer func() {
		for _, id := range largeIDs {
			deleteOutboxRow(t, pool, id)
		}
	}()

	for i := 0; i < eventPairs; i++ {
		id := largeIDs[i]
		require.True(t, mgr.enqueueOutbox(postgresSsePayload{
			Command:      cmdBroadcastToUser,
			TargetUserID: targetUser,
			OutboxID:     &id,
		}))

		smallEvt := smallTestEvent()
		mgr.dispatchResolved(postgresSsePayload{
			Command:      cmdBroadcastToUser,
			TargetUserID: targetUser,
		}, smallEvt)
	}

	var smallCount, largeCount int
	timeout := time.After(15 * time.Second)
	for i := 0; i < eventPairs*2; i++ {
		select {
		case evt := <-conn.Send:
			require.NotNil(t, evt)
			chatMod := evt.GetChatModule()
			if chatMod.GetSendMessageSseEvent() != nil {
				largeCount++
			} else if chatMod.GetAcknowledgeDeliverySseEvent() != nil {
				smallCount++
			}
		case <-timeout:
			t.Fatalf("timed out: received %d small and %d large events", smallCount, largeCount)
		}
	}

	cancel()
	wg.Wait()

	assert.Equal(t, eventPairs, smallCount, "all small events must be received")
	assert.Equal(t, eventPairs, largeCount, "all large events must be received")
}

func TestIntegration_EdgeCase_ZeroLeakedConnections(t *testing.T) {
	pool := integrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	mgr := NewManager(pool)

	var wg sync.WaitGroup
	for i := 0; i < outboxWorkerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.outboxWorker(ctx)
		}()
	}

	targetUser := uuid.New()
	sessID := uuid.New()
	conn, ok := mgr.Register(targetUser, sessID, false)
	require.True(t, ok)
	defer mgr.Unregister(conn)

	const totalMsgs = 30
	ids := make([]uuid.UUID, totalMsgs)
	for i := 0; i < totalMsgs; i++ {
		id := uuid.New()
		ids[i] = id
		_, raw := bigTestEvent(t)
		require.NoError(t, insertOutboxRow(t, pool, ctx, id, postgresSsePayload{
			Command:      cmdBroadcastToUser,
			TargetUserID: targetUser,
		}, raw))
	}
	defer func() {
		for _, id := range ids {
			deleteOutboxRow(t, pool, id)
		}
	}()

	for _, id := range ids {
		currID := id
		require.True(t, mgr.enqueueOutbox(postgresSsePayload{
			Command:      cmdBroadcastToUser,
			TargetUserID: targetUser,
			OutboxID:     &currID,
		}))
	}

	for i := 0; i < totalMsgs; i++ {
		select {
		case <-conn.Send:
		case <-time.After(20 * time.Second):
			t.Fatalf("timed out at msg %d", i)
		}
	}

	cancel()
	wg.Wait()

	stats := pool.Stat()
	assert.Equal(t, int32(0), stats.AcquiredConns(), "must have 0 acquired/leaked connections")
}

func TestIntegration_EdgeCase_MultiSessionTargetingAndExclusion(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	mgr := NewManager(pool)

	targetUser := uuid.New()
	phoneSess := uuid.New()
	desktopSess := uuid.New()
	tabletSess := uuid.New()

	phoneConn, ok1 := mgr.Register(targetUser, phoneSess, false)
	desktopConn, ok2 := mgr.Register(targetUser, desktopSess, false)
	tabletConn, ok3 := mgr.Register(targetUser, tabletSess, false)
	require.True(t, ok1 && ok2 && ok3)
	defer func() {
		mgr.Unregister(phoneConn)
		mgr.Unregister(desktopConn)
		mgr.Unregister(tabletConn)
	}()

	_, raw := bigTestEvent(t)

	exceptID := uuid.New()
	require.NoError(t, insertOutboxRow(t, pool, ctx, exceptID, postgresSsePayload{
		Command:            cmdBroadcastToUserExcept,
		TargetUserID:       targetUser,
		ExcludeSessionUUID: &phoneSess,
	}, raw))
	defer deleteOutboxRow(t, pool, exceptID)

	mgr.processOutboxPayload(ctx, postgresSsePayload{
		Command:            cmdBroadcastToUserExcept,
		TargetUserID:       targetUser,
		ExcludeSessionUUID: &phoneSess,
		OutboxID:           &exceptID,
	})

	select {
	case <-phoneConn.Send:
		t.Fatal("excluded phone session received event!")
	default:
	}

	select {
	case evt := <-desktopConn.Send:
		require.NotNil(t, evt)
	case <-time.After(2 * time.Second):
		t.Fatal("desktop session timed out waiting for event")
	}
	select {
	case evt := <-tabletConn.Send:
		require.NotNil(t, evt)
	case <-time.After(2 * time.Second):
		t.Fatal("tablet session timed out waiting for event")
	}

	targetSessionID := uuid.New()
	require.NoError(t, insertOutboxRow(t, pool, ctx, targetSessionID, postgresSsePayload{
		Command:           cmdBroadcastToUserSession,
		TargetUserID:      targetUser,
		TargetSessionUUID: &tabletSess,
	}, raw))
	defer deleteOutboxRow(t, pool, targetSessionID)

	mgr.processOutboxPayload(ctx, postgresSsePayload{
		Command:           cmdBroadcastToUserSession,
		TargetUserID:      targetUser,
		TargetSessionUUID: &tabletSess,
		OutboxID:          &targetSessionID,
	})

	select {
	case <-phoneConn.Send:
		t.Fatal("phone received targeted session event!")
	default:
	}
	select {
	case <-desktopConn.Send:
		t.Fatal("desktop received targeted session event!")
	default:
	}

	select {
	case evt := <-tabletConn.Send:
		require.NotNil(t, evt)
	case <-time.After(2 * time.Second):
		t.Fatal("targeted tablet session timed out waiting for event")
	}
}

func TestIntegration_EdgeCase_RapidSessionFlapping(t *testing.T) {
	pool := integrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mgr := NewManager(pool)

	var wg sync.WaitGroup
	for i := 0; i < outboxWorkerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.outboxWorker(ctx)
		}()
	}

	targetUser := uuid.New()
	_, raw := bigTestEvent(t)
	outboxID := uuid.New()
	require.NoError(t, insertOutboxRow(t, pool, ctx, outboxID, postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: targetUser,
	}, raw))
	defer deleteOutboxRow(t, pool, outboxID)

	var stopFlapping atomic.Bool
	var flapperWg sync.WaitGroup

	for i := 0; i < 5; i++ {
		flapperWg.Add(1)
		go func() {
			defer flapperWg.Done()
			for !stopFlapping.Load() {
				sessID := uuid.New()
				conn, ok := mgr.Register(targetUser, sessID, false)
				if ok {
					time.Sleep(100 * time.Microsecond)
					mgr.Unregister(conn)
				}
			}
		}()
	}

	for i := 0; i < 30; i++ {
		currID := outboxID
		mgr.enqueueOutbox(postgresSsePayload{
			Command:      cmdBroadcastToUser,
			TargetUserID: targetUser,
			OutboxID:     &currID,
		})
		time.Sleep(10 * time.Millisecond)
	}

	stopFlapping.Store(true)
	flapperWg.Wait()
	cancel()
	wg.Wait()
}

func TestIntegration_CeilingRejectsBeforePersistence(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	mgr := NewManager(pool)

	countRows := func() int64 {
		var n int64
		require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM personal_sse_outbox`).Scan(&n))
		return n
	}

	oversized := make([]byte, maxEventBytes+1024)
	before := countRows()
	err := mgr.routeAndNotify(ctx, postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: uuid.New(),
	}, oversized)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ceiling")
	assert.Equal(t, before, countRows(), "rejected event must persist nothing")
}
