package services

import (
	"chatbasket-api/internal/db/auth"
	"chatbasket-api/model"
	"chatbasket-api/utils"
	"context"
	"log"
	"net/http"

	"github.com/google/uuid"
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

// IsSessionCentral checks if a session is the primary (central) device
func (s *AuthService) IsSessionCentral(ctx context.Context, userID uuid.UUID, sessionToken string) (bool, *model.ApiError) {
	tokenHash, err := utils.ComputeHMAC(sessionToken, s.AuthSecret)
	if err != nil {
		log.Printf("[DEBUG-SESSION] ComputeHMAC ERROR: %v", err)
		return false, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to process token", Type: "internal_error"}
	}

	session, err := s.AuthQueries.GetSessionByToken(ctx, auth.GetSessionByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		log.Printf("[DEBUG-SESSION] GetSessionByToken FAILED for user %s: %v", userID, err)
		if err.Error() == "no rows in result set" {
			log.Printf("[DEBUG-SESSION] No session found for hash=%s", tokenHash)
			return false, &model.ApiError{Code: http.StatusUnauthorized, Message: "Session not found", Type: "unauthorized"}
		}
		return false, &model.ApiError{Code: http.StatusInternalServerError, Message: "Database error: " + err.Error(), Type: "internal_error"}
	}

	log.Printf("[DEBUG-SESSION] Session found: id=%s, is_central=%v", session.ID, session.IsCentral)
	return session.IsCentral, nil
}
