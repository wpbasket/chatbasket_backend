package model

import (
	"net/http"

	"github.com/appwrite/sdk-for-go/models"
	"github.com/labstack/echo/v4"
)

// 🔐 Full DB model (used internally, never exposed directly in APIs)
type User struct {
	Id               string `json:"$id"`                         // Always required
	Username         string `json:"username"`                    // Required for identity
	Name             string `json:"name"`                        // Optional display name
	Email            string `json:"email"`                       // Required for login/contact
	Bio              string `json:"bio,omitempty"`               // Optional user bio
	Avatar           string `json:"avatar,omitempty"`            // Optional profile image
	Followers        int64  `json:"followers"`                   // Follower count
	Following        int    `json:"following"`                   // Following count
	ProfileVisibleTo string `json:"profileVisibleTo"`            // "public", "followers", "private"
	IsAdminBlocked   bool   `json:"isAdminBlocked,omitempty"`    // Admin blocked flag
	AdminBlockReason string `json:"adminBlockReason,omitempty"`  // Reason for admin block
	CreatedAt        string `json:"$createdAt,omitempty"`        // Timestamp
	UpdatedAt        string `json:"$updatedAt,omitempty"`        // Timestamp
}

// AppwriteUserPayload is the structure for creating/updating user documents in Appwrite.
// It includes all fields that can be directly set in the database.
type AppwriteUserPayload struct {
	Username         string `json:"username"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Bio              string `json:"bio"`
	Avatar           string `json:"avatar"`
	Followers        int64  `json:"followers"`
	Following        int    `json:"following"`
	ProfileVisibleTo string `json:"profileVisibleTo"`
	IsAdminBlocked   bool   `json:"isAdminBlocked"`
	AdminBlockReason string `json:"adminBlockReason"`
}


// 📝 Signup payload (used in signup endpoint)
type SignupPayload struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// 🔐 Login payload (supports email or username login)
type LoginPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// 🌐 Public user view (used when others view your profile)
type PublicUser struct {
	Id       string `json:"id"`                // Required for linking
	Username string `json:"username"`          // Public ID
	Name     string `json:"name"`              // Display name
	Bio      string `json:"bio,omitempty"`     // Bio (optional)
	Avatar   string `json:"avatar,omitempty"`  // Profile image (optional)
}

// 🧩 Preview user view (used in post/comment cards, follow lists)
type PreviewPublicUser struct {
	Id       string `json:"id"`               // ID
	Username string `json:"username"`         // Username
	Name     string `json:"name"`             // Display name
	Avatar   string `json:"avatar,omitempty"` // Optional avatar
}

// 📝 Login initial response (used in login endpoint)
type LoginIntialResponse struct {
	Status string `json:"status"`
}

// 📝 Signup initial response (used in signup endpoint)
type SignupIntialResponse struct {
    Status string `json:"status"`
}


// 👤 Private user view (for user settings or own profile)
type PrivateUser struct {
	Id       string `json:"id"`                // Required
	Username string `json:"username"`          // Required
	Name     string `json:"name"`              // Display name
	Email    string `json:"email"`             // Required for settings
	Bio      string `json:"bio,omitempty"`     // Bio
	Avatar   string `json:"avatar,omitempty"`  // Profile image
	CreatedAt string `json:"createdAt"`         // Created at
	UpdatedAt string `json:"updatedAt"`         // Updated at
}

// 🧾 SessionResponse - final response format
type SessionResponse struct {
	User      		*PrivateUser 	`json:"user"`
	SessionID 		string			`json:"sessionId"`
	SessionExpiry 	string			`json:"sessionExpiry"`
}

type AccountVerificationPayload struct {
	Name string `json:"name"`
	Email string `json:"email"`
	Secret string `json:"secret"` // OTP code from email
}

// 🔐 Login payload (supports email login)
type LoginVerificationPayload struct {
	Email string `json:"email"`
	Secret string `json:"secret"` // OTP code from email
}

// 🔁 Convert full user model → session response
func ToSessionResponse(u *User, s *models.Session) *SessionResponse {
	return &SessionResponse{
		User:          ToPrivateUser(u),
		SessionID:     s.Id,
		SessionExpiry: s.Expire,
	}
}


// 🔁 Convert full user model → public view
func ToPublicUser(u *User) *PublicUser {
	return &PublicUser{
		Id:       u.Id,
		Username: u.Username,
		Name:     u.Name,
		Bio:      u.Bio,
		Avatar:   u.Avatar,
	}
}

// 🔁 Convert full user model → preview view
func ToPreviewPublicUser(u *User) PreviewPublicUser {
	return PreviewPublicUser{
		Id:       u.Id,
		Username: u.Username,
		Name:     u.Name,
		Avatar:   u.Avatar,
	}
}

// 🔁 Convert full user model → private view
func ToPrivateUser(u *User) *PrivateUser {
	return &PrivateUser{
		Id:       u.Id,
		Username: u.Username,
		Name:     u.Name,
		Email:    u.Email,
		Bio:      u.Bio,
		Avatar:   u.Avatar,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

// ✅ Check if user's profile is viewable by the current viewer
func CanViewUserProfile(user User, viewerId string, isFollower bool) bool {
	switch user.ProfileVisibleTo {
	case "private":
		return viewerId == user.Id
	case "followers":
		return viewerId == user.Id || isFollower
	case "public":
		return true
	default:
		return false
	}
}

// 🔒 Check if user is blocked by admin
func IsUserBlockedByAdmin(user User) bool {
	return user.IsAdminBlocked
}

// ✅ Check and return error if user is blocked
func CheckIfUserBlocked(user *User) error {
	if IsUserBlockedByAdmin(*user) {
		return echo.NewHTTPError(http.StatusForbidden, "Your account has been blocked "+user.AdminBlockReason)
	}
	return nil
}
