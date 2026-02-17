-- +migrate Up

-- ======================================
-- Phase 1: Unsend & Sync Redesign
-- ======================================

-- 1. Updates to chats table
ALTER TABLE chats ADD COLUMN last_message_id UUID;

COMMENT ON COLUMN chats.last_message_id IS 'UUID of the message currently displayed as the preview';

-- ======================================
-- Table: message_sync_actions
--        Relay for cross-device synchronization (unsend, delete_for_me)
-- ======================================
CREATE TABLE IF NOT EXISTS message_sync_actions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    action_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    delivered_to_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    CONSTRAINT fk_sync_actions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ck_sync_action_type CHECK (
        action_type IN ('unsend', 'delete_for_me')
    )
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS sync_actions_timestamps_trigger ON message_sync_actions;

-- Attach auto timestamp trigger
CREATE TRIGGER sync_actions_timestamps_trigger
BEFORE INSERT OR UPDATE ON message_sync_actions
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Index for fetching pending actions for a specific user
CREATE INDEX IF NOT EXISTS idx_sync_actions_user_pending ON message_sync_actions (
    user_id,
    delivered_to_primary,
    created_at
);

-- ======================================
-- End of migration: Phase 9 Sync & Unsend
-- ======================================