-- +migrate Up
-- ======================================
-- Cleanup indexes: Phase 1
--
-- These partial indexes support the bounded DB-only cleanup deletes in the
-- message-cleanup background worker.
--
-- NOTE: For production deploys against an existing populated DB, these should
-- be created with CONCURRENTLY so application writes are not blocked. The
-- goose migration runner used here wraps each migration file in a transaction,
-- so CONCURRENTLY cannot be used inside this file. For a greenfield deploy
-- (empty / freshly migrated DB) the regular CREATE INDEX is fine and much
-- faster. To create these CONCURRENTLY on a live DB, run the statements
-- manually outside the migration runner.
-- ======================================

-- 1. TTL-expired messages with no R2 file attached
CREATE INDEX IF NOT EXISTS idx_messages_expired_no_file
    ON messages (expires_at)
    WHERE file_id IS NULL;

-- 2. Fully-acknowledged text-only messages (both primary devices confirmed)
CREATE INDEX IF NOT EXISTS idx_messages_ack_no_file
    ON messages (created_at)
    WHERE file_id IS NULL
      AND delivered_to_recipient_primary = TRUE
      AND synced_to_sender_primary = TRUE;

-- 3. Messages in a blocked-user chat with no R2 file attached
CREATE INDEX IF NOT EXISTS idx_messages_chat_no_file
    ON messages (chat_id)
    WHERE file_id IS NULL;

-- 4. Old cross-device sync actions (30-day retention rule)
CREATE INDEX IF NOT EXISTS idx_sync_actions_created_at
    ON message_sync_actions (created_at);

-- 4b. Blocked-user sync actions payload chatId lookups (expression index)
CREATE INDEX IF NOT EXISTS idx_sync_actions_payload_chat_id
    ON message_sync_actions (((payload->>'chatId')::uuid));

-- 5. user_blocks pair lookups (support blocked-user cleanup joins).
-- The cleanup query joins user_blocks to chats on either (blocker, blocked)
-- or (blocked, blocker). Two B-tree indexes (one per ordering) let the
-- planner use Index Scans instead of a Seq Scan if user_blocks grows to 
-- millions of rows in production.
CREATE INDEX IF NOT EXISTS idx_user_blocks_pair_p1
    ON user_blocks (blocker_user_id, blocked_user_id);

CREATE INDEX IF NOT EXISTS idx_user_blocks_pair_p2
    ON user_blocks (blocked_user_id, blocker_user_id);

-- 6. Expired history-sync records
CREATE INDEX IF NOT EXISTS idx_history_sync_expires_at
    ON history_sync (expires_at);

-- ======================================
-- End of cleanup indexes section
-- ======================================
