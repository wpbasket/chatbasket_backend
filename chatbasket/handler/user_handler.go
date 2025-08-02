package handler

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"chatbasket/model"
	"chatbasket/services"
)

type UserHandler struct {
	Service *services.GlobalService
}

func NewUserHandler(service *services.GlobalService) *UserHandler {
	return &UserHandler{Service: service}
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
	user, err := h.Service.Signup(c.Request().Context(), &payload)
	if err != nil {
		return c.JSON(err.Code,err)
		// return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Return sanitized user info
	return c.JSON(http.StatusCreated, user)
}

func (h *UserHandler) AcountVerification(c echo.Context) error {
	var payload model.AuthVerificationPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid OTP payload")
	}

	if payload.Email == "" || payload.Secret == "" || payload.Platform == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing required fields")
	}

	user, err := h.Service.AccountVerification(c.Request().Context(), &payload)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	// Handle web platform - set httpOnly cookies
	if payload.Platform == "web" {

		expiry, err := time.Parse(time.RFC3339, user.SessionExpiry)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "invalid session expiry format")
		}

		// Set cookies with actual values (before they get emptied in response)
		sessionCookie := &http.Cookie{
			Name:     "sessionId",
			Value:    user.SessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			Domain:   "localhost:8081",
			// Domain:   "chatbasket.me",
			SameSite: http.SameSiteNoneMode,
			Expires:  expiry,
		}

		userCookie := &http.Cookie{
			Name:  "userId",
			Value: user.UserId,
			Path:  "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			Domain:   "localhost:8081",
			// Domain:   "chatbasket.me",
			Expires:  expiry,
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
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid login payload: "+err.Error())
	}

	// Validate required fields
	if payload.Email == "" || payload.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing required fields")
	}

	// Login via service
	user, err := h.Service.Login(c.Request().Context(), &payload)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Return sanitized user info
	return c.JSON(http.StatusOK, user)
}

func (h *UserHandler) LoginVerification(c echo.Context) error {
	var payload model.AuthVerificationPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid OTP payload")
	}

	if payload.Email == "" || payload.Secret == "" || payload.Platform == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing required fields")
	}

	user, err := h.Service.LoginVerification(c.Request().Context(), &payload)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	// Handle web platform - set httpOnly cookies
	if payload.Platform == "web" {

		expiry, err := time.Parse(time.RFC3339, user.SessionExpiry)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "invalid session expiry format")
		}

		// Set cookies with actual values (before they get emptied in response)
		sessionCookie := &http.Cookie{
			Name:     "sessionId",
			Value:    user.SessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			Domain:   "localhost:8081",
			// Domain:   "chatbasket.me",
			Expires:  expiry,
		}

		userCookie := &http.Cookie{
			Name:  "userId",
			Value: user.UserId,
			Path:  "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			Domain:   "localhost:8081",
			// Domain:   "chatbasket.me",
			Expires:  expiry,
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
