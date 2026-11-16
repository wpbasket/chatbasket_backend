-- +migrate Down

-- Drop history_sync
DROP TRIGGER IF EXISTS history_sync_timestamps_trigger ON history_sync; -- Timestamp trigger
DROP INDEX IF EXISTS idx_history_sync_pending;                            -- Pending sync replay index
DROP INDEX IF EXISTS idx_history_sync_expires;                            -- TTL sweeper index
DROP INDEX IF EXISTS idx_history_sync_user;                               -- User lookup index
DROP TABLE IF EXISTS history_sync CASCADE;                                -- Also drops PK and UNIQUE constraints
