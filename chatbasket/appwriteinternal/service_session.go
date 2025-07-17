package appwriteinternal

import (
	"github.com/appwrite/sdk-for-go/appwrite"
	"github.com/appwrite/sdk-for-go/users"
)

type AppwriteServiceSession struct {
	Users *users.Users
}

func NewAppwriteServiceSession(endpoint, projectID, apiKey string) *AppwriteServiceSession {

	c := appwrite.NewClient(
		appwrite.WithEndpoint(endpoint),
		appwrite.WithProject(projectID),
		appwrite.WithKey(apiKey),
	)

	return &AppwriteServiceSession{
		Users: appwrite.NewUsers(c),
	}
}
