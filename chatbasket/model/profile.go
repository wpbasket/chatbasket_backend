package model

// 🔐 Full DB model (used internally, never exposed directly in APIs)
type User struct {
	Id               string `json:"$id"`                        // Always required
	Username         string `json:"username"`                   // Required for identity
	Name             string `json:"name"`                       // Optional display name
	Email            string `json:"email"`                      // Required for login/contact
	Bio              string `json:"bio,omitempty"`              // Optional user bio
	Avatar           string `json:"avatar,omitempty"`           // Optional profile image
	Followers        int64  `json:"followers"`                  // Follower count
	Following        int    `json:"following"`                  // Following count
	Posts 	         int    `json:"posts"`                      // Post count
	ProfileVisibleTo string `json:"profileVisibleTo"`           // "public", "followers", "private"
	IsAdminBlocked   bool   `json:"isAdminBlocked,omitempty"`   // Admin blocked flag
	AdminBlockReason string `json:"adminBlockReason,omitempty"` // Reason for admin block
	CreatedAt        string `json:"$createdAt,omitempty"`       // Timestamp
	UpdatedAt        string `json:"$updatedAt,omitempty"`       // Timestamp
}

// 👤 Private user view (for user settings or own profile)
type PrivateUser struct {
	Id        string `json:"id"`               // Required
	Username  string `json:"username"`         // Required
	Name      string `json:"name"`             // Display name
	Email     string `json:"email"`            // Required for settings
	Bio       string `json:"bio,omitempty"`    // Bio
	Avatar    string `json:"avatar,omitempty"` // Profile image
	CreatedAt string `json:"createdAt"`        // Created at
	UpdatedAt string `json:"updatedAt"`        // Updated at
	ProfileVisibleTo string `json:"profileVisibleTo"` // Profile visibility setting
	Followers int64  `json:"followers"`        // Follower count
	Following int    `json:"following"`        // Following count
	Posts 	  int    `json:"posts"`            // Post count
}

// db payload for creating user profile
type CreateOrUpdateUserProfile struct {
	Username         string `json:"username"`                   // Required for identity
	Name             string `json:"name"`                       // Optional display name
	Email            string `json:"email"`                      // Required for login/contact
	Bio              string `json:"bio,omitempty"`              // Optional user bio
	Avatar           string `json:"avatar,omitempty"`           // Optional profile image
	ProfileVisibleTo string `json:"profileVisibleTo"`           // "public", "followers", "private"
}


//  payload for creating user profile
type CreateUserProfilePayload struct {
	Username 		 string `json:"username" validate:"required,min=1,max=30,regexp=^[a-z0-9][a-z0-9._]*$"`                   // Required for identity
	Name             string `json:"name" validate:"required,min=1,max=70,regexp=^[a-zA-Z0-9]+(?: [a-zA-Z0-9]+)*$"`
	Bio              string `json:"bio" validate:"required,max=200"`
	ProfileVisibleTo string `json:"profileVisibleTo" validate:"required,oneof=public followers private"`           // "public", "followers", "private"
}



//  payload for creating user profile
type UpdateUserProfilePayload struct {
	Username         string `json:"username,omitempty" validate:"omitempty,min=1,max=30,regexp=^[a-z0-9][a-z0-9._]*$"`
	Name             string `json:"name,omitempty" validate:"omitempty,min=1,max=70,regexp=^[a-zA-Z0-9]+(?: [a-zA-Z0-9]+)*$"`
	Bio              string `json:"bio,omitempty" validate:"omitempty,max=200"`
	ProfileVisibleTo string `json:"profileVisibleTo,omitempty" validate:"omitempty,oneof=public followers private"`
	Avatar           string `json:"avatar,omitempty"`
}

type UploadUserProfilePictureResponse struct {
	Id string `json:"id"`
	Name string `json:"name"`
}

// 🔁 Convert full user model → private view
func ToPrivateUser(u *User) *PrivateUser {
	return &PrivateUser{
		Id:        u.Id,
		Username:  u.Username,
		Name:      u.Name,
		Email:     u.Email,
		Bio:       u.Bio,
		Avatar:    u.Avatar,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		ProfileVisibleTo: u.ProfileVisibleTo,
		Followers: u.Followers,
		Following: u.Following,
		Posts: u.Posts,
	}
}