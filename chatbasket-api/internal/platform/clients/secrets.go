package clients

import "chatbasket-api/internal/platform/config"

// SecretClient holds domain-level security keys used for encryption, hashing, and authentication.
type SecretClient struct {
	AuthSecret          []byte
	PersonalUsernameKey []byte
}

// NewSecretClient creates a new SecretClient with the provided keys.
func NewSecretClient(cfg *config.SecurityConfig) *SecretClient {
	return &SecretClient{
		AuthSecret:          cfg.AuthSecret,
		PersonalUsernameKey: cfg.PersonalUsernameKey,
	}
}

