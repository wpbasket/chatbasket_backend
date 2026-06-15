package core_auth

import (
	"context"
	"time"
	"chatbasket-api/internal/platform/middleware"
	"chatbasket-api/internal/platform/websocket"

	"github.com/labstack/echo/v5"
)

// Register initializes the Auth module dependencies and registers its routes.
func Register(group *echo.Group, authService *AuthService, hub *websocket.WSHub) {
	qrHub := NewQRHub()
	// Start Postgres listener for QR WebRTC syncing
	go StartPostgresListener(context.Background(), authService.Pool, qrHub)

	// Start Cleanup Worker
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			_ = authService.CleanupQRLoginRequests(context.Background())
		}
	}()

	handler := newAuthHandler(authService, hub, qrHub)

	// Auth Routes (shared across domains)
	auth := group.Group("/auth")
	
	// QR Login Routes
	qr := auth.Group("/qr")
	qr.POST("/initiate", handler.QRInitiate)
	qr.GET("/ws", handler.QRWebSocket)
	qr.POST("/signal", handler.QRSignal)
	qr.POST("/callback", handler.QRCallback)
	
	// Mobile (requires auth)
	qrAuth := qr.Group("")
	qrAuth.Use(middleware.AuthSessionMiddleware(authService, true, hub))
	qrAuth.POST("/approve", handler.QRApprove)
	auth.POST("/signup", handler.Signup)
	auth.POST("/signup-verification", handler.AccountVerification)
	auth.POST("/login", handler.Login)
	auth.POST("/login-verification", handler.LoginVerification)
	auth.POST("/resend-otp", handler.ResendOTP)
	auth.POST("/forgot-password", handler.ForgotPassword)
	auth.POST("/forgot-password-verify", handler.VerifyForgotPassword)

	// Common Auth Routes (logout works for both modes)
	common := group.Group("/common")
	common.Use(middleware.AuthSessionMiddleware(authService, true, hub))
	common.POST("/logout", handler.Logout)
	common.GET("/me", handler.GetUser)

	// Common Settings Routes (works for both public and personal modes)
	settings := common.Group("/settings")
	settings.POST("/update/request", handler.RequestUpdateOTP)
	settings.POST("/password/confirm", handler.ConfirmPasswordUpdate)
	settings.POST("/email/request", handler.RequestEmailUpdate)
	settings.POST("/email/confirm", handler.ConfirmEmailUpdate)
}
