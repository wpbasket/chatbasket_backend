package personal_chat

import (
	"context"
	"testing"
	"time"

	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/kit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendMessage_Idempotency_MessageAlreadyExists(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()
	chatID := uuid.New()
	messageID := uuid.New()
	now := time.Now()

	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()
	store := personal_chat_store.New(mockPool)

	// 1. Mock CheckMessageExists query returning TRUE (already exists)
	mockPool.ExpectQuery(`SELECT EXISTS.*messages`).WithArgs(messageID).WillReturnRows(
		pgxmock.NewRows([]string{"exists"}).AddRow(true),
	)

	// 2. Mock GetMessageByID query returning the existing message
	msgCols := []string{"id", "chat_id", "sender_id", "recipient_id",
		"content", "message_type",
		"file_id", "file_name", "file_size", "file_mime_type",
		"file_token_id", "file_token_secret", "file_token_expiry",
		"thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary",
		"synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient",
		"delivery_attempts", "expires_at", "created_at", "updated_at",
		"read_by_recipient", "read_acked_by_sender", "read_at"}

	mockPool.ExpectQuery(`SELECT (.+) FROM messages WHERE id =`).WithArgs(messageID).WillReturnRows(
		pgxmock.NewRows(msgCols).AddRow(
			messageID, chatID, senderID.UuidUserId, recipientID,
			"test-content", "text",
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, nil,
			false, false,
			true,
			false, false,
			int32(0), now.Add(DefaultMessageTTL), now, now, false, false, nil,
		),
	)

	svc := &chatService{
		AuthProvider:    &mockAuthProvider{},
		ProfileProvider: &mockProfileProvider{},
		ContactProvider: &mockContactProvider{},
		PostgresQuerier: store,
		PostgresQueries: store,
	}

	// Send message
	msg, sendErr := svc.SendMessage(context.Background(), SendMessageParams{
		MessageID:             messageID,
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "test-content",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 5,
		SenderKeysRevision:    0,
	})

	require.NoError(t, sendErr)
	assert.Equal(t, messageID, msg.ID)
	assert.Equal(t, "test-content", msg.Content)

	require.NoError(t, mockPool.ExpectationsWereMet())
}

func TestSendMessage_Idempotency_MessageDoesNotExist(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()
	chatID := uuid.New()
	messageID := uuid.New()
	now := time.Now()

	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()
	store := personal_chat_store.New(mockPool)

	profileProvider := &mockProfileProvider{
		getE2EEPublicKeyFunc: func(ctx context.Context, userID uuid.UUID) (*string, int32, error) {
			key := "key"
			return &key, 5, nil
		},
		getUserCoreProfileFunc: func(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error) {
			return &personal_profile.UserCoreProfile{
				ID:            userID,
				IsAdminBlocked: false,
				ProfileType:    "public",
			}, nil
		},
	}

	// 1. Mock CheckMessageExists query returning FALSE (does not exist yet)
	mockPool.ExpectQuery(`SELECT EXISTS.*messages`).WithArgs(messageID).WillReturnRows(
		pgxmock.NewRows([]string{"exists"}).AddRow(false),
	)

	// 2. Mock CreateOrGetChat query (INSERT INTO chats)
	chatCols := []string{"id", "participant_1_id", "participant_2_id",
		"p1_unread_count", "p2_unread_count",
		"p1_last_read_at", "p2_last_read_at",
		"p1_last_delivered_at", "p2_last_delivered_at",
		"last_message_created_at", "last_message_sender_id", "last_message_id",
		"p1_last_message_content", "p2_last_message_content",
		"p1_last_message_type", "p2_last_message_type",
		"created_at", "updated_at"}
	mockPool.ExpectQuery(`INSERT INTO.*chats`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(
		pgxmock.NewRows(chatCols).AddRow(
			chatID, senderID.UuidUserId, recipientID,
			int32(0), int32(0),
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, nil, nil,
			now, now,
		),
	)

	// 3. Mock CreateMessage query
	msgCols := []string{"id", "chat_id", "sender_id", "recipient_id",
		"content", "message_type",
		"file_id", "file_name", "file_size", "file_mime_type",
		"file_token_id", "file_token_secret", "file_token_expiry",
		"thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary",
		"synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient",
		"delivery_attempts", "expires_at", "created_at", "updated_at",
		"read_by_recipient", "read_acked_by_sender", "read_at"}
	mockPool.ExpectQuery(`INSERT INTO.*messages`).WithArgs(messageID, chatID, senderID.UuidUserId, recipientID, "test-content", "text", pgxmock.AnyArg(), true, pgxmock.AnyArg()).WillReturnRows(
		pgxmock.NewRows(msgCols).AddRow(
			messageID, chatID, senderID.UuidUserId, recipientID,
			"test-content", "text",
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, nil,
			false, false,
			true,
			false, false,
			int32(0), now.Add(DefaultMessageTTL), now, now, false, false, nil,
		),
	)

	// 4. Mock UpdateChatStatus
	mockPool.ExpectExec(`UPDATE.*chats`).WithArgs(chatID, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	svc := &chatService{
		AuthProvider:    &mockAuthProvider{},
		ProfileProvider: profileProvider,
		ContactProvider: &mockContactProvider{},
		PostgresQuerier: store,
		PostgresQueries: store,
	}

	// Send message
	msg, sendErr := svc.SendMessage(context.Background(), SendMessageParams{
		MessageID:             messageID,
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "test-content",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 5,
		SenderKeysRevision:    0,
	})

	require.NoError(t, sendErr)
	assert.Equal(t, messageID, msg.ID)

	require.NoError(t, mockPool.ExpectationsWereMet())
}

func TestSendMessage_Idempotency_TOCTOURaceConflict(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()
	chatID := uuid.New()
	messageID := uuid.New()
	now := time.Now()

	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()
	store := personal_chat_store.New(mockPool)

	profileProvider := &mockProfileProvider{
		getE2EEPublicKeyFunc: func(ctx context.Context, userID uuid.UUID) (*string, int32, error) {
			key := "key"
			return &key, 5, nil
		},
		getUserCoreProfileFunc: func(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error) {
			return &personal_profile.UserCoreProfile{
				ID:            userID,
				IsAdminBlocked: false,
				ProfileType:    "public",
			}, nil
		},
	}

	// 1. Mock CheckMessageExists returning FALSE (TOCTOU: both think it doesn't exist)
	mockPool.ExpectQuery(`SELECT EXISTS.*messages`).WithArgs(messageID).WillReturnRows(
		pgxmock.NewRows([]string{"exists"}).AddRow(false),
	)

	// 2. Mock CreateOrGetChat query (INSERT INTO chats)
	chatCols := []string{"id", "participant_1_id", "participant_2_id",
		"p1_unread_count", "p2_unread_count",
		"p1_last_read_at", "p2_last_read_at",
		"p1_last_delivered_at", "p2_last_delivered_at",
		"last_message_created_at", "last_message_sender_id", "last_message_id",
		"p1_last_message_content", "p2_last_message_content",
		"p1_last_message_type", "p2_last_message_type",
		"created_at", "updated_at"}
	mockPool.ExpectQuery(`INSERT INTO.*chats`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(
		pgxmock.NewRows(chatCols).AddRow(
			chatID, senderID.UuidUserId, recipientID,
			int32(0), int32(0),
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, nil, nil,
			now, now,
		),
	)

	// 3. Mock CreateMessage returning unique_violation error (Code: 23505)
	mockPool.ExpectQuery(`INSERT INTO.*messages`).WithArgs(messageID, chatID, senderID.UuidUserId, recipientID, "test-content", "text", pgxmock.AnyArg(), true, pgxmock.AnyArg()).WillReturnError(
		&pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"},
	)

	// 4. Mock GetMessageByID queries returning the existing message
	msgCols := []string{"id", "chat_id", "sender_id", "recipient_id",
		"content", "message_type",
		"file_id", "file_name", "file_size", "file_mime_type",
		"file_token_id", "file_token_secret", "file_token_expiry",
		"thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary",
		"synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient",
		"delivery_attempts", "expires_at", "created_at", "updated_at",
		"read_by_recipient", "read_acked_by_sender", "read_at"}

	mockPool.ExpectQuery(`SELECT (.+) FROM messages WHERE id =`).WithArgs(messageID).WillReturnRows(
		pgxmock.NewRows(msgCols).AddRow(
			messageID, chatID, senderID.UuidUserId, recipientID,
			"test-content", "text",
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, nil,
			false, false,
			true,
			false, false,
			int32(0), now.Add(DefaultMessageTTL), now, now, false, false, nil,
		),
	)

	svc := &chatService{
		AuthProvider:    &mockAuthProvider{},
		ProfileProvider: profileProvider,
		ContactProvider: &mockContactProvider{},
		PostgresQuerier: store,
		PostgresQueries: store,
	}

	// Send message
	msg, sendErr := svc.SendMessage(context.Background(), SendMessageParams{
		MessageID:             messageID,
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "test-content",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 5,
		SenderKeysRevision:    0,
	})

	require.NoError(t, sendErr)
	assert.Equal(t, messageID, msg.ID)
	assert.Equal(t, "test-content", msg.Content)

	require.NoError(t, mockPool.ExpectationsWereMet())
}
