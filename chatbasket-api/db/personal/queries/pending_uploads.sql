-- ===========================================
-- Pending Upload Operations (R2 lifecycle)
-- ===========================================

-- name: InsertPendingUpload :exec
-- Registers a new presigned upload in the tracking table.
-- created_at/updated_at are auto-populated by the set_timestamps() trigger.
-- ON CONFLICT DO NOTHING: account deletion re-registers already-tracked files
-- (e.g. unfinished presigns) — those rows are exactly what the async R2
-- cleanup + sweeper want, so a duplicate must not abort the deletion tx.
INSERT INTO pending_uploads (file_id, bucket_name, r2_key, expires_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (file_id) DO NOTHING;

-- name: InsertPendingUploadsBatch :exec
-- ON CONFLICT DO NOTHING: see InsertPendingUpload — deletion re-registers
-- already-tracked files in batches; duplicates must be skipped, not fail the tx.
-- Deletion rows are intentionally born-expired (expires_at = now()) so the
-- background sweeper retries them if the async R2 goroutine dies.
INSERT INTO pending_uploads (file_id, bucket_name, r2_key, expires_at)
SELECT * FROM ROWS FROM (
    unnest(sqlc.arg(file_ids)::text[]),
    unnest(sqlc.arg(bucket_names)::text[]),
    unnest(sqlc.arg(r2_keys)::text[]),
    unnest(sqlc.arg(expires_ats)::timestamptz[])
) AS t(file_id, bucket_name, r2_key, expires_at)
ON CONFLICT (file_id) DO NOTHING;


-- name: GetPendingUpload :one
-- Fetches a pending upload by file_id. Returns sql.ErrNoRows if not found.
SELECT file_id, bucket_name, r2_key, expires_at
FROM pending_uploads
WHERE file_id = $1;

-- name: DeletePendingUpload :exec
-- Removes a pending upload by file_id (after successful confirm).
DELETE FROM pending_uploads
WHERE file_id = $1;

-- name: GetExpiredPendingUploadsBatch :many
-- Fetches a batch of expired pending uploads. Uses keyset pagination on file_id (text)
-- for efficient cleanup of large tables.
SELECT file_id, bucket_name, r2_key, expires_at
FROM pending_uploads
WHERE expires_at < now()
  AND file_id > sqlc.arg(last_file_id)
ORDER BY file_id ASC
LIMIT sqlc.arg(batch_size);
