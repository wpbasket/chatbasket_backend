package commonservice

import (
	"chatbasket-api/common/commonmodel"
	"chatbasket-api/internal/db/auth"
	"chatbasket-api/model"
	"chatbasket-api/utils"
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Logout handles logout from single or all sessions
// Works for both public and personal modes
func (s *Service) Logout(ctx context.Context, payload *commonmodel.LogoutPayload, userId model.UserId, sessionId string) (*model.StatusOkay, *model.ApiError) {
	if payload.AllSessions {
		// Logout from all sessions - delete from PostgreSQL
		err := s.AuthQueries.DeleteAllUserSessions(ctx, userId.UuidUserId)
		if err != nil {
			return nil, &model.ApiError{
				Code:    http.StatusInternalServerError,
				Message: "Failed to logout from all sessions: " + err.Error(),
				Type:    "internal_server_error",
			}
		}

		// Delete all FCM/APN tokens for this user (only exists in personal mode)
		if s.PersonalQueries != nil {
			err = s.PersonalQueries.DeleteUserTokens(ctx, userId.UuidUserId)
			if err != nil {
				// Log error but don't fail the logout
				// Tokens will be cleaned up by periodic cleanup job
			}
		}
	} else {
		// Logout from single session - delete from PostgreSQL using token hash
		tokenHash, err := utils.ComputeHMAC(sessionId, s.AuthSecret)
		if err != nil {
			return nil, &model.ApiError{
				Code:    http.StatusInternalServerError,
				Message: "Failed to hash session token: " + err.Error(),
				Type:    "internal_server_error",
			}
		}

		err = s.AuthQueries.DeleteSessionByToken(ctx, auth.DeleteSessionByTokenParams{
			TokenHash:  tokenHash,
			AuthUserID: userId.UuidUserId,
		})
		if err != nil {
			return nil, &model.ApiError{
				Code:    http.StatusInternalServerError,
				Message: "Failed to logout from session: " + err.Error(),
				Type:    "internal_server_error",
			}
		}

		// Token cleanup will happen via periodic cleanup job
	}

	return &model.StatusOkay{Status: true, Message: "Logged out successfully"}, nil
}

// GetUserWithSession retrieves user and session details (similar to login response)
func (s *Service) GetUserWithSession(ctx context.Context, userID uuid.UUID, sessionToken string) (*model.SessionResponse, *model.ApiError) {
	// 1. Compute HMAC
	tokenHash, err := utils.ComputeHMAC(sessionToken, s.AuthSecret)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to process token", Type: "internal_error"}
	}

	// 2. Get Session
	session, err := s.AuthQueries.GetSessionByToken(ctx, auth.GetSessionByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.ApiError{Code: http.StatusUnauthorized, Message: "Session not found", Type: "unauthorized"}
		}
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Database error: " + err.Error(), Type: "internal_error"}
	}

	// 3. Get User
	user, err := s.AuthQueries.GetAuthUserByID(ctx, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.ApiError{Code: http.StatusNotFound, Message: "User not found", Type: "not_found"}
		}
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Database error: " + err.Error(), Type: "internal_error"}
	}

	// 4. Determine Central Device Name
	centralDeviceName := ""
	if session.IsCentral {
		if session.DeviceName != nil {
			centralDeviceName = *session.DeviceName
		}
	} else {
		centralSession, err := s.AuthQueries.GetCentralSession(ctx, userID)
		if err == nil && centralSession.DeviceName != nil {
			centralDeviceName = *centralSession.DeviceName
		}
	}

	// 5. Construct Response
	return &model.SessionResponse{
		UserId:            user.ID.String(),
		Name:              user.Name,
		Email:             user.Email,
		SessionID:         session.ID.String(),
		SessionExpiry:     session.ExpiresAt.Time.Format(time.RFC3339),
		IsPrimary:         session.IsCentral,
		PrimaryDeviceName: centralDeviceName,
	}, nil
}
