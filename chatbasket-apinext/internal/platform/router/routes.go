package router

import (
	"chatbasket-apinext/internal/modules/core/auth/authapi"
	"chatbasket-apinext/internal/modules/core/auth/authservice"
	"chatbasket-apinext/internal/modules/personal/profile/profileapi"
	"chatbasket-apinext/internal/modules/personal/profile/profileservice"
	"chatbasket-apinext/internal/platform/clients"
	"chatbasket-apinext/internal/platform/config"
	"chatbasket-apinext/internal/platform/kit"
	"chatbasket-apinext/internal/platform/services"
	"chatbasket-apinext/internal/store/postgresgen"
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
)

// Router orchestrates the registration of routes from various modules.
type Router struct {
	App    *echo.Echo
	Pool   *pgxpool.Pool
	Config *config.Config
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
		App:    e,
		Pool:   pool,
		Config: cfg,
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
	// Create postgres store once at router level
	postgresStore := postgresgen.New(r.Pool)

	// Initialize global services
	globalService := services.NewGlobalService(postgresStore)

	// 1. Auth Module
	authService := authservice.NewAuthService(globalService, r.Config.Security.AuthSecret)
	authapi.Register(apiGroup, authService)

	// 2. Personal Category (Group of modules)
	personalGroup := apiGroup.Group("/personal")
	profileService := profileservice.NewProfileService(globalService, r.Config.Security.PersonalUsernameKey, r.AppwriteStorage, r.Config.Appwrite.PersonalProfilePicBucketID)
	profileapi.Register(personalGroup, profileService, authService)
}
