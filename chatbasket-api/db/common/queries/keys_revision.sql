-- ======================================
-- Keys Revision Queries
-- ======================================

-- name: GetKeysRevision :one
-- Get the current keys_revision for a user (atomic read)
SELECT keys_revision FROM auth_users WHERE id = $1 FOR UPDATE;

-- name: IncrementKeysRevision :exec
-- Increment keys_revision by one (atomic)
UPDATE auth_users SET keys_revision = keys_revision + 1 WHERE id = $1;

-- name: ResetKeysRevision :exec
-- Reset keys_revision to zero
UPDATE auth_users SET keys_revision = 0 WHERE id = $1;
