package router

import (
	"chatbasket-api/internal/modules/core/core_auth"
	"chatbasket-api/internal/modules/core/pending_uploads"
	"chatbasket-api/internal/modules/personal/personal_chat"
	"chatbasket-api/internal/modules/personal/personal_contact"
	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/modules/personal/personal_setting"
	"chatbasket-api/internal/modules/personal/personal_sse"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/config"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/middleware"
	"chatbasket-api/internal/platform/services"
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
)

// Router orchestrates route registration from various modules.
type Router struct {
	App             *echo.Echo
	Pool            *pgxpool.Pool
	MaintenancePool *pgxpool.Pool
	Config          *config.Config
	R2Pool          *clients.R2ClientPool
}

// Register is the single entry point to set up all platform and module routes.
func Register(e *echo.Echo, pool *pgxpool.Pool, maintenancePool *pgxpool.Pool, cfg *config.Config, r2Pool *clients.R2ClientPool) {
	r := New(e, pool, maintenancePool, cfg, r2Pool)
	apiGroup := r.RegisterGlobalRoutes()
	r.RegisterModuleRoutes(apiGroup)
}

// New creates a new Router instance.
func New(e *echo.Echo, pool *pgxpool.Pool, maintenancePool *pgxpool.Pool, cfg *config.Config, r2Pool *clients.R2ClientPool) *Router {
	return &Router{
		App:             e,
		Pool:            pool,
		MaintenancePool: maintenancePool,
		Config:          cfg,
		R2Pool:          r2Pool,
	}
}

// RegisterGlobalRoutes sets up the base API group and health check.
func (r *Router) RegisterGlobalRoutes() *echo.Group {
	apiGroup := r.App.Group("/api")

	// Restrict healthz endpoint to 2 requests per 5 seconds per IP (0.4 req/sec)
	healthzLimiter := echomiddleware.RateLimiter(echomiddleware.NewRateLimiterMemoryStore(0.4))

	apiGroup.GET("/healthz", func(c *echo.Context) error {
		if r.Pool != nil {
			pingCtx, cancel := context.WithTimeout(c.Request().Context(), 200*time.Millisecond)
			defer cancel()
			if err := r.Pool.Ping(pingCtx); err != nil {
				return c.JSON(http.StatusServiceUnavailable, &kit.StatusOkay{Status: false, Message: "unhealthy"})
			}
		}
		return c.JSON(http.StatusOK, &kit.StatusOkay{Status: true, Message: "ok"})
	}, healthzLimiter)
	return apiGroup
}

// RegisterModuleRoutes orchestrates registration of all domain modules.
func (r *Router) RegisterModuleRoutes(apiGroup *echo.Group) {
	globalService := services.NewGlobalService(r.Config.CORSOrigin)

	// Instantiate shared pending_uploads service (used by chat + profile).
	pendingUploadsSvc := pending_uploads.NewService(r.Pool, r.MaintenancePool, r.R2Pool)

	// 1. Auth Service
	authService := core_auth.NewAuthService(globalService, r.Pool, r.MaintenancePool, r.Config.Security.AuthSecret)

	// 2. Personal Category & SSE Stream Infrastructure
	personalGroup := apiGroup.Group("/personal")
	personalSseManager := personal_sse.Register(r.Pool, r.MaintenancePool)
	personalGroup.Use(middleware.AuthSessionMiddleware(authService, true, personalSseManager))
	personal_sse.RegisterRoutes(personalGroup, personalSseManager)

	// Register Auth Routes
	core_auth.Register(apiGroup, authService, personalSseManager)

	// 2.2 Profile Module
	profileService := personal_profile.NewProfileService(
		globalService, r.Pool, r.MaintenancePool, authService,
		r.Config.Security.PersonalUsernameKey,
		pendingUploadsSvc, r.R2Pool,
	)
	personal_profile.Register(personalGroup, profileService)
	authService.RegisterProfileCleanupProvider(profileService)

	// 2.3 Contact Module
	contactService := personal_contact.NewContactService(globalService, r.Pool, r.MaintenancePool, profileService, r.Config.Security.PersonalUsernameKey, r.Config.Security.PersonalContactKey)
	personal_contact.Register(personalGroup, contactService)

	// 2.4 Chat Module
	chatService := personal_chat.NewChatService(
		globalService,
		r.Pool, r.MaintenancePool, authService, profileService, contactService,
		pendingUploadsSvc, r.R2Pool,
	)
	personal_chat.Register(personalGroup, chatService, personalSseManager)
	contactService.RegisterChatCleanupProvider(chatService)
	authService.RegisterChatCleanupProvider(chatService)

	// Synchronous Preflight Bootstrap (ensures message_history partitions exist before opening HTTP traffic)
	if r.MaintenancePool != nil {
		bootstrapCtx, bootstrapCancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := chatService.PreflightBootstrap(bootstrapCtx); err != nil {
			bootstrapCancel()
			slog.Error("Fatal: chat module preflight bootstrap failed", "error", err)
			os.Exit(1)
		}
		bootstrapCancel()
		slog.Info("Chat module preflight bootstrap completed successfully (forward partitions verified)")
	}

	// 2.5 Settings Module
	settingService := personal_setting.NewSettingService(authService)
	personal_setting.Register(personalGroup, settingService, personalSseManager)

	// 3. Start Background Cleanup Jobs (after all modules are registered)
	pending_uploads.StartCleanupJob(pendingUploadsSvc, 20*time.Minute)
	personal_chat.StartMessageCleanupJob(chatService, 1*time.Hour)
	personal_chat.StartDatabaseCleanupJob(chatService, 1*time.Hour)
	personal_chat.StartPartitionMaintenanceJob(chatService, 1*time.Hour)
}
