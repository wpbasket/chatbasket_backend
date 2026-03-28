-- +migrate Up

-- ======================================
-- Update: remove_contact_on_block()
-- Extend trigger to also remove contact_requests when a block is created
-- ======================================
CREATE OR REPLACE FUNCTION remove_contact_on_block()
RETURNS TRIGGER AS $$
BEGIN
    -- Remove contacts (both directions)
    DELETE FROM user_contacts
    WHERE (owner_user_id, contact_user_id) = ANY (VALUES
        (NEW.blocker_user_id, NEW.blocked_user_id),
        (NEW.blocked_user_id, NEW.blocker_user_id)
    );

    -- Remove contact requests (both directions)
    DELETE FROM contact_requests
    WHERE (requester_user_id, receiver_user_id) = ANY (VALUES
        (NEW.blocker_user_id, NEW.blocked_user_id),
        (NEW.blocked_user_id, NEW.blocker_user_id)
    );

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ======================================
-- End of migration
-- ======================================