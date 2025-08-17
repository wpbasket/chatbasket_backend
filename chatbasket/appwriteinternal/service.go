package appwriteinternal

import (
	"github.com/appwrite/sdk-for-go/account"
	"github.com/appwrite/sdk-for-go/appwrite"
	"github.com/appwrite/sdk-for-go/databases"
	"github.com/appwrite/sdk-for-go/messaging"
	"github.com/appwrite/sdk-for-go/storage"
	"github.com/appwrite/sdk-for-go/tokens"
	"github.com/appwrite/sdk-for-go/users"
)

type AppwriteService struct {
	Account                   *account.Account
	Database                  *databases.Databases
	Storage                   *storage.Storage
	Users                     *users.Users
	Message                   *messaging.Messaging
	Tokens                    *tokens.Tokens
	DatabaseID                string
	UsersCollectionID         string
	PostsCollectionID         string
	CommentsCollectionID      string
	BlockCollectionID         string
	LikesCollectionID         string
	FollowCollectionID        string
	RefreshTokensCollectionID string
	FollowRequestsCollectionID string
	TempOtpCollectionID       string
	ProfilePicBucketID        string
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
	refreshTokensCollectionID,
	followRequestsCollectionID,
	tempOtpCollectionID,
	profilePicBucketID string) *AppwriteService {

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
		Message:                   appwrite.NewMessaging(c),
		Tokens:                    appwrite.NewTokens(c),
		DatabaseID:                databaseID,
		UsersCollectionID:         usersCollectionID,
		PostsCollectionID:         postsCollectionID,
		CommentsCollectionID:      commentsCollectionID,
		BlockCollectionID:         blockCollectionID,
		LikesCollectionID:         likesCollectionID,
		FollowCollectionID:        followCollectionID,
		RefreshTokensCollectionID: refreshTokensCollectionID,
		FollowRequestsCollectionID: followRequestsCollectionID,
		TempOtpCollectionID:       tempOtpCollectionID,
		ProfilePicBucketID:        profilePicBucketID,
	}
}
