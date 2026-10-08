-- +migrate Down
DROP INDEX IF EXISTS idx_personal_sse_outbox_expires_at;
DROP TABLE IF EXISTS personal_sse_outbox;
