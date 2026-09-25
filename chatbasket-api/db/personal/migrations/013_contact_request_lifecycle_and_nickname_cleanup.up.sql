-- +migrate Up

-- ======================================
-- Migration: 013_contact_request_lifecycle_and_nickname_cleanup
-- 1. Drop auto_add_contact_on_accept trigger and add_contact_on_accept() function
-- 2. Drop idx_contact_requests_processed_cleanup index
-- 3. Clean up any legacy non-pending contact requests
-- 4. Enforce status = 'pending' check constraint on contact_requests
-- 5. Drop nickname column from contact_requests
-- ======================================

-- Drop legacy trigger and function on contact_requests
DROP TRIGGER IF EXISTS auto_add_contact_on_accept ON contact_requests;
DROP FUNCTION IF EXISTS add_contact_on_accept();

-- Drop legacy cleanup index for processed requests
DROP INDEX IF EXISTS idx_contact_requests_processed_cleanup;

-- Clean up any existing accepted or declined requests
DELETE FROM contact_requests WHERE status != 'pending';

-- Update check constraint on status so only 'pending' is valid
ALTER TABLE contact_requests DROP CONSTRAINT IF EXISTS contact_requests_status_check;
ALTER TABLE contact_requests ADD CONSTRAINT contact_requests_status_check CHECK (status = 'pending');

-- Drop nickname from contact_requests
ALTER TABLE contact_requests DROP COLUMN IF EXISTS nickname;
