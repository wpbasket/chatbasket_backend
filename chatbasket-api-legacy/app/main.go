package main

import (
	"chatbasket-api-legacy/db"
	"chatbasket-api-legacy/routes"
	"chatbasket-api-legacy/utils"
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/labstack/gommon/log"
	"log/slog"
)

func main() {
	e := echo.New()

	// Configure structured logging with slog (Source of Truth)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	e.Logger = logger

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
		// AllowOrigins: []string{"http://localhost:8081"},
		AllowOrigins: []string{"https://chatbasket.live"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		// AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "x-api-key"},
		AllowCredentials: true,
	}))

	// Try loading .env files, but don't fail if missing (production uses real env vars)
	if err := godotenv.Load(".env"); err != nil {
		if err := godotenv.Load("../.env"); err != nil {
			e.Logger.Warn("No .env file found, using system environment variables")
		}
	}

	// Initialize Firebase
	firebaseCtx, firebaseCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := utils.InitializeFirebase(firebaseCtx); err != nil {
		log.Printf("âš ï¸  Firebase initialization failed: %v\n", err)
	} else {
		log.Printf("âœ… Firebase initialized successfully\n")
	}
	firebaseCancel()

	// Rate limit: 100 requests per second per IP
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(100)))

	cfg, err := db.LoadPostgresConfig()
	if err != nil {
		e.Logger.Error("failed to load postgres config", "error", err)
		os.Exit(1)
	}
	// Create pool with startup timeout context
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := db.NewPool(startupCtx, cfg)
	startupCancel()
	if err != nil {
		e.Logger.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}

	// Initialize Azure Cosmos DB (NoSQL API)
	var cosmosClient *azcosmos.Client // Define variable in outer scope
	cosmosCfg, err := db.LoadCosmosConfig()
	if err != nil {
		log.Printf("âš ï¸  Cosmos DB config issue: %v\n", err)
	} else {
		// Initialize Cosmos DB Client
		var clientErr error
		cosmosClient, clientErr = db.NewCosmosClient(cosmosCfg)
		if clientErr != nil {
			log.Printf("âš ï¸  Cosmos DB client creation failed: %v\n", clientErr)
		} else {
			log.Printf("âœ… Cosmos DB client initialized successfully (Database: %s)", cosmosCfg.Database)
		}
	}

	routes.RegisterRoutes(e, pool, cosmosClient)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Prepare Echo v5 StartConfig
	sc := echo.StartConfig{
		Address:         ":" + port,
		HideBanner:      true,
		GracefulTimeout: 15 * time.Second,
		BeforeServeFunc: func(s *http.Server) error {
			s.ReadHeaderTimeout = 10 * time.Second
			s.ReadTimeout = 600 * time.Second
			s.WriteTimeout = 600 * time.Second
			s.IdleTimeout = 120 * time.Second
			return nil
		},
		OnShutdownError: func(err error) {
			e.Logger.Error("Server forced to shutdown", "error", err)
		},
	}

	// Wait for interrupt signal to gracefully shutdown the server
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start server with config and context (v5 handles graceful shutdown)
	e.Logger.Info("Starting server", "port", port)
	if err := sc.Start(ctx, e); err != nil {
		e.Logger.Error("Server error", "error", err)
		os.Exit(1)
	}

	// After Start returns, the server has shut down gracefully according to StartConfig
	e.Logger.Info("Server stopped - starting final cleanup...")

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

	// Wait for background emails to finish
	e.Logger.Info("Waiting for background email tasks...")
	utils.WaitEmails()
	e.Logger.Info("Background emails finished")

	e.Logger.Info("Server exited")
}

