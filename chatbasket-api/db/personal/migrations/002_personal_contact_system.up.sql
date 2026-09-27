-- +migrate Up

-- ======================================
-- Table: user_contacts
--        Stores the contacts (friends) of each user
-- ======================================
CREATE TABLE IF NOT EXISTS user_contacts (
    owner_user_id       UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_user_id     UUID            NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    nickname            TEXT            CHECK (length(nickname) <= 512),
    created_at          TIMESTAMPTZ     NOT NULL,
    updated_at          TIMESTAMPTZ     NOT NULL,
    
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
-- Table: contact_requests
--        Stores pending friend/contact requests
-- ======================================
CREATE TABLE IF NOT EXISTS contact_requests (
    id                      UUID                    PRIMARY KEY,  -- Direct index via PK
    requester_user_id       UUID                    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    receiver_user_id        UUID                    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status                  TEXT                    NOT NULL DEFAULT 'pending' CHECK (status = 'pending'),
    created_at              TIMESTAMPTZ             NOT NULL,
    updated_at              TIMESTAMPTZ             NOT NULL,

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

-- ======================================
-- End of Contacts and Requests section
-- ======================================