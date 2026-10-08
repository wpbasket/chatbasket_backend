package personal_sse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"time"

	rpc_personal_ssev1 "chatbasket-api/gen/proto/personal/personal_sse"
	"chatbasket-api/internal/modules/personal/personal_sse/internal/personal_sse_store"
	"chatbasket-api/internal/platform/connect_sse"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

// Manager embeds connect_sse.Manager and adds cluster-wide Postgres NOTIFY broadcasting.
//
// By embedding *connect_sse.Manager, the following 3 local-node methods are automatically promoted
// and DO NOT require any redundant wrapper methods:
//  1. Register(userID, sessionUUID, isPrimary)    -> registers a new stream connection on this node
//  2. Unregister(conn)                           -> unregisters and closes a connection on this node
//  3. IsSessionActive(userID, sessionUUID)        -> checks if a session is currently active on this node
//
// Only the 5 methods below are explicitly overridden on Manager to dispatch cluster-wide pg_notify
// commands across all server nodes in a multi-node cluster.
type Manager struct {
	*connect_sse.Manager[*rpc_personal_ssev1.PersonalSseEvent]
	pool            *pgxpool.Pool
	postgresQueries *personal_sse_store.Queries
	// outboxQueue decouples NOTIFY reception from DB hydration so one
	// slow 65KB fetch never blocks urgent small events behind it.
	// Delivery is at-most-once per node: a full queue drops (log only)
	// and the client falls back to polling. Large events may hydrate
	// out of order across workers; clients sort by created_at.
	outboxQueue chan postgresSsePayload
}

// outboxQueueCapacity bounds queued large events (~25KB of pointers).
// outboxWorkerCount fits the pool budget: 2 LISTEN + 33 HTTP + 5 = 40.
const (
	outboxQueueCapacity = 256
	outboxWorkerCount   = 5
)

// NewManager instantiates a new Manager for personal SSE streams.
func NewManager(pool *pgxpool.Pool) *Manager {
	return &Manager{
		Manager:         connect_sse.NewManager[*rpc_personal_ssev1.PersonalSseEvent](),
		pool:            pool,
		postgresQueries: personal_sse_store.New(pool),
		outboxQueue:     make(chan postgresSsePayload, outboxQueueCapacity),
	}
}

// publishAndLog runs fn with a publishTimeout deadline and logs failures.
// One place owns timeout creation so the 5 broadcast entry points stay one-liners.
func (m *Manager) publishAndLog(msg string, fn func(ctx context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	if err := fn(ctx); err != nil {
		log.Printf("[personal_sse] %s: %v", msg, err)
	}
}

// BroadcastToUser publishes to Postgres NOTIFY across all cluster nodes.
func (m *Manager) BroadcastToUser(userID uuid.UUID, event *rpc_personal_ssev1.PersonalSseEvent) {
	if m == nil || m.pool == nil {
		return
	}
	m.publishAndLog("Failed to publish cluster broadcast", func(ctx context.Context) error {
		if event == nil {
			return nil
		}
		eventBytes, err := marshalEventBytes(event)
		if err != nil {
			return err
		}
		return m.routeAndNotify(ctx, postgresSsePayload{
			Command:      cmdBroadcastToUser,
			TargetUserID: userID,
		}, eventBytes)
	})
}

// BroadcastToUserExcept publishes to Postgres NOTIFY excluding excludeSessionUUID.
func (m *Manager) BroadcastToUserExcept(userID uuid.UUID, excludeSessionUUID uuid.UUID, event *rpc_personal_ssev1.PersonalSseEvent) {
	if m == nil || m.pool == nil {
		return
	}
	m.publishAndLog("Failed to publish cluster broadcast except session", func(ctx context.Context) error {
		if event == nil {
			return nil
		}
		eventBytes, err := marshalEventBytes(event)
		if err != nil {
			return err
		}
		return m.routeAndNotify(ctx, postgresSsePayload{
			Command:            cmdBroadcastToUserExcept,
			TargetUserID:       userID,
			ExcludeSessionUUID: &excludeSessionUUID,
		}, eventBytes)
	})
}

// BroadcastToUserSession publishes to Postgres NOTIFY targeting a single specific session across all nodes.
func (m *Manager) BroadcastToUserSession(userID uuid.UUID, targetSessionUUID uuid.UUID, event *rpc_personal_ssev1.PersonalSseEvent) {
	if m == nil || m.pool == nil {
		return
	}
	m.publishAndLog("Failed to publish cluster broadcast for session", func(ctx context.Context) error {
		if event == nil {
			return nil
		}
		eventBytes, err := marshalEventBytes(event)
		if err != nil {
			return err
		}
		return m.routeAndNotify(ctx, postgresSsePayload{
			Command:           cmdBroadcastToUserSession,
			TargetUserID:      userID,
			TargetSessionUUID: &targetSessionUUID,
		}, eventBytes)
	})
}

// UnregisterSession tells all cluster nodes to close a specific session.
func (m *Manager) UnregisterSession(userID uuid.UUID, sessionUUID uuid.UUID) {
	if m == nil || m.pool == nil {
		return
	}
	m.publishAndLog("Failed to publish unregister session", func(ctx context.Context) error {
		return m.notify(ctx, postgresSsePayload{
			Command:           cmdUnregisterSession,
			TargetUserID:      userID,
			TargetSessionUUID: &sessionUUID,
		})
	})
}

// UnregisterUserConnections tells all cluster nodes to close all sessions for a user.
func (m *Manager) UnregisterUserConnections(userID uuid.UUID) {
	if m == nil || m.pool == nil {
		return
	}
	m.publishAndLog("Failed to publish unregister user", func(ctx context.Context) error {
		return m.notify(ctx, postgresSsePayload{
			Command:      cmdUnregisterUserConnections,
			TargetUserID: userID,
		})
	})
}

func (m *Manager) notify(ctx context.Context, payload postgresSsePayload) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal postgres sse payload: %w", err)
	}
	_, err = m.pool.Exec(ctx, "SELECT pg_notify($1, $2)", sseNotificationChannel, string(payloadBytes))
	return err
}

// marshalEventBytes turns the event into bytes. Small events get wrapped
// into text later; big events go into the table exactly as these bytes.
func marshalEventBytes(event *rpc_personal_ssev1.PersonalSseEvent) ([]byte, error) {
	eventBytes, err := proto.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal sse event: %w", err)
	}
	return eventBytes, nil
}

// routeAndNotify picks the road. Small event: pack it as text inside the
// NOTIFY itself. Big event: save the bytes in the outbox table and send
// only the row id, because Postgres drops NOTIFY messages over 8000 bytes.
func (m *Manager) routeAndNotify(ctx context.Context, payload postgresSsePayload, eventBytes []byte) error {
	if len(eventBytes) > maxEventBytes {
		return fmt.Errorf("sse event size %d exceeds %d ceiling", len(eventBytes), maxEventBytes)
	}
	if !shouldUseOutbox(payload, eventBytes) {
		evtB64 := base64.StdEncoding.EncodeToString(eventBytes)
		payload.ProtoEventBase64 = evtB64
		return m.notify(ctx, payload)
	}
	// From here on, payload is only the pointer: the bytes already have
	// a home in eventBytes for the table insert below.
	outboxID := uuid.New()
	payload.OutboxID = &outboxID
	payload.ProtoEventBase64 = ""
	if err := m.postgresQueries.CreateSseOutboxRow(ctx, personal_sse_store.CreateSseOutboxRowParams{
		ID:               outboxID,
		Command:          payload.Command,
		TargetUserID:     payload.TargetUserID,
		TargetSessionID:  payload.TargetSessionUUID,
		ExcludeSessionID: payload.ExcludeSessionUUID,
		Payload:          eventBytes,
		ExpiresAt:        time.Now().Add(outboxTTL),
	}); err != nil {
		return err
	}
	return m.notify(ctx, payload)
}
