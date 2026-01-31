-- +migrate Down

-- Drop verification_codes
DROP INDEX IF EXISTS idx_verification_codes_expired;
-- Partial index for expired codes cleanup
DROP INDEX IF EXISTS idx_verification_codes_lookup;
-- Email + type lookup index
DROP TRIGGER IF EXISTS verification_codes_timestamps_trigger ON verification_codes;
-- Timestamp trigger
DROP TABLE IF EXISTS verification_codes CASCADE;
-- Also drops PK, FK, CHECK constraints

-- Drop sessions
DROP INDEX IF EXISTS idx_sessions_expired;
-- Partial index for expired sessions cleanup
DROP INDEX IF EXISTS idx_sessions_user_id;
-- User session lookup index
DROP TRIGGER IF EXISTS sessions_timestamps_trigger ON sessions;
-- Timestamp trigger
DROP TABLE IF EXISTS sessions CASCADE;
-- Also drops PK, FK, UNIQUE constraints

-- Drop auth_users
DROP TRIGGER IF EXISTS auth_users_timestamps_trigger ON auth_users;
-- Timestamp trigger
DROP INDEX IF EXISTS idx_auth_users_email;
-- Unique email index
DROP TABLE IF EXISTS auth_users CASCADE;
-- Also drops PK constraints