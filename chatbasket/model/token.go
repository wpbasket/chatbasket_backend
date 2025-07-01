package model

type RefreshToken struct {
	Id           string `json:"$id"`
	UserId       string `json:"userId"`
	Token        string `json:"token"`
	CreatedAt    string `json:"$createdAt"`
	UpdatedAt    string `json:"$updatedAt"`
	ExpiresAt    string `json:"expiresAt"`
}


