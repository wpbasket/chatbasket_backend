-- +migrate Up
-- 003_personal_user_contacts.up.sql

-- ======================================
-- Table: user_contacts
--        Stores the contacts (friends) of each user
-- ======================================
CREATE TABLE IF NOT EXISTS user_contacts (
    owner_user_id       UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_user_id     UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    nickname            TEXT            CHECK (length(nickname) <= 40),
    created_at          TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ,
    
    CONSTRAINT user_contacts_pk PRIMARY KEY(owner_user_id, contact_user_id)  -- Composite PK creates direct index
);

-- Drop existing trigger if already present
DROP TRIGGER IF EXISTS user_contacts_timestamps_trigger ON user_contacts;

-- Attach auto timestamp trigger
CREATE TRIGGER user_contacts_timestamps_trigger
BEFORE INSERT OR UPDATE ON user_contacts
FOR EACH ROW
EXECUTE FUNCTION set_timestamps();

-- Explicit index for reverse lookup: find all owners who have a given user as contact
CREATE INDEX IF NOT EXISTS idx_user_contacts_contact_user_id
    ON user_contacts(contact_user_id);

-- ======================================
-- End of user_contacts table section
-- ======================================