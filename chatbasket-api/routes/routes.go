package routes

import (
	"chatbasket-api/appwriteinternal"
	"chatbasket-api/handler"
	"chatbasket-api/model"
	"chatbasket-api/services"
	"context"
	"net/http"
	"time"

	"chatbasket-api/utils"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

func RegisterRoutes(
	e *echo.Echo,
	pool *pgxpool.Pool,
	cosmosClient *azcosmos.Client, // Added Cosmos Client injection
	hello echo.HandlerFunc,
) {

	cfg, err := loadAppwriteConfig()
	if err != nil {
		e.Logger.Fatal("failed to load appwrite config: " + err.Error())
	}

	as := appwriteinternal.NewAppwriteService(
		cfg.Endpoint,
		cfg.ProjectID,
		cfg.ApiKey,
		cfg.DatabaseID,
		cfg.UsersCollectionID,
		cfg.PostsCollectionID,
		cfg.CommentsCollectionID,
		cfg.BlockCollectionID,
		cfg.LikesCollectionID,
		cfg.FollowCollectionID,
		cfg.RefreshTokensCollectionID,
		cfg.FollowRequestsCollectionID,
		cfg.TempOtpCollectionID,
		cfg.FileUserProfilePicBucketID,
		cfg.PersonalUsersCollectionID,
		cfg.PersonalAloneUsernameCollectionID,
		cfg.PersonalDatabaseID,
		cfg.PersonalProfilePicBucketID,
		cfg.PersonalUsernameKey,
	)

	ass := appwriteinternal.NewAppwriteStorageService(
		cfg.Endpoint,
		cfg.ProjectID,
		cfg.ApiKey,
	)

	// Load AUTH_SECRET as bytes (Base64 encoded in Env)
	authSecret, err := utils.LoadKeyFromEnvInByte("AUTH_SECRET")
	if err != nil {
		e.Logger.Warn("AUTH_SECRET issue (" + err.Error() + "), using default dev key")
		// authSecret will be nil/empty, triggering NewAuthService fallback
	}

	authService := services.NewAuthService(pool, authSecret)
	globalService := services.NewGlobalService(as, ass, pool, cosmosClient, authService)
	userHandler := handler.NewUserHandler(authService)

	// Global API Group
	api := e.Group("/api")

	api.GET("/", hello)
	api.GET("/healthz", func(c echo.Context) error {
		pingCtx, cancel := context.WithTimeout(c.Request().Context(), 200*time.Millisecond)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, &model.StatusOkay{Status: false, Message: "unhealthy"})
		}
		return c.JSON(http.StatusOK, &model.StatusOkay{Status: true, Message: "ok"})
	})

	// Auth Routes (shared across domains)
	authGroup := api.Group("/auth")
	authGroup.POST("/signup", userHandler.Signup)
	authGroup.POST("/signup-verification", userHandler.AcountVerification)
	authGroup.POST("/login", userHandler.Login)
	authGroup.POST("/login-verification", userHandler.LoginVerification)
	authGroup.POST("/resend-otp", userHandler.ResendOTP)

	// Register common routes (shared between public and personal)
	RegisterCommonRoutes(api, globalService, authService, authSecret)

	// Register domain-specific routes
	RegisterPublicRoutes(api, globalService, authService, authSecret)
	RegisterPersonalRoutes(api, globalService, authService, authSecret)
}
