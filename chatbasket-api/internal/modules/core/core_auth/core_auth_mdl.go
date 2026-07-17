package core_auth

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

// Signup SessionResponse is the response structure after successful signup.
type SessionResponse struct {
	UserId            string `json:"userId"`
	Name              string `json:"name"`
	Email             string `json:"email"`
	SessionID         string `json:"sessionId"`
	SessionExpiry     string `json:"sessionExpiry"`
	IsPrimary         bool   `json:"isPrimary"`
	PrimaryDeviceName string `json:"primaryDeviceName,omitempty"`
	PrimaryKey        string `json:"primaryKey"`
	KeysRevision      int32  `json:"keysRevision"`
}

type AuthVerificationPayload struct {
	Email    string `json:"email"`
	Secret   string `json:"secret"` // OTP code from email
	Platform string `json:"platform"`
}

// 📝 Resend OTP payload
type ResendOTPPayload struct {
	Email string `json:"email"`
	Type  string `json:"type"` // "signup" or "login"
}

// 🔑 Forgot Password payload
type ForgotPasswordPayload struct {
	Email string `json:"email"`
}

// 🔑 Forgot Password Verify payload
type ForgotPasswordVerifyPayload struct {
	UpdateID    string `json:"updateId"`
	Otp         string `json:"otp"`
	NewPassword string `json:"newPassword"`
}
