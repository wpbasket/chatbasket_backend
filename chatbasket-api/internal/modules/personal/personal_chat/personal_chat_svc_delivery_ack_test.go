package personal_chat

import (
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcknowledgeDeliveryBatch_MultiSenderGrouping verifies that when a batch of messages
// spanning multiple chats and distinct senders is acknowledged by a recipient:
// 1. Exactly 1 set-based batch UPDATE query is executed for all message IDs.
// 2. Chat last_delivered_at is updated once per distinct chat.
// 3. DeliveryGroups partitions messages strictly by (SenderID, ChatID).
// 4. Sender A's group contains ONLY Sender A's message IDs for Chat 1.
// 5. Sender B's group contains ONLY Sender B's message IDs for Chat 2 (neither omitted nor mixed).
func TestAcknowledgeDeliveryBatch_MultiSenderGrouping(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)
	mock.MatchExpectationsInOrder(false)

	recipientID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	senderA := uuid.New()
	senderB := uuid.New()
	chatID1 := uuid.New()
	chatID2 := uuid.New()
	msgID1 := uuid.New()
	msgID2 := uuid.New()
	now := time.Now().Truncate(time.Microsecond)

	// 1. Upfront GET query for all message IDs
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(msgID1, chatID1, senderA, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now, now.Add(24*time.Hour)).
			AddRow(msgID2, chatID2, senderB, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now.Add(time.Second), now.Add(24*time.Hour)))

	// 2. Exactly ONE batch update query for all eligible message IDs
	batchRows := pgxmock.NewRows(batchAckColumns()).
		AddRow(msgID1, chatID1, senderA, nil, true, false, false, false, now).
		AddRow(msgID2, chatID2, senderB, nil, true, false, false, false, now.Add(time.Second))

	mock.ExpectQuery(`(?s)UPDATE messages\s+SET.*delivered_to_recipient_primary = TRUE.*WHERE.*`).
		WithArgs(recipientID.UuidUserId, pgxmock.AnyArg()).
		WillReturnRows(batchRows)

	// 3. Chat last_delivered_at updates (one per chat)
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+id = \$3`).
		WithArgs(recipientID.UuidUserId, now, chatID1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+id = \$3`).
		WithArgs(recipientID.UuidUserId, now.Add(time.Second), chatID2).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	res, err := svc.AcknowledgeDeliveryBatch(context.Background(), []string{msgID1.String(), msgID2.String()}, "recipient", true, recipientID)
	require.NoError(t, err)
	assert.Equal(t, int32(2), res.AcknowledgedCount)
	assert.ElementsMatch(t, []string{msgID1.String(), msgID2.String()}, res.AcknowledgedMessageIDs)

	// Verify DeliveryGroups partitioning
	require.Len(t, res.DeliveryGroups, 2, "must produce exactly 2 distinct delivery groups for 2 senders")

	groupA := res.DeliveryGroups[0]
	assert.Equal(t, senderA, groupA.SenderID)
	assert.Equal(t, chatID1, groupA.ChatID)
	assert.Equal(t, []string{msgID1.String()}, groupA.MessageIDs, "senderA group must contain only msgID1")
	assert.Equal(t, now, groupA.LatestCreatedAt)

	groupB := res.DeliveryGroups[1]
	assert.Equal(t, senderB, groupB.SenderID)
	assert.Equal(t, chatID2, groupB.ChatID)
	assert.Equal(t, []string{msgID2.String()}, groupB.MessageIDs, "senderB group must contain only msgID2 (was omitted in buggy code)")
	assert.Equal(t, now.Add(time.Second), groupB.LatestCreatedAt)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeDeliveryBatch_DeliveryGroupsFollowRequestOrder pins the grouped
// SSE payload order: the UPDATE ... RETURNING rows arrive in database order, but
// every group and the IDs inside it must follow the caller's request order. The
// already-acked row must stay out of the groups, and membership/timestamps must
// not change.
func TestAcknowledgeDeliveryBatch_DeliveryGroupsFollowRequestOrder(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)
	mock.MatchExpectationsInOrder(false)

	recipientID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	senderA := uuid.New()
	senderB := uuid.New()
	chatID1 := uuid.New()
	chatID2 := uuid.New()
	msgA1 := uuid.New()
	msgA2 := uuid.New()
	msgB1 := uuid.New()
	msgB2 := uuid.New()
	msgAAlready := uuid.New()
	now := time.Now().Truncate(time.Microsecond)

	// Lookup returns database order, unrelated to the request order.
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs([]uuid.UUID{msgA2, msgB1, msgAAlready, msgA1, msgB2}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(msgB2, chatID2, senderB, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now.Add(3*time.Second), now.Add(24*time.Hour)).
			AddRow(msgA1, chatID1, senderA, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now.Add(time.Second), now.Add(24*time.Hour)).
			AddRow(msgA2, chatID1, senderA, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now.Add(2*time.Second), now.Add(24*time.Hour)).
			AddRow(msgB1, chatID2, senderB, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now.Add(time.Second), now.Add(24*time.Hour)).
			AddRow(msgAAlready, chatID1, senderA, recipientID.UuidUserId, nil, true, true, false, false, nil, false, now, now.Add(24*time.Hour)))

	// The UPDATE returns a third order: B2, A1, B1, A2.
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET.*delivered_to_recipient_primary = TRUE.*WHERE.*`).
		WithArgs(recipientID.UuidUserId, []uuid.UUID{msgA2, msgB1, msgA1, msgB2}).
		WillReturnRows(pgxmock.NewRows(batchAckColumns()).
			AddRow(msgB2, chatID2, senderB, nil, true, false, false, false, now.Add(3*time.Second)).
			AddRow(msgA1, chatID1, senderA, nil, true, false, false, false, now.Add(time.Second)).
			AddRow(msgB1, chatID2, senderB, nil, true, false, false, false, now.Add(time.Second)).
			AddRow(msgA2, chatID1, senderA, nil, true, false, false, false, now.Add(2*time.Second)))

	// One last_delivered_at update per chat, with the max created_at in that chat.
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+id = \$3`).
		WithArgs(recipientID.UuidUserId, now.Add(2*time.Second), chatID1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+id = \$3`).
		WithArgs(recipientID.UuidUserId, now.Add(3*time.Second), chatID2).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	res, err := svc.AcknowledgeDeliveryBatch(context.Background(), []string{
		msgA2.String(),
		msgB1.String(),
		msgAAlready.String(),
		msgA1.String(),
		msgB2.String(),
	}, "recipient", true, recipientID)
	require.NoError(t, err)

	assert.Equal(t, int32(5), res.AcknowledgedCount)
	assert.Equal(t, []string{
		msgA2.String(),
		msgB1.String(),
		msgAAlready.String(),
		msgA1.String(),
		msgB2.String(),
	}, res.AcknowledgedMessageIDs)

	require.Len(t, res.DeliveryGroups, 2)
	groupA, groupB := res.DeliveryGroups[0], res.DeliveryGroups[1]
	assert.Equal(t, senderA, groupA.SenderID)
	assert.Equal(t, chatID1, groupA.ChatID)
	assert.Equal(t, []string{msgA2.String(), msgA1.String()}, groupA.MessageIDs,
		"group A must follow request order and exclude the already-acked row")
	assert.Equal(t, now.Add(2*time.Second), groupA.LatestCreatedAt)

	assert.Equal(t, senderB, groupB.SenderID)
	assert.Equal(t, chatID2, groupB.ChatID)
	assert.Equal(t, []string{msgB1.String(), msgB2.String()}, groupB.MessageIDs,
		"group B must follow request order")
	assert.Equal(t, now.Add(3*time.Second), groupB.LatestCreatedAt)
	assert.Empty(t, res.PermanentlyRejectedIDs)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeDeliveryBatch_ConnectHandlerIntegration verifies the ConnectRPC handler
// delegates cleanly to the batch service and returns the confirmed messages.

// TestAcknowledgeDeliveryBatch_IdempotentRetry confirms that when a client retries a batch
// where some messages were already delivered, the endpoint confirms all messages belonging
// to the recipient without emitting duplicate SSE delivery groups.
func TestAcknowledgeDeliveryBatch_IdempotentRetry(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	recipientID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	senderA := uuid.New()
	chatID1 := uuid.New()
	msgID1 := uuid.New() // newly delivered
	msgID2 := uuid.New() // already delivered previously
	now := time.Now().Truncate(time.Microsecond)

	// 1. Upfront GET query inspects all requested messages
	msgID3 := uuid.New() // deleted from relay
	existingRows := pgxmock.NewRows(deliveryAckColumns()).
		AddRow(msgID1, chatID1, senderA, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now, now.Add(24*time.Hour)).
		AddRow(msgID2, chatID1, senderA, recipientID.UuidUserId, nil, true, true, false, false, nil, false, now, now.Add(24*time.Hour))
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(existingRows)

	// 2. Batch UPDATE updates only eligible msgID1 (msgID2 was already delivered, msgID3 does not exist)
	batchRows := pgxmock.NewRows(batchAckColumns()).
		AddRow(msgID1, chatID1, senderA, nil, true, false, false, false, now)

	mock.ExpectQuery(`(?s)UPDATE messages\s+SET.*delivered_to_recipient_primary = TRUE.*WHERE.*`).
		WithArgs(recipientID.UuidUserId, []uuid.UUID{msgID1}).
		WillReturnRows(batchRows)

	// 3. Chat last_delivered_at updated for msgID1
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+id = \$3`).
		WithArgs(recipientID.UuidUserId, now, chatID1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	res, err := svc.AcknowledgeDeliveryBatch(context.Background(), []string{msgID1.String(), msgID2.String(), msgID3.String()}, "recipient", true, recipientID)
	require.NoError(t, err)
	// Strictly only real existing messages confirmed: msgID1 (fresh) and msgID2 (already-true).
	// msgID3 (deleted from relay) is strictly excluded.
	assert.Equal(t, int32(2), res.AcknowledgedCount)
	assert.ElementsMatch(t, []string{msgID1.String(), msgID2.String()}, res.AcknowledgedMessageIDs)
	assert.NotContains(t, res.AcknowledgedMessageIDs, msgID3.String())
	assert.Equal(t, []string{msgID3.String()}, res.PermanentlyRejectedIDs, "the row deleted from relay must be reported as permanently rejected")

	// DeliveryGroups must ONLY contain newly delivered msgID1, NOT msgID2 or msgID3
	require.Len(t, res.DeliveryGroups, 1)
	assert.Equal(t, []string{msgID1.String()}, res.DeliveryGroups[0].MessageIDs)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeDeliveryBatch_SenderPrimarySync confirms that a primary sender device can
// batch-acknowledge its sent messages, while a secondary session is rejected with 403 Forbidden.

// TestAcknowledgeDeliveryBatch_SenderPrimarySync confirms that a primary sender device can
// batch-acknowledge its sent messages, while a secondary session is rejected with 403 Forbidden.
func TestAcknowledgeDeliveryBatch_SenderPrimarySync(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	msgID1 := uuid.New()
	msgID2 := uuid.New()
	now := time.Now()

	// Secondary session must be rejected immediately without DB queries
	_, secErr := svc.AcknowledgeDeliveryBatch(context.Background(), []string{msgID1.String()}, "sender", false, senderID)
	require.Error(t, secErr)
	pe, ok := secErr.(kit.ProcessedError)
	require.True(t, ok)
	assert.Equal(t, 403, pe.Status())
	assert.Contains(t, pe.Error(), "Only primary device can ACK sender sync")

	// Primary session executes upfront query then batch update
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs([]uuid.UUID{msgID1, msgID2}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(msgID1, uuid.New(), senderID.UuidUserId, uuid.New(), nil, false, false, false, false, nil, false, now, now.Add(24*time.Hour)).
			AddRow(msgID2, uuid.New(), senderID.UuidUserId, uuid.New(), nil, false, false, false, false, nil, false, now, now.Add(24*time.Hour)))

	batchRows := pgxmock.NewRows(batchAckColumns()).
		AddRow(msgID1, uuid.New(), senderID.UuidUserId, nil, false, true, false, false, now).
		AddRow(msgID2, uuid.New(), senderID.UuidUserId, nil, false, true, false, false, now)

	mock.ExpectQuery(`(?s)UPDATE messages\s+SET synced_to_sender_primary = TRUE.*WHERE.*`).
		WithArgs(senderID.UuidUserId, []uuid.UUID{msgID1, msgID2}).
		WillReturnRows(batchRows)

	res, primErr := svc.AcknowledgeDeliveryBatch(context.Background(), []string{msgID1.String(), msgID2.String()}, "sender", true, senderID)
	require.NoError(t, primErr)
	assert.Equal(t, int32(2), res.AcknowledgedCount)
	assert.ElementsMatch(t, []string{msgID1.String(), msgID2.String()}, res.AcknowledgedMessageIDs)
	assert.Empty(t, res.DeliveryGroups, "sender sync ACK should not generate any delivery SSE groups")

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeAndReadBatch_OmitsNonExistentMessages verifies that non-existent message IDs
// are strictly omitted from AckAndReadBatchResponse (AcknowledgedCount and ReadMessages)
// and that targetChatID returns Nil without executing any DB transaction.

// TestAcknowledgeDelivery_SingleEndpointFollowsBatchRules verifies that the single ACK endpoint
// (/api/personal/chat/ack and ConnectRPC AcknowledgeDelivery) follows all rules of the batch ACK:
// 1. Newly delivered -> Acknowledged: true, produces 1 delivery group (SSE emitted).
// 2. Already delivered -> Acknowledged: true, produces 0 delivery groups (SSE strictly excluded).
// 3. Non-existent -> Acknowledged: false, produces 0 delivery groups (SSE strictly excluded).
// 4. Ineligible (caller is not recipient) -> Acknowledged: false, produces 0 delivery groups.
func TestAcknowledgeDelivery_SingleEndpointFollowsBatchRules(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)
	mock.MatchExpectationsInOrder(false)

	recipientID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	senderID := uuid.New()
	chatID := uuid.New()
	newMsgID := uuid.New()
	alreadyDeliveredMsgID := uuid.New()
	nonExistentMsgID := uuid.New()
	ineligibleMsgID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)

	// 1. Case: Newly delivered
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs([]uuid.UUID{newMsgID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(newMsgID, chatID, senderID, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now, now.Add(24*time.Hour)))

	mock.ExpectQuery(`(?s)UPDATE messages\s+SET.*delivered_to_recipient_primary = TRUE.*WHERE.*`).
		WithArgs(recipientID.UuidUserId, []uuid.UUID{newMsgID}).
		WillReturnRows(pgxmock.NewRows(batchAckColumns()).
			AddRow(newMsgID, chatID, senderID, nil, true, false, false, false, now))

	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+id = \$3`).
		WithArgs(recipientID.UuidUserId, now, chatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// Case 1 leaves a detached cleanup goroutine running. It talks to the same
	// mock, so expect it here and wait for it below before registering case 2's
	// expectations — pgxmock's expectation slice is not safe for that overlap.
	mock.ExpectExec(`(?s)DELETE FROM messages\s+WHERE\s+chat_id = \$1.*read_acked_by_sender = TRUE`).
		WithArgs(chatID).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	resNew, groupsNew, errNew := svc.AcknowledgeDeliveryHandler(context.Background(), &AcknowledgeDeliveryPayload{
		MessageID:      newMsgID.String(),
		AcknowledgedBy: "recipient",
		Success:        true,
	}, recipientID, true)

	require.NoError(t, errNew)
	assert.True(t, resNew.Acknowledged)
	require.Len(t, groupsNew, 1, "newly delivered message must produce 1 delivery group for SSE")
	assert.Equal(t, senderID, groupsNew[0].SenderID)
	assert.Equal(t, []string{newMsgID.String()}, groupsNew[0].MessageIDs)

	require.Eventually(t, func() bool {
		return mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond, "detached cleanup must finish before the next case reuses the mock")

	// 2. Case: Already delivered (idempotent, excluded from SSE)
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs([]uuid.UUID{alreadyDeliveredMsgID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(alreadyDeliveredMsgID, chatID, senderID, recipientID.UuidUserId, nil, true, true, false, false, nil, false, now, now.Add(24*time.Hour)))

	resAlready, groupsAlready, errAlready := svc.AcknowledgeDeliveryHandler(context.Background(), &AcknowledgeDeliveryPayload{
		MessageID:      alreadyDeliveredMsgID.String(),
		AcknowledgedBy: "recipient",
		Success:        true,
	}, recipientID, true)

	require.NoError(t, errAlready)
	assert.True(t, resAlready.Acknowledged, "already delivered message must be confirmed for client idempotency")
	assert.Empty(t, groupsAlready, "already delivered message must NOT produce any delivery group (SSE strictly excluded)")

	// 3. Case: Non-existent message
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs([]uuid.UUID{nonExistentMsgID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()))

	resNonExistent, groupsNonExistent, errNonExistent := svc.AcknowledgeDeliveryHandler(context.Background(), &AcknowledgeDeliveryPayload{
		MessageID:      nonExistentMsgID.String(),
		AcknowledgedBy: "recipient",
		Success:        true,
	}, recipientID, true)

	require.NoError(t, errNonExistent)
	assert.False(t, resNonExistent.Acknowledged, "non-existent message must return Acknowledged: false")
	assert.Empty(t, groupsNonExistent, "non-existent message must not produce SSE groups")

	// 4. Case: Ineligible message (caller is not recipient)
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs([]uuid.UUID{ineligibleMsgID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(ineligibleMsgID, chatID, recipientID.UuidUserId, uuid.New(), nil, false, false, false, false, nil, false, now, now.Add(24*time.Hour))) // caller is sender, not recipient

	resIneligible, groupsIneligible, errIneligible := svc.AcknowledgeDeliveryHandler(context.Background(), &AcknowledgeDeliveryPayload{
		MessageID:      ineligibleMsgID.String(),
		AcknowledgedBy: "recipient",
		Success:        true,
	}, recipientID, true)

	require.NoError(t, errIneligible)
	assert.False(t, resIneligible.Acknowledged, "ineligible message must return Acknowledged: false")
	assert.Empty(t, groupsIneligible, "ineligible message must not produce SSE groups")

	// 5. Case: Expired message (TTL exceeded, completely rejected)
	expiredMsgID := uuid.New()
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs([]uuid.UUID{expiredMsgID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(expiredMsgID, chatID, senderID, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now.Add(-3*time.Hour), now.Add(-1*time.Hour)))

	resExpired, groupsExpired, errExpired := svc.AcknowledgeDeliveryHandler(context.Background(), &AcknowledgeDeliveryPayload{
		MessageID:      expiredMsgID.String(),
		AcknowledgedBy: "recipient",
		Success:        true,
	}, recipientID, true)

	require.NoError(t, errExpired)
	assert.False(t, resExpired.Acknowledged, "expired message must return Acknowledged: false")
	assert.Empty(t, groupsExpired, "expired message must not produce SSE groups")

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_RejectsMixedChat verifies that if any message in the batch belongs
// to a different chat than the target chat, the entire batch is rejected upfront with 400 Bad Request.

func TestValidateAcknowledgedBy(t *testing.T) {
	assert.NoError(t, validateAcknowledgedBy("recipient"))
	assert.NoError(t, validateAcknowledgedBy("sender"))

	for _, bad := range []string{"", "nobody", "Recipient", "SENDER", "recipient "} {
		err := validateAcknowledgedBy(bad)
		require.Error(t, err, "value %q must be rejected", bad)
		pe, ok := err.(kit.ProcessedError)
		require.True(t, ok)
		assert.Equal(t, 400, pe.Status())
		assert.Equal(t, "invalid_request", pe.Kind())
	}
}

func TestAcknowledgeDeliveryHandler_RejectsInvalidAcknowledgedBy(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	store := personal_chat_store.New(mock)
	svc := &chatService{PostgresQuerier: store, PostgresQueries: store}
	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}

	for _, success := range []bool{true, false} {
		_, _, ackErr := svc.AcknowledgeDeliveryHandler(context.Background(), &AcknowledgeDeliveryPayload{
			MessageID:      uuid.New().String(),
			AcknowledgedBy: "nobody",
			Success:        success,
		}, userID, true)

		require.Error(t, ackErr)
		pe, ok := ackErr.(kit.ProcessedError)
		require.True(t, ok)
		assert.Equal(t, 400, pe.Status(), "success=%v", success)
	}

	// Validation must run before any DB access.
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeDeliveryHandler_ValidValueStillIdempotent confirms that when a message was already
// acknowledged previously, retrying returns Acknowledged: true without emitting duplicate SSE.

// TestAcknowledgeDeliveryHandler_ValidValueStillIdempotent confirms that when a message was already
// acknowledged previously, retrying returns Acknowledged: true without emitting duplicate SSE.
func TestAcknowledgeDeliveryHandler_ValidValueStillIdempotent(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	store := personal_chat_store.New(mock)
	svc := &chatService{PostgresQuerier: store, PostgresQueries: store}
	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	messageID := uuid.New()
	chatID := uuid.New()

	// Upfront query verifies message exists and is already synced to sender primary
	mock.ExpectQuery(`(?s)SELECT.*FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{messageID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(messageID, chatID, userID.UuidUserId, uuid.New(), nil, false, false, true, false, nil, false, time.Now(), time.Now().Add(24*time.Hour)))

	res, groups, ackErr := svc.AcknowledgeDeliveryHandler(context.Background(), &AcknowledgeDeliveryPayload{
		MessageID:      messageID.String(),
		AcknowledgedBy: "sender",
		Success:        true,
	}, userID, true)

	require.NoError(t, ackErr)
	assert.True(t, res.Acknowledged)
	assert.Empty(t, groups, "idempotent retry must not emit SSE")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeDeliveryHandler_OmitsNonExistentMessage confirms Rule 3:
// Non-existent messages must return Acknowledged: false and emit zero SSE.
