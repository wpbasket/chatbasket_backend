package personalmodel

import (
	"chatbasket/db/postgresCode"
	// "chatbasket/utils"
	"time"
)

type User struct {
    Id                                   string   `json:"$id"`
    HmacSha256HexUsername                string   `json:"hmac_sha256_hex_username"`
    B64CipherChacha20Poly1305Username    string   `json:"b64_cipher_chacha20poly1305_username"`
    Name                                 string   `json:"name"`
    Bio                                  string   `json:"bio"`
    Avatar                               string   `json:"avatar"` // avatar id in storage
    AvatarTokens                       []string   `json:"avatar_tokens"`
    ProfileType                          string   `json:"profile_type"` // User profile type: private/public/personal
    IsAdminBlocked                       bool     `json:"is_admin_blocked"`
    AdminBlockReason                     string   `json:"admin_block_reason"`
    CreatedAt                            string   `json:"$createdAt"`
    UpdatedAt                            string   `json:"$updatedAt"`
}

type CreateUserProfilePayload struct {
    Name        string `json:"name" validate:"required,min=1,max=40"` 
    ProfileType string `json:"profile_type" validate:"required,oneof=public private personal"`
}

type LogoutPayload struct {
    AllSessions bool   `json:"all_sessions"`
}

type PrivateUser struct {
    Id                                   string      `json:"id"`
    Username                             string      `json:"username"`
    Name                                 string      `json:"name"`
    Email                                string      `json:"email"`
    Bio                                 *string      `json:"bio"`
    AvatarUrl                           *string      `json:"avatar_url"` 
    ProfileType                          string      `json:"profile_type"` // User profile type: private/public/personal
    CreatedAt                            time.Time   `json:"createdAt"`
    UpdatedAt                            time.Time   `json:"updatedAt"`    
}


type UpdateUserProfilePayload struct {
    Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=40"` 
    Bio         *string `json:"bio,omitempty" validate:"omitempty,max=150"`
    ProfileType *string `json:"profile_type,omitempty" validate:"omitempty,oneof=public private personal"`
}

















func ToPrivateUserWithAvatar(user *postgresCode.GetUserProfileRow,username string,email string,avatarUrl *string) *PrivateUser {
    return &PrivateUser{
		Id:             user.ID.String(),
		Username:       username,
		Name:           user.Name,
		Email:          email,
        AvatarUrl:      avatarUrl,
		Bio:            user.Bio,
		ProfileType:    user.ProfileType,
		CreatedAt:      user.CreatedAt.Time,
		UpdatedAt:      user.UpdatedAt.Time,
	}
}

func ToPrivateUser(user *postgresCode.User,username string,email string) *PrivateUser {
	return &PrivateUser{
		Id:             user.ID.String(),
		Username:       username,
		Name:           user.Name,
		Email:          email,
		Bio:            user.Bio,
        AvatarUrl:      nil,
		ProfileType:    user.ProfileType,
		CreatedAt:      user.CreatedAt.Time,
		UpdatedAt:      user.UpdatedAt.Time,
	}
}