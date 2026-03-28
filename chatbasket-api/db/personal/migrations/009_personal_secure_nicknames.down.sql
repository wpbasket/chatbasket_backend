-- +migrate Down
-- 009_personal_secure_nicknames.down.sql

-- ======================================
-- Revert constraints for nicknames
-- ======================================

-- 1. Revert to 40-character constraint for user_contacts
ALTER TABLE user_contacts
DROP CONSTRAINT IF EXISTS user_contacts_nickname_check;

ALTER TABLE user_contacts
ADD CONSTRAINT user_contacts_nickname_check CHECK (length(nickname) <= 40);

-- 2. Revert to 40-character constraint for contact_requests
ALTER TABLE contact_requests
DROP CONSTRAINT IF EXISTS contact_requests_nickname_check;

ALTER TABLE contact_requests
ADD CONSTRAINT contact_requests_nickname_check CHECK (length(nickname) <= 40);

-- ======================================
-- End of migration
-- ======================================
