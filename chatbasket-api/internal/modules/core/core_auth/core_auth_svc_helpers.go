package core_auth

import (
	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// IsSessionCentral checks if a session is the primary (central) device
func (s *AuthService) IsSessionCentral(ctx context.Context, userID uuid.UUID, sessionToken string) (bool, error) {
	tokenHash, err := kit.ComputeHMAC(sessionToken, s.AuthSecret, true, new(userID.String()))
	if err != nil {
		log.Printf("[DEBUG-SESSION] ComputeHMAC ERROR: %v", err)
		return false, kit.NewError(http.StatusInternalServerError, "internal_error", "Failed to process token")
	}

	session, err := s.PostgresQuerier.GetSessionByToken(ctx, core_auth_store.GetSessionByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		log.Printf("[DEBUG-SESSION] GetSessionByToken FAILED for user %s: %v", userID, err)
		if err.Error() == "no rows in result set" {
			log.Printf("[DEBUG-SESSION] No session found for hash=%s", tokenHash)
			return false, kit.NewError(http.StatusUnauthorized, "unauthorized", "Session not found")
		}
		return false, kit.NewError(http.StatusInternalServerError, "internal_error", "Database error: "+err.Error())
	}

	log.Printf("[DEBUG-SESSION] Session found: id=%s, is_central=%v", session.ID, session.IsCentral)
	return session.IsCentral, nil
}

// GetUserPrimarySession returns user's primary device session.
// This is used by other modules (like Personal/Chat) for eligibility checks.
func (s *AuthService) GetUserPrimarySession(ctx context.Context, userID uuid.UUID) (*core_auth_store.Session, error) {
	session, err := s.PostgresQuerier.GetUserPrimarySession(ctx, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusNotFound, "not_found", "Primary session not found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_error", "Database error: "+err.Error())
	}
	return &session, nil
}

// GetUserPrimarySessionID returns the session ID of the user's primary device.
// Public wrapper around GetUserPrimarySession that avoids exposing core_auth_store types.
func (s *AuthService) GetUserPrimarySessionID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	session, err := s.GetUserPrimarySession(ctx, userID)
	if err != nil {
		return uuid.Nil, err
	}
	return session.ID, nil
}

// SetCentralDevice promotes a specific session (by token) to be the Central Device.
// It first demotes all other sessions for this user to ensure uniqueness.
func (s *AuthService) SetCentralDevice(ctx context.Context, userID uuid.UUID, token string) (*kit.StatusOkay, error) {
	// 1. Compute Token Hash
	tokenHash, err := kit.ComputeHMAC(token, s.AuthSecret, true, new(userID.String()))
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to process session token")
	}

	// 2. Get session details to validate platform
	session, err := s.PostgresQuerier.GetSessionByToken(ctx, core_auth_store.GetSessionByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusNotFound, "not_found", "Session not found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to get session details: "+err.Error())
	}

	// 3. Validate platform - only native devices (iOS/Android) can be primary
	if session.Platform != nil && *session.Platform == "web" {
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "Web devices cannot be set as primary device")
	}

	// 4. Reset all sessions to non-central
	err = s.PostgresQuerier.ResetCentralSessions(ctx, userID)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to reset central sessions: "+err.Error())
	}

	// 5. Set the specific session as central
	err = s.PostgresQuerier.SetSessionCentralByToken(ctx, core_auth_store.SetSessionCentralByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusNotFound, "not_found", "Session not found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to set central session: "+err.Error())
	}

	return &kit.StatusOkay{Status: true, Message: "Session set as central device"}, nil
}

// RegisterOrUpdateFcmOrApnToken updates the FCM/APN device token for the current session.
// Device tokens are stored in the sessions table (device_token, platform, device_name columns).
// This allows push notifications to be sent to the user's device and automatically cleaned up when the session is deleted.
func (s *AuthService) RegisterOrUpdateFcmOrApnToken(ctx context.Context, payload *RegisterOrUpdateFcmOrApnTokenPayload, userID uuid.UUID, sessionToken string) (*kit.StatusOkay, error) {
	// 1. Compute session token hash
	tokenHash, err := kit.ComputeHMAC(sessionToken, s.AuthSecret, true, new(userID.String()))
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to hash session ID")
	}

	// 2. Map token type to platform (fcm -> android, apn -> ios)
	platform := "android"
	if payload.Type == "apn" {
		platform = "ios"
	}

	// 3. Update the session record with device token, platform, and device name
	err = s.PostgresQuerier.UpdateSessionDeviceToken(ctx, core_auth_store.UpdateSessionDeviceTokenParams{
		AuthUserID:  userID,
		TokenHash:   tokenHash,
		DeviceToken: &payload.Token,
		Platform:    &platform,
		DeviceName:  payload.DeviceName,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to register token: "+err.Error())
	}

	return &kit.StatusOkay{
		Status:  true,
		Message: "Notification token updated",
	}, nil
}

