package commonservice

import (
	"chatbasket-api/common/commonmodel"
	"chatbasket-api/internal/db/auth"
	"chatbasket-api/model"
	"chatbasket-api/utils"
	"context"
	"net/http"
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
