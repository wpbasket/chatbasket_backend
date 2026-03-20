package core_auth

import (
	"chatbasket-apinext/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-apinext/internal/platform/middleware"
	"context"

	"github.com/google/uuid"
)

// GetSessionByToken satisfies the middleware.AuthSessionProvider interface.
func (s *AuthService) GetSessionByToken(ctx context.Context, tokenHash string, userID uuid.UUID) (*middleware.SessionInfo, error) {
	row, err := s.PostgresQuerier.GetSessionByToken(ctx, core_auth_store.GetSessionByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		return nil, err
	}
	return &middleware.SessionInfo{
		ID:        row.ID,
		ExpiresAt: row.ExpiresAt.Time,
		IsCentral: row.IsCentral,
	}, nil
}

// GetAuthUserByID satisfies the middleware.AuthSessionProvider interface.
func (s *AuthService) GetAuthUserByID(ctx context.Context, userID uuid.UUID) (*middleware.UserInfo, error) {
	row, err := s.PostgresQuerier.GetAuthUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &middleware.UserInfo{
		ID:              row.ID,
		Email:           row.Email,
		IsEmailVerified: row.IsEmailVerified,
	}, nil
}

// GetAuthSecret satisfies the middleware.AuthSessionProvider interface.
func (s *AuthService) GetAuthSecret() []byte {
	return s.AuthSecret
}
