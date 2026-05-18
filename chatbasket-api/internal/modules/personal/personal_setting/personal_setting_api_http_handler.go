package personal_setting

import (
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/websocket"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type settingHandler struct {
	service *settingService
	hub     *websocket.WSHub
}

func newSettingHandler(service *settingService, hub *websocket.WSHub) *settingHandler {
	return &settingHandler{
		service: service,
		hub:     hub,
	}
}

// updateSessionCentral promotes the current session to be the primary device.
func (h *settingHandler) updateSessionCentral(c *echo.Context) error {
	// 1. Get UserID and SessionID from Context (set by AuthSessionMiddleware)
	userID, ok := c.Get("uuidUserId").(uuid.UUID)
	if !ok {
		return ErrInvalidUserContext
	}

	sessionToken, ok := c.Get("sessionId").(string)
	if !ok || sessionToken == "" {
		return ErrInvalidSessionContext
	}

	// 2. Call Service
	okResponse, err := h.service.setCentralDevice(c.Request().Context(), userID, sessionToken)
	if err != nil {
		return err // Echo GlobalErrorHandler takes over
	}

	// 3. Invalidate all active WebSocket connections for this user so they reconnect
	//    and pick up the updated session.IsCentral state from the auth middleware.
	if h.hub != nil {
		h.hub.CloseUserConnections(userID)
	}

	return c.JSON(http.StatusOK, okResponse)
}

// updateSessionNotificationToken updates the push notification token for the current session.
func (h *settingHandler) updateSessionNotificationToken(c *echo.Context) error {
	var payload registerOrUpdateFcmOrApnTokenPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "Invalid payload: "+err.Error())
	}

	// 1. Get UserID and SessionID from Context (set by AuthSessionMiddleware)
	userID, ok := c.Get("uuidUserId").(uuid.UUID)
	if !ok {
		return ErrInvalidUserContext
	}

	sessionToken, ok := c.Get("sessionId").(string)
	if !ok || sessionToken == "" {
		return ErrInvalidSessionContext
	}

	// 2. Call Service
	okResponse, err := h.service.updateSessionNotificationToken(c.Request().Context(), userID, sessionToken, &payload)
	if err != nil {
		return err // Echo GlobalErrorHandler takes over
	}

	return c.JSON(http.StatusOK, okResponse)
}
