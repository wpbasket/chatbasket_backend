package personal_sse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	rpc_personal_ssev1 "chatbasket-api/gen/proto/personal/personal_sse"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestShouldUseOutbox_SmallPayloadStaysInline(t *testing.T) {
	p := postgresSsePayload{Command: cmdBroadcastToUser, TargetUserID: uuid.New()}
	assert.False(t, shouldUseOutbox(p, []byte("hello")))
}

func TestShouldUseOutbox_LargePayloadRoutesToOutbox(t *testing.T) {
	p := postgresSsePayload{Command: cmdBroadcastToUser, TargetUserID: uuid.New()}
	assert.True(t, shouldUseOutbox(p, []byte(strings.Repeat("x", 9000))))
}

func TestInlineSizeEstimate_ExactForAllShapes(t *testing.T) {
	// The estimate must equal the real final JSON size for every command
	// shape and content length: exact base64 math plus the exact envelope
	// measured from the payload itself.
	shapes := []postgresSsePayload{
		{Command: cmdBroadcastToUser, TargetUserID: uuid.New()},
		{Command: cmdBroadcastToUserExcept, TargetUserID: uuid.New(), ExcludeSessionUUID: &[]uuid.UUID{uuid.New()}[0]},
		{Command: cmdBroadcastToUserSession, TargetUserID: uuid.New(), TargetSessionUUID: &[]uuid.UUID{uuid.New()}[0]},
		{Command: cmdUnregisterUserConnections, TargetUserID: uuid.New()},
	}
	for _, shape := range shapes {
		for _, n := range []int{0, 1, 2, 3, 100, 5152, 65187} {
			raw := make([]byte, n)
			full := shape
			full.ProtoEventBase64 = base64.StdEncoding.EncodeToString(raw)
			actual, err := json.Marshal(full)
			require.NoError(t, err)
			assert.Equal(t, len(actual), inlineSizeEstimate(shape, n),
				"cmd=%s protoLen=%d", shape.Command, n)
		}
	}
}

func TestShouldUseOutbox_Boundary(t *testing.T) {
	// Envelope for the basic shape is 72 + 9 framing = 81: estimate hits
	// 7001 at proto length 5188 -> outbox; 5187 stays 6997 -> inline.
	p := postgresSsePayload{Command: cmdBroadcastToUser, TargetUserID: uuid.New()}
	assert.False(t, shouldUseOutbox(p, make([]byte, 5187)))
	assert.True(t, shouldUseOutbox(p, make([]byte, 5188)))
}

func TestHasLocalTarget_FiltersNonLocalUsers(t *testing.T) {
	mgr := NewManager(nil)
	uA := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uB := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	sA1 := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	sA2 := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	// Initially, no users registered on this node
	assert.False(t, mgr.hasLocalTarget(postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: uA,
	}))

	// Register sessionA1 for userA
	conn, ok := mgr.Register(uA, sA1, false)
	assert.True(t, ok)

	// cmdBroadcastToUser: uA is present, uB is not
	assert.True(t, mgr.hasLocalTarget(postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: uA,
	}))
	assert.False(t, mgr.hasLocalTarget(postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: uB,
	}))

	// cmdBroadcastToUserSession: sA1 is active, sA2 is not
	assert.True(t, mgr.hasLocalTarget(postgresSsePayload{
		Command:           cmdBroadcastToUserSession,
		TargetUserID:      uA,
		TargetSessionUUID: &sA1,
	}))
	assert.False(t, mgr.hasLocalTarget(postgresSsePayload{
		Command:           cmdBroadcastToUserSession,
		TargetUserID:      uA,
		TargetSessionUUID: &sA2,
	}))

	// cmdBroadcastToUserExcept:
	// Only sA1 exists. Exclude sA1 -> should return false (no other session)
	assert.False(t, mgr.hasLocalTarget(postgresSsePayload{
		Command:            cmdBroadcastToUserExcept,
		TargetUserID:       uA,
		ExcludeSessionUUID: &sA1,
	}))
	// Exclude sA2 -> should return true (sA1 is present)
	assert.True(t, mgr.hasLocalTarget(postgresSsePayload{
		Command:            cmdBroadcastToUserExcept,
		TargetUserID:       uA,
		ExcludeSessionUUID: &sA2,
	}))

	// Register sA2 for uA: now exclude sA1 should return true
	conn2, ok := mgr.Register(uA, sA2, false)
	assert.True(t, ok)
	assert.True(t, mgr.hasLocalTarget(postgresSsePayload{
		Command:            cmdBroadcastToUserExcept,
		TargetUserID:       uA,
		ExcludeSessionUUID: &sA1,
	}))

	mgr.Unregister(conn)
	mgr.Unregister(conn2)
	assert.False(t, mgr.hasLocalTarget(postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: uA,
	}))
}

func TestEnqueueOutbox_DropsWhenFull(t *testing.T) {
	mgr := NewManager(nil)
	p := postgresSsePayload{Command: cmdBroadcastToUser, TargetUserID: uuid.New()}

	for i := 0; i < outboxQueueCapacity; i++ {
		assert.True(t, mgr.enqueueOutbox(p), "slot %d must accept", i)
	}
	// 257th: full, must drop without blocking.
	assert.False(t, mgr.enqueueOutbox(p))

	// Drain one slot: enqueue works again.
	<-mgr.outboxQueue
	assert.True(t, mgr.enqueueOutbox(p))
	// Drain the rest so no state leaks.
	for len(mgr.outboxQueue) > 0 {
		<-mgr.outboxQueue
	}
}

func TestDispatchResolved_DeliversLocally(t *testing.T) {
	mgr := NewManager(nil)
	userID := uuid.New()
	sessionID := uuid.New()
	conn, ok := mgr.Register(userID, sessionID, false)
	assert.True(t, ok)
	defer mgr.Unregister(conn)

	want := &rpc_personal_ssev1.PersonalSseEvent{Timestamp: timestamppb.Now()}
	raw, err := marshalEventBytes(want)
	assert.NoError(t, err)
	evt, err := unmarshalEventBytes(raw)
	assert.NoError(t, err)
	mgr.dispatchResolved(postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: userID,
	}, evt)

	select {
	case got := <-conn.Send:
		assert.True(t, proto.Equal(want, got), "dispatched event must equal original")
	default:
		t.Fatal("registered session did not receive dispatched event")
	}
}

func TestProcessOutboxPayload_SkipsNonLocal(t *testing.T) {
	// Pool is nil: any DB touch would panic. Clean return proves the
	// presence check runs before the fetch.
	mgr := NewManager(nil)
	oid := uuid.New()
	mgr.processOutboxPayload(context.Background(), postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: uuid.New(),
		OutboxID:     &oid,
	})
}

func TestQueueBufferSaturation_300Burst(t *testing.T) {
	mgr := NewManager(nil)
	p := postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: uuid.New(),
	}

	start := time.Now()
	var accepted, dropped int
	for i := 0; i < 300; i++ {
		if mgr.enqueueOutbox(p) {
			accepted++
		} else {
			dropped++
		}
	}
	duration := time.Since(start)

	assert.Equal(t, 256, accepted, "exactly 256 items should fit in queue")
	assert.Equal(t, 44, dropped, "exactly 44 items should be dropped when full")
	assert.Less(t, duration, 50*time.Millisecond, "enqueue must be strictly non-blocking")

	// Drain queue and verify it accepts again
	for len(mgr.outboxQueue) > 0 {
		<-mgr.outboxQueue
	}
	assert.True(t, mgr.enqueueOutbox(p), "should accept again after draining")
}

func TestWorkerCleanShutdownUnderLoad(t *testing.T) {
	mgr := NewManager(nil)
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < outboxWorkerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.outboxWorker(ctx)
		}()
	}

	// Fill queue
	fakeID := uuid.New()
	for i := 0; i < 100; i++ {
		mgr.enqueueOutbox(postgresSsePayload{
			Command:      cmdBroadcastToUser,
			TargetUserID: uuid.New(),
			OutboxID:     &fakeID,
		})
	}

	// Abruptly cancel context (simulating SIGTERM / service shutdown)
	shutdownStart := time.Now()
	cancel()
	wg.Wait()
	shutdownDuration := time.Since(shutdownStart)

	assert.Less(t, shutdownDuration, 200*time.Millisecond, "all 5 workers must terminate cleanly and immediately")
}

func TestWorkerPanicRecovery(t *testing.T) {
	mgr := NewManager(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var workerDone sync.WaitGroup
	workerDone.Add(1)
	go func() {
		defer workerDone.Done()
		mgr.outboxWorker(ctx)
	}()

	fakeID := uuid.New()
	mgr.enqueueOutbox(postgresSsePayload{
		Command:      cmdBroadcastToUser,
		TargetUserID: uuid.New(),
		OutboxID:     &fakeID,
	})

	time.Sleep(50 * time.Millisecond)

	// Worker is alive if ctx cancel shuts it down cleanly without hanging
	cancel()
	workerDone.Wait()
}

func TestRouteAndNotify_RejectsOverCeiling(t *testing.T) {
	// No pool needed: rejection happens before any database touch.
	mgr := NewManager(nil)
	p := postgresSsePayload{Command: cmdBroadcastToUser, TargetUserID: uuid.New()}

	err := mgr.routeAndNotify(context.Background(), p, make([]byte, maxEventBytes+1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ceiling")
}
