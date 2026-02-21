-- +migrate Up

-- ======================================
-- Per-Participant Last Message Preview
-- Split the shared last_message_content/type into per-participant columns
-- so that "Delete for Me" can clear only the owner's preview.
-- ======================================

-- Add per-participant columns
ALTER TABLE chats ADD COLUMN p1_last_message_content TEXT;

ALTER TABLE chats ADD COLUMN p2_last_message_content TEXT;

ALTER TABLE chats ADD COLUMN p1_last_message_type TEXT;

ALTER TABLE chats ADD COLUMN p2_last_message_type TEXT;

COMMENT ON COLUMN chats.p1_last_message_content IS 'Last message preview content for participant_1';

COMMENT ON COLUMN chats.p2_last_message_content IS 'Last message preview content for participant_2';

COMMENT ON COLUMN chats.p1_last_message_type IS 'Last message type for participant_1 preview';

COMMENT ON COLUMN chats.p2_last_message_type IS 'Last message type for participant_2 preview';

-- Drop the old shared columns (system is not live, no backfill needed)
ALTER TABLE chats DROP COLUMN IF EXISTS last_message_content;

ALTER TABLE chats DROP COLUMN IF EXISTS last_message_type;