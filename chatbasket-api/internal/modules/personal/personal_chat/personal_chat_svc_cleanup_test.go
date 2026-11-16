package personal_chat

import (
	"chatbasket-api/internal/modules/core/pending_uploads"
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/config"
	"chatbasket-api/internal/platform/kit"
	"context"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPendingUploadsChat struct{}

func (m *mockPendingUploadsChat) Register(ctx context.Context, fileID, bucket, r2Key string, expiresAt time.Time) error {
	return nil
}

func (m *mockPendingUploadsChat) Lookup(ctx context.Context, fileID string) (pending_uploads.PendingUpload, error) {
	return pending_uploads.PendingUpload{FileID: fileID}, nil
}

func (m *mockPendingUploadsChat) Remove(ctx context.Context, fileID string) error {
	return nil
}

func (m *mockPendingUploadsChat) LookupTx(ctx context.Context, tx pgx.Tx, fileID string) (pending_uploads.PendingUpload, error) {
	return pending_uploads.PendingUpload{FileID: fileID}, nil
}

func (m *mockPendingUploadsChat) RemoveTx(ctx context.Context, tx pgx.Tx, fileID string) error {
	return nil
}

func (m *mockPendingUploadsChat) RegisterTx(ctx context.Context, tx pgx.Tx, fileID, bucket, r2Key string, expiresAt time.Time) error {
	return nil
}

func (m *mockPendingUploadsChat) RegisterBatchTx(ctx context.Context, tx pgx.Tx, fileIDs, buckets, r2Keys []string, expiresAt time.Time) error {
	return nil
}

func setupChatServiceMock(t *testing.T) (*chatService, pgxmock.PgxPoolIface) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)

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

	store := personal_chat_store.New(mock)

	svc := &chatService{
		Pool:               nil, // Not used in CleanupExpiredMessages
		PostgresQuerier:    store,
		PostgresQueries:    store,
		MaintenanceQuerier: store,
		MaintenanceQueries: store,
		PendingUploads:     &mockPendingUploadsChat{},
		R2Pool:             r2Pool,
	}

	return svc, mock
}

func TestCleanupExpiredMessages_Mock(t *testing.T) {
	svc, mock := setupChatServiceMock(t)
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Millisecond)
	defer cancel()

	// Define standard 29 columns returned by GetExpiredMessagesWithFiles & GetMessagesWithFilesForBlockedUsers
	columns := []string{
		"id", "chat_id", "sender_id", "recipient_id", "content", "message_type", "reply_to_message_id",
		"file_id", "file_name", "file_size", "file_mime_type", "file_token_id", "file_token_secret", "file_token_expiry",
		"thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary", "synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient", "delivery_attempts", "expires_at", "created_at", "updated_at",
		"read_by_recipient", "read_acked_by_sender", "read_at",
		"sender_session_id",
	}

	msg1ID := uuid.New()
	fileID1 := "retired:file-unsent"
	msg1 := personal_chat_store.Message{
		ID:          msg1ID,
		MessageType: "unsent",
		FileID:      &fileID1,
	}

	msg2ID := uuid.New()
	fileID2 := "retired:file-text"
	msg2 := personal_chat_store.Message{
		ID:          msg2ID,
		MessageType: "text",
		FileID:      &fileID2,
	}

	msg3ID := uuid.New()
	msg3 := personal_chat_store.Message{
		ID:          msg3ID,
		MessageType: "unsent",
		FileID:      nil,
	}

	msg4ID := uuid.New()
	msg4 := personal_chat_store.Message{
		ID:          msg4ID,
		MessageType: "text",
		FileID:      nil,
	}

	msg5ID := uuid.New()
	fileID5 := "retired:file-unexpired"
	msg5 := personal_chat_store.Message{
		ID:          msg5ID,
		MessageType: "file",
		FileID:      &fileID5,
	}
	futureTime := time.Now().Add(time.Hour)

	trueVal := true

	// 1. Mock first call to GetMessagesWithFilesForBlockedUsers (Job 1: returns 1 message)
	msgBlockedID := uuid.New()
	fileIDBlocked := "retired:file-blocked"
	msgBlocked := personal_chat_store.Message{
		ID:          msgBlockedID,
		MessageType: "text",
		FileID:      &fileIDBlocked,
	}

	mock.ExpectQuery(`(?s)SELECT .* FROM messages m INNER JOIN chats c ON m.chat_id = c.id INNER JOIN user_blocks ub .* WHERE m.file_id IS NOT NULL`).
		WithArgs(uuid.Nil, int32(100)).
		WillReturnRows(pgxmock.NewRows(columns).
			AddRow(msgBlocked.ID, uuid.New(), uuid.New(), uuid.New(), "content", msgBlocked.MessageType, nil, msgBlocked.FileID, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, trueVal, true, false, false, int32(0), time.Now(), time.Now(), time.Now(), false, false, nil, uuid.New()))

	// Mock DB expectation for blocked message -> calls DeleteMessage
	mock.ExpectExec(`DELETE FROM messages WHERE id = \$1`).
		WithArgs(msgBlockedID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	// 2. Mock call to GetExpiredMessagesWithFiles (Job 2: returns 5 messages)
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE .* expires_at < now\(\) .* file_id IS NOT NULL`).
		WithArgs(int32(100), uuid.Nil).
		WillReturnRows(pgxmock.NewRows(columns).
			AddRow(msg1.ID, uuid.New(), uuid.New(), uuid.New(), "content", msg1.MessageType, nil, msg1.FileID, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, trueVal, true, false, false, int32(0), time.Now(), time.Now(), time.Now(), false, false, nil, uuid.New()).
			AddRow(msg2.ID, uuid.New(), uuid.New(), uuid.New(), "content", msg2.MessageType, nil, msg2.FileID, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, trueVal, true, false, false, int32(0), time.Now(), time.Now(), time.Now(), false, false, nil, uuid.New()).
			AddRow(msg3.ID, uuid.New(), uuid.New(), uuid.New(), "content", msg3.MessageType, nil, msg3.FileID, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, trueVal, true, false, false, int32(0), time.Now(), time.Now(), time.Now(), false, false, nil, uuid.New()).
			AddRow(msg4.ID, uuid.New(), uuid.New(), uuid.New(), "content", msg4.MessageType, nil, msg4.FileID, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, trueVal, true, false, false, int32(0), time.Now(), time.Now(), time.Now(), false, false, nil, uuid.New()).
			AddRow(msg5.ID, uuid.New(), uuid.New(), uuid.New(), "content", msg5.MessageType, nil, msg5.FileID, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, trueVal, true, false, false, int32(0), futureTime, time.Now(), time.Now(), false, false, nil, uuid.New()))

	// Mock cleanup database expectations for GetExpiredMessagesWithFiles.
	// TTL-expired file rows are deleted wholesale after R2 acceptance; unexpired
	// both-primary rows are only stripped and stay for relay cleanup.
	mock.ExpectExec(`(?s)UPDATE messages.*SET.*content = ''.*WHERE id = \$1`).
		WithArgs(msg5ID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	mock.ExpectExec(`(?s)DELETE FROM messages\s+WHERE id = ANY`).
		WithArgs([]uuid.UUID{msg1ID, msg2ID}).
		WillReturnResult(pgxmock.NewResult("DELETE", 2))

	// Run cleanup job
	err := svc.CleanupExpiredMessages(ctx)
	assert.NoError(t, err)

	// Verify all mock expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestCleanupExpiredMessages_Job1InFlightBatchPreserved asserts that if Job 1 (Blocked users) hits its
// budget during an in-flight batch of 100 rows, that batch is NOT cancelled, finishes
// completely, and the worker stops before starting a second batch for Job 1.
func TestCleanupExpiredMessages_Job1InFlightBatchPreserved(t *testing.T) {
	svc, mock := setupChatServiceMock(t)
	defer mock.Close()

	columns := []string{
		"id", "chat_id", "sender_id", "recipient_id", "content", "message_type", "reply_to_message_id",
		"file_id", "file_name", "file_size", "file_mime_type", "file_token_id", "file_token_secret", "file_token_expiry",
		"thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary", "synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient", "delivery_attempts", "expires_at", "created_at", "updated_at",
		"read_by_recipient", "read_acked_by_sender", "read_at",
		"sender_session_id",
	}

	// Job 1 (Blocked Users): Return 100 blocked messages in batch 1
	rowsJob1 := pgxmock.NewRows(columns)
	blockedIDs := make([]uuid.UUID, 100)
	trueVal := true
	for i := 0; i < 100; i++ {
		blockedIDs[i] = uuid.New()
		fID := fmt.Sprintf("retired:file_%03d", i)
		rowsJob1.AddRow(
			blockedIDs[i], uuid.New(), uuid.New(), uuid.New(), "content", "file", nil,
			&fID, nil, nil, nil, nil, nil, nil,
			nil, nil, nil,
			true, trueVal, true,
			false, false, int32(0), time.Now(), time.Now(), time.Now(),
			false, false, nil,
			uuid.New(),
		)
	}

	mock.ExpectQuery(`(?s)SELECT .* FROM messages m INNER JOIN chats c ON m.chat_id = c.id INNER JOIN user_blocks ub .* WHERE m.file_id IS NOT NULL`).
		WithArgs(uuid.Nil, int32(100)).
		WillReturnRows(rowsJob1).
		WillDelayFor(50 * time.Millisecond)

	// In-flight batch of 100 rows must all be deleted via DeleteMessage
	for i := 0; i < 100; i++ {
		mock.ExpectExec(`DELETE FROM messages WHERE id = \$1`).
			WithArgs(blockedIDs[i]).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))
	}

	// Call with a 20ms outer budget (Job 1 budget: ~9ms). The mocked fetch takes
	// 50ms, so the batch finishes after its budget expires. The worker must still
	// complete the whole batch, then stop before requesting Batch 2 and skip Job 2
	// because the outer budget is already gone.
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := svc.CleanupExpiredMessages(ctxTimeout)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestCleanupExpiredMessages_BudgetTransferAndGap asserts that if Job 1 (Blocked users) finishes early,
// its unused budget is transferred to Job 2 (Expired messages), and the gap between jobs is honored.

// TestCleanupExpiredMessages_BudgetTransferAndGap asserts that if Job 1 (Blocked users) finishes early,
// its unused budget is transferred to Job 2 (Expired messages), and the gap between jobs is honored.
func TestCleanupExpiredMessages_BudgetTransferAndGap(t *testing.T) {
	svc, mock := setupChatServiceMock(t)
	defer mock.Close()

	columns := []string{
		"id", "chat_id", "sender_id", "recipient_id", "content", "message_type", "reply_to_message_id",
		"file_id", "file_name", "file_size", "file_mime_type", "file_token_id", "file_token_secret", "file_token_expiry",
		"thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary", "synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient", "delivery_attempts", "expires_at", "created_at", "updated_at",
		"read_by_recipient", "read_acked_by_sender", "read_at",
		"sender_session_id",
	}

	// Job 1 (Blocked Users): Empty (finishes immediately, early)
	mock.ExpectQuery(`(?s)SELECT .* FROM messages m INNER JOIN chats c ON m.chat_id = c.id INNER JOIN user_blocks ub .* WHERE m.file_id IS NOT NULL`).
		WithArgs(uuid.Nil, int32(100)).
		WillReturnRows(pgxmock.NewRows(columns))

	// Job 2 (Expired Messages): Empty
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE .* expires_at < now\(\) .* file_id IS NOT NULL`).
		WithArgs(int32(100), uuid.Nil).
		WillReturnRows(pgxmock.NewRows(columns))

	const outerBudget = 200 * time.Millisecond
	expectedGap := (outerBudget * 5) / 55
	ctxTimeout, cancel := context.WithTimeout(context.Background(), outerBudget)
	defer cancel()

	start := time.Now()
	err := svc.CleanupExpiredMessages(ctxTimeout)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.True(t, elapsed >= expectedGap, "must wait at least the gap duration before finishing")
}

// TestCleanupExpiredMessages_Job2InFlightBatchPreserved asserts that if Job 2 (Expired messages) hits its
// budget during an in-flight batch of 100 rows, that batch is NOT cancelled, finishes
// completely, and the worker stops before starting a second batch for Job 2.

// TestCleanupExpiredMessages_Job2InFlightBatchPreserved asserts that if Job 2 (Expired messages) hits its
// budget during an in-flight batch of 100 rows, that batch is NOT cancelled, finishes
// completely, and the worker stops before starting a second batch for Job 2.
func TestCleanupExpiredMessages_Job2InFlightBatchPreserved(t *testing.T) {
	svc, mock := setupChatServiceMock(t)
	defer mock.Close()

	columns := []string{
		"id", "chat_id", "sender_id", "recipient_id", "content", "message_type", "reply_to_message_id",
		"file_id", "file_name", "file_size", "file_mime_type", "file_token_id", "file_token_secret", "file_token_expiry",
		"thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary", "synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient", "delivery_attempts", "expires_at", "created_at", "updated_at",
		"read_by_recipient", "read_acked_by_sender", "read_at",
		"sender_session_id",
	}

	// Job 1 (Blocked Users): Empty (finishes immediately, early)
	mock.ExpectQuery(`(?s)SELECT .* FROM messages m INNER JOIN chats c ON m.chat_id = c.id INNER JOIN user_blocks ub .* WHERE m.file_id IS NOT NULL`).
		WithArgs(uuid.Nil, int32(100)).
		WillReturnRows(pgxmock.NewRows(columns))

	// Job 2 (Expired Messages): Return 100 expired messages in Batch 1
	rowsJob2 := pgxmock.NewRows(columns)
	expiredIDs := make([]uuid.UUID, 100)
	trueVal := true
	for i := 0; i < 100; i++ {
		expiredIDs[i] = uuid.New()
		fID := fmt.Sprintf("retired:file_%03d", i)
		rowsJob2.AddRow(
			expiredIDs[i], uuid.New(), uuid.New(), uuid.New(), "content", "file", nil,
			&fID, nil, nil, nil, nil, nil, nil,
			nil, nil, nil,
			true, trueVal, true,
			false, false, int32(0), time.Now().Add(-time.Hour), time.Now(), time.Now(),
			false, false, nil,
			uuid.New(),
		)
	}

	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE .* expires_at < now\(\) .* file_id IS NOT NULL`).
		WithArgs(int32(100), uuid.Nil).
		WillReturnRows(rowsJob2).
		WillDelayFor(50 * time.Millisecond)

	// All 100 items in Job 2's in-flight batch must be deleted via DeleteExpiredMessagesWithFilesBatch
	mock.ExpectExec(`(?s)DELETE FROM messages\s+WHERE id = ANY`).
		WithArgs(expiredIDs).
		WillReturnResult(pgxmock.NewResult("DELETE", 100))

	// Call with a 20ms outer budget. Job 2's mocked fetch takes 50ms, so the
	// in-flight batch finishes after the budget expires. It must still process
	// all 100 rows, then stop before requesting Batch 2.
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := svc.CleanupExpiredMessages(ctxTimeout)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

type mockAuthProviderForAck struct {
	isCentral bool
}

func (m *mockAuthProviderForAck) IsSessionCentral(ctx context.Context, userID uuid.UUID, sessionID string) (bool, error) {
	return m.isCentral, nil
}

func (m *mockAuthProviderForAck) GetUserPrimarySessionID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func (m *mockAuthProviderForAck) GetSessionE2EEPublicKey(ctx context.Context, sessionID uuid.UUID) (*string, error) {
	return nil, nil
}

// TestAcknowledgeDelivery_TriggersAsyncRelayAndChatCleanup verifies the ACK path
// hands relay deletion and chat cleanup to the background goroutine: the write
// path itself only runs the synchronous queries, but both async steps must run.

// TestAcknowledgeDelivery_TriggersAsyncRelayAndChatCleanup verifies the ACK path
// hands relay deletion and chat cleanup to the background goroutine: the write
// path itself only runs the synchronous queries, but both async steps must run.
func TestAcknowledgeDelivery_TriggersAsyncRelayAndChatCleanup(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	safeBuf := &safeLogBuffer{}
	origOutput := log.Writer()
	log.SetOutput(safeBuf)
	defer log.SetOutput(origOutput)

	store := personal_chat_store.New(mock)
	authMock := &mockAuthProviderForAck{isCentral: true}
	svc := &chatService{
		PostgresQuerier: store,
		PostgresQueries: store,
		AuthProvider:    authMock,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	messageID := uuid.New()
	chatID := uuid.New()
	now := time.Now()

	initialMsg := personal_chat_store.Message{
		ID:                          messageID,
		ChatID:                      chatID,
		SenderID:                    uuid.New(),
		RecipientID:                 userID.UuidUserId,
		Content:                     "encrypted-content",
		MessageType:                 "text",
		DeliveredToRecipient:        false,
		DeliveredToRecipientPrimary: false,
		SyncedToSenderPrimary:       true,
		ReadByRecipient:             false,
		ReadAckedBySender:           false,
		CreatedAt:                   now,
		UpdatedAt:                   now,
		ExpiresAt:                   now.Add(24 * time.Hour),
	}

	updatedMsg := initialMsg
	updatedMsg.DeliveredToRecipient = true
	updatedMsg.DeliveredToRecipientPrimary = true

	// 1. Upfront GET query
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs([]uuid.UUID{messageID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(messageID, chatID, initialMsg.SenderID, userID.UuidUserId, nil, false, false, true, false, nil, false, now, now.Add(24*time.Hour)))

	// 2. Batch UPDATE query
	batchRows := pgxmock.NewRows(batchAckColumns()).
		AddRow(messageID, chatID, initialMsg.SenderID, nil, true, true, false, false, now)

	mock.ExpectQuery(`(?s)UPDATE messages\s+SET.*delivered_to_recipient_primary = TRUE.*WHERE.*`).
		WithArgs(userID.UuidUserId, []uuid.UUID{messageID}).
		WillReturnRows(batchRows)

	// 3. UpdateChatLastDeliveredAt
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+id = \$3`).
		WithArgs(userID.UuidUserId, now, chatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 4. Chat cleanup is expected on the background goroutine, not skipped.
	mock.ExpectExec(`(?s)DELETE FROM messages\s+WHERE\s+chat_id = \$1`).
		WithArgs(chatID).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	err = svc.AcknowledgeDelivery(context.Background(), messageID, "recipient", true, userID)
	require.NoError(t, err)

	// The relay candidate reaches deleteMessageFromRelay (Pool is nil here, so it
	// logs and returns) and the cleanup query is executed asynchronously.
	relayLog := fmt.Sprintf("[Relay-Cleanup] Message %s fully delivered and synced", messageID)
	require.Eventually(t, func() bool {
		return strings.Contains(safeBuf.String(), relayLog) && mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond, "async relay and chat cleanup must complete")
}

// TestDeleteMessageForMe_TriggersAsyncRelayCleanup verifies an eligible delete
// hands relay cleanup to the background goroutine instead of skipping it.

// TestDeleteMessageForMe_TriggersAsyncRelayCleanup verifies an eligible delete
// hands relay cleanup to the background goroutine instead of skipping it.
func TestDeleteMessageForMe_TriggersAsyncRelayCleanup(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	safeBuf := &safeLogBuffer{}
	origOutput := log.Writer()
	log.SetOutput(safeBuf)
	defer log.SetOutput(origOutput)

	store := personal_chat_store.New(mock)
	svc := &chatService{
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	messageID := uuid.New()
	chatID := uuid.New()
	now := time.Now()

	msg := personal_chat_store.Message{
		ID:                          messageID,
		ChatID:                      chatID,
		SenderID:                    userID.UuidUserId,
		RecipientID:                 uuid.New(),
		Content:                     "delete-me",
		MessageType:                 "text",
		DeliveredToRecipientPrimary: true,
		SyncedToSenderPrimary:       true,
		CreatedAt:                   now,
		UpdatedAt:                   now,
		ExpiresAt:                   now.Add(24 * time.Hour),
	}

	// 1. Batch fetch GetMessagesForDeleteForMeBatch
	cols := []string{
		"id", "chat_id", "sender_id", "recipient_id", "file_id",
		"delivered_to_recipient_primary", "synced_to_sender_primary",
		"read_by_recipient", "read_acked_by_sender",
		"deleted_by_sender", "deleted_by_recipient", "expires_at",
	}
	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{messageID}).
		WillReturnRows(pgxmock.NewRows(cols).AddRow(
			messageID, chatID, userID.UuidUserId, msg.RecipientID, nil,
			true, true, false, false, false, false, now.Add(24*time.Hour),
		))

	// 2. ClearLastMessageForParticipantBatch
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(userID.UuidUserId, []uuid.UUID{messageID}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 3. MarkMessagesDeletedBySenderBatch
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_sender = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+sender_id = \$2\s+AND\s+deleted_by_sender = FALSE`).
		WithArgs([]uuid.UUID{messageID}, userID.UuidUserId).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	_, err = svc.DeleteMessageForMe(context.Background(), []uuid.UUID{messageID}, userID, true)
	require.NoError(t, err)

	// The background goroutine must reach deleteMessageFromRelay. Pool is nil in
	// this unit test, so the observable effect is the relay cleanup log line.
	relayLog := fmt.Sprintf("[Relay-Cleanup] Message %s fully delivered and synced", messageID)
	require.Eventually(t, func() bool {
		return strings.Contains(safeBuf.String(), relayLog)
	}, 5*time.Second, 10*time.Millisecond, "async relay cleanup must be triggered")
}

func TestCleanupJobs_UseMaintenanceQueriesNotMainPool(t *testing.T) {
	mainMock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mainMock.Close()

	maintMock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer maintMock.Close()

	mainStore := personal_chat_store.New(mainMock)
	maintStore := personal_chat_store.New(maintMock)

	svc := &chatService{
		PostgresQuerier:    mainStore,
		PostgresQueries:    mainStore,
		MaintenanceQuerier: maintStore,
		MaintenanceQueries: maintStore,
		PendingUploads:     &mockPendingUploadsChat{},
	}

	// Verify CleanupExpiredMessages routes queries strictly through MaintenanceQueries
	cols := []string{
		"id", "chat_id", "sender_id", "recipient_id", "content", "message_type", "reply_to_message_id",
		"file_id", "file_name", "file_size", "file_mime_type", "file_token_id", "file_token_secret", "file_token_expiry",
		"thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary", "synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient", "delivery_attempts", "expires_at", "created_at", "updated_at",
		"read_by_recipient", "read_acked_by_sender", "read_at",
		"sender_session_id",
	}
	maintMock.ExpectQuery("SELECT .* FROM messages").
		WithArgs(uuid.Nil, int32(100)).
		WillReturnRows(pgxmock.NewRows(cols))
	maintMock.ExpectQuery("SELECT .* FROM messages").
		WithArgs(int32(100), uuid.Nil).
		WillReturnRows(pgxmock.NewRows(cols))

	ctxMsg, cancelMsg := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelMsg()
	err = svc.CleanupExpiredMessages(ctxMsg)
	require.NoError(t, err)

	// Verify CleanupDatabaseOnly routes all 6 batch jobs strictly through MaintenanceQueries
	for i := 0; i < 6; i++ {
		maintMock.ExpectExec("WITH batch AS").
			WithArgs(int32(5000)).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
	}

	err = svc.CleanupDatabaseOnly(context.Background())
	require.NoError(t, err)

	assert.NoError(t, mainMock.ExpectationsWereMet())
	assert.NoError(t, maintMock.ExpectationsWereMet())
}
