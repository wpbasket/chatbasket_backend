package commonhandler

import (
	"chatbasket-api/common/commonmodel"
	"chatbasket-api/common/commonservice"
	"chatbasket-api/model"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type AuthHandler struct {
	Service *commonservice.Service
}

func NewAuthHandler(service *commonservice.Service) *AuthHandler {
	return &AuthHandler{Service: service}
}

// Logout handles logout from single or all sessions
func (h *AuthHandler) Logout(c echo.Context) error {
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
	return c.JSON(http.StatusOK, res)
}
