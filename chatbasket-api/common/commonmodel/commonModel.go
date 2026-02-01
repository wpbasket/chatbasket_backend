package commonmodel

// LogoutPayload for logout requests (works for both public and personal modes)
type LogoutPayload struct {
	AllSessions bool `json:"all_sessions"` // true = logout from all sessions, false = logout from current session only
}

// RequestUpdateOTPPayload RequestUpdateOTPPayload is used to request an OTP for update operations
type RequestUpdateOTPPayload struct {
	UpdateType string `json:"updateType"` // "password_update" or "email_update"
}

// ConfirmPasswordUpdatePayload is used to confirm password update with OTP
type ConfirmPasswordUpdatePayload struct {
	UpdateID    string `json:"updateId"`    // UUID from RequestUpdateOTP
	Otp         string `json:"otp"`         // OTP code
	NewPassword string `json:"newPassword"` // New password to set
}

// RequestEmailUpdatePayload is used to request email update
type RequestEmailUpdatePayload struct {
	NewEmail string `json:"newEmail"` // New email address
	Password string `json:"password"` // Current password for verification
}

// ConfirmEmailUpdatePayload is used to confirm email update with OTP
type ConfirmEmailUpdatePayload struct {
	UpdateID string `json:"updateId"` // UUID from RequestEmailUpdate
	Otp      string `json:"otp"`      // OTP code
}
