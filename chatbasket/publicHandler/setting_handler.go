package publicHandler

import (
	"chatbasket/model"
	"chatbasket/publicServices"
	"net/http"

	"github.com/labstack/echo/v4"
)

type SettingHandler struct {
	Service *publicServices.Service
}

func NewSettingHandler(service *publicServices.Service) *SettingHandler {
	return &SettingHandler{Service: service}
}

func (h *SettingHandler) UpdateEmail(c echo.Context) error{
	var payload model.UpdateEmailPayload
	if err := c.Bind(&payload); err != nil {	
		return c.JSON(http.StatusBadRequest, "Invalid email payload")
	}
	userId, ok := c.Get("userId").(string)
	if !ok {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}
	
	user, err := h.Service.UpdateEmail(c.Request().Context(), &payload, userId)	
	if err != nil {
		return c.JSON(err.Code, err)
	}
	
	return c.JSON(http.StatusOK, user)

}


func (h *SettingHandler) UpdateEmailVerification(c echo.Context) error{
	var payload model.UpdateEmailVerification
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, "Invalid email payload")
	}
	userId, ok := c.Get("userId").(string)
	if !ok {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}
	
	user, err := h.Service.UpdateEmailVerification(c.Request().Context(), &payload, userId)
	if err != nil {
		return c.JSON(err.Code, err)
	}
	
	return c.JSON(http.StatusOK, user)
}


func (h *SettingHandler) UpdatePassword(c echo.Context) error{
	var payload model.UpdatePassword
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, "Invalid password payload")
	}
	userId, ok := c.Get("userId").(string)
	if !ok {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}
	
	user, err := h.Service.UpdatePassword(c.Request().Context(), &payload, userId)
	if err != nil {
		return c.JSON(err.Code, err)
	}
	
	return c.JSON(http.StatusOK, user)	
}

func (h *SettingHandler) SendOtp(c echo.Context) error{
	var payload model.SendOtpPayload
	if err:= c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, "Invalid OTP payload")
	}
	userId, ok := c.Get("userId").(string)
	if !ok {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}

	user, err := h.Service.SendOtp(c.Request().Context(), &payload, userId)
	if err != nil {
		return c.JSON(err.Code, err)
	}

	return c.JSON(http.StatusOK, user)
}

func (h *SettingHandler) VerifyOtp(c echo.Context) error{
	var payload model.OtpVerificationPayload
	if err:= c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, "Invalid OTP payload")
	}
	userId, ok := c.Get("userId").(string)
	if !ok {
		return c.JSON(http.StatusInternalServerError, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Invalid user context",
			Type:    "internal_server_error",
		})
	}

	user, err := h.Service.VerifyOtp(c.Request().Context(), &payload, userId)
	if err != nil {
		return c.JSON(err.Code, err)
	}

	return c.JSON(http.StatusOK, user)
}