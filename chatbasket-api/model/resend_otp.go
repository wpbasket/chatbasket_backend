package model

// 📝 Resend OTP payload
type ResendOTPPayload struct {
	Email string `json:"email"`
	Type  string `json:"type"` // "signup" or "login"
}
