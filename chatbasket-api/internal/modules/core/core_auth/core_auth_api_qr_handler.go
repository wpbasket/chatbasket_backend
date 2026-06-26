package core_auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"chatbasket-api/internal/platform/kit"
	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
)

// QRInitiate triggers the creation of a new QR token.
func (h *authHandler) QRInitiate(c *echo.Context) error {
	payload, err := h.Service.QRInitiate(c.Request().Context())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, payload)
}

// QRWebSocket upgrades the HTTP connection to a WebSocket for real-time signaling.
func (h *authHandler) QRWebSocket(c *echo.Context) error {
	tokenStr := c.QueryParam("token")
	token, err := h.Service.ParseAndVerifyQRToken(tokenStr)
	if err != nil {
		return err
	}

	req, err := h.Service.PostgresQuerier.GetQRLoginRequest(c.Request().Context(), token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return kit.NewError(http.StatusNotFound, "not_found", "QR request expired or not found")
		}
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to validate QR request")
	}

	wsConn, err := websocket.Accept(c.Response(), c.Request(), &websocket.AcceptOptions{
		// OriginPatterns: []string{"http://localhost:8081"},
		OriginPatterns: []string{"https://chatbasket.live"},
	})
	if err != nil {
		return err // Upgrader handles writing error response
	}

	// Register in QRHub
	h.qrHub.Register(token, wsConn)
	defer h.qrHub.Unregister(token, wsConn)
	defer wsConn.Close(websocket.StatusNormalClosure, "")

	// Close the connection when the QR request expires
	ctx, cancel := context.WithDeadline(c.Request().Context(), req.ExpiresAt)
	defer cancel()

	// Keep connection open and read messages (though mostly we use this to push)
	for {
		_, _, err := wsConn.Read(ctx)
		if err != nil {
			// Connection closed, expired, or error
			break
		}
	}

	return nil
}



// QRApprove links the user ID from the active session to the QR token.
func (h *authHandler) QRApprove(c *echo.Context) error {
	var payload struct {
		QRToken string `json:"qr_token"`
	}

	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_payload", "Invalid JSON payload")
	}

	// Extract authenticated user
	userIDVal := c.Get("uuidUserId")
	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		return kit.NewError(http.StatusUnauthorized, "unauthorized", "User ID not found in context")
	}

	resp, err := h.Service.QRApprove(c.Request().Context(), userID, payload.QRToken)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, resp)
}

// QRCallback converts the APPROVED QR request into a session cookie.
func (h *authHandler) QRCallback(c *echo.Context) error {
	var payload struct {
		QRToken string `json:"qr_token"`
	}

	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_payload", "Invalid JSON payload")
	}

	platform := "web"

	token, err := h.Service.ParseAndVerifyQRToken(payload.QRToken)
	if err != nil {
		return err
	}

	user, err := h.Service.QRCallback(c.Request().Context(), payload.QRToken, platform)
	if err != nil {
		return err
	}

	// Close the waiting QR WebSocket now that login is complete
	h.qrHub.Close(token)

	expiry, err := time.Parse(time.RFC3339, user.SessionExpiry)
	if err != nil {
		return ErrInvalidExpiryFormat
	}

	origin := c.Request().Header.Get("Origin")
	isLocal := strings.Contains(origin, "localhost:8081")
	cookieDomain := "chatbasket.live"
	cookieSecure := true
	if isLocal {
		cookieDomain = ""
		cookieSecure = false
	}

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
		Domain:   cookieDomain,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiry,
	}

	c.SetCookie(sessionCookie)
	c.SetCookie(userCookie)

	// Return SessionResponse with empty sensitive fields for web (except UserId for E2EE mapping)
	webResponse := &SessionResponse{
		UserId:            user.UserId,
		Name:              user.Name,
		Email:             user.Email,
		SessionID:         "",
		SessionExpiry:     user.SessionExpiry,
		IsPrimary:         user.IsPrimary,
		PrimaryDeviceName: user.PrimaryDeviceName,
		KeysRevision:      user.KeysRevision,
	}

	return c.JSON(http.StatusOK, webResponse)
}
