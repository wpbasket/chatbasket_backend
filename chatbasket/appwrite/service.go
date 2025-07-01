package appwrite

import (
	"github.com/appwrite/sdk-for-go/account"
	"github.com/appwrite/sdk-for-go/appwrite"
	"github.com/appwrite/sdk-for-go/databases"
	"github.com/appwrite/sdk-for-go/storage"
	"github.com/appwrite/sdk-for-go/users"
)

type AppwriteService struct {
	Account                   *account.Account
	Database                  *databases.Databases
	Storage                   *storage.Storage
	Users                     *users.Users
	DatabaseID                string
	UsersCollectionID         string
	PostsCollectionID         string
	CommentsCollectionID      string
	BlockCollectionID         string
	LikesCollectionID         string
	FollowCollectionID        string
	RefreshTokensCollectionID string
	
}

func NewAppwriteService(
	endpoint,
	projectID,
	apiKey,
	databaseID,
	usersCollectionID,
	postsCollectionID,
	commentsCollectionID,
	blockCollectionID,
	likesCollectionID,
	followCollectionID,
	refreshTokensCollectionID string) *AppwriteService {

	c := appwrite.NewClient(
		appwrite.WithEndpoint(endpoint),
		appwrite.WithProject(projectID),
		appwrite.WithKey(apiKey),
	)

	return &AppwriteService{
		Account:                   appwrite.NewAccount(c),
		Database:                  appwrite.NewDatabases(c),
		Storage:                   appwrite.NewStorage(c),
		Users:                     appwrite.NewUsers(c),
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
