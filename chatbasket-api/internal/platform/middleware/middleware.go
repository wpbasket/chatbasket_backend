package middleware

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// Register applies common middlewares to the Echo instance, ported directly from chatbasket-api/app/main.go
func Register(e *echo.Echo) {
	e.Pre(middleware.RemoveTrailingSlash())
	e.Use(middleware.RequestLogger()) // Most efficient v5 way: uses e.Logger + slog.LogAttrs
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.Secure())
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5,
		Skipper: func(c *echo.Context) bool {
			// Skip Gzip for WebSocket upgrades
			return c.Request().Header.Get("Upgrade") == "websocket"
		},
	}))
	e.Use(middleware.BodyLimit(209715200)) // 200MB as int64

	// Safeguard: Limit logic execution to 30s as per best practices
	e.Use(middleware.ContextTimeoutWithConfig(middleware.ContextTimeoutConfig{
		Timeout: 30 * time.Second,
		Skipper: func(c *echo.Context) bool {
			p := c.Path()
			// Skip timeout for long-lived connections
			return p == "/api/personal/chat/upload" ||
				p == "/api/personal/profile/upload-avatar" ||
				p == "/api/public/profile/upload-avatar" ||
				c.Request().Header.Get("Upgrade") == "websocket"
		},
	}))

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:8081"},
		// AllowOrigins: []string{"https://chatbasket.live"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		// AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "x-api-key"},
		AllowCredentials: true,
	}))

	// Rate limit: 100 requests per second per IP
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(100)))
}
