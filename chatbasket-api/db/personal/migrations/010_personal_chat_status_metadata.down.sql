-- +migrate Down

-- ======================================
-- Remove Chat Status & Preview Metadata from chats table
-- ======================================

ALTER TABLE chats DROP COLUMN IF EXISTS p1_unread_count;

ALTER TABLE chats DROP COLUMN IF EXISTS p2_unread_count;

ALTER TABLE chats DROP COLUMN IF EXISTS p1_last_read_at;

ALTER TABLE chats DROP COLUMN IF EXISTS p2_last_read_at;

ALTER TABLE chats DROP COLUMN IF EXISTS last_message_content;

ALTER TABLE chats DROP COLUMN IF EXISTS last_message_created_at;

ALTER TABLE chats DROP COLUMN IF EXISTS last_message_type;

ALTER TABLE chats DROP COLUMN IF EXISTS last_message_sender_id;