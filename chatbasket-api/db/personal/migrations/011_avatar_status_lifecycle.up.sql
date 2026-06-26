-- +migrate Up
-- ======================================
-- Avatar status column (default 'normal') with two-phase cleanup support
-- ======================================
-- Avatar rows have a 'status' field:
--   - 'normal': active avatar visible to users (default)
--   - 'stale':   pending cleanup by the background sweeper
--
-- Cleanup flow (hybrid inline + sweeper):
--   1. ConfirmAvatarUpload tx: insert new 'normal' row
--   2. Post-commit inline: try R2 delete of old file (best-effort)
--   3a. If R2 delete succeeds → DELETE old DB row (status was 'normal', now gone)
--   3b. If R2 delete fails → mark old DB row as 'stale' (sweeper will retry later)
--   4. Background sweeper: scans for 'stale' rows → R2 delete → DB delete
--
-- All read queries filter by status='normal' so users never see stale rows.

ALTER TABLE avatars ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'normal';

-- Constraint: only valid statuses allowed
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

-- Index for sweeper (finds stale rows efficiently)
CREATE INDEX IF NOT EXISTS idx_avatars_status_user
    ON avatars (status, user_id)
    WHERE status = 'stale';

-- Drop the old unique index that restricts one avatar row per user
DROP INDEX IF EXISTS idx_avatars_user_profile;

-- Recreate the unique index to allow only one active ('normal') avatar per user
CREATE UNIQUE INDEX IF NOT EXISTS idx_avatars_user_profile
    ON avatars (user_id, avatar_type)
    WHERE avatar_type = 'profile' AND status = 'normal';

-- ======================================
-- End of avatar status column
-- ======================================

