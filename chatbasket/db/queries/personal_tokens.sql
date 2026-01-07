-- ======================================
-- Tokens Table Queries for sqlc
-- ======================================

-- name: UpsertToken :one
-- Inserts or updates a push notification token for a user session
-- If the (session_id, user_id, type) combination exists, updates the token and marks it active
INSERT INTO
    tokens (
        id,
        user_id,
        sha256_hex_session_id,
        token,
        type,
        is_active
    )
VALUES ($1, $2, $3, $4, $5, TRUE)
ON CONFLICT (
    sha256_hex_session_id,
    user_id,
    type
) DO
UPDATE
SET
    token = EXCLUDED.token,
    is_active = TRUE,
    updated_at = now()
RETURNING
    *;

-- name: DeactivateSessionTokens :exec
-- Marks all tokens for a specific session as inactive (useful for logout)
UPDATE tokens
SET
    is_active = FALSE,
    updated_at = now()
WHERE
    sha256_hex_session_id = $1
    AND user_id = $2;

-- name: DeactivateUserTokens :exec
-- Marks all tokens for a user as inactive (useful for logout from all sessions)
UPDATE tokens
SET
    is_active = FALSE,
    updated_at = now()
WHERE
    user_id = $1;

-- name: GetActiveTokensByUser :many
-- Returns all active tokens for a user (for sending push notifications)
SELECT *
FROM tokens
WHERE
    user_id = $1
    AND is_active = TRUE
ORDER BY created_at DESC;

-- name: GetActiveTokensByType :many
-- Returns all active tokens for a user filtered by type (fcm or apn)
SELECT *
FROM tokens
WHERE
    user_id = $1
    AND type = $2
    AND is_active = TRUE
ORDER BY created_at DESC;

-- name: DeleteInactiveTokens :exec
-- Cleanup query: deletes tokens that have been inactive for a specified period
DELETE FROM tokens WHERE is_active = FALSE AND updated_at < $1;