package personal_sse

import (
	"context"
	"log"

	rpc_personal_ssev1 "chatbasket-api/gen/proto/personal/personal_sse"
	"chatbasket-api/internal/platform/connect_sse"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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
	pool *pgxpool.Pool
}

// NewManager instantiates a new Manager for personal SSE streams.
func NewManager(pool *pgxpool.Pool) *Manager {
	return &Manager{
		Manager: connect_sse.NewManager[*rpc_personal_ssev1.PersonalSseEvent](),
		pool:    pool,
	}
}

// BroadcastToUser publishes to Postgres NOTIFY across all cluster nodes.
func (m *Manager) BroadcastToUser(userID uuid.UUID, event *rpc_personal_ssev1.PersonalSseEvent) {
	if m == nil || m.pool == nil {
		return
	}
	if err := publishBroadcastToUser(context.Background(), m.pool, userID, event); err != nil {
		log.Printf("[personal_sse] Failed to publish cluster broadcast for user %s: %v", userID, err)
	}
}

// BroadcastToUserExcept publishes to Postgres NOTIFY excluding excludeSessionUUID.
func (m *Manager) BroadcastToUserExcept(userID uuid.UUID, excludeSessionUUID uuid.UUID, event *rpc_personal_ssev1.PersonalSseEvent) {
	if m == nil || m.pool == nil {
		return
	}
	if err := publishBroadcastToUserExcept(context.Background(), m.pool, userID, excludeSessionUUID, event); err != nil {
		log.Printf("[personal_sse] Failed to publish cluster broadcast except session %s (user %s): %v", excludeSessionUUID, userID, err)
	}
}

// BroadcastToUserSession publishes to Postgres NOTIFY targeting a single specific session across all nodes.
func (m *Manager) BroadcastToUserSession(userID uuid.UUID, targetSessionUUID uuid.UUID, event *rpc_personal_ssev1.PersonalSseEvent) {
	if m == nil || m.pool == nil {
		return
	}
	if err := publishBroadcastToUserSession(context.Background(), m.pool, userID, targetSessionUUID, event); err != nil {
		log.Printf("[personal_sse] Failed to publish cluster broadcast for session %s (user %s): %v", targetSessionUUID, userID, err)
	}
}

// UnregisterSession tells all cluster nodes to close a specific session.
func (m *Manager) UnregisterSession(userID uuid.UUID, sessionUUID uuid.UUID) {
	if m == nil || m.pool == nil {
		return
	}
	if err := publishUnregisterSession(context.Background(), m.pool, userID, sessionUUID); err != nil {
		log.Printf("[personal_sse] Failed to publish unregister session %s (user %s): %v", sessionUUID, userID, err)
	}
}

// UnregisterUserConnections tells all cluster nodes to close all sessions for a user.
func (m *Manager) UnregisterUserConnections(userID uuid.UUID) {
	if m == nil || m.pool == nil {
		return
	}
	if err := publishUnregisterUserConnections(context.Background(), m.pool, userID); err != nil {
		log.Printf("[personal_sse] Failed to publish unregister user %s: %v", userID, err)
	}
}
