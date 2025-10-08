package model

import "fmt"

// 🔐 Full DB model (used internally, never exposed directly in APIs)
type User struct {
	Id               string   `json:"$id"`                        // Always required
	Username         string   `json:"username"`                   // Required for identity
	Name             string   `json:"name"`                       // Optional display name
	Email            string   `json:"email"`                      // Required for login/contact
	Bio              string   `json:"bio,omitempty"`              // Optional user bio
	AvatarFileId     string   `json:"avatarFileId,omitempty"`     // Optional profile image
	AvatarFileTokens []string `json:"avatarFileTokens,omitempty"` // Tokens for accessing Avatar ["personal_token","personal_token_secret"]
	Followers        int64    `json:"followers"`                  // Follower count
	Following        int      `json:"following"`                  // Following count
	Posts            int      `json:"posts"`                      // Post count
	ProfileVisibleTo string   `json:"profileVisibleTo"`           // "public", "followers", "private"
	IsAdminBlocked   bool     `json:"isAdminBlocked,omitempty"`   // Admin blocked flag
	AdminBlockReason string   `json:"adminBlockReason,omitempty"` // Reason for admin block
	CreatedAt        string   `json:"$createdAt,omitempty"`       // Timestamp
	UpdatedAt        string   `json:"$updatedAt,omitempty"`       // Timestamp
}

// 👤 Private user view (for user settings or own profile)
type PrivateUser struct {
	Id               string `json:"id"`                  // Required
	Username         string `json:"username"`            // Required
	Name             string `json:"name"`                // Display name
	Email            string `json:"email"`               // Required for settings
	Bio              string `json:"bio,omitempty"`       // Bio
	AvatarUri        string `json:"avatarUri,omitempty"` // Avatar URI
	CreatedAt        string `json:"createdAt"`           // Created at
	UpdatedAt        string `json:"updatedAt"`           // Updated at
	ProfileVisibleTo string `json:"profileVisibleTo"`    // Profile visibility setting
	Followers        int64  `json:"followers"`           // Follower count
	Following        int    `json:"following"`           // Following count
	Posts            int    `json:"posts"`               // Post count
}

// db payload for creating user profile
type CreateUserProfileDbPayload struct {
	Username         string   `json:"username"`                   // Required for identity
	Name             string   `json:"name"`                       // Optional display name
	Email            string   `json:"email"`                      // Required for login/contact
	Bio              string   `json:"bio,omitempty"`              // Optional user bio
	AvatarFileId     string   `json:"avatarFileId,omitempty"`     // Optional profile image
	AvatarFileTokens []string `json:"avatarFileTokens,omitempty"` // Tokens for accessing Avatar ["personal_token","personal_token_secret"]
	ProfileVisibleTo string   `json:"profileVisibleTo"`           // "public", "followers", "private"
}

// db payload for creating user profile
type UpdateUserProfileDbPayload struct {
	Username         string   `json:"username,omitempty"`         // Required for identity
	Name             string   `json:"name,omitempty"`             // Optional display name
	Email            string   `json:"email,omitempty"`            // Required for login/contact
	Bio              string   `json:"bio,omitempty"`              // Optional user bio
	AvatarFileId     string   `json:"avatarFileId,omitempty"`     // Optional profile image
	AvatarFileTokens []string `json:"avatarFileTokens,omitempty"` // Tokens for accessing Avatar ["personal_token","personal_token_secret"]
	ProfileVisibleTo string   `json:"profileVisibleTo,omitempty"` // "public", "followers", "private"
}

// db payload for removing profile picture
type RemoveProfilePictureDbPayload struct {
	AvatarFileId     string   `json:"avatarFileId"`
	AvatarFileTokens []string `json:"avatarFileTokens"`
}

// payload for creating user profile
type CreateUserProfilePayload struct {
	Username         string `json:"username" validate:"required,min=1,max=30,regexp=^[a-z0-9][a-z0-9._]*$"` // Required for identity
	Name             string `json:"name" validate:"required,min=1,max=70,regexp=^[a-zA-Z0-9]+(?: [a-zA-Z0-9]+)*$"`
	Bio              string `json:"bio" validate:"required,max=200"`
	ProfileVisibleTo string `json:"profileVisibleTo" validate:"required,oneof=public followers private"` // "public", "followers", "private"
}

// payload for creating user profile
type UpdateUserProfilePayload struct {
	Username         string   `json:"username,omitempty" validate:"omitempty,min=1,max=30,regexp=^[a-z0-9][a-z0-9._]*$"`
	Name             string   `json:"name,omitempty" validate:"omitempty,min=1,max=70,regexp=^[a-zA-Z0-9]+(?: [a-zA-Z0-9]+)*$"`
	Bio              string   `json:"bio,omitempty" validate:"omitempty,max=200"`
	ProfileVisibleTo string   `json:"profileVisibleTo,omitempty" validate:"omitempty,oneof=public followers private"`
	AvatarFileId     string   `json:"avatarFileId,omitempty"`     // fileid is userid
	AvatarFileTokens []string `json:"avatarFileTokens,omitempty"` // Tokens for accessing Avatar ["personal_token","personal_token_secret"]
}

type UploadUserProfilePictureResponse struct {
	AvatarFileId     string   `json:"avatarFileId"`
	Name             string   `json:"name"`
	AvatarFileTokens []string `json:"avatarFileTokens,omitempty"` // Tokens for accessing Avatar ["personal_token","personal_token_secret"]
}

// AppwriteFileData represents the data needed to construct an appwrite file URI
type AppwriteFileData struct {
	FileId     string   `json:"fileId"`
	FileTokens []string `json:"fileTokens"`
}

// BuildAvatarURI constructs the avatar URL from AppwriteFileData
// Returns empty string if data is invalid or insufficient tokens
func BuildAvatarURI(ad *AppwriteFileData, minTokens int) string {
	if ad == nil || ad.FileId == "" || len(ad.FileTokens) < minTokens {
		return ""
	}

	// Use the personal_token_secret (index 1) for avatar access
	return fmt.Sprintf("https://fra.cloud.appwrite.io/v1/storage/buckets/685bc613002edcfee6bb/files/%s/view?project=6858ed4d0005c859ea03&token=%s",
		ad.FileId, ad.FileTokens[1])
}

// 🔁 Convert full user model → private view
func ToPrivateUser(u *User, avataruri string) *PrivateUser {
	return &PrivateUser{
		Id:               u.Id,
		Username:         u.Username,
		Name:             u.Name,
		Email:            u.Email,
		Bio:              u.Bio,
		AvatarUri:        avataruri,
		CreatedAt:        u.CreatedAt,
		UpdatedAt:        u.UpdatedAt,
		ProfileVisibleTo: u.ProfileVisibleTo,
		Followers:        u.Followers,
		Following:        u.Following,
		Posts:            u.Posts,
	}
}
