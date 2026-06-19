-- +migrate Down

-- Drop keys_revision from auth_users table
ALTER TABLE auth_users DROP COLUMN IF EXISTS keys_revision;
