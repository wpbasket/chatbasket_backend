-- +migrate Down

-- ======================================
-- Rollback: Remove file messaging columns
-- ======================================

-- Drop indexes
DROP INDEX IF EXISTS idx_messages_expired_file_tokens;
DROP INDEX IF EXISTS idx_messages_file_cleanup;

-- Drop constraints
ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_file_type_validation;
ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_file_size_limit;

-- Drop columns
ALTER TABLE messages DROP COLUMN IF EXISTS thumbnail_token_secret;
ALTER TABLE messages DROP COLUMN IF EXISTS thumbnail_token_id;
ALTER TABLE messages DROP COLUMN IF EXISTS thumbnail_file_id;
ALTER TABLE messages DROP COLUMN IF EXISTS file_token_expiry;
ALTER TABLE messages DROP COLUMN IF EXISTS file_token_secret;
ALTER TABLE messages DROP COLUMN IF EXISTS file_token_id;
ALTER TABLE messages DROP COLUMN IF EXISTS file_mime_type;
ALTER TABLE messages DROP COLUMN IF EXISTS file_size;
ALTER TABLE messages DROP COLUMN IF EXISTS file_name;
ALTER TABLE messages DROP COLUMN IF EXISTS file_id;

-- ======================================
-- End of rollback
-- ======================================
