package routes

import (
	"chatbasket-api/appwriteinternal"
	"chatbasket-api/handler"
	"chatbasket-api/services"

	"chatbasket-api/utils"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

func RegisterRoutes(
	e *echo.Echo,
	pool *pgxpool.Pool,
	cosmosClient *azcosmos.Client, // Added Cosmos Client injection
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

	// Load AUTH_SECRET as bytes (Base64 encoded in Env)
	authSecret, err := utils.LoadKeyFromEnvInByte("AUTH_SECRET")
	if err != nil {
		e.Logger.Warn("AUTH_SECRET issue (" + err.Error() + "), using default dev key")
		// authSecret will be nil/empty, triggering NewAuthService fallback
	}

	globalService := services.NewGlobalService(as, pool, cosmosClient)
	authService := services.NewAuthService(pool, authSecret)
	userHandler := handler.NewUserHandler(authService)

	// Auth Routes (shared across domains)
	authGroup := e.Group("/auth")
	authGroup.POST("/signup", userHandler.Signup)
	authGroup.POST("/signup-verification", userHandler.AcountVerification)
	authGroup.POST("/login", userHandler.Login)
	authGroup.POST("/login-verification", userHandler.LoginVerification)
	authGroup.POST("/resend-otp", userHandler.ResendOTP)

	// Register common routes (shared between public and personal)
	RegisterCommonRoutes(e, globalService, authService, authSecret)

	// Register domain-specific routes
	RegisterPublicRoutes(e, globalService, authService, authSecret)
	RegisterPersonalRoutes(e, globalService, authService, authSecret)
}
