package services

import (
	"chatbasket/utils"
	"context"
	"fmt"
	"log"

	"firebase.google.com/go/v4/messaging"
)

// FCMService handles Firebase Cloud Messaging operations
type FCMService struct {
	client *messaging.Client
}

// NewFCMService creates a new FCM service instance
func NewFCMService() (*FCMService, error) {
	client, err := utils.GetMessagingClient()
	if err != nil {
		return nil, err
	}
	return &FCMService{client: client}, nil
}

// SendNotificationToToken sends a notification to a single FCM token
func (f *FCMService) SendNotificationToToken(ctx context.Context, token, title, body string, data map[string]string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("FCM token is required")
	}

	message := &messaging.Message{
		Token: token,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
	}

	// Send the message
	response, err := f.client.Send(ctx, message)
	if err != nil {
		log.Printf("❌ Failed to send FCM notification to token %s: %v", token, err)
		return "", fmt.Errorf("failed to send FCM notification: %w", err)
	}

	log.Printf("✅ Successfully sent FCM notification to token %s, message ID: %s", token, response)
	return response, nil
}

// SendNotificationToMultipleTokens sends the same notification to multiple FCM tokens
func (f *FCMService) SendNotificationToMultipleTokens(ctx context.Context, tokens []string, title, body string, data map[string]string) (*messaging.BatchResponse, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("at least one FCM token is required")
	}

	if len(tokens) > 500 {
		return nil, fmt.Errorf("cannot send to more than 500 tokens at once")
	}

	multicastMessage := &messaging.MulticastMessage{
		Tokens: tokens,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
	}

	// Send the message to multiple tokens
	response, err := f.client.SendEachForMulticast(ctx, multicastMessage)
	if err != nil {
		log.Printf("❌ Failed to send FCM multicast notification: %v", err)
		return nil, fmt.Errorf("failed to send FCM multicast notification: %w", err)
	}

	log.Printf("✅ Successfully sent FCM multicast notification. Success: %d, Failure: %d", response.SuccessCount, response.FailureCount)

	// Log any failures
	if response.FailureCount > 0 {
		for idx, resp := range response.Responses {
			if !resp.Success {
				log.Printf("⚠️  Failed to send to token %s: %v", tokens[idx], resp.Error)
			}
		}
	}

	return response, nil
}

// SendDataMessage sends a data-only message (no notification) to a single FCM token
func (f *FCMService) SendDataMessage(ctx context.Context, token string, data map[string]string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("FCM token is required")
	}

	if len(data) == 0 {
		return "", fmt.Errorf("data payload is required")
	}

	message := &messaging.Message{
		Token: token,
		Data:  data,
	}

	// Send the message
	response, err := f.client.Send(ctx, message)
	if err != nil {
		log.Printf("❌ Failed to send FCM data message to token %s: %v", token, err)
		return "", fmt.Errorf("failed to send FCM data message: %w", err)
	}

	log.Printf("✅ Successfully sent FCM data message to token %s, message ID: %s", token, response)
	return response, nil
}

// SendWithPlatformConfig sends a notification with platform-specific configurations
func (f *FCMService) SendWithPlatformConfig(ctx context.Context, token, title, body string, data map[string]string, androidPriority string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("FCM token is required")
	}

	message := &messaging.Message{
		Token: token,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
		Android: &messaging.AndroidConfig{
			Priority: androidPriority, // "normal" or "high"
			Notification: &messaging.AndroidNotification{
				Sound: "default",
			},
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound: "default",
				},
			},
		},
	}

	// Send the message
	response, err := f.client.Send(ctx, message)
	if err != nil {
		log.Printf("❌ Failed to send FCM notification with platform config to token %s: %v", token, err)
		return "", fmt.Errorf("failed to send FCM notification: %w", err)
	}

	log.Printf("✅ Successfully sent FCM notification with platform config to token %s, message ID: %s", token, response)
	return response, nil
}
