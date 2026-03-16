package commonhandler

import (
	"chatbasket-api/common/commonmodel"
	"chatbasket-api/common/commonservice"
	"chatbasket-api/model"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type SettingHandler struct {
	Service *commonservice.Service
}

func NewSettingHandler(service *commonservice.Service) *SettingHandler {
	return &SettingHandler{Service: service}
}

// RequestUpdateOTP handles OTP request for update operations
func (h *SettingHandler) RequestUpdateOTP(c *echo.Context) error {
	var payload commonmodel.RequestUpdateOTPPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid request payload",
			Type:    "bad_request",
		})
	}

	// Get userId from context (set by auth middleware) - SAFE TYPE ASSERTION
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}
	stringUserId, okString := c.Get("userId").(string)
	if !okString {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}
	userId := model.UserId{
		StringUserId: stringUserId,
		UuidUserId:   uuidUserId,
	}

	// Call service
	response, apiErr := h.Service.RequestUpdateOTP(c.Request().Context(), &payload, userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	return c.JSON(http.StatusOK, response)
}

// ConfirmPasswordUpdate handles password update confirmation with OTP
func (h *SettingHandler) ConfirmPasswordUpdate(c *echo.Context) error {
	var payload commonmodel.ConfirmPasswordUpdatePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid request payload",
			Type:    "bad_request",
		})
	}

	// Get userId from context (set by auth middleware) - SAFE TYPE ASSERTION
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}
	stringUserId, okString := c.Get("userId").(string)
	if !okString {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}
	userId := model.UserId{
		StringUserId: stringUserId,
		UuidUserId:   uuidUserId,
	}

	// Call service
	response, apiErr := h.Service.ConfirmPasswordUpdate(c.Request().Context(), &payload, userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	return c.JSON(http.StatusOK, response)
}

// RequestEmailUpdate handles email update request
func (h *SettingHandler) RequestEmailUpdate(c *echo.Context) error {
	var payload commonmodel.RequestEmailUpdatePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid request payload",
			Type:    "bad_request",
		})
	}

	// Get userId from context (set by auth middleware) - SAFE TYPE ASSERTION
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}
	stringUserId, okString := c.Get("userId").(string)
	if !okString {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}
	userId := model.UserId{
		StringUserId: stringUserId,
		UuidUserId:   uuidUserId,
	}

	// Call service
	response, apiErr := h.Service.RequestEmailUpdate(c.Request().Context(), &payload, userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	return c.JSON(http.StatusOK, response)
}

// ConfirmEmailUpdate handles email update confirmation with OTP
func (h *SettingHandler) ConfirmEmailUpdate(c *echo.Context) error {
	var payload commonmodel.ConfirmEmailUpdatePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid request payload",
			Type:    "bad_request",
		})
	}

	// Get userId from context (set by auth middleware) - SAFE TYPE ASSERTION
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}
	stringUserId, okString := c.Get("userId").(string)
	if !okString {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}
	userId := model.UserId{
		StringUserId: stringUserId,
		UuidUserId:   uuidUserId,
	}

	// Call service
	response, apiErr := h.Service.ConfirmEmailUpdate(c.Request().Context(), &payload, userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	return c.JSON(http.StatusOK, response)
}
