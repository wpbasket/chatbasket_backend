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



-- name: GetUserContactsLite :many
SELECT
    uc.contact_user_id AS id,
    uc.nickname,
    uc.created_at AS contact_created_at,
    uc.updated_at AS contact_updated_at
FROM user_contacts uc
WHERE uc.owner_user_id = $1
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
WHERE uc.contact_user_id = $1
ORDER BY uc.created_at DESC;

-- name: GetPendingContactRequestsLite :many
SELECT
    cr.requester_user_id AS id,
    cr.nickname,
    cr.created_at AS request_created_at,
    cr.updated_at AS request_updated_at,
    cr.status::text AS status
FROM contact_requests cr
WHERE cr.receiver_user_id = $1
  AND cr.status = 'pending'
ORDER BY cr.created_at DESC;

-- name: GetSentContactRequestsLite :many
SELECT
    cr.receiver_user_id AS id,
    cr.nickname,
    cr.created_at AS request_created_at,
    cr.updated_at AS request_updated_at,
    cr.status::text AS status
FROM contact_requests cr
WHERE cr.requester_user_id = $1
  AND cr.status IN ('pending', 'declined')
ORDER BY cr.created_at DESC;