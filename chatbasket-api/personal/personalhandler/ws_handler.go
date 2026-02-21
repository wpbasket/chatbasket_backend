package personalhandler

import (
	"chatbasket-api/model"
	"chatbasket-api/personal/personalservice"
	"context"
	"log"
	"net/http"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// WebSocketUpgrade handles the HTTP → WebSocket upgrade for real-time chat events.
//
// Endpoint: GET /personal/chat/ws
// Auth:     Same AuthSessionMiddleware as all other /personal/chat/* routes.
//
// The client connects and receives server-pushed events (new_message, read_receipt, etc.).
// The client does NOT send data over this connection — all mutations go through REST.
func (h *ChatHandler) WebSocketUpgrade(c echo.Context) error {
	// ── 1. Extract auth context (set by AuthSessionMiddleware) ──────────────
	uuidUserId, ok := c.Get("uuidUserId").(uuid.UUID)
	if !ok {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	sessionId, _ := c.Get("sessionId").(string)
	if sessionId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Session id is missing",
			Type:    "unauthorized",
		})
	}

	isPrimary, _ := c.Get("isPrimary").(bool)

	// ── 2. Check that the WSHub is available ────────────────────────────────
	hub := h.service.WSHub
	if hub == nil {
		return c.JSON(http.StatusServiceUnavailable, &model.ApiError{
			Code:    http.StatusServiceUnavailable,
			Message: "WebSocket service not available",
			Type:    "ws_unavailable",
		})
	}

	// ── 3. Accept the WebSocket upgrade ─────────────────────────────────────
	// AcceptOptions:
	//   - InsecureSkipVerify: true for now — the auth middleware already validates sessions.
	//     In production, set OriginPatterns to your frontend domain(s).
	wsConn, err := websocket.Accept(c.Response().Writer, c.Request(), &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("[WS] WebSocketUpgrade: Accept failed for user %s: %v", uuidUserId, err)
		// websocket.Accept already wrote the HTTP error response
		return nil
	}

	// ── 4. Create WSConn and register with hub ──────────────────────────────
	wc := personalservice.NewWSConn(wsConn, uuidUserId, sessionId, isPrimary)

	if !hub.Register(wc) {
		wsConn.Close(websocket.StatusTryAgainLater, "too many connections")
		return nil
	}

	// ── 5. Run the connection pumps (blocks until disconnect) ────────────────
	// Use a background context — the Echo request context will be cancelled
	// when this handler returns, but we want the WS connection to outlive it.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start write pump in background goroutine
	go wc.WritePump(ctx)

	// Read pump blocks — when it returns, the connection is done
	wc.ReadPump(ctx)

	// ── 6. Cleanup ──────────────────────────────────────────────────────────
	hub.Unregister(wc)
	wsConn.CloseNow()

	return nil
}
