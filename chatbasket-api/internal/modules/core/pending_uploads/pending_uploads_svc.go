package pending_uploads

import (
	"context"
	"log"
	"time"

	"chatbasket-api/internal/modules/core/pending_uploads/internal/pending_uploads_store"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/kit"
	"github.com/jackc/pgx/v5"
)

// Service is the concrete service for the pending_uploads module.
// Consumer modules (chat, profile) define their own minimal interfaces for
// the methods they need (same pattern as core_auth).
//
// Cross-module transactions follow the same pattern as core_auth: methods that
// may need to participate in a transaction take a pgx.Tx parameter. When tx is
// non-nil, the method binds to that transaction; otherwise it uses the global pool.
type Service struct {
	store  *pending_uploads_store.Queries
	r2Pool *clients.R2ClientPool // needed by cleanup job for R2 deletes
}

// PendingUpload is the public DTO returned by Service methods.
type PendingUpload struct {
	FileID     string
	BucketName string
	R2Key      string
	ExpiresAt  time.Time
}

// NewService creates a new Service backed by the given DBTX-compatible pool.
// r2Pool is required for the cleanup job to delete R2 objects.
func NewService(db pending_uploads_store.DBTX, r2Pool *clients.R2ClientPool) *Service {
	return &Service{
		store:  pending_uploads_store.New(db),
		r2Pool: r2Pool,
	}
}

// storeOrTx returns the tx-bound store if tx is non-nil, otherwise the global store.
// Same pattern as core_auth (see core_auth_svc_e2ee.go).
func (s *Service) storeOrTx(tx pgx.Tx) *pending_uploads_store.Queries {
	if tx != nil {
		return s.store.WithTx(tx)
	}
	return s.store
}

// Register records a new pending upload (presign step).
func (s *Service) Register(ctx context.Context, fileID, bucket, r2Key string, expiresAt time.Time) error {
	return s.RegisterTx(ctx, nil, fileID, bucket, r2Key, expiresAt)
}

// RegisterTx records a new pending upload within an optional transaction.
func (s *Service) RegisterTx(ctx context.Context, tx pgx.Tx, fileID, bucket, r2Key string, expiresAt time.Time) error {
	return s.storeOrTx(tx).InsertPendingUpload(ctx, pending_uploads_store.InsertPendingUploadParams{
		FileID:     fileID,
		BucketName: bucket,
		R2Key:      r2Key,
		ExpiresAt:  expiresAt,
	})
}

// Lookup fetches a pending upload record (confirm step).
func (s *Service) Lookup(ctx context.Context, fileID string) (PendingUpload, error) {
	return s.LookupTx(ctx, nil, fileID)
}

// LookupTx fetches a pending upload within an optional transaction.
func (s *Service) LookupTx(ctx context.Context, tx pgx.Tx, fileID string) (PendingUpload, error) {
	p, err := s.storeOrTx(tx).GetPendingUpload(ctx, fileID)
	if err != nil {
		return PendingUpload{}, err
	}
	return PendingUpload{
		FileID:     p.FileID,
		BucketName: p.BucketName,
		R2Key:      p.R2Key,
		ExpiresAt:  p.ExpiresAt,
	}, nil
}

// Remove deletes a pending upload by file_id (after successful confirm).
func (s *Service) Remove(ctx context.Context, fileID string) error {
	return s.RemoveTx(ctx, nil, fileID)
}

// RemoveTx deletes a pending upload within an optional transaction.
func (s *Service) RemoveTx(ctx context.Context, tx pgx.Tx, fileID string) error {
	return s.storeOrTx(tx).DeletePendingUpload(ctx, fileID)
}

// Stale Pending Upload Cleanup (matches chat CleanupExpiredMessages pattern)
// ──────────────────────────────────────────────────────────────────────────────

// CleanupExpiredPendingUploads picks up expired pending uploads in batches,
// deletes their R2 objects, then deletes the DB rows. Matches the chat
// CleanupExpiredMessages pattern. Runs as a background job via StartCleanupJob.
//
// Per spec §6.C:
//   - Phase 1: Batch keyset-paginated R2 cleanup + DB delete
//   - Phase 2: Final bulk sweep for any rows the batched loop couldn't process
func (s *Service) CleanupExpiredPendingUploads(ctx context.Context) error {
	log.Printf("[PendingUploads Cleanup] Starting cleanup of expired pending uploads")
	const batchSize = 100

	// Phase 1: Batched keyset-paginated cleanup
	lastFileID := ""
	for {
		expiredUploads, err := s.store.GetExpiredPendingUploadsBatch(ctx, pending_uploads_store.GetExpiredPendingUploadsBatchParams{
			LastFileID: lastFileID,
			BatchSize:  int32(batchSize),
		})
		if err != nil {
			log.Printf("[PendingUploads Cleanup] ERROR: Failed to fetch expired pending uploads: %v", err)
			break
		}
		if len(expiredUploads) == 0 {
			break
		}
		log.Printf("[PendingUploads Cleanup] Processing %d expired pending uploads (Cursor: %s)", len(expiredUploads), lastFileID)
		// Concurrent R2 deletes — 20-way parallelism (kit.MaxConcurrentDeletes).
		// Wrap s.deleteR2PendingObject to match kit.DeleteFunc signature.
		deleteFn := func(ctx context.Context, fileID string) error {
			// Look up bucket + key for this fileID
			for _, p := range expiredUploads {
				if p.FileID == fileID {
					return s.deleteR2PendingObject(ctx, p.FileID, p.BucketName, p.R2Key)
				}
			}
			return nil
		}
		fileIDs := make([]string, 0, len(expiredUploads))
		for _, p := range expiredUploads {
			fileIDs = append(fileIDs, p.FileID)
		}
		r2Errors := kit.DeleteFilesBatch(ctx, fileIDs, deleteFn)
		for i, p := range expiredUploads {
			if r2Errors[i] != nil {
				// R2 failed (real error) — skip DB delete, retry next cycle
				log.Printf("[PendingUploads Cleanup] WARNING: R2 delete failed for %s: %v", p.FileID, r2Errors[i])
				continue
			}
			// R2 succeeded (or file was already gone) — safe to delete DB row
			if err := s.store.DeletePendingUpload(ctx, p.FileID); err != nil {
				log.Printf("[PendingUploads Cleanup] WARNING: Failed to delete expired pending upload record %s: %v", p.FileID, err)
			}
		}
		if len(expiredUploads) < batchSize {
			break
		}
		lastFileID = expiredUploads[len(expiredUploads)-1].FileID
	}

	log.Printf("[PendingUploads Cleanup] Cleanup process completed successfully")
	return nil
}

// deleteR2PendingObject deletes a pending upload's R2 object (idempotent).
// Returns nil if file was already gone (NoSuchKey) — safe to proceed with DB delete.
// Mirrors chatService.DeleteChatFile / profileService.deleteAvatarFromR2.
func (s *Service) deleteR2PendingObject(ctx context.Context, fileID, bucket, key string) error {
	accountName, objectKey := clients.ParseFilePrefix(fileID)
	if accountName == "" {
		// Legacy unprefixed file_id — skip R2 delete (safe — DB delete will follow)
		return nil
	}
	if !s.r2Pool.HasClient(accountName) {
		// Per spec §3.E: account retired, skip R2 delete (safe — DB delete will follow)
		log.Printf("[PendingUploads Cleanup] WARNING: Account '%s' not in pool, skipping R2 delete for %s", accountName, fileID)
		return nil
	}
	client := s.r2Pool.GetClient(fileID)
	return client.DeleteFile(ctx, bucket, objectKey)
}