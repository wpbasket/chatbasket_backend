package personal_setting

import (
	"chatbasket-api/internal/platform/kit"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type settingHandler struct {
	service *settingService
}

func newSettingHandler(service *settingService) *settingHandler {
	return &settingHandler{
		service: service,
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

