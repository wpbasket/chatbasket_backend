package personalServices

import (
	"chatbasket/db/postgresCode"
	"chatbasket/model"
	"chatbasket/services"
	"chatbasket/utils"
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Service wraps the shared GlobalService for personal-mode endpoints.
// Extend with personal-specific utilities as the feature evolves.
type Service struct {
	*services.GlobalService
}

// New constructs a personal Service from the shared GlobalService.
func New(gs *services.GlobalService) *Service {
	return &Service{GlobalService: gs}
}

func (ps *Service) buildAvatarURL(
	ctx context.Context,
	fileID, tokenID, tokenSecret *string,
	tokenExpiry pgtype.Timestamptz,
	ownerID uuid.UUID,
) (*string, *model.ApiError) {
	if fileID == nil || *fileID == "" {
		return nil, nil
	}

	finalAvatar := utils.BuildAvatarURI(&utils.AppwriteFileData{
		FileId:     fileID,
		FileToken:  tokenID,
		FileSecret: tokenSecret,
	})

	now := time.Now().UTC()
	needsRefresh := false
	if tokenExpiry.Valid {
		needsRefresh = !tokenExpiry.Time.UTC().After(now)
	} else if tokenID != nil && *tokenID != "" && tokenSecret != nil && *tokenSecret != "" {
		needsRefresh = true
	}

	if needsRefresh {
		exp := now.AddDate(1, 0, 0).Format("2006-01-02 15:04:05")
		tok, err := ps.Appwrite.Tokens.CreateFileToken(ps.Appwrite.PersonalProfilePicBucketID, *fileID, ps.Appwrite.Tokens.WithCreateFileTokenExpire(exp))
		if err != nil {
			return nil, &model.ApiError{
				Code:    http.StatusInternalServerError,
				Message: "Failed to create personal token: " + err.Error(),
				Type:    "internal_server_error",
			}
		}

		tokTime, err := time.Parse(time.RFC3339, tok.Expire)
		if err != nil {
			return nil, &model.ApiError{
				Code:    http.StatusInternalServerError,
				Message: "Failed to parse expire time: " + err.Error(),
				Type:    "internal_server_error",
			}
		}

		_, err = ps.Queries.UpdateAvatarTokens(ctx, postgresCode.UpdateAvatarTokensParams{
			UserID:      ownerID,
			TokenID:     &tok.Id,
			TokenSecret: &tok.Secret,
			TokenExpiry: pgtype.Timestamptz{Valid: true, Time: tokTime},
		})
		if err != nil {
			return nil, &model.ApiError{
				Code:    http.StatusInternalServerError,
				Message: "Failed to update avatar tokens: " + utils.GetPostgresError(err).Message,
				Type:    "internal_server_error",
			}
		}

		finalAvatar = utils.BuildAvatarURI(&utils.AppwriteFileData{
			FileId:     fileID,
			FileToken:  &tok.Id,
			FileSecret: &tok.Secret,
		})
	}

	return finalAvatar, nil
}
