-- +migrate Down

-- Drop pending_uploads
DROP TRIGGER IF EXISTS pending_uploads_timestamps_trigger ON pending_uploads; -- Timestamp trigger
DROP INDEX IF EXISTS idx_pending_uploads_expires_at;                            -- Cleanup sweeper index
DROP TABLE IF EXISTS pending_uploads CASCADE;                                   -- Also drops PK constraint
