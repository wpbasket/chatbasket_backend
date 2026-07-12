-- +migrate Down
-- Drop Phase 1 cleanup indexes added in 012_cleanup_indexes.up.sql.
-- These statements can run inside a transaction.
DROP INDEX IF EXISTS idx_messages_expired_no_file;
DROP INDEX IF EXISTS idx_messages_ack_no_file;
DROP INDEX IF EXISTS idx_messages_chat_no_file;
DROP INDEX IF EXISTS idx_user_blocks_pair_p1;
DROP INDEX IF EXISTS idx_user_blocks_pair_p2;
DROP INDEX IF EXISTS idx_sync_actions_created_at;
DROP INDEX IF EXISTS idx_sync_actions_payload_chat_id;
DROP INDEX IF EXISTS idx_history_sync_expires_at;
