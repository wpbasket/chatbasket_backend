package core_auth

import (
	"chatbasket-api/internal/platform/middleware"
	"chatbasket-api/internal/platform/websocket"

	"github.com/labstack/echo/v5"
)

// Register initializes the Auth module dependencies and registers its routes.
func Register(group *echo.Group, authService *AuthService, hub *websocket.WSHub) {
	handler := newAuthHandler(authService, hub)

	// Auth Routes (shared across domains)
	auth := group.Group("/auth")
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
