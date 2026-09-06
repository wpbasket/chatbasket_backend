-- Migration: 013_messages_keyset_pagination.down.sql

DROP INDEX IF EXISTS idx_messages_recipient_keyset;
DROP INDEX IF EXISTS idx_messages_sender_keyset;
