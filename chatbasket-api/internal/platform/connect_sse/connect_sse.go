package connect_sse

import (
	"sync"

	"github.com/google/uuid"
)

const (
	maxConnsPerUser   = 5
	channelBufferSize = 128
)

type Conn[T any] struct {
	UserID      uuid.UUID
	SessionUUID uuid.UUID
	IsPrimary   bool
	Send        chan T
	closeOnce   sync.Once
}

// Close idempotently closes the Send channel (safe to call multiple times, never panics).
func (c *Conn[T]) Close() {
	c.closeOnce.Do(func() {
		close(c.Send)
	})
}

type Manager[T any] struct {
	mu    sync.RWMutex
	conns map[uuid.UUID]map[uuid.UUID]*Conn[T]
}

func NewManager[T any]() *Manager[T] {
	return &Manager[T]{
		conns: make(map[uuid.UUID]map[uuid.UUID]*Conn[T]),
	}
}

// Register registers a new SSE stream connection for a user session.
func (m *Manager[T]) Register(userID uuid.UUID, sessionUUID uuid.UUID, isPrimary bool) (*Conn[T], bool) {
	if sessionUUID == uuid.Nil {
		sessionUUID = uuid.New()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	userConns, exists := m.conns[userID]
	if !exists {
		userConns = make(map[uuid.UUID]*Conn[T])
		m.conns[userID] = userConns
	}

	// Strict 1 Session = 1 SSE Stream rule:
	// If an SSE stream for this exact SessionUUID already exists (e.g. client reconnected or IP changed),
	// forcibly close the previous connection immediately before registering the new one.
	if old, ok := userConns[sessionUUID]; ok {
		old.Close()
	} else if len(userConns) >= maxConnsPerUser {
		return nil, false
	}

	conn := &Conn[T]{
		UserID:      userID,
		SessionUUID: sessionUUID,
		IsPrimary:   isPrimary,
		Send:        make(chan T, channelBufferSize),
	}

	userConns[sessionUUID] = conn
	return conn, true
}

// Unregister unregisters and closes an active SSE connection.
func (m *Manager[T]) Unregister(conn *Conn[T]) {
	if conn == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	userConns, exists := m.conns[conn.UserID]
	if !exists {
		return
	}

	if existing, ok := userConns[conn.SessionUUID]; ok && existing == conn {
		delete(userConns, conn.SessionUUID)
		conn.Close()

		if len(userConns) == 0 {
			delete(m.conns, conn.UserID)
		}
	}
}

// UnregisterUserConnections closes and unregisters all active SSE connections for a user.
// WS Equivalent: WSHub.CloseUserConnections(userID uuid.UUID)
func (m *Manager[T]) UnregisterUserConnections(userID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userConns, exists := m.conns[userID]
	if !exists {
		return
	}

	for suid, conn := range userConns {
		conn.Close()
		delete(userConns, suid)
	}
	delete(m.conns, userID)
}

// UnregisterSession closes and unregisters a specific session connection for a user.
// WS Equivalent: WSHub.CloseSessionConnection(userID uuid.UUID, sessionID string)
func (m *Manager[T]) UnregisterSession(userID uuid.UUID, sessionUUID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userConns, exists := m.conns[userID]
	if !exists {
		return
	}

	if conn, ok := userConns[sessionUUID]; ok {
		delete(userConns, sessionUUID)
		conn.Close()

		if len(userConns) == 0 {
			delete(m.conns, userID)
		}
	}
}

// IsSessionActive checks if the specified session is currently connected to THIS local server node.
// Note: In a multi-node cluster, a session might be connected to a peer node. Because one-way Postgres
// NOTIFY cannot perform synchronous request-response presence queries, cluster operations (such as
// history sync and targeted device push) should use optimistic cluster broadcast (BroadcastToUserSession)
// rather than relying on synchronous IsSessionActive pre-checks.
func (m *Manager[T]) IsSessionActive(userID uuid.UUID, sessionUUID uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userConns, exists := m.conns[userID]
	if !exists {
		return false
	}

	_, active := userConns[sessionUUID]
	return active
}

// BroadcastToUser sends an SSE event to ALL connected sessions of a user on this node.
func (m *Manager[T]) BroadcastToUser(userID uuid.UUID, event T) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userConns, exists := m.conns[userID]
	if !exists {
		return
	}

	for _, conn := range userConns {
		select {
		case conn.Send <- event:
		default:
		}
	}
}

// BroadcastToUserExcept sends an SSE event to all connected sessions of a user EXCEPT excludeSessionUUID.
func (m *Manager[T]) BroadcastToUserExcept(userID uuid.UUID, excludeSessionUUID uuid.UUID, event T) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userConns, exists := m.conns[userID]
	if !exists {
		return
	}

	for suid, conn := range userConns {
		if suid == excludeSessionUUID {
			continue
		}
		select {
		case conn.Send <- event:
		default:
		}
	}
}

// BroadcastToUserSession sends an SSE event directly to a single specific session of a user.
func (m *Manager[T]) BroadcastToUserSession(userID uuid.UUID, targetSessionUUID uuid.UUID, event T) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userConns, exists := m.conns[userID]
	if !exists {
		return
	}

	conn, ok := userConns[targetSessionUUID]
	if !ok {
		return
	}

	select {
	case conn.Send <- event:
	default:
	}
}
