package websocket

import (
	"chatbasket-api/internal/platform/kit"
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// WSEvent is the envelope for server→client push events (broadcasts).
type WSEvent struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// MarshalJSON ensures that if Payload is a Protobuf message, it is serialized
// via protojson (camelCase field names, ISO 8601 timestamps) for consistency
// with the rest of the API surface.
func (e WSEvent) MarshalJSON() ([]byte, error) {
	var payloadBytes []byte
	var err error

	if pm, ok := e.Payload.(proto.Message); ok {
		m := protojson.MarshalOptions{
			UseProtoNames:   false,
			EmitUnpopulated: false,
		}
		payloadBytes, err = m.Marshal(pm)
		if err != nil {
			return nil, err
		}
	} else {
		payloadBytes, err = json.Marshal(e.Payload)
		if err != nil {
			return nil, err
		}
	}

	return json.Marshal(struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}{
		Type:    e.Type,
		Payload: payloadBytes,
	})
}


// WSClientEvent is the envelope for client→server messages.
type WSClientEvent struct {
	Type    string          `json:"type"`
	Ref     string          `json:"ref"`
	Payload json.RawMessage `json:"payload"`
}

// WSResponseEvent is the envelope for server→client responses.
type WSResponseEvent struct {
	Type    string   `json:"type"`
	Ref     string   `json:"ref"`
	Payload any      `json:"payload"`
	Error   *WSError `json:"error"`
}

// MarshalJSON ensures proto payloads in direct WS responses are serialized via
// protojson (camelCase keys, ISO 8601 timestamps) for consistency with WSEvent.
func (e WSResponseEvent) MarshalJSON() ([]byte, error) {
	var payloadBytes []byte
	var err error

	if pm, ok := e.Payload.(proto.Message); ok {
		m := protojson.MarshalOptions{
			UseProtoNames:   false,
			EmitUnpopulated: false,
		}
		payloadBytes, err = m.Marshal(pm)
		if err != nil {
			return nil, err
		}
	} else {
		payloadBytes, err = json.Marshal(e.Payload)
		if err != nil {
			return nil, err
		}
	}

	errBytes, err := json.Marshal(e.Error)
	if err != nil {
		return nil, err
	}

	return json.Marshal(struct {
		Type    string          `json:"type"`
		Ref     string          `json:"ref"`
		Payload json.RawMessage `json:"payload"`
		Error   json.RawMessage `json:"error"`
	}{
		Type:    e.Type,
		Ref:     e.Ref,
		Payload: payloadBytes,
		Error:   errBytes,
	})
}


// WSError represents an error in a WS response.
type WSError struct {
	Code    int         `json:"code"`
	Type    string      `json:"type"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// WSConn – a single authenticated WebSocket connection
// ──────────────────────────────────────────────────────────────────────────────

const (
	wsWriteWait       = 10 * time.Second
	wsPingInterval    = 30 * time.Second
	wsPongTimeout     = 10 * time.Second
	wsSendBufferSize  = 256
	wsMaxConnsPerUser = 5
)

// WSConn wraps a single WebSocket connection with metadata.
type WSConn struct {
	Conn        *websocket.Conn
	UserID      kit.UserId
	SessionID   string
	SessionUUID uuid.UUID
	IsPrimary   bool
	Send        chan []byte // buffered outbound queue
	closeOnce   sync.Once
}

// NewWSConn creates a new WSConn with a buffered send channel.
func NewWSConn(conn *websocket.Conn, userID kit.UserId, sessionID string, sessionUUID uuid.UUID, isPrimary bool) *WSConn {
	return &WSConn{
		Conn:        conn,
		UserID:      userID,
		SessionID:   sessionID,
		SessionUUID: sessionUUID,
		IsPrimary:   isPrimary,
		Send:        make(chan []byte, wsSendBufferSize),
	}
}

// WritePump drains the send channel and writes messages to the WS connection.
// It also sends periodic pings to keep the connection alive.
func (wc *WSConn) WritePump(ctx context.Context) {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-wc.Send:
			if !ok {
				wc.Conn.Close(websocket.StatusNormalClosure, "server closing")
				return
			}
			log.Printf("[WS] WritePump: SENDING %d bytes to session %s (user=%v): %s",
				len(msg), wc.SessionID, wc.UserID, string(msg))
			writeCtx, cancel := context.WithTimeout(ctx, wsWriteWait)
			err := wc.Conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				log.Printf("[WS] WritePump: WRITE ERROR for session %s: %v", wc.SessionID, err)
				wc.Conn.Close(websocket.StatusAbnormalClosure, "write failed")
				return
			}
			log.Printf("[WS] WritePump: SENT OK to session %s", wc.SessionID)

		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, wsPongTimeout)
			err := wc.Conn.Ping(pingCtx)
			cancel()
			if err != nil {
				log.Printf("[WS] WritePump: PING ERROR for session %s: %v", wc.SessionID, err)
				wc.Conn.Close(websocket.StatusGoingAway, "ping failed")
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

// ReadPump blocks reading from the WS connection and processes client→server messages.
// The handler function should deserialize, route, and respond to client events.
func (wc *WSConn) ReadPump(ctx context.Context, handler func(context.Context, *WSConn, []byte)) {
	wc.Conn.SetReadLimit(65536) // Increased for larger message payloads (up to 5000 chars)

	for {
		_, data, err := wc.Conn.Read(ctx)
		if err != nil {
			return
		}

		// Delegate to the handler provided by the module
		if handler != nil {
			handler(ctx, wc, data)
		}
	}
}

// Close gracefully shuts down the connection (idempotent).
func (wc *WSConn) Close() {
	wc.closeOnce.Do(func() {
		close(wc.Send)
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// WSHub – central registry of all active WebSocket connections
// ──────────────────────────────────────────────────────────────────────────────

// WSHub manages WebSocket connections grouped by user ID.
type WSHub struct {
	mu    sync.RWMutex
	conns map[uuid.UUID]map[string]*WSConn // UserID → SessionID → WSConn
}

// NewWSHub creates a new WebSocket hub.
func NewWSHub() *WSHub {
	return &WSHub{
		conns: make(map[uuid.UUID]map[string]*WSConn),
	}
}

// Register adds a connection to the hub. Returns false if the user has too many
// connections (limit: wsMaxConnsPerUser).
func (h *WSHub) Register(wc *WSConn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	userConns, exists := h.conns[wc.UserID.UuidUserId]
	if !exists {
		userConns = make(map[string]*WSConn)
		h.conns[wc.UserID.UuidUserId] = userConns
	}

	// Enforce per-user connection limit
	if len(userConns) >= wsMaxConnsPerUser {
		// If this session already has a conn, replace it
		if old, ok := userConns[wc.SessionID]; ok {
			old.Close()
		} else {
			return false
		}
	}

	userConns[wc.SessionID] = wc
	return true
}

// Unregister removes a connection from the hub.
func (h *WSHub) Unregister(wc *WSConn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	userConns, exists := h.conns[wc.UserID.UuidUserId]
	if !exists {
		return
	}

	if existing, ok := userConns[wc.SessionID]; ok && existing == wc {
		delete(userConns, wc.SessionID)
		wc.Close()

		if len(userConns) == 0 {
			delete(h.conns, wc.UserID.UuidUserId)
		}
	}
}

// IsSessionActive returns true if the specified session is currently connected.
func (h *WSHub) IsSessionActive(userID uuid.UUID, sessionUUID uuid.UUID) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	userConns, exists := h.conns[userID]
	if !exists {
		return false
	}

	for _, conn := range userConns {
		if conn.SessionUUID == sessionUUID {
			return true
		}
	}
	return false
}

// BroadcastToUser sends a WSEvent to ALL connected devices of a user.
func (h *WSHub) BroadcastToUser(userID uuid.UUID, event WSEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	userConns, exists := h.conns[userID]
	if !exists {
		return
	}

	for sid, wc := range userConns {
		select {
		case wc.Send <- data:
			log.Printf("[WS] Hub.BroadcastToUser: QUEUED event=%s to session=%s (user=%s)", event.Type, sid, userID)
		default:
			log.Printf("[WS] Hub.BroadcastToUser: BUFFER FULL for session=%s (user=%s), event DROPPED", sid, userID)
		}
	}
}

// BroadcastToUserExcept sends a WSEvent to all devices of a user EXCEPT the given session.
func (h *WSHub) BroadcastToUserExcept(userID uuid.UUID, excludeSessionID string, event WSEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	userConns, exists := h.conns[userID]
	if !exists {
		return
	}

	for sid, wc := range userConns {
		if sid == excludeSessionID {
			log.Printf("[WS] Hub.BroadcastToUserExcept: SKIPPING excluded session=%s", sid)
			continue
		}
		select {
		case wc.Send <- data:
			log.Printf("[WS] Hub.BroadcastToUserExcept: QUEUED event=%s to session=%s (user=%s)", event.Type, sid, userID)
		default:
			log.Printf("[WS] Hub.BroadcastToUserExcept: BUFFER FULL for session=%s (user=%s), event DROPPED", sid, userID)
		}
	}
}

// BroadcastToUserSession sends a WSEvent ONLY to the specified session of a user.
func (h *WSHub) BroadcastToUserSession(userID uuid.UUID, targetSessionID string, event WSEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	userConns, exists := h.conns[userID]
	if !exists {
		return
	}

	var wc *WSConn
	if conn, ok := userConns[targetSessionID]; ok {
		wc = conn
	} else {
		// Fallback: search by SessionUUID
		for _, conn := range userConns {
			if conn.SessionUUID.String() == targetSessionID {
				wc = conn
				break
			}
		}
	}

	if wc == nil {
		log.Printf("[WS] Hub.BroadcastToUserSession: TARGET SESSION NOT ACTIVE session=%s", targetSessionID)
		return
	}

	select {
	case wc.Send <- data:
		log.Printf("[WS] Hub.BroadcastToUserSession: QUEUED event=%s to session=%s (user=%s)", event.Type, targetSessionID, userID)
	default:
		log.Printf("[WS] Hub.BroadcastToUserSession: BUFFER FULL for session=%s (user=%s), event DROPPED", targetSessionID, userID)
	}
}

// CloseUserConnections gracefully closes all active WebSocket connections for a given user.
// This forces clients to reconnect and re-authenticate, picking up any session state
// changes (e.g. isPrimary) from the database.
func (h *WSHub) CloseUserConnections(userID uuid.UUID) {
	h.mu.RLock()
	userConns, exists := h.conns[userID]
	if !exists {
		h.mu.RUnlock()
		return
	}

	connsToClose := make([]*WSConn, 0, len(userConns))
	for _, wc := range userConns {
		connsToClose = append(connsToClose, wc)
	}
	h.mu.RUnlock()

	for _, wc := range connsToClose {
		log.Printf("[WS] Hub.CloseUserConnections: closing session=%s (user=%s)", wc.SessionID, userID)
		wc.Close()
	}
}

// CloseSessionConnection gracefully closes one active WebSocket connection for a user/session.
func (h *WSHub) CloseSessionConnection(userID uuid.UUID, sessionID string) {
	h.mu.RLock()
	userConns, exists := h.conns[userID]
	if !exists {
		h.mu.RUnlock()
		return
	}

	wc := userConns[sessionID]
	h.mu.RUnlock()

	if wc == nil {
		return
	}

	log.Printf("[WS] Hub.CloseSessionConnection: closing session=%s (user=%s)", sessionID, userID)
	wc.Close()
}

// ConnectionCount returns the total number of active WebSocket connections.
func (h *WSHub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	count := 0
	for _, userConns := range h.conns {
		count += len(userConns)
	}
	return count
}
