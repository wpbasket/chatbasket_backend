package core_auth

// RegisterOrUpdateFcmOrApnTokenPayload remains consistent with original naming
type RegisterOrUpdateFcmOrApnTokenPayload struct {
	Token      string  `json:"token" validate:"required,min=1"`
	Type       string  `json:"type" validate:"required,oneof=fcm apn"`
	DeviceName *string `json:"device_name,omitempty"`
}
