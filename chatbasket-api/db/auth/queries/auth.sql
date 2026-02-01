-- ======================================
-- Auth Queries
-- ======================================

-- name: CheckSessionIsValid :one
-- Checks if a session is valid for a specific user and not expired
SELECT EXISTS (
        SELECT 1
        FROM sessions
        WHERE
            token_hash = $1
            AND auth_user_id = $2
            AND expires_at > now()
    );

-- name: GetAuthUserByID :one
-- Returns auth user by ID
SELECT * FROM auth_users WHERE id = $1;

-- name: GetAuthUserByEmail :one
SELECT * FROM auth_users WHERE email = $1;

-- name: CheckEmailExists :one
SELECT EXISTS ( SELECT 1 FROM auth_users WHERE email = $1 );

-- name: CreateAuthUser :one
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

-- name: UpdateAuthUserEmailVerified :exec
UPDATE auth_users SET is_email_verified = $2 WHERE id = $1;

-- name: DeleteAuthUser :exec
-- Delete auth user (cascade will automatically delete sessions and verification_codes)
DELETE FROM auth_users WHERE id = $1;

-- name: CreateSession :one
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

-- name: CreateVerificationCode :one
INSERT INTO
    verification_codes (id, email, code_hash, type)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO
UPDATE
SET
    email = EXCLUDED.email,
    code_hash = EXCLUDED.code_hash,
    type = EXCLUDED.type,
    created_at = now()
RETURNING
    *;

-- name: GetVerificationCode :one
-- Get the verification code by User ID (PK) and Type
SELECT * FROM verification_codes WHERE id = $1 AND type = $2;

-- name: DeleteVerificationCode :exec
DELETE FROM verification_codes WHERE id = $1;

-- name: DeleteAllVerificationCodesForEmail :exec
DELETE FROM verification_codes WHERE email = $1 AND type = $2;

-- name: UpdateAuthUserPassword :exec
UPDATE auth_users SET password_hash = $2 WHERE id = $1;

-- name: UpdateAuthUserEmail :exec
UPDATE auth_users
SET
    email = $2,
    is_email_verified = $3
WHERE
    id = $1;

-- name: CreateVerificationCodeWithUpdateID :one
INSERT INTO
    verification_codes (
        id,
        update_id,
        email,
        code_hash,
        type
    )
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO
UPDATE
SET
    update_id = EXCLUDED.update_id,
    email = EXCLUDED.email,
    code_hash = EXCLUDED.code_hash,
    type = EXCLUDED.type,
    created_at = now()
RETURNING
    *;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1 AND auth_user_id = $2;

-- name: DeleteAllUserSessions :exec
DELETE FROM sessions WHERE auth_user_id = $1;
-- name: DeleteSessionByToken :exec
DELETE FROM sessions WHERE token_hash = $1 AND auth_user_id = $2;