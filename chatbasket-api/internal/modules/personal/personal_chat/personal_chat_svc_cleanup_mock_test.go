package personal_chat

import (
	"context"
	"testing"
	"time"

	"chatbasket-api/internal/modules/core/pending_uploads"
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/config"
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
		Pool:            nil, // Not used in CleanupExpiredMessages
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
		R2Pool:          r2Pool,
	}

	return svc, mock
}

func TestCleanupExpiredMessages_Mock(t *testing.T) {
	svc, mock := setupChatServiceMock(t)
	defer mock.Close()

	ctx := context.Background()

	// Define standard 25 columns returned by GetExpiredMessagesWithFiles & GetMessagesWithFilesForBlockedUsers
	columns := []string{
		"id", "chat_id", "sender_id", "recipient_id", "content", "message_type",
		"file_id", "file_name", "file_size", "file_mime_type", "file_token_id", "file_token_secret", "file_token_expiry",
		"thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary", "synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient", "delivery_attempts", "expires_at", "created_at", "updated_at",
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

	trueVal := true

	// 1. Mock first call to GetExpiredMessagesWithFiles (returns 4 messages)
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE .* expires_at < now\(\) .* file_id IS NOT NULL`).
		WithArgs(int32(100), uuid.Nil).
		WillReturnRows(pgxmock.NewRows(columns).
			AddRow(msg1.ID, uuid.New(), uuid.New(), uuid.New(), "content", msg1.MessageType, msg1.FileID, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, &trueVal, true, false, false, int32(0), time.Now(), time.Now(), time.Now()).
			AddRow(msg2.ID, uuid.New(), uuid.New(), uuid.New(), "content", msg2.MessageType, msg2.FileID, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, &trueVal, true, false, false, int32(0), time.Now(), time.Now(), time.Now()).
			AddRow(msg3.ID, uuid.New(), uuid.New(), uuid.New(), "content", msg3.MessageType, msg3.FileID, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, &trueVal, true, false, false, int32(0), time.Now(), time.Now(), time.Now()).
			AddRow(msg4.ID, uuid.New(), uuid.New(), uuid.New(), "content", msg4.MessageType, msg4.FileID, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, &trueVal, true, false, false, int32(0), time.Now(), time.Now(), time.Now()))

	// Mock cleanup database expectations for GetExpiredMessagesWithFiles
	// msg1 (unsent, has file) -> calls ClearMessageFileFields
	mock.ExpectExec(`(?s)UPDATE messages.*SET.*file_id = NULL.*WHERE id = \$1`).
		WithArgs(msg1ID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// msg2 (text, has file) -> calls DeleteMessage
	mock.ExpectExec(`DELETE FROM messages WHERE id = \$1`).
		WithArgs(msg2ID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	// msg3 (unsent, no file) -> calls ClearMessageFileFields
	mock.ExpectExec(`(?s)UPDATE messages.*SET.*file_id = NULL.*WHERE id = \$1`).
		WithArgs(msg3ID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// msg4 (text, no file) -> calls DeleteMessage
	mock.ExpectExec(`DELETE FROM messages WHERE id = \$1`).
		WithArgs(msg4ID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))


	// 2. Mock first call to GetMessagesWithFilesForBlockedUsers (returns 1 message)
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
			AddRow(msgBlocked.ID, uuid.New(), uuid.New(), uuid.New(), "content", msgBlocked.MessageType, msgBlocked.FileID, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, &trueVal, true, false, false, int32(0), time.Now(), time.Now(), time.Now()))

	// Mock DB expectation for blocked message -> calls DeleteMessage
	mock.ExpectExec(`DELETE FROM messages WHERE id = \$1`).
		WithArgs(msgBlockedID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))




	// Run cleanup job
	err := svc.CleanupExpiredMessages(ctx)
	assert.NoError(t, err)

	// Verify all mock expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}
