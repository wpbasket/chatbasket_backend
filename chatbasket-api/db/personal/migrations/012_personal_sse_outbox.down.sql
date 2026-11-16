-- +migrate Down

-- Drop personal_sse_outbox
DROP INDEX IF EXISTS idx_personal_sse_outbox_expires_at;        -- TTL sweeper index
DROP TABLE IF EXISTS personal_sse_outbox CASCADE;               -- Also drops PK constraint
