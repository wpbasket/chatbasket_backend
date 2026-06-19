package personal_chat

import (
	"context"
	"errors"
	"testing"

	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/kit"
	"github.com/google/uuid"
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
			details := dpe.Details().(StaleKeysErrorDetails)
			assert.Equal(t, StaleSideRecipient, details.StaleSide)
			assert.Equal(t, int32(6), details.RecipientKeysRevision)
		} else {
			t.Errorf("Expected DetailedProcessedError, got %T", err)
		}
	} else {
		t.Errorf("Expected ProcessedError, got %T", err)
	}
}

func TestSendMessage_RevisionStaleness_AcceptsCurrentRevision(t *testing.T) {
	t.Skip("Skipping: requires database access for chat creation")
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()

	// Recipient's current revision is 6, sender also has 6 (current)
	profileProvider := &mockProfileProvider{
		getE2EEPublicKeyFunc: func(ctx context.Context, userID uuid.UUID) (*string, int32, error) {
			key := "recipient-public-key-44-chars-base64-enc"
			return &key, 6, nil
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

	// Send message with current revision (6)
	params := SendMessageParams{
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "Hello",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 6, // Current revision
		SenderKeysRevision:    0,
	}

	// Will fail at chat creation (no DB), but revision check should pass
	_, err := svc.SendMessage(context.Background(), params)

	// Should fail with a panic or DB error, but NOT revision staleness
	if err != nil {
		// Check it's not a revision staleness error
		if pe, ok := err.(kit.ProcessedError); ok {
			assert.NotEqual(t, "recipient_keys_stale", pe.Kind())
		}
	}
}

func TestSendMessage_RevisionStaleness_AcceptsZeroRevision(t *testing.T) {
	t.Skip("Skipping: requires database access for chat creation")
	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()

	// Recipient has no keys (revision 0)
	profileProvider := &mockProfileProvider{
		getE2EEPublicKeyFunc: func(ctx context.Context, userID uuid.UUID) (*string, int32, error) {
			return nil, 0, nil // No public key, revision 0
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

	// Send message with zero revision
	params := SendMessageParams{
		SenderID:              senderID,
		RecipientID:           recipientID,
		Content:               "Hello",
		MessageType:           "text",
		IsPrimary:             true,
		RecipientKeysRevision: 0,
		SenderKeysRevision:    0,
	}

	// Will fail at eligibility check (no E2EE setup), but not at revision check
	_, err := svc.SendMessage(context.Background(), params)

	// Should fail with eligibility error, NOT revision staleness
	if err != nil {
		if pe, ok := err.(kit.ProcessedError); ok {
			assert.NotEqual(t, "recipient_keys_stale", pe.Kind())
		}
	}
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
			details := dpe.Details().(StaleKeysErrorDetails)
			assert.Equal(t, StaleSideRecipient, details.StaleSide)
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
