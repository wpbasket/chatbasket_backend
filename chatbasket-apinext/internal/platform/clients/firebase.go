package clients

// NOTE: This file matches the original Firebase implementation logic.
// Key changes from original:
// 1. Uses google.CredentialsFromJSONWithType instead of deprecated CredentialsFromJSON.
// 2. Credentials loaded from environment variable only (no file reading).
// 3. Added explicit JSON validation for credentials.
// 4. Uses cloud-platform OAuth scope for Firebase services.

import (
	"chatbasket-apinext/internal/platform/config"
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

var (
	firebaseApp     *firebase.App
	messagingClient *messaging.Client
	once            sync.Once
	initErr         error
)

// InitializeFirebase initializes the Firebase app and messaging client.
// It uses pre-loaded credentials from the config, following the latest
// secure patterns recommended by the Google API Client Libraries.
func InitializeFirebase(ctx context.Context, cfg *config.FirebaseConfig) error {
	once.Do(func() {
		if len(cfg.CredentialsJSON) == 0 {
			initErr = fmt.Errorf("Firebase credentials not found in configuration")
			return
		}

		// Security Best Practice: Explicitly validate JSON format before passing to option helpers.
		// This mitigates risks associated with untrusted credential sources.
		var jsonCheck struct{}
		if err := json.Unmarshal(cfg.CredentialsJSON, &jsonCheck); err != nil {
			initErr = fmt.Errorf("invalid Firebase credentials JSON: %w", err)
			return
		}

		// Parse credentials explicitly to ensure correct OAuth scope and credential type.
		// Using CredentialsFromJSONWithType to avoid deprecated CredentialsFromJSON.
		creds, err := google.CredentialsFromJSONWithType(ctx, cfg.CredentialsJSON, google.ServiceAccount, "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			initErr = fmt.Errorf("failed to parse service account credentials: %w", err)
			return
		}

		// Verify credentials by attempting to fetch a token.
		if _, err := creds.TokenSource.Token(); err != nil {
			initErr = fmt.Errorf("failed to verify Firebase credentials (network/auth error): %w", err)
			return
		}

		// Initialize Firebase app with credentials
		opt := option.WithCredentials(creds)
		firebaseApp, initErr = firebase.NewApp(ctx, nil, opt)
		if initErr != nil {
			initErr = fmt.Errorf("failed to initialize Firebase app: %w", initErr)
			return
		}

		// Get messaging client instance
		messagingClient, initErr = firebaseApp.Messaging(ctx)
		if initErr != nil {
			initErr = fmt.Errorf("failed to get Firebase messaging client: %w", initErr)
			return
		}
	})

	return initErr
}

// GetMessagingClient returns the Firebase messaging client instance.
// Must call InitializeFirebase first.
func GetMessagingClient() (*messaging.Client, error) {
	if messagingClient == nil {
		return nil, fmt.Errorf("Firebase messaging client not initialized")
	}
	return messagingClient, nil
}

// GetFirebaseApp returns the Firebase app instance.
// Must call InitializeFirebase first.
func GetFirebaseApp() (*firebase.App, error) {
	if firebaseApp == nil {
		return nil, fmt.Errorf("Firebase app not initialized")
	}
	return firebaseApp, nil
}
