package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// Register applies common middlewares to the Echo instance, ported directly from chatbasket-api/app/main.go
func Register(e *echo.Echo, corsOrigin string) {
	e.Pre(middleware.RemoveTrailingSlash())
	e.Use(middleware.RequestLogger()) // Most efficient v5 way: uses e.Logger + slog.LogAttrs
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.Secure())
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5,
		Skipper: func(c *echo.Context) bool {
			// Skip Gzip for WebSocket upgrades and Connect/gRPC RPCs (which manage their own compression)
			ct := c.Request().Header.Get("Content-Type")
			return c.Request().Header.Get("Upgrade") == "websocket" ||
				strings.HasPrefix(ct, "application/grpc") ||
				strings.HasPrefix(ct, "application/connect") ||
				strings.HasPrefix(ct, "application/proto")
		},
	}))
	e.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
		LimitBytes: 5242880, // 5MB global limit
		Skipper: func(c *echo.Context) bool {
			return c.Path() == "/api/personal/chat/history-sync/upload"
		},
	}))

	// Safeguard: Limit logic execution to 30s as per best practices
	e.Use(middleware.ContextTimeoutWithConfig(middleware.ContextTimeoutConfig{
		Timeout: 30 * time.Second,
		Skipper: func(c *echo.Context) bool {
			ct := c.Request().Header.Get("Content-Type")
			// Skip timeout for WebSockets and Connect/gRPC streams
			return c.Request().Header.Get("Upgrade") == "websocket" ||
				strings.HasPrefix(ct, "application/connect") ||
				strings.HasPrefix(ct, "application/grpc")
		},
	}))

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{corsOrigin},
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "x-api-key", "Connect-Protocol-Version", "Connect-Timeout-Ms", "Grpc-Timeout", "X-Grpc-Web", "X-User-Agent"},
		ExposeHeaders:    []string{"Connect-Content-Encoding", "Grpc-Status", "Grpc-Message", "Grpc-Status-Details-Bin"},
		AllowCredentials: true,
		MaxAge:           86400, // Cache preflight OPTIONS requests for 24 hours to reduce latency
	}))

	// Rate limit: 100 requests per second per IP
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(100)))
}

// BodyLimit returns a middleware that limits the request body size.
func BodyLimit(limitBytes int64) echo.MiddlewareFunc {
	return middleware.BodyLimit(limitBytes)
}
