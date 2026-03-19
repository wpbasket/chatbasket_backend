package authservice

import (
	"chatbasket-apinext/internal/modules/core/auth/authmodels"
	"chatbasket-apinext/internal/platform/kit"
	"chatbasket-apinext/internal/store/postgresgen"
	"context"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// IsSessionCentral checks if a session is the primary (central) device
func (s *AuthService) IsSessionCentral(ctx context.Context, userID uuid.UUID, sessionToken string) (bool, *kit.ApiError) {
	tokenHash, err := kit.ComputeHMAC(sessionToken, s.AuthSecret, true, new(userID.String()))
	if err != nil {
		log.Printf("[DEBUG-SESSION] ComputeHMAC ERROR: %v", err)
		return false, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to process token", Type: "internal_error"}
	}

	session, err := s.PostgresQuerier.GetSessionByToken(ctx, postgresgen.GetSessionByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		log.Printf("[DEBUG-SESSION] GetSessionByToken FAILED for user %s: %v", userID, err)
		if err.Error() == "no rows in result set" {
			log.Printf("[DEBUG-SESSION] No session found for hash=%s", tokenHash)
			return false, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Session not found", Type: "unauthorized"}
		}
		return false, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Database error: " + err.Error(), Type: "internal_error"}
	}

	log.Printf("[DEBUG-SESSION] Session found: id=%s, is_central=%v", session.ID, session.IsCentral)
	return session.IsCentral, nil
}

// GetUserPrimarySession returns user's primary device session.
// This is used by other modules (like Personal/Chat) for eligibility checks.
func (s *AuthService) GetUserPrimarySession(ctx context.Context, userID uuid.UUID) (*postgresgen.Session, *kit.ApiError) {
	session, err := s.PostgresQuerier.GetUserPrimarySession(ctx, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &kit.ApiError{Code: http.StatusNotFound, Message: "Primary session not found", Type: "not_found"}
		}
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Database error: " + err.Error(), Type: "internal_error"}
	}
	return &session, nil
}

// SetCentralDevice promotes a specific session (by token) to be the Central Device.
// It first demotes all other sessions for this user to ensure uniqueness.
func (s *AuthService) SetCentralDevice(ctx context.Context, userID uuid.UUID, token string) (*kit.StatusOkay, *kit.ApiError) {
	// 1. Compute Token Hash
	tokenHash, err := kit.ComputeHMAC(token, s.AuthSecret, true, new(userID.String()))
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to process session token", Type: "internal_server_error"}
	}

	// 2. Get session details to validate platform
	session, err := s.PostgresQuerier.GetSessionByToken(ctx, postgresgen.GetSessionByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &kit.ApiError{Code: http.StatusNotFound, Message: "Session not found", Type: "not_found"}
		}
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to get session details: " + err.Error(), Type: "internal_server_error"}
	}

	// 3. Validate platform - only native devices (iOS/Android) can be primary
	if session.Platform != nil && *session.Platform == "web" {
		return nil, &kit.ApiError{Code: http.StatusForbidden, Message: "Web devices cannot be set as primary device", Type: "forbidden"}
	}

	// 4. Reset all sessions to non-central
	err = s.PostgresQuerier.ResetCentralSessions(ctx, userID)
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to reset central sessions: " + err.Error(), Type: "internal_server_error"}
	}

	// 5. Set the specific session as central
	err = s.PostgresQuerier.SetSessionCentralByToken(ctx, postgresgen.SetSessionCentralByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &kit.ApiError{Code: http.StatusNotFound, Message: "Session not found", Type: "not_found"}
		}
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to set central session: " + err.Error(), Type: "internal_server_error"}
	}

	return &kit.StatusOkay{Status: true, Message: "Session set as central device"}, nil
}

// RegisterOrUpdateFcmOrApnToken updates the FCM/APN device token for the current session.
// Device tokens are stored in the sessions table (device_token, platform, device_name columns).
// This allows push notifications to be sent to the user's device and automatically cleaned up when the session is deleted.
func (s *AuthService) RegisterOrUpdateFcmOrApnToken(ctx context.Context, payload *authmodels.RegisterOrUpdateFcmOrApnTokenPayload, userID uuid.UUID, sessionToken string) (*kit.StatusOkay, *kit.ApiError) {
	// 1. Compute session token hash
	tokenHash, err := kit.ComputeHMAC(sessionToken, s.AuthSecret, true, new(userID.String()))
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to hash session ID",
			Type:    "internal_server_error",
		}
	}

	// 2. Map token type to platform (fcm -> android, apn -> ios)
	platform := "android"
	if payload.Type == "apn" {
		platform = "ios"
	}

	// 3. Update the session record with device token, platform, and device name
	err = s.PostgresQuerier.UpdateSessionDeviceToken(ctx, postgresgen.UpdateSessionDeviceTokenParams{
		AuthUserID:  userID,
		TokenHash:   tokenHash,
		DeviceToken: &payload.Token,
		Platform:    &platform,
		DeviceName:  payload.DeviceName,
	})
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to register token: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	return &kit.StatusOkay{
		Status:  true,
		Message: "Token registered successfully",
	}, nil
}
