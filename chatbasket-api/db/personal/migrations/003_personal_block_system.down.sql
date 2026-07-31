-- +migrate Down

-- Drop user_blocks
DROP TRIGGER IF EXISTS user_blocks_timestamps_trigger ON user_blocks;  -- Timestamp trigger
DROP INDEX IF EXISTS idx_user_blocks_blocked;                        -- Blocked user lookup index
DROP INDEX IF EXISTS idx_user_blocks_pair_p1;
DROP INDEX IF EXISTS idx_user_blocks_pair_p2;
DROP TABLE IF EXISTS user_blocks CASCADE;                            -- Also drops PK, FK, UNIQUE constraints and indexes
