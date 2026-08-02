package personal_profile

import (
	"chatbasket-api/internal/modules/personal/personal_profile/internal/personal_profile_store"
	"github.com/google/uuid"
)

type UserCoreProfile struct {
	ID             uuid.UUID `json:"id"`
	IsAdminBlocked bool      `json:"is_admin_blocked"`
	ProfileType    string    `json:"profile_type"`
}

// UserBlock exposes the generated SQLC row returned by GetUserBlocks.
type UserBlock = personal_profile_store.GetUserBlocksRow

// BlockStatusResult is returned by IsBlockedBetweenUsers and indicates whether a
// block exists between two users and which conditions are true. TargetID is
// populated for batch query results.
type BlockStatusResult struct {
	IsBlocked                      bool      `json:"isBlocked"`
	IsRequesterAdminBlocked        bool      `json:"isRequesterAdminBlocked"`
	IsTargetAdminBlocked           bool      `json:"isTargetAdminBlocked"`
	IsRequesterUserBlockedByTarget bool      `json:"isRequesterUserBlockedByTarget"`
	IsTargetUserBlockedByRequester bool      `json:"isTargetUserBlockedByRequester"`
	IsTargetProfilePrivate         bool      `json:"isTargetProfilePrivate"`
	TargetID                       uuid.UUID `json:"targetId,omitempty"`
}

type AdminOrPrivateBlockStatus struct {
	IsBlocked               bool `json:"isBlocked"`
	IsRequesterAdminBlocked bool `json:"isRequesterAdminBlocked"`
	IsTargetAdminBlocked    bool `json:"isTargetAdminBlocked"`
	IsTargetProfilePrivate  bool `json:"isTargetProfilePrivate"`
}

// BlockStatusFlags mirrors the block-status queries. It is sent to the frontend inside the
// ApiError.Details field so clients can branch on the exact condition.
type BlockStatusFlags struct {
	IsRequesterAdminBlocked        bool `json:"isRequesterAdminBlocked"`
	IsTargetAdminBlocked           bool `json:"isTargetAdminBlocked"`
	IsRequesterUserBlockedByTarget bool `json:"isRequesterUserBlockedByTarget"`
	IsTargetUserBlockedByRequester bool `json:"isTargetUserBlockedByRequester"`
	IsTargetProfilePrivate         bool `json:"isTargetProfilePrivate"`
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

type updateUserProfilePayload struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=40"`
	Bio         *string `json:"bio,omitempty" validate:"omitempty,max=150"`
	ProfileType *string `json:"profile_type,omitempty" validate:"omitempty,oneof=public private personal"`
}

type uploadE2EEPublicKeyPayload struct {
	E2eePublicKey string `json:"e2ee_public_key"`
}

type getE2EEPublicKeyPayload struct {
	UserID string `query:"user_id"`
}

// ──────────────────────────────────────────────────────────────────────────────
// R2 avatar presign/confirm types (added for R2 migration)
// ──────────────────────────────────────────────────────────────────────────────

// ConfirmAvatarPayload is the request body for POST /profile/confirm-avatar.
type ConfirmAvatarPayload struct {
	FileID string `json:"file_id" validate:"required"`
}
