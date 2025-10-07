package personalmodel

type User struct {
    Id                                   string   `json:"$id"`
    HmacSha256HexUsername                string   `json:"hmac_sha256_hex_username"`
    B64CipherChacha20Poly1305Username    string   `json:"b64_cipher_chacha20poly1305_username"`
    Name                                 string   `json:"name"`
    Bio                                  string   `json:"bio"`
    Avatar                               string   `json:"avatar"` // avatar id in storage
    AvatarTokens                       []string   `json:"avatar_tokens"`
    Contacts                             int      `json:"contacts"`
    ProfileType                          string   `json:"profile_type"` // User profile type: private/public/personal
    IsAdminBlocked                       bool     `json:"is_admin_blocked"`
    AdminBlockReason                     string   `json:"admin_block_reason"`
    CreatedAt                            string   `json:"$createdAt"`
    UpdatedAt                            string   `json:"$updatedAt"`
}

type CreateUserProfilePayload struct {
    Name        string `json:"name" validate:"required,min=1,max=40"` 
    Bio         string `json:"bio" validate:"max=150"`
    ProfileType string `json:"profile_type" validate:"required,oneof=public private personal"`
}

type CreateUserProfileDbPayload struct {
    Name                                 string   `json:"name" validate:"required,min=1,max=40"` 
    Bio                                  string   `json:"bio" validate:"max=150"`
    ProfileType                          string   `json:"profile_type" validate:"required,oneof=public private personal"`
    HmacSha256HexUsername                string   `json:"hmac_sha256_hex_username"`
    B64CipherChacha20Poly1305Username    string   `json:"b64_cipher_chacha20poly1305_username"`
}



type AloneUsername struct {
	Id 			string `json:"$id"`
	Username 	string `json:"username"`
	CreatedAt 	string `json:"$createdAt"`
	UpdatedAt 	string `json:"$updatedAt"`
}

type AloneUsernameDbPayload struct {
	Username string `json:"username"`
}

type PrivateUser struct {
    Id                                   string   `json:"id"`
    Username                             string   `json:"username"`
    Name                                 string   `json:"name"`
    Email                                string   `json:"email"`
    Bio                                  string   `json:"bio"`
    Avatar                               string   `json:"avatar"` // avatar id in storage
    AvatarTokens                       []string   `json:"avatar_tokens"`
    Contacts                             int      `json:"contacts"`
    ProfileType                          string   `json:"profile_type"` // User profile type: private/public/personal
    CreatedAt                            string   `json:"createdAt"`
    UpdatedAt                            string   `json:"updatedAt"`    
}

type LogoutPayload struct {
	AllSessions bool   `json:"all_sessions"`
}

func ToPrivateUser(user *User,username string,email string) *PrivateUser {
	return &PrivateUser{
		Id:       user.Id,
		Username: username,
		Name:     user.Name,
		Email:    email,
		Bio:      user.Bio,
		Avatar:   user.Avatar,
		AvatarTokens: user.AvatarTokens,
		Contacts: user.Contacts,
		ProfileType: user.ProfileType,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}