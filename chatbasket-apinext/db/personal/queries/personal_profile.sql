-- ======================================
-- Profile Table Queries for sqlc
-- ======================================

-- name: CreateUser :one
-- Inserts a new user and returns all columns
INSERT INTO
    users (
        id,
        name,
        b64_cipher_chacha20poly1305_username,
        hmac_sha256_hex_username,
        profile_type
    )
VALUES ($1, $2, $3, $4, $5)
RETURNING
    *;

-- name: GetUserProfile :one
-- Returns full user record along with its profile avatar tokens and file_id
SELECT u.*, a.file_id, a.token_id, a.token_secret, a.token_expiry
FROM users u
    LEFT JOIN avatars a ON a.user_id = u.id
    AND a.file_id = u.id::TEXT -- main profile avatar (file_id == user_id)
WHERE
    u.id = $1;

-- name: ListUsersAfter :many
-- Returns users created before a certain timestamp (keyset pagination)
SELECT *
FROM users
WHERE
    created_at < $1
ORDER BY created_at DESC
LIMIT $2;

-- name: IsUserExists :one
SELECT EXISTS ( SELECT 1 FROM users WHERE id = $1 );

-- name: CreateAloneUsername :one
INSERT INTO
    alone_username (id, username)
VALUES ($1, $2)
RETURNING
    *;

-- name: IsUserProfilePicExists :one
-- Checks if the user exists and has a main profile picture
SELECT EXISTS (
        SELECT 1
        FROM users u
            JOIN avatars a ON a.user_id = u.id
        WHERE
            u.id = $1
            AND a.file_id = u.id::TEXT
    );

-- name: CreateAvatar :one
-- Inserts a new avatar and returns all columns
INSERT INTO
    avatars (
        id,
        user_id,
        file_id,
        avatar_type,
        token_id,
        token_secret,
        token_expiry
    )
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING
    *;

-- name: UpdateAvatarTokens :one
-- Updates token_id, token_secret, and token_expiry for the main profile avatar (where user_id == file_id)
UPDATE avatars
SET
    token_id = $2,
    token_secret = $3,
    token_expiry = $4
WHERE
    user_id = $1
    AND file_id = $1::TEXT
RETURNING
    *;

-- name: UpdateUserProfile :one
-- Updates user profile fields conditionally based on provided values (NULL values are ignored)
UPDATE users
SET
    name = COALESCE(sqlc.narg ('name'), name),
    bio = COALESCE(sqlc.narg ('bio'), bio),
    profile_type = COALESCE(
        sqlc.narg ('profile_type'),
        profile_type
    )
WHERE
    id = $1
RETURNING
    *;

-- name: DeleteAvatar :exec
-- Deletes the main profile avatar for a user
DELETE FROM avatars WHERE user_id = $1 AND file_id = $1::TEXT;

-- name: IsUserAdminBlocked :one
-- Returns true if the user is admin-blocked
SELECT EXISTS (
        SELECT 1
        FROM users
        WHERE
            id = $1
            AND is_admin_blocked IS TRUE
    );

-- name: GetUserCoreProfile :one
-- Minimal user profile without avatar join
SELECT * FROM users WHERE id = $1;

-- name: GetProfilesForContactViewer :many
SELECT
    u.id,
    u.name,
    u.b64_cipher_chacha20poly1305_username AS username,
    u.bio,
    a.file_id,
    a.token_id,
    a.token_secret,
    a.token_expiry,
    COALESCE(ugr.restrict_profile, FALSE) AS global_restrict_profile,
    COALESCE(ugr.restrict_avatar, FALSE) AS global_restrict_avatar,
    COALESCE(ugre.exception_profile, FALSE) AS exception_global_profile,
    COALESCE(ugre.exception_avatar, FALSE) AS exception_global_avatar,
    COALESCE(ur.restrict_profile, FALSE) AS user_restrict_profile,
    COALESCE(ur.restrict_avatar, FALSE) AS user_restrict_avatar
FROM users u
LEFT JOIN avatars a
    ON u.id = a.user_id
   AND a.avatar_type = 'profile'
LEFT JOIN user_global_restrictions ugr
    ON u.id = ugr.user_id
LEFT JOIN user_global_restriction_exemptions ugre
    ON u.id = ugre.user_id
   AND ugre.exempted_user_id = sqlc.arg(viewer_user_id)
LEFT JOIN user_restrictions ur
    ON u.id = ur.user_id
   AND ur.restricted_user_id = sqlc.arg(viewer_user_id)
WHERE u.id = ANY(sqlc.arg(target_user_ids)::uuid[])
  AND u.is_admin_blocked IS FALSE
ORDER BY u.id;

-- name: GetUserByHashedUsernameForContact :one
SELECT *
FROM users
WHERE hmac_sha256_hex_username = $1
  AND is_admin_blocked IS NOT TRUE;
