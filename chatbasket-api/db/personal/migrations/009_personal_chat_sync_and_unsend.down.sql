-- +migrate Down

-- ======================================
-- Rollback: Phase 9 Sync & Unsend
-- ======================================

-- Drop message_sync_actions table
DROP TRIGGER IF EXISTS sync_actions_timestamps_trigger ON message_sync_actions;

DROP INDEX IF EXISTS idx_sync_actions_user_pending;

DROP TABLE IF EXISTS message_sync_actions CASCADE;

-- Revert changes to chats table
ALTER TABLE chats DROP COLUMN IF EXISTS last_message_id;

-- ======================================
-- End of rollback
-- ======================================