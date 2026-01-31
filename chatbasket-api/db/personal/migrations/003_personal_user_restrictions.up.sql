-- +migrate Up

-- ======================================
-- Table: user_restrictions
--        Stores user-specific restrictions for profile, avatar, and status visibility
-- ======================================
CREATE TABLE IF NOT EXISTS user_restrictions (
    id                      UUID            PRIMARY KEY,  -- Direct index via PK
    user_id                 UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    restricted_user_id      UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    restrict_profile        BOOLEAN         NOT NULL DEFAULT FALSE,
    restrict_avatar         BOOLEAN         NOT NULL DEFAULT FALSE,
    restrict_status         BOOLEAN         NOT NULL DEFAULT FALSE,
    created_at              TIMESTAMPTZ,
    updated_at              TIMESTAMPTZ,
    
    CONSTRAINT user_restrictions_unique_pair UNIQUE(user_id, restricted_user_id)  -- Composite unique index covers main lookup
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS user_restrictions_timestamps_trigger ON user_restrictions;

-- Attach auto timestamp trigger
CREATE TRIGGER user_restrictions_timestamps_trigger
BEFORE INSERT OR UPDATE ON user_restrictions
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Partial index for optimized profile restriction filtering
CREATE INDEX IF NOT EXISTS idx_user_restrict_profile
    ON user_restrictions(user_id, restricted_user_id)
    WHERE restrict_profile = TRUE;

-- Partial index for optimized avatar restriction filtering
CREATE INDEX IF NOT EXISTS idx_user_restrict_avatar
    ON user_restrictions(user_id, restricted_user_id)
    WHERE restrict_avatar = TRUE;

-- Partial index for optimized status restriction filtering
CREATE INDEX IF NOT EXISTS idx_user_restrict_status
    ON user_restrictions(user_id, restricted_user_id)
    WHERE restrict_status = TRUE;

-- Explicit index for reverse lookup: find all users who have restricted a given user
CREATE INDEX IF NOT EXISTS idx_user_restricted_user_id
    ON user_restrictions(restricted_user_id);

-- Covering index for user restrictions lookup (optimized for joins)
CREATE INDEX IF NOT EXISTS idx_user_restrictions_covering
    ON user_restrictions(user_id, restricted_user_id)
    INCLUDE (restrict_profile, restrict_avatar, restrict_status);

-- ======================================
-- End of user_restrictions table section
-- ======================================