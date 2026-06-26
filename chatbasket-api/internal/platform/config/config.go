package config

import (
	"chatbasket-api/internal/platform/kit"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// PostgresConfig holds the database configuration.
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

// Config holds the application-wide configuration.
type Config struct {
	Port       string
	CORSOrigin string
	Postgres   *PostgresConfig
	Cosmos     *CosmosConfig
	Email      *EmailConfig
	Security   *SecurityConfig
	Firebase   *FirebaseConfig
	R2         *R2PoolConfig
}

// R2AccountConfig holds credentials for a single Cloudflare R2 account.
type R2AccountConfig struct {
	Name             string `json:"name"`
	AccountID        string `json:"account_id"`
	AccessKeyID      string `json:"access_key_id"`
	SecretAccessKey  string `json:"secret_access_key"`
	ChatFilesBucket  string `json:"chat_files_bucket"`
	ProfilePicBucket string `json:"profile_pic_bucket"`
}

// R2PoolConfig holds the R2 client pool configuration.
type R2PoolConfig struct {
	Accounts              []R2AccountConfig
	PrimaryChatAccount    string
	PrimaryProfileAccount string
}

// SecurityConfig holds domain-level security keys.
type SecurityConfig struct {
	AuthSecret          []byte
	PersonalUsernameKey []byte
	PersonalContactKey  []byte
}

// CosmosConfig holds Cosmos DB configuration.
type CosmosConfig struct {
	ConnectionString string
	Database         string
	Container        string
}

// EmailConfig holds email relay configuration.
type EmailConfig struct {
	RelayURL    string
	RelaySecret string
}

// FirebaseConfig holds Firebase configuration.
type FirebaseConfig struct {
	CredentialsJSON []byte
}

// Load loads configuration from environment variables and .env files.
func Load() (*Config, error) {
	if err := godotenv.Load(".env"); err != nil {
		_ = godotenv.Load("../.env")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	corsOrigin := os.Getenv("CORS_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "https://chatbasket.live"
	}

	dbSelector := os.Getenv("DB_SELECTOR")
	if dbSelector == "" {
		return nil, fmt.Errorf("DB_SELECTOR is required (e.g. DATABASE_URL_PG_CB or DATABASE_URL_PG_DEV)")
	}

	dsn := os.Getenv(dbSelector)
	if dsn == "" {
		return nil, fmt.Errorf("%s is required", dbSelector)
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

	secCfg := &SecurityConfig{}
	var secErr error
	if secCfg.PersonalUsernameKey, secErr = kit.LoadKeyFromEnvInByte("PERSONAL_USERNAME_KEY"); secErr != nil {
		return nil, secErr
	}
	if secCfg.PersonalContactKey, secErr = kit.LoadKeyFromEnvInByte("PERSONAL_CONTACT_KEY"); secErr != nil {
		return nil, secErr
	}
	if secret := os.Getenv("AUTH_SECRET"); secret != "" {
		if decoded, err := base64.StdEncoding.DecodeString(secret); err == nil {
			secCfg.AuthSecret = decoded
		}
	}

	cosmosCfg := &CosmosConfig{
		ConnectionString: os.Getenv("COSMOS_CONNECTION_STRING"),
		Database:         os.Getenv("COSMOS_DATABASE"),
		Container:        os.Getenv("COSMOS_CONTAINER"),
	}

	emailCfg := &EmailConfig{
		RelayURL:    os.Getenv("MAIL_RELAY_URL"),
		RelaySecret: os.Getenv("MAIL_RELAY_SECRET"),
	}

	fbCfg := &FirebaseConfig{}
	if fbCredsJSON := os.Getenv("FIREBASE_CREDENTIALS_JSON"); fbCredsJSON != "" {
		fbCfg.CredentialsJSON = []byte(fbCredsJSON)
	}

	r2Cfg, err := loadR2PoolConfig()
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:       port,
		CORSOrigin: corsOrigin,
		Postgres:   pgCfg,
		Cosmos:     cosmosCfg,
		Email:      emailCfg,
		Security:   secCfg,
		Firebase:   fbCfg,
		R2:         r2Cfg,
	}, nil
}

// loadR2PoolConfig loads R2 accounts from R2_ACCOUNTS_JSON (JSON array).
func loadR2PoolConfig() (*R2PoolConfig, error) {
	jsonStr := os.Getenv("R2_ACCOUNTS_JSON")
	if jsonStr == "" {
		return nil, fmt.Errorf("R2_ACCOUNTS_JSON is required (must be a JSON array of R2 account configs)")
	}

	var accounts []R2AccountConfig
	if err := json.Unmarshal([]byte(jsonStr), &accounts); err != nil {
		return nil, fmt.Errorf("failed to parse R2_ACCOUNTS_JSON: %w", err)
	}

	if len(accounts) == 0 {
		return nil, fmt.Errorf("R2_ACCOUNTS_JSON must contain at least one account")
	}

	seen := make(map[string]bool, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc.Name == "" {
			return nil, fmt.Errorf("R2_ACCOUNTS_JSON[%d]: 'name' is required", i)
		}
		if seen[acc.Name] {
			return nil, fmt.Errorf("R2_ACCOUNTS_JSON: duplicate account name '%s'", acc.Name)
		}
		seen[acc.Name] = true

		if acc.AccountID == "" || acc.AccessKeyID == "" || acc.SecretAccessKey == "" {
			return nil, fmt.Errorf("R2_ACCOUNTS_JSON[%d] (%s): account_id, access_key_id, and secret_access_key are required", i, acc.Name)
		}
		if acc.ChatFilesBucket == "" && acc.ProfilePicBucket == "" {
			return nil, fmt.Errorf("R2_ACCOUNTS_JSON[%d] (%s): at least one of chat_files_bucket or profile_pic_bucket is required", i, acc.Name)
		}
	}

	primaryChat := os.Getenv("R2_PRIMARY_CHAT_ACCOUNT")
	if primaryChat == "" {
		primaryChat = accounts[0].Name
	} else if !seen[primaryChat] {
		return nil, fmt.Errorf("R2_PRIMARY_CHAT_ACCOUNT='%s' not found in R2_ACCOUNTS_JSON", primaryChat)
	}

	primaryProfile := os.Getenv("R2_PRIMARY_PROFILE_ACCOUNT")
	if primaryProfile == "" {
		primaryProfile = accounts[0].Name
	} else if !seen[primaryProfile] {
		return nil, fmt.Errorf("R2_PRIMARY_PROFILE_ACCOUNT='%s' not found in R2_ACCOUNTS_JSON", primaryProfile)
	}

	return &R2PoolConfig{
		Accounts:              accounts,
		PrimaryChatAccount:    primaryChat,
		PrimaryProfileAccount: primaryProfile,
	}, nil
}
