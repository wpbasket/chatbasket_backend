-- +migrate Up

-- ======================================
-- Extend messages table for file attachments
-- ======================================

ALTER TABLE messages ADD COLUMN IF NOT EXISTS file_id TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS file_name TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS file_size BIGINT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS file_mime_type TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS file_token_id TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS file_token_secret TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS file_token_expiry TIMESTAMPTZ;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS thumbnail_file_id TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS thumbnail_token_id TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS thumbnail_token_secret TEXT;

-- Add constraints
ALTER TABLE messages ADD CONSTRAINT messages_file_size_limit 
    CHECK (file_size IS NULL OR file_size <= 104857600); -- 100MB limit

ALTER TABLE messages ADD CONSTRAINT messages_file_type_validation
    CHECK (
        (message_type = 'text' AND file_id IS NULL) OR
        (message_type IN ('image', 'video', 'audio', 'file') AND file_id IS NOT NULL)
    );

-- Add index for file cleanup
CREATE INDEX IF NOT EXISTS idx_messages_file_cleanup
    ON messages(file_id)
    WHERE file_id IS NOT NULL;

-- Add index for expired file tokens
CREATE INDEX IF NOT EXISTS idx_messages_expired_file_tokens
    ON messages(file_token_expiry)
    WHERE file_id IS NOT NULL AND file_token_expiry IS NOT NULL;

-- ======================================
-- End of file messaging extension
-- ======================================
