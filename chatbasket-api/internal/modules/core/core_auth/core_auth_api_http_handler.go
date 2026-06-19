package core_auth

import (
	"fmt"
	"net/http"
	"time"

	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/websocket"
	"github.com/labstack/echo/v5"
	"strings"
)

type authHandler struct {
	Service *AuthService
	hub     *websocket.WSHub
	qrHub   *QRHub
}

func newAuthHandler(authService *AuthService, hub *websocket.WSHub, qrHub *QRHub) *authHandler {
	return &authHandler{
		Service: authService,
		hub:     hub,
		qrHub:   qrHub,
	}
}

func (h *authHandler) Signup(c *echo.Context) error {
	var payload SignupPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(400, "missing_value", fmt.Sprintf("Invalid signup payload: %v", err))
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
		return kit.NewError(400, "bad_request", fmt.Sprintf("Invalid OTP payload: %v", err))
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

		// Determine cookie security based on host (targeting local frontend at 8081)
		origin := c.Request().Header.Get("Origin")
		isLocal := strings.Contains(origin, "localhost:8081")
		cookieDomain := "chatbasket.live"
		cookieSecure := true
		if isLocal {
			cookieDomain = "" // Browser defaults to current host (localhost)
			cookieSecure = false
		}

		// Set cookies with actual values (before they get emptied in response)
		sessionCookie := &http.Cookie{
			Name:     "sessionId",
			Value:    user.SessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   cookieSecure,
			Domain:   cookieDomain,
			SameSite: http.SameSiteLaxMode,
			Expires:  expiry,
		}

		userCookie := &http.Cookie{
			Name:     "userId",
			Value:    user.UserId,
			Path:     "/",
			HttpOnly: true,
			Secure:   cookieSecure,
			SameSite: http.SameSiteLaxMode,
			Domain:   cookieDomain,
			Expires:  expiry,
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
			KeysRevision:  user.KeysRevision,
		}
		return c.JSON(http.StatusOK, webResponse)
	}

	return c.JSON(http.StatusOK, user)
}

func (h *authHandler) Login(c *echo.Context) error {
	var payload LoginPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(400, "bad_request", fmt.Sprintf("Invalid login payload: %v", err))
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
		return kit.NewError(400, "bad_request", fmt.Sprintf("Invalid OTP payload: %v", err))
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

		// Determine cookie security based on host (targeting local frontend at 8081)
		origin := c.Request().Header.Get("Origin")
		isLocal := strings.Contains(origin, "localhost:8081")
		cookieDomain := "chatbasket.live"
		cookieSecure := true
		if isLocal {
			cookieDomain = "" // Browser defaults to current host (localhost)
			cookieSecure = false
		}

		// Set cookies with actual values (before they get emptied in response)
		sessionCookie := &http.Cookie{
			Name:     "sessionId",
			Value:    user.SessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   cookieSecure,
			SameSite: http.SameSiteLaxMode,
			Domain:   cookieDomain,
			Expires:  expiry,
		}

		userCookie := &http.Cookie{
			Name:     "userId",
			Value:    user.UserId,
			Path:     "/",
			HttpOnly: true,
			Secure:   cookieSecure,
			SameSite: http.SameSiteLaxMode,
			Domain:   cookieDomain,
			Expires:  expiry,
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
			KeysRevision:  user.KeysRevision,
		}
		return c.JSON(http.StatusOK, webResponse)
	}

	return c.JSON(http.StatusOK, user)
}

func (h *authHandler) ResendOTP(c *echo.Context) error {
	var payload ResendOTPPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(400, "bad_request", fmt.Sprintf("Invalid resend OTP payload: %v", err))
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

func (h *authHandler) ForgotPassword(c *echo.Context) error {
	var payload ForgotPasswordPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(400, "bad_request", fmt.Sprintf("Invalid forgot password payload: %v", err))
	}

	// Validate required fields
	if payload.Email == "" {
		return ErrMissingRequired
	}

	// Initiate forgot password flow via service
	response, err := h.Service.ForgotPassword(c.Request().Context(), &payload)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, response)
}

func (h *authHandler) VerifyForgotPassword(c *echo.Context) error {
	var payload ForgotPasswordVerifyPayload

	// Parse and bind request body
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(400, "bad_request", fmt.Sprintf("Invalid forgot password verify payload: %v", err))
	}

	// Validate required fields
	if payload.UpdateID == "" || payload.Otp == "" || payload.NewPassword == "" {
		return ErrMissingRequired
	}

	// Verify OTP and update password via service
	response, err := h.Service.VerifyForgotPassword(c.Request().Context(), &payload)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, response)
}
