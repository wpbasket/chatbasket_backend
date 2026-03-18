-- +migrate Down

-- Drop user_blocks (trigger)
DROP TRIGGER IF EXISTS auto_remove_contact_on_block ON user_blocks;  -- Trigger for auto removing contacts on block

-- Drop function
DROP FUNCTION IF EXISTS remove_contact_on_block();                   -- Function that removes contacts on block

-- Drop user_blocks
DROP TRIGGER IF EXISTS user_blocks_timestamps_trigger ON user_blocks;  -- Timestamp trigger
DROP INDEX IF EXISTS idx_user_blocks_blocked;                        -- Blocked user lookup index
DROP TABLE IF EXISTS user_blocks CASCADE;                            -- Also drops PK, FK, UNIQUE constraints and indexes