-- +migrate Down
ALTER TABLE avatars ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'normal';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_name = 'avatars' AND constraint_name = 'avatars_status_check'
    ) THEN
        ALTER TABLE avatars ADD CONSTRAINT avatars_status_check
            CHECK (status IN ('normal', 'stale'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_avatars_status_user
    ON avatars (status, user_id)
    WHERE status = 'stale';

DROP INDEX IF EXISTS idx_avatars_user_profile;

CREATE UNIQUE INDEX IF NOT EXISTS idx_avatars_user_profile
    ON avatars (user_id, avatar_type)
    WHERE avatar_type = 'profile' AND status = 'normal';
