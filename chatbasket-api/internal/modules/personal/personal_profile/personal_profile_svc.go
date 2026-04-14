package personal_profile

import (
	"chatbasket-api/internal/modules/personal/personal_profile/internal/personal_profile_store"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/services"
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

	// 1. Get current avatar metadata to find the existing file_id
	existingFileID, err := ps.PostgresQueries.GetAvatarFileID(ctx, userId.UuidUserId)
	hasExisting := err == nil && existingFileID != nil

	if hasExisting {
		// 2. Safety check: List file in Appwrite to confirm existence before purging
		checkFiles, err := ps.AppwriteStorage.Storage.ListFiles(
			ps.PersonalProfilePicBucketID,
			ps.AppwriteStorage.Storage.WithListFilesQueries([]string{
				query.Equal("$id", *existingFileID),
			}),
		)
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to verify existing avatar in storage: "+err.Error())
		}

		if checkFiles.Total > 0 {
			// Clear tokens
			toks, err := ps.AppwriteStorage.Tokens.List(ps.PersonalProfilePicBucketID, *existingFileID)
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to list tokens for old avatar: "+err.Error())
			}
			if toks.Total > 0 {
				for _, t := range toks.Tokens {
					_, _ = ps.AppwriteStorage.Tokens.Delete(t.Id)
				}
			}

			// Delete file
			_, _ = ps.AppwriteStorage.Storage.DeleteFile(ps.PersonalProfilePicBucketID, *existingFileID)
		}
	}

	// 3. Generate a NEW unique File ID (using UUID v7 for sequential/sorting benefits)
	newFileId, err := uuid.NewV7()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to generate unique file id")
	}
	stringFileId := newFileId.String()

	// 4. Upload fresh file
	result, err := services.UploadFileFromMultipart(
		ps.AppwriteStorage,
		ps.PersonalProfilePicBucketID,
		stringFileId,
		fh,
		services.UploadOptions{DeleteExisting: false, GenerateTokens: true}, // Manual deletion handled above
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

	if !hasExisting {
		rdmUUID, err := uuid.NewV7()
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate uuid")
		}
		_, err = ps.PostgresQueries.CreateAvatar(ctx, personal_profile_store.CreateAvatarParams{
			ID:          rdmUUID,
			UserID:      userId.UuidUserId,
			FileID:      &stringFileId,
			AvatarType:  "profile",
			TokenID:     new(result.TokenIDs[0]),
			TokenSecret: new(result.TokenSecrets[0]),
			TokenExpiry: &expireTime,
		})
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to create avatar: "+kit.GetPostgresError(err).Message)
		}
	} else {
		_, err := ps.PostgresQueries.UpdateAvatarFull(ctx, personal_profile_store.UpdateAvatarFullParams{
			UserID:      userId.UuidUserId,
			FileID:      &stringFileId,
			TokenID:     new(result.TokenIDs[0]),
			TokenSecret: new(result.TokenSecrets[0]),
			TokenExpiry: &expireTime,
		})
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to update avatar record: "+kit.GetPostgresError(err).Message)
		}
	}

	return &kit.StatusOkay{Status: true, Message: "Avatar uploaded successfully"}, nil
}

func (ps *profileService) RemoveUserProfilePicture(ctx context.Context, userId kit.UserId) (*kit.StatusOkay, error) {
	// 1. Get the current avatar to find the stored file_id
	fileIDPtr, err := ps.PostgresQueries.GetAvatarFileID(ctx, userId.UuidUserId)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusNotFound, "not_found", "Profile picture record not found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to fetch avatar record: "+kit.GetPostgresError(err).Message)
	}

	if fileIDPtr == nil {
		return nil, kit.NewError(http.StatusNotFound, "not_found", "Profile picture file ID not found")
	}

	fileID := *fileIDPtr

	// 2. Safety check: List file in Appwrite
	checkFiles, err := ps.AppwriteStorage.Storage.ListFiles(
		ps.PersonalProfilePicBucketID,
		ps.AppwriteStorage.Storage.WithListFilesQueries([]string{
			query.Equal("$id", fileID),
		}),
	)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to verify avatar existence: "+err.Error())
	}

	if checkFiles.Total > 0 {
		// 3. Delete access tokens from Appwrite
		tok, err := ps.AppwriteStorage.Tokens.List(ps.PersonalProfilePicBucketID, fileID)
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to list tokens for avatar removal: "+err.Error())
		}

		if tok.Total > 0 {
			for _, tokens := range tok.Tokens {
				_, _ = ps.AppwriteStorage.Tokens.Delete(tokens.Id)
			}
		}

		// 4. Delete the file from Appwrite storage
		_, _ = ps.AppwriteStorage.Storage.DeleteFile(ps.PersonalProfilePicBucketID, fileID)
	}

	// 5. Delete the avatar record from the database
	err = ps.PostgresQueries.DeleteAvatar(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to delete avatar from database: "+kit.GetPostgresError(err).Message)
	}

	return &kit.StatusOkay{Status: true, Message: "Profile picture removed successfully"}, nil
}
