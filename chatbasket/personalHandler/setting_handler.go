package personalHandler

import (
	"chatbasket/model"
	"chatbasket/personalServices"
	"net/http"

	"github.com/labstack/echo/v4"
)

// SettingHandler handles personal-mode settings endpoints
// It uses personalServices.Service which wraps the shared services.GlobalService
// and is intended for personal mode specific behaviors.
type SettingHandler struct {
	Service *personalServices.Service
}

func NewSettingHandler(service *personalServices.Service) *SettingHandler {
	return &SettingHandler{Service: service}
}

func (h *SettingHandler) UpdateEmail(c echo.Context) error {
	var payload model.UpdateEmailPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, "Invalid email payload")
	}
	userId := c.Get("userId").(string)
	res, apiErr := h.Service.UpdateEmail(c.Request().Context(), &payload, userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *SettingHandler) UpdateEmailVerification(c echo.Context) error {
	var payload model.UpdateEmailVerification
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, "Invalid email payload")
	}
	userId := c.Get("userId").(string)
	res, apiErr := h.Service.UpdateEmailVerification(c.Request().Context(), &payload, userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *SettingHandler) UpdatePassword(c echo.Context) error {
	var payload model.UpdatePassword
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, "Invalid password payload")
	}
	userId := c.Get("userId").(string)
	res, apiErr := h.Service.UpdatePassword(c.Request().Context(), &payload, userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *SettingHandler) SendOtp(c echo.Context) error {
	var payload model.SendOtpPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, "Invalid OTP payload")
	}
	userId := c.Get("userId").(string)
	res, apiErr := h.Service.SendOtp(c.Request().Context(), &payload, userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *SettingHandler) VerifyOtp(c echo.Context) error {
	var payload model.OtpVerificationPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, "Invalid OTP payload")
	}
	userId := c.Get("userId").(string)
	res, apiErr := h.Service.VerifyOtp(c.Request().Context(), &payload, userId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, res)
}
