package personalmodel

import "time"

type Contact struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Username         string    `json:"username"`
	Bio              *string   `json:"bio"`
	CreatedAt        time.Time `json:"created_at"`
	AvatarFileID     *string   `json:"avatar_file_id"`
	AvatarFileSecret *string   `json:"avatar_file_secret"`
	IsMutual         bool      `json:"is_mutual"` // ✅ new field
}

type GetContactsResponse struct {
	Contacts []Contact `json:"contacts"`
}
