-- ===========================================
-- Pending Upload Operations (R2 lifecycle)
-- ===========================================

-- name: InsertPendingUpload :exec
-- Registers a new presigned upload in the tracking table.
-- created_at/updated_at are auto-populated by the set_timestamps() trigger.
INSERT INTO pending_uploads (file_id, bucket_name, r2_key, expires_at)
VALUES ($1, $2, $3, $4);

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
