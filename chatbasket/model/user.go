package model

import (
	"net/http"
	"github.com/labstack/echo/v4"
)



// AppwriteUserPayload is the structure for creating/updating user documents in Appwrite.
// It includes all fields that can be directly set in the database.
type AppwriteUserPayload struct {
	Name             string `json:"name"`
	Email            string `json:"email"`
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
	Id       string `json:"id"`               // Required for linking
	Username string `json:"username"`         // Public ID
	Name     string `json:"name"`             // Display name
	Bio      string `json:"bio,omitempty"`    // Bio (optional)
	Avatar   string `json:"avatar,omitempty"` // Profile image (optional)
	HasPendingRequest bool `json:"hasPendingRequest,omitempty"` // 
}

// 🧩 Preview user view (used in post/comment cards, follow lists)
type PreviewPublicUser struct {
	Id       			string `json:"id"`               				// ID
	Username 			string `json:"username"`         				// Username
	Name     			string `json:"name"`             				// Display name
	Avatar   			string `json:"avatar,omitempty"` 				// Optional avatar
	HasPendingRequest 	bool   `json:"hasPendingRequest,omitempty"` 	// 
}

// 📝 Login initial response (used in login endpoint)
type LoginIntialResponse struct {
	Status string `json:"status"`
}

// 📝 Signup initial response (used in signup endpoint)
type SignupIntialResponse struct {
	Status string `json:"status"`
}



// SignupSessionResponse is the response structure after successful signup.
type SessionResponse struct {
	UserId		  string       `json:"userId"`	
	Name          string       `json:"name"`
	Email         string       `json:"email"`
	SessionID     string       `json:"sessionId"`
	SessionExpiry string       `json:"sessionExpiry"`
}



type AuthVerificationPayload struct {
	Email  string `json:"email"`
	Secret string `json:"secret"` // OTP code from email
	Platform string `json:"platform"`
}


// 📝 Logout payload
type LogoutPayload struct {
	AllSessions bool   `json:"allSessions"`
}

// 📝 Logout response
type LogoutResponse struct {
	Status string `json:"status"`
}


type CheckIfUserNameAvailablePayload struct {
	Username string `json:"username"`
}





// 
type UpdateEmailVerification struct {
	Email string `json:"email"`
	Otp string `json:"otp"`
} 


//  
type StatusOkay struct {
	Status bool `json:"status"`
	Message string `json:"message"`
}






// 
type TempOtp struct{
	Id        string `json:"$id"`
	Email     string `json:"email"`
	Otp       string `json:"otp"`
	UserId    string `json:"userId"`
	MessageId string `json:"messageId"`
	CreatedAt string `json:"$createdAt"`
	UpdatedAt string `json:"$updatedAt"`
}

// 
type TempOtpPayload struct{
	Email     string `json:"email"`
	Otp       string `json:"otp"`
	UserId    string `json:"userId"`
	MessageId string `json:"messageId"`
} 

// 
type UpdatePassword struct{
	NewPassword string `json:"newPassword"`
}

// 🔁 Convert full user model → public view
func ToPublicUser(u *User, hasPendingRequest bool) *PublicUser {
	return &PublicUser{
		Id:       u.Id,
		Username: u.Username,
		Name:     u.Name,
		Bio:      u.Bio,
		Avatar:   u.AvatarFileId,
		HasPendingRequest: hasPendingRequest,
	}
}

// 🔁 Convert full user model → preview view
func ToPreviewPublicUser(u *User, hasPendingRequest bool) PreviewPublicUser {
	return PreviewPublicUser{
		Id:       u.Id,
		Username: u.Username,
		Name:     u.Name,
		Avatar:   u.AvatarFileId,
		HasPendingRequest: hasPendingRequest,

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

