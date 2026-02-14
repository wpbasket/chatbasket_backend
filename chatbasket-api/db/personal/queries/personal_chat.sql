-- ===========================================
-- Chat Queries for sqlc
-- ===========================================

-- name: CreateChat :one
INSERT INTO
    chats (
        id,
        participant_1_id,
        participant_2_id
    )
VALUES (
        $1,
        LEAST($2::uuid, $3::uuid),
        GREATEST($2::uuid, $3::uuid)
    )
ON CONFLICT (
    participant_1_id,
    participant_2_id
) DO
UPDATE
SET
    updated_at = now()
RETURNING
    *;

-- name: GetChatByParticipants :one
SELECT *
FROM chats
WHERE
    participant_1_id = LEAST($1::uuid, $2::uuid)
    AND participant_2_id = GREATEST($1::uuid, $2::uuid)
LIMIT 1;

-- name: GetUserChats :many
-- name: GetUserChats :many
SELECT DISTINCT
    ON (c.id) c.*,
    u.name AS other_user_name,
    u.b64_cipher_chacha20poly1305_username AS other_user_username,
    u.id AS other_user_id,

-- Last Message
m.content AS last_message_content,
m.created_at AS last_message_created_at,
m.message_type AS last_message_type,
m.sender_id AS last_message_sender_id,

-- Unread Count
(
    SELECT COUNT(*)
    FROM messages m2
    WHERE
        m2.chat_id = c.id
        AND m2.recipient_id = $1
        AND m2.delivered_to_recipient = FALSE
        AND m2.expires_at > now()
)::INT AS unread_count,

-- Raw avatar data
a.file_id AS avatar_file_id,
a.token_id AS avatar_token_id,
a.token_secret AS avatar_token_secret,
a.token_expiry AS avatar_token_expiry,

-- Privacy flags
COALESCE(ugr.restrict_profile, FALSE) AS global_restrict_profile,
COALESCE(ugr.restrict_avatar, FALSE) AS global_restrict_avatar,
COALESCE(ugre.exception_profile, FALSE) AS exception_global_profile,
COALESCE(ugre.exception_avatar, FALSE) AS exception_global_avatar,
COALESCE(ur.restrict_profile, FALSE) AS user_restrict_profile,
COALESCE(ur.restrict_avatar, FALSE) AS user_restrict_avatar
FROM
    chats c
    JOIN users u ON u.id = CASE
        WHEN c.participant_1_id = $1 THEN c.participant_2_id
        ELSE c.participant_1_id
    END
    LEFT JOIN messages m ON m.chat_id = c.id
    LEFT JOIN avatars a ON u.id = a.user_id
    AND a.avatar_type = 'profile'
    LEFT JOIN user_global_restrictions ugr ON u.id = ugr.user_id
    LEFT JOIN user_global_restriction_exemptions ugre ON u.id = ugre.user_id
    AND ugre.exempted_user_id = $1
    LEFT JOIN user_restrictions ur ON u.id = ur.user_id
    AND ur.restricted_user_id = $1
WHERE
    c.participant_1_id = $1
    OR c.participant_2_id = $1
ORDER BY c.id, m.created_at DESC;

-- name: GetChatByID :one
SELECT * FROM chats WHERE id = $1 LIMIT 1;

-- ===========================================
-- Message Operations
-- ===========================================

-- name: CreateMessage :one
INSERT INTO
    messages (
        id,
        chat_id,
        sender_id,
        recipient_id,
        content,
        message_type,
        expires_at
    )
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING
    *;

-- name: GetMessageByID :one
SELECT * FROM messages WHERE id = $1 LIMIT 1;

-- name: GetPendingMessagesForRecipient :many
SELECT *
FROM messages
WHERE
    recipient_id = $1
    AND delivered_to_recipient = FALSE
    AND expires_at > now()
ORDER BY created_at ASC
LIMIT $2;

-- name: GetPendingSenderSyncMessages :many
SELECT *
FROM messages
WHERE
    sender_id = $1
    AND synced_to_sender_primary = FALSE
    AND expires_at > now()
ORDER BY created_at ASC
LIMIT $2;

-- name: MarkMessageDeliveredToRecipient :exec
UPDATE messages
SET
    delivered_to_recipient = TRUE,
    updated_at = now()
WHERE
    id = $1;

-- name: MarkMessageSyncedToSenderPrimary :exec
UPDATE messages
SET
    synced_to_sender_primary = TRUE,
    updated_at = now()
WHERE
    id = $1;

-- name: DeleteMessage :exec
DELETE FROM messages WHERE id = $1;

-- name: MarkChatMessagesAsRead :exec
UPDATE messages
SET
    delivered_to_recipient = TRUE,
    updated_at = now()
WHERE
    chat_id = $1
    AND recipient_id = $2
    AND delivered_to_recipient = FALSE;

-- name: DeleteDeliveredMessages :exec
DELETE FROM messages
WHERE
    delivered_to_recipient = TRUE
    AND synced_to_sender_primary = TRUE;

-- name: DeleteExpiredMessages :exec
DELETE FROM messages WHERE expires_at < now();

-- name: IncrementDeliveryAttempts :exec
UPDATE messages
SET
    delivery_attempts = delivery_attempts + 1,
    updated_at = now()
WHERE
    id = $1;

-- name: GetChatMessages :many
SELECT *
FROM messages
WHERE
    chat_id = $1
    AND expires_at > now()
ORDER BY created_at DESC
LIMIT $2
OFFSET
    $3;

-- name: DeletePendingMessagesBetweenUsers :exec
DELETE FROM messages
WHERE (
        sender_id = $1
        AND recipient_id = $2
    )
    OR (
        sender_id = $2
        AND recipient_id = $1
    );

-- ===========================================
-- Messaging Eligibility Checks
-- ===========================================

-- name: CanSendMessage :one
SELECT
    CASE
        WHEN NOT EXISTS (
            SELECT 1
            FROM users
            WHERE
                id = $2::uuid
        ) THEN 'recipient_not_found'
        WHEN NOT EXISTS (
            SELECT 1
            FROM user_contacts
            WHERE
                owner_user_id = $1::uuid
                AND contact_user_id = $2::uuid
        ) THEN 'not_in_contacts'
        WHEN EXISTS (
            SELECT 1
            FROM users
            WHERE
                id = $2::uuid
                AND profile_type = 'private'
        ) THEN 'recipient_private'
        WHEN EXISTS (
            SELECT 1
            FROM user_blocks
            WHERE
                blocker_user_id = $2::uuid
                AND blocked_user_id = $1::uuid
        ) THEN 'blocked_by_recipient'
        WHEN EXISTS (
            SELECT 1
            FROM user_blocks
            WHERE
                blocker_user_id = $1::uuid
                AND blocked_user_id = $2::uuid
        ) THEN 'blocked_by_me'
        WHEN EXISTS (
            SELECT 1
            FROM users
            WHERE
                id IN ($1::uuid, $2::uuid)
                AND is_admin_blocked = TRUE
        ) THEN 'admin_blocked'
        ELSE 'allowed'
    END AS eligibility_status;

-- name: IsChatParticipant :one
SELECT EXISTS (
        SELECT 1
        FROM chats
        WHERE
            id = $1::uuid
            AND (
                $2::uuid = participant_1_id
                OR $2::uuid = participant_2_id
            )
    );

-- ===========================================
-- Message Delivery Log Operations
-- ===========================================

-- name: CreateDeliveryLog :exec
INSERT INTO
    message_delivery_log (
        id,
        message_id,
        attempt_number,
        status,
        error_reason
    )
VALUES ($1, $2, $3, $4, $5);

-- name: GetDeliveryLogsByMessage :many
SELECT *
FROM message_delivery_log
WHERE
    message_id = $1
ORDER BY attempted_at DESC;

-- ===========================================
-- File Messaging Operations
-- ===========================================

-- name: CreateMessageWithFile :one
INSERT INTO
    messages (
        id,
        chat_id,
        sender_id,
        recipient_id,
        content,
        message_type,
        file_id,
        file_name,
        file_size,
        file_mime_type,
        file_token_id,
        file_token_secret,
        file_token_expiry,
        expires_at
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
        $10,
        $11,
        $12,
        $13,
        $14
    )
RETURNING
    *;

-- name: UpdateMessageFileToken :exec
UPDATE messages
SET
    file_token_id = $2,
    file_token_secret = $3,
    file_token_expiry = $4,
    updated_at = now()
WHERE
    id = $1;

-- name: GetMessagesWithExpiredFileTokens :many
SELECT *
FROM messages
WHERE
    file_id IS NOT NULL
    AND file_token_expiry IS NOT NULL
    AND file_token_expiry < now()
    AND expires_at > now()
ORDER BY created_at ASC
LIMIT $1;

-- name: GetExpiredMessagesWithFiles :many
SELECT *
FROM messages
WHERE
    expires_at < now()
    AND file_id IS NOT NULL
ORDER BY expires_at ASC
LIMIT $1;