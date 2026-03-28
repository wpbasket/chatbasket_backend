package router

import (
	"chatbasket-api/internal/modules/core/core_auth"
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

// Router orchestrates the registration of routes from various modules.
type Router struct {
	App             *echo.Echo
	Pool            *pgxpool.Pool
	Config          *config.Config
	AppwriteStorage *clients.AppwriteStorageService
}

// Register is the single entry point to set up all platform and module routes.
func Register(e *echo.Echo, pool *pgxpool.Pool, cfg *config.Config, appwriteStorage *clients.AppwriteStorageService) {
	r := New(e, pool, cfg, appwriteStorage)
	apiGroup := r.RegisterGlobalRoutes()
	r.RegisterModuleRoutes(apiGroup)
}

// New creates a new Router instance.
func New(e *echo.Echo, pool *pgxpool.Pool, cfg *config.Config, appwriteStorage *clients.AppwriteStorageService) *Router {
	return &Router{
		App:             e,
		Pool:            pool,
		Config:          cfg,
		AppwriteStorage: appwriteStorage,
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

// RegisterModuleRoutes orchestrates the registration of all domain modules.
func (r *Router) RegisterModuleRoutes(apiGroup *echo.Group) {
	// Initialize global services
	globalService := services.NewGlobalService()

	// WebSocket Hub (singleton, shared across all modules)
	wsHub := websocket.NewWSHub()

	// 1. Auth Module
	authService := core_auth.NewAuthService(globalService, r.Pool, r.Config.Security.AuthSecret)
	core_auth.Register(apiGroup, authService)

	// 2. Personal Category (Group of modules)
	personalGroup := apiGroup.Group("/personal")
	profileService := personal_profile.NewProfileService(globalService, r.Pool, r.Config.Security.PersonalUsernameKey, r.AppwriteStorage, r.Config.Appwrite.PersonalProfilePicBucketID)
	personal_profile.Register(personalGroup, profileService, authService)

	contactService := personal_contact.NewContactService(globalService, r.Pool, profileService, r.Config.Security.PersonalUsernameKey, r.Config.Security.PersonalContactKey)
	personal_contact.Register(personalGroup, contactService, authService)

	// 3. Chat Module
	chatService := personal_chat.NewChatService(r.Pool, authService, profileService, r.AppwriteStorage)
	personal_chat.Register(personalGroup, chatService, wsHub, authService)
	personal_chat.StartMessageCleanupJob(chatService, 1*time.Hour)

	// 4. Settings Module
	settingService := personal_setting.NewSettingService(authService)
	personal_setting.Register(personalGroup, settingService)
}

