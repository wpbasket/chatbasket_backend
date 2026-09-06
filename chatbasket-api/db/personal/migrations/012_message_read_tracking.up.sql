-- Migration 012: Add read receipt tracking to messages
-- 1. Add read status and timestamps for recipient read and sender read ACK
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS read_by_recipient BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS read_acked_by_sender BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS read_at TIMESTAMPTZ;

-- Ensure delivered_to_recipient_primary is NOT NULL DEFAULT FALSE
UPDATE messages SET delivered_to_recipient_primary = FALSE WHERE delivered_to_recipient_primary IS NULL;
ALTER TABLE messages
    ALTER COLUMN delivered_to_recipient_primary SET DEFAULT FALSE,
    ALTER COLUMN delivered_to_recipient_primary SET NOT NULL;


-- 2. Update messages_file_type_validation to allow file_id to be NULL for media types ONLY when payload is stripped upon delivery
ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS messages_file_type_validation;

ALTER TABLE messages
    ADD CONSTRAINT messages_file_type_validation
        CHECK (
            (message_type = 'text' AND file_id IS NULL) OR
            (message_type = 'unsent') OR
            (message_type IN ('image', 'video', 'audio', 'file') AND (
                file_id IS NOT NULL OR
                (delivered_to_recipient_primary = TRUE AND synced_to_sender_primary = TRUE)
            ))
        );

-- 3. Partial index for fast unread message lookups
CREATE INDEX IF NOT EXISTS idx_messages_unread
    ON messages (chat_id, recipient_id, read_by_recipient)
    WHERE read_by_recipient = FALSE;

-- 4. Partial index for fast sender read-receipt catchup
CREATE INDEX IF NOT EXISTS idx_messages_read_unacked
    ON messages (chat_id, sender_id, read_by_recipient, read_acked_by_sender)
    WHERE read_by_recipient = TRUE AND read_acked_by_sender = FALSE;

-- 5. Update idx_messages_ack_no_file to require all 4 lifecycle flags before row cleanup
DROP INDEX IF EXISTS idx_messages_ack_no_file;

CREATE INDEX IF NOT EXISTS idx_messages_ack_no_file
    ON messages (created_at)
    WHERE file_id IS NULL
      AND delivered_to_recipient_primary = TRUE
      AND synced_to_sender_primary = TRUE
      AND read_by_recipient = TRUE
      AND read_acked_by_sender = TRUE;

-- 6. Add lifecycle prerequisite check constraints
ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS chk_messages_read_prerequisite,
    DROP CONSTRAINT IF EXISTS chk_messages_read_ack_prerequisite;

ALTER TABLE messages
    ADD CONSTRAINT chk_messages_read_prerequisite
        CHECK (read_by_recipient = FALSE OR delivered_to_recipient = TRUE),
    ADD CONSTRAINT chk_messages_read_ack_prerequisite
        CHECK (read_acked_by_sender = FALSE OR read_by_recipient = TRUE);

