-- ======================================
-- Create tokens table with TEXT + CHECK constraints
-- ======================================
-- +migrate Up

-- Create table
CREATE TABLE IF NOT EXISTS tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    sha256_hex_session_id TEXT NOT NULL CHECK (
        length(sha256_hex_session_id) = 64
    ),
    token TEXT NOT NULL,
    type TEXT NOT NULL,
    CONSTRAINT tokens_type_check CHECK (type IN ('fcm', 'apn')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    CONSTRAINT tokens_unique_session_user_type UNIQUE (
        sha256_hex_session_id,
        user_id,
        type
    )
);

-- Drop existing trigger if present (safe idempotent)
DROP TRIGGER IF EXISTS tokens_timestamps_trigger ON tokens;

-- Attach timestamp trigger (expects set_timestamps() function already exists)
CREATE TRIGGER tokens_timestamps_trigger
BEFORE INSERT OR UPDATE ON tokens
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Index: token lookup for sending push notifications (active only)
CREATE INDEX IF NOT EXISTS idx_tokens_token_type_active
    ON tokens (token, type)
    WHERE is_active = TRUE;

-- Index: user's active tokens ordered by newest
CREATE INDEX IF NOT EXISTS idx_tokens_user_type_active
    ON tokens (user_id, type, created_at DESC)
    WHERE is_active = TRUE;

-- Index: cleanup inactive tokens
CREATE INDEX IF NOT EXISTS idx_tokens_inactive_cleanup ON tokens (updated_at)
WHERE
    is_active = FALSE;

-- Index: session-based token lookup (for session cleanup/logout)
CREATE INDEX IF NOT EXISTS idx_tokens_session_active ON tokens (
    sha256_hex_session_id,
    is_active
)
WHERE
    is_active = TRUE;

-- ======================================
-- End of tokens table section
-- ======================================