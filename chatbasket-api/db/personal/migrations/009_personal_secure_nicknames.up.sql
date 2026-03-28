-- +migrate Up
-- 009_personal_secure_nicknames.up.sql

-- ======================================
-- Update constraints for encrypted nicknames
-- ======================================

-- 1. Remove 40-character constraint from user_contacts
ALTER TABLE user_contacts
DROP CONSTRAINT IF EXISTS user_contacts_nickname_check;

ALTER TABLE user_contacts
ADD CONSTRAINT user_contacts_nickname_check CHECK (length(nickname) <= 512);

-- 2. Remove 40-character constraint from contact_requests
ALTER TABLE contact_requests
DROP CONSTRAINT IF EXISTS contact_requests_nickname_check;

ALTER TABLE contact_requests
ADD CONSTRAINT contact_requests_nickname_check CHECK (length(nickname) <= 512);

-- ======================================
-- End of migration
-- ======================================
