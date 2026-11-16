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
	store            *pending_uploads_store.Queries
	maintenanceStore *pending_uploads_store.Queries
	r2Pool           *clients.R2ClientPool // needed by cleanup job for R2 deletes
}

// PendingUpload is the public DTO returned by Service methods.
type PendingUpload struct {
	FileID     string
	BucketName string
	R2Key      string
	ExpiresAt  time.Time
}

// NewService creates a new Service backed by the given DBTX-compatible pools.
// r2Pool is required for the cleanup job to delete R2 objects.
func NewService(db pending_uploads_store.DBTX, maintenanceDB pending_uploads_store.DBTX, r2Pool *clients.R2ClientPool) *Service {
	return &Service{
		store:            pending_uploads_store.New(db),
		maintenanceStore: pending_uploads_store.New(maintenanceDB),
		r2Pool:           r2Pool,
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

// RegisterBatch records multiple pending uploads in a single batch query.
func (s *Service) RegisterBatch(ctx context.Context, fileIDs, buckets, r2Keys []string, expiresAt time.Time) error {
	return s.RegisterBatchTx(ctx, nil, fileIDs, buckets, r2Keys, expiresAt)
}

// RegisterBatchTx records multiple pending uploads in a single batch query within an optional transaction.
func (s *Service) RegisterBatchTx(ctx context.Context, tx pgx.Tx, fileIDs, buckets, r2Keys []string, expiresAt time.Time) error {
	if len(fileIDs) == 0 {
		return nil
	}
	expiresAts := make([]time.Time, len(fileIDs))
	for i := range expiresAts {
		expiresAts[i] = expiresAt
	}
	return s.storeOrTx(tx).InsertPendingUploadsBatch(ctx, pending_uploads_store.InsertPendingUploadsBatchParams{
		FileIds:     fileIDs,
		BucketNames: buckets,
		R2Keys:      r2Keys,
		ExpiresAts:  expiresAts,
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

const (
	pendingUploadsCleanupTotalBudget = 20 * time.Minute
	pendingUploadsBatchSize          = 100
	pendingUploadsBatchTimeout       = 2 * time.Minute
)

// CleanupExpiredPendingUploads picks up expired pending uploads in batches,
// deletes their R2 objects, then deletes the DB rows. Runs as a background job via StartCleanupJob.
// It follows a budget pattern over a 20-minute interval:
//   - Soft Active Work Limit (85% / 17 minutes): Stops initiating new batches once elapsed time reaches 85% of the budget.
//   - Ongoing Batch Completion: In-flight R2 and DB work is not cancelled by the job budget,
//     but each batch has a 2-minute hard ceiling so a hung call cannot park the worker forever.
//     The ceiling fits inside the 15% (3 minutes) safety buffer.
//   - Early Exit: Terminates immediately once all expired uploads are cleared (fewer than batchSize returned).
func (s *Service) CleanupExpiredPendingUploads(ctx context.Context) error {
	budget := pendingUploadsCleanupTotalBudget
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d > 0 && d < budget {
			budget = d
		}
	}

	log.Printf("[PendingUploads Cleanup] Starting cleanup of expired pending uploads (budget: %v)", budget)
	start := time.Now()
	workBudget := (budget * 85) / 100

	lastFileID := ""
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Soft active work limit: Do not trigger a new batch if 85% work budget has been reached
		if time.Since(start) >= workBudget {
			log.Printf("[PendingUploads Cleanup] 85%% work budget reached (%v elapsed >= %v work budget), stopping before new batch", time.Since(start), workBudget)
			break
		}

		// Ongoing batch must run to completion without being cancelled mid-batch,
		// but it still needs a hard ceiling so a hung database call cannot park
		// the cleanup goroutine forever.
		batchCtx, batchCancel := context.WithTimeout(context.WithoutCancel(ctx), pendingUploadsBatchTimeout)
		expiredUploads, err := s.maintenanceStore.GetExpiredPendingUploadsBatch(batchCtx, pending_uploads_store.GetExpiredPendingUploadsBatchParams{
			LastFileID: lastFileID,
			BatchSize:  int32(pendingUploadsBatchSize),
		})
		if err != nil {
			batchCancel()
			log.Printf("[PendingUploads Cleanup] ERROR: Failed to fetch expired pending uploads: %v", err)
			break
		}
		if len(expiredUploads) == 0 {
			batchCancel()
			break
		}
		log.Printf("[PendingUploads Cleanup] Processing %d expired pending uploads (Cursor: %s)", len(expiredUploads), lastFileID)

		// Concurrent R2 deletes — 20-way parallelism (kit.MaxConcurrentDeletes).
		// Note: The ongoing batch runs to completion without being force-killed by the budget.
		deleteFn := func(ctx context.Context, fileID string) error {
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
		r2Errors := kit.DeleteFilesBatch(batchCtx, fileIDs, deleteFn)
		for i, p := range expiredUploads {
			if r2Errors[i] != nil {
				log.Printf("[PendingUploads Cleanup] WARNING: R2 delete failed for %s: %v", p.FileID, r2Errors[i])
				continue
			}
			if err := s.maintenanceStore.DeletePendingUpload(batchCtx, p.FileID); err != nil {
				log.Printf("[PendingUploads Cleanup] WARNING: Failed to delete expired pending upload record %s: %v", p.FileID, err)
			}
		}

		// The batch is complete; release its deadline before deciding whether
		// to start another batch.
		batchCancel()

		// If fewer rows than batchSize were returned, all expired uploads are cleared early.
		if len(expiredUploads) < pendingUploadsBatchSize {
			break
		}
		lastFileID = expiredUploads[len(expiredUploads)-1].FileID

		// If 85% work budget is reached after completing this batch, do not start another batch
		if time.Since(start) >= workBudget {
			log.Printf("[PendingUploads Cleanup] 85%% work budget reached after batch completion (%v elapsed >= %v work budget), stopping", time.Since(start), workBudget)
			break
		}
	}

	log.Printf("[PendingUploads Cleanup] Cleanup process completed (elapsed: %v)", time.Since(start))
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
		// Account retired: skip R2 delete (safe — DB delete will follow)
		log.Printf("[PendingUploads Cleanup] WARNING: Account '%s' not in pool, skipping R2 delete for %s", accountName, fileID)
		return nil
	}
	client := s.r2Pool.GetClient(fileID)
	return client.DeleteFile(ctx, bucket, objectKey)
}
