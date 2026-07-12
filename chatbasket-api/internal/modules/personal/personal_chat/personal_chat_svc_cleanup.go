package personal_chat

import (
	"context"
	"log"
	"time"
)

// MessageCleanupService is the interface for the cleanup job.
type MessageCleanupService interface {
	CleanupExpiredMessages(ctx context.Context) error
	CleanupDatabaseOnly(ctx context.Context) error
}

// StartMessageCleanupJob starts a background goroutine that periodically cleans up expired messages.
func StartMessageCleanupJob(service MessageCleanupService, interval time.Duration) {
	ticker := time.NewTicker(interval)
	log.Printf("[Message Cleanup] Starting background job with interval: %v (startup delay: 5m)", interval)

	go func() {
		// 1. Initial delay of 5 minutes before the first run
		time.Sleep(5 * time.Minute)

		ctx := context.Background()
		log.Printf("[Message Cleanup] Executing initial startup cleanup...")
		if err := service.CleanupExpiredMessages(ctx); err != nil {
			log.Printf("[Message Cleanup] Initial cleanup failed: %v", err)
		}

		// 2. Regular interval thereafter
		for range ticker.C {
			err := service.CleanupExpiredMessages(ctx)
			if err != nil {
				log.Printf("[Message Cleanup] Failed to cleanup expired messages: %v", err)
			} else {
				log.Printf("[Message Cleanup] Successfully cleaned up expired messages")
			}
		}
	}()
}

// StartDatabaseCleanupJob starts a background goroutine that periodically cleans up database records without files.
func StartDatabaseCleanupJob(service MessageCleanupService, interval time.Duration) {
	ticker := time.NewTicker(interval)
	log.Printf("[Database Cleanup] Starting background job with interval: %v (startup delay: 5m)", interval)

	go func() {
		// 1. Initial delay of 5 minutes before the first run
		time.Sleep(5 * time.Minute)

		ctx := context.Background()
		log.Printf("[Database Cleanup] Executing initial startup cleanup...")
		if err := service.CleanupDatabaseOnly(ctx); err != nil {
			log.Printf("[Database Cleanup] Initial cleanup failed: %v", err)
		}

		// 2. Regular interval thereafter
		for range ticker.C {
			err := service.CleanupDatabaseOnly(ctx)
			if err != nil {
				log.Printf("[Database Cleanup] Failed to cleanup database only: %v", err)
			} else {
				log.Printf("[Database Cleanup] Successfully cleaned up database only")
			}
		}
	}()
}
