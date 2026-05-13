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

-- name: UpdateAuthUserSignup :exec
-- Updates an existing unverified user during signup to reuse the ID and preserve rate limits
UPDATE auth_users SET name = $2, password_hash = $3, updated_at = now() WHERE id = $1;

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
        expires_at,
        device_token,
        platform,
        device_name,
        is_central
    )
VALUES (
        $1,
        $2,
        $3,
        $4,
        $5,
        $6,
        $7,
        $8,
        $9,
        $10
    )
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

-- name: ResetCentralSessions :exec
-- Sets is_central = false for ALL sessions of this user (Upgrade preparation)
UPDATE sessions SET is_central = FALSE WHERE auth_user_id = $1;

-- name: SetSessionCentral :exec
-- Sets is_central = true for a specific session
UPDATE sessions
SET
    is_central = TRUE
WHERE
    id = $1
    AND auth_user_id = $2;

-- name: SetSessionCentralByToken :exec
-- Sets is_central = true for a session identified by token hash
UPDATE sessions
SET
    is_central = TRUE
WHERE
    token_hash = $1
    AND auth_user_id = $2;

-- name: CheckHasCentralDevice :one
-- Checks if the user already has a central device
SELECT EXISTS (
        SELECT 1
        FROM sessions
        WHERE
            auth_user_id = $1
            AND is_central = TRUE
    );

-- name: UpdateSessionDeviceToken :exec
UPDATE sessions
SET
    device_token = $1,
    platform = $2,
    device_name = $3,
    updated_at = now()
WHERE
    token_hash = $4
    AND auth_user_id = $5;

-- name: GetCentralSession :one
-- Returns the details of the central session for a user
SELECT *
FROM sessions
WHERE
    auth_user_id = $1
    AND is_central = TRUE
LIMIT 1;

-- name: GetSessionByToken :one
SELECT * FROM sessions WHERE token_hash = $1 AND auth_user_id = $2;

-- name: GetUserPrimarySession :one
-- Returns user's primary device session (for Phase 6 messaging eligibility)
SELECT *
FROM sessions
WHERE
    auth_user_id = $1
    AND is_central = TRUE
    AND expires_at > now()
LIMIT 1;

-- ======================================
-- Rate Limiter Queries
-- ======================================

-- name: GetAuthRateLimiter :one
SELECT * FROM auth_rate_limiters WHERE auth_user_id = $1;

-- name: UpsertAuthRateLimiterSend :one
INSERT INTO
    auth_rate_limiters (
        auth_user_id,
        otp_hourly_count,
        otp_24h_count,
        last_otp_send_at,
        otp_24h_window_start_at
    )
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (auth_user_id) DO
UPDATE
SET
    otp_hourly_count = EXCLUDED.otp_hourly_count,
    otp_24h_count = EXCLUDED.otp_24h_count,
    last_otp_send_at = EXCLUDED.last_otp_send_at,
    otp_24h_window_start_at = EXCLUDED.otp_24h_window_start_at,
    updated_at = now()
RETURNING
    *;

-- name: UpsertAuthRateLimiterVerify :one
INSERT INTO
    auth_rate_limiters (
        auth_user_id,
        otp_verify_errors,
        last_verify_attempt_at
    )
VALUES ($1, $2, $3)
ON CONFLICT (auth_user_id) DO
UPDATE
SET
    otp_verify_errors = EXCLUDED.otp_verify_errors,
    last_verify_attempt_at = EXCLUDED.last_verify_attempt_at,
    updated_at = now()
RETURNING
    *;

-- name: ResetVerifyErrors :exec
UPDATE auth_rate_limiters
SET
    otp_verify_errors = 0,
    last_verify_attempt_at = NULL
WHERE
    auth_user_id = $1;