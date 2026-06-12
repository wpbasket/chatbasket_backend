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

type ContactProfileView struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Username      string    `json:"username"`
	Bio           *string   `json:"bio"`
	AvatarURL     *string   `json:"avatar_url"`
	AvatarFileId  *string   `json:"avatar_file_id"`
	E2eePublicKey *string   `json:"e2ee_public_key"`
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
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type updateUserProfilePayload struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=40"`
	Bio         *string `json:"bio,omitempty" validate:"omitempty,max=150"`
	ProfileType *string `json:"profile_type,omitempty" validate:"omitempty,oneof=public private personal"`
}

func toPrivateUserWithAvatar(user *personal_profile_store.GetUserProfileRow, username string, email string, avatarUrl *string) *privateUser {
	return &privateUser{
		Id:           user.ID.String(),
		Username:     username,
		Name:         user.Name,
		Email:        email,
		AvatarUrl:    avatarUrl,
		AvatarFileId: user.FileID,
		Bio:          user.Bio,
		ProfileType:  user.ProfileType,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
	}
}

func toPrivateUser(user *personal_profile_store.User, username string, email string) *privateUser {
	return &privateUser{
		Id:           user.ID.String(),
		Username:     username,
		Name:         user.Name,
		Email:        email,
		Bio:          user.Bio,
		AvatarUrl:    nil,
		AvatarFileId: nil,
		ProfileType:  user.ProfileType,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
	}
}

type uploadE2EEPublicKeyPayload struct {
	E2eePublicKey string `json:"e2ee_public_key"`
}

type getE2EEPublicKeyPayload struct {
	UserID string `query:"user_id"`
}

type getE2EEPublicKeyResponse struct {
	E2eePublicKey *string `json:"e2ee_public_key"`
}

