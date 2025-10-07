package routes

import (
	"chatbasket/utils"
)

type appwriteConfig struct {
	Endpoint                        string
	ProjectID                       string
	ApiKey                          string
	DatabaseID                      string
	UsersCollectionID               string
	PostsCollectionID               string
	CommentsCollectionID            string
	BlockCollectionID               string
	LikesCollectionID               string
	FollowCollectionID              string
	RefreshTokensCollectionID       string
	FollowRequestsCollectionID      string
	TempOtpCollectionID             string
	FileUserProfilePicBucketID      string
	PersonalUsersCollectionID       string
	PersonalAloneUsernameCollectionID string
	PersonalDatabaseID              string
	PersonalUsernameKey             []byte
}

func loadAppwriteConfig() (*appwriteConfig, error) {
	var c appwriteConfig
	var err error

	if c.Endpoint, err = utils.LoadKeyFromEnv("APPWRITE_ENDPOINT"); err != nil {
		return nil, err
	}
	if c.ProjectID, err = utils.LoadKeyFromEnv("APPWRITE_PROJECT_ID"); err != nil {
		return nil, err
	}
	if c.ApiKey, err = utils.LoadKeyFromEnv("APPWRITE_API_KEY"); err != nil {
		return nil, err
	}
	if c.DatabaseID, err = utils.LoadKeyFromEnv("APPWRITE_DATABASE_ID"); err != nil {
		return nil, err
	}
	if c.UsersCollectionID, err = utils.LoadKeyFromEnv("APPWRITE_USERS_COLLECTION_ID"); err != nil {
		return nil, err
	}
	if c.PostsCollectionID, err = utils.LoadKeyFromEnv("APPWRITE_POSTS_COLLECTION_ID"); err != nil {
		return nil, err
	}
	if c.CommentsCollectionID, err = utils.LoadKeyFromEnv("APPWRITE_COMMENTS_COLLECTION_ID"); err != nil {
		return nil, err
	}
	if c.BlockCollectionID, err = utils.LoadKeyFromEnv("APPWRITE_BLOCK_COLLECTION_ID"); err != nil {
		return nil, err
	}
	if c.LikesCollectionID, err = utils.LoadKeyFromEnv("APPWRITE_LIKES_COLLECTION_ID"); err != nil {
		return nil, err
	}
	if c.FollowCollectionID, err = utils.LoadKeyFromEnv("APPWRITE_FOLLOW_COLLECTION_ID"); err != nil {
		return nil, err
	}
	if c.RefreshTokensCollectionID, err = utils.LoadKeyFromEnv("APPWRITE_REFRESH_TOKENS_COLLECTION_ID"); err != nil {
		return nil, err
	}
	if c.FollowRequestsCollectionID, err = utils.LoadKeyFromEnv("APPWRITE_FOLLOW_REQUESTS_COLLECTION_ID"); err != nil {
		return nil, err
	}
	if c.TempOtpCollectionID, err = utils.LoadKeyFromEnv("APPWRITE_TEMP_OTP_COLLECTION_ID"); err != nil {
		return nil, err
	}
	if c.FileUserProfilePicBucketID, err = utils.LoadKeyFromEnv("APPWRITE_FILE_USERPROFILEPIC_BUCKET_ID"); err != nil {
		return nil, err
	}
	if c.PersonalUsersCollectionID, err = utils.LoadKeyFromEnv("APPWRITE_PERSONAL_USERS_COLLECTION_ID"); err != nil {
		return nil, err
	}
	if c.PersonalAloneUsernameCollectionID, err = utils.LoadKeyFromEnv("APPWRITE_PERSONAL_ALONE_USERNAME_COLLECTION_ID"); err != nil {
		return nil, err
	}
	if c.PersonalUsernameKey, err = utils.LoadKeyFromEnvInByte("PERSONAL_USERNAME_KEY"); err != nil {
		return nil, err
	}
	if c.PersonalDatabaseID, err = utils.LoadKeyFromEnv("APPWRITE_PERSONAL_DATABASE_ID"); err != nil {
		return nil, err
	}
	return &c, nil
}
