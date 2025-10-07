package model

type UpdateEmailPayload struct {
	Email string `json:"email"`
}

type OtpVerificationPayload struct {
	Secret string `json:"secret"`
}

type SendOtpPayload struct {
	Subject string `json:"subject"`
}

