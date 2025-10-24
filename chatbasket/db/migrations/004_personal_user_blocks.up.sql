-- +migrate Up

-- ======================================
-- Table: user_blocks
--        Stores block relationships between users
-- ======================================
CREATE TABLE IF NOT EXISTS user_blocks (
    id                  UUID            PRIMARY KEY,  -- Direct index via PK
    blocker_user_id     UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_user_id     UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at          TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ,
    
    CONSTRAINT user_blocks_unique_pair UNIQUE(blocker_user_id, blocked_user_id)  -- Composite unique index
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS user_blocks_timestamps_trigger ON user_blocks;

-- Attach auto timestamp trigger
CREATE TRIGGER user_blocks_timestamps_trigger
BEFORE INSERT OR UPDATE ON user_blocks
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Explicit index for fast lookups by blocked user
CREATE INDEX IF NOT EXISTS idx_user_blocks_blocked ON user_blocks(blocked_user_id);

-- ======================================
-- Function: remove_contact_on_block()
-- Automatically removes mutual contacts when a block is created
-- ======================================
CREATE OR REPLACE FUNCTION remove_contact_on_block()
RETURNS TRIGGER AS $$
BEGIN
    -- Use VALUES for better performance with tuple deletion
    DELETE FROM user_contacts
    WHERE (owner_user_id, contact_user_id) = ANY (VALUES 
        (NEW.blocker_user_id, NEW.blocked_user_id),
        (NEW.blocked_user_id, NEW.blocker_user_id)
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS auto_remove_contact_on_block ON user_blocks;

-- Attach trigger to automatically remove contacts on block
CREATE TRIGGER auto_remove_contact_on_block
AFTER INSERT ON user_blocks
FOR EACH ROW
EXECUTE FUNCTION remove_contact_on_block();

-- ======================================
-- End of user_blocks table section
-- ======================================