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