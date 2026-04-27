-- ===========================================
-- Contacts Queries for sqlc
-- ===========================================

-- ===========================================
-- People Who Added You Query
-- ===========================================

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
SELECT (CASE
    WHEN EXISTS(SELECT 1 FROM user_blocks ub1 WHERE ub1.blocker_user_id = $1 AND ub1.blocked_user_id = $2) THEN 1
    WHEN EXISTS(SELECT 1 FROM user_blocks ub2 WHERE ub2.blocker_user_id = $2 AND ub2.blocked_user_id = $1) THEN 2
    ELSE 0
END)::INT;

-- name: IsAlreadyContact :one
SELECT EXISTS(
    SELECT 1 FROM user_contacts
    WHERE owner_user_id = $1 AND contact_user_id = $2
);

-- name: InsertUserContact :exec
INSERT INTO user_contacts (owner_user_id, contact_user_id, nickname)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: GetSingleUserContactLite :one
SELECT
    uc.contact_user_id AS id,
    uc.nickname,
    uc.created_at AS contact_created_at,
    uc.updated_at AS contact_updated_at
FROM user_contacts uc
WHERE uc.owner_user_id = $1
  AND uc.contact_user_id = $2
LIMIT 1;

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
INSERT INTO contact_requests (id, requester_user_id, receiver_user_id, status, nickname)
SELECT $1, $2, $3, 'pending', $4
WHERE EXISTS (SELECT 1 FROM deleted) OR NOT EXISTS (
    SELECT 1 FROM contact_requests 
    WHERE requester_user_id = $2 AND receiver_user_id = $3
);

-- name: InsertContactRequest :exec
INSERT INTO contact_requests (id, requester_user_id, receiver_user_id, status, nickname)
VALUES ($1, $2, $3, 'pending', $4)
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
    (CASE
        WHEN EXISTS (SELECT 1 FROM updated) THEN 'accepted'
        WHEN (SELECT status FROM existing) IS NULL THEN 'not_found'
        ELSE 'processed'
    END)::TEXT AS outcome;

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
    (CASE
        WHEN EXISTS (SELECT 1 FROM updated) THEN 'declined'
        WHEN (SELECT status FROM existing) IS NULL THEN 'not_found'
        ELSE 'processed'
    END)::TEXT AS outcome;

-- name: DeleteContact :one
WITH deleted AS (
    DELETE FROM user_contacts AS uc
    WHERE uc.owner_user_id = @owner_user_id
      AND uc.contact_user_id = ANY(@contact_user_ids::uuid[])
    RETURNING uc.contact_user_id
)
SELECT COUNT(*) AS removed
FROM deleted;

-- name: UpdateContactNickname :one
UPDATE user_contacts
SET nickname = $3,
    updated_at = now()
WHERE owner_user_id = $1
  AND contact_user_id = $2
RETURNING true AS updated;

-- name: CreateUserBlock :exec
INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id)
VALUES ($1, $2, $3)
ON CONFLICT (blocker_user_id, blocked_user_id) DO NOTHING;

-- name: UndoContactRequest :one
WITH deleted AS (
    DELETE FROM contact_requests AS cr
    WHERE cr.requester_user_id = @requester_user_id
      AND cr.receiver_user_id = @receiver_user_id
      AND cr.status = 'pending'
    RETURNING cr.id
)
SELECT
    (CASE
        WHEN EXISTS (SELECT 1 FROM deleted) THEN 'undone'
        ELSE 'not_found'
    END)::TEXT AS outcome;


-- ===========================================
-- Contact existence helpers
-- ===========================================


-- ===========================================
-- Cleanup queries for orphaned data
-- ===========================================

-- name: CleanupOrphanedContactsFromBlocks :exec
-- Removes contact relationships that should have been deleted by block trigger
DELETE FROM user_contacts uc
WHERE EXISTS (
    SELECT 1 FROM user_blocks ub
    WHERE (
        -- Either direction of block should remove the contact
        (ub.blocker_user_id = uc.owner_user_id AND ub.blocked_user_id = uc.contact_user_id)
        OR
        (ub.blocker_user_id = uc.contact_user_id AND ub.blocked_user_id = uc.owner_user_id)
    )
);

-- name: CleanupOrphanedContactRequestsFromBlocks :exec
-- Removes contact requests that should have been deleted when block was created
DELETE FROM contact_requests cr
WHERE EXISTS (
    SELECT 1 FROM user_blocks ub
    WHERE (
        -- Either direction of block should remove the request
        (ub.blocker_user_id = cr.requester_user_id AND ub.blocked_user_id = cr.receiver_user_id)
        OR
        (ub.blocker_user_id = cr.receiver_user_id AND ub.blocked_user_id = cr.requester_user_id)
    )
);

-- name: DetectOrphanedContactsFromBlocks :many
-- Finds contacts that exist despite blocks (trigger failure detection)
SELECT
    uc.owner_user_id,
    uc.contact_user_id,
    uc.created_at as contact_created_at,
    ub.created_at as block_created_at,
    (CASE
        WHEN ub.blocker_user_id = uc.owner_user_id THEN 'owner_blocked_contact'
        ELSE 'contact_blocked_owner'
    END)::TEXT AS block_direction
FROM user_contacts uc
INNER JOIN user_blocks ub ON (
    (ub.blocker_user_id = uc.owner_user_id AND ub.blocked_user_id = uc.contact_user_id)
    OR
    (ub.blocker_user_id = uc.contact_user_id AND ub.blocked_user_id = uc.owner_user_id)
)
ORDER BY uc.created_at DESC;

-- name: DetectOrphanedContactRequestsFromBlocks :many
-- Finds contact requests that exist despite blocks (trigger failure detection)
SELECT
    cr.requester_user_id,
    cr.receiver_user_id,
    cr.status::text,
    cr.created_at as request_created_at,
    ub.created_at as block_created_at,
    (CASE
        WHEN ub.blocker_user_id = cr.requester_user_id THEN 'requester_blocked_receiver'
        ELSE 'receiver_blocked_requester'
    END)::TEXT AS block_direction
FROM contact_requests cr
INNER JOIN user_blocks ub ON (
    (ub.blocker_user_id = cr.requester_user_id AND ub.blocked_user_id = cr.receiver_user_id)
    OR
    (ub.blocker_user_id = cr.receiver_user_id AND ub.blocked_user_id = cr.requester_user_id)
)
ORDER BY cr.created_at DESC;

-- name: GetUserContactsLite :many
SELECT
    uc.contact_user_id AS id,
    uc.nickname,
    uc.created_at AS contact_created_at,
    uc.updated_at AS contact_updated_at
FROM user_contacts uc
LEFT JOIN user_blocks ub1
    ON ub1.blocker_user_id = $1 AND ub1.blocked_user_id = uc.contact_user_id
LEFT JOIN user_blocks ub2
    ON ub2.blocker_user_id = uc.contact_user_id AND ub2.blocked_user_id = $1
WHERE uc.owner_user_id = $1
  AND ub1.id IS NULL
  AND ub2.id IS NULL
ORDER BY uc.created_at DESC;

-- name: GetUsersWhoAddedYouLite :many
SELECT
    uc.owner_user_id AS id,
    my_uc.nickname,
    uc.created_at AS contact_created_at,
    uc.updated_at AS contact_updated_at
FROM user_contacts uc
LEFT JOIN user_contacts my_uc
    ON my_uc.owner_user_id = $1
   AND my_uc.contact_user_id = uc.owner_user_id
LEFT JOIN user_blocks ub1
    ON ub1.blocker_user_id = $1 AND ub1.blocked_user_id = uc.owner_user_id
LEFT JOIN user_blocks ub2
    ON ub2.blocker_user_id = uc.owner_user_id AND ub2.blocked_user_id = $1
WHERE uc.contact_user_id = $1
  AND ub1.id IS NULL
  AND ub2.id IS NULL
ORDER BY uc.created_at DESC;

-- name: GetPendingContactRequestsLite :many
SELECT
    cr.requester_user_id AS id,
    cr.nickname,
    cr.created_at AS request_created_at,
    cr.updated_at AS request_updated_at,
    cr.status::text AS status
FROM contact_requests cr
LEFT JOIN user_blocks ub1
    ON ub1.blocker_user_id = $1 AND ub1.blocked_user_id = cr.requester_user_id
LEFT JOIN user_blocks ub2
    ON ub2.blocker_user_id = cr.requester_user_id AND ub2.blocked_user_id = $1
WHERE cr.receiver_user_id = $1
  AND cr.status = 'pending'
  AND ub1.id IS NULL
  AND ub2.id IS NULL
ORDER BY cr.created_at DESC;

-- name: GetSentContactRequestsLite :many
SELECT
    cr.receiver_user_id AS id,
    cr.nickname,
    cr.created_at AS request_created_at,
    cr.updated_at AS request_updated_at,
    cr.status::text AS status
FROM contact_requests cr
LEFT JOIN user_blocks ub1
    ON ub1.blocker_user_id = $1 AND ub1.blocked_user_id = cr.receiver_user_id
LEFT JOIN user_blocks ub2
    ON ub2.blocker_user_id = cr.receiver_user_id AND ub2.blocked_user_id = $1
WHERE cr.requester_user_id = $1
  AND cr.status IN ('pending', 'declined')
  AND ub1.id IS NULL
  AND ub2.id IS NULL
ORDER BY cr.created_at DESC;
