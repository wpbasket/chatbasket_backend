package personal_chat

import (
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

// TestUnsendMessage_RejectsMixedChat verifies that if any message in the batch belongs
// to a different chat than the target chat, the entire batch is rejected upfront with 400 Bad Request.
func TestUnsendMessage_RejectsMixedChat(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	differentChatID := uuid.New()
	otherUserID := uuid.New()

	msg1 := uuid.New()
	msg2 := uuid.New()

	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)

	// GetMessagesForUnsendBatch returns msg1 in targetChatID and msg2 in differentChatID
	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{msg1, msg2}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(msg1, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry).
			AddRow(msg2, differentChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry))

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{msg1.String(), msg2.String()},
	}, callerID, true)

	require.Error(t, err)
	assert.Empty(t, out.AcknowledgedIDs)
	assert.Equal(t, uuid.Nil, out.ChatID)
	assert.Equal(t, uuid.Nil, out.RecipientID)
	assert.Empty(t, out.NewlyUnsentIDs)
	assert.Contains(t, err.Error(), "All messages in an in-chat batch must belong to the same chat")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_SmoothPartialAndExcludesAlreadyTrueFromSSE verifies that:
// 1. In a batch with 1 eligible message, 1 already unsent message, 1 non-existent message, and 1 already read message:
//   - Smooth processing does NOT fail the entire batch.
//   - Only the eligible message is updated in the DB to unsent.
//   - Response returns confirmed IDs (eligible newly unsent + already unsent).
//   - Non-existent and ineligible (read) messages are omitted from response and DB write.
//   - SSE newlyUnsentIDs contains ONLY the eligible newly unsent message ID (strictly excluding already-unsent, non-existent, and read messages).

// TestUnsendMessage_SmoothPartialAndExcludesAlreadyTrueFromSSE verifies that:
// 1. In a batch with 1 eligible message, 1 already unsent message, 1 non-existent message, and 1 already read message:
//   - Smooth processing does NOT fail the entire batch.
//   - Only the eligible message is updated in the DB to unsent.
//   - Response returns confirmed IDs (eligible newly unsent + already unsent).
//   - Non-existent and ineligible (read) messages are omitted from response and DB write.
//   - SSE newlyUnsentIDs contains ONLY the eligible newly unsent message ID (strictly excluding already-unsent, non-existent, and read messages).
func TestUnsendMessage_SmoothPartialAndExcludesAlreadyTrueFromSSE(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUserID := uuid.New()
	targetChatID := uuid.New()

	eligibleMsgID := uuid.New()
	alreadyUnsentMsgID := uuid.New()
	nonExistentMsgID := uuid.New()
	alreadyReadMsgID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)

	// 1. Upfront batch lookup: GetMessagesForUnsendBatch runs FIRST
	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{eligibleMsgID, alreadyUnsentMsgID, nonExistentMsgID, alreadyReadMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(eligibleMsgID, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry).
			AddRow(alreadyUnsentMsgID, targetChatID, callerID.UuidUserId, otherUserID, "unsent", nil, false, futureExpiry).
			AddRow(alreadyReadMsgID, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, true, futureExpiry))

	// 2. Chat lookup and participant check runs on targetChatID derived from DB
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// Lock chat row
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// UpdateMessageToUnsent for eligible message ONLY
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+content = 'Message unsent'`).
		WithArgs(eligibleMsgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// Sync action for recipient
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), otherUserID, "unsend", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), otherUserID, "unsend", []byte("{}"), false, now, now))

	// Preview update
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_last_message_content = 'Message unsent'.*WHERE\s+id = \$1\s+AND last_message_id = \$2`).
		WithArgs(targetChatID, &eligibleMsgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// Decrement unread
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_unread_count.*WHERE id = \$3`).
		WithArgs(otherUserID, int32(1), targetChatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{
			eligibleMsgID.String(),
			alreadyUnsentMsgID.String(),
			nonExistentMsgID.String(),
			alreadyReadMsgID.String(),
		},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, targetChatID, out.ChatID)
	assert.Equal(t, otherUserID, out.RecipientID)

	// Response returns acknowledgedCount: 2 and contains both eligible newly unsent and already unsent
	assert.ElementsMatch(t, []string{eligibleMsgID.String(), alreadyUnsentMsgID.String()}, out.AcknowledgedIDs)

	// SSE newlyUnsentIDs contains ONLY the eligible newly unsent message ID!
	// (strictly excluding already-unsent, non-existent, and already-read messages)
	assert.Equal(t, []string{eligibleMsgID.String()}, out.NewlyUnsentIDs)

	// Only the missing row is permanently rejected. The read message is the
	// caller's own message that fails the unsend rule, so it is only skipped.
	assert.Equal(t, []string{nonExistentMsgID.String()}, out.PermanentlyRejectedIDs)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_ConnectHandlerIntegration verifies that the ConnectRPC handler returns
// UnsendMessageResponse with AcknowledgedCount and AcknowledgedMessageIds instead of StatusOkay.

// TestUnsendMessage_SyncActions_SecondaryCaller verifies that:
// 1. If caller is secondary (isPrimary = false), a sync action is created for the caller AND for the recipient.
// 2. For the recipient, a sync action is ALWAYS created.
func TestUnsendMessage_SyncActions_SecondaryCaller(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUserID := uuid.New()
	targetChatID := uuid.New()
	eligibleMsgID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)

	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{eligibleMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(eligibleMsgID, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry))

	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+content = 'Message unsent'`).
		WithArgs(eligibleMsgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 1. Sync action for secondary CALLER (since isPrimary = false)
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), callerID.UuidUserId, "unsend", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), callerID.UuidUserId, "unsend", []byte("{}"), false, now, now))

	// 2. Sync action for RECIPIENT (always created)
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), otherUserID, "unsend", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), otherUserID, "unsend", []byte("{}"), false, now, now))

	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_last_message_content = 'Message unsent'`).
		WithArgs(targetChatID, &eligibleMsgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_unread_count.*WHERE id = \$3`).
		WithArgs(otherUserID, int32(1), targetChatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// isPrimary = false
	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{eligibleMsgID.String()},
	}, callerID, false)

	require.NoError(t, err)
	assert.Equal(t, targetChatID, out.ChatID)
	assert.Equal(t, otherUserID, out.RecipientID)
	assert.Equal(t, []string{eligibleMsgID.String()}, out.AcknowledgedIDs)
	assert.Equal(t, []string{eligibleMsgID.String()}, out.NewlyUnsentIDs)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_ChatComesFromMessageRows verifies that UnsendMessage takes
// targetChatID strictly from the actual DB messages: the request names no chat.
func TestUnsendMessage_ChatComesFromMessageRows(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUserID := uuid.New()
	actualChatID := uuid.New()
	eligibleMsgID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)

	// 1. Upfront Batch Lookup happens FIRST (deriving targetChatID from DB message row)
	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{eligibleMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(eligibleMsgID, actualChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry))

	// 2. Chat lookup and participant check use actualChatID taken from the DB row
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1`).
		WithArgs(actualChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(actualChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// Chat lock for no key update using actualChatID
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(actualChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(actualChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+content = 'Message unsent'`).
		WithArgs(eligibleMsgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// Sync action for recipient
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), otherUserID, "unsend", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), otherUserID, "unsend", []byte("{}"), false, now, now))

	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_last_message_content = 'Message unsent'`).
		WithArgs(actualChatID, &eligibleMsgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_unread_count.*WHERE id = \$3`).
		WithArgs(otherUserID, int32(1), actualChatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{eligibleMsgID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, actualChatID, out.ChatID)
	assert.Equal(t, otherUserID, out.RecipientID)
	assert.Equal(t, []string{eligibleMsgID.String()}, out.AcknowledgedIDs)
	assert.Equal(t, []string{eligibleMsgID.String()}, out.NewlyUnsentIDs)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_SyncActionError_FailsTransactionAndAborts verifies that if CreateSyncAction fails,
// UnsendMessageHandler aborts immediately and returns a server_error instead of swallowing the failure.
func TestUnsendMessage_SyncActionError_FailsTransactionAndAborts(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUserID := uuid.New()
	targetChatID := uuid.New()
	eligibleMsgID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)

	// 1. Message lookup
	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{eligibleMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(eligibleMsgID, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry))

	// 2. Chat lookup
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// 3. Lock chat
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// 4. UpdateMessageToUnsent succeeds
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+content = 'Message unsent'`).
		WithArgs(eligibleMsgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 5. CreateSyncAction for recipient FAILS with DB error
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), otherUserID, "unsend", pgxmock.AnyArg()).
		WillReturnError(errors.New("sync action insert failed"))

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{eligibleMsgID.String()},
	}, callerID, true)

	require.Error(t, err, "UnsendMessageHandler must return an error when CreateSyncAction fails")
	assert.Empty(t, out.AcknowledgedIDs, "outcome must be empty on failure")
	assert.Equal(t, uuid.Nil, out.ChatID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_MultiMessage_SinglePassBatch verifies that a batch with multiple eligible messages
// executes in a single pass (vectorized SQL) instead of an N+1 loop:
// - 1 batch UPDATE messages ... WHERE id = ANY(...) RETURNING id
// - 1 CreateSyncAction for sender carrying all newly unsent IDs
// - 1 CreateSyncAction for recipient carrying all newly unsent IDs
// - 1 batch UPDATE chats preview WHERE last_message_id = ANY(...)
// - 1 UPDATE chats decrement with Amount = len(newlyUnsentIDs)
func TestUnsendMessage_MultiMessage_SinglePassBatch(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUserID := uuid.New()
	targetChatID := uuid.New()

	m1 := uuid.New()
	m2 := uuid.New()
	m3 := uuid.New()

	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)

	// 1. Upfront Batch Lookup
	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{m1, m2, m3}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(m1, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry).
			AddRow(m2, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry).
			AddRow(m3, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry))

	// 2. Chat lookup
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(3), int32(0), nil, nil, nil, nil, nil, nil, &m3, nil, nil, nil, nil, now, now))

	// 3. Lock chat
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(3), int32(0), nil, nil, nil, nil, nil, nil, &m3, nil, nil, nil, nil, now, now))

	// 4. Single-pass batch UPDATE messages ... WHERE id = ANY(...) RETURNING id
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET\s+content = 'Message unsent'.*WHERE\s+id = ANY\(\$1::uuid\[\]\).*RETURNING id`).
		WithArgs([]uuid.UUID{m1, m2, m3}).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).
			AddRow(m1).
			AddRow(m2).
			AddRow(m3))

	// 5. Single Sync Action for sender (since isPrimary = false) carrying ALL 3 IDs
	senderPayloadArg := &capturedArgument{}
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), callerID.UuidUserId, "unsend", senderPayloadArg).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), callerID.UuidUserId, "unsend", []byte("{}"), false, now, now))

	// 6. Single Sync Action for recipient carrying ALL 3 IDs
	recipientPayloadArg := &capturedArgument{}
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), otherUserID, "unsend", recipientPayloadArg).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), otherUserID, "unsend", []byte("{}"), false, now, now))

	// 7. Single-pass batch chat preview update
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_last_message_content = 'Message unsent'.*WHERE\s+id = \$1\s+AND last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(targetChatID, []uuid.UUID{m1, m2, m3}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 8. Single-pass chat unread decrement with Amount = 3
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_unread_count.*WHERE id = \$3`).
		WithArgs(otherUserID, int32(3), targetChatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// Execute with isPrimary = false
	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{m1.String(), m2.String(), m3.String()},
	}, callerID, false)

	require.NoError(t, err)
	assert.Equal(t, targetChatID, out.ChatID)
	assert.Equal(t, otherUserID, out.RecipientID)
	assert.ElementsMatch(t, []string{m1.String(), m2.String(), m3.String()}, out.AcknowledgedIDs)
	assert.ElementsMatch(t, []string{m1.String(), m2.String(), m3.String()}, out.NewlyUnsentIDs)

	// Both sync-action payloads carry the full batch and the target chat.
	senderPayload := senderPayloadArg.payload(t)
	assert.ElementsMatch(t, []string{m1.String(), m2.String(), m3.String()}, senderPayload.MessageIDs)
	assert.Equal(t, targetChatID.String(), senderPayload.ChatID)

	recipientPayload := recipientPayloadArg.payload(t)
	assert.ElementsMatch(t, []string{m1.String(), m2.String(), m3.String()}, recipientPayload.MessageIDs)
	assert.Equal(t, targetChatID.String(), recipientPayload.ChatID)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_AcknowledgedIDsFollowRequestOrder pins review point 5:
// UPDATE ... RETURNING rows arrive in database order, but the response, SSE
// event and sync-action payload must follow payload.MessageIDs. The request
// mixes an already-unsent row, two newly unsent rows, a missing row and a
// duplicate, while the database returns rows in a different order.
func TestUnsendMessage_AcknowledgedIDsFollowRequestOrder(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()
	chatID := uuid.New()

	alreadyID := uuid.New() // already unsent: acknowledged, not newly unsent
	newID1 := uuid.New()    // newly unsent
	missingID := uuid.New() // not in DB: permanently rejected
	newID2 := uuid.New()    // newly unsent

	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)

	// Upfront lookup returns rows in database order: newID2, newID1, alreadyID.
	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{alreadyID, newID1, missingID, newID2}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(newID2, chatID, callerID.UuidUserId, recipientID, "text", nil, false, futureExpiry).
			AddRow(newID1, chatID, callerID.UuidUserId, recipientID, "text", nil, false, futureExpiry).
			AddRow(alreadyID, chatID, callerID.UuidUserId, recipientID, "unsent", nil, false, futureExpiry))

	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1`).
		WithArgs(chatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(chatID, callerID.UuidUserId, recipientID, int32(2), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(chatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(chatID, callerID.UuidUserId, recipientID, int32(2), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// The batch UPDATE keeps the lookup order: newID2, newID1.
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET\s+content = 'Message unsent'.*WHERE\s+id = ANY\(\$1::uuid\[\]\).*RETURNING id`).
		WithArgs([]uuid.UUID{newID2, newID1}).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).
			AddRow(newID2).
			AddRow(newID1))

	// Recipient sync action (primary caller): the payload must be request-ordered.
	recipientPayloadArg := &capturedArgument{}
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), recipientID, "unsend", recipientPayloadArg).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), recipientID, "unsend", []byte("{}"), false, now, now))

	// Preview uses the raw RETURNING order; only the response and payload order matter here.
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_last_message_content = 'Message unsent'.*WHERE\s+id = \$1\s+AND last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(chatID, []uuid.UUID{newID2, newID1}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_unread_count.*WHERE id = \$3`).
		WithArgs(recipientID, int32(2), chatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		// Request: already-unsent, newly unsent, missing, newly unsent, then a
		// duplicate of newID1.
		MessageIDs: []string{alreadyID.String(), newID1.String(), missingID.String(), newID2.String(), newID1.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, []string{alreadyID.String(), newID1.String(), newID2.String()}, out.AcknowledgedIDs)
	assert.Equal(t, []string{newID1.String(), newID2.String()}, out.NewlyUnsentIDs)
	assert.Equal(t, []string{missingID.String()}, out.PermanentlyRejectedIDs)

	payload := recipientPayloadArg.payload(t)
	assert.Equal(t, []string{newID1.String(), newID2.String()}, payload.MessageIDs)
	assert.Equal(t, chatID.String(), payload.ChatID)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// of review point 6: when every requested message is already unsent but still
// has a file attached, eligibleRows is empty while retryFileRows is not, so
// execution reaches the update branch with len(eligibleRows) == 0. The
// `else if len(eligibleRows) > 1` guard is therefore not equivalent to a plain
// `else`: a plain else would run UpdateMessagesToUnsentBatch with an empty
// slice on every retry-only call.
func TestUnsendMessage_RetryOnlyPathSkipsMessageUpdate(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()
	chatID := uuid.New()
	msgID := uuid.New()
	fileID := "retry-only-file"

	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)

	// The only message is already unsent but still carries a file, so it goes
	// to retryFileRows and eligibleRows stays empty.
	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(msgID, chatID, callerID.UuidUserId, recipientID, "unsent", &fileID, false, futureExpiry))

	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1`).
		WithArgs(chatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(chatID, callerID.UuidUserId, recipientID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(chatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(chatID, callerID.UuidUserId, recipientID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// Only the retry file cleanup runs. Neither UpdateMessageToUnsent nor
	// UpdateMessagesToUnsentBatch is expected.
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+file_id = NULL`).
		WithArgs([]uuid.UUID{msgID}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{msgID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, []string{msgID.String()}, out.AcknowledgedIDs)
	assert.Empty(t, out.NewlyUnsentIDs)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_PartialBatchRaceRecheckIsOneBatchQuery pins the single-pass
// behaviour when some eligible rows lose the race between the upfront lookup and
// the batch UPDATE (they became read or expired). The re-check must classify every
// missing ID with ONE GetMessagesForUnsendBatch call, not one GetMessageByID per ID.
func TestUnsendMessage_PartialBatchRaceRecheckIsOneBatchQuery(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUserID := uuid.New()
	targetChatID := uuid.New()

	m1 := uuid.New() // updated by the batch
	m2 := uuid.New() // lost the race, now already unsent
	m3 := uuid.New() // lost the race, now read by the recipient

	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)

	// 1. Upfront lookup: all three look eligible.
	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{m1, m2, m3}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(m1, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry).
			AddRow(m2, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry).
			AddRow(m3, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry))

	// 2. Chat lookup and lock.
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(3), int32(0), nil, nil, nil, nil, nil, nil, &m3, nil, nil, nil, nil, now, now))

	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(3), int32(0), nil, nil, nil, nil, nil, nil, &m3, nil, nil, nil, nil, now, now))

	// 3. Batch UPDATE touches only m1: m2/m3 lost the race.
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET\s+content = 'Message unsent'.*WHERE\s+id = ANY\(\$1::uuid\[\]\).*RETURNING id`).
		WithArgs([]uuid.UUID{m1, m2, m3}).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(m1))

	// 4. ONE batch re-check for both missing IDs. No per-ID GetMessageByID is expected.
	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{m2, m3}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(m2, targetChatID, callerID.UuidUserId, otherUserID, "unsent", nil, false, futureExpiry).
			AddRow(m3, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, true, futureExpiry))

	// 5. One recipient sync action for the only newly unsent message (primary caller).
	recipientPayloadArg := &capturedArgument{}
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), otherUserID, "unsend", recipientPayloadArg).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), otherUserID, "unsend", []byte("{}"), false, now, now))

	// 6. Preview + decrement for the one newly unsent message.
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_last_message_content = 'Message unsent'.*WHERE\s+id = \$1\s+AND last_message_id = \$2`).
		WithArgs(targetChatID, &m1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_unread_count.*WHERE id = \$3`).
		WithArgs(otherUserID, int32(1), targetChatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{m1.String(), m2.String(), m3.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, targetChatID, out.ChatID)
	assert.Equal(t, otherUserID, out.RecipientID)
	assert.Equal(t, []string{m1.String()}, out.NewlyUnsentIDs)
	assert.ElementsMatch(t, []string{m1.String(), m2.String()}, out.AcknowledgedIDs)
	assert.Empty(t, out.PermanentlyRejectedIDs)

	// The payload must contain only the newly unsent row, never the raced ones.
	payload := recipientPayloadArg.payload(t)
	assert.Equal(t, []string{m1.String()}, payload.MessageIDs)
	assert.Equal(t, targetChatID.String(), payload.ChatID)

	assert.NoError(t, mock.ExpectationsWereMet())
}
