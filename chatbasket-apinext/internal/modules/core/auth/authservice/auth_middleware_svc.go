package authservice

import (
	"chatbasket-apinext/internal/store/postgresgen"
	"context"
	"github.com/google/uuid"
)

// GetSessionByToken satisfies the middleware.AuthSessionProvider interface.
func (s *AuthService) GetSessionByToken(ctx context.Context, params postgresgen.GetSessionByTokenParams) (postgresgen.Session, error) {
	return s.PostgresQuerier.GetSessionByToken(ctx, params)
}

// GetAuthUserByID satisfies the middleware.AuthSessionProvider interface.
func (s *AuthService) GetAuthUserByID(ctx context.Context, userID uuid.UUID) (postgresgen.AuthUser, error) {
	return s.PostgresQuerier.GetAuthUserByID(ctx, userID)
}

// GetAuthSecret satisfies the middleware.AuthSessionProvider interface.
func (s *AuthService) GetAuthSecret() []byte {
	return s.AuthSecret
}
