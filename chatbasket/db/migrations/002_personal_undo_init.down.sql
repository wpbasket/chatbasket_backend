-- +migrate Down

-- ======================================
-- Undo script for 001_personal_init.up.sql
-- Drops all tables, indexes, functions, and triggers created in the initial migration
-- ======================================

-- Cleanup avatars table and related objects
DROP INDEX IF EXISTS idx_avatars_user_id;        -- User avatars index
DROP INDEX IF EXISTS idx_avatars_type;           -- Avatar type index
DROP TRIGGER IF EXISTS avatars_timestamps_trigger ON avatars;  -- Timestamp trigger
DROP TABLE IF EXISTS avatars CASCADE;            -- Also drops PK, FK, UNIQUE constraints and indexes

-- Cleanup alone_username table and related objects
DROP TRIGGER IF EXISTS alone_username_timestamps_trigger ON alone_username;  -- Timestamp trigger
DROP TABLE IF EXISTS alone_username CASCADE;     -- Also drops PK, UNIQUE constraints and indexes

-- Cleanup users table and related objects
DROP INDEX IF EXISTS idx_users_is_admin_blocked; -- Admin block status index
DROP INDEX IF EXISTS idx_users_profile_type;     -- Profile type index
DROP TRIGGER IF EXISTS users_timestamps_trigger ON users;      -- Timestamp trigger
DROP TABLE IF EXISTS users CASCADE;              -- Also drops PK, UNIQUE constraints and indexes

-- Cleanup functions
DROP FUNCTION IF EXISTS set_timestamps();        -- Timestamp management function