-- ===========================================
-- Contacts Queries for sqlc
-- ===========================================

-- name: GetUserContacts :many
-- Retrieves user contacts (people YOU added) with raw restriction data for Go processing
SELECT
    cu.id,
    cu.name,
    cu.b64_cipher_chacha20poly1305_username AS username,
    cu.bio,
    uc.created_at AS contact_created_at,
    uc.updated_at AS contact_updated_at,
    
    -- Raw avatar data (Go applies visibility logic)
    a.file_id AS avatar_file_id,
    a.token_id AS avatar_token_id,
    a.token_secret AS avatar_token_secret,
    a.token_expiry AS avatar_token_expiry,
    
    -- Global restriction flags (Priority 1 & 2)
    COALESCE(ugr.restrict_profile, FALSE) AS global_restrict_profile,
    COALESCE(ugr.restrict_avatar, FALSE) AS global_restrict_avatar,
    
    -- Global exemption flags (Priority 1 & 2 override)
    COALESCE(ugre.exception_profile, FALSE) AS exception_global_profile,
    COALESCE(ugre.exception_avatar, FALSE) AS exception_global_avatar,
    
    -- User-level restriction flags (Priority 3 & 4)
    COALESCE(ur.restrict_profile, FALSE) AS user_restrict_profile,
    COALESCE(ur.restrict_avatar, FALSE) AS user_restrict_avatar

FROM user_contacts uc
INNER JOIN users cu 
    ON uc.contact_user_id = cu.id 
    AND cu.is_admin_blocked IS FALSE
    AND cu.profile_type IN ('public', 'personal')
LEFT JOIN avatars a 
    ON cu.id = a.user_id 
    AND a.avatar_type = 'profile'
LEFT JOIN user_global_restrictions ugr 
    ON cu.id = ugr.user_id
LEFT JOIN user_global_restriction_exemptions ugre 
    ON cu.id = ugre.user_id 
    AND ugre.exempted_user_id = $1
LEFT JOIN user_restrictions ur 
    ON cu.id = ur.user_id 
    AND ur.restricted_user_id = $1
WHERE uc.owner_user_id = $1
ORDER BY uc.created_at DESC;


-- ===========================================
-- People Who Added You Query
-- ===========================================

-- name: GetUsersWhoAddedYou :many
-- Retrieves users who have added YOU as a contact with raw restriction data for Go processing
SELECT
    cu.id,
    cu.name,
    cu.b64_cipher_chacha20poly1305_username AS username,
    cu.bio,
    uc.created_at AS contact_created_at,
    uc.updated_at AS contact_updated_at,
    
    -- Raw avatar data (Go applies visibility logic)
    a.file_id AS avatar_file_id,
    a.token_id AS avatar_token_id,
    a.token_secret AS avatar_token_secret,
    a.token_expiry AS avatar_token_expiry,
    
    -- Global restriction flags (Priority 1 & 2)
    COALESCE(ugr.restrict_profile, FALSE) AS global_restrict_profile,
    COALESCE(ugr.restrict_avatar, FALSE) AS global_restrict_avatar,
    
    -- Global exemption flags (Priority 1 & 2 override)
    COALESCE(ugre.exception_profile, FALSE) AS exception_global_profile,
    COALESCE(ugre.exception_avatar, FALSE) AS exception_global_avatar,
    
    -- User-level restriction flags (Priority 3 & 4)
    COALESCE(ur.restrict_profile, FALSE) AS user_restrict_profile,
    COALESCE(ur.restrict_avatar, FALSE) AS user_restrict_avatar

FROM user_contacts uc
INNER JOIN users cu 
    ON uc.owner_user_id = cu.id 
    AND cu.is_admin_blocked IS FALSE
    AND cu.profile_type IN ('public', 'personal')
LEFT JOIN avatars a 
    ON cu.id = a.user_id 
    AND a.avatar_type = 'profile'
LEFT JOIN user_global_restrictions ugr 
    ON cu.id = ugr.user_id
LEFT JOIN user_global_restriction_exemptions ugre 
    ON cu.id = ugre.user_id 
    AND ugre.exempted_user_id = $1
LEFT JOIN user_restrictions ur 
    ON cu.id = ur.user_id 
    AND ur.restricted_user_id = $1
WHERE uc.contact_user_id = $1
ORDER BY uc.created_at DESC;


-- ===========================================
-- Avatar Privacy Circuit Breaker Logic
-- ===========================================
-- Both queries return RAW restriction flags for Go to process.
-- Go applies the following priority order (circuit breaker pattern):
--
-- Priority 1: Global PROFILE restriction
--   → If restrict_profile = TRUE AND exception_profile = FALSE → HIDE avatar
--   → If restrict_profile = TRUE AND exception_profile = TRUE → SHOW avatar
--   → Otherwise, continue to Priority 2
--
-- Priority 2: Global AVATAR restriction  
--   → If restrict_avatar = TRUE AND exception_avatar = FALSE → HIDE avatar
--   → If restrict_avatar = TRUE AND exception_avatar = TRUE → SHOW avatar
--   → Otherwise, continue to Priority 3
--
-- Priority 3: User-level PROFILE restriction
--   → If user_restrict_profile = TRUE → HIDE avatar
--   → Otherwise, continue to Priority 4
--
-- Priority 4: User-level AVATAR restriction
--   → If user_restrict_avatar = TRUE → HIDE avatar
--   → Otherwise, SHOW avatar
--
-- Each level short-circuits evaluation (circuit breaker pattern).
-- Privacy checks work identically for both queries because:
--   - cu.id = the contact being viewed (their restrictions apply)
--   - $1 = the viewer (you, checking if you're restricted/exempted)
--   - Direction of contact relationship doesn't affect privacy logic
-- ===========================================




-- ===========================================
-- Contact creation helpers
-- ===========================================

-- name: IsEitherBlocked :one
-- Returns 0 if no block, 1 if blocker is $1 (requester blocked target), 2 if blocker is $2 (target blocked requester)
SELECT CASE
    WHEN EXISTS(SELECT 1 FROM user_blocks ub1 WHERE ub1.blocker_user_id = $1 AND ub1.blocked_user_id = $2) THEN 1
    WHEN EXISTS(SELECT 1 FROM user_blocks ub2 WHERE ub2.blocker_user_id = $2 AND ub2.blocked_user_id = $1) THEN 2
    ELSE 0
END;

-- name: IsAlreadyContact :one
SELECT EXISTS(
    SELECT 1 FROM user_contacts
    WHERE owner_user_id = $1 AND contact_user_id = $2
);

-- name: InsertUserContact :exec
INSERT INTO user_contacts (owner_user_id, contact_user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: HasPendingRequest :one
SELECT EXISTS(
    SELECT 1 FROM contact_requests
    WHERE requester_user_id = $1 AND receiver_user_id = $2 AND status = 'pending'
);

-- name: GetContactRequestStatus :one
SELECT status::text FROM contact_requests
WHERE requester_user_id = $1 AND receiver_user_id = $2
LIMIT 1;

-- name: DeleteAndInsertContactRequest :exec
WITH deleted AS (
    DELETE FROM contact_requests
    WHERE requester_user_id = $2 AND receiver_user_id = $3
    RETURNING requester_user_id
)
INSERT INTO contact_requests (id, requester_user_id, receiver_user_id, status)
SELECT $1, $2, $3, 'pending'
WHERE EXISTS (SELECT 1 FROM deleted) OR NOT EXISTS (
    SELECT 1 FROM contact_requests 
    WHERE requester_user_id = $2 AND receiver_user_id = $3
);

-- name: InsertContactRequest :exec
INSERT INTO contact_requests (id, requester_user_id, receiver_user_id, status)
VALUES ($1, $2, $3, 'pending')
ON CONFLICT DO NOTHING;

-- name: AcceptContactRequest :one
WITH updated AS (
    UPDATE contact_requests AS cr
    SET status = 'accepted'
    WHERE cr.requester_user_id = $1
      AND cr.receiver_user_id = $2
      AND cr.status = 'pending'
    RETURNING cr.id
), existing AS (
    SELECT cr.status
    FROM contact_requests AS cr
    WHERE cr.requester_user_id = $1
      AND cr.receiver_user_id = $2
    LIMIT 1
)
SELECT
    CASE
        WHEN EXISTS (SELECT 1 FROM updated) THEN 'accepted'
        WHEN (SELECT status FROM existing) IS NULL THEN 'not_found'
        ELSE 'processed'
    END AS outcome;

-- name: RejectContactRequest :one
WITH updated AS (
    UPDATE contact_requests AS cr
    SET status = 'declined'
    WHERE cr.requester_user_id = $1
      AND cr.receiver_user_id = $2
      AND cr.status = 'pending'
    RETURNING cr.id
), existing AS (
    SELECT cr.status
    FROM contact_requests AS cr
    WHERE cr.requester_user_id = $1
      AND cr.receiver_user_id = $2
    LIMIT 1
)
SELECT
    CASE
        WHEN EXISTS (SELECT 1 FROM updated) THEN 'declined'
        WHEN (SELECT status FROM existing) IS NULL THEN 'not_found'
        ELSE 'processed'
    END AS outcome;

-- name: DeleteContact :one
WITH deleted AS (
    DELETE FROM user_contacts AS uc
    WHERE uc.owner_user_id = @owner_user_id
      AND uc.contact_user_id = ANY(@contact_user_ids::uuid[])
    RETURNING uc.contact_user_id
)
SELECT COUNT(*) AS removed
FROM deleted;

-- name: GetPendingContactRequests :many
SELECT
    ru.id,
    ru.name,
    ru.b64_cipher_chacha20poly1305_username AS username,
    ru.bio,
    cr.created_at AS request_created_at,
    cr.updated_at AS request_updated_at,
    cr.status::text AS status,
    a.file_id AS avatar_file_id,
    a.token_id AS avatar_token_id,
    a.token_secret AS avatar_token_secret,
    a.token_expiry AS avatar_token_expiry,
    COALESCE(ugr.restrict_profile, FALSE) AS global_restrict_profile,
    COALESCE(ugr.restrict_avatar, FALSE) AS global_restrict_avatar,
    COALESCE(ugre.exception_profile, FALSE) AS exception_global_profile,
    COALESCE(ugre.exception_avatar, FALSE) AS exception_global_avatar,
    COALESCE(ur.restrict_profile, FALSE) AS user_restrict_profile,
    COALESCE(ur.restrict_avatar, FALSE) AS user_restrict_avatar
FROM contact_requests AS cr
INNER JOIN users AS ru
    ON cr.requester_user_id = ru.id
    AND ru.is_admin_blocked IS FALSE
    AND ru.profile_type IN ('public', 'personal')
LEFT JOIN avatars AS a
    ON ru.id = a.user_id
    AND a.avatar_type = 'profile'
LEFT JOIN user_global_restrictions AS ugr
    ON ru.id = ugr.user_id
LEFT JOIN user_global_restriction_exemptions AS ugre
    ON ru.id = ugre.user_id
    AND ugre.exempted_user_id = $1
LEFT JOIN user_restrictions AS ur
    ON ru.id = ur.user_id
    AND ur.restricted_user_id = $1
WHERE cr.receiver_user_id = $1
  AND cr.status = 'pending'
ORDER BY cr.created_at DESC;

-- name: GetSentContactRequests :many
SELECT
    ru.id,
    ru.name,
    ru.b64_cipher_chacha20poly1305_username AS username,
    ru.bio,
    cr.created_at AS request_created_at,
    cr.updated_at AS request_updated_at,
    cr.status::text AS status,
    a.file_id AS avatar_file_id,
    a.token_id AS avatar_token_id,
    a.token_secret AS avatar_token_secret,
    a.token_expiry AS avatar_token_expiry,
    COALESCE(ugr.restrict_profile, FALSE) AS global_restrict_profile,
    COALESCE(ugr.restrict_avatar, FALSE) AS global_restrict_avatar,
    COALESCE(ugre.exception_profile, FALSE) AS exception_global_profile,
    COALESCE(ugre.exception_avatar, FALSE) AS exception_global_avatar,
    COALESCE(ur.restrict_profile, FALSE) AS user_restrict_profile,
    COALESCE(ur.restrict_avatar, FALSE) AS user_restrict_avatar
FROM contact_requests AS cr
INNER JOIN users AS ru
    ON cr.receiver_user_id = ru.id
    AND ru.is_admin_blocked IS FALSE
    AND ru.profile_type IN ('public', 'personal')
LEFT JOIN avatars AS a
    ON ru.id = a.user_id
    AND a.avatar_type = 'profile'
LEFT JOIN user_global_restrictions AS ugr
    ON ru.id = ugr.user_id
LEFT JOIN user_global_restriction_exemptions AS ugre
    ON ru.id = ugre.user_id
    AND ugre.exempted_user_id = $1
LEFT JOIN user_restrictions AS ur
    ON ru.id = ur.user_id
    AND ur.restricted_user_id = $1
WHERE cr.requester_user_id = $1
  AND cr.status IN ('pending', 'declined')
ORDER BY cr.created_at DESC;

-- name: UndoContactRequest :one
WITH deleted AS (
    DELETE FROM contact_requests AS cr
    WHERE cr.requester_user_id = @requester_user_id
      AND cr.receiver_user_id = @receiver_user_id
      AND cr.status = 'pending'
    RETURNING cr.id
)
SELECT
    CASE
        WHEN EXISTS (SELECT 1 FROM deleted) THEN 'undone'
        ELSE 'not_found'
    END AS outcome;


-- ===========================================
-- Contact existence helpers
-- ===========================================

-- name: GetUserByHashedUsername :one
SELECT *
FROM users
WHERE hmac_sha256_hex_username = $1
  AND is_admin_blocked IS NOT TRUE;
