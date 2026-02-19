package appwriteinternal

import (
	"time"

	"github.com/appwrite/sdk-for-go/appwrite"
	"github.com/appwrite/sdk-for-go/storage"
	"github.com/appwrite/sdk-for-go/tokens"
)

type AppwriteStorageService struct {
	Storage  *storage.Storage
	Tokens   *tokens.Tokens
	Endpoint string
	Project  string
}

func NewAppwriteStorageService(endpoint, projectID, apiKey string) *AppwriteStorageService {
	// High timeout for storage operations (10 minutes)
	c := appwrite.NewClient(
		appwrite.WithEndpoint(endpoint),
		appwrite.WithProject(projectID),
		appwrite.WithKey(apiKey),
		appwrite.WithTimeout(600*time.Second),
	)

	return &AppwriteStorageService{
		Storage:  appwrite.NewStorage(c),
		Tokens:   appwrite.NewTokens(c),
		Endpoint: endpoint,
		Project:  projectID,
	}
}
