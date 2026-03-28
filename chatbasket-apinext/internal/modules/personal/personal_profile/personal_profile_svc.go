package personal_profile

import (
	"chatbasket-apinext/internal/modules/personal/personal_profile/internal/personal_profile_store"
	"chatbasket-apinext/internal/platform/clients"
	"chatbasket-apinext/internal/platform/kit"
	"chatbasket-apinext/internal/platform/services"
	"context"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/appwrite/sdk-for-go/query"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type profileService struct {
	GlobalService              *services.GlobalService
	PostgresQuerier            personal_profile_store.Querier
	PostgresQueries            *personal_profile_store.Queries
	PersonalUsernameKey        []byte
	AppwriteStorage            *clients.AppwriteStorageService
	PersonalProfilePicBucketID string
}

// NewProfileService creates a new ProfileService instance.
func NewProfileService(globalService *services.GlobalService, pool *pgxpool.Pool, personalUsernameKey []byte, appwriteStorage *clients.AppwriteStorageService, personalProfilePicBucketID string) *profileService {
	store := personal_profile_store.New(pool)
	return &profileService{
		GlobalService:              globalService,
		PostgresQuerier:            store,
		PostgresQueries:            store,
		PersonalUsernameKey:        personalUsernameKey,
		AppwriteStorage:            appwriteStorage,
		PersonalProfilePicBucketID: personalProfilePicBucketID,
	}
}

func (ps *profileService) CreateUserProfile(ctx context.Context, payload *createUserProfilePayload, userId *kit.UserId, email string) (*privateUser, error) {
	if payload == nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}
	if userId == nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid user id")
	}
	// check if user profile already exists
	res, err := ps.PostgresQueries.IsUserExists(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	if res {
		return nil, kit.NewError(http.StatusConflict, "conflict", "User profile already exists")
	}

	// generate username
	generatedUsername, err := generateRandomUsername()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Username generation failed")
	}
	// hash username
	sha256Username, err := kit.ComputeHMAC(generatedUsername, ps.PersonalUsernameKey, false, nil)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Username hashing failed")
	}
	// encrypt username
	b64CipherChacha20Poly1305Username, err := EncryptUsername(generatedUsername, ps.PersonalUsernameKey, userId.StringUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Username encryption failed")
	}

	// create user profile in db separate
	dbPayload := personal_profile_store.CreateUserParams{
		ID:                                userId.UuidUserId,
		HmacSha256HexUsername:             sha256Username,
		B64CipherChacha20poly1305Username: b64CipherChacha20Poly1305Username,
		Name:                              payload.Name,
		ProfileType:                       payload.ProfileType,
	}
	responseUser, err := ps.PostgresQueries.CreateUser(ctx, dbPayload)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	// create alone username in db separate from main user profile for plaintext username lookup
	rdmUUID, err := uuid.NewV7()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate uuid")
	}
	aloneUsernameDbPayload := personal_profile_store.CreateAloneUsernameParams{
		ID:       rdmUUID,
		Username: generatedUsername,
	}
	_, err = ps.PostgresQueries.CreateAloneUsername(ctx, aloneUsernameDbPayload)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to create alone username")
	}

	return toPrivateUser(&responseUser, generatedUsername, email), nil
}

func (ps *profileService) GetProfile(ctx context.Context, userId *kit.UserId, email string) (*privateUser, error) {
	// get user profile from db
	profile, err := ps.PostgresQueries.GetUserProfile(ctx, userId.UuidUserId)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusNotFound, "not_found", err.Error())
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	// decrypt username
	decodeUsername, err := DecryptUsername(profile.B64CipherChacha20poly1305Username, ps.PersonalUsernameKey)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "personal GetProfile failed")
	}

	finalAvatarUrl, err := ps.GetRefreshedAvatarURL(ctx, userId.UuidUserId, profile.FileID, profile.TokenID, profile.TokenSecret, profile.TokenExpiry)
	if err != nil {
		return nil, err
	}

	return toPrivateUserWithAvatar(&profile, decodeUsername, email, finalAvatarUrl), nil
}

func (ps *profileService) UpdateUserProfile(ctx context.Context, payload *updateUserProfilePayload, userId kit.UserId) (*kit.StatusOkay, error) {
	_, err := ps.PostgresQueries.UpdateUserProfile(ctx, personal_profile_store.UpdateUserProfileParams{
		ID:          userId.UuidUserId,
		Name:        payload.Name,
		Bio:         payload.Bio,
		ProfileType: payload.ProfileType,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to update user profile: "+kit.GetPostgresError(err).Message)
	}

	return &kit.StatusOkay{Status: true, Message: "Profile updated successfully"}, nil
}

func (ps *profileService) UploadUserProfilePicture(ctx context.Context, fh *multipart.FileHeader, userId kit.UserId) (*kit.StatusOkay, error) {
	if fh == nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "no file provided")
	}
	// check if user profile pic exists and if it exists, delete it
	resUser, err := ps.PostgresQueries.IsUserProfilePicExists(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to check user profile pic: "+kit.GetPostgresError(err).Message)
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
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to list files: "+err.Error())
	}

	var deleteExisting bool
	if checkExistInStorage.Total == 1 {
		deleteExisting = true
	}

	result, err := services.UploadFileFromMultipart(
		ps.AppwriteStorage,
		ps.PersonalProfilePicBucketID,
		userId.StringUserId,
		fh,
		services.UploadOptions{DeleteExisting: deleteExisting, GenerateTokens: true},
	)
	if err != nil {
		return nil, err
	}

	if len(result.TokenIDs) == 0 || len(result.TokenSecrets) == 0 {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "missing file access tokens")
	}

	expireTime, err := time.Parse(time.RFC3339, result.Expire)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to parse expire time")
	}

	if !resUser {
		rdmUUID, err := uuid.NewV7()
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate uuid")
		}
		_, err = ps.PostgresQueries.CreateAvatar(ctx, personal_profile_store.CreateAvatarParams{
			ID:          rdmUUID,
			UserID:      userId.UuidUserId,
			FileID:      result.FileId,
			AvatarType:  "profile",
			TokenID:     new(result.TokenIDs[0]),
			TokenSecret: new(result.TokenSecrets[0]),
			TokenExpiry: &expireTime,
		})
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to create avatar: "+kit.GetPostgresError(err).Message)
		}
	}

	if resUser {
		_, err := ps.PostgresQueries.UpdateAvatarTokens(ctx, personal_profile_store.UpdateAvatarTokensParams{
			UserID:      userId.UuidUserId,
			TokenID:     new(result.TokenIDs[0]),
			TokenSecret: new(result.TokenSecrets[0]),
			TokenExpiry: &expireTime,
		})
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to update avatar tokens: "+kit.GetPostgresError(err).Message)
		}
	}

	return &kit.StatusOkay{Status: true, Message: "Avatar uploaded successfully"}, nil
}

func (ps *profileService) RemoveUserProfilePicture(ctx context.Context, userId kit.UserId) (*kit.StatusOkay, error) {
	resUser, err := ps.PostgresQueries.IsUserProfilePicExists(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to check user profile pic: "+kit.GetPostgresError(err).Message)
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
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to list files: "+err.Error())
	}

	if checkExistInStorage.Total == 0 {
		if resUser {
			err = ps.PostgresQueries.DeleteAvatar(ctx, userId.UuidUserId)
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to delete avatar from database: "+kit.GetPostgresError(err).Message)
			}
		}
		return nil, kit.NewError(http.StatusNotFound, "not_found", "Profile picture not found")
	}

	// Delete the file access token
	tok, err := ps.AppwriteStorage.Tokens.List(ps.PersonalProfilePicBucketID, userId.StringUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to query token data: "+err.Error())
	}
	if tok.Total > 0 {
		for _, tokens := range tok.Tokens {
			_, err := ps.AppwriteStorage.Tokens.Delete(tokens.Id)
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to delete token data: "+err.Error())
			}
		}
	}

	// Delete the file from storage
	_, err = ps.AppwriteStorage.Storage.DeleteFile(
		ps.PersonalProfilePicBucketID,
		userId.StringUserId,
	)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "not_found", "Failed to delete profile picture from storage: "+err.Error())
	}

	if resUser {
		// Delete the avatar from the database
		err = ps.PostgresQueries.DeleteAvatar(ctx, userId.UuidUserId)
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to delete avatar from database: "+kit.GetPostgresError(err).Message)
		}
	}

	return &kit.StatusOkay{Status: true, Message: "Profile picture removed successfully"}, nil
}
