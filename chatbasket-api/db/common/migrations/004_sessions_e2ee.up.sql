-- +migrate Up

-- Add e2ee_public_key to sessions table
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS e2ee_public_key CHAR(44);

-- Add length constraint to sessions.e2ee_public_key
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'sessions_e2ee_public_key_length_check'
    ) THEN
        ALTER TABLE sessions ADD CONSTRAINT sessions_e2ee_public_key_length_check CHECK (
            e2ee_public_key IS NULL OR length(e2ee_public_key) = 44
        );
    END IF;
END $$;

COMMENT ON COLUMN sessions.e2ee_public_key IS 'Base64-encoded X25519 public key associated with the active session. NULL = E2EE not initialized on this device.';
