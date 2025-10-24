-- +migrate Down

-- Drop user_restrictions
DROP TRIGGER IF EXISTS user_restrictions_timestamps_trigger ON user_restrictions;  -- Timestamp trigger
DROP INDEX IF EXISTS idx_user_restrictions_covering;        -- Covering index for join optimization
DROP INDEX IF EXISTS idx_user_restricted_user_id;           -- Reverse lookup index
DROP INDEX IF EXISTS idx_user_restrict_status;              -- Partial index for status restriction filtering
DROP INDEX IF EXISTS idx_user_restrict_avatar;              -- Partial index for avatar restriction filtering
DROP INDEX IF EXISTS idx_user_restrict_profile;             -- Partial index for profile restriction filtering
DROP TABLE IF EXISTS user_restrictions CASCADE;             -- Also drops PK, FK, UNIQUE constraints and indexes