-- +migrate Down

-- Drop user_global_restrictions (trigger)
DROP TRIGGER IF EXISTS trg_clean_global_restrictions ON user_global_restrictions;

-- Drop function
DROP FUNCTION IF EXISTS clean_global_restriction_exemptions();

-- Drop user_global_restriction_exemptions
DROP TRIGGER IF EXISTS user_global_restriction_exemptions_timestamps_trigger ON user_global_restriction_exemptions;
DROP INDEX IF EXISTS idx_global_exemptions_covering;
DROP TABLE IF EXISTS user_global_restriction_exemptions CASCADE;

-- Drop user_global_restrictions
DROP TRIGGER IF EXISTS user_global_restrictions_timestamps_trigger ON user_global_restrictions;
DROP INDEX IF EXISTS idx_user_global_restrictions_covering;
DROP INDEX IF EXISTS idx_user_global_restrictions_profile;
DROP INDEX IF EXISTS idx_user_global_restrictions_status;
DROP INDEX IF EXISTS idx_user_global_restrictions_avatar;
DROP TABLE IF EXISTS user_global_restrictions CASCADE;

-- Drop user_restrictions
DROP TRIGGER IF EXISTS user_restrictions_timestamps_trigger ON user_restrictions;  -- Timestamp trigger
DROP INDEX IF EXISTS idx_user_restrictions_covering;        -- Covering index for join optimization
DROP INDEX IF EXISTS idx_user_restricted_user_id;           -- Reverse lookup index
DROP INDEX IF EXISTS idx_user_restrict_status;              -- Partial index for status restriction filtering
DROP INDEX IF EXISTS idx_user_restrict_avatar;              -- Partial index for avatar restriction filtering
DROP INDEX IF EXISTS idx_user_restrict_profile;             -- Partial index for profile restriction filtering
DROP TABLE IF EXISTS user_restrictions CASCADE;             -- Also drops PK, FK, UNIQUE constraints and indexes