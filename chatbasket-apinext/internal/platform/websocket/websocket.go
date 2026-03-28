package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"log"
)

// WSEvent is the envelope for server→client push events (broadcasts).
type WSEvent struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
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

// WSError represents an error in a WS response.
type WSError struct {
	Code    int    `json:"code"`
	Type    string `json:"type,omitempty"`
	Message string `json:"message"`
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
	Conn      *websocket.Conn
	UserID    uuid.UUID
	SessionID string
	IsPrimary bool
	Send      chan []byte // buffered outbound queue
	closeOnce sync.Once
}

// NewWSConn creates a new WSConn with a buffered send channel.
func NewWSConn(conn *websocket.Conn, userID uuid.UUID, sessionID string, isPrimary bool) *WSConn {
	return &WSConn{
		Conn:      conn,
		UserID:    userID,
		SessionID: sessionID,
		IsPrimary: isPrimary,
		Send:      make(chan []byte, wsSendBufferSize),
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
			log.Printf("[WS] WritePump: SENDING %d bytes to session %s (user=%s): %s",
				len(msg), wc.SessionID, wc.UserID, string(msg))
			writeCtx, cancel := context.WithTimeout(ctx, wsWriteWait)
			err := wc.Conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				log.Printf("[WS] WritePump: WRITE ERROR for session %s: %v", wc.SessionID, err)
				return
			}
			log.Printf("[WS] WritePump: SENT OK to session %s", wc.SessionID)

		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, wsPongTimeout)
			err := wc.Conn.Ping(pingCtx)
			cancel()
			if err != nil {
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

	userConns, exists := h.conns[wc.UserID]
	if !exists {
		userConns = make(map[string]*WSConn)
		h.conns[wc.UserID] = userConns
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

	userConns, exists := h.conns[wc.UserID]
	if !exists {
		return
	}

	if existing, ok := userConns[wc.SessionID]; ok && existing == wc {
		delete(userConns, wc.SessionID)
		wc.Close()

		if len(userConns) == 0 {
			delete(h.conns, wc.UserID)
		}
	}
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
