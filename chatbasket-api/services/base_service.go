package services

import (
	"chatbasket-api/appwriteinternal"
	"chatbasket-api/internal/db/auth"
	"chatbasket-api/internal/db/personal"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GlobalService struct {
	Appwrite        *appwriteinternal.AppwriteService
	DB              *pgxpool.Pool
	AuthQueries     *auth.Queries
	PersonalQueries *personal.Queries
	CosmosClient    *azcosmos.Client
	AuthService     *AuthService // Added to share Auth logic
}

func NewGlobalService(app *appwriteinternal.AppwriteService, dbpool *pgxpool.Pool, cosmosClient *azcosmos.Client, authService *AuthService) *GlobalService {
	return &GlobalService{
		Appwrite:        app,
		DB:              dbpool,
		AuthQueries:     auth.New(dbpool), // Kept for backward compat, but AuthService has them too
		PersonalQueries: personal.New(dbpool),
		CosmosClient:    cosmosClient,
		AuthService:     authService,
	}
}
