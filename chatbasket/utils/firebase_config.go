package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	firebase "firebase.google.com/go/v4"
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

// InitializeFirebase initializes the Firebase app and messaging client
// This should be called once during application startup
func InitializeFirebase(ctx context.Context) error {
	once.Do(func() {
		var serviceAccountBytes []byte
		var err error

		// Hardcoded service account filename
		filename := "chatbasket-207b6-firebase-adminsdk-fbsvc-360aca9832.json"

		// Try file first (for local development)
		serviceAccountBytes, err = os.ReadFile(filename)
		if err != nil {
			// Try parent directory
			serviceAccountBytes, err = os.ReadFile("../" + filename)
			if err != nil {
				// File not found, try environment variable (for Heroku/production)
				firebaseCredsJSON := os.Getenv("FIREBASE_CREDENTIALS_JSON")
				
				if firebaseCredsJSON == "" {
					initErr = fmt.Errorf("Firebase credentials not found: no file '%s' and no FIREBASE_CREDENTIALS_JSON env var", filename)
					return
				}
				
				serviceAccountBytes = []byte(firebaseCredsJSON)
				log.Println("📍 Using Firebase credentials from environment variable")
			} else {
				log.Println("📍 Using Firebase credentials from file (parent directory)")
			}
		} else {
			log.Println("📍 Using Firebase credentials from file (current directory)")
		}

		// Validate JSON format
		var jsonCheck map[string]interface{}
		if err := json.Unmarshal(serviceAccountBytes, &jsonCheck); err != nil {
			initErr = fmt.Errorf("invalid Firebase credentials JSON: %w", err)
			return
		}

		// Parse credentials explicitly to avoid deprecation warnings
		// Using cloud-platform scope which covers Firebase services
		creds, err := google.CredentialsFromJSON(ctx, serviceAccountBytes, "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			initErr = fmt.Errorf("failed to parse service account credentials: %w", err)
			return
		}

		// Initialize Firebase app with credentials
		opt := option.WithCredentials(creds)
		firebaseApp, initErr = firebase.NewApp(ctx, nil, opt)
		if initErr != nil {
			initErr = fmt.Errorf("failed to initialize Firebase app: %w", initErr)
			return
		}

		// Get messaging client
		messagingClient, initErr = firebaseApp.Messaging(ctx)
		if initErr != nil {
			initErr = fmt.Errorf("failed to get Firebase messaging client: %w", initErr)
			return
		}

	})

	return initErr
}

// GetMessagingClient returns the Firebase messaging client instance
// Must call InitializeFirebase first
func GetMessagingClient() (*messaging.Client, error) {
	if messagingClient == nil {
		return nil, fmt.Errorf("Firebase not initialized. Call InitializeFirebase first")
	}
	return messagingClient, nil
}

// GetFirebaseApp returns the Firebase app instance
// Must call InitializeFirebase first
func GetFirebaseApp() (*firebase.App, error) {
	if firebaseApp == nil {
		return nil, fmt.Errorf("Firebase not initialized. Call InitializeFirebase first")
	}
	return firebaseApp, nil
}