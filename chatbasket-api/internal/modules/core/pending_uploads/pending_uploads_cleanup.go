package pending_uploads

import (
	"context"
	"log"
	"time"
)

// PendingUploadsCleanupService is the interface for the pending_uploads cleanup job.
// Matches the MessageCleanupService pattern from personal_chat/personal_chat_svc_cleanup.go.
type PendingUploadsCleanupService interface {
	CleanupExpiredPendingUploads(ctx context.Context) error
}

// StartCleanupJob starts a background goroutine that periodically deletes expired
// pending uploads AND their associated R2 objects. Matches the
// StartMessageCleanupJob pattern from personal_chat.
//
// Per spec §6.C:
//
//	The worker queries the pending_uploads table for all records where expires_at is in the past.
//	For each expired upload record:
//	  - The worker parses the account name prefix from the file_id.
//	  - The worker fetches the correct R2 client from the lookup map and requests it to delete the object.
//	  - The worker deletes the tracking row from the pending_uploads table.
func StartCleanupJob(service PendingUploadsCleanupService, interval time.Duration) {
	ticker := time.NewTicker(interval)
	log.Printf("[PendingUploads Cleanup] Starting background job with interval: %v (startup delay: 5m)", interval)

	go func() {
		// 1. Initial delay of 5 minutes before the first run
		time.Sleep(5 * time.Minute)

		ctx := context.Background()
		log.Printf("[PendingUploads Cleanup] Executing initial startup cleanup...")
		if err := service.CleanupExpiredPendingUploads(ctx); err != nil {
			log.Printf("[PendingUploads Cleanup] Initial cleanup failed: %v", err)
		}

		// 2. Regular interval thereafter
		for range ticker.C {
			if err := service.CleanupExpiredPendingUploads(ctx); err != nil {
				log.Printf("[PendingUploads Cleanup] Failed to cleanup expired pending uploads: %v", err)
			} else {
				log.Printf("[PendingUploads Cleanup] Successfully cleaned up expired pending uploads")
			}
		}
	}()
}
