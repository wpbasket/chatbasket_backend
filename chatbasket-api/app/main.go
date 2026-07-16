package main

import (
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/config"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/logger"
	"chatbasket-api/internal/platform/middleware"
	"chatbasket-api/internal/platform/router"
	"chatbasket-api/internal/platform/websocket"
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
)

func main() {
	e := echo.New()

	// Configure structured logging with slog( Source of Truth )
	slog.SetDefault(logger.New())
	e.Logger = slog.Default()

	// Register Global Error Handler for clean domain error mapping
	e.HTTPErrorHandler = kit.GlobalErrorHandler

	// Load application configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Register common middlewares
	middleware.Register(e, cfg.CORSOrigin)

	// Initialize Firebase
	firebaseCtx, firebaseCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := clients.InitializeFirebase(firebaseCtx, cfg.Firebase); err != nil {
		slog.Warn("Firebase initialization failed", "error", err)
	} else {
		slog.Info("Firebase initialized successfully")
	}
	firebaseCancel()

	// Initialize WebSocket Hub
	hub := websocket.NewWSHub()
	slog.Info("WebSocket Hub initialized", "active_connections", hub.ConnectionCount())

	// Initialize Secret Client (Domain Keys)
	secretClient := clients.NewSecretClient(cfg.Security)
	_ = secretClient // satisfy compiler for now

	// Initialize R2 Client Pool (mandatory — fatal if no accounts configured)
	r2Pool, err := clients.NewR2ClientPool(cfg.R2)
	if err != nil {
		slog.Error("failed to initialize R2 client pool", "error", err)
		os.Exit(1)
	}
	slog.Info("R2 client pool initialized", "accounts", len(cfg.R2.Accounts), "primaryChat", cfg.R2.PrimaryChatAccount, "primaryProfile", cfg.R2.PrimaryProfileAccount)

	// Initialize Cosmos DB Client
	cosmosClient, err := clients.ConnectCosmos(cfg.Cosmos)
	if err != nil {
		slog.Warn("Cosmos DB initialization failed", "error", err)
	} else {
		slog.Info("Cosmos DB client initialized successfully", "database", cfg.Cosmos.Database)
		_ = cosmosClient // satisfy compiler for now
	}

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := clients.ConnectPostgres(startupCtx, cfg.Postgres)
	startupCancel()
	if err != nil {
		slog.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	slog.Info("Postgres client initialized successfully")

	// --- Route Registration ---
	router.Register(e, pool, cfg, r2Pool)

	// Graceful shutdown context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Prepare StartConfig for graceful shutdown (Echo v5)
	sc := echo.StartConfig{
		Address:         ":" + cfg.Port,
		HideBanner:      true,
		GracefulTimeout: 15 * time.Second,
		BeforeServeFunc: func(s *http.Server) error {
			s.ReadHeaderTimeout = 10 * time.Second
			s.ReadTimeout = 600 * time.Second
			s.WriteTimeout = 600 * time.Second
			s.IdleTimeout = 120 * time.Second

			// Enable native HTTP/2 cleartext (h2c) support directly in the standard library (Go 1.24+)
			protocols := new(http.Protocols)
			protocols.SetHTTP1(true)
			protocols.SetUnencryptedHTTP2(true)
			s.Protocols = protocols

			return nil
		},
	}

	slog.Info("Starting server", "port", cfg.Port)
	if err := sc.Start(ctx, e); err != nil {
		slog.Error("Server error", "error", err)
		os.Exit(1)
	}

	slog.Info("Server stopped - starting final cleanup...")

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
		slog.Info("Database pool closed gracefully")
	case <-poolCloseCtx.Done():
		slog.Warn("Database pool close timeout - forcing shutdown")
	}

	// Wait for background tasks
	slog.Info("Waiting for background email tasks...")
	clients.WaitEmails()
	slog.Info("Background emails finished")

	slog.Info("Server exited")
}

