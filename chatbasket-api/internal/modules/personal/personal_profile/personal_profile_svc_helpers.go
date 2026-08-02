package personal_profile

import (
	"chatbasket-api/internal/modules/personal/personal_profile/internal/personal_profile_store"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/kit"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/chacha20poly1305"
)

// Character sets for username generation
const (
	letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits  = "0123456789"
)

func generateRandomUsername() (string, error) {
	username := make([]byte, 10)

	// first 4 letters
	for i := 0; i < 4; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		username[i] = letters[idx.Int64()]
	}

	// next 6 digits
	for i := 4; i < 10; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		username[i] = digits[idx.Int64()]
	}

	return string(username), nil
}

func EncryptUsername(username string, encryptionKey []byte, userIDStr string) (string, error) {
	u, err := uuid.Parse(userIDStr)
	if err != nil {
		return "", fmt.Errorf("invalid UUID string: %w", err)
	}

	if len(encryptionKey) != chacha20poly1305.KeySize {
		return "", fmt.Errorf("encryptionKey must be %d bytes, got %d", chacha20poly1305.KeySize, len(encryptionKey))
	}

	aead, err := chacha20poly1305.New(encryptionKey)
	if err != nil {
		return "", err
	}

	// Use first 12 bytes of UUID as nonce
	nonce := u[:chacha20poly1305.NonceSize]

	ciphertext := aead.Seal(nonce, nonce, []byte(username), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func DecryptUsername(encryptedB64 string, encryptionKey []byte) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encryptedB64)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	if len(encryptionKey) != chacha20poly1305.KeySize {
		return "", fmt.Errorf("encryptionKey must be %d bytes, got %d", chacha20poly1305.KeySize, len(encryptionKey))
	}

	aead, err := chacha20poly1305.New(encryptionKey)
	if err != nil {
		return "", err
	}

	nonceSize := chacha20poly1305.NonceSize
	if len(raw) < nonceSize {
		return "", fmt.Errorf("ciphertext too short: %d bytes, need at least %d", len(raw), nonceSize)
	}

	nonce := raw[:nonceSize]
	ciphertext := raw[nonceSize:]

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}
	return string(plaintext), nil
}

// ShouldExposeAvatar determines if a user's avatar should be visible based on global and user-level privacy restrictions.
// Ported from contact_service.go for full fidelity and centralized privacy logic.
func ShouldExposeAvatar(globalRestrictProfile, exceptionGlobalProfile, globalRestrictAvatar, exceptionGlobalAvatar, userRestrictProfile, userRestrictAvatar bool) bool {
	if globalRestrictProfile {
		return exceptionGlobalProfile
	}
	if globalRestrictAvatar {
		return exceptionGlobalAvatar
	}
	if userRestrictProfile {
		return false
	}
	if userRestrictAvatar {
		return false
	}
	return true
}

// GetAvatarURL generates a 15-minute R2 presigned GET URL for the avatar file.
// Returns nil if fileID is nil/empty. Routes to the correct R2 account via prefix parsing.
func (ps *profileService) GetAvatarURL(ctx context.Context, fileID *string) (*string, error) {
	if fileID == nil || *fileID == "" {
		return nil, nil
	}
	client := ps.R2Pool.GetClient(*fileID)
	_, objectKey := clients.ParseFilePrefix(*fileID)
	url, err := client.GenerateDownloadURL(ctx, client.ProfileBucket(), objectKey, r2PresignedURLLifetime)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to generate avatar URL: "+err.Error())
	}
	return &url, nil
}

// FindContactableUserByUsername looks up a user by username for contact operations
func (ps *profileService) FindContactableUserByUsername(
	ctx context.Context,
	viewerID uuid.UUID,
	username string,
) (*ContactLookupResult, error) {
	// Hash username
	hashedUsername, err := kit.ComputeHMAC(username, ps.PersonalUsernameKey, false, nil)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to hash username")
	}

	// Query user
	user, err := ps.PostgresQueries.GetUserByHashedUsernameForContact(ctx, hashedUsername)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &ContactLookupResult{Exists: false}, nil
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	// Exclude self
	if user.ID == viewerID {
		return &ContactLookupResult{Exists: false}, nil
	}

	return &ContactLookupResult{
		ID:          user.ID,
		Name:        user.Name,
		ProfileType: user.ProfileType,
		Exists:      true,
	}, nil
}

// IsUserAdminBlocked checks if a user is admin blocked
func (ps *profileService) IsUserAdminBlocked(ctx context.Context, userID uuid.UUID) (bool, error) {
	return ps.PostgresQueries.IsUserAdminBlocked(ctx, userID)
}

// IsBlockedBetweenUsers checks if either user is admin-blocked or if either user has blocked the other.
// Returns a BlockStatusResult with individual boolean flags for every condition.
func (ps *profileService) IsBlockedBetweenUsers(ctx context.Context, requesterID uuid.UUID, targetID uuid.UUID) (*BlockStatusResult, error) {
	row, err := ps.PostgresQueries.IsBlockedBetweenUsers(ctx, personal_profile_store.IsBlockedBetweenUsersParams{
		RequesterUserID: requesterID,
		TargetUserID:    targetID,
	})
	if err != nil {
		return nil, err
	}
	return blockStatusFromRow(row.RequesterAdminBlocked, row.TargetAdminBlocked, row.RequesterUserBlockedByTarget, row.TargetUserBlockedByRequester, row.IsTargetProfilePrivate, uuid.Nil), nil
}

func (ps *profileService) IsBlockedByAdminOrPrivate(ctx context.Context, requesterID, targetID uuid.UUID) (*AdminOrPrivateBlockStatus, error) {
	row, err := ps.PostgresQueries.IsBlockedByAdminOrPrivate(ctx, personal_profile_store.IsBlockedByAdminOrPrivateParams{
		RequesterUserID: requesterID,
		TargetUserID:    targetID,
	})
	if err != nil {
		return nil, err
	}

	isBlocked := row.RequesterAdminBlocked || row.TargetAdminBlocked || row.IsTargetProfilePrivate

	return &AdminOrPrivateBlockStatus{
		IsBlocked:               isBlocked,
		IsRequesterAdminBlocked: row.RequesterAdminBlocked,
		IsTargetAdminBlocked:    row.TargetAdminBlocked,
		IsTargetProfilePrivate:  row.IsTargetProfilePrivate,
	}, nil
}

// IsBlockedBetweenUsersBatch checks a requester against multiple target users in one query.
// Returns a BlockStatusResult per target user, preserving the input order.
func (ps *profileService) IsBlockedBetweenUsersBatch(ctx context.Context, requesterID uuid.UUID, targetIDs []uuid.UUID) ([]*BlockStatusResult, error) {
	rows, err := ps.PostgresQueries.IsBlockedBetweenUsersBatch(ctx, personal_profile_store.IsBlockedBetweenUsersBatchParams{
		RequesterUserID: requesterID,
		TargetUserIds:   targetIDs,
	})
	if err != nil {
		return nil, err
	}
	results := make([]*BlockStatusResult, len(rows))
	for i, row := range rows {
		results[i] = blockStatusFromRow(row.RequesterAdminBlocked, row.TargetAdminBlocked, row.RequesterUserBlockedByTarget, row.TargetUserBlockedByRequester, row.IsTargetProfilePrivate, row.TargetID)
	}
	return results, nil
}

func blockStatusFromRow(requesterAdmin, targetAdmin, requesterBlockedByTarget, targetBlockedByRequester bool, targetProfilePrivate bool, targetID uuid.UUID) *BlockStatusResult {
	return &BlockStatusResult{
		IsBlocked:                       requesterAdmin || targetAdmin || requesterBlockedByTarget || targetBlockedByRequester || targetProfilePrivate,
		IsRequesterAdminBlocked:         requesterAdmin,
		IsTargetAdminBlocked:            targetAdmin,
		IsRequesterUserBlockedByTarget:  requesterBlockedByTarget,
		IsTargetUserBlockedByRequester:  targetBlockedByRequester,
		IsTargetProfilePrivate:         targetProfilePrivate,
		TargetID:                        targetID,
	}
}

// GetUserCoreProfile fetches core profile information
func (ps *profileService) GetUserCoreProfile(ctx context.Context, userID uuid.UUID) (*UserCoreProfile, error) {
	u, err := ps.PostgresQueries.GetUserCoreProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &UserCoreProfile{
		ID:             u.ID,
		IsAdminBlocked: u.IsAdminBlocked,
		ProfileType:    u.ProfileType,
	}, nil
}

// GetContactableProfilesForViewer fetches profiles for contact enrichment with
// privacy filtering. The returned map only contains users that pass every
// privacy-exclusion check (not admin-blocked, public/personal profile type,
// and not blocked in either direction). Users that fail any check are omitted
// from the map entirely — the wire payload the
// caller builds for them will have name/username/profile_type as `""` and
// bio/avatar_url/avatar_file_id as `null`.
//
// This binary per-user exclusion is the wire contract. The frontend
// ($userProfilesState.upsertFromServer) interprets the empty values
// differently per field:
//   - name/username `""` is "no data" — frontend preserves the owner's
//     prior-known identity so the chat list stays identifiable across
//     multiple excluded chats.
//   - profileType `""` is authoritative clear — frontend would otherwise
//     render a misleading "Public" badge on the user profile screen.
//   - bio/avatar_url/avatar_file_id `null` is authoritative clear — the
//     PrivacyAvatar resolver nulls the trio so a stale cached photo of
//     a privacy-excluded user can't leak.
//
// Avatar visibility within the returned rows is gated further by
// ShouldExposeAvatar (per-user and global restrictions) — the row may
// be returned with name/username but no avatar URL/file_id.
func (ps *profileService) GetContactableProfilesForViewer(
	ctx context.Context,
	viewerID uuid.UUID,
	targetIDs []uuid.UUID,
) (map[uuid.UUID]*ContactProfileView, error) {
	if len(targetIDs) == 0 {
		return map[uuid.UUID]*ContactProfileView{}, nil
	}

	rows, err := ps.PostgresQueries.GetContactableProfilesForViewer(ctx, personal_profile_store.GetContactableProfilesForViewerParams{
		ViewerUserID:  viewerID,
		TargetUserIds: targetIDs,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to fetch contactable profiles: "+kit.GetPostgresError(err).Message)
	}

	profileUserIDs := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		profileUserIDs[i] = r.ID
	}

	revisionsMap, err := ps.AuthProvider.GetKeysRevisions(ctx, profileUserIDs)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to fetch keys revisions from auth provider: "+err.Error())
	}

	result := make(map[uuid.UUID]*ContactProfileView, len(rows))
	for _, row := range rows {
		// Decrypt username
		username, err := DecryptUsername(row.Username, ps.PersonalUsernameKey)
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to decrypt username")
		}

		// Check avatar visibility
		var avatarURL *string
		var avatarFileID *string
		if ShouldExposeAvatar(
			row.GlobalRestrictProfile,
			row.ExceptionGlobalProfile,
			row.GlobalRestrictAvatar,
			row.ExceptionGlobalAvatar,
			row.UserRestrictProfile,
			row.UserRestrictAvatar,
		) {
			url, err := ps.GetAvatarURL(ctx, row.FileID)
			if err != nil {
				return nil, err
			}
			avatarURL = url
			avatarFileID = row.FileID
		}

		result[row.ID] = &ContactProfileView{
			ID:           row.ID,
			Name:         row.Name,
			Username:     username,
			Bio:          row.Bio,
			AvatarURL:    avatarURL,
			AvatarFileId: avatarFileID,
			KeysRevision: revisionsMap[row.ID],
			ProfileType:  row.ProfileType,
		}
	}

	return result, nil
}

// GetBlockListProfilesForViewer fetches profiles eligible for the block list.
// Admin-blocked and private targets are excluded; a target-side block keeps the
// identity fields but hides bio and avatar fields.
func (ps *profileService) GetBlockListProfilesForViewer(
	ctx context.Context,
	viewerID uuid.UUID,
	targetIDs []uuid.UUID,
) (map[uuid.UUID]*ContactProfileView, error) {
	if len(targetIDs) == 0 {
		return map[uuid.UUID]*ContactProfileView{}, nil
	}

	rows, err := ps.PostgresQueries.GetBlockListProfilesForViewer(ctx, personal_profile_store.GetBlockListProfilesForViewerParams{
		ViewerUserID:  viewerID,
		TargetUserIds: targetIDs,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to fetch block-list profiles: "+kit.GetPostgresError(err).Message)
	}

	result := make(map[uuid.UUID]*ContactProfileView, len(rows))
	for _, row := range rows {
		username, err := DecryptUsername(row.Username, ps.PersonalUsernameKey)
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to decrypt username")
		}

		var avatarURL *string
		var avatarFileID *string
		if !row.TargetBlockedViewer && ShouldExposeAvatar(
			row.GlobalRestrictProfile,
			row.ExceptionGlobalProfile,
			row.GlobalRestrictAvatar,
			row.ExceptionGlobalAvatar,
			row.UserRestrictProfile,
			row.UserRestrictAvatar,
		) {
			url, err := ps.GetAvatarURL(ctx, row.FileID)
			if err != nil {
				return nil, err
			}
			avatarURL = url
			avatarFileID = row.FileID
		}

		bio := row.Bio
		if row.TargetBlockedViewer {
			bio = nil
		}

		result[row.ID] = &ContactProfileView{
			ID:           row.ID,
			Name:         row.Name,
			Username:     username,
			Bio:          bio,
			AvatarURL:    avatarURL,
			AvatarFileId: avatarFileID,
			ProfileType:  row.ProfileType,
		}
	}

	return result, nil
}

// GetContactableUserIDs checks which target user IDs are contactable for a viewer.
// It applies the same bidirectional block exclusion as
// GetContactableProfilesForViewer, but returns only IDs for chat message
// filtering. It also excludes admin-blocked users.
func (ps *profileService) GetContactableUserIDs(
	ctx context.Context,
	viewerID uuid.UUID,
	targetIDs []uuid.UUID,
) ([]uuid.UUID, error) {
	if len(targetIDs) == 0 {
		return []uuid.UUID{}, nil
	}

	ids, err := ps.PostgresQueries.GetContactableUserIDs(ctx, personal_profile_store.GetContactableUserIDsParams{
		ViewerUserID:  viewerID,
		TargetUserIds: targetIDs,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to check contactable user IDs: "+kit.GetPostgresError(err).Message)
	}
	return ids, nil
}

func (ps *profileService) GetUserBlocks(ctx context.Context, blockerID uuid.UUID) ([]UserBlock, error) {
	rows, err := ps.PostgresQueries.GetUserBlocks(ctx, blockerID)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	return rows, nil
}

func (ps *profileService) CreateUserBlock(ctx context.Context, id, blockerID, blockedID uuid.UUID) error {
	return ps.PostgresQueries.CreateUserBlock(ctx, personal_profile_store.CreateUserBlockParams{
		ID:            id,
		BlockerUserID: blockerID,
		BlockedUserID: blockedID,
	})
}

func (ps *profileService) DeleteUserBlock(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	return ps.PostgresQueries.DeleteUserBlock(ctx, personal_profile_store.DeleteUserBlockParams{
		BlockerUserID: blockerID,
		BlockedUserID: blockedID,
	})
}
