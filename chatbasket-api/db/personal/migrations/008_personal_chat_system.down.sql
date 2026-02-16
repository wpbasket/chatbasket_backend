-- +migrate Down

-- ======================================
-- Rollback: Phase 6 Chat System (Merged)
-- ======================================

-- Drop message_delivery_log table
DROP TRIGGER IF EXISTS message_delivery_log_timestamps_trigger ON message_delivery_log;

DROP INDEX IF EXISTS idx_delivery_log_message;

DROP TABLE IF EXISTS message_delivery_log CASCADE;

-- Drop messages table
DROP TRIGGER IF EXISTS messages_timestamps_trigger ON messages;

DROP INDEX IF EXISTS idx_messages_expired_file_tokens;

DROP INDEX IF EXISTS idx_messages_file_cleanup;

DROP INDEX IF EXISTS idx_messages_chat_history;

DROP INDEX IF EXISTS idx_messages_expired;

DROP INDEX IF EXISTS idx_messages_pending_sender_sync;

DROP INDEX IF EXISTS idx_messages_pending_delivery;

DROP TABLE IF EXISTS messages CASCADE;

-- Drop chats table
DROP TRIGGER IF EXISTS chats_timestamps_trigger ON chats;

DROP INDEX IF EXISTS idx_chats_participant_2;

DROP INDEX IF EXISTS idx_chats_participant_1;

DROP TABLE IF EXISTS chats CASCADE;

-- ======================================
-- End of rollback
-- ======================================