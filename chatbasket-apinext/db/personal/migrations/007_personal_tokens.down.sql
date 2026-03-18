-- +migrate Down

-- Drop tokens
DROP INDEX IF EXISTS idx_tokens_session_active;
-- Session-based token lookup index
DROP INDEX IF EXISTS idx_tokens_inactive_cleanup;
-- Inactive token cleanup index
DROP INDEX IF EXISTS idx_tokens_user_type_active;
-- User's active tokens index
DROP INDEX IF EXISTS idx_tokens_token_type_active;
-- Token lookup for push notifications index
DROP TRIGGER IF EXISTS tokens_timestamps_trigger ON tokens;
-- Timestamp trigger
DROP TABLE IF EXISTS tokens CASCADE;
-- Also drops PK, FK, UNIQUE constraints and indexes