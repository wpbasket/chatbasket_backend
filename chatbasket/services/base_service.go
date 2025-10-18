package services

import (
	"chatbasket/appwriteinternal"
	"chatbasket/db/postgresCode"

	"github.com/jackc/pgx/v5/pgxpool"
)

type GlobalService struct {
    Appwrite *appwriteinternal.AppwriteService
    DB       *pgxpool.Pool
    Queries  *postgresCode.Queries
}

func NewGlobalService(app *appwriteinternal.AppwriteService, dbpool *pgxpool.Pool) *GlobalService {
    return &GlobalService{
        Appwrite: app,
        DB:       dbpool,
        Queries:  postgresCode.New(dbpool),
    }
}
