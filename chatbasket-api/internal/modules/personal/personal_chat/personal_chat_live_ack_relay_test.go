package personal_chat

import (
	"chatbasket-api/internal/modules/core/pending_uploads"
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/config"
	"chatbasket-api/internal/platform/kit"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

func TestAcknowledgeAndReadBatch_AlreadyDelivered_SkipsDeliveryUpdate(t *testing.T) {
	ctx, pool := newLivePool(t, 30*time.Second)

	chatID := uuid.New()
	msgID := uuid.New()

	u1, u2 := insertLiveUsers(t, ctx, pool, "MR", "mr-")

	initialDeliveredAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	_, err := pool.Exec(ctx, `
		INSERT INTO chats (id, participant_1_id, participant_2_id, p2_last_delivered_at, p2_unread_count, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 1, now(), now())`, chatID, u1, u2, initialDeliveredAt)
	require.NoError(t, err)

	msgCreatedAt := time.Now().Add(-1 * time.Hour).Truncate(time.Microsecond)
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
			now() + interval '1 day', $6, $6
		)`, msgID, chatID, u1, u2, uuid.New(), msgCreatedAt)
	require.NoError(t, err)

	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE chat_id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM chats WHERE id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, u1, u2)
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_users WHERE id IN ($1, $2)`, u1, u2)
	}()

	store := personal_chat_store.New(pool)
	svc := &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	out, ackErr := svc.AcknowledgeAndReadBatch(ctx, &AckAndReadBatchPayload{
		MessageIDs: []string{msgID.String()},
	}, kit.UserId{UuidUserId: u2, StringUserId: u2.String()}, true)

	require.NoError(t, ackErr)
	require.Equal(t, chatID, out.ChatID)
	require.Equal(t, u1, out.OtherUserID)
	require.False(t, out.ReadAt.IsZero())
	require.Len(t, out.Receipts, 1)
	require.Len(t, out.NewlyRead, 1)

	// Verify message was marked as read
	var readByRecipient bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT read_by_recipient FROM messages WHERE id = $1`, msgID).Scan(&readByRecipient))
	require.True(t, readByRecipient, "message must be marked as read")

	// Verify chat unread count was reset
	var p2UnreadCount int32
	var p2LastDeliveredAt *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT p2_unread_count, p2_last_delivered_at FROM chats WHERE id = $1`, chatID).Scan(&p2UnreadCount, &p2LastDeliveredAt))
	require.Equal(t, int32(0), p2UnreadCount, "unread count must be reset to 0")

	// Upfront delivery filtering rule: since message was already delivered, p2_last_delivered_at
	// must NOT be falsely advanced to time.Now()
	require.NotNil(t, p2LastDeliveredAt)
	require.True(t, initialDeliveredAt.Equal(*p2LastDeliveredAt),
		"p2_last_delivered_at must remain untouched when message was already delivered (expected %v, got %v)",
		initialDeliveredAt, *p2LastDeliveredAt)
}

func TestAcknowledgeAndReadBatch_NotYetDelivered_UpdatesDeliveryAndChat(t *testing.T) {
	ctx, pool := newLivePool(t, 30*time.Second)

	chatID := uuid.New()
	msgID := uuid.New()

	u1, u2 := insertLiveUsers(t, ctx, pool, "MR", "mr-")

	initialDeliveredAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	_, err := pool.Exec(ctx, `
		INSERT INTO chats (id, participant_1_id, participant_2_id, p2_last_delivered_at, p2_unread_count, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 1, now(), now())`, chatID, u1, u2, initialDeliveredAt)
	require.NoError(t, err)

	msgCreatedAt := time.Now().Add(-30 * time.Minute).Truncate(time.Microsecond)
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
			FALSE, FALSE, TRUE,
			FALSE, FALSE,
			FALSE, FALSE,
			now() + interval '1 day', $6, $6
		)`, msgID, chatID, u1, u2, uuid.New(), msgCreatedAt)
	require.NoError(t, err)

	var actualCreatedAt time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT created_at FROM messages WHERE id = $1`, msgID).Scan(&actualCreatedAt))

	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE chat_id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM chats WHERE id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, u1, u2)
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_users WHERE id IN ($1, $2)`, u1, u2)
	}()

	store := personal_chat_store.New(pool)
	svc := &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	out, ackErr := svc.AcknowledgeAndReadBatch(ctx, &AckAndReadBatchPayload{
		MessageIDs: []string{msgID.String()},
	}, kit.UserId{UuidUserId: u2, StringUserId: u2.String()}, true)

	require.NoError(t, ackErr)
	require.Equal(t, chatID, out.ChatID)
	require.Equal(t, u1, out.OtherUserID)
	require.False(t, out.ReadAt.IsZero())
	require.Len(t, out.Receipts, 1)
	require.Len(t, out.NewlyRead, 1)

	// Verify message was marked as delivered and read
	var deliveredToRecipientPrimary, readByRecipient bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT delivered_to_recipient_primary, read_by_recipient FROM messages WHERE id = $1`, msgID).Scan(&deliveredToRecipientPrimary, &readByRecipient))
	require.True(t, deliveredToRecipientPrimary, "message must be marked as delivered to recipient primary")
	require.True(t, readByRecipient, "message must be marked as read")

	// Verify chat unread count was reset
	var p2UnreadCount int32
	var p2LastDeliveredAt *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT p2_unread_count, p2_last_delivered_at FROM chats WHERE id = $1`, chatID).Scan(&p2UnreadCount, &p2LastDeliveredAt))
	require.Equal(t, int32(0), p2UnreadCount, "unread count must be reset to 0")

	// Verify p2_last_delivered_at WAS updated to actualCreatedAt
	require.NotNil(t, p2LastDeliveredAt)
	require.True(t, actualCreatedAt.Equal(*p2LastDeliveredAt),
		"p2_last_delivered_at must be updated to message created_at (expected %v, got %v)",
		actualCreatedAt, *p2LastDeliveredAt)
}

func TestAcknowledgeAndReadBatch_SecondaryDevice_AlreadyDelivered_SkipsDeliveryUpdate(t *testing.T) {
	ctx, pool := newLivePool(t, 30*time.Second)

	chatID := uuid.New()
	msgID := uuid.New()

	u1, u2 := insertLiveUsers(t, ctx, pool, "MR", "mr-")

	initialDeliveredAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	_, err := pool.Exec(ctx, `
		INSERT INTO chats (id, participant_1_id, participant_2_id, p2_last_delivered_at, p2_unread_count, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 1, now(), now())`, chatID, u1, u2, initialDeliveredAt)
	require.NoError(t, err)

	msgCreatedAt := time.Now().Add(-1 * time.Hour).Truncate(time.Microsecond)
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
			TRUE, FALSE, TRUE,
			FALSE, FALSE,
			FALSE, FALSE,
			now() + interval '1 day', $6, $6
		)`, msgID, chatID, u1, u2, uuid.New(), msgCreatedAt)
	require.NoError(t, err)

	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE chat_id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM chats WHERE id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, u1, u2)
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_users WHERE id IN ($1, $2)`, u1, u2)
	}()

	store := personal_chat_store.New(pool)
	svc := &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	// isPrimary = false (secondary device)
	out, ackErr := svc.AcknowledgeAndReadBatch(ctx, &AckAndReadBatchPayload{
		MessageIDs: []string{msgID.String()},
	}, kit.UserId{UuidUserId: u2, StringUserId: u2.String()}, false)

	require.NoError(t, ackErr)
	require.Equal(t, chatID, out.ChatID)
	require.Equal(t, u1, out.OtherUserID)
	require.False(t, out.ReadAt.IsZero())
	require.Len(t, out.Receipts, 1)
	require.Len(t, out.NewlyRead, 1)

	// Verify message was marked as read
	var readByRecipient bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT read_by_recipient FROM messages WHERE id = $1`, msgID).Scan(&readByRecipient))
	require.True(t, readByRecipient, "message must be marked as read")

	// Upfront delivery filtering rule: since message was already delivered for secondary device (delivered_to_recipient=true),
	// p2_last_delivered_at must NOT be falsely advanced to time.Now()
	var p2LastDeliveredAt *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT p2_last_delivered_at FROM chats WHERE id = $1`, chatID).Scan(&p2LastDeliveredAt))
	require.NotNil(t, p2LastDeliveredAt)
	require.True(t, initialDeliveredAt.Equal(*p2LastDeliveredAt),
		"p2_last_delivered_at must remain untouched when message was already delivered to secondary (expected %v, got %v)",
		initialDeliveredAt, *p2LastDeliveredAt)
}

// TestAcknowledgeDeliveryBatch_DeletesFullyReadMessageFromRelay pins the relay
// decision for the batch ACK path on a real database. When the message is
// already read and read-acked, the
// batch must hard-delete the row, exactly like the old per-message loop did.
//
// The batch UPDATE and the follow-up chat cleanup run against pgxmock so that
// deleteMessageFromRelay's own transaction is the only thing that touches the
// real row. A strip leaves the row behind with empty content; a delete removes
// it.
func TestAcknowledgeDeliveryBatch_DeletesFullyReadMessageFromRelay(t *testing.T) {
	ctx, pool := newLivePool(t, 30*time.Second)

	chatID := uuid.New()
	msgID := uuid.New()

	u1, u2 := insertLiveUsers(t, ctx, pool, "RL", "rl-")

	_, err := pool.Exec(ctx, `
		INSERT INTO chats (id, participant_1_id, participant_2_id, created_at, updated_at)
		VALUES ($1, $2, $3, now(), now())`, chatID, u1, u2)
	require.NoError(t, err)

	// The recipient has already read the message on another device, but its
	// primary device has not confirmed delivery yet — that is exactly the state
	// the batch ACK is about to resolve.
	_, err = pool.Exec(ctx, `
		INSERT INTO messages (
			id, chat_id, sender_id, recipient_id, content, message_type,
			sender_session_id,
			delivered_to_recipient, delivered_to_recipient_primary, synced_to_sender_primary,
			read_by_recipient, read_acked_by_sender,
			deleted_by_sender, deleted_by_recipient,
			expires_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, 'relay-hello', 'text',
			$5,
			TRUE, FALSE, TRUE,
			TRUE, TRUE,
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

	now := time.Now().Truncate(time.Microsecond)

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	// Upfront lookup: the row exists in the relay, the recipient has already read
	// it on a secondary device, and primary delivery is still pending.
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs([]uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(msgID, chatID, u1, u2, nil, true, false, true, true, nil, true, now, now.Add(24*time.Hour)))

	mock.ExpectQuery(`(?s)UPDATE messages\s+SET.*delivered_to_recipient_primary = TRUE.*WHERE.*`).
		WithArgs(u2, []uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(batchAckColumns()).
			AddRow(msgID, chatID, u1, nil, true, true, true, true, now))
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+id = \$3`).
		WithArgs(u2, now, chatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	// Keep the cleanup out of the real table so only deleteMessageFromRelay can
	// change the row.
	mock.ExpectExec(`(?s)DELETE FROM messages\s+WHERE\s+chat_id = \$1`).
		WithArgs(chatID).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	store := personal_chat_store.New(mock)
	svc := &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	res, ackErr := svc.AcknowledgeDeliveryBatch(ctx, []string{msgID.String()}, "recipient", true, kit.UserId{UuidUserId: u2, StringUserId: u2.String()})
	require.NoError(t, ackErr)
	require.Equal(t, int32(1), res.AcknowledgedCount)

	require.Eventually(t, func() bool {
		return mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond, "async relay cleanup must complete")

	var rowExists bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1)`, msgID).Scan(&rowExists))
	require.False(t, rowExists,
		"fully read message must be deleted from the relay by the batch ACK, not stripped")
}

// TestAcknowledgeDeliveryBatch_RetriesRelayCleanupForAlreadyAckedRow pins the
// retry behaviour on a real database. When both primary devices already have
// the message, the batch UPDATE matches no rows, so the row never shows up in
// RETURNING. A repeat ACK must still retry the relay cleanup, exactly like the
// old single-message endpoint did when it re-read the row after the update.
//
// The batch UPDATE and the chat cleanup run against pgxmock; only
// deleteMessageFromRelay's own transaction touches the real row. A strip
// leaves the row with empty content, which is the observable effect here.
func TestAcknowledgeDeliveryBatch_RetriesRelayCleanupForAlreadyAckedRow(t *testing.T) {
	ctx, pool := newLivePool(t, 30*time.Second)

	chatID := uuid.New()
	msgID := uuid.New()

	u1, u2 := insertLiveUsers(t, ctx, pool, "RR", "rr-")

	_, err := pool.Exec(ctx, `
		INSERT INTO chats (id, participant_1_id, participant_2_id, created_at, updated_at)
		VALUES ($1, $2, $3, now(), now())`, chatID, u1, u2)
	require.NoError(t, err)

	// Both primary devices already hold the message; the payload is still on the
	// relay because the first cleanup failed. Read flags stay false so the retry
	// must strip (not delete), which leaves a row we can inspect.
	_, err = pool.Exec(ctx, `
		INSERT INTO messages (
			id, chat_id, sender_id, recipient_id, content, message_type,
			sender_session_id,
			delivered_to_recipient, delivered_to_recipient_primary, synced_to_sender_primary,
			read_by_recipient, read_acked_by_sender,
			deleted_by_sender, deleted_by_recipient,
			expires_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, 'retry-hello', 'text',
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

	now := time.Now().Truncate(time.Microsecond)

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	// Upfront lookup confirms the row exists, belongs to the caller, and is already delivered to primary.
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs([]uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(msgID, chatID, u1, u2, nil, true, true, true, false, nil, false, now, now.Add(24*time.Hour)))
	// Keep the chat cleanup out of the real table.
	mock.ExpectExec(`(?s)DELETE FROM messages\s+WHERE\s+chat_id = \$1`).
		WithArgs(chatID).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	store := personal_chat_store.New(mock)
	svc := &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	res, ackErr := svc.AcknowledgeDeliveryBatch(ctx, []string{msgID.String()}, "recipient", true, kit.UserId{UuidUserId: u2, StringUserId: u2.String()})
	require.NoError(t, ackErr)
	require.Equal(t, int32(1), res.AcknowledgedCount, "an existing owned row is acknowledged even when nothing changed")

	require.Eventually(t, func() bool {
		var content string
		if err := pool.QueryRow(ctx, `SELECT content FROM messages WHERE id = $1`, msgID).Scan(&content); err != nil {
			return false
		}
		return content == ""
	}, 5*time.Second, 20*time.Millisecond,
		"repeat ACK must clean up the already-acknowledged relay row")

	require.NoError(t, mock.ExpectationsWereMet())
}

type spyPendingUploads struct {
	mu                   sync.Mutex
	registeredFiles      []string
	registeredBatchFiles []string
	removedFiles         []string
}

func (s *spyPendingUploads) Register(ctx context.Context, fileID, bucket, r2Key string, expiresAt time.Time) error {
	return nil
}

func (s *spyPendingUploads) Lookup(ctx context.Context, fileID string) (pending_uploads.PendingUpload, error) {
	return pending_uploads.PendingUpload{FileID: fileID}, nil
}

func (s *spyPendingUploads) Remove(ctx context.Context, fileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removedFiles = append(s.removedFiles, fileID)
	return nil
}

func (s *spyPendingUploads) LookupTx(ctx context.Context, tx pgx.Tx, fileID string) (pending_uploads.PendingUpload, error) {
	return pending_uploads.PendingUpload{FileID: fileID}, nil
}

func (s *spyPendingUploads) RemoveTx(ctx context.Context, tx pgx.Tx, fileID string) error {
	return nil
}

func (s *spyPendingUploads) RegisterTx(ctx context.Context, tx pgx.Tx, fileID, bucket, r2Key string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registeredFiles = append(s.registeredFiles, fileID)
	return nil
}

func (s *spyPendingUploads) RegisterBatchTx(ctx context.Context, tx pgx.Tx, fileIDs, buckets, r2Keys []string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registeredBatchFiles = append(s.registeredBatchFiles, fileIDs...)
	s.registeredFiles = append(s.registeredFiles, fileIDs...)
	return nil
}

func TestDeleteMessageFromRelay_LiveDBAndPendingUploadsVerification(t *testing.T) {
	ctx, pool := newLivePool(t, 30*time.Second)

	chatID := uuid.New()
	msgID := uuid.New()
	fileID := "mock-account:test-file-key-relay"

	u1, u2 := insertLiveUsers(t, ctx, pool, "R2", "r2-")

	_, err := pool.Exec(ctx, `
		INSERT INTO chats (id, participant_1_id, participant_2_id, p1_last_message_content, p1_last_message_type, p2_last_message_content, p2_last_message_type, created_at, updated_at)
		VALUES ($1, $2, $3, 'preview 1', 'file', 'preview 2', 'file', now(), now());
	`, chatID, u1, u2)
	require.NoError(t, err)

	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE chat_id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM chats WHERE id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, u1, u2)
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_users WHERE id IN ($1, $2)`, u1, u2)
	}()

	// 2. Insert message with file_id
	_, err = pool.Exec(ctx, `
		INSERT INTO messages (
			id, chat_id, sender_id, recipient_id, content, message_type,
			file_id, file_name, file_size,
			sender_session_id,
			delivered_to_recipient, delivered_to_recipient_primary, synced_to_sender_primary,
			read_by_recipient, read_acked_by_sender,
			deleted_by_sender, deleted_by_recipient,
			expires_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, 'file content', 'file',
			$5, 'test.jpg', 1024,
			$6,
			TRUE, TRUE, TRUE,
			TRUE, TRUE,
			FALSE, FALSE,
			now() + interval '1 day', now(), now()
		);
	`, msgID, chatID, u1, u2, fileID, uuid.New())
	require.NoError(t, err)

	// 3. Setup chatService
	r2Pool, err := clients.NewR2ClientPool(&config.R2PoolConfig{
		Accounts: []config.R2AccountConfig{
			{
				Name:             "mock-account",
				AccountID:        "mock-account-id",
				AccessKeyID:      "mock-access-key",
				SecretAccessKey:  "mock-secret-key",
				ChatFilesBucket:  "mock-bucket",
				ProfilePicBucket: "mock-profile-bucket",
			},
		},
		PrimaryChatAccount:    "mock-account",
		PrimaryProfileAccount: "mock-account",
	})
	require.NoError(t, err)

	store := personal_chat_store.New(pool)
	spy := &spyPendingUploads{}

	svc := &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  spy,
		R2Pool:          r2Pool,
	}

	msgRow, err := store.GetMessageByID(ctx, msgID)
	require.NoError(t, err)

	// 4. Run deleteMessageFromRelay
	svc.deleteMessageFromRelay(ctx, msgRow)

	// 5. Verification:
	// A) File must be registered in pending_uploads inside the database transaction
	spy.mu.Lock()
	require.Contains(t, spy.registeredFiles, fileID, "File must be registered in pending_uploads before R2 deletion")
	spy.mu.Unlock()

	// B) Message must be deleted from messages table because all acks (read, read_ack) are true
	var exists bool
	err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1)`, msgID).Scan(&exists)
	require.NoError(t, err)
	require.False(t, exists, "Message must be deleted from database after successful relay cleanup")
}

// TestDeleteMessageFromRelay_LiveDB_SkipsUntilBothPrimaryDevicesHoldMessage
// pins the helper's own gate on a real database. A secondary device may mark a
// message read while the recipient's primary device still has no copy, and the
// sender's primary device may still be missing it too. In both states the relay
// row and its R2 file must survive; only the helper's own primary flags decide.

// TestDeleteMessageFromRelay_LiveDB_SkipsUntilBothPrimaryDevicesHoldMessage
// pins the helper's own gate on a real database. A secondary device may mark a
// message read while the recipient's primary device still has no copy, and the
// sender's primary device may still be missing it too. In both states the relay
// row and its R2 file must survive; only the helper's own primary flags decide.
func TestDeleteMessageFromRelay_LiveDB_SkipsUntilBothPrimaryDevicesHoldMessage(t *testing.T) {
	ctx, pool := newLivePool(t, 30*time.Second)

	chatID := uuid.New()

	u1, u2 := insertLiveUsers(t, ctx, pool, "Gate", "gate-")

	_, err := pool.Exec(ctx, `
		INSERT INTO chats (id, participant_1_id, participant_2_id, created_at, updated_at)
		VALUES ($1, $2, $3, now(), now())`, chatID, u1, u2)
	require.NoError(t, err)

	type blockedMessage struct {
		id     uuid.UUID
		fileID string
	}
	messages := []blockedMessage{
		{id: uuid.New(), fileID: "mock-account:blocked-without-recipient-primary"},
		{id: uuid.New(), fileID: "mock-account:blocked-without-sender-primary"},
	}

	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM pending_uploads WHERE file_id = ANY($1)`,
			[]string{messages[0].fileID, messages[1].fileID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE chat_id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM chats WHERE id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, u1, u2)
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_users WHERE id IN ($1, $2)`, u1, u2)
	}()

	for i, m := range messages {
		// Row 0 is read on a secondary device before the recipient's primary
		// device has it. Row 1 is read and acked before the sender's primary
		// device has it. Read/read-ack are true on both, so only
		// the primary flags can stop the helper.
		deliveredPrimary := i != 0
		syncedPrimary := i != 1

		_, err = pool.Exec(ctx, `
			INSERT INTO messages (
				id, chat_id, sender_id, recipient_id, content, message_type,
				file_id, file_name, file_size,
				sender_session_id,
				delivered_to_recipient, delivered_to_recipient_primary, synced_to_sender_primary,
				read_by_recipient, read_acked_by_sender,
				deleted_by_sender, deleted_by_recipient,
				expires_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, 'file content', 'file',
				$5, 'test.jpg', 1024,
				$6,
				TRUE, $7, $8,
				TRUE, TRUE,
				FALSE, FALSE,
				now() + interval '1 day', now(), now()
			);
		`, m.id, chatID, u1, u2, m.fileID, uuid.New(), deliveredPrimary, syncedPrimary)
		require.NoError(t, err)
	}

	r2Pool, err := clients.NewR2ClientPool(&config.R2PoolConfig{
		Accounts: []config.R2AccountConfig{
			{
				Name:             "mock-account",
				AccountID:        "mock-account-id",
				AccessKeyID:      "mock-access-key",
				SecretAccessKey:  "mock-secret-key",
				ChatFilesBucket:  "mock-bucket",
				ProfilePicBucket: "mock-profile-pic-bucket",
			},
		},
		PrimaryChatAccount:    "mock-account",
		PrimaryProfileAccount: "mock-account",
	})
	require.NoError(t, err)

	store := personal_chat_store.New(pool)
	spy := &spyPendingUploads{}

	svc := &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  spy,
		R2Pool:          r2Pool,
	}

	for i, m := range messages {
		msgRow, getErr := store.GetMessageByID(ctx, m.id)
		require.NoError(t, getErr)
		require.Equal(t, i != 0, msgRow.DeliveredToRecipientPrimary)
		require.Equal(t, i != 1, msgRow.SyncedToSenderPrimary)

		svc.deleteMessageFromRelay(ctx, msgRow)

		spy.mu.Lock()
		require.NotContains(t, spy.registeredFiles, m.fileID,
			"file must not be queued for R2 deletion before both primary devices hold the message")
		spy.mu.Unlock()

		var content string
		require.NoError(t, pool.QueryRow(ctx, `SELECT content FROM messages WHERE id = $1`, m.id).Scan(&content))
		require.Equal(t, "file content", content, "relay payload must stay intact")
	}
}
