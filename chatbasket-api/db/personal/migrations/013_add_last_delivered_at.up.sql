-- +migrate Up
ALTER TABLE chats
ADD COLUMN p1_last_delivered_at TIMESTAMPTZ,
ADD COLUMN p2_last_delivered_at TIMESTAMPTZ;

COMMENT ON COLUMN chats.p1_last_delivered_at IS 'Timestamp of the last message delivered to participant 1';

COMMENT ON COLUMN chats.p2_last_delivered_at IS 'Timestamp of the last message delivered to participant 2';