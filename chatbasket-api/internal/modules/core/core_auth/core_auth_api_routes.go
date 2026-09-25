package core_auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"chatbasket-api/internal/modules/personal/personal_sse"
	"chatbasket-api/internal/platform/middleware"

	rpc_core_authv1connect "chatbasket-api/gen/proto/core/core_auth/rpc_core_authv1connect"

	"github.com/labstack/echo/v5"
)

// Register initializes the Auth module dependencies and registers its routes.
func Register(group *echo.Group, authService *AuthService, personalSseManager *personal_sse.Manager) {
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

	handler := newAuthHandler(authService, personalSseManager, qrHub)

	// Normal login and signup calls: max 5MB upload, max 30 seconds.
	// The QR websocket stays open long, so it skips the 30-second limit.
	auth := group.Group("/auth", middleware.BodyLimit(5242880), middleware.ContextTimeoutWithSkipper(30*time.Second, func(c *echo.Context) bool {
		return c.Request().Header.Get("Upgrade") == "websocket" || strings.HasSuffix(c.Request().URL.Path, "/ws")
	}))
	
	// QR Login Routes
	qr := auth.Group("/qr")
	qr.POST("/initiate", handler.QRInitiate)
	qr.GET("/ws", handler.QRWebSocket)
	qr.POST("/callback", handler.QRCallback)
	
	// Mobile (requires auth)
	qrAuth := qr.Group("")
	qrAuth.Use(middleware.AuthSessionMiddleware(authService, true, personalSseManager))
	qrAuth.POST("/approve", handler.QRApprove)
	auth.POST("/signup", handler.Signup)
	auth.POST("/signup-verification", handler.AccountVerification)
	auth.POST("/login", handler.Login)
	auth.POST("/login-verification", handler.LoginVerification)
	auth.POST("/resend-otp", handler.ResendOTP)
	auth.POST("/forgot-password", handler.ForgotPassword)
	auth.POST("/forgot-password-verify", handler.VerifyForgotPassword)

	// Logout and profile calls: max 5MB upload, max 30 seconds.
	// Delete-account needs up to 10 minutes, so it skips the 30-second
	// limit here and sets its own 10-minute limit on its route below.
	common := group.Group("/common", middleware.BodyLimit(5242880), middleware.ContextTimeoutWithSkipper(30*time.Second, func(c *echo.Context) bool {
		return c.Request().URL.Path == "/api/common/settings/account/delete/personal"
	}))
	common.Use(middleware.AuthSessionMiddleware(authService, true, personalSseManager))
	common.POST("/logout", handler.Logout)
	common.GET("/me", handler.GetUser)

	// Common Settings Routes (works for both public and personal modes)
	settings := common.Group("/settings")
	settings.POST("/update/request", handler.RequestUpdateOTP)
	settings.POST("/password/confirm", handler.ConfirmPasswordUpdate)
	settings.POST("/email/request", handler.RequestEmailUpdate)
	settings.POST("/email/confirm", handler.ConfirmEmailUpdate)
	// Delete-account erases a lot of data, so it may run up to 10 minutes.
	settings.POST("/account/delete/personal", handler.DeletePersonalAccount, middleware.ContextTimeout(10*time.Minute))

	// Same rules for the RPC version: 5MB for all calls, except
	// DeletePersonalAccount which may run up to 10 minutes.
	connectServer := newAuthConnectServer(authService, personalSseManager, qrHub)
	path, connectHandler := rpc_core_authv1connect.NewAuthServiceHandler(connectServer)
	
	group.Any(
		"/personal"+path+"*",
		echo.WrapHandler(http.StripPrefix("/api/personal", connectHandler)),
		middleware.BodyLimit(5242880),
		middleware.DynamicContextTimeout(30*time.Second, middleware.ContextTimeoutOverrides{
			rpc_core_authv1connect.AuthServiceDeletePersonalAccountProcedure: 10 * time.Minute,
		}),
		middleware.AuthSessionMiddlewareWithConfig(middleware.AuthSessionConfig{
			AuthProvider:       authService,
			RequireVerified:    true,
			PersonalSseManager: personalSseManager,
			Skipper: func(c *echo.Context) bool {
				p := c.Request().URL.Path
				return strings.HasSuffix(p, "/Signup") ||
					strings.HasSuffix(p, "/AccountVerification") ||
					strings.HasSuffix(p, "/Login") ||
					strings.HasSuffix(p, "/LoginVerification") ||
					strings.HasSuffix(p, "/ResendOTP") ||
					strings.HasSuffix(p, "/ForgotPassword") ||
					strings.HasSuffix(p, "/VerifyForgotPassword") ||
					strings.HasSuffix(p, "/QRInitiate") ||
					strings.HasSuffix(p, "/QRCallback")
			},
		}),
	)
}
