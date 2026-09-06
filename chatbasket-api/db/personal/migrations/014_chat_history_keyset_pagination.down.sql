-- Migration: 014_chat_history_keyset_pagination.down.sql
-- Purpose: Drop composite keyset index on chat messages.

DROP INDEX IF EXISTS idx_messages_chat_keyset;
