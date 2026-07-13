package router

import (
	"chatbasket-api/internal/modules/core/core_auth"
	"chatbasket-api/internal/modules/core/pending_uploads"
	"chatbasket-api/internal/modules/personal/personal_chat"
	"chatbasket-api/internal/modules/personal/personal_contact"
	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/modules/personal/personal_setting"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/config"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/services"
	"chatbasket-api/internal/platform/websocket"
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
)

// Router orchestrates route registration from various modules.
type Router struct {
	App    *echo.Echo
	Pool   *pgxpool.Pool
	Config *config.Config
	R2Pool *clients.R2ClientPool
}

// Register is the single entry point to set up all platform and module routes.
func Register(e *echo.Echo, pool *pgxpool.Pool, cfg *config.Config, r2Pool *clients.R2ClientPool) {
	r := New(e, pool, cfg, r2Pool)
	apiGroup := r.RegisterGlobalRoutes()
	r.RegisterModuleRoutes(apiGroup)
}

// New creates a new Router instance.
func New(e *echo.Echo, pool *pgxpool.Pool, cfg *config.Config, r2Pool *clients.R2ClientPool) *Router {
	return &Router{
		App:    e,
		Pool:   pool,
		Config: cfg,
		R2Pool: r2Pool,
	}
}

// RegisterGlobalRoutes sets up the base API group and health check.
func (r *Router) RegisterGlobalRoutes() *echo.Group {
	apiGroup := r.App.Group("/api")
	apiGroup.GET("/healthz", func(c *echo.Context) error {
		pingCtx, cancel := context.WithTimeout(c.Request().Context(), 200*time.Millisecond)
		defer cancel()
		if err := r.Pool.Ping(pingCtx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, &kit.StatusOkay{Status: false, Message: "unhealthy"})
		}
		return c.JSON(http.StatusOK, &kit.StatusOkay{Status: true, Message: "ok"})
	})
	return apiGroup
}

// RegisterModuleRoutes orchestrates registration of all domain modules.
func (r *Router) RegisterModuleRoutes(apiGroup *echo.Group) {
	globalService := services.NewGlobalService(r.Config.CORSOrigin)
	wsHub := websocket.NewWSHub()

	// Instantiate shared pending_uploads service (used by chat + profile).
	pendingUploadsSvc := pending_uploads.NewService(r.Pool, r.R2Pool)

	// 1. Auth Module
	authService := core_auth.NewAuthService(globalService, r.Pool, r.Config.Security.AuthSecret)
	core_auth.Register(apiGroup, authService, wsHub)

	// 2. Personal Category
	personalGroup := apiGroup.Group("/personal")
	profileService := personal_profile.NewProfileService(
		globalService, r.Pool, authService,
		r.Config.Security.PersonalUsernameKey,
		pendingUploadsSvc, r.R2Pool,
	)
	personal_profile.Register(personalGroup, profileService, authService, wsHub)


	contactService := personal_contact.NewContactService(globalService, r.Pool, profileService, r.Config.Security.PersonalUsernameKey, r.Config.Security.PersonalContactKey)
	personal_contact.Register(personalGroup, contactService, authService)

	// 3. Chat Module
	chatService := personal_chat.NewChatService(
		globalService,
		r.Pool, authService, profileService, contactService,
		pendingUploadsSvc, r.R2Pool,
	)
	personal_chat.Register(personalGroup, chatService, wsHub, authService)
	contactService.RegisterChatCleanupProvider(chatService)

	// 4. Settings Module
	settingService := personal_setting.NewSettingService(authService)
	personal_setting.Register(personalGroup, settingService, wsHub)

	// 5. Start Background Cleanup Jobs (after all modules are registered)
	pending_uploads.StartCleanupJob(pendingUploadsSvc, 15*time.Minute)
	personal_chat.StartMessageCleanupJob(chatService, 1*time.Hour)
	personal_chat.StartDatabaseCleanupJob(chatService, 1*time.Hour)
}
