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
    contacts                                INTEGER     Not NULL DEFAULT 0 CHECK (contacts >= 0 AND contacts <= 500),
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

-- Explicit indexes for faster filtering
CREATE INDEX IF NOT EXISTS idx_users_profile_type ON users(profile_type);
CREATE INDEX IF NOT EXISTS idx_users_is_admin_blocked ON users(is_admin_blocked);

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
    file_id             TEXT            NOT NULL,     -- Can sometimes equal user_id (e.g., default avatar),
    avatar_type         TEXT            NOT NULL DEFAULT 'profile',  -- Avatar type: profile, cover, thumbnail, etc.
    token_id            TEXT,
    token_secret        TEXT,
    token_expiry        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ,
    
    UNIQUE(user_id, file_id)  -- Composite unique index (allows file_id = user_id)
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS avatars_timestamps_trigger ON avatars;

-- Attach auto timestamp trigger
CREATE TRIGGER avatars_timestamps_trigger
BEFORE INSERT OR UPDATE ON avatars
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Explicit index for fast avatar lookup by user
CREATE INDEX IF NOT EXISTS idx_avatars_user_id ON avatars(user_id);

-- Explicit index for fast avatar lookup by type
CREATE INDEX IF NOT EXISTS idx_avatars_type ON avatars(avatar_type);

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
