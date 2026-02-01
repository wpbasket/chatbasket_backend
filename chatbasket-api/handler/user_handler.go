package handler

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"chatbasket-api/model"
	"chatbasket-api/services"
)

type UserHandler struct {
	AuthService *services.AuthService
}

func NewUserHandler(service *services.AuthService) *UserHandler {
	return &UserHandler{AuthService: service}
}

func (h *UserHandler) Signup(c echo.Context) error {
	var payload model.SignupPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid signup payload: " + err.Error(),
			Type:    "missing_value",
		})
	}

	// Validate required fields
	if payload.Email == "" || payload.Password == "" {
		return c.JSON(http.StatusBadRequest, model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Missing required fields",
			Type:    "missing_value",
		})
	}

	// Create user via service
	user, err := h.AuthService.Signup(c.Request().Context(), &payload)
	if err != nil {
		return c.JSON(err.Code, err)
		// return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Return sanitized user info
	return c.JSON(http.StatusCreated, user)
}

func (h *UserHandler) AcountVerification(c echo.Context) error {
	var payload model.AuthVerificationPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, model.ApiError{Code: http.StatusBadRequest, Message: "Invalid OTP payload: " + err.Error(), Type: "bad_request"})
	}

	if payload.Email == "" || payload.Secret == "" || payload.Platform == "" {
		return c.JSON(http.StatusBadRequest, model.ApiError{Code: http.StatusBadRequest, Message: "Missing required fields", Type: "missing_value"})
	}

	user, err := h.AuthService.AccountVerification(c.Request().Context(), &payload)
	if err != nil {
		return c.JSON(err.Code, err)
	}

	// Handle web platform - set httpOnly cookies
	if payload.Platform == "web" {

		expiry, err := time.Parse(time.RFC3339, user.SessionExpiry)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, model.ApiError{Code: http.StatusInternalServerError, Message: "invalid session expiry format", Type: "internal_server_error"})
		}

		// Set cookies with actual values (before they get emptied in response)
		sessionCookie := &http.Cookie{
			Name:     "sessionId",
			Value:    user.SessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			// Domain:   "localhost:8081",
			Domain:   "chatbasket.live",
			SameSite: http.SameSiteNoneMode,
			Expires:  expiry,
		}

		userCookie := &http.Cookie{
			Name:     "userId",
			Value:    user.UserId,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			// Domain:   "localhost:8081",
			Domain:  "chatbasket.live",
			Expires: expiry,
		}

		c.SetCookie(sessionCookie)
		c.SetCookie(userCookie)

		// Return SessionResponse with empty sensitive fields for web
		webResponse := &model.SessionResponse{
			UserId:        "",
			Name:          user.Name,
			Email:         user.Email,
			SessionID:     "",
			SessionExpiry: user.SessionExpiry,
		}
		return c.JSON(http.StatusOK, webResponse)
	}

	return c.JSON(http.StatusOK, user)
}
func (h *UserHandler) Login(c echo.Context) error {
	var payload model.LoginPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, model.ApiError{Code: http.StatusBadRequest, Message: "Invalid login payload: " + err.Error(), Type: "bad_request"})
	}

	// Validate required fields
	if payload.Email == "" || payload.Password == "" {
		return c.JSON(http.StatusBadRequest, model.ApiError{Code: http.StatusBadRequest, Message: "Missing required fields", Type: "missing_value"})
	}

	// Login via service
	user, err := h.AuthService.Login(c.Request().Context(), &payload)
	if err != nil {
		return c.JSON(err.Code, err)

	}

	// Return sanitized user info
	return c.JSON(http.StatusOK, user)
}

func (h *UserHandler) LoginVerification(c echo.Context) error {
	var payload model.AuthVerificationPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, model.ApiError{Code: http.StatusBadRequest, Message: "Invalid OTP payload: " + err.Error(), Type: "bad_request"})
	}

	if payload.Email == "" || payload.Secret == "" || payload.Platform == "" {
		return c.JSON(http.StatusBadRequest, model.ApiError{Code: http.StatusBadRequest, Message: "Missing required fields", Type: "missing_value"})
	}

	user, err := h.AuthService.LoginVerification(c.Request().Context(), &payload)
	if err != nil {
		return c.JSON(err.Code, err)
	}

	// Handle web platform - set httpOnly cookies
	if payload.Platform == "web" {

		expiry, err := time.Parse(time.RFC3339, user.SessionExpiry)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, model.ApiError{Code: http.StatusInternalServerError, Message: "invalid session expiry format", Type: "internal_server_error"})
		}

		// Set cookies with actual values (before they get emptied in response)
		sessionCookie := &http.Cookie{
			Name:     "sessionId",
			Value:    user.SessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			// Domain:   "localhost:8081",
			Domain:  "chatbasket.live",
			Expires: expiry,
		}

		userCookie := &http.Cookie{
			Name:     "userId",
			Value:    user.UserId,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			// Domain:   "localhost:8081",
			Domain:  "chatbasket.live",
			Expires: expiry,
		}

		c.SetCookie(sessionCookie)
		c.SetCookie(userCookie)

		// Return SessionResponse with empty sensitive fields for web
		webResponse := &model.SessionResponse{
			UserId:        "",
			Name:          user.Name,
			Email:         user.Email,
			SessionID:     "",
			SessionExpiry: user.SessionExpiry,
		}
		return c.JSON(http.StatusOK, webResponse)
	}

	return c.JSON(http.StatusOK, user)
}

func (h *UserHandler) ResendOTP(c echo.Context) error {
	var payload model.ResendOTPPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid resend OTP payload: " + err.Error(),
			Type:    "bad_request",
		})
	}

	// Validate required fields
	if payload.Email == "" || payload.Type == "" {
		return c.JSON(http.StatusBadRequest, model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Missing required fields",
			Type:    "missing_value",
		})
	}

	// Resend OTP via service
	response, err := h.AuthService.ResendOTP(c.Request().Context(), &payload)
	if err != nil {
		return c.JSON(err.Code, err)
	}

	return c.JSON(http.StatusOK, response)
}
