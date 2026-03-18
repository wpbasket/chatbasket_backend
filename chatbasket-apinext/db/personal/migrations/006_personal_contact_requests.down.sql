-- +migrate Down

-- Drop contact_requests (trigger)
DROP TRIGGER IF EXISTS auto_add_contact_on_accept ON contact_requests;  -- Trigger for auto adding contact on accept

-- Drop function
DROP FUNCTION IF EXISTS add_contact_on_accept();                       -- Function that adds contact on accept

-- Drop contact_requests
DROP TRIGGER IF EXISTS contact_requests_timestamps_trigger ON contact_requests;  -- Timestamp trigger
DROP INDEX IF EXISTS idx_contact_requests_processed_cleanup;          -- Cleanup index for processed requests
DROP INDEX IF EXISTS idx_contact_requests_requester_pending;          -- Requester pending requests index
DROP INDEX IF EXISTS idx_contact_requests_receiver_pending;           -- Receiver pending requests index
DROP TABLE IF EXISTS contact_requests CASCADE;                        -- Also drops PK, FK, UNIQUE, CHECK constraints and indexes

-- Drop enum type
DROP TYPE IF EXISTS request_status_enum;
