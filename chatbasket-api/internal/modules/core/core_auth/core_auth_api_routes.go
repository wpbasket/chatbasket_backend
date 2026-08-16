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

	// Auth Routes (shared across domains)
	auth := group.Group("/auth")
	
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

	// Common Auth Routes (logout works for both modes)
	common := group.Group("/common")
	common.Use(middleware.AuthSessionMiddleware(authService, true, personalSseManager))
	common.POST("/logout", handler.Logout)
	common.GET("/me", handler.GetUser)

	// Common Settings Routes (works for both public and personal modes)
	settings := common.Group("/settings")
	settings.POST("/update/request", handler.RequestUpdateOTP)
	settings.POST("/password/confirm", handler.ConfirmPasswordUpdate)
	settings.POST("/email/request", handler.RequestEmailUpdate)
	settings.POST("/email/confirm", handler.ConfirmEmailUpdate)

	// Connect RPC Routes
	connectServer := newAuthConnectServer(authService, personalSseManager, qrHub)
	path, connectHandler := rpc_core_authv1connect.NewAuthServiceHandler(connectServer)
	
	group.Any(
		"/personal"+path+"*",
		echo.WrapHandler(http.StripPrefix("/api/personal", connectHandler)),
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
