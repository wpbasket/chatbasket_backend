-- +migrate Down

-- ======================================
-- Revert: Block → Sync Actions Cleanup
-- Removes the trigger that cleaned up message_sync_actions on block
-- ======================================

-- Drop the trigger from user_blocks table
DROP TRIGGER IF EXISTS cleanup_sync_actions_on_block ON user_blocks;

-- Drop the trigger function
DROP FUNCTION IF EXISTS cleanup_sync_actions_on_block();

-- ======================================
-- End of revert
-- ======================================
