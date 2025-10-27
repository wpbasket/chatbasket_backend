package personalmodel

import "time"

type Contact struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Username         string    `json:"username"`
	Bio              *string   `json:"bio"`
	CreatedAt        time.Time `json:"created_at"`
	AvatarURL        *string   `json:"avatar_url"`
	IsMutual         bool      `json:"is_mutual"`
}

type GetContactsResponse struct {
	Contacts          []Contact `json:"contacts"`             // ✅ You added
	PeopleWhoAddedYou []Contact `json:"people_who_added_you"` // ✅ They added you
}
