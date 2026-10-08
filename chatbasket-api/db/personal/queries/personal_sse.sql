-- ===========================================
-- Personal SSE Outbox Operations (cluster fanout)
-- ===========================================
-- Large SSE events (>7000-byte NOTIFY payload) are stored here.
-- NOTIFY carries only the row id; the listener hydrates the full
-- event, broadcasts locally. IDs and expiry come
-- from Go (uuid.New, now+3min). Rows live until TTL so every cluster
-- node can hydrate; sweeper clears orphans and expired rows.

-- name: CreateSseOutboxRow :exec
INSERT INTO personal_sse_outbox (
    id,
    command,
    target_user_id,
    target_session_id,
    exclude_session_id,
    payload,
    expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetSseOutboxRow :one
-- Fetches only the event payload: routing fields already arrived
-- inside the NOTIFY JSON envelope, so selecting them again would
-- transfer redundant columns on every hydrate.
SELECT payload
FROM personal_sse_outbox
WHERE id = $1;

-- name: DeleteExpiredSseOutboxBatch :execrows
-- Bounded cleanup batch: deletes expired outbox orphans in small
-- batches so a huge pile makes progress instead of rolling back one
-- giant DELETE on timeout.
WITH batch AS (
  SELECT id FROM personal_sse_outbox
  WHERE expires_at < now()
  FOR UPDATE SKIP LOCKED
  LIMIT sqlc.arg('batch_size')
)
DELETE FROM personal_sse_outbox AS o
USING batch
WHERE o.id = batch.id;
