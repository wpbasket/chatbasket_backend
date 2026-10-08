-- +migrate Up
-- ======================================
-- Table: personal_sse_outbox
--        Cluster fanout for SSE events larger than the 8000-byte
--        Postgres NOTIFY cap. Publisher inserts the full event,
--        NOTIFY carries only the row id. Listener hydrates and
--        broadcasts locally. Rows live until TTL so all cluster nodes
--        can hydrate; sweeper clears orphans and expired rows.
--        IDs are generated in Go (uuid.New), no DB default.
-- ======================================
CREATE TABLE IF NOT EXISTS personal_sse_outbox (
    id                  UUID         PRIMARY KEY,                       -- Go-generated UUID (uuid.New)
    command             TEXT         NOT NULL,                          -- SSE command name
    target_user_id      UUID         NOT NULL,                          -- Recipient user ID
    target_session_id   UUID,                                           -- Optional target session ID
    exclude_session_id  UUID,                                           -- Optional sender session ID to skip
    payload             BYTEA        NOT NULL,                          -- Serialized event payload
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),            -- Event creation timestamp
    expires_at          TIMESTAMPTZ  NOT NULL                           -- TTL timestamp; swept by background worker
);

-- Index for the periodic TTL cleanup sweeper
CREATE INDEX IF NOT EXISTS idx_personal_sse_outbox_expires_at
    ON personal_sse_outbox (expires_at);

-- ======================================
-- End of personal_sse_outbox table section
-- ======================================
