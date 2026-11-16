package personal_chat

import (
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/config"
	"chatbasket-api/internal/platform/kit"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func deliveryAckExtendedColumns() []string {
	return []string{
		"id", "chat_id", "sender_id", "recipient_id",
		"file_id",
		"delivered_to_recipient", "delivered_to_recipient_primary", "synced_to_sender_primary",
		"read_by_recipient", "read_at", "read_acked_by_sender",
		"created_at", "expires_at",
	}
}

const deliveryAckExtendedPattern = `(?s)SELECT\s+id,\s+chat_id,\s+sender_id,\s+recipient_id,\s+file_id,\s+delivered_to_recipient,\s+delivered_to_recipient_primary,\s+synced_to_sender_primary,\s+read_by_recipient,\s+read_at,\s+read_acked_by_sender,\s+created_at,\s+expires_at\s+FROM messages\s+WHERE id = ANY\(\$1::uuid\[\]\)`

// TestAcknowledgeDeliveryBatch_ExpiredMessage_CompletelyRejected verifies Phase 1:
// An expired message must be completely rejected:
// 1. Upfront GET query retrieves the row with expires_at in the past.
// 2. NO UPDATE query is executed.
// 3. It does NOT appear in AcknowledgedMessageIDs (count is 0).
// 4. NO DeliveryGroups (SSE) are produced.

// TestAcknowledgeDeliveryBatch_ExpiredMessage_CompletelyRejected verifies Phase 1:
// An expired message must be completely rejected:
// 1. Upfront GET query retrieves the row with expires_at in the past.
// 2. NO UPDATE query is executed.
// 3. It does NOT appear in AcknowledgedMessageIDs (count is 0).
// 4. NO DeliveryGroups (SSE) are produced.
func TestAcknowledgeDeliveryBatch_ExpiredMessage_CompletelyRejected(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	recipientID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	senderID := uuid.New()
	chatID := uuid.New()
	expiredMsgID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)
	pastExpiry := now.Add(-2 * time.Hour)

	// Upfront GET query returns the message as expired (expires_at in past)
	mock.ExpectQuery(deliveryAckExtendedPattern).
		WithArgs([]uuid.UUID{expiredMsgID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckExtendedColumns()).
			AddRow(expiredMsgID, chatID, senderID, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now.Add(-3*time.Hour), pastExpiry))

	// NO UPDATE query should be called!
	// NO chat update should be called!

	res, err := svc.AcknowledgeDeliveryBatch(context.Background(), []string{expiredMsgID.String()}, "recipient", true, recipientID)
	require.NoError(t, err)
	assert.Equal(t, int32(0), res.AcknowledgedCount, "expired message must NOT be acknowledged")
	assert.Empty(t, res.AcknowledgedMessageIDs, "acknowledged IDs list must be empty for expired row")
	assert.Empty(t, res.DeliveryGroups, "expired message must NOT produce SSE delivery groups")
	assert.Equal(t, []string{expiredMsgID.String()}, res.PermanentlyRejectedIDs, "expired message must be reported as permanently rejected")

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeDeliveryBatch_MixedBatch_FiltersExpiredAndForeign verifies Phase 1:
// In a mixed batch with:
// - 1 eligible unexpired message
// - 1 expired message
// - 1 already-acknowledged message
// - 1 foreign message (different recipient)
// Exactly:
// 1. Upfront GET query fetches all 4 messages.
// 2. ONLY the 1 eligible unexpired message is sent to UPDATE.
// 3. ONLY the eligible + already-acked messages are acknowledged (count: 2).
// 4. ONLY the newly updated eligible message produces SSE DeliveryGroups.
// 5. Expired and foreign messages are completely rejected.

// TestAcknowledgeDeliveryBatch_MixedBatch_FiltersExpiredAndForeign verifies Phase 1:
// In a mixed batch with:
// - 1 eligible unexpired message
// - 1 expired message
// - 1 already-acknowledged message
// - 1 foreign message (different recipient)
// Exactly:
// 1. Upfront GET query fetches all 4 messages.
// 2. ONLY the 1 eligible unexpired message is sent to UPDATE.
// 3. ONLY the eligible + already-acked messages are acknowledged (count: 2).
// 4. ONLY the newly updated eligible message produces SSE DeliveryGroups.
// 5. Expired and foreign messages are completely rejected.
func TestAcknowledgeDeliveryBatch_MixedBatch_FiltersExpiredAndForeign(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	recipientID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	foreignRecipientID := uuid.New()
	senderID := uuid.New()
	chatID := uuid.New()

	eligibleID := uuid.New()
	expiredID := uuid.New()
	alreadyAckedID := uuid.New()
	foreignID := uuid.New()

	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)
	pastExpiry := now.Add(-2 * time.Hour)

	// 1. Upfront GET query returns all 4 messages
	mock.ExpectQuery(deliveryAckExtendedPattern).
		WithArgs([]uuid.UUID{eligibleID, expiredID, alreadyAckedID, foreignID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckExtendedColumns()).
			// Eligible unexpired
			AddRow(eligibleID, chatID, senderID, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now, futureExpiry).
			// Expired
			AddRow(expiredID, chatID, senderID, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now.Add(-2*time.Hour), pastExpiry).
			// Already acknowledged unexpired
			AddRow(alreadyAckedID, chatID, senderID, recipientID.UuidUserId, nil, true, true, false, false, nil, false, now.Add(-1*time.Hour), futureExpiry).
			// Foreign unexpired (different recipient)
			AddRow(foreignID, chatID, senderID, foreignRecipientID, nil, false, false, false, false, nil, false, now, futureExpiry))

	// 2. Batch UPDATE must be executed ONLY for eligibleID (not expired, not alreadyAcked, not foreign)
	batchRows := pgxmock.NewRows(batchAckColumns()).
		AddRow(eligibleID, chatID, senderID, nil, true, false, false, false, now)

	mock.ExpectQuery(`(?s)UPDATE messages\s+SET.*delivered_to_recipient_primary = TRUE.*WHERE.*`).
		WithArgs(recipientID.UuidUserId, []uuid.UUID{eligibleID}).
		WillReturnRows(batchRows)

	// 3. Chat last_delivered_at updated ONLY for the newly updated eligible message
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+id = \$3`).
		WithArgs(recipientID.UuidUserId, now, chatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	reqIDs := []string{eligibleID.String(), expiredID.String(), alreadyAckedID.String(), foreignID.String()}
	res, err := svc.AcknowledgeDeliveryBatch(context.Background(), reqIDs, "recipient", true, recipientID)
	require.NoError(t, err)

	// Assertions
	assert.Equal(t, int32(2), res.AcknowledgedCount, "must acknowledge exactly 2 (eligible + alreadyAcked)")
	assert.ElementsMatch(t, []string{eligibleID.String(), alreadyAckedID.String()}, res.AcknowledgedMessageIDs)
	assert.NotContains(t, res.AcknowledgedMessageIDs, expiredID.String(), "expired message must NOT be in acknowledged IDs")
	assert.NotContains(t, res.AcknowledgedMessageIDs, foreignID.String(), "foreign message must NOT be in acknowledged IDs")
	assert.Equal(t, []string{expiredID.String(), foreignID.String()}, res.PermanentlyRejectedIDs,
		"expired and foreign messages must be reported as permanently rejected, in request order")

	// Verify DeliveryGroups only contains eligibleID
	require.Len(t, res.DeliveryGroups, 1)
	assert.Equal(t, senderID, res.DeliveryGroups[0].SenderID)
	assert.Equal(t, chatID, res.DeliveryGroups[0].ChatID)
	assert.Equal(t, []string{eligibleID.String()}, res.DeliveryGroups[0].MessageIDs)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeDelivery_SingleMethod_ExpiredMessage_Rejected verifies Phase 1:
// svc.AcknowledgeDelivery for a single message must follow the exact same batch rules:
// An expired message must be queried upfront via GetMessagesForDeliveryAckBatch,
// must NOT be updated, and must NOT trigger relay cleanup.

// TestAcknowledgeDelivery_SingleMethod_ExpiredMessage_Rejected verifies Phase 1:
// svc.AcknowledgeDelivery for a single message must follow the exact same batch rules:
// An expired message must be queried upfront via GetMessagesForDeliveryAckBatch,
// must NOT be updated, and must NOT trigger relay cleanup.
func TestAcknowledgeDelivery_SingleMethod_ExpiredMessage_Rejected(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	recipientID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	senderID := uuid.New()
	chatID := uuid.New()
	expiredID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)
	pastExpiry := now.Add(-2 * time.Hour)

	// Upfront GET query returns expired message
	mock.ExpectQuery(deliveryAckExtendedPattern).
		WithArgs([]uuid.UUID{expiredID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckExtendedColumns()).
			AddRow(expiredID, chatID, senderID, recipientID.UuidUserId, nil, false, false, false, false, nil, false, now.Add(-3*time.Hour), pastExpiry))

	// NO UPDATE, NO CHAT UPDATE, NO RELAY CLEANUP

	err := svc.AcknowledgeDelivery(context.Background(), expiredID, "recipient", true, recipientID)
	require.NoError(t, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestSQL_DeliveryAndSyncWriteQueries_EnforceExpirationGuard verifies Phase 1:
// The batch delivery and sync UPDATE queries sent to PostgreSQL MUST include "expires_at > now()"
// in their WHERE clauses to protect against TOCTOU race conditions and clock skew.

// TestSQL_DeliveryAndSyncWriteQueries_EnforceExpirationGuard verifies Phase 1:
// The batch delivery and sync UPDATE queries sent to PostgreSQL MUST include "expires_at > now()"
// in their WHERE clauses to protect against TOCTOU race conditions and clock skew.
func TestSQL_DeliveryAndSyncWriteQueries_EnforceExpirationGuard(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	queries := personal_chat_store.New(mock)
	recipientID := uuid.New()
	senderID := uuid.New()
	msgID := uuid.New()

	cols := []string{"id", "chat_id", "sender_id", "file_id", "delivered_to_recipient_primary", "synced_to_sender_primary", "read_by_recipient", "read_acked_by_sender", "created_at"}

	// 1. MarkMessagesDeliveredToRecipientPrimaryBatch must include expires_at > now()
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET.*delivered_to_recipient_primary = TRUE.*WHERE.*expires_at > now\(\)`).
		WithArgs(recipientID, []uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(cols))

	_, err = queries.MarkMessagesDeliveredToRecipientPrimaryBatch(context.Background(), personal_chat_store.MarkMessagesDeliveredToRecipientPrimaryBatchParams{
		RecipientID: recipientID,
		MessageIds:  []uuid.UUID{msgID},
	})
	require.NoError(t, err)

	// 2. MarkMessagesDeliveredToRecipientBatch must include expires_at > now()
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET.*delivered_to_recipient = TRUE.*WHERE.*expires_at > now\(\)`).
		WithArgs(recipientID, []uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(cols))

	_, err = queries.MarkMessagesDeliveredToRecipientBatch(context.Background(), personal_chat_store.MarkMessagesDeliveredToRecipientBatchParams{
		RecipientID: recipientID,
		MessageIds:  []uuid.UUID{msgID},
	})
	require.NoError(t, err)

	// 3. MarkMessagesSyncedToSenderPrimaryBatch must include expires_at > now()
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET\s+synced_to_sender_primary = TRUE.*WHERE.*expires_at > now\(\)`).
		WithArgs(senderID, []uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(cols))

	_, err = queries.MarkMessagesSyncedToSenderPrimaryBatch(context.Background(), personal_chat_store.MarkMessagesSyncedToSenderPrimaryBatchParams{
		SenderID:   senderID,
		MessageIds: []uuid.UUID{msgID},
	})
	require.NoError(t, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func markReadAckedReturningColumns() []string {
	return []string{
		"id", "chat_id", "sender_id", "file_id",
		"delivered_to_recipient_primary", "synced_to_sender_primary",
		"read_by_recipient", "read_acked_by_sender",
		"created_at",
	}
}

func readAckExtendedColumns() []string {
	return []string{
		"id", "chat_id", "sender_id", "recipient_id",
		"file_id",
		"delivered_to_recipient_primary", "synced_to_sender_primary",
		"read_by_recipient", "read_at", "read_acked_by_sender",
		"created_at", "expires_at",
	}
}

const readAckUpfrontPattern = `(?s)SELECT\s+id,\s+chat_id,\s+sender_id,\s+recipient_id,\s+file_id,\s+delivered_to_recipient_primary,\s+synced_to_sender_primary,\s+read_by_recipient,\s+read_at,\s+read_acked_by_sender,\s+created_at,\s+expires_at\s+FROM messages\s+WHERE id = ANY\(\$1::uuid\[\]\)`

// TestAcknowledgeReadReceiptBatch_ExpiredMessage_CompletelyRejected verifies Phase 1:
// An expired message must be completely rejected:
// 1. Upfront GET query retrieves the row with expires_at in the past.
// 2. NO UPDATE query is executed.
// 3. It does NOT appear in AcknowledgedMessageIds (count is 0).

// TestAcknowledgeReadReceiptBatch_ExpiredMessage_CompletelyRejected verifies Phase 1:
// An expired message must be completely rejected:
// 1. Upfront GET query retrieves the row with expires_at in the past.
// 2. NO UPDATE query is executed.
// 3. It does NOT appear in AcknowledgedMessageIds (count is 0).
func TestAcknowledgeReadReceiptBatch_ExpiredMessage_CompletelyRejected(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	recipientID := uuid.New()
	chatID := uuid.New()
	expiredMsgID := uuid.New()

	// 1. Upfront GET query returns the expired message
	now := time.Now().UTC()
	expiredAt := now.Add(-2 * time.Hour)
	readAt := now.Add(-3 * time.Hour)
	mock.ExpectQuery(readAckUpfrontPattern).
		WithArgs([]uuid.UUID{expiredMsgID}).
		WillReturnRows(pgxmock.NewRows(readAckExtendedColumns()).
			AddRow(
				expiredMsgID, chatID, senderID.UuidUserId, recipientID,
				nil, true, true,
				true, &readAt, false, // read_by_recipient = true, read_acked_by_sender = false
				now.Add(-4*time.Hour), expiredAt,
			))

	// NO chat lookup (this endpoint never reads the chat) and NO UPDATE queries!

	res, err := svc.AcknowledgeReadReceiptBatch(context.Background(), &AckReadReceiptBatchPayload{
		MessageIDs: []string{expiredMsgID.String()},
	}, senderID, true)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, int32(0), res.AcknowledgedCount, "Expired message must have AcknowledgedCount = 0")
	assert.Empty(t, res.AcknowledgedMessageIds, "Expired message must NOT be in AcknowledgedMessageIds")
	assert.Equal(t, []string{expiredMsgID.String()}, res.PermanentlyRejectedMessageIds, "Expired message must be reported as permanently rejected")

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeReadReceiptBatch_MixedBatch_FiltersExpiredForeignAndNonExistent verifies:
// Mixed batch of 5 messages:
// 1. Eligible: unexpired, read_by_recipient = true, read_acked_by_sender = false -> updated & confirmed
// 2. Already True: unexpired, read_by_recipient = true, read_acked_by_sender = true -> confirmed (idempotent, no update)
// 3. Expired: expires_at < now() -> completely rejected
// 4. Foreign: sender_id != caller -> completely rejected without failing the whole batch!
// 5. Non-existent: missing from upfront query -> completely rejected

// TestAcknowledgeReadReceiptBatch_MixedBatch_FiltersExpiredForeignAndNonExistent verifies:
// Mixed batch of 5 messages:
// 1. Eligible: unexpired, read_by_recipient = true, read_acked_by_sender = false -> updated & confirmed
// 2. Already True: unexpired, read_by_recipient = true, read_acked_by_sender = true -> confirmed (idempotent, no update)
// 3. Expired: expires_at < now() -> completely rejected
// 4. Foreign: sender_id != caller -> completely rejected without failing the whole batch!
// 5. Non-existent: missing from upfront query -> completely rejected
func TestAcknowledgeReadReceiptBatch_MixedBatch_FiltersExpiredForeignAndNonExistent(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	senderID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	foreignSenderID := uuid.New()
	recipientID := uuid.New()
	chatID := uuid.New()

	msgEligible := uuid.New()
	msgAlreadyTrue := uuid.New()
	msgExpired := uuid.New()
	msgForeign := uuid.New()
	msgNonExistent := uuid.New()

	now := time.Now().UTC()
	freshExpiry := now.Add(24 * time.Hour)
	pastExpiry := now.Add(-2 * time.Hour)
	readAt := now.Add(-1 * time.Hour)

	// 1. Upfront GET query returns 4 found rows (msgNonExistent is omitted)
	mock.ExpectQuery(readAckUpfrontPattern).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows(readAckExtendedColumns()).
			AddRow(
				msgEligible, chatID, senderID.UuidUserId, recipientID,
				nil, true, true,
				true, &readAt, false, // eligible
				now.Add(-2*time.Hour), freshExpiry,
			).
			AddRow(
				msgAlreadyTrue, chatID, senderID.UuidUserId, recipientID,
				nil, true, true,
				true, &readAt, true, // already true
				now.Add(-2*time.Hour), freshExpiry,
			).
			AddRow(
				msgExpired, chatID, senderID.UuidUserId, recipientID,
				nil, true, true,
				true, &readAt, false, // expired
				now.Add(-4*time.Hour), pastExpiry,
			).
			AddRow(
				msgForeign, chatID, foreignSenderID, recipientID,
				nil, true, true,
				true, &readAt, false, // foreign sender!
				now.Add(-2*time.Hour), freshExpiry,
			))

	// 2. Batch UPDATE should ONLY be called for msgEligible (with expires_at > now()).
	// No chat lookup comes first: the UPDATE is scoped by the sender and the IDs.
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET\s+read_acked_by_sender = TRUE\s+WHERE.*expires_at > now\(\)`).
		WithArgs(senderID.UuidUserId, []uuid.UUID{msgEligible}).
		WillReturnRows(pgxmock.NewRows(markReadAckedReturningColumns()).
			AddRow(msgEligible, chatID, senderID.UuidUserId, nil, true, true, true, true, now))

	// 3. Async cleanup: one sweep for the one chat of the cleaned rows
	// (msgEligible and the retried msgAlreadyTrue)
	mock.ExpectExec(`(?s)DELETE FROM messages\s+WHERE\s+chat_id = \$1.*read_acked_by_sender = TRUE`).
		WithArgs(chatID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	allIDs := []string{
		msgEligible.String(),
		msgAlreadyTrue.String(),
		msgExpired.String(),
		msgForeign.String(),
		msgNonExistent.String(),
	}

	res, err := svc.AcknowledgeReadReceiptBatch(context.Background(), &AckReadReceiptBatchPayload{
		MessageIDs: allIDs,
	}, senderID, true)

	require.NoError(t, err, "Must NOT fail the whole batch for foreign or expired message IDs")
	require.NotNil(t, res)
	assert.Equal(t, int32(2), res.AcknowledgedCount, "Should acknowledge exactly 2 (Eligible + AlreadyTrue)")
	assert.ElementsMatch(t, []string{msgEligible.String(), msgAlreadyTrue.String()}, res.AcknowledgedMessageIds)
	assert.Equal(t, []string{msgExpired.String(), msgForeign.String(), msgNonExistent.String()}, res.PermanentlyRejectedMessageIds,
		"expired, foreign, and non-existent messages must be reported as permanently rejected, in request order")

	require.Eventually(t, func() bool {
		return mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond, "async chat cleanup must complete")
}

// TestSQL_MarkMessagesReadAckedBySender_EnforcesExpirationGuard verifies that the SQL update query has expires_at > now()

// TestSQL_MarkMessagesReadAckedBySender_EnforcesExpirationGuard verifies that the SQL update query has expires_at > now()
func TestSQL_MarkMessagesReadAckedBySender_EnforcesExpirationGuard(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	queries := personal_chat_store.New(mock)
	chatID := uuid.New()
	senderID := uuid.New()
	msgID := uuid.New()
	now := time.Now()

	// The WHERE clause names no chat: the caller's sender_id and the IDs scope
	// it, so one request can ACK rows from several chats.
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET\s+read_acked_by_sender = TRUE\s+WHERE\s+sender_id = \$1\s+AND id = ANY\(\$2::uuid\[\]\)\s+AND read_by_recipient = TRUE\s+AND read_acked_by_sender = FALSE\s+AND expires_at > now\(\)`).
		WithArgs(senderID, []uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(markReadAckedReturningColumns()).
			AddRow(msgID, chatID, senderID, nil, true, true, true, true, now))

	rows, err := queries.MarkMessagesReadAckedBySender(context.Background(), personal_chat_store.MarkMessagesReadAckedBySenderParams{
		SenderID:   senderID,
		MessageIds: []uuid.UUID{msgID},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeAndReadBatch_ExpiredMessage_CompletelyRejected verifies that an expired message
// is completely rejected upfront: zero DB writes, zero count in response, empty ReadMessages,
// and zero items in newlyRead (SSE).
func TestAcknowledgeAndReadBatch_ExpiredMessage_CompletelyRejected(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	recipientID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	senderID := uuid.New()
	chatID := uuid.New()
	expiredMsgID := uuid.New()
	pastTime := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)

	cols := []string{
		"id", "chat_id", "sender_id", "recipient_id",
		"file_id", "delivered_to_recipient", "delivered_to_recipient_primary",
		"synced_to_sender_primary",
		"read_by_recipient", "read_acked_by_sender", "read_at",
		"created_at", "expires_at",
	}

	// With dedicated upfront query GetMessagesForAckAndReadBatch, query returns expired row
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{expiredMsgID}).
		WillReturnRows(pgxmock.NewRows(cols).
			AddRow(expiredMsgID, chatID, senderID, recipientID.UuidUserId,
				nil, true, true, true, false, false, nil, pastTime, pastTime))

	out, err := svc.AcknowledgeAndReadBatch(context.Background(), &AckAndReadBatchPayload{
		MessageIDs: []string{expiredMsgID.String()},
	}, recipientID, true)

	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, out.ChatID, "targetChatID must be Nil for fully expired batch")
	assert.Equal(t, uuid.Nil, out.OtherUserID, "otherUserID must be Nil for fully expired batch")
	assert.True(t, out.ReadAt.IsZero())
	assert.Empty(t, out.Receipts, "expired message must not be in ReadMessages")
	assert.Empty(t, out.NewlyRead, "expired message must not be in newlyRead for SSE")
	assert.Equal(t, []string{expiredMsgID.String()}, out.PermanentlyRejectedIDs, "expired message must be reported as permanently rejected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeAndReadBatch_MixedBatch_FiltersExpiredForeignAndNonExistent verifies that
// in a mixed batch containing:
// 1. Eligible unread message
// 2. Already-read message
// 3. Expired message
// 4. Foreign message (sender is caller)
// 5. Non-existent message
// ONLY the Eligible and Already-Read messages are confirmed in response;
// ONLY the Eligible message is updated in DB and broadcast via SSE (newlyRead);
// and expired, foreign, and non-existent messages are completely rejected.

// TestAcknowledgeAndReadBatch_MixedBatch_FiltersExpiredForeignAndNonExistent verifies that
// in a mixed batch containing:
// 1. Eligible unread message
// 2. Already-read message
// 3. Expired message
// 4. Foreign message (sender is caller)
// 5. Non-existent message
// ONLY the Eligible and Already-Read messages are confirmed in response;
// ONLY the Eligible message is updated in DB and broadcast via SSE (newlyRead);
// and expired, foreign, and non-existent messages are completely rejected.
func TestAcknowledgeAndReadBatch_MixedBatch_FiltersExpiredForeignAndNonExistent(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	store := personal_chat_store.New(mock)
	svc := &chatService{
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
		// Note: svc.Pool is nil in unit mock, but we test the upfront filter and response logic
	}

	recipientID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	senderID := uuid.New()
	chatID := uuid.New()

	eligibleMsgID := uuid.New()
	alreadyReadMsgID := uuid.New()
	expiredMsgID := uuid.New()
	foreignMsgID := uuid.New()
	nonExistentMsgID := uuid.New()

	now := time.Now().UTC()
	futureExpiry := now.Add(24 * time.Hour)
	pastExpiry := now.Add(-2 * time.Hour)
	origReadAt := now.Add(-10 * time.Minute)

	cols := []string{
		"id", "chat_id", "sender_id", "recipient_id",
		"file_id", "delivered_to_recipient", "delivered_to_recipient_primary",
		"synced_to_sender_primary",
		"read_by_recipient", "read_acked_by_sender", "read_at",
		"created_at", "expires_at",
	}

	// 1. Upfront query returns 4 rows (nonExistentMsgID is omitted by DB)
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{eligibleMsgID, alreadyReadMsgID, expiredMsgID, foreignMsgID, nonExistentMsgID}).
		WillReturnRows(pgxmock.NewRows(cols).
			AddRow(eligibleMsgID, chatID, senderID, recipientID.UuidUserId, nil, true, true, true, false, false, nil, now, futureExpiry).
			AddRow(alreadyReadMsgID, chatID, senderID, recipientID.UuidUserId, nil, true, true, true, true, false, &origReadAt, now, futureExpiry).
			AddRow(expiredMsgID, chatID, senderID, recipientID.UuidUserId, nil, true, true, true, false, false, nil, now, pastExpiry).
			AddRow(foreignMsgID, chatID, recipientID.UuidUserId, senderID, nil, true, true, true, false, false, nil, now, futureExpiry))

	out, err := svc.AcknowledgeAndReadBatch(context.Background(), &AckAndReadBatchPayload{
		MessageIDs: []string{
			eligibleMsgID.String(),
			alreadyReadMsgID.String(),
			expiredMsgID.String(),
			foreignMsgID.String(),
			nonExistentMsgID.String(),
		},
	}, recipientID, true)

	require.NoError(t, err)
	assert.Equal(t, chatID, out.ChatID)
	assert.Equal(t, uuid.Nil, out.OtherUserID, "no transaction ran, so the peer is not resolved in this unit test")
	assert.False(t, out.ReadAt.IsZero())

	// Since Pool is nil in this mock svc, write transaction is skipped, and alreadyReadReceipts is returned.
	// We verify that alreadyReadMsgID is confirmed with original timestamp,
	// and expired, foreign, and non-existent messages are strictly excluded!
	require.Len(t, out.Receipts, 1)
	assert.Equal(t, alreadyReadMsgID.String(), out.Receipts[0].MessageId)
	assert.Equal(t, origReadAt.Unix(), out.Receipts[0].ReadAt.Seconds)
	assert.Empty(t, out.NewlyRead)
	// The foreign row is the caller's own sent message: it only fails this
	// endpoint's recipient rule, so it is skipped, not permanently rejected.
	assert.Equal(t, []string{expiredMsgID.String(), nonExistentMsgID.String()}, out.PermanentlyRejectedIDs)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestSQL_MarkMessagesAsRead_EnforcesExpirationGuard verifies that the SQL query
// definition for MarkMessagesAsRead strictly includes 'AND expires_at > now()'.

// TestSQL_MarkMessagesAsRead_EnforcesExpirationGuard verifies that the SQL query
// definition for MarkMessagesAsRead strictly includes 'AND expires_at > now()'.
func TestSQL_MarkMessagesAsRead_EnforcesExpirationGuard(t *testing.T) {
	sqlBytes, err := os.ReadFile("../../../../db/personal/queries/personal_chat.sql")
	require.NoError(t, err)
	sqlContent := string(sqlBytes)

	startIdx := strings.Index(sqlContent, "-- name: MarkMessagesAsRead :many")
	require.True(t, startIdx != -1, "MarkMessagesAsRead query must exist in personal_chat.sql")

	endIdx := strings.Index(sqlContent[startIdx:], ";")
	require.True(t, endIdx != -1, "closing semicolon must exist")
	queryText := sqlContent[startIdx : startIdx+endIdx]

	assert.Contains(t, queryText, "expires_at > now()",
		"MarkMessagesAsRead must include 'AND expires_at > now()' to protect DB writes against expired rows")
}

// TestMarkChatRead_ExpiredMessage_CompletelyRejected verifies that an expired message
// is completely rejected upfront by MarkChatRead:
// 1. Omitted from response readReceipts.
// 2. Omitted from newlyRead (0 SSE emitted).
// 3. 0 DB write operations executed.
// 4. Returns empty receipts and uuid.Nil targetChatID when all messages are expired.
func TestMarkChatRead_ExpiredMessage_CompletelyRejected(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	senderID := uuid.New()
	expiredMsgID := uuid.New()

	now := time.Now().UTC()
	pastExpiry := now.Add(-2 * time.Hour)

	// Upfront dedicated query returns the expired message
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{expiredMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForMarkChatReadColumns()).
			AddRow(expiredMsgID, targetChatID, senderID, callerID.UuidUserId, false, nil, false, now, pastExpiry))

	// No queries to chats table expected because expired messages are completely rejected upfront!
	out, err := svc.MarkChatRead(context.Background(), &MarkChatReadPayload{
		MessageIDs: []string{expiredMsgID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, out.ChatID, "targetChatID must be Nil when all messages are expired")
	assert.Equal(t, uuid.Nil, out.OtherUserID, "otherUserID must be Nil when all messages are expired")
	assert.Empty(t, out.Receipts, "expired message must be omitted from readReceipts")
	assert.Empty(t, out.NewlyRead, "expired message must be omitted from newlyRead / SSE")
	assert.True(t, out.ReadAt.IsZero())
	assert.Equal(t, []string{expiredMsgID.String()}, out.PermanentlyRejectedIDs, "expired message must be reported as permanently rejected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMarkChatRead_MixedBatch_FiltersExpiredAndNonExistent verifies that in a mixed batch:
// 1. Eligible unread message
// 2. Already-read message
// 3. Expired message
// 4. Non-existent message
// ONLY Eligible and Already-Read messages are confirmed in response;
// Expired and non-existent messages are completely omitted.

// TestMarkChatRead_MixedBatch_FiltersExpiredAndNonExistent verifies that in a mixed batch:
// 1. Eligible unread message
// 2. Already-read message
// 3. Expired message
// 4. Non-existent message
// ONLY Eligible and Already-Read messages are confirmed in response;
// Expired and non-existent messages are completely omitted.
func TestMarkChatRead_MixedBatch_FiltersExpiredAndNonExistent(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	senderID := uuid.New()

	eligibleMsgID := uuid.New()
	alreadyReadMsgID := uuid.New()
	expiredMsgID := uuid.New()
	nonExistentMsgID := uuid.New()

	now := time.Now().UTC()
	futureExpiry := now.Add(24 * time.Hour)
	pastExpiry := now.Add(-2 * time.Hour)
	origReadAt := now.Add(-10 * time.Minute)

	// Upfront query returns 3 rows (nonExistentMsgID omitted by DB)
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{eligibleMsgID, alreadyReadMsgID, expiredMsgID, nonExistentMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForMarkChatReadColumns()).
			AddRow(eligibleMsgID, targetChatID, senderID, callerID.UuidUserId, false, nil, false, now, futureExpiry).
			AddRow(alreadyReadMsgID, targetChatID, senderID, callerID.UuidUserId, true, &origReadAt, false, now, futureExpiry).
			AddRow(expiredMsgID, targetChatID, senderID, callerID.UuidUserId, false, nil, false, now, pastExpiry))

	// Chat lookup for targetChatID
	mock.ExpectQuery(`(?s)SELECT .* FROM chats WHERE id = \$1 LIMIT 1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, senderID, int32(1), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	out, err := svc.MarkChatRead(context.Background(), &MarkChatReadPayload{
		MessageIDs: []string{eligibleMsgID.String(), alreadyReadMsgID.String(), expiredMsgID.String(), nonExistentMsgID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, targetChatID, out.ChatID)
	assert.Equal(t, senderID, out.OtherUserID)

	// Only alreadyReadMsgID is confirmed in response (Pool is nil, so eligibleMsgID is not mutated in unit mock)
	// Crucially, expiredMsgID and nonExistentMsgID MUST NOT be in readReceipts!
	var receiptIDs []string
	for _, r := range out.Receipts {
		receiptIDs = append(receiptIDs, r.MessageId)
	}
	assert.Contains(t, receiptIDs, alreadyReadMsgID.String())
	assert.NotContains(t, receiptIDs, expiredMsgID.String(), "expired message must be rejected")
	assert.NotContains(t, receiptIDs, nonExistentMsgID.String(), "non-existent message must be rejected")
	assert.Empty(t, out.NewlyRead)
	assert.Equal(t, []string{expiredMsgID.String(), nonExistentMsgID.String()}, out.PermanentlyRejectedIDs,
		"expired and non-existent messages must be reported as permanently rejected, in request order")
	assert.NotContains(t, out.PermanentlyRejectedIDs, eligibleMsgID.String(), "an eligible message is never rejected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_ExpiredMessage_CompletelyRejected verifies that:
// 1. Expired messages are rejected upfront.
// 2. Omitted from response AcknowledgedMessageIds.
// 3. 0 DB write operations executed.
// 4. Returns empty response and uuid.Nil targetChatID when all messages are expired.
func TestUnsendMessage_ExpiredMessage_CompletelyRejected(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	otherUserID := uuid.New()
	expiredMsgID := uuid.New()

	now := time.Now().UTC()
	pastExpiry := now.Add(-2 * time.Hour)

	// Upfront query returns the expired message
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{expiredMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(expiredMsgID, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, pastExpiry))

	// ZERO queries to chats table because all messages are expired!
	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{expiredMsgID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, out.ChatID, "targetChatID must be Nil when all messages are expired")
	assert.Equal(t, uuid.Nil, out.RecipientID, "recipientID must be Nil when all messages are expired")
	assert.Empty(t, out.AcknowledgedIDs, "expired message must be omitted from response")
	assert.Empty(t, out.NewlyUnsentIDs, "expired message must be omitted from SSE")
	assert.Equal(t, []string{expiredMsgID.String()}, out.PermanentlyRejectedIDs, "expired message must be reported as permanently rejected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_AlreadyTrue_TriggersFileCleanup verifies that:
// When a message is already unsent (message_type == 'unsent') but still has a file_id,
// UnsendMessageHandler locks the chat row, calls RegisterTx to register the file in pending_uploads,
// executes ClearMessageFileFields, and triggers async R2 cleanup.

// TestUnsendMessage_AlreadyTrue_TriggersFileCleanup verifies that:
// When a message is already unsent (message_type == 'unsent') but still has a file_id,
// UnsendMessageHandler locks the chat row, calls RegisterTx to register the file in pending_uploads,
// executes ClearMessageFileFields, and triggers async R2 cleanup.
func TestUnsendMessage_AlreadyTrue_TriggersFileCleanup(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	store := personal_chat_store.New(mock)
	spy := &spyPendingUploads{}
	svc := &chatService{
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  spy,
	}

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	otherUserID := uuid.New()
	alreadyUnsentMsgID := uuid.New()
	fileID := "test-unsend-file-id"

	now := time.Now().UTC()
	futureExpiry := now.Add(24 * time.Hour)

	// Upfront query returns the already-unsent message with file_id
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{alreadyUnsentMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(alreadyUnsentMsgID, targetChatID, callerID.UuidUserId, otherUserID, "unsent", &fileID, false, futureExpiry))

	// Chat participant check for eligibility
	mock.ExpectQuery(`(?s)SELECT .* FROM chats WHERE id = \$1 LIMIT 1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// Transactional outbox flow for already-unsent message with attached file:
	// 1. Lock chat row FOR NO KEY UPDATE
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))
	// 2. ClearMessagesFileFieldsBatch inside the transaction
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+file_id = NULL.*WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{alreadyUnsentMsgID}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{alreadyUnsentMsgID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, targetChatID, out.ChatID)
	assert.Equal(t, otherUserID, out.RecipientID)
	require.Len(t, out.AcknowledgedIDs, 1)
	assert.Contains(t, out.AcknowledgedIDs, alreadyUnsentMsgID.String())
	assert.Empty(t, out.NewlyUnsentIDs, "already unsent message must not trigger SSE")

	assert.NoError(t, mock.ExpectationsWereMet())
	spy.mu.Lock()
	assert.Contains(t, spy.registeredBatchFiles, fileID, "file must be registered via RegisterBatchTx in pending_uploads")
	spy.mu.Unlock()
}

// TestUnsendMessage_EligibleWithFile_TransactionalOutboxFileCleanup verifies that:
// When an eligible message with a file is unsent, it:
// 1. Locks chat row FOR NO KEY UPDATE
// 2. Updates message to unsent
// 3. Registers file in pending_uploads via RegisterTx
// 4. Clears message file fields via ClearMessageFileFields within transaction
// 5. Creates sync actions, updates unsend preview, decrements unread
// 6. Asynchronously deletes file from R2 and removes from pending_uploads

// TestUnsendMessage_EligibleWithFile_TransactionalOutboxFileCleanup verifies that:
// When an eligible message with a file is unsent, it:
// 1. Locks chat row FOR NO KEY UPDATE
// 2. Updates message to unsent
// 3. Registers file in pending_uploads via RegisterTx
// 4. Clears message file fields via ClearMessageFileFields within transaction
// 5. Creates sync actions, updates unsend preview, decrements unread
// 6. Asynchronously deletes file from R2 and removes from pending_uploads
func TestUnsendMessage_EligibleWithFile_TransactionalOutboxFileCleanup(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	store := personal_chat_store.New(mock)
	spy := &spyPendingUploads{}
	svc := &chatService{
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  spy,
	}

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	recipientID := uuid.New()
	msgID := uuid.New()
	fileID := "test-eligible-file-id"

	now := time.Now().UTC()
	futureExpiry := now.Add(24 * time.Hour)

	// Upfront query returns eligible message with file
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(msgID, targetChatID, callerID.UuidUserId, recipientID, "media", &fileID, false, futureExpiry))

	// Chat participant check for eligibility
	mock.ExpectQuery(`(?s)SELECT .* FROM chats WHERE id = \$1 LIMIT 1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, recipientID, int32(1), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// Transactional outbox flow:
	// 1. Lock chat row
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, recipientID, int32(1), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// 2. UpdateMessageToUnsent
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+content = 'Message unsent'`).
		WithArgs(msgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 3. CreateSyncAction (recipient)
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), recipientID, "unsend", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), recipientID, "unsend", []byte("{}"), false, now, now))

	// 4. UpdateChatUnsendPreview
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+id = \$1\s+AND last_message_id = \$2`).
		WithArgs(targetChatID, &msgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 5. UpdateChatUnsendDecrement
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE id = \$3`).
		WithArgs(recipientID, int32(1), targetChatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 6. ClearMessagesFileFieldsBatch within transaction
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+file_id = NULL.*WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{msgID}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{msgID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, targetChatID, out.ChatID)
	assert.Equal(t, recipientID, out.RecipientID)
	assert.Contains(t, out.AcknowledgedIDs, msgID.String())
	assert.Contains(t, out.NewlyUnsentIDs, msgID.String())

	assert.NoError(t, mock.ExpectationsWereMet())
	spy.mu.Lock()
	assert.Contains(t, spy.registeredBatchFiles, fileID, "eligible message file must be registered via RegisterBatchTx in pending_uploads")
	spy.mu.Unlock()
}

// TestUnsendMessage_BatchFiles_BatchPendingUploadAndBatchClearFileFields verifies that:
// When multiple messages (both already-unsent and eligible) with attached files are unsent together,
// it executes EXACTLY ONE batch RegisterBatchTx into pending_uploads and EXACTLY ONE batch
// ClearMessagesFileFieldsBatch update in the database, followed by DeleteFilesBatch in the background.

// TestUnsendMessage_BatchFiles_BatchPendingUploadAndBatchClearFileFields verifies that:
// When multiple messages (both already-unsent and eligible) with attached files are unsent together,
// it executes EXACTLY ONE batch RegisterBatchTx into pending_uploads and EXACTLY ONE batch
// ClearMessagesFileFieldsBatch update in the database, followed by DeleteFilesBatch in the background.
func TestUnsendMessage_BatchFiles_BatchPendingUploadAndBatchClearFileFields(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	store := personal_chat_store.New(mock)
	spy := &spyPendingUploads{}
	svc := &chatService{
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  spy,
	}

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	recipientID := uuid.New()

	alreadyMsgID := uuid.New()
	alreadyFileID := "already-file-1"

	eligMsg1 := uuid.New()
	eligFile1 := "elig-file-1"

	eligMsg2 := uuid.New()
	eligFile2 := "elig-file-2"

	now := time.Now().UTC()
	futureExpiry := now.Add(24 * time.Hour)

	// Upfront batch fetch
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{alreadyMsgID, eligMsg1, eligMsg2}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(alreadyMsgID, targetChatID, callerID.UuidUserId, recipientID, "unsent", &alreadyFileID, false, futureExpiry).
			AddRow(eligMsg1, targetChatID, callerID.UuidUserId, recipientID, "media", &eligFile1, false, futureExpiry).
			AddRow(eligMsg2, targetChatID, callerID.UuidUserId, recipientID, "media", &eligFile2, false, futureExpiry))

	// Chat participant check for eligibility
	mock.ExpectQuery(`(?s)SELECT .* FROM chats WHERE id = \$1 LIMIT 1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, recipientID, int32(2), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// Lock chat row
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, recipientID, int32(2), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// Single-pass batch message update
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET\s+content = 'Message unsent'.*WHERE\s+id = ANY\(\$1::uuid\[\]\).*RETURNING id`).
		WithArgs([]uuid.UUID{eligMsg1, eligMsg2}).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).
			AddRow(eligMsg1).
			AddRow(eligMsg2))

	// Single sync action for recipient with batch payload
	recipientPayloadArg := &capturedArgument{}
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), recipientID, "unsend", recipientPayloadArg).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), recipientID, "unsend", []byte("{}"), false, now, now))

	// Single-pass batch chat preview update
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_last_message_content = 'Message unsent'.*WHERE\s+id = \$1\s+AND last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(targetChatID, []uuid.UUID{eligMsg1, eligMsg2}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// Single-pass chat unread decrement with Amount = 2
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE id = \$3`).
		WithArgs(recipientID, int32(2), targetChatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// SINGLE BATCH EXECUTION for clearing all 3 message file fields
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+file_id = NULL.*WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{alreadyMsgID, eligMsg1, eligMsg2}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 3))

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{alreadyMsgID.String(), eligMsg1.String(), eligMsg2.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, targetChatID, out.ChatID)
	assert.Equal(t, recipientID, out.RecipientID)
	assert.ElementsMatch(t, []string{alreadyMsgID.String(), eligMsg1.String(), eligMsg2.String()}, out.AcknowledgedIDs)
	assert.ElementsMatch(t, []string{eligMsg1.String(), eligMsg2.String()}, out.NewlyUnsentIDs)

	payload := recipientPayloadArg.payload(t)
	assert.ElementsMatch(t, []string{eligMsg1.String(), eligMsg2.String()}, payload.MessageIDs,
		"sync payload must carry only newly unsent IDs, not the already-unsent one")
	assert.Equal(t, targetChatID.String(), payload.ChatID)

	assert.NoError(t, mock.ExpectationsWereMet())
	spy.mu.Lock()
	assert.ElementsMatch(t, []string{alreadyFileID, eligFile1, eligFile2}, spy.registeredBatchFiles, "all 3 files must be batch-registered via RegisterBatchTx")
	spy.mu.Unlock()

	// Verify that the post-commit background goroutine asynchronously ran kit.DeleteFilesBatch
	// and removed all 3 files from pending_uploads upon successful deletion.
	assert.Eventually(t, func() bool {
		spy.mu.Lock()
		defer spy.mu.Unlock()
		return len(spy.removedFiles) == 3
	}, 2*time.Second, 10*time.Millisecond, "background goroutine must call kit.DeleteFilesBatch and remove all 3 files from pending_uploads")

	spy.mu.Lock()
	assert.ElementsMatch(t, []string{alreadyFileID, eligFile1, eligFile2}, spy.removedFiles, "all 3 files must be removed from pending_uploads")
	spy.mu.Unlock()
}

// TestUnsendMessage_AsyncGoroutineBatchCleanup_PartialFailureLeavesFailedFileInPendingUploads verifies:
//  1. When batch contains multiple files, all are registered in pending_uploads via RegisterBatchTx.
//  2. The background goroutine runs kit.DeleteFilesBatch.
//  3. Files whose R2 deletion succeeds (e.g. retired account prefix) ARE removed from pending_uploads.
//  4. Files whose R2 deletion fails (e.g. dummy credentials / network error) ARE NOT removed from pending_uploads,
//     preserving them in pending_uploads for the 15-minute sweeper to retry.

// TestUnsendMessage_AsyncGoroutineBatchCleanup_PartialFailureLeavesFailedFileInPendingUploads verifies:
//  1. When batch contains multiple files, all are registered in pending_uploads via RegisterBatchTx.
//  2. The background goroutine runs kit.DeleteFilesBatch.
//  3. Files whose R2 deletion succeeds (e.g. retired account prefix) ARE removed from pending_uploads.
//  4. Files whose R2 deletion fails (e.g. dummy credentials / network error) ARE NOT removed from pending_uploads,
//     preserving them in pending_uploads for the 15-minute sweeper to retry.
func TestUnsendMessage_AsyncGoroutineBatchCleanup_PartialFailureLeavesFailedFileInPendingUploads(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	r2Pool, err := clients.NewR2ClientPool(&config.R2PoolConfig{
		Accounts: []config.R2AccountConfig{
			{
				Name:             "dummy-pool-account",
				AccountID:        "dummy-account-id",
				AccessKeyID:      "dummy-access-key",
				SecretAccessKey:  "dummy-secret-key",
				ChatFilesBucket:  "dummy-bucket",
				ProfilePicBucket: "dummy-profile-bucket",
			},
		},
		PrimaryChatAccount:    "dummy-pool-account",
		PrimaryProfileAccount: "dummy-pool-account",
	})
	require.NoError(t, err)

	store := personal_chat_store.New(mock)
	spy := &spyPendingUploads{}
	svc := &chatService{
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  spy,
		R2Pool:          r2Pool,
	}

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	targetChatID := uuid.New()
	recipientID := uuid.New()

	msg1ID := uuid.New()
	// Retired account prefix: DeleteChatFile treats missing pool account as skipped/succeeded (returns nil)
	successFileID := "retired-account:file-success-1"

	msg2ID := uuid.New()
	// Account in pool with dummy credentials: DeleteFile fails with network/dial error
	failedFileID := "dummy-pool-account:file-fail-2"

	now := time.Now().UTC()
	futureExpiry := now.Add(24 * time.Hour)

	// Upfront batch fetch
	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{msg1ID, msg2ID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(msg1ID, targetChatID, callerID.UuidUserId, recipientID, "media", &successFileID, false, futureExpiry).
			AddRow(msg2ID, targetChatID, callerID.UuidUserId, recipientID, "media", &failedFileID, false, futureExpiry))

	// Chat participant check
	mock.ExpectQuery(`(?s)SELECT .* FROM chats WHERE id = \$1 LIMIT 1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, recipientID, int32(2), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// Lock chat row
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, recipientID, int32(2), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	// Single-pass batch message update
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET\s+content = 'Message unsent'.*WHERE\s+id = ANY\(\$1::uuid\[\]\).*RETURNING id`).
		WithArgs([]uuid.UUID{msg1ID, msg2ID}).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).
			AddRow(msg1ID).
			AddRow(msg2ID))

	// Single sync action for recipient with batch payload
	recipientPayloadArg := &capturedArgument{}
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), recipientID, "unsend", recipientPayloadArg).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), recipientID, "unsend", []byte("{}"), false, now, now))

	// Single-pass batch chat preview update
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_last_message_content = 'Message unsent'.*WHERE\s+id = \$1\s+AND last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(targetChatID, []uuid.UUID{msg1ID, msg2ID}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// Single-pass chat unread decrement with Amount = 2
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE id = \$3`).
		WithArgs(recipientID, int32(2), targetChatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// ClearMessagesFileFieldsBatch
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+file_id = NULL.*WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{msg1ID, msg2ID}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 2))

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{msg1ID.String(), msg2ID.String()},
	}, callerID, true)

	require.NoError(t, err)
	assert.Equal(t, targetChatID, out.ChatID)
	assert.Equal(t, recipientID, out.RecipientID)
	assert.ElementsMatch(t, []string{msg1ID.String(), msg2ID.String()}, out.NewlyUnsentIDs)

	payload := recipientPayloadArg.payload(t)
	assert.ElementsMatch(t, []string{msg1ID.String(), msg2ID.String()}, payload.MessageIDs)
	assert.Equal(t, targetChatID.String(), payload.ChatID)

	assert.NoError(t, mock.ExpectationsWereMet())

	// 1. Both files registered in pending_uploads
	spy.mu.Lock()
	assert.ElementsMatch(t, []string{successFileID, failedFileID}, spy.registeredBatchFiles)
	spy.mu.Unlock()

	// 2. Wait for goroutine to process batch R2 deletion (AWS SDK DNS/dial timeout takes up to 5s for failed dummy host)
	assert.Eventually(t, func() bool {
		spy.mu.Lock()
		defer spy.mu.Unlock()
		return len(spy.removedFiles) >= 1
	}, 8*time.Second, 50*time.Millisecond, "successful file must be removed by background goroutine")

	// 3. Verify exactly successFileID was removed, and failedFileID was NOT removed
	spy.mu.Lock()
	assert.Contains(t, spy.removedFiles, successFileID, "successful R2 delete must trigger Remove from pending_uploads")
	assert.NotContains(t, spy.removedFiles, failedFileID, "failed R2 delete must NOT trigger Remove from pending_uploads (preserved for sweeper)")
	spy.mu.Unlock()
}
