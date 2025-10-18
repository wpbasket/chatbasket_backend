package personalServices

import (
	"chatbasket/db/postgresCode"
	"chatbasket/model"
	"chatbasket/personalModel"
	"chatbasket/personalUtils"
	"chatbasket/services"
	"chatbasket/utils"
	"context"
	"log"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/appwrite/sdk-for-go/query"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Template: mirror public profile methods for personal mode. Implement later.

func (ps *Service) Logout(ctx context.Context, payload *personalmodel.LogoutPayload, userId, sessionId string) (*model.StatusOkay, *model.ApiError) {
	if payload.AllSessions {
		_, err := ps.Appwrite.Users.DeleteSessions(userId)
		if err != nil {
			return nil, &model.ApiError{
				Code:    401,
				Message: "Failed to Logout from all sessions: " + err.Error(),
				Type:    "unauthorized",
			}
		}
	} else {
		_, err := ps.Appwrite.Users.DeleteSession(userId, sessionId)
		if err != nil {
			return nil, &model.ApiError{
				Code:    401,
				Message: "Failed to Logout from session: " + err.Error(),
				Type:    "unauthorized",
			}
		}
	}

	return &model.StatusOkay{Status: true, Message: "Logged out successfully"}, nil
}

func (ps *Service) CreateUserProfile(ctx context.Context, payload *personalmodel.CreateUserProfilePayload, userId *model.UserId, email string) (*personalmodel.PrivateUser, *model.ApiError) {
	// check if user profile already exists
	res, err := ps.Queries.IsUserExists(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}
	if res {
		return nil, &model.ApiError{Code: http.StatusConflict, Message: "User profile already exists", Type: "conflict"}
	}

	// generate username
	generatedUsername, err := personalutils.GenerateRandomUsername()
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Username generation failed", Type: "internal_server_error"}
	}
	// hash username
	sha256Username, err := utils.HashUsername(generatedUsername, ps.Appwrite.PersonalUsernameKey)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Username hashing failed", Type: "internal_server_error"}
	}
	// encrypt username
	b64CipherChacha20Poly1305Username, err := utils.EncryptUsername(generatedUsername, ps.Appwrite.PersonalUsernameKey, userId.StringUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Username encryption failed", Type: "internal_server_error"}
	}

	// create user profile in db separate
	dbPayload := postgresCode.CreateUserParams{
		ID:                                userId.UuidUserId,
		HmacSha256HexUsername:             sha256Username,
		B64CipherChacha20poly1305Username: b64CipherChacha20Poly1305Username,
		Name:                              payload.Name,
		ProfileType:                       payload.ProfileType,
	}
	responseUser, err := ps.Queries.CreateUser(ctx, dbPayload)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	// create alone username in db separate from main user profile for plaintext username lookup
	rdmUUID, err := uuid.NewV7()
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to generate uuid", Type: "internal_server_error"}
	}
	aloneUsernameDbPayload := postgresCode.CreateAloneUsernameParams{
		ID:       rdmUUID,
		Username: generatedUsername,
	}
	_, err = ps.Queries.CreateAloneUsername(ctx, aloneUsernameDbPayload)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to create alone username", Type: "internal_server_error"}
	}

	return personalmodel.ToPrivateUser(&responseUser, generatedUsername, email), nil
}

func (ps *Service) GetProfile(ctx context.Context, userId model.UserId, email string) (*personalmodel.PrivateUser, *model.ApiError) {
	// get user profile from db
	profile, err := ps.Queries.GetUserProfile(ctx, userId.UuidUserId)
	if err != nil {
		log.Printf("personal GetProfile failed: %v", err.Error())
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	// check if user profile exists
	if len(profile) == 0 {
		return nil, &model.ApiError{Code: http.StatusNotFound, Message: "personal GetProfile not found", Type: "not_found"}
	}

	// decrypt username
	decodeUsername, err := utils.DecryptUsername(profile[0].B64CipherChacha20poly1305Username, ps.Appwrite.PersonalUsernameKey)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "personal GetProfile failed", Type: "internal_server_error"}
	}

	userProfile := profile[0]

	avatarUrl := utils.BuildAvatarURI(&utils.AppwriteFileData{
		FileId:     userProfile.FileID,
		FileToken:  userProfile.TokenID,
		FileSecret: userProfile.TokenSecret,
	})

	var finalAvatarUrl *string
	finalAvatarUrl = avatarUrl

	// check if avatar token is expired
	now := time.Now().UTC()
	needsRefresh := false
	if userProfile.TokenExpiry.Valid {
		needsRefresh = !userProfile.TokenExpiry.Time.UTC().After(now)
	} else if userProfile.TokenID != nil && *userProfile.TokenID != "" && userProfile.TokenSecret != nil && *userProfile.TokenSecret != "" {
		needsRefresh = true
	}
	if needsRefresh {
		exp := now.AddDate(1, 0, 0).Format("2006-01-02 15:04:05")
		tok, err := ps.Appwrite.Tokens.CreateFileToken(ps.Appwrite.ProfilePicBucketID, *userProfile.FileID, ps.Appwrite.Tokens.WithCreateFileTokenExpire(exp))
		if err != nil {
			return nil, &model.ApiError{
				Code:    500,
				Message: "Failed to create personal token: " + err.Error(),
				Type:    "internal_server_error",
			}
		}
		tokTime, err := time.Parse(time.RFC3339, tok.Expire)
		if err != nil {
			return nil, &model.ApiError{
				Code:    500,
				Message: "Failed to parse expire time: " + err.Error(),
				Type:    "internal_server_error",
			}
		}
		_, err = ps.Queries.UpdateAvatarTokens(ctx, postgresCode.UpdateAvatarTokensParams{
			UserID:      userId.UuidUserId,
			TokenID:     &tok.Id,
			TokenSecret: &tok.Secret,
			TokenExpiry: pgtype.Timestamptz{Valid: true, Time: tokTime},
		})
		if err != nil {
			return nil, &model.ApiError{
				Code:    500,
				Message: "Failed to update avatar tokens: " + utils.GetPostgresError(err).Message,
				Type:    "internal_server_error",
			}
		}
		finalAvatarUrl = utils.BuildAvatarURI(&utils.AppwriteFileData{
			FileId:     userProfile.FileID,
			FileToken:  &tok.Id,
			FileSecret: &tok.Secret,
		})
	}

	return personalmodel.ToPrivateUserWithAvatar(&profile[0], decodeUsername, email, finalAvatarUrl), nil
}

func (ps *Service) UploadUserProfilePicture(ctx context.Context, fh *multipart.FileHeader, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	// check if user profile pic exists and if it exists, delete it
	resUser, err := ps.Queries.IsUserProfilePicExists(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to check user profile pic: " + utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	checkExistInStorage, err := ps.Appwrite.Storage.ListFiles(
		ps.Appwrite.PersonalProfilePicBucketID,
		ps.Appwrite.Storage.WithListFilesQueries(
			[]string{
				query.Equal("$id", userId.StringUserId),
			},
		),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to list files: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	var deleteExisting bool
	if checkExistInStorage.Total == 1 {
		deleteExisting = true
	}

	result, apiErr := ps.UploadFileFromMultipart(
		ps.Appwrite.PersonalProfilePicBucketID,
		userId.StringUserId,
		fh,
		services.UploadOptions{DeleteExisting: deleteExisting, GenerateTokens: true},
	)
	if apiErr != nil {
		return nil, apiErr
	}

	expireTime, err := time.Parse(time.RFC3339, result.Expire)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to parse expire time", Type: "internal_server_error"}
	}

	if !resUser {
		rdmUUID, err := uuid.NewV7()
		if err != nil {
			return nil, &model.ApiError{Code: 500, Message: "Failed to generate uuid", Type: "internal_server_error"}
		}
		_, err = ps.Queries.CreateAvatar(ctx, postgresCode.CreateAvatarParams{
			ID:          rdmUUID,
			UserID:      userId.UuidUserId,
			FileID:      result.FileId,
			AvatarType:  "profile",
			TokenID:     &result.TokenIDs[0],
			TokenSecret: &result.TokenSecrets[0],
			TokenExpiry: pgtype.Timestamptz{Valid: true, Time: expireTime},
		})
		if err != nil {
			return nil, &model.ApiError{Code: 500, Message: "Failed to create avatar: " + utils.GetPostgresError(err).Message, Type: "internal_server_error"}
		}
	}

	if resUser {
		_, err := ps.Queries.UpdateAvatarTokens(ctx, postgresCode.UpdateAvatarTokensParams{
			UserID:      userId.UuidUserId,
			TokenID:     &result.TokenIDs[0],
			TokenSecret: &result.TokenSecrets[0],
			TokenExpiry: pgtype.Timestamptz{Valid: true, Time: expireTime},
		})
		if err != nil {
			return nil, &model.ApiError{Code: 500, Message: "Failed to update avatar tokens: " + utils.GetPostgresError(err).Message, Type: "internal_server_error"}
		}
	}

	return &model.StatusOkay{Status: true, Message: "Avatar uploaded successfully"}, nil
}

func (ps *Service) RemoveUserProfilePicture(ctx context.Context, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	resUser, err := ps.Queries.IsUserProfilePicExists(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to check user profile pic: " + utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	checkExistInStorage, err := ps.Appwrite.Storage.ListFiles(
		ps.Appwrite.PersonalProfilePicBucketID,
		ps.Appwrite.Storage.WithListFilesQueries(
			[]string{
				query.Equal("$id", userId.StringUserId),
			},
		),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to list files: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	if checkExistInStorage.Total == 0 {
		if resUser {
			err = ps.Queries.DeleteAvatar(ctx, userId.UuidUserId)
			if err != nil {
				return nil, &model.ApiError{Code: 500, Message: "Failed to delete avatar from database: " + utils.GetPostgresError(err).Message, Type: "internal_server_error"}
			}
		}
		return nil, &model.ApiError{
			Code:    404,
			Message: "Profile picture not found",
			Type:    "not_found",
		}
	}

	// Delete the file access token
	tok, err := ps.Appwrite.Tokens.List(ps.Appwrite.PersonalProfilePicBucketID, userId.StringUserId)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query token data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if tok.Total > 0 {
		for _, token := range tok.Tokens {
			_, err := ps.Appwrite.Tokens.Delete(token.Id)
			if err != nil {
				return nil, &model.ApiError{
					Code:    500,
					Message: "Failed to delete token data: " + err.Error(),
					Type:    "internal_server_error",
				}
			}
		}
	}

	// Delete the file from storage
	_, err = ps.Appwrite.Storage.DeleteFile(
		ps.Appwrite.PersonalProfilePicBucketID,
		userId.StringUserId,
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to delete profile picture from storage: " + err.Error(),
			Type:    "not_found",
		}
	}

	if resUser {
		// Delete the avatar from the database
		err = ps.Queries.DeleteAvatar(ctx, userId.UuidUserId)
		if err != nil {
			return nil, &model.ApiError{Code: 500, Message: "Failed to delete avatar from database: " + utils.GetPostgresError(err).Message, Type: "internal_server_error"}
		}
	}

	return &model.StatusOkay{Status: true, Message: "Profile picture removed successfully"}, nil
}

func (ps *Service) UpdateUserProfile(ctx context.Context, payload *personalmodel.UpdateUserProfilePayload, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	_, err := ps.Queries.UpdateUserProfile(ctx, postgresCode.UpdateUserProfileParams{
		ID:          userId.UuidUserId,
		Name:        payload.Name,
		Bio:         payload.Bio,
		ProfileType: payload.ProfileType,
	})
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to update user profile: " + utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	return &model.StatusOkay{Status: true, Message: "Profile updated successfully"}, nil
}
