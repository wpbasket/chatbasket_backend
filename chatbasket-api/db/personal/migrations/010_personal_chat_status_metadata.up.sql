-- +migrate Up

-- ======================================
-- Add Chat Status & Preview Metadata to chats table
-- ======================================

-- Unread Counts & Read Status
ALTER TABLE chats
ADD COLUMN IF NOT EXISTS p1_unread_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE chats
ADD COLUMN IF NOT EXISTS p2_unread_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE chats
ADD COLUMN IF NOT EXISTS p1_last_read_at TIMESTAMPTZ;

ALTER TABLE chats
ADD COLUMN IF NOT EXISTS p2_last_read_at TIMESTAMPTZ;

-- Last Message Preview (Persistent even if message is deleted)
ALTER TABLE chats ADD COLUMN IF NOT EXISTS last_message_content TEXT;

ALTER TABLE chats
ADD COLUMN IF NOT EXISTS last_message_created_at TIMESTAMPTZ;

ALTER TABLE chats ADD COLUMN IF NOT EXISTS last_message_type TEXT;

ALTER TABLE chats
ADD COLUMN IF NOT EXISTS last_message_sender_id UUID;

-- Add comments for developer clarity
COMMENT ON COLUMN chats.p1_unread_count IS 'Unread count for participant_1_id';

COMMENT ON COLUMN chats.p2_unread_count IS 'Unread count for participant_2_id';

COMMENT ON COLUMN chats.last_message_content IS 'Content of the last message (persisted for preview)';