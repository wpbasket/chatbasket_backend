-- +migrate Down

-- Drop account-deletion file scan indexes
DROP INDEX IF EXISTS idx_messages_files_recipient_id;
DROP INDEX IF EXISTS idx_messages_files_sender_id;
