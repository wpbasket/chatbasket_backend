-- +migrate Up

-- ======================================
-- Table: user_blocks
--        Stores block relationships between users
-- ======================================
CREATE TABLE IF NOT EXISTS user_blocks (
    id                  UUID            PRIMARY KEY,  -- Direct index via PK
    blocker_user_id     UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_user_id     UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at          TIMESTAMPTZ     NOT NULL,
    updated_at          TIMESTAMPTZ     NOT NULL,
    
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

-- user_blocks pair lookups (support blocked-user cleanup joins).
-- The cleanup query joins user_blocks to chats on either (blocker, blocked)
-- or (blocked, blocker). Two B-tree indexes (one per ordering) let the
-- planner use Index Scans instead of a Seq Scan if user_blocks grows to 
-- millions of rows in production.
CREATE INDEX IF NOT EXISTS idx_user_blocks_pair_p1
    ON user_blocks (blocker_user_id, blocked_user_id);

CREATE INDEX IF NOT EXISTS idx_user_blocks_pair_p2
    ON user_blocks (blocked_user_id, blocker_user_id);


-- ======================================
-- End of user_blocks table section
-- ======================================