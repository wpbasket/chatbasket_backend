-- +migrate Up

-- Add keys_revision to auth_users table (auth owns all E2EE state)
ALTER TABLE auth_users ADD COLUMN IF NOT EXISTS keys_revision INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN auth_users.keys_revision IS 'Monotonically increasing version number tracking changes to the user''s active session keys.';
