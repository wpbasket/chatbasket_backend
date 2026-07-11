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
-- Returns full user record along with its profile avatar file_id
SELECT u.*, a.file_id, a.token_id, a.token_secret, a.token_expiry
FROM users u
    LEFT JOIN avatars a ON a.user_id = u.id
    AND a.avatar_type = 'profile'
WHERE
    u.id = $1;

-- name: IsUserExists :one
SELECT EXISTS ( SELECT 1 FROM users WHERE id = $1 );

-- name: CreateAloneUsername :one
INSERT INTO
    alone_username (id, username)
VALUES ($1, $2)
RETURNING
    *;

-- name: GetActiveAvatar :one
-- Fetches the id and file_id for the main profile avatar
SELECT id, file_id FROM avatars WHERE user_id = $1 AND avatar_type = 'profile';

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

-- name: UpdateAvatarFileID :exec
-- Updates only the file_id for the main profile avatar (token columns unused per spec §3.C).
UPDATE avatars
SET file_id = $2
WHERE user_id = $1 AND avatar_type = 'profile';

-- name: GetAvatarFileID :one
-- Fetches the storage file_id for the main profile avatar
SELECT file_id FROM avatars WHERE user_id = $1 AND avatar_type = 'profile';

-- name: UpdateUserProfile :exec
-- Updates user profile fields
UPDATE users
SET
    name = COALESCE(sqlc.narg (name), name),
    bio = COALESCE(sqlc.narg (bio), bio),
    profile_type = COALESCE(sqlc.narg (profile_type), profile_type)
WHERE
    id = $1;

-- name: DeleteAvatar :exec
-- Deletes the main profile avatar for a user (called from ConfirmAvatarUpload tx when replacing)
DELETE FROM avatars WHERE user_id = $1 AND avatar_type = 'profile';

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
-- Fetches minimal user info for eligibility checks
SELECT id, is_admin_blocked, profile_type
FROM users
WHERE id = $1;


-- name: GetContactableProfilesForViewer :many
-- GetContactableProfilesForViewer fetches profiles for contact enrichment with privacy filtering.
SELECT
    u.id,
    u.name,
    u.b64_cipher_chacha20poly1305_username AS username,
    u.bio,
    u.profile_type,
    a.file_id,
    a.token_id,
    a.token_secret,
    a.token_expiry,
    au.keys_revision,
    COALESCE(ugr.restrict_profile, FALSE) AS global_restrict_profile,
    COALESCE(ugr.restrict_avatar, FALSE) AS global_restrict_avatar,
    COALESCE(ugre.exception_profile, FALSE) AS exception_global_profile,
    COALESCE(ugre.exception_avatar, FALSE) AS exception_global_avatar,
    COALESCE(ur.restrict_profile, FALSE) AS user_restrict_profile,
    COALESCE(ur.restrict_avatar, FALSE) AS user_restrict_avatar
FROM
    users u
    INNER JOIN auth_users au ON u.id = au.id
    LEFT JOIN avatars a ON u.id = a.user_id
    AND a.avatar_type = 'profile'
    LEFT JOIN user_global_restrictions ugr ON u.id = ugr.user_id
    LEFT JOIN user_global_restriction_exemptions ugre ON u.id = ugre.user_id
    AND ugre.exempted_user_id = sqlc.arg (viewer_user_id)
    LEFT JOIN user_restrictions ur ON u.id = ur.user_id
    AND ur.restricted_user_id = sqlc.arg (viewer_user_id)
WHERE
    u.id = ANY (
        sqlc.arg (target_user_ids)::uuid []
    )
    AND u.is_admin_blocked IS FALSE
    AND u.profile_type IN ('public', 'personal')
    AND NOT EXISTS (
        SELECT 1 FROM user_blocks ub
        WHERE (ub.blocker_user_id = sqlc.arg (viewer_user_id) AND ub.blocked_user_id = u.id)
           OR (ub.blocker_user_id = u.id AND ub.blocked_user_id = sqlc.arg (viewer_user_id))
    )
ORDER BY u.id;

-- name: GetUserByHashedUsernameForContact :one
SELECT *
FROM users
WHERE
    hmac_sha256_hex_username = $1
    AND is_admin_blocked IS NOT TRUE;

-- name: GetContactableUserIDs :many
-- Checks which target user IDs are contactable for a viewer (not blocked, not admin-blocked).
SELECT u.id
FROM users u
WHERE
    u.id = ANY (
        sqlc.arg (target_user_ids)::uuid []
    )
    AND u.is_admin_blocked IS FALSE
    AND NOT EXISTS (
        SELECT 1 FROM user_blocks ub
        WHERE (ub.blocker_user_id = sqlc.arg (viewer_user_id) AND ub.blocked_user_id = u.id)
           OR (ub.blocker_user_id = u.id AND ub.blocked_user_id = sqlc.arg (viewer_user_id))
    )
ORDER BY u.id;


-- name: CreateUserBlock :exec
INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id)
VALUES ($1, $2, $3)
ON CONFLICT (blocker_user_id, blocked_user_id) DO NOTHING;

-- name: IsEitherBlocked :one
-- Returns 0 if no block, 1 if blocker is $1 (requester blocked target), 2 if blocker is $2 (target blocked requester)
SELECT (CASE
    WHEN EXISTS(SELECT 1 FROM user_blocks ub1 WHERE ub1.blocker_user_id = $1 AND ub1.blocked_user_id = $2) THEN 1
    WHEN EXISTS(SELECT 1 FROM user_blocks ub2 WHERE ub2.blocker_user_id = $2 AND ub2.blocked_user_id = $1) THEN 2
    ELSE 0
END)::INT;