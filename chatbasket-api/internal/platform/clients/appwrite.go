package clients

import (
	"chatbasket-api/internal/platform/config"
	"time"

	"github.com/appwrite/sdk-for-go/account"
	"github.com/appwrite/sdk-for-go/appwrite"
	"github.com/appwrite/sdk-for-go/databases"
	"github.com/appwrite/sdk-for-go/messaging"
	"github.com/appwrite/sdk-for-go/storage"
	"github.com/appwrite/sdk-for-go/tablesdb"
	"github.com/appwrite/sdk-for-go/tokens"
	"github.com/appwrite/sdk-for-go/users"
)

// ported from appwriteinternal/service_storage.go
type AppwriteStorageService struct {
	Storage  *storage.Storage
	Tokens   *tokens.Tokens
	Endpoint string
	Project  string
}

// ported from appwriteinternal/service_storage.go
func NewAppwriteStorageService(cfg *config.AppwriteConfig) *AppwriteStorageService {
	// High timeout for storage operations (10 minutes)
	c := appwrite.NewClient(
		appwrite.WithEndpoint(cfg.Endpoint),
		appwrite.WithProject(cfg.ProjectID),
		appwrite.WithKey(cfg.ApiKey),
		appwrite.WithTimeout(600*time.Second),
	)

	return &AppwriteStorageService{
		Storage:  appwrite.NewStorage(c),
		Tokens:   appwrite.NewTokens(c),
		Endpoint: cfg.Endpoint,
		Project:  cfg.ProjectID,
	}
}

// ported from appwriteinternal/service.go
type AppwriteService struct {
	Account                    *account.Account
	Database                   *databases.Databases
	TableDb                    *tablesdb.TablesDB
	Users                      *users.Users
	Message                    *messaging.Messaging
	Endpoint                   string
	ProjectID                  string
	DatabaseID                 string
	UsersCollectionID          string
	PostsCollectionID          string
	CommentsCollectionID       string
	BlockCollectionID          string
	LikesCollectionID          string
	FollowCollectionID         string
	RefreshTokensCollectionID  string
	FollowRequestsCollectionID string
	TempOtpCollectionID        string
	ProfilePicBucketID         string
	PersonalUsersCollectionID  string
	AloneUsernameCollectionID  string
	PersonalDatabaseID         string
	PersonalProfilePicBucketID string
}

// NewAppwriteService creates a new Appwrite client and initializes all services, ported from appwriteinternal/service.go
func NewAppwriteService(cfg *config.AppwriteConfig) *AppwriteService {

	c := appwrite.NewClient(
		appwrite.WithEndpoint(cfg.Endpoint),
		appwrite.WithProject(cfg.ProjectID),
		appwrite.WithKey(cfg.ApiKey),
		appwrite.WithTimeout(30*time.Second),
	)

	return &AppwriteService{
		Account:                    appwrite.NewAccount(c),
		Database:                   appwrite.NewDatabases(c),
		TableDb:                    appwrite.NewTablesDB(c),
		Users:                      appwrite.NewUsers(c),
		Message:                    appwrite.NewMessaging(c),
		Endpoint:                   cfg.Endpoint,
		ProjectID:                  cfg.ProjectID,
		DatabaseID:                 cfg.DatabaseID,
		UsersCollectionID:          cfg.UsersCollectionID,
		PostsCollectionID:          cfg.PostsCollectionID,
		CommentsCollectionID:       cfg.CommentsCollectionID,
		BlockCollectionID:          cfg.BlockCollectionID,
		LikesCollectionID:          cfg.LikesCollectionID,
		FollowCollectionID:         cfg.FollowCollectionID,
		RefreshTokensCollectionID:  cfg.RefreshTokensCollectionID,
		FollowRequestsCollectionID: cfg.FollowRequestsCollectionID,
		TempOtpCollectionID:        cfg.TempOtpCollectionID,
		ProfilePicBucketID:         cfg.FileUserProfilePicBucketID,
		PersonalUsersCollectionID:  cfg.PersonalUsersCollectionID,
		AloneUsernameCollectionID:  cfg.PersonalAloneUsernameCollectionID,
		PersonalDatabaseID:         cfg.PersonalDatabaseID,
		PersonalProfilePicBucketID: cfg.PersonalProfilePicBucketID,
	}
}

