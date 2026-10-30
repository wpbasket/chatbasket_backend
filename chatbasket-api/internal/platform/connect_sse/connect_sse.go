package connect_sse

import (
	"sync"

	"github.com/google/uuid"
)

const (
	// SSEConnectionIDHeader carries the client-generated SSE stream identity.
	// Re-registering with the same value replaces that exact stream instead
	// of occupying a new slot.
	SSEConnectionIDHeader = "x-sse-connection-id"

	// MaxConnsPerSession caps concurrent SSE streams sharing one session ID.
	// Browser tabs share the session cookie but each tab opens its own stream.
	MaxConnsPerSession = 3

	// maxSessionsPerUser caps distinct signed-in sessions (devices) per user.
	maxSessionsPerUser = 5

	channelBufferSize = 128
)

type Conn[T any] struct {
	UserID       uuid.UUID
	SessionUUID  uuid.UUID
	ConnectionID uuid.UUID
	IsPrimary    bool
	Send         chan T

	// seq is the registration order, used to pick the oldest stream for
	// session-targeted events unless every stream is explicitly requested.
	seq       uint64
	closeOnce sync.Once
}

// Close idempotently closes the Send channel (safe to call multiple times, never panics).
func (c *Conn[T]) Close() {
	c.closeOnce.Do(func() {
		close(c.Send)
	})
}

type Manager[T any] struct {
	mu    sync.RWMutex
	conns map[uuid.UUID]map[uuid.UUID]map[uuid.UUID]*Conn[T] // user -> session -> connection
	seq   uint64
}

func NewManager[T any]() *Manager[T] {
	return &Manager[T]{
		conns: make(map[uuid.UUID]map[uuid.UUID]map[uuid.UUID]*Conn[T]),
	}
}

// Register registers a new SSE stream connection.
//
// A session ID can hold up to MaxConnsPerSession streams because browser tabs
// share one session token while each tab opens its own stream. Registering an
// existing ConnectionID replaces exactly that connection (reconnect / reload)
// instead of consuming another slot. connectionID is required; the stream
// handler rejects requests without the x-sse-connection-id header, and a nil value
// here is a caller bug, so it is rejected rather than turned into a shared key.
func (m *Manager[T]) Register(userID uuid.UUID, sessionUUID uuid.UUID, connectionID uuid.UUID, isPrimary bool) (*Conn[T], bool) {
	if connectionID == uuid.Nil {
		return nil, false
	}
	if sessionUUID == uuid.Nil {
		sessionUUID = uuid.New()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	userConns, exists := m.conns[userID]
	if !exists {
		userConns = make(map[uuid.UUID]map[uuid.UUID]*Conn[T])
		m.conns[userID] = userConns
	}

	sessionConns, exists := userConns[sessionUUID]
	if !exists {
		if len(userConns) >= maxSessionsPerUser {
			return nil, false
		}
		sessionConns = make(map[uuid.UUID]*Conn[T])
		userConns[sessionUUID] = sessionConns
	}

	if old, ok := sessionConns[connectionID]; ok {
		old.Close()
	} else if len(sessionConns) >= MaxConnsPerSession {
		return nil, false
	}

	m.seq++
	conn := &Conn[T]{
		UserID:       userID,
		SessionUUID:  sessionUUID,
		ConnectionID: connectionID,
		IsPrimary:    isPrimary,
		Send:         make(chan T, channelBufferSize),
		seq:          m.seq,
	}

	sessionConns[connectionID] = conn
	return conn, true
}

// Unregister unregisters and closes one SSE connection.
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

	sessionConns, exists := userConns[conn.SessionUUID]
	if !exists {
		return
	}

	if existing, ok := sessionConns[conn.ConnectionID]; ok && existing == conn {
		delete(sessionConns, conn.ConnectionID)
		conn.Close()

		if len(sessionConns) == 0 {
			delete(userConns, conn.SessionUUID)
			if len(userConns) == 0 {
				delete(m.conns, conn.UserID)
			}
		}
	}
}

// UnregisterUserConnections closes and unregisters all active SSE connections for a user.
func (m *Manager[T]) UnregisterUserConnections(userID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userConns, exists := m.conns[userID]
	if !exists {
		return
	}

	for sessionUUID, sessionConns := range userConns {
		for connectionID, conn := range sessionConns {
			conn.Close()
			delete(sessionConns, connectionID)
		}
		delete(userConns, sessionUUID)
	}
	delete(m.conns, userID)
}

// UnregisterSession closes and unregisters every stream of a session.
func (m *Manager[T]) UnregisterSession(userID uuid.UUID, sessionUUID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userConns, exists := m.conns[userID]
	if !exists {
		return
	}

	sessionConns, exists := userConns[sessionUUID]
	if !exists {
		return
	}

	for connectionID, conn := range sessionConns {
		conn.Close()
		delete(sessionConns, connectionID)
	}
	delete(userConns, sessionUUID)

	if len(userConns) == 0 {
		delete(m.conns, userID)
	}
}

// IsSessionActive checks if the specified session has at least one stream connected to THIS local server node.
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

	sessionConns, exists := userConns[sessionUUID]
	return exists && len(sessionConns) > 0
}

// HasUser checks if the user has at least one active connection on this local server node.
func (m *Manager[T]) HasUser(userID uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userConns, exists := m.conns[userID]
	return exists && len(userConns) > 0
}

// HasUserExcept checks if the user has at least one active connection on this local server node
// in a session other than excludeSessionUUID.
func (m *Manager[T]) HasUserExcept(userID uuid.UUID, excludeSessionUUID uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userConns, exists := m.conns[userID]
	if !exists || len(userConns) == 0 {
		return false
	}
	for sessionUUID, sessionConns := range userConns {
		if sessionUUID != excludeSessionUUID && len(sessionConns) > 0 {
			return true
		}
	}
	return false
}

// BroadcastToUser sends an SSE event to ALL connected streams of a user on this node.
func (m *Manager[T]) BroadcastToUser(userID uuid.UUID, event T) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userConns, exists := m.conns[userID]
	if !exists {
		return
	}

	for _, sessionConns := range userConns {
		for _, conn := range sessionConns {
			select {
			case conn.Send <- event:
			default:
			}
		}
	}
}

// BroadcastToUserExcept sends an SSE event to all connected streams of a user on this node
// in sessions other than excludeSessionUUID. Streams of the excluded session are skipped too.
func (m *Manager[T]) BroadcastToUserExcept(userID uuid.UUID, excludeSessionUUID uuid.UUID, event T) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userConns, exists := m.conns[userID]
	if !exists {
		return
	}

	for sessionUUID, sessionConns := range userConns {
		if sessionUUID == excludeSessionUUID {
			continue
		}
		for _, conn := range sessionConns {
			select {
			case conn.Send <- event:
			default:
			}
		}
	}
}

// BroadcastToUserSession delivers an event to streams of the given session on this node.
//
// deliverToAllSessionStreams=false delivers to exactly one stream — the oldest local
// one, most likely to have finished E2EE key init and state hydration. Use it for
// events that trigger real work per delivery (history sync request / upload-ready),
// where fanning out to every tab would duplicate it. In a single-node deployment this
// is exactly one stream; in a cluster the session may own streams on several nodes, so
// up to one stream per node can receive the event.
//
// deliverToAllSessionStreams=true delivers to every stream of the target session, for
// session-scoped events that every tab must observe.
func (m *Manager[T]) BroadcastToUserSession(userID uuid.UUID, targetSessionUUID uuid.UUID, event T, deliverToAllSessionStreams bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userConns, exists := m.conns[userID]
	if !exists {
		return
	}

	sessionConns, exists := userConns[targetSessionUUID]
	if !exists || len(sessionConns) == 0 {
		return
	}

	if deliverToAllSessionStreams {
		for _, conn := range sessionConns {
			select {
			case conn.Send <- event:
			default:
			}
		}
		return
	}

	var oldest *Conn[T]
	for _, conn := range sessionConns {
		if oldest == nil || conn.seq < oldest.seq {
			oldest = conn
		}
	}

	select {
	case oldest.Send <- event:
	default:
	}
}
