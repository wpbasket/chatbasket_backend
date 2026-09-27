-- +migrate Down

-- Drop contact_requests
DROP TRIGGER IF EXISTS contact_requests_timestamps_trigger ON contact_requests;  -- Timestamp trigger
DROP INDEX IF EXISTS idx_contact_requests_requester_pending;          -- Requester pending requests index
DROP INDEX IF EXISTS idx_contact_requests_receiver_pending;           -- Receiver pending requests index
DROP TABLE IF EXISTS contact_requests CASCADE;                        -- Also drops PK, FK, UNIQUE, CHECK constraints and indexes

-- Drop user_contacts
DROP INDEX IF EXISTS idx_user_contacts_contact_user_id;  -- Reverse lookup index
DROP TRIGGER IF EXISTS user_contacts_timestamps_trigger ON user_contacts;  -- Timestamp trigger
DROP TABLE IF EXISTS user_contacts CASCADE;              -- Also drops PK constraint and indexes