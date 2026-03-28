package personalservice

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────────────────────────
// WebSocket Event Types (server → client)
// ──────────────────────────────────────────────────────────────────────────────

const (
	WSEventNewMessage  = "new_message"
	WSEventDeliveryAck = "delivery_ack"
	WSEventReadReceipt = "read_receipt"
	WSEventUnsend      = "unsend"
	WSEventDeleteForMe = "delete_for_me"
	WSEventSyncAction  = "sync_action"
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
// It deserializes JSON frames, routes them through the WSRouter, and sends responses.
func (wc *WSConn) ReadPump(ctx context.Context, router *WSRouter) {
	wc.Conn.SetReadLimit(65536) // Increased for larger message payloads (up to 5000 chars)

	for {
		_, data, err := wc.Conn.Read(ctx)
		if err != nil {
			return
		}

		// ── 1. Deserialize client event ────────────────────────────────────
		var event WSClientEvent
		if err := json.Unmarshal(data, &event); err != nil {
			// Send error response (no ref available)
			errResp := WSResponseEvent{
				Type:  "error",
				Error: &WSError{Code: 400, Message: "Invalid JSON: " + err.Error()},
			}
			if bytes, _ := json.Marshal(errResp); bytes != nil {
				select {
				case wc.Send <- bytes:
				default:
					log.Printf("[WS] ReadPump: response buffer full, dropping error response")
				}
			}
			continue
		}

		// Route message and get response
		response := router.HandleMessage(ctx, wc, event)

		// Send response back to client
		if respBytes, err := json.Marshal(response); err == nil {
			select {
			case wc.Send <- respBytes:
				log.Printf("[WS] ReadPump: sent response type=%s ref=%s", response.Type, response.Ref)
			default:
				log.Printf("[WS] ReadPump: response buffer full, dropping response ref=%s", event.Ref)
			}
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
			log.Printf("[WS] Hub: user %s has too many connections (%d), rejecting session %s",
				wc.UserID, len(userConns), wc.SessionID)
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
	log.Printf("[WS] Hub.BroadcastToUser: event=%s → user=%s", event.Type, userID)

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[WS] Hub.BroadcastToUser: MARSHAL ERROR: %v", err)
		return
	}
	log.Printf("[WS] Hub.BroadcastToUser: payload=%s", string(data))

	h.mu.RLock()
	defer h.mu.RUnlock()

	userConns, exists := h.conns[userID]
	if !exists {
		log.Printf("[WS] Hub.BroadcastToUser: NO ACTIVE CONNECTIONS for user=%s, event DROPPED", userID)
		return
	}

	log.Printf("[WS] Hub.BroadcastToUser: user=%s has %d active connection(s)", userID, len(userConns))
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
// This is used for sender sync — the sender's current device already knows about the action.
func (h *WSHub) BroadcastToUserExcept(userID uuid.UUID, excludeSessionID string, event WSEvent) {
	log.Printf("[WS] Hub.BroadcastToUserExcept: event=%s → user=%s (exclude session=%s)",
		event.Type, userID, excludeSessionID)

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[WS] Hub.BroadcastToUserExcept: MARSHAL ERROR: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	userConns, exists := h.conns[userID]
	if !exists {
		log.Printf("[WS] Hub.BroadcastToUserExcept: NO ACTIVE CONNECTIONS for user=%s", userID)
		return
	}

	log.Printf("[WS] Hub.BroadcastToUserExcept: user=%s has %d connection(s), excluding session=%s",
		userID, len(userConns), excludeSessionID)
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

// ConnectionCount returns the total number of active WebSocket connections (for monitoring).
func (h *WSHub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	count := 0
	for _, userConns := range h.conns {
		count += len(userConns)
	}
	return count
}
