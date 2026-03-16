package personalhandler

import (
	"chatbasket-api/model"
	"chatbasket-api/personal/personalmodel"
	"chatbasket-api/personal/personalservice"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// SettingHandler handles personal-mode settings endpoints
// It uses personalservice.Service which wraps the shared services.GlobalService
// and is intended for personal mode specific behaviors.
type SettingHandler struct {
	Service *personalservice.Service
}

func NewSettingHandler(service *personalservice.Service) *SettingHandler {
	return &SettingHandler{Service: service}
}

func (h *SettingHandler) UpdateSessionCentral(c *echo.Context) error {
	// Get UserID from Context (set by auth middleware)
	userIdStr, ok := c.Get("userId").(string)
	if !ok {
		return c.JSON(http.StatusUnauthorized, model.ApiError{Code: http.StatusUnauthorized, Message: "Unauthorized", Type: "unauthorized"})
	}

	userID, err := uuid.Parse(userIdStr)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, model.ApiError{Code: http.StatusUnauthorized, Message: "Invalid User ID in token", Type: "unauthorized"})
	}

	// Get SessionToken from Context (set by auth middleware)
	sessionToken, ok := c.Get("sessionId").(string)
	if !ok || sessionToken == "" {
		return c.JSON(http.StatusUnauthorized, model.ApiError{Code: http.StatusUnauthorized, Message: "No session context", Type: "unauthorized"})
	}

	// Call Service with raw token
	result, apiErr := h.Service.SetCentralDevice(c.Request().Context(), userID, sessionToken)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	return c.JSON(http.StatusOK, result)
}

func (h *SettingHandler) UpdateSessionNotificationToken(c *echo.Context) error {
	var payload personalmodel.RegisterOrUpdateFcmOrApnTokenPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, model.ApiError{Code: http.StatusBadRequest, Message: "Invalid payload: " + err.Error(), Type: "bad_request"})
	}

	// Get UserID from Context (set by auth middleware)
	userIdStr, ok := c.Get("userId").(string)
	if !ok {
		return c.JSON(http.StatusUnauthorized, model.ApiError{Code: http.StatusUnauthorized, Message: "Unauthorized", Type: "unauthorized"})
	}

	userID, err := uuid.Parse(userIdStr)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, model.ApiError{Code: http.StatusUnauthorized, Message: "Invalid User ID in token", Type: "unauthorized"})
	}

	// Get SessionToken from Context (set by auth middleware)
	sessionToken, ok := c.Get("sessionId").(string)
	if !ok || sessionToken == "" {
		return c.JSON(http.StatusUnauthorized, model.ApiError{Code: http.StatusUnauthorized, Message: "No session context", Type: "unauthorized"})
	}

	// Call Service with raw token
	result, apiErr := h.Service.UpdateSessionNotificationToken(c.Request().Context(), userID, sessionToken, &payload)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	return c.JSON(http.StatusOK, result)
}
