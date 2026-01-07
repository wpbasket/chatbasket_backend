package utils

import (
	"context"
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
		// Hardcoded service account path
		filename := "chatbasket-207b6-firebase-adminsdk-fbsvc-360aca9832.json"

		// Try current directory first
		serviceAccountBytes, err := os.ReadFile(filename)
		if err != nil {
			// Try parent directory
			serviceAccountBytes, err = os.ReadFile("../" + filename)
			if err != nil {
				// Try absolute path if we can guess, or just fail
				initErr = fmt.Errorf("failed to read service account file (checked . and ..): %w", err)
				return
			}
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

		log.Println("✅ Firebase initialized successfully")
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
