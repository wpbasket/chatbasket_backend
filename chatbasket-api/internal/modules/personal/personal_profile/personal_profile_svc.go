package personal_profile

import (
	"chatbasket-api/internal/modules/core/pending_uploads"
	"chatbasket-api/internal/modules/personal/personal_profile/internal/personal_profile_store"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/services"
	"context"
	"log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
)

type coreAuthProfileProvider interface {
	SaveSessionE2EEPublicKey(ctx context.Context, tx pgx.Tx, userID uuid.UUID, sessionID uuid.UUID, publicKey string) error
	GetActiveSessionKeysForUser(ctx context.Context, userID uuid.UUID) ([]string, error)
	GetActiveSessionKeysForUserExcluding(ctx context.Context, userID uuid.UUID, excludeSessionID uuid.UUID) ([]string, error)
	GetKeysRevision(ctx context.Context, userID uuid.UUID) (int32, error)
	IncrementKeysRevision(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error
}

// pendingUploadsProfileProvider defines the minimal set of methods required from
// the pending_uploads module. *pending_uploads.Service satisfies this interface.
// Tx-aware variants accept pgx.Tx for cross-module transactions (same pattern as
// core_auth.SaveSessionE2EEPublicKey).
type pendingUploadsProfileProvider interface {
	Register(ctx context.Context, fileID, bucket, r2Key string, expiresAt time.Time) error
	Lookup(ctx context.Context, fileID string) (pending_uploads.PendingUpload, error)
	Remove(ctx context.Context, fileID string) error
	LookupTx(ctx context.Context, tx pgx.Tx, fileID string) (pending_uploads.PendingUpload, error)
	RemoveTx(ctx context.Context, tx pgx.Tx, fileID string) error
	RegisterTx(ctx context.Context, tx pgx.Tx, fileID, bucket, r2Key string, expiresAt time.Time) error
}

type profileService struct {
	GlobalService       *services.GlobalService
	PostgresQuerier     personal_profile_store.Querier
	PostgresQueries     *personal_profile_store.Queries
	Pool                *pgxpool.Pool
	AuthProvider        coreAuthProfileProvider
	PersonalUsernameKey []byte
	R2Pool              *clients.R2ClientPool
	PendingUploads      pendingUploadsProfileProvider
}

func NewProfileService(globalService *services.GlobalService, pool *pgxpool.Pool, authProvider coreAuthProfileProvider, personalUsernameKey []byte, pendingUploads pendingUploadsProfileProvider, r2Pool *clients.R2ClientPool) *profileService {
	store := personal_profile_store.New(pool)
	return &profileService{
		GlobalService:       globalService,
		PostgresQuerier:     store,
		PostgresQueries:     store,
		Pool:                pool,
		AuthProvider:        authProvider,
		PersonalUsernameKey: personalUsernameKey,
		R2Pool:              r2Pool,
		PendingUploads:      pendingUploads,
	}
}

func (ps *profileService) CreateUserProfile(ctx context.Context, payload *createUserProfilePayload, userId *kit.UserId, email string) (*privateUser, error) {
	if payload == nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}
	if userId == nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid user id")
	}
	res, err := ps.PostgresQueries.IsUserExists(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	if res {
		return nil, kit.NewError(http.StatusConflict, "conflict", "User profile already exists")
	}
	generatedUsername, err := generateRandomUsername()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Username generation failed")
	}
	sha256Username, err := kit.ComputeHMAC(generatedUsername, ps.PersonalUsernameKey, false, nil)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Username hashing failed")
	}
	b64CipherChacha20Poly1305Username, err := EncryptUsername(generatedUsername, ps.PersonalUsernameKey, userId.StringUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Username encryption failed")
	}
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
	return toPrivateUser(&responseUser, generatedUsername, email, 0), nil
}

func (ps *profileService) GetProfile(ctx context.Context, userId *kit.UserId, email string) (*privateUser, error) {
	profile, err := ps.PostgresQueries.GetUserProfile(ctx, userId.UuidUserId)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusNotFound, "not_found", err.Error())
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	decodeUsername, err := DecryptUsername(profile.B64CipherChacha20poly1305Username, ps.PersonalUsernameKey)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "personal GetProfile failed")
	}
	finalAvatarUrl, err := ps.GetAvatarURL(ctx, profile.FileID)
	if err != nil {
		return nil, err
	}
	keysRevision, err := ps.AuthProvider.GetKeysRevision(ctx, userId.UuidUserId)
	if err != nil {
		if err == pgx.ErrNoRows {
			keysRevision = 0
		} else {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to read keys revision: "+kit.GetPostgresError(err).Message)
		}
	}
	return toPrivateUserWithAvatar(&profile, decodeUsername, email, finalAvatarUrl, keysRevision), nil
}

func (ps *profileService) UpdateUserProfile(ctx context.Context, payload *updateUserProfilePayload, userId kit.UserId) (*kit.StatusOkay, error) {
	err := ps.PostgresQueries.UpdateUserProfile(ctx, personal_profile_store.UpdateUserProfileParams{
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

// PresignAvatarUpload selects the next R2 account via round-robin, generates a
// unique prefixed file ID, registers the upload in pending_uploads with a
// 2-hour TTL, and returns a presigned R2 PUT URL.
func (ps *profileService) PresignAvatarUpload(ctx context.Context, userId kit.UserId) (*PresignAvatarResponse, error) {
	accountName := ps.R2Pool.NextProfileAccount()
	client := ps.R2Pool.GetClientByAccount(accountName)
	objectID := uuid.New().String()
	fileID := clients.BuildFileID(accountName, objectID)
	expiresAt := time.Now().UTC().Add(pendingAvatarUploadTTL)
	bucket := client.ProfileBucket()
	r2Key := objectID
	if err := ps.PendingUploads.Register(ctx, fileID, bucket, r2Key, expiresAt); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "pending_upload_failed", "Failed to register pending avatar upload: "+err.Error())
	}
	presignedURL, err := client.GenerateUploadURL(ctx, bucket, r2Key, r2PresignedURLLifetime)
	if err != nil {
		_ = ps.PendingUploads.Remove(ctx, fileID)
		return nil, kit.NewError(http.StatusInternalServerError, "presign_failed", "Failed to generate presigned URL: "+err.Error())
	}
	return &PresignAvatarResponse{
		FileID:       fileID,
		PresignedURL: presignedURL,
	}, nil
}

// ConfirmAvatarUpload verifies the pending avatar upload and updates the user's
// avatar record with the new file_id — all in a single transaction.
//
// Unified cleanup pattern via pending_uploads:
//   1. In tx: verify pending upload exists, capture old file_id
//   2. Register the old file_id in pending_uploads with an expired status (for sweeper backup)
//   3. Delete the old avatar row and insert the new avatar row (enforcing unique active constraint)
//   4. Delete the new avatar file_id from pending_uploads and commit
//   5. Post-commit (best-effort, inline): try R2 delete of the old avatar file
//   6. On success: remove the old avatar file_id from pending_uploads
//   7. If inline R2 delete fails: background sweeper (CleanupExpiredPendingUploads) retries R2 delete + row removal.
//
// Why this pattern:
//   - Fast path: immediate inline cleanup without blocking the DB transaction.
//   - Recovery path: no orphans created if R2 is down; background sweeper automatically cleans up later.
func (ps *profileService) ConfirmAvatarUpload(ctx context.Context, userId kit.UserId, fileID string) (*kit.StatusOkay, error) {
	// 1. Tx: verify pending, register old avatar in pending_uploads, delete old avatar, insert new avatar, delete new avatar from pending_uploads
	tx, err := ps.Pool.Begin(ctx)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to start confirm transaction")
	}
	defer tx.Rollback(ctx)
	qtx := ps.PostgresQueries.WithTx(tx)

	if _, err := ps.PendingUploads.LookupTx(ctx, tx, fileID); err != nil {
		return nil, kit.NewError(http.StatusNotFound, "pending_upload_not_found", "No pending avatar upload found. Please restart the upload process.")
	}

	// Fetch old active avatar within the transaction
	oldAvatar, err := qtx.GetActiveAvatar(ctx, userId.UuidUserId)
	hasExisting := err == nil && oldAvatar.FileID != nil

	var oldFileID string
	if hasExisting {
		oldFileID = *oldAvatar.FileID
	}

	// If old avatar exists and is different from the new one, register it in pending_uploads and delete its row
	if hasExisting && oldFileID != fileID {
		_, objectKey := clients.ParseFilePrefix(oldFileID)
		bucket := ps.R2Pool.GetClient(oldFileID).ProfileBucket()
		if err := ps.PendingUploads.RegisterTx(ctx, tx, oldFileID, bucket, objectKey, time.Now().UTC()); err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to register old avatar for deletion: "+err.Error())
		}
		if err := qtx.DeleteAvatar(ctx, userId.UuidUserId); err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to delete old avatar: "+kit.GetPostgresError(err).Message)
		}
	}

	// Insert new normal avatar row
	rdmUUID, err := uuid.NewV7()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate uuid")
	}
	if _, err := qtx.CreateAvatar(ctx, personal_profile_store.CreateAvatarParams{
		ID:          rdmUUID,
		UserID:      userId.UuidUserId,
		FileID:      &fileID,
		AvatarType:  "profile",
		TokenID:     nil,
		TokenSecret: nil,
		TokenExpiry: nil,
	}); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to create avatar: "+kit.GetPostgresError(err).Message)
	}

	if err := ps.PendingUploads.RemoveTx(ctx, tx, fileID); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "pending_remove_failed", "Failed to remove pending upload: "+err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to commit confirm transaction: "+err.Error())
	}

	// 2. Post-commit: try inline R2 delete of OLD avatar (best-effort).
	if hasExisting && oldFileID != fileID {
		if r2Err := ps.deleteAvatarFromR2(ctx, oldFileID); r2Err == nil {
			if err := ps.PendingUploads.Remove(ctx, oldFileID); err != nil {
				log.Printf("[ConfirmAvatarUpload] WARNING: Failed to remove old avatar %s from pending_uploads: %v", oldFileID, err)
			} else {
				log.Printf("[ConfirmAvatarUpload] Inline R2 delete + pending_uploads removal succeeded for old avatar %s", oldFileID)
			}
		} else {
			log.Printf("[ConfirmAvatarUpload] WARNING: Inline R2 delete failed for old avatar %s: %v (sweeper will retry)", oldFileID, r2Err)
		}
	}

	return &kit.StatusOkay{Status: true, Message: "Avatar uploaded successfully"}, nil
}

// RemoveUserProfilePicture removes the user's avatar.
//
// New unified pattern:
//   1. In tx: fetch avatar file_id, register it in pending_uploads, delete avatar row.
//   2. Post-commit: try inline R2 delete of the file (best-effort).
//   3. On success: remove from pending_uploads.
//   4. Background sweeper (CleanupExpiredPendingUploads) will clean it up if R2 fails.
func (ps *profileService) RemoveUserProfilePicture(ctx context.Context, userId kit.UserId) (*kit.StatusOkay, error) {
	// 1. Tx: fetch file_id, register in pending_uploads, delete row
	tx, err := ps.Pool.Begin(ctx)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to start remove transaction")
	}
	defer tx.Rollback(ctx)
	qtx := ps.PostgresQueries.WithTx(tx)

	fileIDPtr, err := qtx.GetAvatarFileID(ctx, userId.UuidUserId)
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

	_, objectKey := clients.ParseFilePrefix(fileID)
	bucket := ps.R2Pool.GetClient(fileID).ProfileBucket()
	if err := ps.PendingUploads.RegisterTx(ctx, tx, fileID, bucket, objectKey, time.Now().UTC()); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to register avatar for deletion: "+err.Error())
	}

	if err := qtx.DeleteAvatar(ctx, userId.UuidUserId); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to delete avatar: "+kit.GetPostgresError(err).Message)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to commit remove transaction: "+err.Error())
	}

	// 2. Post-commit: try inline R2 delete (best-effort)
	if r2Err := ps.deleteAvatarFromR2(ctx, fileID); r2Err == nil {
		if err := ps.PendingUploads.Remove(ctx, fileID); err != nil {
			log.Printf("[RemoveUserProfilePicture] WARNING: Failed to remove avatar %s from pending_uploads: %v", fileID, err)
		} else {
			log.Printf("[RemoveUserProfilePicture] Inline R2 delete + pending_uploads removal succeeded for avatar %s", fileID)
		}
	} else {
		log.Printf("[RemoveUserProfilePicture] WARNING: Inline R2 delete failed for avatar %s: %v (sweeper will retry)", fileID, r2Err)
	}

	return &kit.StatusOkay{Status: true, Message: "Avatar removed successfully"}, nil
}


func (ps *profileService) SaveE2EEPublicKey(ctx context.Context, userID kit.UserId, sessionID uuid.UUID, publicKey string) (*updateE2EEKeyResponse, error) {
	exists, err := ps.PostgresQueries.IsUserExists(ctx, userID.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	if !exists {
		return nil, kit.NewError(http.StatusNotFound, "not_found", "User profile does not exist. Create profile first.")
	}
	tx, err := ps.Pool.Begin(ctx)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to start key transaction: "+err.Error())
	}
	defer tx.Rollback(ctx)
	if err := ps.AuthProvider.SaveSessionE2EEPublicKey(ctx, tx, userID.UuidUserId, sessionID, publicKey); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to save E2EE public key: "+kit.GetPostgresError(err).Message)
	}
	log.Printf("[E2EE] SaveE2EEPublicKey: key saved for user %s session %s", userID.UuidUserId, sessionID)
	if err := ps.AuthProvider.IncrementKeysRevision(ctx, tx, userID.UuidUserId); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to update keys revision: "+kit.GetPostgresError(err).Message)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to commit key transaction: "+err.Error())
	}

	// Read the new revision after commit (incremented atomically in the tx above).
	revision, err := ps.AuthProvider.GetKeysRevision(ctx, userID.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to read keys revision: "+kit.GetPostgresError(err).Message)
	}
	log.Printf("[E2EE] SaveE2EEPublicKey: user %s new keys_revision=%d", userID.UuidUserId, revision)

	return &updateE2EEKeyResponse{
		Status:       true,
		Message:      "E2EE public key saved successfully",
		KeysRevision: revision,
	}, nil
}

func (ps *profileService) GetE2EEKeySet(ctx context.Context, targetUserID uuid.UUID, callerSessionID *uuid.UUID) ([]string, int32, error) {
	revision, err := ps.AuthProvider.GetKeysRevision(ctx, targetUserID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, 0, kit.NewError(http.StatusNotFound, "not_found", "User profile not found")
		}
		return nil, 0, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to fetch user keys revision: "+kit.GetPostgresError(err).Message)
	}

	var keys []string
	// When querying own keys (caller == target), exclude the caller's own session key.
	// The caller already has their own key in SecureStore — no need to return it.
	if callerSessionID != nil {
		keys, err = ps.AuthProvider.GetActiveSessionKeysForUserExcluding(ctx, targetUserID, *callerSessionID)
	} else {
		keys, err = ps.AuthProvider.GetActiveSessionKeysForUser(ctx, targetUserID)
	}
	if err != nil {
		return nil, 0, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to fetch active E2EE keys: "+kit.GetPostgresError(err).Message)
	}
	log.Printf("[E2EE] GetE2EEKeySet: target user %s → %d key(s), revision=%d (callerSessionExcluded=%v)", targetUserID, len(keys), revision, callerSessionID != nil)
	return keys, revision, nil
}

func (ps *profileService) GetE2EEPublicKey(ctx context.Context, targetUserID uuid.UUID) (*string, int32, error) {
	keys, revision, err := ps.GetE2EEKeySet(ctx, targetUserID, nil)
	if err != nil {
		return nil, 0, err
	}
	if len(keys) == 0 {
		return nil, revision, nil
	}
	return &keys[0], revision, nil
}

func (ps *profileService) GetActiveSessionKeysForUser(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return ps.AuthProvider.GetActiveSessionKeysForUser(ctx, userID)
}

// deleteAvatarFromR2 deletes an avatar file from R2 (idempotent).
// Returns nil if file was already gone (NoSuchKey) — safe to proceed with DB delete.
// Mirrors chatService.DeleteChatFile structure.
func (ps *profileService) deleteAvatarFromR2(ctx context.Context, fileID string) error {
	if fileID == "" {
		return nil
	}
	accountName, objectKey := clients.ParseFilePrefix(fileID)
	if accountName == "" {
		// Legacy unprefixed file_id
		return ps.R2Pool.PrimaryProfileClient().DeleteFile(ctx, ps.R2Pool.PrimaryProfileClient().ProfileBucket(), fileID)
	}
	if !ps.R2Pool.HasClient(accountName) {
		// Per spec §3.E: account retired, skip R2 delete (safe — DB delete will follow)
		log.Printf("[Avatar Cleanup] WARNING: Account '%s' not in pool, skipping R2 delete for stale avatar %s", accountName, fileID)
		return nil
	}
	client := ps.R2Pool.GetClient(fileID)
	return client.DeleteFile(ctx, client.ProfileBucket(), objectKey)
}

