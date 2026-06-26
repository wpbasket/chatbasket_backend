package kit

import (
	"context"
	"sync"
	"time"
)

// MaxConcurrentDeletes is the maximum number of parallel R2 delete operations.
//
// Per Cloudflare R2 docs (https://developers.cloudflare.com/r2/platform/limits/):
//   - 1/sec limit only applies to CONCURRENT WRITES to the SAME key (HTTP 429 if exceeded).
//   - No documented limit on concurrent deletes or different-key operations.
//   - 50/sec limit is for BUCKET management (create/delete/list), not object ops.
//
// Per AWS S3 conventions (R2 is S3-compatible):
//   - Per-prefix throughput limits exist but aren't published.
//   - Practical safe assumption: ~100 deletes/sec per prefix.
//
// Optimization opportunities (NOT implemented here for simplicity):
//   - S3 DeleteObjects API supports up to 1,000 keys per request — could batch deletes
//     into chunks of 1,000 instead of single deletes, achieving ~1-2s per 1,000 files
//     vs the current ~5s per 100 files (50× speedup at scale).
//
// 20 is chosen as a conservative default that:
//   - Gives ~20× throughput vs sequential deletes
//   - Stays well under any plausible R2 limit
//   - Keeps network sockets + memory bounded
//
// For 100+ files/cycle, consider switching to batch DeleteObjects (see comment above).
const MaxConcurrentDeletes = 20

// PerDeleteTimeout caps each individual R2 delete call to prevent slow operations
// from blocking the cleanup loop indefinitely.
const PerDeleteTimeout = 5 * time.Second

// DeleteFunc is the signature for a delete operation that returns nil for success
// or "not found" (idempotent), or a real error for actual failures.
type DeleteFunc func(ctx context.Context, fileID string) error

// DeleteFilesBatch runs deleteFn on each fileID concurrently with a semaphore-bounded
// worker pool. Returns a slice of errors indexed by fileID position — nil means
// R2 confirmed success (or "not found" handled by the underlying idempotent DeleteFile).
//
// Why concurrency: R2 DELETE is the bottleneck (~100ms per call). Sequential
// processing of 100 files = 10 seconds. With 20-way concurrency = ~500ms.
//
// Why semaphore: avoid overwhelming R2 rate limits or local resources.
// MaxConcurrentDeletes caps concurrent in-flight deletes.
//
// Why per-delete timeout: a single slow R2 call shouldn't block the whole batch.
//
// Usage:
//   errors := kit.DeleteFilesBatch(ctx, fileIDs, s.DeleteChatFile)
//   for i, err := range errors {
//       if err != nil {
//           // R2 failed → skip DB delete, retry next cycle
//           continue
//       }
//       // R2 confirmed → safe to delete DB row
//       s.PostgresQueries.DeleteXxx(ctx, batch[i].ID)
//   }
func DeleteFilesBatch(ctx context.Context, fileIDs []string, deleteFn DeleteFunc) []error {
	if len(fileIDs) == 0 {
		return nil
	}

	errors := make([]error, len(fileIDs))
	sem := make(chan struct{}, MaxConcurrentDeletes)
	var wg sync.WaitGroup

	for i, fileID := range fileIDs {
		wg.Add(1)
		sem <- struct{}{} // acquire slot (blocks if MaxConcurrentDeletes are running)

		go func(idx int, fid string) {
			defer wg.Done()
			defer func() { <-sem }() // release slot

			ctxItem, cancel := context.WithTimeout(ctx, PerDeleteTimeout)
			defer cancel()

			errors[idx] = deleteFn(ctxItem, fid)
		}(i, fileID)
	}

	wg.Wait()
	return errors
}
