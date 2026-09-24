-- +migrate Up
-- ======================================
-- Index: messages file cleanup per sender / recipient
--        Speeds up account-deletion file discovery
--        (GetMessagesWithFilesForUserBatch split per side).
--        The old single query with
--          WHERE (sender_id = $1 OR recipient_id = $1)
--        could not use an index and scanned the messages table
--        while holding chat row locks.
-- ======================================

-- Sender-side file scan for account deletion
CREATE INDEX IF NOT EXISTS idx_messages_files_sender_id
    ON messages (sender_id, id)
    WHERE file_id IS NOT NULL;

-- Recipient-side file scan for account deletion
CREATE INDEX IF NOT EXISTS idx_messages_files_recipient_id
    ON messages (recipient_id, id)
    WHERE file_id IS NOT NULL;
