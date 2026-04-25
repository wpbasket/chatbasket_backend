package personal_profile

import (
	"chatbasket-api/internal/modules/personal/personal_profile/internal/personal_profile_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"time"

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

// GetRefreshedAvatarURL refreshes avatar tokens if needed and returns the avatar URL
func (ps *profileService) GetRefreshedAvatarURL(ctx context.Context, userID uuid.UUID, fileID, tokenID, tokenSecret *string, tokenExpiry *time.Time) (*string, error) {
	effectiveExpiry := time.Time{}
	if tokenExpiry != nil {
		effectiveExpiry = *tokenExpiry
	}

	refreshed, needsUpdate, err := kit.EnsureFreshFileTokens(
		fileID,
		tokenID,
		tokenSecret,
		effectiveExpiry,
		ps.AppwriteStorage.Tokens,
		ps.PersonalProfilePicBucketID,
	)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to refresh avatar tokens: "+err.Error())
	}

	if needsUpdate && refreshed != nil {
		_, err := ps.PostgresQueries.UpdateAvatarTokens(ctx, personal_profile_store.UpdateAvatarTokensParams{
			UserID:      userID,
			TokenID:     &refreshed.TokenID,
			TokenSecret: &refreshed.TokenSecret,
			TokenExpiry: &refreshed.TokenExpiry,
		})
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to update refreshed tokens in DB: "+kit.GetPostgresError(err).Message)
		}

		return kit.BuildFileDownloadURL(ps.AppwriteStorage.Endpoint, ps.AppwriteStorage.Project, ps.PersonalProfilePicBucketID, &kit.AppwriteFileData{
			FileId:     fileID,
			FileToken:  &refreshed.TokenID,
			FileSecret: &refreshed.TokenSecret,
		}), nil
	}

	return kit.BuildFileDownloadURL(ps.AppwriteStorage.Endpoint, ps.AppwriteStorage.Project, ps.PersonalProfilePicBucketID, &kit.AppwriteFileData{
		FileId:     fileID,
		FileToken:  tokenID,
		FileSecret: tokenSecret,
	}), nil
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

// GetContactableProfilesForViewer fetches profiles for contact enrichment with privacy filtering,
// excluding users who have switched to a private profile type.
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
			url, err := ps.GetRefreshedAvatarURL(ctx, row.ID, row.FileID, row.TokenID, row.TokenSecret, row.TokenExpiry)
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
		}
	}

	return result, nil
}
