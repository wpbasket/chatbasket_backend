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

// WebSocketUpgrade handles the HTTP → WebSocket upgrade for real-time chat events.
//
// Endpoint: GET /personal/chat/ws
// Auth:     Same AuthSessionMiddleware as all other /personal/chat/* routes.
func (h *chatHandler) WebSocketUpgrade(c *echo.Context) error {
	// ── 1. Extract auth context (set by AuthSessionMiddleware) ──────────────
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

	// ── 2. Check that the WSHub is available ────────────────────────────────
	if h.hub == nil {
		return c.JSON(http.StatusServiceUnavailable, &kit.ApiError{
			Code:    http.StatusServiceUnavailable,
			Message: "WebSocket service not available",
			Type:    "ws_unavailable",
		})
	}

	// ── 3. Accept the WebSocket upgrade ─────────────────────────────────────
	corsOrigin := h.Service.GlobalService.CORSOrigin

	wsConn, err := ws.Accept(c.Response(), c.Request(), &ws.AcceptOptions{
		OriginPatterns: []string{corsOrigin},
	})
	if err != nil {
		log.Printf("[WS] WebSocketUpgrade: Accept failed for user %s: %v", uuidUserId, err)
		// websocket.Accept already wrote the HTTP error response
		return nil
	}

	sessionUUIDVal, ok := c.Get("sessionUUID").(uuid.UUID)
	if !ok {
		return c.JSON(http.StatusUnauthorized, &kit.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Session is invalid",
			Type:    "unauthorized",
		})
	}

	// ──── 4. Create WSConn and register with hub ────────────────────────────
	wc := websocket.NewWSConn(wsConn, kit.UserId{
		StringUserId: userId,
		UuidUserId:   uuidUserId,
	}, sessionId, sessionUUIDVal, isPrimary)

	if !h.hub.Register(wc) {
		wsConn.Close(ws.StatusTryAgainLater, "too many connections")
		return nil
	}

	if isPrimary {
		go func() {
			payloads, _ := h.Service.ReplayPendingForPrimary(context.Background(), uuidUserId, sessionUUIDVal)
			for _, payload := range payloads {
				h.hub.BroadcastToUserSession(uuidUserId, sessionId, websocket.WSEvent{
					Type:    WSEventHistorySyncRequested,
					Payload: payload,
				})
			}
		}()
	}

	// ── 5. Create WS router for handling client→server messages ─────────────
	router := NewChatWSRouter(h.Service, h.hub)

	// ── 6. Run the connection pumps (blocks until disconnect) ───────────────
	// Use a background context — the Echo request context will be cancelled
	// when this handler returns, but we want the WS connection to outlive it.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start write pump in background goroutine
	go wc.WritePump(ctx)

	// Read pump blocks — when it returns, the connection is done
	wc.ReadPump(ctx, router.HandleRawMessage)

	// ── 7. Cleanup ──────────────────────────────────────────────────────────
	h.hub.Unregister(wc)
	wsConn.CloseNow()

	return nil
}
