package main

import (
	"chatbasket-api/db"
	"chatbasket-api/model"
	"chatbasket-api/routes"
	"chatbasket-api/utils"
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
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
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5,
		Skipper: func(c echo.Context) bool {
			// Skip Gzip for WebSocket upgrades — compression corrupts WS frames
			return c.Request().Header.Get("Upgrade") == "websocket"
		},
	}))
	e.Use(middleware.BodyLimit("200M"))

	// Safeguard: Limit logic execution to 30s for all routes EXCEPT uploads
	e.Use(middleware.ContextTimeoutWithConfig(middleware.ContextTimeoutConfig{
		Timeout: 30 * time.Second,
		Skipper: func(c echo.Context) bool {
			p := c.Path()
			// Skip timeout for uploads and WebSocket (long-lived connections)
			return p == "/api/personal/chat/upload" ||
				p == "/api/personal/profile/upload-avatar" ||
				p == "/api/public/profile/upload-avatar" ||
				c.Request().Header.Get("Upgrade") == "websocket"
		},
	}))

	e.Use(middleware.Logger())

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
		log.Printf("⚠️  Firebase initialization failed: %v\n", err)
	} else {
		log.Printf("✅ Firebase initialized successfully\n")
	}
	firebaseCancel()

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

	// Initialize Azure Cosmos DB (NoSQL API)
	var cosmosClient *azcosmos.Client // Define variable in outer scope
	cosmosCfg, err := db.LoadCosmosConfig()
	if err != nil {
		log.Printf("⚠️  Cosmos DB config issue: %v\n", err)
	} else {
		// Initialize Cosmos DB Client
		var clientErr error
		cosmosClient, clientErr = db.NewCosmosClient(cosmosCfg)
		if clientErr != nil {
			log.Printf("⚠️  Cosmos DB client creation failed: %v\n", clientErr)
		} else {
			log.Printf("✅ Cosmos DB client initialized successfully (Database: %s)", cosmosCfg.Database)
		}
	}

	routes.RegisterRoutes(e, pool, cosmosClient, hello)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Fallback
	}
	// // HTTP server timeouts for production safety
	e.Server.ReadHeaderTimeout = 10 * time.Second
	e.Server.ReadTimeout = 600 * time.Second
	e.Server.WriteTimeout = 600 * time.Second
	e.Server.IdleTimeout = 120 * time.Second

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

	// Wait for background emails to finish
	e.Logger.Info("Waiting for background email tasks...")
	utils.WaitEmails()
	e.Logger.Info("Background emails finished")

	e.Logger.Info("Server exited")
}

func hello(c echo.Context) error {
	return c.JSON(http.StatusOK, &model.StatusOkay{Status: true, Message: "Hello "})
}
