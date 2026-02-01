package services

import (
	"chatbasket-api/internal/db/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthService struct {
	DB          *pgxpool.Pool
	AuthQueries *auth.Queries
	AuthSecret  []byte
}

func NewAuthService(dbpool *pgxpool.Pool, secretKey []byte) *AuthService {
	// Fallback/Default handling could be here or at call site.
	// Ensuring a usable key is important.
	if len(secretKey) == 0 {
		// Use a hardcoded dev key if missing (matches previous logic),
		// or panic in prod. For consistency with previous middleware:
		// Note: "super-secret-key" as bytes.
		secretKey = []byte("super-secret-key")
	}
	return &AuthService{
		DB:          dbpool,
		AuthQueries: auth.New(dbpool),
		AuthSecret:  secretKey,
	}
}
