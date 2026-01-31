-- ======================================
-- Auth Tables Queries for sqlc
-- ======================================

-- name: CreateAuthUser :one
-- Inserts a new auth user and returns all columns
INSERT INTO
    auth_users (
        id,
        name,
        email,
        password_hash,
        is_email_verified
    )
VALUES ($1, $2, $3, $4, $5)
RETURNING
    *;

-- name: GetAuthUserByEmail :one
-- Returns auth user by email (for login)
SELECT * FROM auth_users WHERE email = $1;

-- name: GetAuthUserByID :one
-- Returns auth user by ID (same as user ID)
SELECT * FROM auth_users WHERE id = $1;

-- name: UpdateAuthUserEmailVerified :exec
-- Marks user email as verified
UPDATE auth_users SET is_email_verified = $2 WHERE id = $1;

-- ======================================
-- Sessions Table Queries
-- ======================================

-- name: CreateSession :one
-- Creates a new session and returns all columns
INSERT INTO
    sessions (
        id,
        auth_user_id,
        token_hash,
        user_agent,
        ip_address,
        expires_at
    )
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING
    *;

-- name: GetSessionByTokenHash :one
-- Returns session by token hash (for session validation)
SELECT * FROM sessions WHERE token_hash = $1;

-- name: DeleteSession :exec
-- Deletes a session by ID (for logout)
DELETE FROM sessions WHERE id = $1;

-- name: DeleteAllSessionsForUser :exec
-- Deletes all sessions for a user (for logout from all devices)
DELETE FROM sessions WHERE auth_user_id = $1;

-- name: DeleteExpiredSessions :exec
-- Cleanup query: deletes expired sessions
DELETE FROM sessions WHERE expires_at < now();

-- ======================================
-- Verification Codes Table Queries
-- ======================================

-- name: CreateVerificationCode :one
-- Creates a new verification code and returns all columns
INSERT INTO
    verification_codes (
        id,
        auth_user_id,
        email,
        code_hash,
        type,
        expires_at
    )
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING
    *;

-- name: GetVerificationCodeByEmailAndType :one
-- Returns the latest verification code for email and type
SELECT *
FROM verification_codes
WHERE
    email = $1
    AND type = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: DeleteVerificationCode :exec
-- Deletes a verification code by ID
DELETE FROM verification_codes WHERE id = $1;

-- name: DeleteVerificationCodesByEmailAndType :exec
-- Deletes all verification codes for email and type (cleanup after verification)
DELETE FROM verification_codes WHERE email = $1 AND type = $2;

-- name: DeleteExpiredVerificationCodes :exec
-- Cleanup query: deletes expired verification codes
DELETE FROM verification_codes WHERE expires_at < now();