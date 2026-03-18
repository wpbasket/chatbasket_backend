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
    device_token TEXT, -- FCM Token
    platform TEXT,
    device_name TEXT, -- Human readable name
    is_central BOOLEAN NOT NULL DEFAULT FALSE,
    user_agent TEXT,
    ip_address TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    CONSTRAINT sessions_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT sessions_platform_check CHECK (
        platform IN ('ios', 'android', 'web')
    )
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

-- Index: Enforce EXACTLY ONE Central Device per user (Business Logic Integrity)
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_central_session ON sessions (auth_user_id)
WHERE
    is_central = TRUE;

-- ======================================
-- Create verification_codes table
-- ======================================

-- Create table
CREATE TABLE IF NOT EXISTS verification_codes (
    id UUID PRIMARY KEY REFERENCES auth_users (id) ON DELETE CASCADE,
    update_id UUID UNIQUE, -- For independent update operations (nullable)
    email TEXT NOT NULL CHECK (email = lower(email)),
    code_hash TEXT NOT NULL,
    type TEXT NOT NULL,
    CONSTRAINT verification_codes_type_check CHECK (
        type IN (
            'email_verification',
            'login',
            'password_reset',
            'email_update',
            'password_update',
            'account_deletion'
        )
    ),
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

-- ======================================
-- End of auth tables section
-- ======================================