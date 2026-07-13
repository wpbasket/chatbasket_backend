-- +migrate Up

-- ======================================
-- Cross-Module Cleanup: Block → Sync Actions
-- Trigger fires when user_blocks is created
-- Cleans up message_sync_actions for both users
-- ======================================

CREATE OR REPLACE FUNCTION cleanup_sync_actions_on_block()
RETURNS TRIGGER AS $$
DECLARE
    affected_chat_id UUID;
BEGIN
    -- Look up the single chat between blocker and blocked (unique constraint guarantees 0 or 1)
    SELECT id INTO affected_chat_id
    FROM chats
    WHERE participant_1_id = LEAST(NEW.blocker_user_id, NEW.blocked_user_id)
      AND participant_2_id = GREATEST(NEW.blocker_user_id, NEW.blocked_user_id);

    -- Only delete if a chat actually exists between them
    IF affected_chat_id IS NOT NULL THEN
        DELETE FROM message_sync_actions msa
        WHERE msa.user_id IN (NEW.blocker_user_id, NEW.blocked_user_id)
          AND (msa.payload->>'chat_id')::uuid = affected_chat_id;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS cleanup_sync_actions_on_block ON user_blocks;

-- Attach trigger to user_blocks
CREATE TRIGGER cleanup_sync_actions_on_block
AFTER INSERT ON user_blocks
FOR EACH ROW
EXECUTE FUNCTION cleanup_sync_actions_on_block();

-- ======================================
-- End of sync_actions cleanup section
-- ======================================
