-- Migration: 013_messages_keyset_pagination.up.sql
-- Purpose: Add composite keyset indexes for high-performance (created_at ASC, id ASC) seek pagination on pending messages.

CREATE INDEX IF NOT EXISTS idx_messages_recipient_keyset ON messages (recipient_id, created_at ASC, id ASC) WHERE deleted_by_recipient = FALSE;
CREATE INDEX IF NOT EXISTS idx_messages_sender_keyset ON messages (sender_id, created_at ASC, id ASC) WHERE deleted_by_sender = FALSE;
