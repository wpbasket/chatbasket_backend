package personalutils

import (
	"context"
	"log"
	"time"
)

type MessageCleanupService interface {
	CleanupExpiredMessages(ctx context.Context) error
}

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
