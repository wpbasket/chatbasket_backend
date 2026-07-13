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

-- name: GetUserChatsLite :many
-- Lite version: reads ONLY from chats table.
-- Profile hydration (name, username, avatar, privacy) is delegated
-- to the personalProfilePersonalChatProvider at the service layer.
SELECT
    c.id,
    c.participant_1_id,
    c.participant_2_id,
    c.p1_unread_count,
    c.p2_unread_count,
    c.p1_last_read_at,
    c.p2_last_read_at,
    c.p1_last_delivered_at,
    c.p2_last_delivered_at,
    c.p1_last_message_content,
    c.p2_last_message_content,
    c.p1_last_message_type,
    c.p2_last_message_type,
    c.last_message_created_at,
    c.created_at,
    c.updated_at,
    c.last_message_sender_id,
    c.last_message_id,
    (CASE
        WHEN c.participant_1_id = $1 THEN c.participant_2_id
        ELSE c.participant_1_id
    END)::UUID AS other_user_id,

-- Unread Count (From Metadata)
(CASE
    WHEN c.participant_1_id = $1 THEN c.p1_unread_count
    ELSE c.p2_unread_count
END)::INT AS unread_count,

-- Per-Participant Last Message Preview
(CASE
    WHEN c.participant_1_id = $1 THEN COALESCE(c.p1_last_message_content, '')
    ELSE COALESCE(c.p2_last_message_content, '')
END)::TEXT AS last_message_content,
(CASE
    WHEN c.participant_1_id = $1 THEN COALESCE(c.p1_last_message_type, '')
    ELSE COALESCE(c.p2_last_message_type, '')
END)::TEXT AS last_message_type,

-- Last Message Status (Calculated from chat metadata only)
(CASE
    WHEN c.last_message_created_at IS NULL THEN ''
    WHEN c.last_message_created_at <= (
        CASE
            WHEN c.participant_1_id = $1 THEN COALESCE(c.p2_last_read_at, '0001-01-01T00:00:00Z'::TIMESTAMPTZ)
            ELSE COALESCE(c.p1_last_read_at, '0001-01-01T00:00:00Z'::TIMESTAMPTZ)
        END
    ) THEN 'read'
    WHEN c.last_message_created_at <= (
        CASE
            WHEN c.participant_1_id = $1 THEN COALESCE(c.p2_last_delivered_at, '0001-01-01T00:00:00Z'::TIMESTAMPTZ)
            ELSE COALESCE(c.p1_last_delivered_at, '0001-01-01T00:00:00Z'::TIMESTAMPTZ)
        END
    ) THEN 'delivered'
    ELSE 'sent'
END)::TEXT AS last_message_status,
(CASE
    WHEN c.participant_1_id = $1 THEN COALESCE(c.p2_last_read_at, '0001-01-01T00:00:00Z'::TIMESTAMPTZ)
    ELSE COALESCE(c.p1_last_read_at, '0001-01-01T00:00:00Z'::TIMESTAMPTZ)
END)::TIMESTAMPTZ AS other_user_last_read_at,
(CASE
    WHEN c.participant_1_id = $1 THEN COALESCE(c.p2_last_delivered_at, '0001-01-01T00:00:00Z'::TIMESTAMPTZ)
    ELSE COALESCE(c.p1_last_delivered_at, '0001-01-01T00:00:00Z'::TIMESTAMPTZ)
END)::TIMESTAMPTZ AS other_user_last_delivered_at
FROM chats c
WHERE
    c.participant_1_id = $1
    OR c.participant_2_id = $1
ORDER BY c.updated_at DESC;

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
        expires_at,
        synced_to_sender_primary,
        delivered_to_recipient_primary
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
        $9
    )
RETURNING
    *;

-- name: GetMessageByID :one
SELECT * FROM messages WHERE id = $1 LIMIT 1;

-- name: GetPendingMessagesForRecipient :many
SELECT *
FROM messages
WHERE
    recipient_id = $1
    AND deleted_by_recipient = FALSE
    AND expires_at > now()
    AND created_at >= sqlc.arg('session_created_at')
ORDER BY created_at ASC
LIMIT $2;

-- name: GetPendingSenderSyncMessages :many
SELECT *
FROM messages
WHERE
    sender_id = $1
    AND deleted_by_sender = FALSE
    AND synced_to_sender_primary = FALSE
    AND expires_at > now()
    AND created_at >= sqlc.arg('session_created_at')
ORDER BY created_at ASC
LIMIT $2;


-- name: MarkMessageDeliveredToRecipient :exec
UPDATE messages
SET
    delivered_to_recipient = TRUE,
    updated_at = now()
WHERE
    id = $1
    AND delivered_to_recipient = FALSE;

-- name: MarkMessageDeliveredToRecipientPrimary :exec
UPDATE messages
SET
    delivered_to_recipient_primary = TRUE,
    delivered_to_recipient = TRUE,
    updated_at = now()
WHERE
    id = $1
    AND delivered_to_recipient_primary = FALSE;

-- name: MarkOlderMessagesAsDeliveredToRecipientPrimary :exec
-- Marks all messages in a chat as delivered to primary if they are older than a specific message
-- and are of type 'text' (plain messages). This prevents relay bloat.
UPDATE messages
SET
    delivered_to_recipient_primary = TRUE,
    delivered_to_recipient = TRUE,
    updated_at = now()
WHERE
    chat_id = $1
    AND recipient_id = $2
    AND created_at <= $3
    AND message_type = 'text'
    AND delivered_to_recipient_primary = FALSE;

-- name: MarkMessageSyncedToSenderPrimary :exec
UPDATE messages
SET
    synced_to_sender_primary = TRUE,
    updated_at = now()
WHERE
    id = $1
    AND synced_to_sender_primary = FALSE;

-- name: DeleteMessage :exec
DELETE FROM messages WHERE id = $1;

-- name: UpdateMessageToUnsent :exec
UPDATE messages
SET
    content = 'Message unsent',
    message_type = 'unsent',
    updated_at = now()
WHERE
    id = $1;

-- name: ClearMessageFileFields :exec
UPDATE messages
SET
    file_id = NULL,
    file_name = NULL,
    file_size = NULL,
    file_mime_type = NULL,
    file_token_id = NULL,
    file_token_secret = NULL,
    file_token_expiry = NULL,
    updated_at = now()
WHERE
    id = $1;


-- name: MarkMessageDeletedBySender :exec
UPDATE messages
SET
    deleted_by_sender = TRUE,
    synced_to_sender_primary = TRUE,
    updated_at = now()
WHERE
    id = $1
    AND sender_id = $2;

-- name: MarkMessageDeletedByRecipient :exec
UPDATE messages
SET
    deleted_by_recipient = TRUE,
    delivered_to_recipient_primary = TRUE,
    updated_at = now()
WHERE
    id = $1
    AND recipient_id = $2;

-- name: MarkChatMessagesAsRead :exec
UPDATE messages
SET
    delivered_to_recipient = TRUE,
    updated_at = now()
WHERE
    chat_id = $1
    AND recipient_id = $2
    AND delivered_to_recipient = FALSE;

-- name: MarkChatMessagesAsReadPrimary :exec
UPDATE messages
SET
    delivered_to_recipient_primary = TRUE,
    delivered_to_recipient = TRUE, -- Implicitly true
    updated_at = now()
WHERE
    chat_id = $1
    AND recipient_id = $2
    AND delivered_to_recipient_primary = FALSE;


-- name: CleanupOlderFullyAcknowledgedMessages :exec
-- Deletes all messages in a chat that are fully acknowledged (both primary flags TRUE)
-- and are older than or equal to a specific timestamp, but ONLY if they are plain text.
DELETE FROM messages
WHERE
    chat_id = $1
    AND created_at <= $2
    AND message_type = 'text'
    AND delivered_to_recipient_primary = TRUE
    AND synced_to_sender_primary = TRUE;


-- name: GetChatMessages :many
SELECT *
FROM messages
WHERE
    chat_id = $1
    AND expires_at > now()
    AND created_at >= sqlc.arg('session_created_at')
    AND (
        (
            sender_id = $4
            AND deleted_by_sender = FALSE
        )
        OR (
            recipient_id = $4
            AND deleted_by_recipient = FALSE
        )
    )
ORDER BY created_at DESC
LIMIT $2
OFFSET
    $3;



-- ===========================================
-- Messaging Eligibility Checks
-- ===========================================


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
        expires_at,
        synced_to_sender_primary,
        delivered_to_recipient_primary
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
        $14,
        $15,
        $16
    )
RETURNING
    *;



-- ===========================================
-- Chat Status Update Operations (Phase 2b)
-- ===========================================

-- name: UpdateChatStatus :exec
UPDATE chats
SET
    p1_last_message_content = $2,
    p2_last_message_content = $2,
    last_message_created_at = $3,
    p1_last_message_type = $4,
    p2_last_message_type = $4,
    last_message_sender_id = $5,
    last_message_id = $6,
    p1_unread_count = CASE
        WHEN participant_1_id != $5 THEN p1_unread_count + 1
        ELSE p1_unread_count
    END,
    p2_unread_count = CASE
        WHEN participant_2_id != $5 THEN p2_unread_count + 1
        ELSE p2_unread_count
    END,
    updated_at = now()
WHERE
    id = $1;

-- name: UpdateChatUnsendPreview :exec
UPDATE chats
SET
    p1_last_message_content = 'Message unsent',
    p2_last_message_content = 'Message unsent',
    p1_last_message_type = 'unsent',
    p2_last_message_type = 'unsent',
    updated_at = now()
WHERE
    id = sqlc.arg ('id')
    AND last_message_id = sqlc.arg ('last_message_id');

-- name: UpdateChatUnsendDecrement :exec
UPDATE chats
SET
    p1_unread_count = CASE
        WHEN participant_1_id = sqlc.arg ('recipient_id') THEN GREATEST(
            0,
            p1_unread_count - sqlc.arg ('amount')::int
        )
        ELSE p1_unread_count
    END,
    p2_unread_count = CASE
        WHEN participant_2_id = sqlc.arg ('recipient_id') THEN GREATEST(
            0,
            p2_unread_count - sqlc.arg ('amount')::int
        )
        ELSE p2_unread_count
    END,
    updated_at = now()
WHERE
    id = sqlc.arg ('id');

-- name: ResetChatReadStatus :exec
UPDATE chats
SET
    p1_unread_count = CASE
        WHEN participant_1_id = $2 THEN 0
        ELSE p1_unread_count
    END,
    p1_last_read_at = CASE
        WHEN participant_1_id = $2 THEN now()
        ELSE p1_last_read_at
    END,
    p2_unread_count = CASE
        WHEN participant_2_id = $2 THEN 0
        ELSE p2_unread_count
    END,
    p2_last_read_at = CASE
        WHEN participant_2_id = $2 THEN now()
        ELSE p2_last_read_at
    END,
    updated_at = now()
WHERE
    id = $1
    AND (
        participant_1_id = $2
        OR participant_2_id = $2
    );

-- name: GetExpiredMessagesWithFiles :many
SELECT *
FROM messages
WHERE (
        expires_at < now()
        OR (
            delivered_to_recipient_primary = TRUE
            AND synced_to_sender_primary = TRUE
        )
    )
    AND file_id IS NOT NULL
    AND id > sqlc.arg('last_id')
ORDER BY id ASC
LIMIT $1;

-- ===========================================
-- Sync Action Operations
-- ===========================================

-- name: CreateSyncAction :one
INSERT INTO
    message_sync_actions (
        id,
        user_id,
        action_type,
        payload,
        created_at
    )
VALUES ($1, $2, $3, $4, now())
RETURNING
    *;

-- name: GetPendingSyncActions :many
SELECT *
FROM message_sync_actions
WHERE
    user_id = $1
    AND delivered_to_primary = FALSE
ORDER BY created_at ASC
LIMIT $2;

-- name: ConsumeSyncAction :exec
DELETE FROM message_sync_actions WHERE id = $1;


-- ===========================================
-- Per-Participant Preview Operations
-- ===========================================

-- name: ClearLastMessageForParticipant :exec
-- Clears the last message preview for a specific participant only (used by Delete for Me).
-- Only fires if the deleted message is the current preview message.
UPDATE chats
SET
    p1_last_message_content = CASE
        WHEN participant_1_id = sqlc.arg ('user_id') THEN NULL
        ELSE p1_last_message_content
    END,
    p2_last_message_content = CASE
        WHEN participant_2_id = sqlc.arg ('user_id') THEN NULL
        ELSE p2_last_message_content
    END,
    p1_last_message_type = CASE
        WHEN participant_1_id = sqlc.arg ('user_id') THEN NULL
        ELSE p1_last_message_type
    END,
    p2_last_message_type = CASE
        WHEN participant_2_id = sqlc.arg ('user_id') THEN NULL
        ELSE p2_last_message_type
    END,
    updated_at = now()
WHERE
    id = sqlc.arg ('chat_id')
    AND last_message_id = sqlc.arg ('message_id');

-- name: UpdateChatLastDeliveredAt :exec
UPDATE chats
SET
    p1_last_delivered_at = CASE
        WHEN participant_1_id = sqlc.arg ('participant_id') THEN GREATEST(
            COALESCE(
                p1_last_delivered_at,
                '0001-01-01'::TIMESTAMPTZ
            ),
            sqlc.arg ('last_delivered_at')::TIMESTAMPTZ
        )
        ELSE p1_last_delivered_at
    END,
    p2_last_delivered_at = CASE
        WHEN participant_2_id = sqlc.arg ('participant_id') THEN GREATEST(
            COALESCE(
                p2_last_delivered_at,
                '0001-01-01'::TIMESTAMPTZ
            ),
            sqlc.arg ('last_delivered_at')::TIMESTAMPTZ
        )
        ELSE p2_last_delivered_at
    END,
    updated_at = now()
WHERE
    id = sqlc.arg ('chat_id');

-- ===========================================
-- Block Cleanup Operations (Background Worker)
-- ===========================================



-- name: GetMessagesWithFilesForBlockedUsers :many
-- Fetches messages with files for chats between blocked users for cleanup.
SELECT m.*
FROM messages m
INNER JOIN chats c ON m.chat_id = c.id
INNER JOIN user_blocks ub ON (
    (c.participant_1_id = ub.blocker_user_id AND c.participant_2_id = ub.blocked_user_id)
    OR
    (c.participant_1_id = ub.blocked_user_id AND c.participant_2_id = ub.blocker_user_id)
)
WHERE m.file_id IS NOT NULL
AND m.id > sqlc.arg('last_id')
ORDER BY m.id ASC
LIMIT sqlc.arg('limit');


-- name: UpsertHistorySync :one
INSERT INTO history_sync (
    id, user_id, session_id, chats_json, expires_at, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, now(), now()
)
ON CONFLICT (session_id) DO UPDATE SET
    id = EXCLUDED.id,
    chats_json = EXCLUDED.chats_json,
    payload = NULL,
    expires_at = EXCLUDED.expires_at,
    updated_at = now()
RETURNING id;

-- name: UploadHistorySyncPayload :execrows
UPDATE history_sync 
SET payload = $1, updated_at = now() 
WHERE id = $2 AND user_id = $3 AND expires_at > now();

-- name: GetHistorySyncForDownload :one
SELECT payload 
FROM history_sync 
WHERE id = $1 AND session_id = $2;



-- name: DeleteExpiredMessagesWithoutFilesBatch :execrows
-- Bounded cleanup batch: deletes expired messages without attached files.
-- Controlled from Go with a batch_size parameter and a time budget.
WITH batch AS (
  SELECT id FROM messages
  WHERE expires_at < now() AND file_id IS NULL
  FOR UPDATE SKIP LOCKED
  LIMIT sqlc.arg('batch_size')
)
DELETE FROM messages AS m
USING batch
WHERE m.id = batch.id;

-- name: DeleteFullyAcknowledgedMessagesWithoutFilesBatch :execrows
-- Bounded cleanup batch: deletes text-only messages fully acknowledged by both primaries.
-- Controlled from Go with a batch_size parameter and a time budget.
WITH batch AS (
  SELECT id FROM messages
  WHERE file_id IS NULL AND delivered_to_recipient_primary = TRUE AND synced_to_sender_primary = TRUE
  FOR UPDATE SKIP LOCKED
  LIMIT sqlc.arg('batch_size')
)
DELETE FROM messages AS m
USING batch
WHERE m.id = batch.id;

-- name: DeleteBlockedUserMessagesWithoutFilesBatch :execrows
-- Bounded cleanup batch: deletes no-file messages in chats between blocked users.
-- Controlled from Go with a batch_size parameter and a time budget.
WITH batch AS (
  SELECT m.id FROM messages m
  JOIN chats c ON m.chat_id = c.id
  JOIN user_blocks ub ON (
    (c.participant_1_id = ub.blocker_user_id AND c.participant_2_id = ub.blocked_user_id)
    OR (c.participant_1_id = ub.blocked_user_id AND c.participant_2_id = ub.blocker_user_id)
  )
  WHERE m.file_id IS NULL
  FOR UPDATE SKIP LOCKED
  LIMIT sqlc.arg('batch_size')
)
DELETE FROM messages AS m
USING batch
WHERE m.id = batch.id;

-- name: CleanupSyncActionsForBlockedUsersBatch :execrows
-- Bounded cleanup batch: deletes sync actions for chats between blocked users.
-- Controlled from Go with a batch_size parameter and a time budget.
WITH batch AS (
  SELECT msa.id FROM message_sync_actions msa
  JOIN chats c ON (msa.payload::jsonb->>'chat_id')::uuid = c.id
  JOIN user_blocks ub ON (
    (c.participant_1_id = ub.blocker_user_id AND c.participant_2_id = ub.blocked_user_id)
    OR (c.participant_1_id = ub.blocked_user_id AND c.participant_2_id = ub.blocker_user_id)
  )
  FOR UPDATE SKIP LOCKED
  LIMIT sqlc.arg('batch_size')
)
DELETE FROM message_sync_actions AS msa
USING batch
WHERE msa.id = batch.id;

-- name: DeleteOldSyncActionsBatch :execrows
-- Bounded cleanup batch: deletes sync actions older than 30 days.
-- Controlled from Go with a batch_size parameter and a time budget.
WITH batch AS (
  SELECT id FROM message_sync_actions
  WHERE created_at < now() - INTERVAL '30 days'
  FOR UPDATE SKIP LOCKED
  LIMIT sqlc.arg('batch_size')
)
DELETE FROM message_sync_actions AS msa
USING batch
WHERE msa.id = batch.id;

-- name: DeleteExpiredHistorySyncBatch :execrows
-- Bounded cleanup batch: deletes expired history-sync records.
-- Controlled from Go with a batch_size parameter and a time budget.
WITH batch AS (
  SELECT id FROM history_sync
  WHERE expires_at < now()
  FOR UPDATE SKIP LOCKED
  LIMIT sqlc.arg('batch_size')
)
DELETE FROM history_sync AS hs
USING batch
WHERE hs.id = batch.id;

-- name: GetHistorySyncMeta :one
SELECT user_id, session_id, expires_at 
FROM history_sync 
WHERE id = $1;

-- name: GetMessagesWithFilesByChatID :many
SELECT * FROM messages 
WHERE chat_id = $1 AND file_id IS NOT NULL;

-- name: DeleteMessagesByChatID :exec
DELETE FROM messages WHERE chat_id = $1;
