-- +migrate Down

-- Restore the shared columns
ALTER TABLE chats ADD COLUMN last_message_content TEXT;

ALTER TABLE chats ADD COLUMN last_message_type TEXT;

-- Drop the per-participant columns
ALTER TABLE chats DROP COLUMN IF EXISTS p1_last_message_content;

ALTER TABLE chats DROP COLUMN IF EXISTS p2_last_message_content;

ALTER TABLE chats DROP COLUMN IF EXISTS p1_last_message_type;

ALTER TABLE chats DROP COLUMN IF EXISTS p2_last_message_type;