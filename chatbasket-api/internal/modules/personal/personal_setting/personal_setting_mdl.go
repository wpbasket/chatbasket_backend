package personal_setting

type registerOrUpdateFcmOrApnTokenPayload struct {
	Token      string  `json:"token" validate:"required,min=1"`
	Type       string  `json:"type" validate:"required,oneof=fcm apn"`
	DeviceName *string `json:"deviceName,omitempty"`
}
