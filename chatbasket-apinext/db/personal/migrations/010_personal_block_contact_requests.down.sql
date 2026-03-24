-- +migrate Down

-- ======================================
-- Revert: remove_contact_on_block()
-- Restore original function that only removes user_contacts
-- ======================================
CREATE OR REPLACE FUNCTION remove_contact_on_block()
RETURNS TRIGGER AS $$
BEGIN
    -- Use VALUES for better performance with tuple deletion
    DELETE FROM user_contacts
    WHERE (owner_user_id, contact_user_id) = ANY (VALUES
        (NEW.blocker_user_id, NEW.blocked_user_id),
        (NEW.blocked_user_id, NEW.blocker_user_id)
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ======================================
-- End of migration
-- ======================================
