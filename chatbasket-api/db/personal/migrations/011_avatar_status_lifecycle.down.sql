-- +migrate Down

DROP INDEX IF EXISTS idx_avatars_status_user;
ALTER TABLE avatars DROP CONSTRAINT IF EXISTS avatars_status_check;
ALTER TABLE avatars DROP COLUMN IF EXISTS status;

-- Revert unique index to exclude status filter
DROP INDEX IF EXISTS idx_avatars_user_profile;
CREATE UNIQUE INDEX IF NOT EXISTS idx_avatars_user_profile
    ON avatars (user_id, avatar_type)
    WHERE avatar_type = 'profile';

