-- +migrate Up

-- ======================================
-- Table: user_global_restrictions
--        Stores global restrictions set by a user
-- ======================================
CREATE TABLE IF NOT EXISTS user_global_restrictions (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    restrict_avatar BOOLEAN NOT NULL DEFAULT FALSE,
    restrict_status BOOLEAN NOT NULL DEFAULT FALSE,
    restrict_profile BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS user_global_restrictions_timestamps_trigger ON user_global_restrictions;

-- Attach auto timestamp trigger
CREATE TRIGGER user_global_restrictions_timestamps_trigger
BEFORE INSERT OR UPDATE ON user_global_restrictions
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Indexes for fast lookups by restriction type
CREATE INDEX IF NOT EXISTS idx_user_global_restrictions_avatar
    ON user_global_restrictions(user_id)
    WHERE restrict_avatar = TRUE;

CREATE INDEX IF NOT EXISTS idx_user_global_restrictions_status
    ON user_global_restrictions(user_id)
    WHERE restrict_status = TRUE;

CREATE INDEX IF NOT EXISTS idx_user_global_restrictions_profile
    ON user_global_restrictions(user_id)
    WHERE restrict_profile = TRUE;

-- Covering index for global restrictions lookup (optimized for joins)
CREATE INDEX IF NOT EXISTS idx_user_global_restrictions_covering
    ON user_global_restrictions(user_id)
    INCLUDE (restrict_profile, restrict_avatar, restrict_status);


-- ======================================
-- Table: user_global_restriction_exemptions
--        Stores users who are exempted from owner's global restrictions
-- ======================================
CREATE TABLE IF NOT EXISTS user_global_restriction_exemptions (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,          -- Owner of the global restriction
    exempted_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- Contact exempted from restriction
    exception_avatar BOOLEAN NOT NULL DEFAULT FALSE,
    exception_status BOOLEAN NOT NULL DEFAULT FALSE,
    exception_profile BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    
    CONSTRAINT user_global_restriction_exemptions_pk PRIMARY KEY(user_id, exempted_user_id)
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS user_global_restriction_exemptions_timestamps_trigger ON user_global_restriction_exemptions;

-- Attach auto timestamp trigger
CREATE TRIGGER user_global_restriction_exemptions_timestamps_trigger
BEFORE INSERT OR UPDATE ON user_global_restriction_exemptions
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Covering index for exemptions lookup (optimized for joins with owner check)
CREATE INDEX IF NOT EXISTS idx_global_exemptions_covering
    ON user_global_restriction_exemptions(user_id, exempted_user_id)
    INCLUDE (exception_profile, exception_avatar, exception_status);

-- ======================================
-- Function: clean_global_restriction_exemptions
--        Automatically cleans exemptions when corresponding global restriction is lifted
-- ======================================
CREATE OR REPLACE FUNCTION clean_global_restriction_exemptions()
RETURNS TRIGGER AS $$
BEGIN
    -- Update exemption flags: set to FALSE for lifted restrictions
    UPDATE user_global_restriction_exemptions
    SET 
        exception_avatar = CASE WHEN NOT NEW.restrict_avatar THEN FALSE ELSE exception_avatar END,
        exception_status = CASE WHEN NOT NEW.restrict_status THEN FALSE ELSE exception_status END,
        exception_profile = CASE WHEN NOT NEW.restrict_profile THEN FALSE ELSE exception_profile END
    WHERE user_id = NEW.user_id
      AND (
          (NOT NEW.restrict_avatar AND exception_avatar) OR
          (NOT NEW.restrict_status AND exception_status) OR
          (NOT NEW.restrict_profile AND exception_profile)
      );
    
    -- Delete rows where all exemptions are now FALSE
    DELETE FROM user_global_restriction_exemptions
    WHERE user_id = NEW.user_id
      AND exception_avatar = FALSE
      AND exception_status = FALSE
      AND exception_profile = FALSE;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS trg_clean_global_restrictions ON user_global_restrictions;

-- Attach trigger for cleaning exemptions efficiently
CREATE TRIGGER trg_clean_global_restrictions
AFTER UPDATE OF restrict_avatar, restrict_status, restrict_profile
ON user_global_restrictions
FOR EACH ROW
WHEN (
    (OLD.restrict_avatar = TRUE AND NEW.restrict_avatar = FALSE) OR
    (OLD.restrict_status = TRUE AND NEW.restrict_status = FALSE) OR
    (OLD.restrict_profile = TRUE AND NEW.restrict_profile = FALSE)
)
EXECUTE FUNCTION clean_global_restriction_exemptions();

-- ======================================
-- End of user_global_restrictions and exemptions section
-- ======================================