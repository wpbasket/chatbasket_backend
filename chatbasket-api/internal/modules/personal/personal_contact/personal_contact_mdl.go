package personal_contact

type CreateContactPayload struct {
	ContactUserId string  `json:"contact_user_id"`
	Nickname      *string `json:"nickname"`
}

type CheckContactExistancePayload struct {
	ContactUsername string `json:"contact_username"`
}

type AcceptContactRequestPayload struct {
	ContactUserId string `json:"contact_user_id"`
}

type RejectContactRequestPayload struct {
	ContactUserId string `json:"contact_user_id"`
}

type DeleteContactPayload struct {
	ContactUserId []string `json:"contact_user_id"`
}

type UndoContactRequestPayload struct {
	ContactUserId string `json:"contact_user_id"`
}

type UpdateContactNicknamePayload struct {
	ContactUserId string  `json:"contact_user_id"`
	Nickname      *string `json:"nickname"`
}

type RemoveContactNicknamePayload struct {
	ContactUserId string `json:"contact_user_id"`
}

type BlockUserPayload struct {
	BlockedUserId string `json:"blocked_user_id" validate:"required,uuid"`
}

