package core_auth

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type QRHub struct {
	mu    sync.RWMutex
	conns map[uuid.UUID]*websocket.Conn
}

func NewQRHub() *QRHub {
	return &QRHub{
		conns: make(map[uuid.UUID]*websocket.Conn),
	}
}

func (h *QRHub) Register(token uuid.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Close existing if any to prevent leaks (should only be 1 browser)
	if existing, ok := h.conns[token]; ok {
		existing.Close(websocket.StatusPolicyViolation, "New connection established")
	}

	h.conns[token] = conn
}

func (h *QRHub) Unregister(token uuid.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, ok := h.conns[token]; ok && existing == conn {
		delete(h.conns, token)
	}
}

func (h *QRHub) Close(token uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, ok := h.conns[token]; ok {
		delete(h.conns, token)
		_ = conn.Close(websocket.StatusNormalClosure, "qr login completed")
	}
}

func (h *QRHub) Broadcast(token uuid.UUID, eventType string, payload interface{}) {
	h.mu.RLock()
	conn, ok := h.conns[token]
	h.mu.RUnlock()

	if !ok {
		return
	}

	msg, err := json.Marshal(map[string]interface{}{
		"type":    eventType,
		"payload": payload,
	})
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
		h.Unregister(token, conn)
		_ = conn.Close(websocket.StatusInternalError, "write failed")
	}
}

// StartPostgresListener runs a blocking loop to listen for NOTIFY events.
func StartPostgresListener(ctx context.Context, pool *pgxpool.Pool, hub *QRHub) {
	// Acquire a dedicated connection for LISTEN
	conn, err := pool.Acquire(ctx)
	if err != nil {
		log.Printf("[QRHub] Failed to acquire connection for LISTEN: %v", err)
		return
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, "LISTEN qr_login_events")
	if err != nil {
		log.Printf("[QRHub] Failed to execute LISTEN: %v", err)
		return
	}

	log.Println("[QRHub] Started Postgres Listener on 'qr_login_events'")

	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			log.Printf("[QRHub] WaitForNotification error (or context canceled): %v", err)
			return
		}

		// Payload expected format: "<uuid>_<event_type>"
		// Example: "f47ac10b-58cc-4372-a567-0e02b2c3d479_signal"
		payload := notification.Payload
		if len(payload) <= 37 || payload[36] != '_' {
			log.Printf("[QRHub] Ignoring malformed payload: %q", payload)
			continue
		}

		tokenStr := payload[:36]
		eventType := payload[37:]

		token, err := uuid.Parse(tokenStr)
		if err != nil {
			log.Printf("[QRHub] Ignoring payload with invalid token: %q", payload)
			continue
		}

		hub.Broadcast(token, eventType, map[string]string{"status": "updated"})
	}
}
