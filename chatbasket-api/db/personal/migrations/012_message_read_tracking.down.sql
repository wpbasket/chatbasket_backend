-- Rollback Migration 012

-- 1. Revert idx_messages_ack_no_file index
DROP INDEX IF EXISTS idx_messages_ack_no_file;

CREATE INDEX IF NOT EXISTS idx_messages_ack_no_file
    ON messages (created_at)
    WHERE file_id IS NULL
      AND delivered_to_recipient_primary = TRUE
      AND synced_to_sender_primary = TRUE;

-- 2. Drop partial indexes
DROP INDEX IF EXISTS idx_messages_read_unacked;
DROP INDEX IF EXISTS idx_messages_unread;

-- 3. Restore strict messages_file_type_validation (cleaning stripped rows first)
DELETE FROM messages
WHERE message_type IN ('image', 'video', 'audio', 'file') AND file_id IS NULL;

ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS messages_file_type_validation;

ALTER TABLE messages
    ADD CONSTRAINT messages_file_type_validation
        CHECK (
            (message_type = 'text' AND file_id IS NULL) OR
            (message_type = 'unsent') OR
            (message_type IN ('image', 'video', 'audio', 'file') AND file_id IS NOT NULL)
        );

-- 4. Drop lifecycle prerequisite constraints
ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS chk_messages_read_prerequisite,
    DROP CONSTRAINT IF EXISTS chk_messages_read_ack_prerequisite;

-- 5. Drop read columns and revert delivered_to_recipient_primary column modifiers
ALTER TABLE messages
    DROP COLUMN IF EXISTS read_at,
    DROP COLUMN IF EXISTS read_acked_by_sender,
    DROP COLUMN IF EXISTS read_by_recipient;

ALTER TABLE messages
    ALTER COLUMN delivered_to_recipient_primary DROP NOT NULL,
    ALTER COLUMN delivered_to_recipient_primary DROP DEFAULT;

