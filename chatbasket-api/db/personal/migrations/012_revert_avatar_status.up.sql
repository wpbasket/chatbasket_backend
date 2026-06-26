-- +migrate Up
-- Revert avatar status lifecycle (now handled by pending_uploads)
DROP INDEX IF EXISTS idx_avatars_status_user;
DROP INDEX IF EXISTS idx_avatars_user_profile;
ALTER TABLE avatars DROP CONSTRAINT IF EXISTS avatars_status_check;
ALTER TABLE avatars DROP COLUMN IF EXISTS status;
CREATE UNIQUE INDEX IF NOT EXISTS idx_avatars_user_profile
    ON avatars (user_id, avatar_type) WHERE avatar_type = 'profile';
