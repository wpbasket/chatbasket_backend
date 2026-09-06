-- Migration: 014_chat_history_keyset_pagination.up.sql
-- Purpose: Add composite keyset index for high-performance (created_at ASC, id ASC) seek pagination on chat messages.

CREATE INDEX IF NOT EXISTS idx_messages_chat_keyset ON messages (chat_id, created_at ASC, id ASC);
