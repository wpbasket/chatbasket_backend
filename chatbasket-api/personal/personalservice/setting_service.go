package personalservice

import (
	"chatbasket-api/internal/db/auth"
	"chatbasket-api/model"
	"chatbasket-api/personal/personalmodel"
	"chatbasket-api/utils"
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SetCentralDevice promotes a specific session (by token) to be the Central Device.
// It first demotes all other sessions for this user to ensure uniqueness.
func (s *Service) SetCentralDevice(ctx context.Context, userID uuid.UUID, token string) (*model.StatusOkay, *model.ApiError) {
	// 1. Compute Token Hash
	tokenHash, err := utils.ComputeHMAC(token, s.AuthSecret)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to process session token", Type: "internal_server_error"}
	}

	// 2. Get session details to validate platform
	session, err := s.AuthQueries.GetSessionByToken(ctx, auth.GetSessionByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.ApiError{Code: http.StatusNotFound, Message: "Session not found", Type: "not_found"}
		}
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to get session details: " + err.Error(), Type: "internal_server_error"}
	}

	// 3. Validate platform - only native devices (iOS/Android) can be primary
	if session.Platform != nil && *session.Platform == "web" {
		return nil, &model.ApiError{Code: http.StatusForbidden, Message: "Web devices cannot be set as primary device", Type: "forbidden"}
	}

	// 4. Reset all sessions to non-central
	err = s.AuthQueries.ResetCentralSessions(ctx, userID)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to reset central sessions: " + err.Error(), Type: "internal_server_error"}
	}

	// 5. Set the specific session as central
	err = s.AuthQueries.SetSessionCentralByToken(ctx, auth.SetSessionCentralByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.ApiError{Code: http.StatusNotFound, Message: "Session not found", Type: "not_found"}
		}
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to set central session: " + err.Error(), Type: "internal_server_error"}
	}

	return &model.StatusOkay{Status: true, Message: "Session set as central device"}, nil
}

// UpdateSessionNotificationToken updates the FCM/APN token for the current session.
func (s *Service) UpdateSessionNotificationToken(ctx context.Context, userID uuid.UUID, sessionToken string, payload *personalmodel.RegisterOrUpdateFcmOrApnTokenPayload) (*model.StatusOkay, *model.ApiError) {
	// 1. Compute Token Hash
	tokenHash, err := utils.ComputeHMAC(sessionToken, s.AuthSecret)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to process session token", Type: "internal_server_error"}
	}

	// 2. Map platform from payload Type
	platform := "android"
	if payload.Type == "apn" {
		platform = "ios"
	}

	// 3. Update session with token and platform and device_name
	err = s.AuthQueries.UpdateSessionDeviceToken(ctx, auth.UpdateSessionDeviceTokenParams{
		AuthUserID:  userID,
		TokenHash:   tokenHash,
		DeviceToken: &payload.Token,
		Platform:    &platform,
		DeviceName:  payload.DeviceName,
	})
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to update notification token", Type: "internal_server_error"}
	}

	return &model.StatusOkay{Status: true, Message: "Notification token updated"}, nil
}
