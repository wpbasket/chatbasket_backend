-- +migrate Up

-- ======================================
-- Create auth_users table
-- ======================================

-- Create table
CREATE TABLE IF NOT EXISTS auth_users (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL CHECK (email = lower(email)),
    password_hash TEXT NOT NULL,
    is_email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

-- Drop existing trigger if present (safe idempotent)
DROP TRIGGER IF EXISTS auth_users_timestamps_trigger ON auth_users;

-- Attach timestamp trigger (expects set_timestamps() function already exists)
CREATE TRIGGER auth_users_timestamps_trigger
BEFORE INSERT OR UPDATE ON auth_users
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Index: unique email for login lookups (B-tree, optimal for equality)
CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_users_email ON auth_users (email);

-- ======================================
-- Create sessions table
-- ======================================

-- Create table
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY,
    auth_user_id UUID NOT NULL REFERENCES auth_users (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    user_agent TEXT,
    ip_address TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    CONSTRAINT sessions_token_hash_unique UNIQUE (token_hash)
);

-- Drop existing trigger if present (safe idempotent)
DROP TRIGGER IF EXISTS sessions_timestamps_trigger ON sessions;

-- Attach timestamp trigger
CREATE TRIGGER sessions_timestamps_trigger
BEFORE INSERT OR UPDATE ON sessions
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Index: user session lookup for logout all
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions (auth_user_id);

-- Index: partial index for expired sessions cleanup (only indexes expired rows)
CREATE INDEX IF NOT EXISTS idx_sessions_expired ON sessions (expires_at)
WHERE
    expires_at < now();

-- ======================================
-- Create verification_codes table
-- ======================================

-- Create table
CREATE TABLE IF NOT EXISTS verification_codes (
    id UUID PRIMARY KEY,
    auth_user_id UUID NULL REFERENCES auth_users (id) ON DELETE CASCADE,
    email TEXT NOT NULL CHECK (email = lower(email)),
    code_hash TEXT NOT NULL,
    type TEXT NOT NULL,
    CONSTRAINT verification_codes_type_check CHECK (
        type IN (
            'email_verification',
            'login',
            'password_reset'
        )
    ),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

-- Drop existing trigger if present (safe idempotent)
DROP TRIGGER IF EXISTS verification_codes_timestamps_trigger ON verification_codes;

-- Attach timestamp trigger
CREATE TRIGGER verification_codes_timestamps_trigger
BEFORE INSERT OR UPDATE ON verification_codes
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Index: email + type lookup with ORDER BY created_at DESC (covers GetVerificationCodeByEmailAndType)
CREATE INDEX IF NOT EXISTS idx_verification_codes_lookup ON verification_codes (email, type, created_at DESC);

-- Index: partial index for expired codes cleanup (only indexes expired rows)
CREATE INDEX IF NOT EXISTS idx_verification_codes_expired ON verification_codes (expires_at)
WHERE
    expires_at < now();

-- ======================================
-- End of auth tables section
-- ======================================