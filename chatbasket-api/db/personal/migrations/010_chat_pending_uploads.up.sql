-- +migrate Up
-- ======================================
-- Table: pending_uploads
--        Tracks authorized-but-unconfirmed R2 uploads initiated via presigned URLs.
--        Allows transactional confirmation and periodic cleanup of abandoned uploads.
-- ======================================
CREATE TABLE IF NOT EXISTS pending_uploads (
    file_id     TEXT         PRIMARY KEY, -- Prefixed ID: "alpha:782a2cd1-12ef-4da3-bb45-31f0a067e20a"
    bucket_name TEXT         NOT NULL,    -- Target R2 bucket name
    r2_key      TEXT         NOT NULL,    -- Object key within the bucket
    expires_at  TIMESTAMPTZ  NOT NULL,    -- 2-hour TTL; swept by background worker
    created_at  TIMESTAMPTZ  NOT NULL,    -- When the presigned URL was issued
    updated_at  TIMESTAMPTZ  NOT NULL     -- Auto-updated by set_timestamps() trigger
);

-- Index for the periodic cleanup sweeper (every 15-30 minutes)
CREATE INDEX IF NOT EXISTS idx_pending_uploads_expires_at
    ON pending_uploads (expires_at);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS pending_uploads_timestamps_trigger ON pending_uploads;

-- Attach auto timestamp trigger (matches pattern of all other tables)
CREATE TRIGGER pending_uploads_timestamps_trigger
BEFORE INSERT OR UPDATE ON pending_uploads
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- ======================================
-- End of pending_uploads table section
-- ======================================
