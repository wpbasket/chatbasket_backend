package authapi

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"chatbasket-apinext/internal/modules/core/auth/authmodels"
	"chatbasket-apinext/internal/platform/kit"
)

// Logout handles logout from single or all sessions
func (h *AuthHandler) Logout(c *echo.Context) error {
	var payload authmodels.LogoutPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, kit.ApiError{Code: http.StatusBadRequest, Message: "Invalid logout payload", Type: "bad_request"})
	}

	// Extract user ID from context
	uuidUserId, ok := c.Get("uuidUserId").(uuid.UUID)
	if !ok {
		return c.JSON(http.StatusInternalServerError, kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}

	// Extract session ID from context
	sessionId, okSession := c.Get("sessionId").(string)
	if !okSession || sessionId == "" {
		return c.JSON(http.StatusInternalServerError, kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid session context",
			Type:    "internal_server_error",
		})
	}

	res, apiErr := h.Service.Logout(c.Request().Context(), &payload, uuidUserId, sessionId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	// For web, clear cookies
	if c.Get("platform").(string) == "web" {
		c.SetCookie(&http.Cookie{Name: "sessionId", Value: "", Path: "/", MaxAge: -1})
		c.SetCookie(&http.Cookie{Name: "userId", Value: "", Path: "/", MaxAge: -1})
	}

	return c.JSON(http.StatusOK, res)
}

// GetUser returns the current user and session details
func (h *AuthHandler) GetUser(c *echo.Context) error {
	// Extract user ID from context
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, kit.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Invalid user context",
			Type:    "unauthorized",
		})
	}

	// Extract session ID from context
	sessionId, ok := c.Get("sessionId").(string)
	if !ok || sessionId == "" {
		return c.JSON(http.StatusUnauthorized, kit.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "No session context",
			Type:    "unauthorized",
		})
	}

	res, apiErr := h.Service.GetUserWithSession(c.Request().Context(), uuidUserId, sessionId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

// RequestUpdateOTP handles OTP request for update operations
func (h *AuthHandler) RequestUpdateOTP(c *echo.Context) error {
	var payload authmodels.RequestUpdateOTPPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, kit.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid request payload",
			Type:    "bad_request",
		})
	}

	// Get userId from context (set by auth middleware) - SAFE TYPE ASSERTION
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusInternalServerError, kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}

	// Call service
	res, apiErr := h.Service.RequestUpdateOTP(c.Request().Context(), &payload, uuidUserId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

// ConfirmPasswordUpdate handles password update confirmation with OTP
func (h *AuthHandler) ConfirmPasswordUpdate(c *echo.Context) error {
	var payload authmodels.ConfirmPasswordUpdatePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, kit.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid request payload",
			Type:    "bad_request",
		})
	}

	// Get userId from context (set by auth middleware) - SAFE TYPE ASSERTION
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusInternalServerError, kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}

	// Call service
	res, apiErr := h.Service.ConfirmPasswordUpdate(c.Request().Context(), &payload, uuidUserId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

// RequestEmailUpdate handles email update request
func (h *AuthHandler) RequestEmailUpdate(c *echo.Context) error {
	var payload authmodels.RequestEmailUpdatePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, kit.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid request payload",
			Type:    "bad_request",
		})
	}

	// Get userId from context (set by auth middleware) - SAFE TYPE ASSERTION
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusInternalServerError, kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}

	// Call service
	res, apiErr := h.Service.RequestEmailUpdate(c.Request().Context(), &payload, uuidUserId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

// ConfirmEmailUpdate handles email update confirmation with OTP
func (h *AuthHandler) ConfirmEmailUpdate(c *echo.Context) error {
	var payload authmodels.ConfirmEmailUpdatePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, kit.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid request payload",
			Type:    "bad_request",
		})
	}

	// Get userId from context (set by auth middleware) - SAFE TYPE ASSERTION
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusInternalServerError, kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}

	// Call service
	res, apiErr := h.Service.ConfirmEmailUpdate(c.Request().Context(), &payload, uuidUserId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}