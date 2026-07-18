package core_auth

import (
	"chatbasket-api/internal/platform/kit"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// Logout handles logout from single or all sessions
func (h *authHandler) Logout(c *echo.Context) error {
	var payload LogoutPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(400, "bad_request", "Invalid logout payload")
	}

	// Extract user ID from context
	uuidUserId, ok := c.Get("uuidUserId").(uuid.UUID)
	if !ok {
		return ErrInvalidUserContext
	}

	// Extract session ID from context
	sessionId, okSession := c.Get("sessionId").(string)
	if !okSession || sessionId == "" {
		return ErrInvalidSessionContext
	}

	// Primary device always logs out ALL sessions (force), regardless of the flag.
	if isPrimary, _ := c.Get("isPrimary").(bool); isPrimary {
		payload.AllSessions = true
	}

	res, err := h.Service.Logout(c.Request().Context(), &payload, uuidUserId, sessionId)
	if err != nil {
		return err
	}

	if h.hub != nil {
		if payload.AllSessions {
			h.hub.CloseUserConnections(uuidUserId)
		} else {
			h.hub.CloseSessionConnection(uuidUserId, sessionId)
		}
	}

	// For web, clear cookies
	if c.Get("platform").(string) == "web" {
		// Determine cookie security based on host (targeting local frontend at 8081)
		origin := c.Request().Header.Get("Origin")
		isLocal := strings.Contains(origin, "localhost:8081")
		cookieDomain := "chatbasket.live"
		cookieSecure := true
		if isLocal {
			cookieDomain = ""
			cookieSecure = false
		}

		c.SetCookie(&http.Cookie{
			Name:     "sessionId",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   cookieSecure,
			Domain:   cookieDomain,
			MaxAge:   -1,
		})
		c.SetCookie(&http.Cookie{
			Name:     "userId",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   cookieSecure,
			Domain:   cookieDomain,
			MaxAge:   -1,
		})
	}

	return c.JSON(http.StatusOK, res)
}

// GetUser returns the current user and session details
func (h *authHandler) GetUser(c *echo.Context) error {
	// Extract user ID from context
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return kit.NewError(401, "unauthorized", "Invalid user context")
	}

	// Extract session ID from context
	sessionId, ok := c.Get("sessionId").(string)
	if !ok || sessionId == "" {
		return kit.NewError(401, "unauthorized", "No session context")
	}

	res, err := h.Service.GetUserWithSession(c.Request().Context(), uuidUserId, sessionId)
	if err != nil {
		return err
	}

	if platform, ok := c.Get("platform").(string); ok && platform == "web" {
		res.SessionID = ""
	}

	return c.JSON(http.StatusOK, res)
}

// RequestUpdateOTP handles OTP request for update operations
func (h *authHandler) RequestUpdateOTP(c *echo.Context) error {
	var payload RequestUpdateOTPPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(400, "bad_request", "Invalid request payload")
	}

	// Get userId from context (set by auth middleware) - SAFE TYPE ASSERTION
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return ErrInvalidUserContext
	}

	// Call service
	res, err := h.Service.RequestUpdateOTP(c.Request().Context(), &payload, uuidUserId)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

// ConfirmPasswordUpdate handles password update confirmation with OTP
func (h *authHandler) ConfirmPasswordUpdate(c *echo.Context) error {
	var payload ConfirmPasswordUpdatePayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(400, "bad_request", "Invalid request payload")
	}

	// Get userId from context (set by auth middleware) - SAFE TYPE ASSERTION
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return ErrInvalidUserContext
	}

	// Call service
	res, err := h.Service.ConfirmPasswordUpdate(c.Request().Context(), &payload, uuidUserId)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

// RequestEmailUpdate handles email update request
func (h *authHandler) RequestEmailUpdate(c *echo.Context) error {
	var payload RequestEmailUpdatePayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(400, "bad_request", "Invalid request payload")
	}

	// Get userId from context (set by auth middleware) - SAFE TYPE ASSERTION
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return ErrInvalidUserContext
	}

	// Call service
	res, err := h.Service.RequestEmailUpdate(c.Request().Context(), &payload, uuidUserId)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

// ConfirmEmailUpdate handles email update confirmation with OTP
func (h *authHandler) ConfirmEmailUpdate(c *echo.Context) error {
	var payload ConfirmEmailUpdatePayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(400, "bad_request", "Invalid request payload")
	}

	// Get userId from context (set by auth middleware) - SAFE TYPE ASSERTION
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return ErrInvalidUserContext
	}

	// Call service
	res, err := h.Service.ConfirmEmailUpdate(c.Request().Context(), &payload, uuidUserId)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}
