-- +migrate Up
-- ======================================
-- Alter Table: messages
-- Add reply_to_message_id for Message Reply & Quoting
-- ======================================
ALTER TABLE messages ADD COLUMN IF NOT EXISTS reply_to_message_id UUID;
