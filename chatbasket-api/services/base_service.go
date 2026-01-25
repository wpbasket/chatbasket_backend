package services

import (
	"chatbasket/appwriteinternal"
	"chatbasket/internal/db/personal"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GlobalService struct {
	Appwrite        *appwriteinternal.AppwriteService
	DB              *pgxpool.Pool
	PersonalQueries *personal.Queries
	CosmosClient    *azcosmos.Client
}

func NewGlobalService(app *appwriteinternal.AppwriteService, dbpool *pgxpool.Pool, cosmosClient *azcosmos.Client) *GlobalService {
	return &GlobalService{
		Appwrite:        app,
		DB:              dbpool,
		PersonalQueries: personal.New(dbpool),
		CosmosClient:    cosmosClient,
	}
}
