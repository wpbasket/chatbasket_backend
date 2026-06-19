-- +migrate Down

ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_e2ee_public_key_length_check;
ALTER TABLE sessions DROP COLUMN IF EXISTS e2ee_public_key;
