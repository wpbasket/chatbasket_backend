package personal_chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/kit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func replyMsgCols() []string {
	return []string{"id", "chat_id", "sender_id", "recipient_id",
		"content", "message_type",
		"file_id", "file_name", "file_size", "file_mime_type",
		"file_token_id", "file_token_secret", "file_token_expiry",
		"thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary",
		"synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient",
		"delivery_attempts", "expires_at", "created_at", "updated_at",
		"read_by_recipient", "read_acked_by_sender", "read_at", "reply_to_message_id"}
}

func addStoredMessage(mock pgxmock.PgxPoolIface, msgID, chatID, senderID, recipientID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(`SELECT (.+) FROM messages WHERE id =`).WithArgs(msgID).WillReturnRows(
		pgxmock.NewRows(replyMsgCols()).AddRow(
			msgID, chatID, senderID, recipientID,
			"orig-content", "text",
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, nil,
			false, false, false,
			false, false,
			int32(0), now.Add(DefaultMessageTTL), now, now, false, false, nil, nil,
		),
	)
}

func newReplySvc(mock pgxmock.PgxPoolIface) *chatService {
	store := personal_chat_store.New(mock)
	return &chatService{
		AuthProvider:    &mockAuthProvider{},
		ProfileProvider: &mockProfileProvider{},
		ContactProvider: &mockContactProvider{},
		PostgresQuerier: store,
		PostgresQueries: store,
	}
}

func TestValidateReplyToMessage_NilAllowed(t *testing.T) {
	svc := &chatService{}
	err := svc.validateReplyToMessage(context.Background(), nil, uuid.New(), uuid.New(), uuid.New())
	assert.NoError(t, err)
}

func TestValidateReplyToMessage_NilUUIDAllowed(t *testing.T) {
	svc := &chatService{}
	nilID := uuid.Nil
	err := svc.validateReplyToMessage(context.Background(), &nilID, uuid.New(), uuid.New(), uuid.New())
	assert.NoError(t, err)
}

func TestValidateReplyToMessage_SelfLoopRejected(t *testing.T) {
	svc := &chatService{}
	id := uuid.New()
	err := svc.validateReplyToMessage(context.Background(), &id, id, uuid.New(), uuid.New())
	require.Error(t, err)
	if pe, ok := err.(kit.ProcessedError); ok {
		assert.Equal(t, "invalid_reply_to_message_id", pe.Kind())
	} else {
		t.Errorf("expected ProcessedError, got %T", err)
	}
}

func TestValidateReplyToMessage_PrunedAllowed(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	svc := newReplySvc(mock)

	replyID := uuid.New()
	mock.ExpectQuery(`SELECT (.+) FROM messages WHERE id =`).WithArgs(replyID).WillReturnError(pgx.ErrNoRows)

	err = svc.validateReplyToMessage(context.Background(), &replyID, uuid.New(), uuid.New(), uuid.New())
	assert.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateReplyToMessage_SamePairSameDirectionAllowed(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	svc := newReplySvc(mock)

	sender := uuid.New()
	recipient := uuid.New()
	replyID := uuid.New()
	addStoredMessage(mock, replyID, uuid.New(), sender, recipient)

	err = svc.validateReplyToMessage(context.Background(), &replyID, uuid.New(), sender, recipient)
	assert.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateReplyToMessage_SamePairReverseAllowed(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	svc := newReplySvc(mock)

	alice := uuid.New()
	you := uuid.New()
	replyID := uuid.New()
	// Alice sent the original, you reply.
	addStoredMessage(mock, replyID, uuid.New(), alice, you)

	err = svc.validateReplyToMessage(context.Background(), &replyID, uuid.New(), you, alice)
	assert.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateReplyToMessage_CrossConversationRejected(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	svc := newReplySvc(mock)

	bob := uuid.New()
	carol := uuid.New()
	replyID := uuid.New()
	addStoredMessage(mock, replyID, uuid.New(), bob, carol)

	err = svc.validateReplyToMessage(context.Background(), &replyID, uuid.New(), uuid.New(), uuid.New())
	require.Error(t, err)
	if pe, ok := err.(kit.ProcessedError); ok {
		assert.Equal(t, "invalid_reply_to_message_id", pe.Kind())
	} else {
		t.Errorf("expected ProcessedError, got %T", err)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateReplyToMessage_DBErrorGeneric(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	svc := newReplySvc(mock)

	replyID := uuid.New()
	mock.ExpectQuery(`SELECT (.+) FROM messages WHERE id =`).WithArgs(replyID).
		WillReturnError(errors.New("connection refused: table messages SELECT *"))

	err = svc.validateReplyToMessage(context.Background(), &replyID, uuid.New(), uuid.New(), uuid.New())
	require.Error(t, err)
	if pe, ok := err.(kit.ProcessedError); ok {
		assert.Equal(t, "internal_server_error", pe.Kind())
	} else {
		t.Errorf("expected ProcessedError, got %T", err)
	}
	assert.NotContains(t, err.Error(), "table messages")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSendMessage_WithValidReplyStoresID(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()
	chatID := uuid.New()
	messageID := uuid.New()
	replyID := uuid.New()
	now := time.Now()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	store := personal_chat_store.New(mock)

	profileProvider := &mockProfileProvider{
		getE2EEPublicKeyFunc: func(ctx context.Context, userID uuid.UUID) (*string, int32, error) {
			key := "key"
			return &key, 5, nil
		},
	}

	// Idempotency: new message does not exist.
	mock.ExpectQuery(`SELECT EXISTS.*messages`).WithArgs(messageID).WillReturnRows(
		pgxmock.NewRows([]string{"exists"}).AddRow(false),
	)
	// Reply target exists in the same conversation.
	addStoredMessage(mock, replyID, chatID, senderID.UuidUserId, recipientID)

	chatCols := []string{"id", "participant_1_id", "participant_2_id",
		"p1_unread_count", "p2_unread_count",
		"p1_last_read_at", "p2_last_read_at",
		"p1_last_delivered_at", "p2_last_delivered_at",
		"last_message_created_at", "last_message_sender_id", "last_message_id",
		"p1_last_message_content", "p2_last_message_content",
		"p1_last_message_type", "p2_last_message_type",
		"created_at", "updated_at"}
	mock.ExpectQuery(`INSERT INTO.*chats`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(
		pgxmock.NewRows(chatCols).AddRow(
			chatID, senderID.UuidUserId, recipientID,
			int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now,
		),
	)
	mock.ExpectQuery(`INSERT INTO.*messages`).WithArgs(messageID, chatID, senderID.UuidUserId, recipientID, "reply", "text", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), &replyID).WillReturnRows(
		pgxmock.NewRows(replyMsgCols()).AddRow(
			messageID, chatID, senderID.UuidUserId, recipientID,
			"reply", "text", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			false, false, true, false, false, int32(0), now.Add(DefaultMessageTTL), now, now, false, false, nil, &replyID,
		),
	)
	mock.ExpectExec(`UPDATE.*chats`).WithArgs(chatID, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	svc := &chatService{
		AuthProvider:    &mockAuthProvider{},
		ProfileProvider: profileProvider,
		ContactProvider: &mockContactProvider{},
		PostgresQuerier: store,
		PostgresQueries: store,
	}

	msg, sendErr := svc.SendMessage(context.Background(), SendMessageParams{
		MessageID:             messageID,
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "reply",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 5,
		SenderKeysRevision:    0,
		ReplyToMessageID:      &replyID,
	})
	require.NoError(t, sendErr)
	require.NotNil(t, msg.ReplyToMessageID)
	assert.Equal(t, replyID, *msg.ReplyToMessageID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSendMessage_WithNilUUIDReplyPersistsNull(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()
	chatID := uuid.New()
	messageID := uuid.New()
	nilID := uuid.Nil
	now := time.Now()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	store := personal_chat_store.New(mock)

	profileProvider := &mockProfileProvider{
		getE2EEPublicKeyFunc: func(ctx context.Context, userID uuid.UUID) (*string, int32, error) {
			key := "key"
			return &key, 5, nil
		},
	}

	// Idempotency: new message does not exist.
	mock.ExpectQuery(`SELECT EXISTS.*messages`).WithArgs(messageID).WillReturnRows(
		pgxmock.NewRows([]string{"exists"}).AddRow(false),
	)
	// uuid.Nil skips the reply-target lookup in the validator (early-allow).

	chatCols := []string{"id", "participant_1_id", "participant_2_id",
		"p1_unread_count", "p2_unread_count",
		"p1_last_read_at", "p2_last_read_at",
		"p1_last_delivered_at", "p2_last_delivered_at",
		"last_message_created_at", "last_message_sender_id", "last_message_id",
		"p1_last_message_content", "p2_last_message_content",
		"p1_last_message_type", "p2_last_message_type",
		"created_at", "updated_at"}
	mock.ExpectQuery(`INSERT INTO.*chats`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(
		pgxmock.NewRows(chatCols).AddRow(
			chatID, senderID.UuidUserId, recipientID,
			int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now,
		),
	)
	// The 10th INSERT arg must be NULL (typed nil), never &uuid.Nil.
	mock.ExpectQuery(`INSERT INTO.*messages`).WithArgs(messageID, chatID, senderID.UuidUserId, recipientID, "hi", "text", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), (*uuid.UUID)(nil)).WillReturnRows(
		pgxmock.NewRows(replyMsgCols()).AddRow(
			messageID, chatID, senderID.UuidUserId, recipientID,
			"hi", "text", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			false, false, true, false, false, int32(0), now.Add(DefaultMessageTTL), now, now, false, false, nil, nil,
		),
	)
	mock.ExpectExec(`UPDATE.*chats`).WithArgs(chatID, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	svc := &chatService{
		AuthProvider:    &mockAuthProvider{},
		ProfileProvider: profileProvider,
		ContactProvider: &mockContactProvider{},
		PostgresQuerier: store,
		PostgresQueries: store,
	}

	msg, sendErr := svc.SendMessage(context.Background(), SendMessageParams{
		MessageID:             messageID,
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "hi",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 5,
		ReplyToMessageID:      &nilID,
	})
	require.NoError(t, sendErr)
	assert.Nil(t, msg.ReplyToMessageID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSendMessage_WithCrossConversationReplyRejected(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()
	messageID := uuid.New()
	replyID := uuid.New()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	store := personal_chat_store.New(mock)

	profileProvider := &mockProfileProvider{
		getE2EEPublicKeyFunc: func(ctx context.Context, userID uuid.UUID) (*string, int32, error) {
			key := "key"
			return &key, 5, nil
		},
	}

	mock.ExpectQuery(`SELECT EXISTS.*messages`).WithArgs(messageID).WillReturnRows(
		pgxmock.NewRows([]string{"exists"}).AddRow(false),
	)
	// Reply target belongs to a different pair.
	addStoredMessage(mock, replyID, uuid.New(), uuid.New(), uuid.New())

	svc := &chatService{
		AuthProvider:    &mockAuthProvider{},
		ProfileProvider: profileProvider,
		ContactProvider: &mockContactProvider{},
		PostgresQuerier: store,
		PostgresQueries: store,
	}

	_, sendErr := svc.SendMessage(context.Background(), SendMessageParams{
		MessageID:             messageID,
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "reply",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 5,
		SenderKeysRevision:    0,
		ReplyToMessageID:      &replyID,
	})
	require.Error(t, sendErr)
	if pe, ok := sendErr.(kit.ProcessedError); ok {
		assert.Equal(t, "invalid_reply_to_message_id", pe.Kind())
	} else {
		t.Errorf("expected ProcessedError, got %T", sendErr)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNormalizeReplyToMessageID(t *testing.T) {
	assert.Nil(t, normalizeReplyToMessageID(nil))

	nilID := uuid.Nil
	assert.Nil(t, normalizeReplyToMessageID(&nilID))

	validID := uuid.New()
	assert.Equal(t, &validID, normalizeReplyToMessageID(&validID))
}

func TestSendMessage_IdempotentRetry_PreservesReplyID(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()
	chatID := uuid.New()
	messageID := uuid.New()
	replyID := uuid.New()
	now := time.Now()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	// Idempotency: message already exists.
	mock.ExpectQuery(`SELECT EXISTS.*messages`).WithArgs(messageID).WillReturnRows(
		pgxmock.NewRows([]string{"exists"}).AddRow(true),
	)
	// Existing row carries a non-nil reply target.
	mock.ExpectQuery(`SELECT (.+) FROM messages WHERE id =`).WithArgs(messageID).WillReturnRows(
		pgxmock.NewRows(replyMsgCols()).AddRow(
			messageID, chatID, senderID.UuidUserId, recipientID,
			"reply", "text",
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, nil,
			false, false, true,
			false, false,
			int32(0), now.Add(DefaultMessageTTL), now, now, false, false, nil, &replyID,
		),
	)

	svc := newReplySvc(mock)

	msg, sendErr := svc.SendMessage(context.Background(), SendMessageParams{
		MessageID:             messageID,
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "reply",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 5,
		SenderKeysRevision:    0,
		ReplyToMessageID:      &replyID,
	})
	require.NoError(t, sendErr)
	require.NotNil(t, msg.ReplyToMessageID)
	assert.Equal(t, replyID, *msg.ReplyToMessageID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMessagesHandler_SurfacesReplyID(t *testing.T) {
	userID := uuid.New()
	kitUserID := kit.UserId{UuidUserId: userID, StringUserId: userID.String()}
	otherID := uuid.New()
	chatID := uuid.New()
	messageID := uuid.New()
	replyID := uuid.New()
	now := time.Now()
	sessionCreatedAt := now.Add(-24 * time.Hour)

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	store := personal_chat_store.New(mock)

	chatCols := []string{"id", "participant_1_id", "participant_2_id",
		"p1_unread_count", "p2_unread_count",
		"p1_last_read_at", "p2_last_read_at",
		"p1_last_delivered_at", "p2_last_delivered_at",
		"last_message_created_at", "last_message_sender_id", "last_message_id",
		"p1_last_message_content", "p2_last_message_content",
		"p1_last_message_type", "p2_last_message_type",
		"created_at", "updated_at"}
	mock.ExpectQuery(`SELECT (.+) FROM chats WHERE id =`).WithArgs(chatID).WillReturnRows(
		pgxmock.NewRows(chatCols).AddRow(
			chatID, userID, otherID,
			int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now,
		),
	)
	mock.ExpectQuery(`SELECT (.+) FROM messages`).WithArgs(chatID, int32(50), sessionCreatedAt, userID, (*time.Time)(nil), (*uuid.UUID)(nil)).WillReturnRows(
		pgxmock.NewRows(replyMsgCols()).AddRow(
			messageID, chatID, otherID, userID,
			"cipher", "text",
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, nil,
			false, false, false,
			false, false,
			int32(0), now.Add(DefaultMessageTTL), now, now, false, false, nil, &replyID,
		),
	)

	svc := &chatService{
		AuthProvider:    &mockAuthProvider{},
		ProfileProvider: &mockProfileProvider{},
		ContactProvider: &mockContactProvider{},
		PostgresQuerier: store,
		PostgresQueries: store,
	}

	resp, err := svc.GetMessagesHandler(context.Background(), &GetMessagesPayload{
		ChatID: chatID.String(),
		Limit:  50,
	}, kitUserID, sessionCreatedAt, true)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Messages, 1)
	require.NotNil(t, resp.Messages[0].ReplyToMessageId)
	assert.Equal(t, replyID.String(), *resp.Messages[0].ReplyToMessageId)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetPendingMessagesHandler_SurfacesReplyID(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	userID := uuid.New()
	kitUserID := kit.UserId{UuidUserId: userID, StringUserId: userID.String()}
	sessionCreatedAt := time.Now().Add(-24 * time.Hour)

	sender := uuid.New()
	replyID := uuid.New()
	nowTime := time.Now()

	chatSvc := &chatService{
		Pool:            nil,
		PostgresQueries: personal_chat_store.New(mockPool),
	}
	chatSvc.ProfileProvider = &mockPersonalProfileProvider{
		IsUserAdminBlockedFunc: func(ctx context.Context, uID uuid.UUID) (bool, error) {
			return false, nil
		},
		GetContactableUserIDsFunc: func(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) ([]uuid.UUID, error) {
			return []uuid.UUID{sender}, nil
		},
	}

	msgCols := []string{
		"id", "chat_id", "sender_id", "recipient_id",
		"content", "message_type", "file_id", "file_name",
		"file_size", "file_mime_type", "file_token_id", "file_token_secret",
		"file_token_expiry", "thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary", "synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient", "delivery_attempts",
		"expires_at", "created_at", "updated_at",
		"read_by_recipient", "read_acked_by_sender", "read_at", "reply_to_message_id",
	}
	// Recipient stream: one reply-carrying message.
	mockPool.ExpectQuery("SELECT (.+) FROM messages").
		WithArgs(userID, int32(200), sessionCreatedAt, (*time.Time)(nil), (*uuid.UUID)(nil)).
		WillReturnRows(pgxmock.NewRows(msgCols).AddRow(
			uuid.New(), uuid.New(), sender, userID,
			"reply cipher", "text", nil, nil,
			nil, nil, nil, nil,
			nil, nil, nil, nil,
			false, false, false,
			false, false, int32(0),
			nowTime.Add(1*time.Hour), nowTime, nowTime, false, false, nil, &replyID,
		))

	// Sender stream: empty.
	mockPool.ExpectQuery("SELECT (.+) FROM messages").
		WithArgs(userID, int32(200), sessionCreatedAt, (*time.Time)(nil), (*uuid.UUID)(nil)).
		WillReturnRows(pgxmock.NewRows(msgCols))

	resp, err := chatSvc.GetPendingMessagesHandler(context.Background(), &GetPendingMessagesPayload{Limit: 100}, kitUserID, sessionCreatedAt, false)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Messages, 1)
	require.NotNil(t, resp.Messages[0].ReplyToMessageId)
	assert.Equal(t, replyID.String(), *resp.Messages[0].ReplyToMessageId)
	require.NoError(t, mockPool.ExpectationsWereMet())
}
