package commonhandler

import (
	"chatbasket-api-legacy/common/commonmodel"
	"chatbasket-api-legacy/common/commonservice"
	"chatbasket-api-legacy/model"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"strings"
)

type AuthHandler struct {
	Service *commonservice.Service
}

func NewAuthHandler(service *commonservice.Service) *AuthHandler {
	return &AuthHandler{Service: service}
}

// Logout handles logout from single or all sessions
func (h *AuthHandler) Logout(c *echo.Context) error {
	var payload commonmodel.LogoutPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid logout payload")
	}

	// Extract user ID from context
	userId, ok := c.Get("userId").(string)
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !ok || !okUUID {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}

	// Extract session ID from context
	sessionId, ok := c.Get("sessionId").(string)
	if !ok || sessionId == "" {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid session context",
			Type:    "internal_server_error",
		})
	}

	res, apiErr := h.Service.Logout(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, sessionId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	// Clear cookies for web platform
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

	return c.JSON(http.StatusOK, res)
}

// GetUser returns the current user and session details
func (h *AuthHandler) GetUser(c *echo.Context) error {
	// Extract user ID from context
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Invalid user context",
			Type:    "unauthorized",
		})
	}

	// Extract session ID from context
	sessionId, ok := c.Get("sessionId").(string)
	if !ok || sessionId == "" {
		return c.JSON(http.StatusUnauthorized, model.ApiError{
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

