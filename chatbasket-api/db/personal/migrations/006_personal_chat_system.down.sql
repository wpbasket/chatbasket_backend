-- +migrate Down

-- ======================================
-- Rollback: Chat System (Consolidated)
-- ======================================

-- Drop message_sync_actions table
DROP TRIGGER IF EXISTS sync_actions_timestamps_trigger ON message_sync_actions;  -- Timestamp trigger
DROP INDEX IF EXISTS idx_sync_actions_payload_chat_id;
DROP INDEX IF EXISTS idx_sync_actions_created_at;
DROP INDEX IF EXISTS idx_sync_actions_user_pending;                              -- Pending actions index
DROP TABLE IF EXISTS message_sync_actions CASCADE;                               -- Also drops PK, FK constraints

-- Drop messages table
DROP TRIGGER IF EXISTS messages_timestamps_trigger ON messages;                  -- Timestamp trigger
DROP INDEX IF EXISTS idx_messages_chat_keyset;
DROP INDEX IF EXISTS idx_messages_sender_keyset;
DROP INDEX IF EXISTS idx_messages_recipient_keyset;
DROP INDEX IF EXISTS idx_messages_read_unacked;
DROP INDEX IF EXISTS idx_messages_unread;
DROP INDEX IF EXISTS idx_messages_chat_no_file;
DROP INDEX IF EXISTS idx_messages_ack_no_file;
DROP INDEX IF EXISTS idx_messages_expired_no_file;
DROP INDEX IF EXISTS idx_messages_expired_file_tokens;                           -- Expired file tokens index
DROP INDEX IF EXISTS idx_messages_file_cleanup;                                  -- File cleanup index
DROP INDEX IF EXISTS idx_messages_chat_history;                                  -- Chat history index
DROP INDEX IF EXISTS idx_messages_expired;                                       -- TTL cleanup index
DROP INDEX IF EXISTS idx_messages_pending_sender_sync;                           -- Sender sync queue index
DROP INDEX IF EXISTS idx_messages_pending_delivery;                              -- Pending delivery index
DROP TABLE IF EXISTS messages CASCADE;                                           -- Also drops PK, FK, CHECK constraints

-- Drop chats table
DROP TRIGGER IF EXISTS chats_timestamps_trigger ON chats;                        -- Timestamp trigger
DROP INDEX IF EXISTS idx_chats_participant_2;                                    -- Participant 2 lookup index
DROP INDEX IF EXISTS idx_chats_participant_1;                                    -- Participant 1 lookup index
DROP TABLE IF EXISTS chats CASCADE;                                              -- Also drops PK, UNIQUE, CHECK constraints

-- ======================================
-- End of rollback
-- ======================================