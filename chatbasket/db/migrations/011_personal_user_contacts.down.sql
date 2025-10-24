-- +migrate Down

-- Drop user_contacts
DROP INDEX IF EXISTS idx_user_contacts_contact_user_id;  -- Reverse lookup index
DROP TRIGGER IF EXISTS user_contacts_timestamps_trigger ON user_contacts;  -- Timestamp trigger
DROP TABLE IF EXISTS user_contacts CASCADE;              -- Also drops PK constraint and indexes