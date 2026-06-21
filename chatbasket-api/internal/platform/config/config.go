package config

import (
	"chatbasket-api/internal/platform/kit"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// PostgresConfig holds the database configuration, ported from chatbasket-api/db/config.go
type PostgresConfig struct {
	DatabaseURL           string
	DatabaseURLTesting    string
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
	PersonalContactKey  []byte
}

// AppwriteConfig holds Appwrite specific configuration, ported from routes/config.go and appwriteinternal/service.go
type AppwriteConfig struct {
	Endpoint                   string
	ProjectID                  string
	ApiKey                     string
	PersonalProfilePicBucketID string
	PersonalChatFilesBucketID  string
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
	// TOGGLE HERE: Swap comments below to easily switch between Production and Development locally
	dsn := os.Getenv("DATABASE_URL_PG_CB")
	// dsn := os.Getenv("DATABASE_URL_PG_DEV")

	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL_PG_CB is required")
	}

	pgCfg := &PostgresConfig{
		DatabaseURL:           dsn,
		DatabaseURLTesting:    os.Getenv("DATABASE_URL_PG_TESTING"),
		MaxConns:              30,
		MinConns:              2,
		MinIdleConns:          2,
		MaxConnLifetime:       kit.DefaultPostgresMaxConnLifetime,

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
	if awCfg.PersonalProfilePicBucketID, awErr = kit.LoadKeyFromEnv("APPWRITE_FILE_PERSONAL_USERPROFILEPIC_BUCKET_ID"); awErr != nil {
		return nil, awErr
	}
	if awCfg.PersonalChatFilesBucketID, awErr = kit.LoadKeyFromEnv("APPWRITE_PERSONAL_CHAT_FILES_BUCKET_ID"); awErr != nil {
		return nil, awErr
	}

	// Load Security config
	secCfg := &SecurityConfig{}
	if secCfg.PersonalUsernameKey, awErr = kit.LoadKeyFromEnvInByte("PERSONAL_USERNAME_KEY"); awErr != nil {
		return nil, awErr
	}
	if secCfg.PersonalContactKey, awErr = kit.LoadKeyFromEnvInByte("PERSONAL_CONTACT_KEY"); awErr != nil {
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

