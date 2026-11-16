package personal_chat

import (
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestMarkChatRead_ReturnsFreshReadAt pins the chat-level readAt contract on a
// real database: the value must be the timestamp ResetChatReadStatus wrote, not
// the chat row's updated_at captured before the transaction started.
func TestMarkChatRead_ReturnsFreshReadAt(t *testing.T) {
	ctx, pool := newLivePool(t, 30*time.Second)

	chatID := uuid.New()
	msgID := uuid.New()

	u1, u2 := insertLiveUsers(t, ctx, pool, "MR", "mr-")

	_, err := pool.Exec(ctx, `
		INSERT INTO chats (id, participant_1_id, participant_2_id, created_at, updated_at)
		VALUES ($1, $2, $3, now(), now())`, chatID, u1, u2)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO messages (
			id, chat_id, sender_id, recipient_id, content, message_type,
			sender_session_id,
			delivered_to_recipient, delivered_to_recipient_primary, synced_to_sender_primary,
			read_by_recipient, read_acked_by_sender,
			deleted_by_sender, deleted_by_recipient,
			expires_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, 'hello', 'text',
			$5,
			TRUE, TRUE, TRUE,
			FALSE, FALSE,
			FALSE, FALSE,
			now() + interval '1 day', now(), now()
		)`, msgID, chatID, u1, u2, uuid.New())
	require.NoError(t, err)

	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE chat_id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM chats WHERE id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, u1, u2)
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_users WHERE id IN ($1, $2)`, u1, u2)
	}()

	// Snapshot the value the old implementation used to return.
	var staleUpdatedAt time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT updated_at FROM chats WHERE id = $1`, chatID).Scan(&staleUpdatedAt))

	store := personal_chat_store.New(pool)
	svc := &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	out, err := svc.MarkChatRead(ctx, &MarkChatReadPayload{
		MessageIDs: []string{msgID.String()},
	}, kit.UserId{UuidUserId: u1, StringUserId: u1.String()}, true)
	require.NoError(t, err)
	require.Equal(t, chatID, out.ChatID)
	require.Equal(t, u2, out.OtherUserID)
	require.False(t, out.ReadAt.IsZero())

	returned := out.ReadAt

	var lastReadAt time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT p1_last_read_at FROM chats WHERE id = $1`, chatID).Scan(&lastReadAt))

	require.True(t, returned.Equal(lastReadAt),
		"readAt must equal the timestamp ResetChatReadStatus wrote (db=%s returned=%s)", lastReadAt, returned)
	require.True(t, returned.After(staleUpdatedAt),
		"readAt must be newer than the pre-transaction updated_at (before=%s returned=%s)", staleUpdatedAt, returned)
}

func TestDeleteMessageForMe_LiveNeonBatchExecution(t *testing.T) {
	ctx, pool := newLivePool(t, 60*time.Second)

	chatID := uuid.New()

	u1, u2 := insertLiveUsers(t, ctx, pool, "Live", "")

	_, err := pool.Exec(ctx, `
		INSERT INTO chats (id, participant_1_id, participant_2_id, p1_last_message_content, p1_last_message_type, p2_last_message_content, p2_last_message_type, created_at, updated_at)
		VALUES ($1, $2, $3, 'preview 1', 'text', 'preview 2', 'text', now(), now());
	`, chatID, u1, u2)
	require.NoError(t, err)

	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM message_sync_actions WHERE user_id = $1`, u1)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE chat_id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM chats WHERE id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, u1, u2)
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_users WHERE id IN ($1, $2)`, u1, u2)
	}()

	const totalMessages = 100
	messageIDs := make([]uuid.UUID, totalMessages)
	for i := 0; i < totalMessages; i++ {
		msgID := uuid.New()
		messageIDs[i] = msgID

		sender := u1
		recipient := u2
		if i%2 == 1 {
			sender = u2
			recipient = u1
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO messages (
				id, chat_id, sender_id, recipient_id, content, message_type,
				sender_session_id,
				delivered_to_recipient_primary, synced_to_sender_primary,
				deleted_by_sender, deleted_by_recipient,
				expires_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, 'test content', 'text',
				$5,
				TRUE, TRUE,
				FALSE, FALSE,
				now() + interval '1 day', now(), now()
			);
		`, msgID, chatID, sender, recipient, uuid.New())
		require.NoError(t, err)
	}

	// Link last_message_id on chat
	_, err = pool.Exec(ctx, `UPDATE chats SET last_message_id = $1 WHERE id = $2`, messageIDs[totalMessages-1], chatID)
	require.NoError(t, err)

	store := personal_chat_store.New(pool)
	svc := &chatService{
		PostgresQuerier: store,
		PostgresQueries: store,
		Pool:            pool,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	start := time.Now()
	_, err = svc.DeleteMessageForMe(ctx, messageIDs, kit.UserId{UuidUserId: u1, StringUserId: u1.String()}, false)
	duration := time.Since(start)
	require.NoError(t, err)

	t.Logf(">>> 100-message batch DeleteMessageForMe execution time over Neon DB: %v <<<", duration)
	require.Less(t, duration, 10*time.Second, "Batch execution must finish in under 10 seconds over remote Neon DB")

	// 1. Verify sender soft deletes (50 rows)
	var senderDeletedCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE id = ANY($1) AND sender_id = $2 AND deleted_by_sender = TRUE`, messageIDs, u1).Scan(&senderDeletedCount)
	require.NoError(t, err)
	require.Equal(t, 50, senderDeletedCount)

	// 2. Verify recipient soft deletes (50 rows)
	var recipientDeletedCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE id = ANY($1) AND recipient_id = $2 AND deleted_by_recipient = TRUE`, messageIDs, u1).Scan(&recipientDeletedCount)
	require.NoError(t, err)
	require.Equal(t, 50, recipientDeletedCount)

	// 3. Verify chat preview cleared for u1, preserved for u2
	var p1Content, p2Content *string
	err = pool.QueryRow(ctx, `SELECT p1_last_message_content, p2_last_message_content FROM chats WHERE id = $1`, chatID).Scan(&p1Content, &p2Content)
	require.NoError(t, err)
	require.Nil(t, p1Content)
	require.NotNil(t, p2Content)
	require.Equal(t, "preview 2", *p2Content)

	// 4. Verify 100 sync actions created
	var syncActionCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM message_sync_actions WHERE user_id = $1 AND action_type = 'delete_for_me'`, u1).Scan(&syncActionCount)
	require.NoError(t, err)
	require.Equal(t, 100, syncActionCount)

	// 5. Verify sample JSONB payload format
	var rawPayload []byte
	err = pool.QueryRow(ctx, `SELECT payload FROM message_sync_actions WHERE user_id = $1 AND action_type = 'delete_for_me' LIMIT 1`, u1).Scan(&rawPayload)
	require.NoError(t, err)
	t.Logf("Raw payload: %s", string(rawPayload))

	var sap SyncActionPayload
	err = json.Unmarshal(rawPayload, &sap)
	require.NoError(t, err)
	require.Len(t, sap.MessageIDs, 1)

	// 6. Test Idempotency: Second DeleteMessageForMe by u1 should be a no-op (0 new sync actions, 0 errors)
	_, err = svc.DeleteMessageForMe(ctx, messageIDs, kit.UserId{UuidUserId: u1, StringUserId: u1.String()}, false)
	require.NoError(t, err)

	var u1SyncAfterRepeat int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM message_sync_actions WHERE user_id = $1 AND action_type = 'delete_for_me'`, u1).Scan(&u1SyncAfterRepeat)
	require.NoError(t, err)
	require.Equal(t, 100, u1SyncAfterRepeat, "Repeated delete by u1 must not create duplicate sync actions")

	// 7. Delete by u2 (second participant)
	_, err = svc.DeleteMessageForMe(ctx, messageIDs, kit.UserId{UuidUserId: u2, StringUserId: u2.String()}, false)
	require.NoError(t, err)

	var u2SyncCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM message_sync_actions WHERE user_id = $1 AND action_type = 'delete_for_me'`, u2).Scan(&u2SyncCount)
	require.NoError(t, err)
	require.Equal(t, 100, u2SyncCount, "u2 delete must create 100 sync actions")

	// Both participants have now deleted: verify all messages have both flags = true
	var bothDeletedCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE id = ANY($1) AND deleted_by_sender = TRUE AND deleted_by_recipient = TRUE`, messageIDs).Scan(&bothDeletedCount)
	require.NoError(t, err)
	require.Equal(t, 100, bothDeletedCount, "All 100 messages must have deleted_by_sender=TRUE and deleted_by_recipient=TRUE")

	// 8. Test Idempotency for u2: Repeat call by u2 is a no-op
	_, err = svc.DeleteMessageForMe(ctx, messageIDs, kit.UserId{UuidUserId: u2, StringUserId: u2.String()}, false)
	require.NoError(t, err)

	var u2SyncAfterRepeat int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM message_sync_actions WHERE user_id = $1 AND action_type = 'delete_for_me'`, u2).Scan(&u2SyncAfterRepeat)
	require.NoError(t, err)
	require.Equal(t, 100, u2SyncAfterRepeat, "Repeated delete by u2 must not create duplicate sync actions")

	// Relay cleanup runs asynchronously after each call. Wait until every payload
	// has been stripped before the deferred pool.Close tears the connection down.
	require.Eventually(t, func() bool {
		var stripped int
		if queryErr := pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE id = ANY($1) AND content = ''`, messageIDs).Scan(&stripped); queryErr != nil {
			return false
		}
		return stripped == totalMessages
	}, 40*time.Second, 50*time.Millisecond, "async relay cleanup must strip all payloads")
}

// Live test for Option B cleanup semantics:
//   - R2 deletion is treated as successful (R2Pool is nil in this fixture).
//   - TTL-expired media rows are deleted entirely, even if the recipient primary
//     never acknowledged them.
//   - Unexpired media rows selected only because both primaries acknowledged are
//     not deleted; they keep their row and only have the payload stripped.
func TestCleanupExpiredMessages_LiveNeonExpiredFileRowsDeleted(t *testing.T) {
	ctx, pool := newLivePool(t, 60*time.Second)

	chatID := uuid.New()

	u1, u2 := insertLiveUsers(t, ctx, pool, "Cleanup", "cleanup-")

	_, err := pool.Exec(ctx, `
		INSERT INTO chats (id, participant_1_id, participant_2_id, p1_last_message_content, p1_last_message_type, p2_last_message_content, p2_last_message_type, created_at, updated_at)
		VALUES ($1, $2, $3, 'preview 1', 'text', 'preview 2', 'text', now(), now());
	`, chatID, u1, u2)
	require.NoError(t, err)

	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE chat_id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM chats WHERE id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, u1, u2)
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_users WHERE id IN ($1, $2)`, u1, u2)
	}()

	msg1ID := uuid.New() // expired, undelivered → deleted
	msg2ID := uuid.New() // expired, both primaries → deleted
	msg3ID := uuid.New() // unexpired, both primaries → row kept, payload stripped

	file1 := "cleanup-test-file-1-" + msg1ID.String()
	file2 := "cleanup-test-file-2-" + msg2ID.String()
	file3 := "cleanup-test-file-3-" + msg3ID.String()

	_, err = pool.Exec(ctx, `
		INSERT INTO messages (
			id, chat_id, sender_id, recipient_id, content, message_type,
			file_id,
			sender_session_id,
			delivered_to_recipient_primary, synced_to_sender_primary,
			deleted_by_sender, deleted_by_recipient,
			expires_at, created_at, updated_at
		) VALUES
			($1, $4, $5, $6, 'expired undelivered', 'file', $7, $10, FALSE, FALSE, FALSE, FALSE, now() - interval '1 day', now() - interval '2 days', now()),
			($2, $4, $5, $6, 'expired both primaries', 'file', $8, $10, TRUE, TRUE, FALSE, FALSE, now() - interval '1 day', now() - interval '2 days', now()),
			($3, $4, $5, $6, 'unexpired both primaries', 'file', $9, $10, TRUE, TRUE, FALSE, FALSE, now() + interval '1 day', now() - interval '2 days', now());
	`, msg1ID, msg2ID, msg3ID, chatID, u1, u2, file1, file2, file3, uuid.New())
	require.NoError(t, err)

	store := personal_chat_store.New(pool)
	svc := &chatService{
		Pool:               pool,
		PostgresQuerier:    store,
		PostgresQueries:    store,
		MaintenancePool:    pool,
		MaintenanceQuerier: store,
		MaintenanceQueries: store,
		PendingUploads:     &mockPendingUploadsChat{},
		R2Pool:             nil, // Treat every R2 delete as successful.
	}

	require.NoError(t, svc.CleanupExpiredMessages(ctx))

	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE id = $1`, msg1ID).Scan(&count))
	require.Zero(t, count, "expired undelivered media row must be deleted")

	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE id = $1`, msg2ID).Scan(&count))
	require.Zero(t, count, "expired media row with both primaries must also be deleted")

	var fileID *string
	var content string
	require.NoError(t, pool.QueryRow(ctx, `SELECT file_id, content FROM messages WHERE id = $1`, msg3ID).Scan(&fileID, &content))
	require.Nil(t, fileID, "unexpired both-primary row must keep its row but have file_id cleared")
	require.Empty(t, content)
}

func bytesCompareUUID(a, b uuid.UUID) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
