package profileservice

import (
	"chatbasket-apinext/internal/modules/personal/profile/profilekit"
	"chatbasket-apinext/internal/modules/personal/profile/profilemodels"
	"chatbasket-apinext/internal/platform/clients"
	"chatbasket-apinext/internal/platform/kit"
	"chatbasket-apinext/internal/platform/services"
	"chatbasket-apinext/internal/store/postgresgen"
	"context"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/appwrite/sdk-for-go/query"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type ProfileService struct {
	GlobalService              *services.GlobalService
	PostgresQuerier            postgresgen.Querier
	PostgresQueries            *postgresgen.Queries
	PersonalUsernameKey        []byte
	AppwriteStorage            *clients.AppwriteStorageService
	PersonalProfilePicBucketID string
}

// NewProfileService creates a new ProfileService instance.
func NewProfileService(globalService *services.GlobalService, personalUsernameKey []byte, appwriteStorage *clients.AppwriteStorageService, personalProfilePicBucketID string) *ProfileService {
	return &ProfileService{
		GlobalService:              globalService,
		PostgresQuerier:            globalService.PostgresQuerier,
		PostgresQueries:            globalService.PostgresQueries,
		PersonalUsernameKey:        personalUsernameKey,
		AppwriteStorage:            appwriteStorage,
		PersonalProfilePicBucketID: personalProfilePicBucketID,
	}
}

func (ps *ProfileService) CreateUserProfile(ctx context.Context, payload *profilemodels.CreateUserProfilePayload, userId *kit.UserId, email string) (*profilemodels.PrivateUser, *kit.ApiError) {
	if payload == nil {
		return nil, &kit.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"}
	}
	if userId == nil {
		return nil, &kit.ApiError{Code: http.StatusBadRequest, Message: "invalid user id", Type: "bad_request"}
	}
	// check if user profile already exists
	res, err := ps.PostgresQueries.IsUserExists(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: kit.GetPostgresError(err).Message, Type: "internal_server_error"}
	}
	if res {
		return nil, &kit.ApiError{Code: http.StatusConflict, Message: "User profile already exists", Type: "conflict"}
	}

	// generate username
	generatedUsername, err := profilekit.GenerateRandomUsername()
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Username generation failed", Type: "internal_server_error"}
	}
	// hash username
	sha256Username, err := kit.ComputeHMAC(generatedUsername, ps.PersonalUsernameKey, false, nil)
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Username hashing failed", Type: "internal_server_error"}
	}
	// encrypt username
	b64CipherChacha20Poly1305Username, err := profilekit.EncryptUsername(generatedUsername, ps.PersonalUsernameKey, userId.StringUserId)
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Username encryption failed", Type: "internal_server_error"}
	}

	// create user profile in db separate
	dbPayload := postgresgen.CreateUserParams{
		ID:                                userId.UuidUserId,
		HmacSha256HexUsername:             sha256Username,
		B64CipherChacha20poly1305Username: b64CipherChacha20Poly1305Username,
		Name:                              payload.Name,
		ProfileType:                       payload.ProfileType,
	}
	responseUser, err := ps.PostgresQueries.CreateUser(ctx, dbPayload)
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: kit.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	// create alone username in db separate from main user profile for plaintext username lookup
	rdmUUID, err := uuid.NewV7()
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to generate uuid", Type: "internal_server_error"}
	}
	aloneUsernameDbPayload := postgresgen.CreateAloneUsernameParams{
		ID:       rdmUUID,
		Username: generatedUsername,
	}
	_, err = ps.PostgresQueries.CreateAloneUsername(ctx, aloneUsernameDbPayload)
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to create alone username", Type: "internal_server_error"}
	}

	return profilemodels.ToPrivateUser(&responseUser, generatedUsername, email), nil
}

func (ps *ProfileService) GetProfile(ctx context.Context, userId *kit.UserId, email string) (*profilemodels.PrivateUser, *kit.ApiError) {
	// get user profile from db
	profile, err := ps.PostgresQueries.GetUserProfile(ctx, userId.UuidUserId)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &kit.ApiError{Code: http.StatusNotFound, Message: err.Error(), Type: "not_found"}
		}
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: kit.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	// decrypt username
	decodeUsername, err := profilekit.DecryptUsername(profile.B64CipherChacha20poly1305Username, ps.PersonalUsernameKey)
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "personal GetProfile failed", Type: "internal_server_error"}
	}

	var finalAvatarUrl *string
	if profile.FileID != nil && *profile.FileID != "" {
		finalAvatarUrl = kit.BuildAvatarURI(&kit.AppwriteFileData{
			FileId:     profile.FileID,
			FileToken:  profile.TokenID,
			FileSecret: profile.TokenSecret,
		})
	}

	return profilemodels.ToPrivateUserWithAvatar(&profile, decodeUsername, email, finalAvatarUrl), nil
}

func (ps *ProfileService) UpdateUserProfile(ctx context.Context, payload *profilemodels.UpdateUserProfilePayload, userId kit.UserId) (*kit.StatusOkay, *kit.ApiError) {
	_, err := ps.PostgresQueries.UpdateUserProfile(ctx, postgresgen.UpdateUserProfileParams{
		ID:          userId.UuidUserId,
		Name:        payload.Name,
		Bio:         payload.Bio,
		ProfileType: payload.ProfileType,
	})
	if err != nil {
		return nil, &kit.ApiError{Code: 500, Message: "Failed to update user profile: " + kit.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	return &kit.StatusOkay{Status: true, Message: "Profile updated successfully"}, nil
}

func (ps *ProfileService) UploadUserProfilePicture(ctx context.Context, fh *multipart.FileHeader, userId kit.UserId) (*kit.StatusOkay, *kit.ApiError) {
	if fh == nil {
		return nil, &kit.ApiError{Code: 400, Message: "no file provided", Type: "bad_request"}
	}
	// check if user profile pic exists and if it exists, delete it
	resUser, err := ps.PostgresQueries.IsUserProfilePicExists(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &kit.ApiError{Code: 500, Message: "Failed to check user profile pic: " + kit.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	checkExistInStorage, err := ps.AppwriteStorage.Storage.ListFiles(
		ps.PersonalProfilePicBucketID,
		ps.AppwriteStorage.Storage.WithListFilesQueries(
			[]string{
				query.Equal("$id", userId.StringUserId),
			},
		),
	)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    500,
			Message: "Failed to list files: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	var deleteExisting bool
	if checkExistInStorage.Total == 1 {
		deleteExisting = true
	}

	result, apiErr := services.UploadFileFromMultipart(
		ps.AppwriteStorage,
		ps.PersonalProfilePicBucketID,
		userId.StringUserId,
		fh,
		services.UploadOptions{DeleteExisting: deleteExisting, GenerateTokens: true},
	)
	if apiErr != nil {
		return nil, apiErr
	}

	if len(result.TokenIDs) == 0 || len(result.TokenSecrets) == 0 {
		return nil, &kit.ApiError{Code: 500, Message: "missing file access tokens", Type: "internal_server_error"}
	}

	expireTime, err := time.Parse(time.RFC3339, result.Expire)
	if err != nil {
		return nil, &kit.ApiError{Code: 500, Message: "Failed to parse expire time", Type: "internal_server_error"}
	}

	if !resUser {
		rdmUUID, err := uuid.NewV7()
		if err != nil {
			return nil, &kit.ApiError{Code: 500, Message: "Failed to generate uuid", Type: "internal_server_error"}
		}
		_, err = ps.PostgresQueries.CreateAvatar(ctx, postgresgen.CreateAvatarParams{
			ID:          rdmUUID,
			UserID:      userId.UuidUserId,
			FileID:      result.FileId,
			AvatarType:  "profile",
			TokenID:     new(result.TokenIDs[0]),
			TokenSecret: new(result.TokenSecrets[0]),
			TokenExpiry: pgtype.Timestamptz{Valid: true, Time: expireTime},
		})
		if err != nil {
			return nil, &kit.ApiError{Code: 500, Message: "Failed to create avatar: " + kit.GetPostgresError(err).Message, Type: "internal_server_error"}
		}
	}

	if resUser {
		_, err := ps.PostgresQueries.UpdateAvatarTokens(ctx, postgresgen.UpdateAvatarTokensParams{
			UserID:      userId.UuidUserId,
			TokenID:     new(result.TokenIDs[0]),
			TokenSecret: new(result.TokenSecrets[0]),
			TokenExpiry: pgtype.Timestamptz{Valid: true, Time: expireTime},
		})
		if err != nil {
			return nil, &kit.ApiError{Code: 500, Message: "Failed to update avatar tokens: " + kit.GetPostgresError(err).Message, Type: "internal_server_error"}
		}
	}

	return &kit.StatusOkay{Status: true, Message: "Avatar uploaded successfully"}, nil
}

func (ps *ProfileService) RemoveUserProfilePicture(ctx context.Context, userId kit.UserId) (*kit.StatusOkay, *kit.ApiError) {
	resUser, err := ps.PostgresQueries.IsUserProfilePicExists(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &kit.ApiError{Code: 500, Message: "Failed to check user profile pic: " + kit.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	checkExistInStorage, err := ps.AppwriteStorage.Storage.ListFiles(
		ps.PersonalProfilePicBucketID,
		ps.AppwriteStorage.Storage.WithListFilesQueries(
			[]string{
				query.Equal("$id", userId.StringUserId),
			},
		),
	)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    500,
			Message: "Failed to list files: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	if checkExistInStorage.Total == 0 {
		if resUser {
			err = ps.PostgresQueries.DeleteAvatar(ctx, userId.UuidUserId)
			if err != nil {
				return nil, &kit.ApiError{Code: 500, Message: "Failed to delete avatar from database: " + kit.GetPostgresError(err).Message, Type: "internal_server_error"}
			}
		}
		return nil, &kit.ApiError{
			Code:    404,
			Message: "Profile picture not found",
			Type:    "not_found",
		}
	}

	// Delete the file access token
	tok, err := ps.AppwriteStorage.Tokens.List(ps.PersonalProfilePicBucketID, userId.StringUserId)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    500,
			Message: "Failed to query token data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if tok.Total > 0 {
		for _, tokens := range tok.Tokens {
			_, err := ps.AppwriteStorage.Tokens.Delete(tokens.Id)
			if err != nil {
				return nil, &kit.ApiError{
					Code:    500,
					Message: "Failed to delete token data: " + err.Error(),
					Type:    "internal_server_error",
				}
			}
		}
	}

	// Delete the file from storage
	_, err = ps.AppwriteStorage.Storage.DeleteFile(
		ps.PersonalProfilePicBucketID,
		userId.StringUserId,
	)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    500,
			Message: "Failed to delete profile picture from storage: " + err.Error(),
			Type:    "not_found",
		}
	}

	if resUser {
		// Delete the avatar from the database
		err = ps.PostgresQueries.DeleteAvatar(ctx, userId.UuidUserId)
		if err != nil {
			return nil, &kit.ApiError{Code: 500, Message: "Failed to delete avatar from database: " + kit.GetPostgresError(err).Message, Type: "internal_server_error"}
		}
	}

	return &kit.StatusOkay{Status: true, Message: "Profile picture removed successfully"}, nil
}
