package personal_profile

import (
	"chatbasket-api/internal/modules/personal/personal_profile/internal/personal_profile_store"
	"time"

	"github.com/google/uuid"
)

type UserCoreProfile struct {
	ID             uuid.UUID `json:"id"`
	IsAdminBlocked bool      `json:"is_admin_blocked"`
	ProfileType    string    `json:"profile_type"`
}

// BlockStatusResult is returned by IsBlockedBetweenUsers and indicates whether a
// block exists between two users and which conditions are true. TargetID is
// populated for batch query results.
type BlockStatusResult struct {
	IsBlocked                      bool      `json:"is_blocked"`
	IsRequesterAdminBlocked        bool      `json:"is_requester_admin_blocked"`
	IsTargetAdminBlocked           bool      `json:"is_target_admin_blocked"`
	IsRequesterUserBlockedByTarget bool      `json:"is_requester_user_blocked_by_target"`
	IsTargetUserBlockedByRequester bool      `json:"is_target_user_blocked_by_requester"`
	IsTargetProfilePrivate         bool      `json:"is_target_profile_private"`
	TargetID                       uuid.UUID `json:"target_id,omitempty"`
}

// BlockStatusFlags mirrors the block-status queries. It is sent to the frontend inside the
// ApiError.Details field so clients can branch on the exact condition.
type BlockStatusFlags struct {
	IsRequesterAdminBlocked        bool `json:"is_requester_admin_blocked"`
	IsTargetAdminBlocked           bool `json:"is_target_admin_blocked"`
	IsRequesterUserBlockedByTarget bool `json:"is_requester_user_blocked_by_target"`
	IsTargetUserBlockedByRequester bool `json:"is_target_user_blocked_by_requester"`
	IsTargetProfilePrivate         bool `json:"is_target_profile_private"`
}

type ContactProfileView struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Username      string    `json:"username"`
	Bio           *string   `json:"bio"`
	AvatarURL     *string   `json:"avatar_url"`
	AvatarFileId  *string   `json:"avatar_file_id"`
	KeysRevision  int32     `json:"keys_revision"`
	ProfileType   string    `json:"profile_type"`
}

type ContactLookupResult struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	ProfileType string    `json:"profile_type"`
	Exists      bool      `json:"exists"`
}

type createUserProfilePayload struct {
	Name        string `json:"name" validate:"required,min=1,max=40"`
	ProfileType string `json:"profile_type" validate:"required,oneof=public private personal"`
}

type privateUser struct {
	Id           string    `json:"id"`
	Username     string    `json:"username"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	Bio          *string   `json:"bio"`
	AvatarUrl    *string   `json:"avatar_url"`
	AvatarFileId *string   `json:"avatar_file_id"`
	ProfileType  string    `json:"profile_type"` // User profile type: private/public/personal
	KeysRevision int32     `json:"keys_revision"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type updateUserProfilePayload struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=40"`
	Bio         *string `json:"bio,omitempty" validate:"omitempty,max=150"`
	ProfileType *string `json:"profile_type,omitempty" validate:"omitempty,oneof=public private personal"`
}

func toPrivateUserWithAvatar(user *personal_profile_store.GetUserProfileRow, username string, email string, avatarUrl *string, keysRevision int32) *privateUser {
	return &privateUser{
		Id:           user.ID.String(),
		Username:     username,
		Name:         user.Name,
		Email:        email,
		AvatarUrl:    avatarUrl,
		AvatarFileId: user.FileID,
		Bio:          user.Bio,
		ProfileType:  user.ProfileType,
		KeysRevision: keysRevision,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
	}
}

func toPrivateUser(user *personal_profile_store.User, username string, email string, keysRevision int32) *privateUser {
	return &privateUser{
		Id:           user.ID.String(),
		Username:     username,
		Name:         user.Name,
		Email:        email,
		Bio:          user.Bio,
		AvatarUrl:    nil,
		AvatarFileId: nil,
		KeysRevision: keysRevision,
		ProfileType:  user.ProfileType,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
	}
}

type uploadE2EEPublicKeyPayload struct {
	E2eePublicKey string `json:"e2ee_public_key"`
}

type updateE2EEKeyResponse struct {
	Status       bool   `json:"status"`
	Message      string `json:"message"`
	KeysRevision int32  `json:"keys_revision"`
}

type getE2EEPublicKeyPayload struct {
	UserID string `query:"user_id"`
}

type getE2EEPublicKeyResponse struct {
	E2eePublicKeys []string `json:"e2ee_public_keys"`
	KeysRevision   int32    `json:"keys_revision"`
}

// ──────────────────────────────────────────────────────────────────────────────
// R2 avatar presign/confirm types (added for R2 migration)
// ──────────────────────────────────────────────────────────────────────────────

// PresignAvatarResponse is returned by POST /profile/presign-avatar.
type PresignAvatarResponse struct {
	FileID       string `json:"file_id"`
	PresignedURL string `json:"presigned_url"`
}

// ConfirmAvatarPayload is the request body for POST /profile/confirm-avatar.
type ConfirmAvatarPayload struct {
	FileID string `json:"file_id" validate:"required"`
}
