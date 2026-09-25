-- +migrate Down

-- ======================================
-- Rollback: 013_contact_request_lifecycle_and_nickname_cleanup
-- ======================================

-- Re-add nickname column to contact_requests
ALTER TABLE contact_requests ADD COLUMN IF NOT EXISTS nickname TEXT CHECK (length(nickname) <= 512);

-- Revert status check constraint to allow ('pending', 'accepted', 'declined')
ALTER TABLE contact_requests DROP CONSTRAINT IF EXISTS contact_requests_status_check;
ALTER TABLE contact_requests ADD CONSTRAINT contact_requests_status_check CHECK (status IN ('pending', 'accepted', 'declined'));

-- Re-create cleanup index for processed requests
CREATE INDEX IF NOT EXISTS idx_contact_requests_processed_cleanup
    ON contact_requests(updated_at)
    WHERE status IN ('accepted', 'declined');

-- Re-create function: add_contact_on_accept()
CREATE OR REPLACE FUNCTION add_contact_on_accept()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO user_contacts (owner_user_id, contact_user_id, nickname)
    VALUES (NEW.requester_user_id, NEW.receiver_user_id, NEW.nickname)
    ON CONFLICT (owner_user_id, contact_user_id) DO NOTHING;
    RETURN NEW;
END;
$$;

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS auto_add_contact_on_accept ON contact_requests;

-- Attach trigger to automatically add contact on accept
CREATE TRIGGER auto_add_contact_on_accept
AFTER UPDATE OF status ON contact_requests
FOR EACH ROW
WHEN (OLD.status = 'pending' AND NEW.status = 'accepted')
EXECUTE FUNCTION add_contact_on_accept();
