-- +migrate Down

-- Restore deprecated e2ee_public_key to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS e2ee_public_key TEXT;
ALTER TABLE users ADD CONSTRAINT users_e2ee_public_key_length_check CHECK (
    e2ee_public_key IS NULL OR length(e2ee_public_key) = 44
);
