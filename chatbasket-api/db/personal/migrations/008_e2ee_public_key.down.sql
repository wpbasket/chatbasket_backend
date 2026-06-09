-- +migrate Down

-- ======================================
-- E2EE: Remove public key from users table
-- ======================================
ALTER TABLE users DROP COLUMN IF EXISTS e2ee_public_key;
