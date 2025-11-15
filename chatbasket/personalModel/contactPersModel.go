package personalmodel

import "time"

type Contact struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Username  string    `json:"username"`
	Bio       *string   `json:"bio"`
	Nickname  *string   `json:"nickname"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	AvatarURL *string   `json:"avatar_url"`
	IsMutual  bool      `json:"is_mutual"`
}

type GetContactsResponse struct {
	Contacts          []Contact `json:"contacts"`             // ✅ You added
	PeopleWhoAddedYou []Contact `json:"people_who_added_you"` // ✅ They added you
}

type CreateContactPayload struct {
	ContactUserId string  `json:"contact_user_id"`
	Nickname      *string `json:"nickname"`
}

type CheckContactExistancePayload struct {
	ContactUsername string `json:"contact_username"`
}

type CheckContactExistanceResponse struct {
	Exists          bool    `json:"exists"`
	ProfileType     string  `json:"profile_type"`
	RecipientUserId *string `json:"recipient_user_id"`
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

type PendingContactRequest struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Username    string    `json:"username"`
	Bio         *string   `json:"bio"`
	Nickname    *string   `json:"nickname"`
	RequestedAt time.Time `json:"requested_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Status      string    `json:"status"`
	AvatarURL   *string   `json:"avatar_url"`
}

type SentContactRequest struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Username    string    `json:"username"`
	Bio         *string   `json:"bio"`
	Nickname    *string   `json:"nickname"`
	RequestedAt time.Time `json:"requested_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Status      string    `json:"status"`
	AvatarURL   *string   `json:"avatar_url"`
}

type GetContactRequestsResponse struct {
	Pending []PendingContactRequest `json:"pending_requests"`
	Sent    []SentContactRequest    `json:"sent_requests"`
}

type UpdateContactNicknamePayload struct {
	ContactUserId string  `json:"contact_user_id"`
	Nickname      *string `json:"nickname"`
}

type RemoveContactNicknamePayload struct {
	ContactUserId string `json:"contact_user_id"`
}
