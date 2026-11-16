package personal_chat

import (
	"chatbasket-api/internal/platform/kit"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcknowledgeAndReadBatch_OmitsNonExistentMessages verifies that non-existent message IDs
// are strictly omitted from AckAndReadBatchResponse (AcknowledgedCount and ReadMessages)
// and that targetChatID returns Nil without executing any DB transaction.
func TestAcknowledgeAndReadBatch_OmitsNonExistentMessages(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	nonExistentID1 := uuid.New()
	nonExistentID2 := uuid.New()

	// DB returns empty rows for non-existent message IDs
	emptyRows := pgxmock.NewRows(getMessagesForAckAndReadBatchColumns())
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{nonExistentID1, nonExistentID2}).
		WillReturnRows(emptyRows)

	out, err := svc.AcknowledgeAndReadBatch(context.Background(), &AckAndReadBatchPayload{
		MessageIDs: []string{nonExistentID1.String(), nonExistentID2.String()},
	}, userID, true)

	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, out.ChatID)
	assert.Equal(t, uuid.Nil, out.OtherUserID)
	assert.True(t, out.ReadAt.IsZero())
	assert.Empty(t, out.Receipts, "non-existent messages must not be counted")
	assert.Empty(t, out.NewlyRead, "non-existent messages must not be in newlyRead")
	assert.Equal(t, []string{nonExistentID1.String(), nonExistentID2.String()}, out.PermanentlyRejectedIDs,
		"non-existent messages must be reported as permanently rejected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeAndReadBatch_OmitsAlreadyTrueFromSSE verifies that when an already-read message
// is acknowledged, it is included in the response (res.ReadMessages) with original read timestamp
// for idempotent client confirmation, but STRICTLY EXCLUDED from newlyRead (which powers the SSE broadcast).

// TestAcknowledgeAndReadBatch_OmitsAlreadyTrueFromSSE verifies that when an already-read message
// is acknowledged, it is included in the response (res.ReadMessages) with original read timestamp
// for idempotent client confirmation, but STRICTLY EXCLUDED from newlyRead (which powers the SSE broadcast).
func TestAcknowledgeAndReadBatch_OmitsAlreadyTrueFromSSE(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	alreadyReadMsgID := uuid.New()
	chatID := uuid.New()
	originalReadAt := time.Now().Add(-5 * time.Minute).Truncate(time.Microsecond)

	// DB returns 1 message already marked as read
	futureExpiry := time.Now().Add(24 * time.Hour)
	existingRows := pgxmock.NewRows(getMessagesForAckAndReadBatchColumns()).
		AddRow(alreadyReadMsgID, chatID, uuid.New(), userID.UuidUserId, nil, true, true, true, true, false, &originalReadAt, originalReadAt, futureExpiry)

	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{alreadyReadMsgID}).
		WillReturnRows(existingRows)

	out, err := svc.AcknowledgeAndReadBatch(context.Background(), &AckAndReadBatchPayload{
		MessageIDs: []string{alreadyReadMsgID.String()},
	}, userID, true)

	require.NoError(t, err)
	assert.Equal(t, chatID, out.ChatID)
	assert.Equal(t, uuid.Nil, out.OtherUserID, "no transaction ran, so the peer is not resolved in this unit test")
	assert.False(t, out.ReadAt.IsZero(), "the read timestamp falls back to the call time")

	// Response MUST include already-read message with original read_at for client idempotency
	require.Len(t, out.Receipts, 1)
	assert.Equal(t, alreadyReadMsgID.String(), out.Receipts[0].MessageId)
	assert.Equal(t, originalReadAt.Unix(), out.Receipts[0].ReadAt.Seconds)

	// SSE payload (newlyRead) MUST strictly omit already-read message
	assert.Empty(t, out.NewlyRead, "already-read messages must be excluded from SSE broadcast")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMarkChatRead_RejectsMixedChat verifies Rule 1:
// When MarkChatRead is called with message IDs belonging to different chats,
// the entire request is rejected with 400 Bad Request (invalid_batch).
func TestMarkChatRead_RejectsMixedChat(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	differentChatID := uuid.New()
	msg1 := uuid.New()
	msg2 := uuid.New()

	// GetMessagesForMarkChatRead returns one message from target chat and one message from different chat
	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)
	mock.ExpectQuery(getMessagesForMarkChatReadPattern).
		WithArgs([]uuid.UUID{msg1, msg2}).
		WillReturnRows(pgxmock.NewRows(getMessagesForMarkChatReadColumns()).
			AddRow(msg1, targetChatID, uuid.New(), callerID.UuidUserId, false, nil, false, now, futureExpiry).
			AddRow(msg2, differentChatID, uuid.New(), callerID.UuidUserId, false, nil, false, now, futureExpiry))

	out, err := svc.MarkChatRead(context.Background(), &MarkChatReadPayload{
		MessageIDs: []string{msg1.String(), msg2.String()},
	}, callerID, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "All messages in an in-chat batch must belong to the same chat")
	assert.Empty(t, out.Receipts)
	assert.Empty(t, out.NewlyRead)
	assert.Equal(t, uuid.Nil, out.ChatID)
	assert.Equal(t, uuid.Nil, out.OtherUserID)
	assert.True(t, out.ReadAt.IsZero())
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMarkChatRead_RejectsWhenCallerNotParticipant verifies Rule 2:
// If the caller is not a participant in the target chat, request is rejected with 403 Forbidden.

// TestMarkChatRead_RejectsWhenCallerNotParticipant verifies Rule 2:
// If the caller is not a participant in the target chat, request is rejected with 403 Forbidden.
func TestMarkChatRead_RejectsWhenCallerNotParticipant(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	p1 := uuid.New()
	p2 := uuid.New()
	msgID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)

	// 1. GetMessagesForMarkChatRead returns message in targetChatID
	futureExpiry := now.Add(24 * time.Hour)
	mock.ExpectQuery(getMessagesForMarkChatReadPattern).
		WithArgs([]uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForMarkChatReadColumns()).
			AddRow(msgID, targetChatID, p1, callerID.UuidUserId, false, nil, false, now, futureExpiry))

	// 2. Caller is neither p1 nor p2 in the chat derived from DB
	mock.ExpectQuery(`(?s)SELECT .* FROM chats WHERE id = \$1 LIMIT 1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, p1, p2, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	out, err := svc.MarkChatRead(context.Background(), &MarkChatReadPayload{
		MessageIDs: []string{msgID.String()},
	}, callerID, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "You are not a participant in this chat")
	assert.Empty(t, out.Receipts)
	assert.Empty(t, out.NewlyRead)
	assert.Equal(t, uuid.Nil, out.ChatID)
	assert.Equal(t, uuid.Nil, out.OtherUserID)
	assert.True(t, out.ReadAt.IsZero())
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMarkChatRead_NoDBMessages_ReturnsEmptySmoothlyWithoutFallback verifies that when no messages exist in DB,
// MarkChatRead returns empty receipts without querying the chats table: no row names a chat.
func TestMarkChatRead_NoDBMessages_ReturnsEmptySmoothlyWithoutFallback(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	nonExistentMsgID := uuid.New()

	// 1. GetMessagesForMarkChatRead returns 0 rows (non-existent)
	mock.ExpectQuery(getMessagesForMarkChatReadPattern).
		WithArgs([]uuid.UUID{nonExistentMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForMarkChatReadColumns()))

	// ZERO queries to chats table! No fallback!
	out, err := svc.MarkChatRead(context.Background(), &MarkChatReadPayload{
		MessageIDs: []string{nonExistentMsgID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Empty(t, out.Receipts)
	assert.Empty(t, out.NewlyRead)
	assert.Equal(t, uuid.Nil, out.ChatID)
	assert.Equal(t, uuid.Nil, out.OtherUserID)
	assert.True(t, out.ReadAt.IsZero())
	assert.Equal(t, []string{nonExistentMsgID.String()}, out.PermanentlyRejectedIDs, "non-existent message must be reported as permanently rejected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMarkChatRead_OmitsNonExistentAndAlreadyTrueFromSSE verifies Rules 3 and 4:
//  1. Non-existent message IDs are strictly omitted from response and SSE.
//  2. Already-read messages are included in endpoint response (with original DB timestamp) for client idempotency,
//     but strictly omitted from newlyRead (which triggers SSE broadcast).

// TestMarkChatRead_OmitsNonExistentAndAlreadyTrueFromSSE verifies Rules 3 and 4:
//  1. Non-existent message IDs are strictly omitted from response and SSE.
//  2. Already-read messages are included in endpoint response (with original DB timestamp) for client idempotency,
//     but strictly omitted from newlyRead (which triggers SSE broadcast).
func TestMarkChatRead_OmitsNonExistentAndAlreadyTrueFromSSE(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	otherUserID := uuid.New()
	alreadyReadMsgID := uuid.New()
	nonExistentMsgID := uuid.New()
	originalReadAt := time.Now().Add(-10 * time.Minute).Truncate(time.Microsecond)
	now := time.Now().Truncate(time.Microsecond)

	// 1. GetMessagesForMarkChatRead: returns 1 row for alreadyReadMsgID, 0 rows for nonExistentMsgID
	futureExpiry := now.Add(24 * time.Hour)
	mock.ExpectQuery(getMessagesForMarkChatReadPattern).
		WithArgs([]uuid.UUID{alreadyReadMsgID, nonExistentMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForMarkChatReadColumns()).
			AddRow(alreadyReadMsgID, targetChatID, otherUserID, callerID.UuidUserId, true, &originalReadAt, false, now, futureExpiry))

	// 2. Chat exists and caller is participant
	mock.ExpectQuery(`(?s)SELECT .* FROM chats WHERE id = \$1 LIMIT 1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	out, err := svc.MarkChatRead(context.Background(), &MarkChatReadPayload{
		MessageIDs: []string{alreadyReadMsgID.String(), nonExistentMsgID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.False(t, out.ReadAt.IsZero())
	assert.Equal(t, targetChatID, out.ChatID)
	assert.Equal(t, otherUserID, out.OtherUserID, "peer must be resolved from the chat row, not the caller")

	// Rule 3 & 4: Response contains alreadyReadMsgID with original timestamp; nonExistentMsgID is omitted
	require.Len(t, out.Receipts, 1, "only existing message should be in response")
	assert.Equal(t, alreadyReadMsgID.String(), out.Receipts[0].MessageId)
	assert.Equal(t, originalReadAt.Unix(), out.Receipts[0].ReadAt.Seconds)

	// Rule 3: SSE payload (newlyRead) is empty! Already-read and non-existent messages must be excluded from SSE broadcast
	assert.Empty(t, out.NewlyRead, "already-read and non-existent messages must be excluded from SSE broadcast")
	assert.Equal(t, []string{nonExistentMsgID.String()}, out.PermanentlyRejectedIDs, "only the non-existent message is permanently rejected")

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMarkChatRead_IneligibleMessageWhereCallerIsSender verifies Rule 2:
// When a caller passes a message ID where the caller is the SENDER (not the recipient),
// that message is completely ineligible:
// 1. It is excluded from DB write (never added to validIDs for MarkMessagesAsRead).
// 2. It is excluded from the response (readReceipts).
// 3. It is excluded from SSE (newlyRead).

// TestMarkChatRead_IneligibleMessageWhereCallerIsSender verifies Rule 2:
// When a caller passes a message ID where the caller is the SENDER (not the recipient),
// that message is completely ineligible:
// 1. It is excluded from DB write (never added to validIDs for MarkMessagesAsRead).
// 2. It is excluded from the response (readReceipts).
// 3. It is excluded from SSE (newlyRead).
func TestMarkChatRead_IneligibleMessageWhereCallerIsSender(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	otherUserID := uuid.New()
	sentByCallerMsgID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)

	// 1. GetMessagesForMarkChatRead: returns 1 row where sender_id = caller, recipient_id = otherUser
	futureExpiry := now.Add(24 * time.Hour)
	mock.ExpectQuery(getMessagesForMarkChatReadPattern).
		WithArgs([]uuid.UUID{sentByCallerMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForMarkChatReadColumns()).
			AddRow(sentByCallerMsgID, targetChatID, callerID.UuidUserId, otherUserID, false, nil, false, now, futureExpiry))

	// 2. Chat exists and caller is participant
	mock.ExpectQuery(`(?s)SELECT .* FROM chats WHERE id = \$1 LIMIT 1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	out, err := svc.MarkChatRead(context.Background(), &MarkChatReadPayload{
		MessageIDs: []string{sentByCallerMsgID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.False(t, out.ReadAt.IsZero())
	assert.Equal(t, targetChatID, out.ChatID)

	// Ineligible message (caller is sender) MUST be strictly excluded from response
	assert.Empty(t, out.Receipts, "messages where caller is sender must not be included in response")

	// Ineligible message (caller is sender) MUST be strictly excluded from SSE
	assert.Empty(t, out.NewlyRead, "messages where caller is sender must not be included in SSE")
	assert.Empty(t, out.PermanentlyRejectedIDs, "the caller's own sent message is skipped, not permanently rejected")

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeAndReadBatch_IneligibleMessageWhereCallerIsSender verifies that messages
// where caller is the sender are strictly omitted from DB write, response, and SSE.

// TestAcknowledgeAndReadBatch_IneligibleMessageWhereCallerIsSender verifies that messages
// where caller is the sender are strictly omitted from DB write, response, and SSE.
func TestAcknowledgeAndReadBatch_IneligibleMessageWhereCallerIsSender(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	otherUserID := uuid.New()
	sentByCallerMsgID := uuid.New()

	// GetMessagesForAckAndReadBatch: returns 1 row where sender_id = caller, recipient_id = otherUser
	futureExpiry := time.Now().Add(24 * time.Hour)
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{sentByCallerMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForAckAndReadBatchColumns()).
			AddRow(sentByCallerMsgID, targetChatID, callerID.UuidUserId, otherUserID, nil, true, true, true, false, false, nil, time.Now(), futureExpiry))

	out, err := svc.AcknowledgeAndReadBatch(context.Background(), &AckAndReadBatchPayload{
		MessageIDs: []string{sentByCallerMsgID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, out.ChatID)
	assert.Equal(t, uuid.Nil, out.OtherUserID)
	assert.True(t, out.ReadAt.IsZero())
	assert.Empty(t, out.Receipts)
	assert.Empty(t, out.NewlyRead)
	assert.Empty(t, out.PermanentlyRejectedIDs, "the caller's own sent message is skipped, not permanently rejected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeDelivery_SingleEndpointFollowsBatchRules verifies that the single ACK endpoint
// (/api/personal/chat/ack and ConnectRPC AcknowledgeDelivery) follows all rules of the batch ACK:
// 1. Newly delivered -> Acknowledged: true, produces 1 delivery group (SSE emitted).
// 2. Already delivered -> Acknowledged: true, produces 0 delivery groups (SSE strictly excluded).
// 3. Non-existent -> Acknowledged: false, produces 0 delivery groups (SSE strictly excluded).
// 4. Ineligible (caller is not recipient) -> Acknowledged: false, produces 0 delivery groups.

// TestMarkChatRead_ChatComesFromMessageRows verifies that MarkChatRead takes
// targetChatID strictly from the actual DB messages: the request names no chat.
func TestMarkChatRead_ChatComesFromMessageRows(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUserID := uuid.New()
	actualChatID := uuid.New()
	msgID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)

	// 1. Upfront message lookup happens FIRST
	futureExpiry := now.Add(24 * time.Hour)
	mock.ExpectQuery(getMessagesForMarkChatReadPattern).
		WithArgs([]uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForMarkChatReadColumns()).
			AddRow(msgID, actualChatID, otherUserID, callerID.UuidUserId, true, &now, false, now, futureExpiry))

	// 2. Chat lookup and participant check use actualChatID taken from the DB row
	mock.ExpectQuery(`(?s)SELECT .* FROM chats WHERE id = \$1 LIMIT 1`).
		WithArgs(actualChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(actualChatID, callerID.UuidUserId, otherUserID, int32(1), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	out, err := svc.MarkChatRead(context.Background(), &MarkChatReadPayload{
		MessageIDs: []string{msgID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.False(t, out.ReadAt.IsZero())
	assert.Equal(t, actualChatID, out.ChatID)
	assert.Len(t, out.Receipts, 1)
	assert.Empty(t, out.NewlyRead)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_SyncActionError_FailsTransactionAndAborts verifies that if CreateSyncAction fails,
// UnsendMessageHandler aborts immediately and returns a server_error instead of swallowing the failure.

// TestAcknowledgeAndReadBatch_RejectsCrossChat verifies:
// When AcknowledgeAndReadBatch is called with message IDs belonging to different chats,
// the entire request is strictly rejected with 400 Bad Request (invalid_batch),
// guaranteeing that batches must belong to exactly one chat.
func TestAcknowledgeAndReadBatch_RejectsCrossChat(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	differentChatID := uuid.New()
	msg1 := uuid.New()
	msg2 := uuid.New()

	now := time.Now().UTC()
	futureExpiry := now.Add(24 * time.Hour)

	cols := []string{
		"id", "chat_id", "sender_id", "recipient_id",
		"file_id", "delivered_to_recipient", "delivered_to_recipient_primary",
		"synced_to_sender_primary",
		"read_by_recipient", "read_acked_by_sender", "read_at",
		"created_at", "expires_at",
	}

	// Upfront query returns 2 messages from different chats
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{msg1, msg2}).
		WillReturnRows(pgxmock.NewRows(cols).
			AddRow(msg1, targetChatID, uuid.New(), callerID.UuidUserId, nil, true, true, true, false, false, nil, now, futureExpiry).
			AddRow(msg2, differentChatID, uuid.New(), callerID.UuidUserId, nil, true, true, true, false, false, nil, now, futureExpiry))

	out, err := svc.AcknowledgeAndReadBatch(context.Background(), &AckAndReadBatchPayload{
		MessageIDs: []string{msg1.String(), msg2.String()},
	}, callerID, true)

	require.Error(t, err, "cross-chat batch must be completely rejected")
	assert.Contains(t, err.Error(), "All messages in an in-chat batch must belong to the same chat")
	assert.Empty(t, out.Receipts)
	assert.Empty(t, out.NewlyRead)
	assert.Equal(t, uuid.Nil, out.ChatID)
	assert.Equal(t, uuid.Nil, out.OtherUserID)
	assert.True(t, out.ReadAt.IsZero())
	assert.NoError(t, mock.ExpectationsWereMet())
}
