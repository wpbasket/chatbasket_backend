package appwrite

import (
	"github.com/appwrite/sdk-for-go/account"
	"github.com/appwrite/sdk-for-go/appwrite"
	"github.com/appwrite/sdk-for-go/databases"
	"github.com/appwrite/sdk-for-go/storage"
)

type AppwriteServiceSession struct {
	Account                   *account.Account
	Database                  *databases.Databases
	Storage                   *storage.Storage
	DatabaseID                string
	UsersCollectionID         string
	PostsCollectionID         string
	CommentsCollectionID      string
	BlockCollectionID         string
	LikesCollectionID         string
	FollowCollectionID        string
	RefreshTokensCollectionID string
	
}

func NewAppwriteServiceWithSession(endpoint, projectID, sessionID, databaseID, usersCollectionID, postsCollectionID, commentsCollectionID, blockCollectionID, likesCollectionID, followCollectionID, refreshTokensCollectionID string) *AppwriteServiceSession {
	c := appwrite.NewClient(
		appwrite.WithEndpoint(endpoint),
		appwrite.WithProject(projectID),
		appwrite.WithSession(sessionID), // 🔐 Use session token instead of API key
	)

	return &AppwriteServiceSession{
		Account:  appwrite.NewAccount(c),
		Database: appwrite.NewDatabases(c),
		Storage:  appwrite.NewStorage(c),
		DatabaseID:                databaseID,
		UsersCollectionID:         usersCollectionID,
		PostsCollectionID:         postsCollectionID,
		CommentsCollectionID:      commentsCollectionID,
		BlockCollectionID:         blockCollectionID,
		LikesCollectionID:         likesCollectionID,
		FollowCollectionID:        followCollectionID,
		RefreshTokensCollectionID: refreshTokensCollectionID,
	}
}
