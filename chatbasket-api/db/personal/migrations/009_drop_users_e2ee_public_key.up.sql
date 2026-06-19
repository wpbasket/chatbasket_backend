-- +migrate Up

-- Drop deprecated e2ee_public_key from users table
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_e2ee_public_key_length_check;
ALTER TABLE users DROP COLUMN IF EXISTS e2ee_public_key;
