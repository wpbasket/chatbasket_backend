package personal_chat

import (
	rpc_common_modelv1 "chatbasket-api/gen/proto/common/model"
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/kit"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProfileProvider implements personalProfilePersonalChatProvider interface
type mockProfileProvider struct {
	getE2EEPublicKeyFunc                func(ctx context.Context, targetUserID uuid.UUID) (*string, int32, error)
	getContactableProfilesForViewerFunc func(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error)
	getUserCoreProfileFunc              func(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error)
	isUserAdminBlockedFunc              func(ctx context.Context, userID uuid.UUID) (bool, error)
}

func (m *mockProfileProvider) GetE2EEPublicKey(ctx context.Context, targetUserID uuid.UUID) (*string, int32, error) {
	if m.getE2EEPublicKeyFunc != nil {
		return m.getE2EEPublicKeyFunc(ctx, targetUserID)
	}
	key := "test-key"
	return &key, 5, nil
}

func (m *mockProfileProvider) GetContactableProfilesForViewer(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error) {
	if m.getContactableProfilesForViewerFunc != nil {
		return m.getContactableProfilesForViewerFunc(ctx, viewerID, targetIDs)
	}
	return make(map[uuid.UUID]*personal_profile.ContactProfileView), nil
}

func (m *mockProfileProvider) GetUserCoreProfile(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error) {
	if m.getUserCoreProfileFunc != nil {
		return m.getUserCoreProfileFunc(ctx, userID)
	}
	return &personal_profile.UserCoreProfile{
		ID:            userID,
		IsAdminBlocked: false,
		ProfileType:    "public",
	}, nil
}

func (m *mockProfileProvider) IsUserAdminBlocked(ctx context.Context, userID uuid.UUID) (bool, error) {
	if m.isUserAdminBlockedFunc != nil {
		return m.isUserAdminBlockedFunc(ctx, userID)
	}
	return false, nil
}

func (m *mockProfileProvider) GetActiveSessionKeysForUser(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return []string{}, nil
}

func (m *mockProfileProvider) GetContactableUserIDs(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) ([]uuid.UUID, error) {
	return targetIDs, nil
}

// mockContactProvider implements personalContactPersonalChatProvider interface
type mockContactProvider struct {
	isAlreadyContactFunc       func(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (bool, error)
	getMessagingBlockStatusFunc func(ctx context.Context, user1ID uuid.UUID, user2ID uuid.UUID) (int32, error)
}

// mockAuthProvider implements coreAuthChatProvider interface
type mockAuthProvider struct {
	isSessionCentralFunc       func(ctx context.Context, userID uuid.UUID, sessionToken string) (bool, error)
	getUserPrimarySessionIDFunc func(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}

func (m *mockAuthProvider) IsSessionCentral(ctx context.Context, userID uuid.UUID, sessionToken string) (bool, error) {
	if m.isSessionCentralFunc != nil {
		return m.isSessionCentralFunc(ctx, userID, sessionToken)
	}
	return true, nil
}

func (m *mockAuthProvider) GetUserPrimarySessionID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	if m.getUserPrimarySessionIDFunc != nil {
		return m.getUserPrimarySessionIDFunc(ctx, userID)
	}
	return uuid.New(), nil
}

func (m *mockAuthProvider) GetSessionE2EEPublicKey(ctx context.Context, sessionID uuid.UUID) (*string, error) {
	key := "mock_public_key"
	return &key, nil
}

func (m *mockContactProvider) IsAlreadyContact(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (bool, error) {
	if m.isAlreadyContactFunc != nil {
		return m.isAlreadyContactFunc(ctx, ownerID, contactID)
	}
	return true, nil
}

func (m *mockContactProvider) GetMessagingBlockStatus(ctx context.Context, user1ID uuid.UUID, user2ID uuid.UUID) (int32, error) {
	if m.getMessagingBlockStatusFunc != nil {
		return m.getMessagingBlockStatusFunc(ctx, user1ID, user2ID)
	}
	return 0, nil
}

// ============================================================================
// Revision Staleness Tests
// ============================================================================

func TestSendMessage_RevisionStaleness_RejectsStaleRevision(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()

	// Recipient's current revision is 6, but sender has revision 5 (stale)
	profileProvider := &mockProfileProvider{
		getE2EEPublicKeyFunc: func(ctx context.Context, userID uuid.UUID) (*string, int32, error) {
			key := "recipient-public-key-44-chars-base64-enc"
			return &key, 6, nil // Current revision is 6
		},
		getUserCoreProfileFunc: func(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error) {
			return &personal_profile.UserCoreProfile{
				ID:            userID,
				IsAdminBlocked: false,
				ProfileType:    "public",
			}, nil
		},
	}

	contactProvider := &mockContactProvider{
		isAlreadyContactFunc: func(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (bool, error) {
			return true, nil
		},
	}

	authProvider := &mockAuthProvider{}

	svc := &chatService{
		AuthProvider:    authProvider,
		ProfileProvider: profileProvider,
		ContactProvider: contactProvider,
	}

	// Send message with stale revision (5 when current is 6)
	params := SendMessageParams{
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "Hello",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 5, // Stale revision
		SenderKeysRevision:    0,
	}

	_, err := svc.SendMessage(context.Background(), params)

	// Should return staleness error
	require.Error(t, err)
	// Check if it's a StaleKeysError with the correct type
	if pe, ok := err.(kit.ProcessedError); ok {
		assert.Equal(t, "keys_stale", pe.Kind())
		// Check details
		if dpe, ok := err.(kit.DetailedProcessedError); ok {
			details := dpe.Details().(*rpc_common_modelv1.StaleKeysErrorDetails)
			assert.Equal(t, string(StaleSideRecipient), details.StaleSide)
			assert.Equal(t, int32(6), details.RecipientKeysRevision)
		} else {
			t.Errorf("Expected DetailedProcessedError, got %T", err)
		}
	} else {
		t.Errorf("Expected ProcessedError, got %T", err)
	}
}

func TestSendMessage_RevisionStaleness_AcceptsCurrentRevision(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()
	chatID := uuid.New()
	messageID := uuid.New()
	now := time.Now()

	// Recipient's current revision is 6; client also sends 6 — not stale
	profileProvider := &mockProfileProvider{
		getE2EEPublicKeyFunc: func(ctx context.Context, userID uuid.UUID) (*string, int32, error) {
			key := "recipient-public-key-44-chars-base64-enc"
			return &key, 6, nil
		},
		getUserCoreProfileFunc: func(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error) {
			return &personal_profile.UserCoreProfile{
				ID:             userID,
				IsAdminBlocked: false,
				ProfileType:    "public",
			}, nil
		},
	}
	contactProvider := &mockContactProvider{
		isAlreadyContactFunc: func(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (bool, error) {
			return true, nil
		},
	}

	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	store := personal_chat_store.New(mockPool)

	// CreateChat mock — returns a valid Chat row
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

	// CreateMessage mock — returns a valid Message row
	msgCols := []string{"id", "chat_id", "sender_id", "recipient_id",
		"content", "message_type",
		"file_id", "file_name", "file_size", "file_mime_type",
		"file_token_id", "file_token_secret", "file_token_expiry",
		"thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary",
		"synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient",
		"delivery_attempts", "expires_at", "created_at", "updated_at"}
	mockPool.ExpectQuery(`INSERT INTO.*messages`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(
		pgxmock.NewRows(msgCols).AddRow(
			messageID, chatID, senderID.UuidUserId, recipientID,
			"Hello", "text",
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, nil,
			false, nil,
			true,
			false, false,
			int32(0), now.Add(DefaultMessageTTL), now, now,
		),
	)

	// UpdateChatStatus mock — fire and forget, returning no rows
	mockPool.ExpectExec(`UPDATE.*chats`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	svc := &chatService{
		AuthProvider:    &mockAuthProvider{},
		ProfileProvider: profileProvider,
		ContactProvider: contactProvider,
		PostgresQuerier: store,
		PostgresQueries: store,
	}

	_, sendErr := svc.SendMessage(context.Background(), SendMessageParams{
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "Hello",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 6, // Matches DB revision — not stale
		SenderKeysRevision:    0,
	})

	// Revision check passes — any error must NOT be a staleness error
	if sendErr != nil {
		if pe, ok := sendErr.(kit.ProcessedError); ok {
			assert.NotEqual(t, "recipient_keys_stale", pe.Kind())
			assert.NotEqual(t, "sender_keys_stale", pe.Kind())
			assert.NotEqual(t, "keys_stale", pe.Kind())
		}
	}
	assert.NoError(t, mockPool.ExpectationsWereMet())
}

func TestSendMessage_RevisionStaleness_AcceptsZeroRevision(t *testing.T) {
	// Revision 0 means the client does not supply a revision — treated as always-fresh.
	// checkRevisionStaleness skips the stale check when both revisions == 0,
	// so message should be stored successfully.
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()
	chatID := uuid.New()
	messageID := uuid.New()
	now := time.Now()

	profileProvider := &mockProfileProvider{
		getE2EEPublicKeyFunc: func(ctx context.Context, userID uuid.UUID) (*string, int32, error) {
			key := "recipient-public-key-44-chars-base64-enc"
			return &key, 1, nil // DB has revision 1 so eligibility passes; client sends 0 to skip stale check
		},
		getUserCoreProfileFunc: func(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error) {
			return &personal_profile.UserCoreProfile{
				ID:             userID,
				IsAdminBlocked: false,
				ProfileType:    "public",
			}, nil
		},
	}
	contactProvider := &mockContactProvider{
		isAlreadyContactFunc: func(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (bool, error) {
			return true, nil
		},
	}

	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	store := personal_chat_store.New(mockPool)

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

	msgCols := []string{"id", "chat_id", "sender_id", "recipient_id",
		"content", "message_type",
		"file_id", "file_name", "file_size", "file_mime_type",
		"file_token_id", "file_token_secret", "file_token_expiry",
		"thumbnail_file_id", "thumbnail_token_id", "thumbnail_token_secret",
		"delivered_to_recipient", "delivered_to_recipient_primary",
		"synced_to_sender_primary",
		"deleted_by_sender", "deleted_by_recipient",
		"delivery_attempts", "expires_at", "created_at", "updated_at"}
	mockPool.ExpectQuery(`INSERT INTO.*messages`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(
		pgxmock.NewRows(msgCols).AddRow(
			messageID, chatID, senderID.UuidUserId, recipientID,
			"Hello", "text",
			nil, nil, nil, nil,
			nil, nil, nil,
			nil, nil, nil,
			false, nil,
			true,
			false, false,
			int32(0), now.Add(DefaultMessageTTL), now, now,
		),
	)
	mockPool.ExpectExec(`UPDATE.*chats`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	svc := &chatService{
		AuthProvider:    &mockAuthProvider{},
		ProfileProvider: profileProvider,
		ContactProvider: contactProvider,
		PostgresQuerier: store,
		PostgresQueries: store,
	}

	_, sendErr := svc.SendMessage(context.Background(), SendMessageParams{
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "Hello",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 0, // Zero revision — never stale
		SenderKeysRevision:    0,
	})

	// Zero revision is always accepted — must NOT return any staleness error
	require.NoError(t, sendErr)
	assert.NoError(t, mockPool.ExpectationsWereMet())
}

func TestSendMessage_RevisionStaleness_ErrorFetchingRevision(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()

	// Error when fetching revision
	profileProvider := &mockProfileProvider{
		getE2EEPublicKeyFunc: func(ctx context.Context, userID uuid.UUID) (*string, int32, error) {
			return nil, 0, errors.New("database connection failed")
		},
		getUserCoreProfileFunc: func(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error) {
			return &personal_profile.UserCoreProfile{
				ID:            userID,
				IsAdminBlocked: false,
				ProfileType:    "public",
			}, nil
		},
	}

	contactProvider := &mockContactProvider{
		isAlreadyContactFunc: func(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (bool, error) {
			return true, nil
		},
	}

	authProvider := &mockAuthProvider{}

	svc := &chatService{
		AuthProvider:    authProvider,
		ProfileProvider: profileProvider,
		ContactProvider: contactProvider,
	}

	// Send message
	params := SendMessageParams{
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "Hello",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 5,
		SenderKeysRevision:    0,
	}

	_, err := svc.SendMessage(context.Background(), params)

	// Should return error about eligibility (failed to fetch revision)
	require.Error(t, err)
	// The error will be from eligibility check, not revision staleness
	if pe, ok := err.(kit.ProcessedError); ok {
		assert.NotEqual(t, "recipient_keys_stale", pe.Kind())
	}
}

func TestSendMessage_RevisionStaleness_LargeRevisionJump(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()

	// Recipient's revision jumped from 5 to 100 (many key changes)
	profileProvider := &mockProfileProvider{
		getE2EEPublicKeyFunc: func(ctx context.Context, userID uuid.UUID) (*string, int32, error) {
			key := "recipient-public-key-44-chars-base64-enc"
			return &key, 100, nil // Current revision is 100
		},
		getUserCoreProfileFunc: func(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error) {
			return &personal_profile.UserCoreProfile{
				ID:            userID,
				IsAdminBlocked: false,
				ProfileType:    "public",
			}, nil
		},
	}

	contactProvider := &mockContactProvider{
		isAlreadyContactFunc: func(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (bool, error) {
			return true, nil
		},
	}

	authProvider := &mockAuthProvider{}

	svc := &chatService{
		AuthProvider:    authProvider,
		ProfileProvider: profileProvider,
		ContactProvider: contactProvider,
	}

	// Send message with old revision (5 when current is 100)
	params := SendMessageParams{
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "Hello",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 5, // Very old revision
		SenderKeysRevision:    0,
	}

	_, err := svc.SendMessage(context.Background(), params)

	// Should reject with staleness error
	require.Error(t, err)
	// Check if it's a StaleKeysError with the correct type
	if pe, ok := err.(kit.ProcessedError); ok {
		assert.Equal(t, "keys_stale", pe.Kind())
		// Check details
		if dpe, ok := err.(kit.DetailedProcessedError); ok {
			details := dpe.Details().(*rpc_common_modelv1.StaleKeysErrorDetails)
			assert.Equal(t, string(StaleSideRecipient), details.StaleSide)
			assert.Equal(t, int32(100), details.RecipientKeysRevision)
		} else {
			t.Errorf("Expected DetailedProcessedError, got %T", err)
		}
	} else {
		t.Errorf("Expected ProcessedError, got %T", err)
	}
}

func TestSendMessage_RevisionStaleness_RecipientNotEligible(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()

	// Recipient is admin blocked
	profileProvider := &mockProfileProvider{
		getUserCoreProfileFunc: func(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error) {
			return &personal_profile.UserCoreProfile{
				ID:            userID,
				IsAdminBlocked: true, // Admin blocked
				ProfileType:    "public",
			}, nil
		},
		isUserAdminBlockedFunc: func(ctx context.Context, userID uuid.UUID) (bool, error) {
			return true, nil
		},
	}

	contactProvider := &mockContactProvider{
		isAlreadyContactFunc: func(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (bool, error) {
			return true, nil
		},
	}

	authProvider := &mockAuthProvider{}

	svc := &chatService{
		AuthProvider:    authProvider,
		ProfileProvider: profileProvider,
		ContactProvider: contactProvider,
	}

	// Send message
	params := SendMessageParams{
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "Hello",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 5,
		SenderKeysRevision:    0,
	}

	_, err := svc.SendMessage(context.Background(), params)

	// Should fail at eligibility check (not at revision check)
	require.Error(t, err)
	if pe, ok := err.(kit.ProcessedError); ok {
		assert.NotEqual(t, "recipient_keys_stale", pe.Kind())
	}
}

func TestSendMessage_RevisionStaleness_NotContacts(t *testing.T) {
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()

	// Users are not contacts
	profileProvider := &mockProfileProvider{
		getUserCoreProfileFunc: func(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error) {
			return &personal_profile.UserCoreProfile{
				ID:            userID,
				IsAdminBlocked: false,
				ProfileType:    "public",
			}, nil
		},
	}

	contactProvider := &mockContactProvider{
		isAlreadyContactFunc: func(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (bool, error) {
			return false, nil // Not contacts
		},
	}

	authProvider := &mockAuthProvider{}

	svc := &chatService{
		AuthProvider:    authProvider,
		ProfileProvider: profileProvider,
		ContactProvider: contactProvider,
	}

	// Send message
	params := SendMessageParams{
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "Hello",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 5,
		SenderKeysRevision:    0,
	}

	_, err := svc.SendMessage(context.Background(), params)

	// Should fail at eligibility check (not contacts)
	require.Error(t, err)
	if pe, ok := err.(kit.ProcessedError); ok {
		assert.NotEqual(t, "recipient_keys_stale", pe.Kind())
	}
}
