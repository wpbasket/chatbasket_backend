package personal_chat

import (
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/websocket"
	"context"
	"log"
	"net/http"

	ws "github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// WebSocketUpgrade handles the HTTP â†’ WebSocket upgrade for real-time chat events.
//
// Endpoint: GET /personal/chat/ws
// Auth:     Same AuthSessionMiddleware as all other /personal/chat/* routes.
func (h *chatHandler) WebSocketUpgrade(c *echo.Context) error {
	// â”€â”€ 1. Extract auth context (set by AuthSessionMiddleware) â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	userId, okStr := c.Get("userId").(string)
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okStr || userId == "" || !okUUID {
		return c.JSON(http.StatusUnauthorized, &kit.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	sessionId, _ := c.Get("sessionId").(string)
	if sessionId == "" {
		return c.JSON(http.StatusUnauthorized, &kit.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Session id is missing",
			Type:    "unauthorized",
		})
	}

	isPrimary, _ := c.Get("isPrimary").(bool)

	// â”€â”€ 2. Check that the WSHub is available â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	if h.hub == nil {
		return c.JSON(http.StatusServiceUnavailable, &kit.ApiError{
			Code:    http.StatusServiceUnavailable,
			Message: "WebSocket service not available",
			Type:    "ws_unavailable",
		})
	}

	// â”€â”€ 3. Accept the WebSocket upgrade â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	wsConn, err := ws.Accept(c.Response(), c.Request(), &ws.AcceptOptions{
		// OriginPatterns: []string{"http://localhost:8081"},
		OriginPatterns: []string{"https://chatbasket.live"},
	})
	if err != nil {
		log.Printf("[WS] WebSocketUpgrade: Accept failed for user %s: %v", uuidUserId, err)
		// websocket.Accept already wrote the HTTP error response
		return nil
	}

	// ——— 4. Create WSConn and register with hub ——————————————————————————————————
	wc := websocket.NewWSConn(wsConn, kit.UserId{
		StringUserId: userId,
		UuidUserId:   uuidUserId,
	}, sessionId, isPrimary)

	if !h.hub.Register(wc) {
		wsConn.Close(ws.StatusTryAgainLater, "too many connections")
		return nil
	}

	// â”€â”€ 5. Create WS router for handling clientâ†’server messages â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	router := NewChatWSRouter(h.Service, h.hub)

	// â”€â”€ 6. Run the connection pumps (blocks until disconnect) â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	// Use a background context â€” the Echo request context will be cancelled
	// when this handler returns, but we want the WS connection to outlive it.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start write pump in background goroutine
	go wc.WritePump(ctx)

	// Read pump blocks â€” when it returns, the connection is done
	wc.ReadPump(ctx, router.HandleRawMessage)

	// â”€â”€ 7. Cleanup â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	h.hub.Unregister(wc)
	wsConn.CloseNow()

	return nil
}

