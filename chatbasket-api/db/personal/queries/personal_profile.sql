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
-- GetContactableProfilesForViewer fetches profiles for contact enrichment with
-- privacy filtering. The returned rows are used by the chat list, contact list,
-- contact requests, and other surfaces to populate the "other user" identity
-- block (name, username, bio, profile_type, avatar trio, keys revision).
--
-- Privacy exclusion is binary per user: a user is either IN the result set
-- (all six identity fields returned) or OUT of it (no row at all, the
-- caller treats them as missing). An excluded user means the wire payload
-- the caller builds will have name/username/profile_type as `""` and
-- bio/avatar_url/avatar_file_id as `null`. The frontend
-- (upsertFromServer in $userProfilesState) treats these wire encodings
-- differently on purpose:
--   - `""` on name/username is "no data this round" (frontend preserves the
--     owner's prior-known identity so the chat list stays identifiable).
--   - `""` on profile_type IS authoritative clear (frontend would otherwise
--     render a misleading "Public" badge on the user profile screen).
--   - `null` on bio/avatar_url/avatar_file_id IS authoritative clear.
--   - Omitted keys preserve the prior value (partial-payload case).
-- This split is what lets the chat list keep showing a prior-known name
-- while the user profile screen shows no stale profile state.
--
-- The WHERE clause is the privacy contract. Each condition excludes a
-- different privacy-exclusion case; the caller does not need to know which
-- one fired, the wire shape is identical for all of them:
--   - `u.is_admin_blocked IS FALSE`           : admin removed this user.
--   - `u.profile_type IN ('public', 'personal')` : the user switched to a
--                                                  private profile.
--   - `NOT EXISTS (... user_blocks ...)`      : either side user-blocked
--                                                  the other.
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
    COALESCE(ugr.restrict_profile, FALSE) AS global_restrict_profile,
    COALESCE(ugr.restrict_avatar, FALSE) AS global_restrict_avatar,
    COALESCE(ugre.exception_profile, FALSE) AS exception_global_profile,
    COALESCE(ugre.exception_avatar, FALSE) AS exception_global_avatar,
    COALESCE(ur.restrict_profile, FALSE) AS user_restrict_profile,
    COALESCE(ur.restrict_avatar, FALSE) AS user_restrict_avatar
FROM
    users u
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
    AND u.profile_type != 'private'
    AND NOT EXISTS (
        SELECT 1 FROM user_blocks ub
        WHERE (ub.blocker_user_id = sqlc.arg (viewer_user_id) AND ub.blocked_user_id = u.id)
           OR (ub.blocker_user_id = u.id AND ub.blocked_user_id = sqlc.arg (viewer_user_id))
    )
ORDER BY u.id;

-- name: GetBlockListProfilesForViewer :many
-- Fetches profiles for the block list. Admin-blocked and private targets are
-- omitted as whole items; a target-side block is returned so the service can
-- retain identity fields while hiding bio and avatar fields.
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
    COALESCE(ugr.restrict_profile, FALSE) AS global_restrict_profile,
    COALESCE(ugr.restrict_avatar, FALSE) AS global_restrict_avatar,
    COALESCE(ugre.exception_profile, FALSE) AS exception_global_profile,
    COALESCE(ugre.exception_avatar, FALSE) AS exception_global_avatar,
    COALESCE(ur.restrict_profile, FALSE) AS user_restrict_profile,
    COALESCE(ur.restrict_avatar, FALSE) AS user_restrict_avatar,
    EXISTS (
        SELECT 1
        FROM user_blocks ub
        WHERE ub.blocker_user_id = u.id
          AND ub.blocked_user_id = sqlc.arg (viewer_user_id)
    ) AS target_blocked_viewer
FROM
    users u
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
    AND u.profile_type != 'private'
ORDER BY u.id;

-- name: GetUserByHashedUsernameForContact :one
SELECT *
FROM users
WHERE
    hmac_sha256_hex_username = $1
    AND is_admin_blocked IS NOT TRUE;

-- name: GetContactableUserIDs :many
-- Returns just the subset of target_user_ids that pass the contactable
-- filter for chat message filtering. It uses the same bidirectional block
-- exclusion as GetContactableProfilesForViewer; the chat endpoint needs to
-- hide messages from users blocked in either direction.
-- Also excludes admin-blocked users.
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


-- name: GetUserBlocks :many
-- Returns the users blocked by a given blocker, newest first.
SELECT blocked_user_id, created_at
FROM user_blocks
WHERE blocker_user_id = $1
ORDER BY created_at DESC;

-- name: CreateUserBlock :exec
INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id)
VALUES ($1, $2, $3)
ON CONFLICT (blocker_user_id, blocked_user_id) DO NOTHING;

-- name: DeleteUserBlock :exec
DELETE FROM user_blocks
WHERE blocker_user_id = $1 AND blocked_user_id = $2;

-- name: IsEitherBlocked :one
-- Returns 0 if no block, 1 if blocker is $1 (requester blocked target), 2 if blocker is $2 (target blocked requester)
SELECT (CASE
    WHEN EXISTS(SELECT 1 FROM user_blocks ub1 WHERE ub1.blocker_user_id = $1 AND ub1.blocked_user_id = $2) THEN 1
    WHEN EXISTS(SELECT 1 FROM user_blocks ub2 WHERE ub2.blocker_user_id = $2 AND ub2.blocked_user_id = $1) THEN 2
    ELSE 0
END)::INT;

-- name: IsBlockedBetweenUsers :one
-- Returns block status flags between two users as individual boolean columns.
SELECT
    u_req.is_admin_blocked                                                                                                                         AS requester_admin_blocked,
    u_tgt.is_admin_blocked                                                                                                                         AS target_admin_blocked,
    (u_tgt.profile_type = 'private')                                                                                                               AS is_target_profile_private,
    EXISTS (SELECT 1 FROM user_blocks ub WHERE ub.blocker_user_id = sqlc.arg(target_user_id)    AND ub.blocked_user_id = sqlc.arg(requester_user_id)) AS requester_user_blocked_by_target,
    EXISTS (SELECT 1 FROM user_blocks ub WHERE ub.blocker_user_id = sqlc.arg(requester_user_id) AND ub.blocked_user_id = sqlc.arg(target_user_id))    AS target_user_blocked_by_requester
FROM users u_req
JOIN users u_tgt ON u_tgt.id = sqlc.arg(target_user_id)
WHERE u_req.id = sqlc.arg(requester_user_id);

-- name: IsBlockedBetweenUsersBatch :many
-- Returns block status flags per target user for a requester and target user list.
SELECT
    t.target_id::uuid AS target_id,
    COALESCE(u_req.is_admin_blocked, FALSE) AS requester_admin_blocked,
    COALESCE(u_target.is_admin_blocked, FALSE) AS target_admin_blocked,
    (COALESCE(u_target.profile_type, '') = 'private') AS is_target_profile_private,
    EXISTS (
        SELECT 1 FROM user_blocks ub
        WHERE ub.blocker_user_id = t.target_id
          AND ub.blocked_user_id = sqlc.arg(requester_user_id)
    ) AS requester_user_blocked_by_target,
    EXISTS (
        SELECT 1 FROM user_blocks ub
        WHERE ub.blocker_user_id = sqlc.arg(requester_user_id)
          AND ub.blocked_user_id = t.target_id
    ) AS target_user_blocked_by_requester
FROM unnest(sqlc.arg(target_user_ids)::uuid[]) AS t(target_id)
LEFT JOIN users u_req ON u_req.id = sqlc.arg(requester_user_id)
LEFT JOIN users u_target ON u_target.id = t.target_id;

-- name: IsBlockedByAdminOrPrivate :one
-- Returns block status for target user. Returns no rows if target user does not exist.
SELECT
    u_req.is_admin_blocked AS requester_admin_blocked,
    u_tgt.is_admin_blocked AS target_admin_blocked,
    (u_tgt.profile_type = 'private') AS is_target_profile_private
FROM users u_req
JOIN users u_tgt ON u_tgt.id = sqlc.arg(target_user_id)
WHERE u_req.id = sqlc.arg(requester_user_id);