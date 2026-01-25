package routes

import (
	"chatbasket/appwriteinternal"
	"chatbasket/handler"
	"chatbasket/services"

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

	globalService := services.NewGlobalService(as, pool, cosmosClient)
	userHandler := handler.NewUserHandler(globalService)

	// Auth Routes (shared across domains)
	authGroup := e.Group("/auth")
	authGroup.POST("/signup", userHandler.Signup)
	authGroup.POST("/signup-verification", userHandler.AcountVerification)
	authGroup.POST("/login", userHandler.Login)
	authGroup.POST("/login-verification", userHandler.LoginVerification)

	// Register domain-specific routes
	RegisterPublicRoutes(e, globalService)
	RegisterPersonalRoutes(e, globalService)
}
