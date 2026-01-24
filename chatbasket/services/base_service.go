package services

import (
	"chatbasket/appwriteinternal"
	"chatbasket/internal/db/personal"

	"github.com/jackc/pgx/v5/pgxpool"
)

type GlobalService struct {
	Appwrite        *appwriteinternal.AppwriteService
	DB              *pgxpool.Pool
	PersonalQueries *personal.Queries
}

func NewGlobalService(app *appwriteinternal.AppwriteService, dbpool *pgxpool.Pool) *GlobalService {
	return &GlobalService{
		Appwrite:        app,
		DB:              dbpool,
		PersonalQueries: personal.New(dbpool),
	}
}
