package config

import (
	"chatbasket-apinext/internal/platform/kit"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// PostgresConfig holds the database configuration, ported from chatbasket-api/db/config.go
type PostgresConfig struct {
	DatabaseURL           string
	MaxConns              int32
	MinConns              int32
	MinIdleConns          int32
	MaxConnLifetime       time.Duration
	MaxConnIdleTime       time.Duration
	HealthCheckPeriod     time.Duration
	MaxConnLifetimeJitter time.Duration
}

// Config holds the application-wide configuration
type Config struct {
	Port     string
	Postgres *PostgresConfig
	Appwrite *AppwriteConfig
	Cosmos   *CosmosConfig
	Email    *EmailConfig
	Security *SecurityConfig
	Firebase *FirebaseConfig
}

// SecurityConfig holds domain-level security keys
type SecurityConfig struct {
	AuthSecret          []byte
	PersonalUsernameKey []byte
}

// AppwriteConfig holds Appwrite specific configuration, ported from routes/config.go and appwriteinternal/service.go
type AppwriteConfig struct {
	Endpoint                          string
	ProjectID                         string
	ApiKey                            string
	DatabaseID                        string
	UsersCollectionID                 string
	PostsCollectionID                 string
	CommentsCollectionID              string
	BlockCollectionID                 string
	LikesCollectionID                 string
	FollowCollectionID                string
	RefreshTokensCollectionID         string
	FollowRequestsCollectionID        string
	TempOtpCollectionID               string
	FileUserProfilePicBucketID        string
	PersonalUsersCollectionID         string
	PersonalAloneUsernameCollectionID string
	PersonalDatabaseID                string
	PersonalProfilePicBucketID        string
}

// CosmosConfig holds Cosmos DB configuration, ported from db/cosmos_config.go
type CosmosConfig struct {
	ConnectionString string
	Database         string
	Container        string
}

// EmailConfig holds email relay configuration, ported from utils/emailUtils.go
type EmailConfig struct {
	RelayURL    string
	RelaySecret string
}

// FirebaseConfig holds Firebase specific configuration
type FirebaseConfig struct {
	CredentialsJSON []byte
}

// Load loads the configuration from environment variables and .env files, following the logic in chatbasket-api/app/main.go
func Load() (*Config, error) {
	// Try loading .env files, but don't fail if missing (production uses real env vars)
	// Ported from chatbasket-api/app/main.go
	if err := godotenv.Load(".env"); err != nil {
		if err := godotenv.Load("../.env"); err != nil {
			// Not a fatal error, will use system environment variables
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Load Postgres config, ported from chatbasket-api/db/config.go:LoadPostgresConfig
	dsn := os.Getenv("DATABASE_URL_PG_DEV")
	if dsn == "" {
		// Fallback to DATABASE_URL if PG_DEV is not set
		dsn = os.Getenv("DATABASE_URL")
	}

	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL_PG_DEV or DATABASE_URL is required")
	}

	pgCfg := &PostgresConfig{
		DatabaseURL:           dsn,
		MaxConns:              30,
		MinConns:              2,
		MinIdleConns:          2,
		MaxConnLifetime:       30 * time.Minute,
		MaxConnIdleTime:       2 * time.Minute,
		HealthCheckPeriod:     1 * time.Minute,
		MaxConnLifetimeJitter: 5 * time.Minute,
	}

	// Load Appwrite config, following the logic in chatbasket-api/routes/config.go:loadAppwriteConfig
	awCfg := &AppwriteConfig{}
	var awErr error

	if awCfg.Endpoint, awErr = kit.LoadKeyFromEnv("APPWRITE_ENDPOINT"); awErr != nil {
		return nil, awErr
	}
	if awCfg.ProjectID, awErr = kit.LoadKeyFromEnv("APPWRITE_PROJECT_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.ApiKey, awErr = kit.LoadKeyFromEnv("APPWRITE_API_KEY"); awErr != nil {
		return nil, awErr
	}
	if awCfg.DatabaseID, awErr = kit.LoadKeyFromEnv("APPWRITE_DATABASE_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.UsersCollectionID, awErr = kit.LoadKeyFromEnv("APPWRITE_USERS_COLLECTION_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.PostsCollectionID, awErr = kit.LoadKeyFromEnv("APPWRITE_POSTS_COLLECTION_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.CommentsCollectionID, awErr = kit.LoadKeyFromEnv("APPWRITE_COMMENTS_COLLECTION_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.BlockCollectionID, awErr = kit.LoadKeyFromEnv("APPWRITE_BLOCK_COLLECTION_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.LikesCollectionID, awErr = kit.LoadKeyFromEnv("APPWRITE_LIKES_COLLECTION_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.FollowCollectionID, awErr = kit.LoadKeyFromEnv("APPWRITE_FOLLOW_COLLECTION_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.RefreshTokensCollectionID, awErr = kit.LoadKeyFromEnv("APPWRITE_REFRESH_TOKENS_COLLECTION_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.FollowRequestsCollectionID, awErr = kit.LoadKeyFromEnv("APPWRITE_FOLLOW_REQUESTS_COLLECTION_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.TempOtpCollectionID, awErr = kit.LoadKeyFromEnv("APPWRITE_TEMP_OTP_COLLECTION_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.FileUserProfilePicBucketID, awErr = kit.LoadKeyFromEnv("APPWRITE_FILE_USERPROFILEPIC_BUCKET_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.PersonalUsersCollectionID, awErr = kit.LoadKeyFromEnv("APPWRITE_PERSONAL_USERS_COLLECTION_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.PersonalAloneUsernameCollectionID, awErr = kit.LoadKeyFromEnv("APPWRITE_PERSONAL_ALONE_USERNAME_COLLECTION_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.PersonalDatabaseID, awErr = kit.LoadKeyFromEnv("APPWRITE_PERSONAL_DATABASE_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.PersonalProfilePicBucketID, awErr = kit.LoadKeyFromEnv("APPWRITE_FILE_PERSONAL_USERPROFILEPIC_BUCKET_ID"); awErr != nil {
		return nil, awErr
	}

	// Load Security config
	secCfg := &SecurityConfig{}
	if secCfg.PersonalUsernameKey, awErr = kit.LoadKeyFromEnvInByte("PERSONAL_USERNAME_KEY"); awErr != nil {
		return nil, awErr
	}
	if secret := os.Getenv("AUTH_SECRET"); secret != "" {
		if decoded, err := base64.StdEncoding.DecodeString(secret); err == nil {
			secCfg.AuthSecret = decoded
		}
	}

	// Load Cosmos config
	cosmosCfg := &CosmosConfig{
		ConnectionString: os.Getenv("COSMOS_CONNECTION_STRING"),
		Database:         os.Getenv("COSMOS_DATABASE"),
		Container:        os.Getenv("COSMOS_CONTAINER"),
	}

	// Load Email config
	emailCfg := &EmailConfig{
		RelayURL:    os.Getenv("MAIL_RELAY_URL"),
		RelaySecret: os.Getenv("MAIL_RELAY_SECRET"),
	}

	// Load Firebase config from environment variable only
	fbCfg := &FirebaseConfig{}
	if fbCredsJSON := os.Getenv("FIREBASE_CREDENTIALS_JSON"); fbCredsJSON != "" {
		fbCfg.CredentialsJSON = []byte(fbCredsJSON)
	}

	return &Config{
		Port:     port,
		Postgres: pgCfg,
		Appwrite: awCfg,
		Cosmos:   cosmosCfg,
		Email:    emailCfg,
		Security: secCfg,
		Firebase: fbCfg,
	}, nil
}
