-- +migrate Up

-- ======================================
-- Function: set_timestamps()
-- Automatically sets created_at and updated_at fields
-- ======================================
CREATE OR REPLACE FUNCTION set_timestamps()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        NEW.created_at := now();
        NEW.updated_at := now();
        RETURN NEW;
    END IF;

    IF TG_OP = 'UPDATE' THEN
        NEW.updated_at := now();
        RETURN NEW;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ======================================
-- Table: users
--        Stores user profile information
-- ======================================
CREATE TABLE IF NOT EXISTS users (
    id                                      UUID        PRIMARY KEY,  -- Direct index via PK
    name                                    TEXT        NOT NULL CHECK (length(name) <= 40),
    bio                                     TEXT        CHECK (length(bio) <= 150),
    profile_type                            TEXT        NOT NULL CHECK (profile_type IN ('public', 'private', 'personal')),
    is_admin_blocked                        BOOLEAN     NOT NULL DEFAULT FALSE,
    admin_block_reason                      TEXT,
    hmac_sha256_hex_username                TEXT        NOT NULL UNIQUE CHECK (length(hmac_sha256_hex_username) = 64),  -- Direct index via UNIQUE
    b64_cipher_chacha20poly1305_username    TEXT        NOT NULL CHECK (length(b64_cipher_chacha20poly1305_username) <= 52),
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS users_timestamps_trigger ON users;

-- Attach auto timestamp trigger
CREATE TRIGGER users_timestamps_trigger
BEFORE INSERT OR UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Explicit composite index for profile type and admin status with partial filtering
CREATE INDEX IF NOT EXISTS idx_users_profile_type_admin_blocked
    ON users(profile_type, is_admin_blocked)
    WHERE is_admin_blocked = FALSE;
-- Explicit index for admin-blocked users only
CREATE INDEX IF NOT EXISTS idx_users_admin_blocked_only
    ON users(id)
    WHERE is_admin_blocked = TRUE;
-- Explicit index for querying recent users by profile type
CREATE INDEX IF NOT EXISTS idx_users_profile_created
    ON users(profile_type, created_at DESC)
    WHERE is_admin_blocked = FALSE;
-- ======================================
-- End of users table section
-- ======================================


-- ======================================
-- Table: avatars
--        Stores user avatars and related tokens
-- ======================================
CREATE TABLE IF NOT EXISTS avatars (
    id                  UUID            PRIMARY KEY,  -- Direct index via PK
    user_id             UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_id             TEXT            NOT NULL,
    avatar_type         TEXT            NOT NULL DEFAULT 'profile',
    token_id            TEXT,
    token_secret        TEXT,
    token_expiry        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ,
    CONSTRAINT avatars_unique_user_file UNIQUE(user_id, file_id),  -- Composite unique index
    CONSTRAINT avatars_check_profile_file CHECK (avatar_type != 'profile' OR file_id::TEXT = user_id::TEXT)  -- Profile avatars must use user_id as file_id
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS avatars_timestamps_trigger ON avatars;

-- Attach auto timestamp trigger
CREATE TRIGGER avatars_timestamps_trigger
BEFORE INSERT OR UPDATE ON avatars
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Explicit unique index for user's profile avatar
CREATE UNIQUE INDEX IF NOT EXISTS idx_avatars_user_profile
    ON avatars(user_id, avatar_type)
    WHERE avatar_type = 'profile';
-- Explicit index for user avatars by type and recency
CREATE INDEX IF NOT EXISTS idx_avatars_user_type_created
    ON avatars(user_id, avatar_type, created_at DESC);
-- Explicit index for token expiry cleanup
CREATE INDEX IF NOT EXISTS idx_avatars_token_expiry
    ON avatars(token_expiry)
    WHERE token_expiry IS NOT NULL;
-- ======================================
-- End of avatars table section
-- ======================================


-- ======================================
-- Table: alone_username
--        stores plain text username of users with random row id
-- ======================================
CREATE TABLE IF NOT EXISTS alone_username (
    id                  UUID            PRIMARY KEY,  -- Direct index via PK
    username            TEXT            NOT NULL UNIQUE CHECK (length(username) = 11),  -- Direct index via UNIQUE
    created_at          TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS alone_username_timestamps_trigger ON alone_username;

-- Attach auto timestamp trigger
CREATE TRIGGER alone_username_timestamps_trigger
BEFORE INSERT OR UPDATE ON alone_username
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();
-- ======================================
-- End of alone_username table section
-- ======================================


-- ======================================
-- End of first migration: users, avatars, alone_username
-- ======================================