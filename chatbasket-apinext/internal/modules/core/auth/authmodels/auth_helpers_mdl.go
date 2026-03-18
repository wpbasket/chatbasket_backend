package authmodels

// RegisterOrUpdateFcmOrApnTokenPayload remains consistent with original naming
type RegisterOrUpdateFcmOrApnTokenPayload struct {
	Token      string  `json:"token"`
	Type       string  `json:"type"` // "fcm" or "apn"
	DeviceName *string `json:"device_name,omitempty"`
}
