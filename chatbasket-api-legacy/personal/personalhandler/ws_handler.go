package personalhandler

import (
	"chatbasket-api-legacy/model"
	"chatbasket-api-legacy/personal/personalservice"
	"context"
	"log"
	"net/http"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// WebSocketUpgrade handles the HTTP â†’ WebSocket upgrade for real-time chat events.
//
// Endpoint: GET /personal/chat/ws
// Auth:     Same AuthSessionMiddleware as all other /personal/chat/* routes.
//
// The client connects and receives server-pushed events (new_message, read_receipt, etc.).
// The client does NOT send data over this connection â€” all mutations go through REST.
func (h *ChatHandler) WebSocketUpgrade(c *echo.Context) error {
	// â”€â”€ 1. Extract auth context (set by AuthSessionMiddleware) â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
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

	// â”€â”€ 2. Check that the WSHub is available â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	hub := h.service.WSHub
	if hub == nil {
		return c.JSON(http.StatusServiceUnavailable, &model.ApiError{
			Code:    http.StatusServiceUnavailable,
			Message: "WebSocket service not available",
			Type:    "ws_unavailable",
		})
	}

	// â”€â”€ 3. Accept the WebSocket upgrade â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	// AcceptOptions:
	//   - InsecureSkipVerify: true for now â€” the auth middleware already validates sessions.
	//     In production, set OriginPatterns to your frontend domain(s).
	wsConn, err := websocket.Accept(c.Response(), c.Request(), &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("[WS] WebSocketUpgrade: Accept failed for user %s: %v", uuidUserId, err)
		// websocket.Accept already wrote the HTTP error response
		return nil
	}

	// â”€â”€ 4. Create WSConn and register with hub â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	wc := personalservice.NewWSConn(wsConn, uuidUserId, sessionId, isPrimary)

	if !hub.Register(wc) {
		wsConn.Close(websocket.StatusTryAgainLater, "too many connections")
		return nil
	}

	// â”€â”€ 5. Create WS router for handling clientâ†’server messages â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	router := personalservice.NewWSRouter(h.service, hub)

	// â”€â”€ 6. Run the connection pumps (blocks until disconnect) â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	// Use a background context â€” the Echo request context will be cancelled
	// when this handler returns, but we want the WS connection to outlive it.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start write pump in background goroutine
	go wc.WritePump(ctx)

	// Read pump blocks â€” when it returns, the connection is done
	wc.ReadPump(ctx, router)

	// â”€â”€ 6. Cleanup â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	hub.Unregister(wc)
	wsConn.CloseNow()

	return nil
}

