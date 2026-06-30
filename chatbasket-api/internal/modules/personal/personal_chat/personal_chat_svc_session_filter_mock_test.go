package personal_chat

import (
	"context"
	"testing"
	"time"

	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/kit"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
)

func TestGetUserChatsHandler_SessionFilter(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer mockPool.Close()

	userID := uuid.New()
	kitUserID := kit.UserId{UuidUserId: userID, StringUserId: userID.String()}
	sessionCreatedAt := time.Now().Add(-24 * time.Hour) // Session created 1 day ago

	chatSvc := &chatService{
		Pool:            nil, // Use nil for any tx, GetUserChatsLite doesn't use it directly here
		PostgresQueries: personal_chat_store.New(mockPool),
	}

	// Mock the ProfileProvider using a manual stub
	mockProfileProvider := &mockPersonalProfileProvider{
		getContactableProfilesForViewerFunc: func(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error) {
			return make(map[uuid.UUID]*personal_profile.ContactProfileView), nil
		},
	}
	chatSvc.ProfileProvider = mockProfileProvider

	// Simulate a chat where the last message was created 2 days ago (older than session)
	chatID := uuid.New()
	oldMessageTime := sessionCreatedAt.Add(-24 * time.Hour)
	oldContent := "old ciphertext"
	oldType := "text"

	// Simulate another chat where the last message was created 1 hour ago (newer than session)
	chatID2 := uuid.New()
	newMessageTime := sessionCreatedAt.Add(1 * time.Hour)
	newContent := "new ciphertext"
	newType := "text"

	rows := pgxmock.NewRows([]string{
		"id", "participant_1_id", "participant_2_id",
		"p1_unread_count", "p2_unread_count",
		"p1_last_read_at", "p2_last_read_at",
		"p1_last_delivered_at", "p2_last_delivered_at",
		"p1_last_message_content", "p2_last_message_content",
		"p1_last_message_type", "p2_last_message_type",
		"last_message_created_at", "created_at", "updated_at",
		"last_message_sender_id", "last_message_id", "other_user_id",
		"unread_count", "last_message_content_1", "last_message_type_1", // Using _1 to avoid duplicate names in mock query? Wait, the sqlc generated struct uses standard struct names mapped from query aliases.
		"last_message_status", "other_user_last_read_at", "other_user_last_delivered_at",
	}).AddRow(
		chatID, userID, uuid.New(),
		int32(0), int32(0),
		nil, nil,
		nil, nil,
		&oldContent, &oldContent,
		&oldType, &oldType,
		&oldMessageTime, time.Now(), time.Now(),
		nil, nil, uuid.New(),
		int32(0), "", "",
		"sent", time.Now(), time.Now(),
	).AddRow(
		chatID2, userID, uuid.New(),
		int32(0), int32(0),
		nil, nil,
		nil, nil,
		&newContent, &newContent,
		&newType, &newType,
		&newMessageTime, time.Now(), time.Now(),
		nil, nil, uuid.New(),
		int32(0), "", "",
		"sent", time.Now(), time.Now(),
	)

	mockPool.ExpectQuery("SELECT (.+) FROM chats").
		WithArgs(userID).
		WillReturnRows(rows)

	ctx := context.Background()
	resp, err := chatSvc.GetUserChatsHandler(ctx, kitUserID, sessionCreatedAt)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, resp.Chats, 2)

	// Check that the old message preview was filtered out
	assert.Equal(t, chatID.String(), resp.Chats[0].ChatID)
	assert.Nil(t, resp.Chats[0].LastMessageContent, "old message content should be nil")
	assert.Nil(t, resp.Chats[0].LastMessageType, "old message type should be nil")

	// Check that the new message preview was kept
	assert.Equal(t, chatID2.String(), resp.Chats[1].ChatID)
	assert.NotNil(t, resp.Chats[1].LastMessageContent, "new message content should not be nil")
	assert.Equal(t, newContent, *resp.Chats[1].LastMessageContent)
	assert.NotNil(t, resp.Chats[1].LastMessageType, "new message type should not be nil")
	assert.Equal(t, newType, *resp.Chats[1].LastMessageType)

	if err := mockPool.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

// Minimal manual mock for the ProfileProvider
type mockPersonalProfileProvider struct {
	getContactableProfilesForViewerFunc func(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error)
}

func (m *mockPersonalProfileProvider) GetContactableProfilesForViewer(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error) {
	if m.getContactableProfilesForViewerFunc != nil {
		return m.getContactableProfilesForViewerFunc(ctx, viewerID, targetIDs)
	}
	return nil, nil
}

func (m *mockPersonalProfileProvider) GetUserCoreProfile(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error) {
	return nil, nil
}
func (m *mockPersonalProfileProvider) GetE2EEPublicKey(ctx context.Context, targetUserID uuid.UUID) (*string, int32, error) {
	return nil, 0, nil
}
func (m *mockPersonalProfileProvider) GetActiveSessionKeysForUser(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockPersonalProfileProvider) IsUserAdminBlocked(ctx context.Context, userID uuid.UUID) (bool, error) {
	return false, nil
}
