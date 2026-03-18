-- +migrate Down

-- Drop alone_username
DROP TRIGGER IF EXISTS alone_username_timestamps_trigger ON alone_username;  -- Timestamp trigger
DROP TABLE IF EXISTS alone_username CASCADE;                        -- Also drops PK, UNIQUE constraints and indexes

-- Drop avatars
DROP TRIGGER IF EXISTS avatars_timestamps_trigger ON avatars;       -- Timestamp trigger
DROP INDEX IF EXISTS idx_avatars_user_profile;                      -- Unique profile avatar per user index
DROP INDEX IF EXISTS idx_avatars_user_type_created;                -- User, type, and creation date composite index
DROP INDEX IF EXISTS idx_avatars_token_expiry;                      -- Token expiry cleanup index
DROP TABLE IF EXISTS avatars CASCADE;                               -- Also drops PK, FK, UNIQUE constraints and indexes

-- Drop users
DROP TRIGGER IF EXISTS users_timestamps_trigger ON users;           -- Timestamp trigger
DROP INDEX IF EXISTS idx_users_profile_type_admin_blocked;          -- Composite profile type and admin block index
DROP INDEX IF EXISTS idx_users_admin_blocked_only;                  -- Admin blocked users index
DROP INDEX IF EXISTS idx_users_profile_created;                     -- Profile type and creation date index
DROP TABLE IF EXISTS users CASCADE;                                 -- Also drops PK, UNIQUE constraints and indexes

-- Drop timestamp function
DROP FUNCTION IF EXISTS set_timestamps();                           -- Timestamp management function
