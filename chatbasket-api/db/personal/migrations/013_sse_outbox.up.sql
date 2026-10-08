-- +migrate Up
-- ======================================
-- Table: personal_sse_outbox
-- Cluster fanout for SSE events larger than the 8000-byte
-- Postgres NOTIFY cap. Publisher inserts the full event,
-- NOTIFY carries only the row id. Listener hydrates and
-- broadcasts locally. Rows live until TTL so all cluster nodes
-- can hydrate; sweeper clears orphans and expired rows.
-- IDs are generated in Go (uuid.New), no DB default.
-- ======================================
CREATE TABLE IF NOT EXISTS personal_sse_outbox (
  id UUID PRIMARY KEY,
  command TEXT NOT NULL,
  target_user_id UUID NOT NULL,
  target_session_id UUID,
  exclude_session_id UUID,
  payload BYTEA NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_personal_sse_outbox_expires_at ON personal_sse_outbox (expires_at);
