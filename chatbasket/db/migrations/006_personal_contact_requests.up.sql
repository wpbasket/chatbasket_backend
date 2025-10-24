-- +migrate Up

-- ======================================
-- Create an ENUM type for the request status
-- ======================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'request_status_enum') THEN
        CREATE TYPE request_status_enum AS ENUM ('pending', 'accepted', 'declined');
    END IF;
END
$$;

-- ======================================
-- Table: contact_requests
--        Stores pending friend/contact requests
-- ======================================
CREATE TABLE IF NOT EXISTS contact_requests (
    id                      UUID            PRIMARY KEY,  -- Direct index via PK
    requester_user_id       UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    receiver_user_id        UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status                  request_status_enum NOT NULL DEFAULT 'pending',
    created_at              TIMESTAMPTZ,
    updated_at              TIMESTAMPTZ,
    
    CONSTRAINT contact_requests_unique_pair UNIQUE(requester_user_id, receiver_user_id),  -- Composite unique index
    CONSTRAINT contact_requests_no_self_request CHECK (requester_user_id != receiver_user_id)  -- Prevent self-contact requests
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS contact_requests_timestamps_trigger ON contact_requests;  -- Timestamp trigger

-- Attach auto timestamp trigger
CREATE TRIGGER contact_requests_timestamps_trigger
BEFORE INSERT OR UPDATE ON contact_requests
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Explicit partial index for receiver's pending requests (most common query)
CREATE INDEX IF NOT EXISTS idx_contact_requests_receiver_pending
    ON contact_requests(receiver_user_id, created_at DESC)
    INCLUDE (requester_user_id)
    WHERE status = 'pending';

-- Explicit partial index for requester's pending requests
CREATE INDEX IF NOT EXISTS idx_contact_requests_requester_pending
    ON contact_requests(requester_user_id, created_at DESC)
    INCLUDE (receiver_user_id)
    WHERE status = 'pending';

-- Explicit index for cleanup of old processed requests
CREATE INDEX IF NOT EXISTS idx_contact_requests_processed_cleanup
    ON contact_requests(updated_at)
    WHERE status IN ('accepted', 'declined');

-- ======================================
-- Function: add_contact_on_accept()
-- Automatically adds the one-way contact when a request is accepted
-- ======================================
CREATE OR REPLACE FUNCTION add_contact_on_accept()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO user_contacts (owner_user_id, contact_user_id)
    VALUES (NEW.requester_user_id, NEW.receiver_user_id)
    ON CONFLICT (owner_user_id, contact_user_id) DO NOTHING;
    RETURN NEW;
END;
$$;

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS auto_add_contact_on_accept ON contact_requests;  -- Trigger for auto adding contact on accept

-- Attach trigger to automatically add contact on accept
CREATE TRIGGER auto_add_contact_on_accept
AFTER UPDATE OF status ON contact_requests
FOR EACH ROW
WHEN (OLD.status = 'pending' AND NEW.status = 'accepted')
EXECUTE FUNCTION add_contact_on_accept();

-- ======================================
-- End of contact_requests table section
-- ======================================