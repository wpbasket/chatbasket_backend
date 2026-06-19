-- ======================================
-- E2EE Session Key Queries
-- ======================================

-- name: SaveSessionE2EEPublicKey :one
-- Save or update the E2EE public key for a specific session (must be active)
UPDATE sessions
SET e2ee_public_key = $1, updated_at = now()
WHERE id = $2 AND auth_user_id = $3 AND expires_at > now()
RETURNING id;

-- name: GetActiveSessionKeysForUser :many
-- Fetch all active (unexpired, non-null key) public keys for a user
SELECT id, e2ee_public_key
FROM sessions
WHERE auth_user_id = $1
  AND e2ee_public_key IS NOT NULL
  AND expires_at > now();

-- name: CountActiveKeyedSessionsForUser :one
-- Count active sessions that have a key for a user
SELECT COUNT(*)
FROM sessions
WHERE auth_user_id = $1
  AND e2ee_public_key IS NOT NULL
  AND expires_at > now();

-- name: GetActiveSessionKeysForUserExcluding :many
-- Fetch all active (unexpired, non-null key) public keys for a user, excluding one session
SELECT id, e2ee_public_key
FROM sessions
WHERE auth_user_id = $1
  AND e2ee_public_key IS NOT NULL
  AND expires_at > now()
  AND id != $2;
