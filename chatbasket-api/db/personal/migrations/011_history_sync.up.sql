CREATE TABLE IF NOT EXISTS history_sync (
    id          UUID         PRIMARY KEY,                       -- request_id (Go-generated)
    user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id  UUID         NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    chats_json  JSONB        NOT NULL,                          -- V3 envelope ciphertext (secondary's encrypted request)
    payload     JSONB,                                          -- V3 envelope ciphertext (primary's encrypted response); NULL until ③
    expires_at  TIMESTAMPTZ  NOT NULL,                          -- 10-min TTL (HistorySyncTTL)
    created_at  TIMESTAMPTZ  NOT NULL,
    updated_at  TIMESTAMPTZ  NOT NULL
);

-- One active request per secondary session (replace rule target)
CREATE UNIQUE INDEX IF NOT EXISTS idx_history_sync_session
    ON history_sync (session_id);

-- ② primary notify + ③ upload auth
CREATE INDEX IF NOT EXISTS idx_history_sync_user
    ON history_sync (user_id);

-- TTL sweep
CREATE INDEX IF NOT EXISTS idx_history_sync_expires
    ON history_sync (expires_at);

-- Replay unanswered asks to a primary that was offline when ① arrived
CREATE INDEX IF NOT EXISTS idx_history_sync_pending
    ON history_sync (user_id)
    WHERE payload IS NULL;

DROP TRIGGER IF EXISTS history_sync_timestamps_trigger ON history_sync;
CREATE TRIGGER history_sync_timestamps_trigger
BEFORE INSERT OR UPDATE ON history_sync
FOR EACH ROW EXECUTE FUNCTION set_timestamps();
