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

	log.Printf("[Message Cleanup] Starting background job with interval: %v", interval)

	go func() {
		for range ticker.C {
			ctx := context.Background()
			err := service.CleanupExpiredMessages(ctx)
			if err != nil {
				log.Printf("[Message Cleanup] Failed to cleanup expired messages: %v", err)
			} else {
				log.Printf("[Message Cleanup] Successfully cleaned up expired messages")
			}
		}
	}()
}
