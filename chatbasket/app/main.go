package main

import (
	"chatbasket/db"
	"chatbasket/model"
	"chatbasket/routes"
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
)

func main() {
	e := echo.New()
	e.Logger.SetLevel(log.ERROR)
	e.HideBanner = true
	e.Pre(middleware.RemoveTrailingSlash())
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.Secure())
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{Level: 5}))
	e.Use(middleware.BodyLimit("10M"))

	e.Use(middleware.Logger())

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:8081"},
		// AllowOrigins: []string{"https://chatbasket.me"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		// AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "x-api-key"},
		AllowCredentials: true,
	}))



	err := godotenv.Load("../.env")
	if err != nil {
		e.Logger.Fatal("Error loading .env file", err)
	}



	// Rate limit: 100 requests per second per IP
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(100)))

	cfg, err := db.LoadPostgresConfig()
	if err != nil {
		e.Logger.Fatal("failed to load postgres config: " + err.Error())
	}
	// Create pool with startup timeout context
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := db.NewPool(startupCtx, cfg)
	startupCancel()
	if err != nil {
		e.Logger.Fatal("failed to connect to postgres: " + err.Error())
	}

	e.GET("/healthz", func(c echo.Context) error {
		pingCtx, cancel := context.WithTimeout(c.Request().Context(), 200*time.Millisecond)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, &model.StatusOkay{Status: false, Message: "unhealthy"})
		}
		return c.JSON(http.StatusOK, &model.StatusOkay{Status: true, Message: "ok"})
	})

	routes.RegisterRoutes(e, pool)

	e.GET("/", hello)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Fallback
	}
	// // HTTP server timeouts for production safety
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 15 * time.Second
	e.Server.WriteTimeout = 15 * time.Second
	e.Server.IdleTimeout = 60 * time.Second

	// Start server in a goroutine
	go func() {
		if err := e.Start(":" + port); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down the server", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	// Kill signal with grace period of 30 seconds
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	e.Logger.Info("Received shutdown signal - starting graceful shutdown...")

	// Heroku allows 30 seconds total for graceful shutdown
	// Allocate 15s for server shutdown, 5s for DB cleanup, 10s buffer
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Shutdown Echo server
	if err := e.Shutdown(ctx); err != nil {
		e.Logger.Error("Server forced to shutdown: ", err)
	}

	// Close PostgreSQL connection pool with timeout
	poolCloseCtx, poolCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer poolCancel()
	
	done := make(chan struct{})
	go func() {
		defer close(done)
		pool.Close()
	}()
	
	select {
	case <-done:
		e.Logger.Info("Database pool closed gracefully")
	case <-poolCloseCtx.Done():
		e.Logger.Warn("Database pool close timeout - forcing shutdown")
	}
	
	e.Logger.Info("Server exited")
}

func hello(c echo.Context) error {
	return c.JSON(http.StatusOK, &model.StatusOkay{Status: true, Message: "Hello "})
}
