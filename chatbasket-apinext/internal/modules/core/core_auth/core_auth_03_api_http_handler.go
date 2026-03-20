package core_auth

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

)

type authHandler struct {
	Service *AuthService
}

func newAuthHandler(authService *AuthService) *authHandler {
	return &authHandler{
		Service: authService,
	}
}

func (h *authHandler) Signup(c *echo.Context) error {
	var payload SignupPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return ErrInvalidPayload
	}

	// Validate required fields
	if payload.Email == "" || payload.Password == "" {
		return ErrMissingRequired
	}

	// Create user via service
	user, err := h.Service.Signup(c.Request().Context(), &payload)
	if err != nil {
		return err
	}

	// Return sanitized user info
	return c.JSON(http.StatusCreated, user)
}

func (h *authHandler) AccountVerification(c *echo.Context) error {
	var payload AuthVerificationPayload
	if err := c.Bind(&payload); err != nil {
		return ErrInvalidPayload
	}

	if payload.Email == "" || payload.Secret == "" || payload.Platform == "" {
		return ErrMissingRequired
	}

	user, err := h.Service.AccountVerification(c.Request().Context(), &payload)
	if err != nil {
		return err
	}

	// Handle web platform - set httpOnly cookies
	if payload.Platform == "web" {

		expiry, err := time.Parse(time.RFC3339, user.SessionExpiry)
		if err != nil {
			return ErrInvalidExpiryFormat
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
	webResponse := &SessionResponse{
			UserId:        "",
			Name:          user.Name,
			Email:         user.Email,
			SessionID:     "",
			SessionExpiry: user.SessionExpiry,
			IsPrimary:     user.IsPrimary,
		}
		return c.JSON(http.StatusOK, webResponse)
	}

	return c.JSON(http.StatusOK, user)
}

func (h *authHandler) Login(c *echo.Context) error {
	var payload LoginPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return ErrInvalidPayload
	}

	// Validate required fields
	if payload.Email == "" || payload.Password == "" {
		return ErrMissingRequired
	}

	// Login via service
	user, err := h.Service.Login(c.Request().Context(), &payload)
	if err != nil {
		return err
	}

	// Return sanitized user info
	return c.JSON(http.StatusOK, user)
}

func (h *authHandler) LoginVerification(c *echo.Context) error {
	var payload AuthVerificationPayload
	if err := c.Bind(&payload); err != nil {
		return ErrInvalidPayload
	}

	if payload.Email == "" || payload.Secret == "" || payload.Platform == "" {
		return ErrMissingRequired
	}

	user, err := h.Service.LoginVerification(c.Request().Context(), &payload)
	if err != nil {
		return err
	}

	// Handle web platform - set httpOnly cookies
	if payload.Platform == "web" {

		expiry, err := time.Parse(time.RFC3339, user.SessionExpiry)
		if err != nil {
			return ErrInvalidExpiryFormat
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
	webResponse := &SessionResponse{
			UserId:        "",
			Name:          user.Name,
			Email:         user.Email,
			SessionID:     "",
			SessionExpiry: user.SessionExpiry,
			IsPrimary:     user.IsPrimary,
		}
		return c.JSON(http.StatusOK, webResponse)
	}

	return c.JSON(http.StatusOK, user)
}

func (h *authHandler) ResendOTP(c *echo.Context) error {
	var payload ResendOTPPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return ErrInvalidPayload
	}

	// Validate required fields
	if payload.Email == "" || payload.Type == "" {
		return ErrMissingRequired
	}

	// Resend OTP via service
	response, err := h.Service.ResendOTP(c.Request().Context(), &payload)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, response)
}
