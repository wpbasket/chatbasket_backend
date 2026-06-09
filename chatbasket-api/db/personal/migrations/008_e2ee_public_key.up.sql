-- +migrate Up

-- ======================================
-- E2EE: Add public key to users table
-- ======================================

-- X25519 public key for end-to-end encryption.
-- Base64-encoded 32 bytes = 44 characters.
-- NULL means user has not set up E2EE yet (graceful degradation).
ALTER TABLE users ADD COLUMN IF NOT EXISTS e2ee_public_key TEXT;

-- Add length constraint (idempotent: only adds if not already present)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'users_e2ee_public_key_length_check'
    ) THEN
        ALTER TABLE users ADD CONSTRAINT users_e2ee_public_key_length_check CHECK (
            e2ee_public_key IS NULL OR length(e2ee_public_key) = 44
        );
    END IF;
END $$;

COMMENT ON COLUMN users.e2ee_public_key IS 'Base64-encoded X25519 public key for E2EE. NULL = E2EE not set up.';
