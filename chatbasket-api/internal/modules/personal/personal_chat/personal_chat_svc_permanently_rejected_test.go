package personal_chat

import (
	rpc_personal_chatv1 "chatbasket-api/gen/proto/personal/personal_chat"
	"chatbasket-api/internal/platform/kit"
	"context"
	"fmt"
	"log"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// These tests pin the permanentlyRejectedMessageIds contract shared by the five
// batch endpoints:
//   - Rejected: malformed IDs, missing rows, rows the caller neither sent nor
//     received, and expired rows. Expiry is checked before the endpoint's own
//     side rules, so an expired row is rejected even when it is on the wrong side.
//   - Only skipped (in neither list): the caller's own rows that fail the
//     endpoint's rule, and eligible rows the UPDATE did not return because a
//     concurrent request changed them first.
//   - The list keeps request order, uses the request's spelling, and holds
//     each UUID once.
//   - Rejected rows never fail the whole request (no 400 for mixed chats, no
//     403 for someone else's chat).
//   - In MarkChatRead, AcknowledgeAndReadBatch and UnsendMessage, every row
//     that passes the four checks must be in one chat, including the caller's
//     own rows that the endpoint skips or reports as already done. Otherwise
//     the whole request fails with 400 invalid_batch and carries no list. The
//     chat comes from the DB rows only: these requests carry message IDs and
//     nothing else.
//   - AcknowledgeReadReceiptBatch has no one-chat rule. Its request also
//     carries message IDs only, and it checks each row on its own, so the rows
//     may come from any chats.

func newTestCaller() kit.UserId {
	id := uuid.New()
	return kit.UserId{UuidUserId: id, StringUserId: id.String()}
}

func TestAcknowledgeDeliveryBatch_ReportsPermanentlyRejectedIDsInRequestOrder(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	caller := newTestCaller()
	peer := uuid.New()
	chatID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)
	live := now.Add(24 * time.Hour)
	past := now.Add(-time.Hour)

	missingID := uuid.New()
	foreignID := uuid.New()        // caller is neither sender nor recipient
	expiredID := uuid.New()        // caller is the recipient, TTL passed
	expiredOwnSentID := uuid.New() // caller is the sender (wrong side), TTL passed
	ownSentID := uuid.New()        // caller is the sender (wrong side), still live
	alreadyAckedID := uuid.New()   // caller is the recipient, already delivered
	racedID := uuid.New()          // eligible, but another request delivered it first

	reqIDs := []string{
		"not-a-uuid",
		missingID.String(),
		foreignID.String(),
		expiredID.String(),
		expiredOwnSentID.String(),
		ownSentID.String(),
		alreadyAckedID.String(),
		racedID.String(),
		strings.ToUpper(missingID.String()), // same UUID, other spelling
		"not-a-uuid",
	}

	mock.ExpectQuery(deliveryAckPattern).
		WithArgs([]uuid.UUID{missingID, foreignID, expiredID, expiredOwnSentID, ownSentID, alreadyAckedID, racedID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(foreignID, uuid.New(), uuid.New(), uuid.New(), nil, false, false, false, false, nil, false, now, live).
			AddRow(expiredID, chatID, peer, caller.UuidUserId, nil, false, false, false, false, nil, false, now, past).
			AddRow(expiredOwnSentID, chatID, caller.UuidUserId, peer, nil, false, false, false, false, nil, false, now, past).
			AddRow(ownSentID, chatID, caller.UuidUserId, peer, nil, false, false, false, false, nil, false, now, live).
			AddRow(alreadyAckedID, chatID, peer, caller.UuidUserId, nil, true, true, false, false, nil, false, now, live).
			AddRow(racedID, chatID, peer, caller.UuidUserId, nil, false, false, false, false, nil, false, now, live))

	// Only the eligible row reaches the UPDATE, and it comes back empty.
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET.*delivered_to_recipient_primary = TRUE.*WHERE.*`).
		WithArgs(caller.UuidUserId, []uuid.UUID{racedID}).
		WillReturnRows(pgxmock.NewRows(batchAckColumns()))

	res, err := svc.AcknowledgeDeliveryBatch(context.Background(), reqIDs, "recipient", true, caller)
	require.NoError(t, err)

	assert.Equal(t, []string{alreadyAckedID.String()}, res.AcknowledgedMessageIDs)
	assert.Equal(t, []string{
		"not-a-uuid",
		missingID.String(),
		foreignID.String(),
		expiredID.String(),
		expiredOwnSentID.String(),
	}, res.PermanentlyRejectedIDs, "the wrong-side row and the raced row must be in neither list")
	assert.Empty(t, res.DeliveryGroups)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAcknowledgeDeliveryBatch_SenderModeReportsPermanentlyRejectedIDs(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	caller := newTestCaller()
	peer := uuid.New()
	chatID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)
	live := now.Add(24 * time.Hour)
	past := now.Add(-time.Hour)

	ownSentID := uuid.New()        // eligible sender sync
	receivedID := uuid.New()       // caller is the recipient (wrong side)
	foreignID := uuid.New()        // caller is neither
	expiredOwnSentID := uuid.New() // caller is the sender, TTL passed

	mock.ExpectQuery(deliveryAckPattern).
		WithArgs([]uuid.UUID{ownSentID, receivedID, foreignID, expiredOwnSentID}).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(ownSentID, chatID, caller.UuidUserId, peer, nil, false, false, false, false, nil, false, now, live).
			AddRow(receivedID, chatID, peer, caller.UuidUserId, nil, false, false, false, false, nil, false, now, live).
			AddRow(foreignID, uuid.New(), uuid.New(), uuid.New(), nil, false, false, false, false, nil, false, now, live).
			AddRow(expiredOwnSentID, chatID, caller.UuidUserId, peer, nil, false, false, false, false, nil, false, now, past))

	mock.ExpectQuery(`(?s)UPDATE messages\s+SET synced_to_sender_primary = TRUE.*WHERE.*`).
		WithArgs(caller.UuidUserId, []uuid.UUID{ownSentID}).
		WillReturnRows(pgxmock.NewRows(batchAckColumns()).
			AddRow(ownSentID, chatID, caller.UuidUserId, nil, false, true, false, false, now))
	// Detached cleanup for the chat of the newly synced row.
	mock.ExpectExec(`(?s)DELETE FROM messages\s+WHERE\s+chat_id = \$1.*read_acked_by_sender = TRUE`).
		WithArgs(chatID).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	reqIDs := []string{ownSentID.String(), receivedID.String(), foreignID.String(), expiredOwnSentID.String()}
	res, err := svc.AcknowledgeDeliveryBatch(context.Background(), reqIDs, "sender", true, caller)
	require.NoError(t, err)

	assert.Equal(t, []string{ownSentID.String()}, res.AcknowledgedMessageIDs)
	assert.Equal(t, []string{foreignID.String(), expiredOwnSentID.String()}, res.PermanentlyRejectedIDs,
		"the received row fails only the sender rule, so it is skipped, not rejected")
	assert.Empty(t, res.DeliveryGroups)
	require.Eventually(t, func() bool {
		return mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond, "detached cleanup must complete")
}

func TestAcknowledgeDeliveryBatch_AllMalformedIDsAreRejectedWithoutDBAccess(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	res, err := svc.AcknowledgeDeliveryBatch(context.Background(), []string{"bad-1", "", "bad-1", "bad-2"}, "recipient", true, newTestCaller())
	require.NoError(t, err)

	assert.Equal(t, int32(0), res.AcknowledgedCount)
	assert.Empty(t, res.AcknowledgedMessageIDs)
	assert.Equal(t, []string{"bad-1", "", "bad-2"}, res.PermanentlyRejectedIDs)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMarkChatRead_RejectsForeignAndExpiredRowsWithoutFailingTheBatch(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	caller := newTestCaller()
	peer := uuid.New()
	targetChatID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)
	live := now.Add(24 * time.Hour)
	past := now.Add(-time.Hour)
	readAt := now.Add(-time.Minute)

	foreignID := uuid.New()     // another chat, caller is neither
	alreadyReadID := uuid.New() // target chat, caller is the recipient
	expiredID := uuid.New()     // a third chat, caller is the recipient, TTL passed
	missingID := uuid.New()

	mock.ExpectQuery(getMessagesForMarkChatReadPattern).
		WithArgs([]uuid.UUID{foreignID, alreadyReadID, expiredID, missingID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForMarkChatReadColumns()).
			AddRow(foreignID, uuid.New(), uuid.New(), uuid.New(), false, nil, false, now, live).
			AddRow(alreadyReadID, targetChatID, peer, caller.UuidUserId, true, &readAt, false, now, live).
			AddRow(expiredID, uuid.New(), peer, caller.UuidUserId, false, nil, false, now, past))
	mock.ExpectQuery(`(?s)SELECT .* FROM chats WHERE id = \$1 LIMIT 1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, caller.UuidUserId, peer, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	out, err := svc.MarkChatRead(context.Background(), &MarkChatReadPayload{
		MessageIDs: []string{foreignID.String(), alreadyReadID.String(), expiredID.String(), missingID.String()},
	}, caller, true)

	require.NoError(t, err, "rows in other chats that are rejected must not turn the batch into invalid_batch")
	assert.Equal(t, targetChatID, out.ChatID)
	assert.Equal(t, peer, out.OtherUserID)
	require.Len(t, out.Receipts, 1)
	assert.Equal(t, alreadyReadID.String(), out.Receipts[0].MessageId)
	assert.Equal(t, []string{foreignID.String(), expiredID.String(), missingID.String()}, out.PermanentlyRejectedIDs)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMarkChatRead_OnlyForeignRowsAreRejectedWithoutChatLookup(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	caller := newTestCaller()
	foreignID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)

	mock.ExpectQuery(getMessagesForMarkChatReadPattern).
		WithArgs([]uuid.UUID{foreignID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForMarkChatReadColumns()).
			AddRow(foreignID, uuid.New(), uuid.New(), uuid.New(), false, nil, false, now, now.Add(24*time.Hour)))
	// No chat lookup: a foreign row must answer exactly like a missing row.

	out, err := svc.MarkChatRead(context.Background(), &MarkChatReadPayload{
		MessageIDs: []string{foreignID.String()},
	}, caller, true)

	require.NoError(t, err, "a foreign message must not produce a 403 that reveals it exists")
	assert.Equal(t, uuid.Nil, out.ChatID)
	assert.Empty(t, out.Receipts)
	assert.Empty(t, out.NewlyRead)
	assert.True(t, out.ReadAt.IsZero())
	assert.Equal(t, []string{foreignID.String()}, out.PermanentlyRejectedIDs)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMarkChatRead_DedupesRepeatedIDs(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	caller := newTestCaller()
	peer := uuid.New()
	targetChatID := uuid.New()
	alreadyReadID := uuid.New()
	missingID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)
	readAt := now.Add(-time.Minute)

	mock.ExpectQuery(getMessagesForMarkChatReadPattern).
		WithArgs([]uuid.UUID{alreadyReadID, missingID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForMarkChatReadColumns()).
			AddRow(alreadyReadID, targetChatID, peer, caller.UuidUserId, true, &readAt, false, now, now.Add(24*time.Hour)))
	mock.ExpectQuery(`(?s)SELECT .* FROM chats WHERE id = \$1 LIMIT 1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, caller.UuidUserId, peer, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	out, err := svc.MarkChatRead(context.Background(), &MarkChatReadPayload{
		MessageIDs: []string{
			alreadyReadID.String(),
			alreadyReadID.String(),
			strings.ToUpper(alreadyReadID.String()),
			missingID.String(),
			strings.ToUpper(missingID.String()),
		},
	}, caller, true)

	require.NoError(t, err)
	require.Len(t, out.Receipts, 1, "a repeated ID must produce one receipt")
	assert.Equal(t, alreadyReadID.String(), out.Receipts[0].MessageId)
	assert.Equal(t, []string{missingID.String()}, out.PermanentlyRejectedIDs, "a repeated missing ID must be reported once")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAcknowledgeAndReadBatch_RejectsForeignAndExpiredRowsWithoutFailingTheBatch(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	caller := newTestCaller()
	peer := uuid.New()
	targetChatID := uuid.New()
	now := time.Now().UTC()
	live := now.Add(24 * time.Hour)
	past := now.Add(-time.Hour)
	readAt := now.Add(-time.Minute)

	foreignID := uuid.New()     // another chat, caller is neither
	alreadyReadID := uuid.New() // target chat, caller is the recipient
	expiredID := uuid.New()     // a third chat, caller is the recipient, TTL passed
	missingID := uuid.New()
	ownSentID := uuid.New() // target chat, caller is the sender (wrong side)

	mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{foreignID, alreadyReadID, expiredID, missingID, ownSentID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForAckAndReadBatchColumns()).
			AddRow(foreignID, uuid.New(), uuid.New(), uuid.New(), nil, true, true, true, false, false, nil, now, live).
			AddRow(alreadyReadID, targetChatID, peer, caller.UuidUserId, nil, true, true, false, true, false, &readAt, now, live).
			AddRow(expiredID, uuid.New(), peer, caller.UuidUserId, nil, true, true, true, false, false, nil, now, past).
			AddRow(ownSentID, targetChatID, caller.UuidUserId, peer, nil, true, true, true, false, false, nil, now, live))

	out, err := svc.AcknowledgeAndReadBatch(context.Background(), &AckAndReadBatchPayload{
		MessageIDs: []string{foreignID.String(), alreadyReadID.String(), expiredID.String(), missingID.String(), ownSentID.String()},
	}, caller, true)

	require.NoError(t, err, "rows in other chats that are rejected must not turn the batch into invalid_batch")
	assert.Equal(t, targetChatID, out.ChatID)
	require.Len(t, out.Receipts, 1)
	assert.Equal(t, alreadyReadID.String(), out.Receipts[0].MessageId)
	assert.Empty(t, out.NewlyRead)
	assert.Equal(t, []string{foreignID.String(), expiredID.String(), missingID.String()}, out.PermanentlyRejectedIDs,
		"the caller's own sent message is skipped, not rejected")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAcknowledgeReadReceiptBatch_ClassifiesRejectedAndSkippedIDs(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	caller := newTestCaller()
	peer := uuid.New()
	chatID := uuid.New()
	now := time.Now().UTC()
	live := now.Add(24 * time.Hour)
	past := now.Add(-time.Hour)
	readAt := now.Add(-time.Minute)

	missingID := uuid.New()
	foreignID := uuid.New()         // caller is neither
	expiredReceivedID := uuid.New() // caller is the recipient (wrong side), TTL passed
	receivedID := uuid.New()        // caller is the recipient (wrong side), live
	unreadID := uuid.New()          // caller's sent message, not read yet
	racedID := uuid.New()           // eligible, but another request ACKed it first
	alreadyAckedID := uuid.New()    // caller's sent message, already ACKed

	lookupIDs := []uuid.UUID{missingID, foreignID, expiredReceivedID, receivedID, unreadID, racedID, alreadyAckedID}
	// Columns: id, chat_id, sender_id, recipient_id, file_id,
	// delivered_to_recipient_primary, synced_to_sender_primary,
	// read_by_recipient, read_at, read_acked_by_sender,
	// created_at, expires_at
	mock.ExpectQuery(readAckUpfrontPattern).
		WithArgs(lookupIDs).
		WillReturnRows(pgxmock.NewRows(readAckExtendedColumns()).
			AddRow(foreignID, uuid.New(), uuid.New(), uuid.New(), nil, false, true, true, &readAt, false, now, live).
			AddRow(expiredReceivedID, chatID, peer, caller.UuidUserId, nil, false, true, true, &readAt, false, now, past).
			AddRow(receivedID, chatID, peer, caller.UuidUserId, nil, false, true, true, &readAt, false, now, live).
			AddRow(unreadID, chatID, caller.UuidUserId, peer, nil, false, true, false, nil, false, now, live).
			AddRow(racedID, chatID, caller.UuidUserId, peer, nil, false, true, true, &readAt, false, now, live).
			AddRow(alreadyAckedID, chatID, caller.UuidUserId, peer, nil, false, true, true, &readAt, true, now, live))

	// No chat lookup: the UPDATE is scoped by the caller and the IDs only.
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET\s+read_acked_by_sender = TRUE\s+WHERE.*expires_at > now\(\)`).
		WithArgs(caller.UuidUserId, []uuid.UUID{racedID}).
		WillReturnRows(pgxmock.NewRows(markReadAckedReturningColumns()))

	reqIDs := []string{"bad"}
	for _, id := range lookupIDs {
		reqIDs = append(reqIDs, id.String())
	}

	res, err := svc.AcknowledgeReadReceiptBatch(context.Background(), &AckReadReceiptBatchPayload{
		MessageIDs: reqIDs,
	}, caller, true)
	require.NoError(t, err)

	assert.Equal(t, []string{alreadyAckedID.String()}, res.AcknowledgedMessageIds)
	assert.Equal(t, []string{
		"bad",
		missingID.String(),
		foreignID.String(),
		expiredReceivedID.String(),
	}, res.PermanentlyRejectedMessageIds, "wrong side, unread, and raced rows are only skipped")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUnsendMessage_OnlyForeignRowsAreRejectedWithoutChatLookup(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	caller := newTestCaller()
	foreignID := uuid.New()
	live := time.Now().Add(24 * time.Hour)

	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{foreignID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(foreignID, uuid.New(), uuid.New(), uuid.New(), "text", nil, false, live))
	// No chat lookup: a foreign row must answer exactly like a missing row.

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{foreignID.String()},
	}, caller, true)

	require.NoError(t, err, "a foreign message must not produce a 403 that reveals it exists")
	assert.Equal(t, uuid.Nil, out.ChatID)
	assert.Empty(t, out.AcknowledgedIDs)
	assert.Empty(t, out.NewlyUnsentIDs)
	assert.Equal(t, []string{foreignID.String()}, out.PermanentlyRejectedIDs)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUnsendMessage_RejectsForeignAndExpiredRowsWithoutFailingTheBatch(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	caller := newTestCaller()
	peer := uuid.New()
	targetChatID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)
	live := now.Add(24 * time.Hour)
	past := now.Add(-time.Hour)

	foreignID := uuid.New()       // another chat, caller is neither
	alreadyUnsentID := uuid.New() // target chat, caller's own, already unsent
	expiredOwnID := uuid.New()    // a third chat, caller's own, TTL passed
	receivedID := uuid.New()      // target chat, caller is the recipient (wrong side)
	readOwnID := uuid.New()       // target chat, caller's own, already read
	missingID := uuid.New()

	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{foreignID, alreadyUnsentID, expiredOwnID, receivedID, readOwnID, missingID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(foreignID, uuid.New(), uuid.New(), uuid.New(), "text", nil, false, live).
			AddRow(alreadyUnsentID, targetChatID, caller.UuidUserId, peer, "unsent", nil, false, live).
			AddRow(expiredOwnID, uuid.New(), caller.UuidUserId, peer, "text", nil, false, past).
			AddRow(receivedID, targetChatID, peer, caller.UuidUserId, "text", nil, false, live).
			AddRow(readOwnID, targetChatID, caller.UuidUserId, peer, "text", nil, true, live))
	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, caller.UuidUserId, peer, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))
	// Nothing is eligible and no file needs cleanup, so no transaction runs.

	out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
		MessageIDs: []string{
			foreignID.String(),
			alreadyUnsentID.String(),
			expiredOwnID.String(),
			receivedID.String(),
			readOwnID.String(),
			missingID.String(),
		},
	}, caller, true)

	require.NoError(t, err, "rows in other chats that are rejected must not turn the batch into invalid_batch")
	assert.Equal(t, targetChatID, out.ChatID)
	assert.Equal(t, peer, out.RecipientID)
	assert.Equal(t, []string{alreadyUnsentID.String()}, out.AcknowledgedIDs)
	assert.Empty(t, out.NewlyUnsentIDs)
	assert.Equal(t, []string{foreignID.String(), expiredOwnID.String(), missingID.String()}, out.PermanentlyRejectedIDs,
		"the received and the read rows are the caller's own rows that fail the unsend rule, so they are only skipped")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ──────────────────────────────────────────────────────────────────────────────
// One-chat rule for the rows that pass the four rejection checks
// ──────────────────────────────────────────────────────────────────────────────
//
// Each case below puts one kept row (it passes the four checks) in chat B and
// an eligible row in chat A, in both request orders, after a missing ID. The
// kept row in chat B is one the endpoint would only skip or report as already
// done, in the caller's chat with someone else. It must still fail the whole
// request with 400 invalid_batch, before any chat lookup or write, and with no
// rejected list.

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

func requireInvalidBatch(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	pe, ok := err.(kit.ProcessedError)
	require.True(t, ok, "want kit.ProcessedError, got %T", err)
	assert.Equal(t, 400, pe.Status())
	assert.Equal(t, "invalid_batch", pe.Kind())
}

// keptRowCase builds the kept row in chat B from its id, chat, caller and peer.
type keptRowCase struct {
	name string
	row  func(id, chatID, caller, peer uuid.UUID) []any
}

// forEachOrder runs fn with the kept row placed after, then before, the
// eligible row. A missing ID always comes first.
func forEachOrder(t *testing.T, cases []keptRowCase, fn func(t *testing.T, tc keptRowCase, ids []uuid.UUID, missingID, eligibleID, keptID uuid.UUID)) {
	for _, tc := range cases {
		for _, keptFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/keptFirst=%v", tc.name, keptFirst), func(t *testing.T) {
				missingID, eligibleID, keptID := uuid.New(), uuid.New(), uuid.New()
				ids := []uuid.UUID{missingID, eligibleID, keptID}
				if keptFirst {
					ids = []uuid.UUID{missingID, keptID, eligibleID}
				}
				fn(t, tc, ids, missingID, eligibleID, keptID)
			})
		}
	}
}

func TestMarkChatRead_KeptRowInAnotherChatFailsTheWholeRequest(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)
	live := now.Add(24 * time.Hour)
	readAt := now.Add(-time.Minute)

	// Columns: id, chat_id, sender_id, recipient_id, read_by_recipient,
	// read_at, read_acked_by_sender, created_at, expires_at
	cases := []keptRowCase{
		{"caller sent it, so mark-read skips it", func(id, chatID, caller, peer uuid.UUID) []any {
			return []any{id, chatID, caller, peer, false, nil, false, now, live}
		}},
		{"caller received it and already read it, so it is done", func(id, chatID, caller, peer uuid.UUID) []any {
			return []any{id, chatID, peer, caller, true, &readAt, false, now, live}
		}},
	}

	forEachOrder(t, cases, func(t *testing.T, tc keptRowCase, ids []uuid.UUID, _, eligibleID, keptID uuid.UUID) {
		mock, svc := newChatMockSvc(t, false)
		caller := newTestCaller()
		peer := uuid.New()
		chatA, chatB := uuid.New(), uuid.New()

		mock.ExpectQuery(getMessagesForMarkChatReadPattern).
			WithArgs(ids).
			WillReturnRows(pgxmock.NewRows(getMessagesForMarkChatReadColumns()).
				AddRow(eligibleID, chatA, peer, caller.UuidUserId, false, nil, false, now, live).
				AddRow(tc.row(keptID, chatB, caller.UuidUserId, uuid.New())...))
		// No chat lookup and no write may follow the message lookup.

		out, err := svc.MarkChatRead(context.Background(), &MarkChatReadPayload{
			MessageIDs: uuidStrings(ids),
		}, caller, true)

		requireInvalidBatch(t, err)
		assert.Empty(t, out.Receipts)
		assert.Empty(t, out.NewlyRead)
		assert.Equal(t, uuid.Nil, out.ChatID)
		assert.Nil(t, out.PermanentlyRejectedIDs, "a failed request carries no rejected list")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestAcknowledgeAndReadBatch_KeptRowInAnotherChatFailsTheWholeRequest(t *testing.T) {
	now := time.Now().UTC()
	live := now.Add(24 * time.Hour)
	readAt := now.Add(-time.Minute)

	// Columns: id, chat_id, sender_id, recipient_id, file_id,
	// delivered_to_recipient, delivered_to_recipient_primary,
	// synced_to_sender_primary,
	// read_by_recipient, read_acked_by_sender, read_at, created_at, expires_at
	cases := []keptRowCase{
		{"caller sent it, so ack-and-read skips it", func(id, chatID, caller, peer uuid.UUID) []any {
			return []any{id, chatID, caller, peer, nil, true, true, true, false, false, nil, now, live}
		}},
		{"caller received it and already read it, so it is done", func(id, chatID, caller, peer uuid.UUID) []any {
			return []any{id, chatID, peer, caller, nil, true, true, false, true, false, &readAt, now, live}
		}},
	}

	forEachOrder(t, cases, func(t *testing.T, tc keptRowCase, ids []uuid.UUID, _, eligibleID, keptID uuid.UUID) {
		mock, svc := newChatMockSvc(t, false)
		caller := newTestCaller()
		peer := uuid.New()
		chatA, chatB := uuid.New(), uuid.New()

		mock.ExpectQuery(`(?s)SELECT .* FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`).
			WithArgs(ids).
			WillReturnRows(pgxmock.NewRows(getMessagesForAckAndReadBatchColumns()).
				AddRow(eligibleID, chatA, peer, caller.UuidUserId, nil, false, false, true, false, false, nil, now, live).
				AddRow(tc.row(keptID, chatB, caller.UuidUserId, uuid.New())...))
		// No chat lock and no write may follow the message lookup.

		out, err := svc.AcknowledgeAndReadBatch(context.Background(), &AckAndReadBatchPayload{
			MessageIDs: uuidStrings(ids),
		}, caller, true)

		requireInvalidBatch(t, err)
		assert.Empty(t, out.Receipts)
		assert.Empty(t, out.NewlyRead)
		assert.Equal(t, uuid.Nil, out.ChatID)
		assert.Nil(t, out.PermanentlyRejectedIDs, "a failed request carries no rejected list")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestUnsendMessage_KeptRowInAnotherChatFailsTheWholeRequest(t *testing.T) {
	live := time.Now().Add(24 * time.Hour)

	// Columns: id, chat_id, sender_id, recipient_id, message_type, file_id,
	// read_by_recipient, expires_at
	cases := []keptRowCase{
		{"caller received it, so unsend skips it", func(id, chatID, caller, peer uuid.UUID) []any {
			return []any{id, chatID, peer, caller, "text", nil, false, live}
		}},
		{"caller sent it and the recipient read it, so unsend skips it", func(id, chatID, caller, peer uuid.UUID) []any {
			return []any{id, chatID, caller, peer, "text", nil, true, live}
		}},
		{"caller sent it and already unsent it, so it is done", func(id, chatID, caller, peer uuid.UUID) []any {
			return []any{id, chatID, caller, peer, "unsent", nil, false, live}
		}},
	}

	forEachOrder(t, cases, func(t *testing.T, tc keptRowCase, ids []uuid.UUID, _, eligibleID, keptID uuid.UUID) {
		mock, svc := newChatMockSvc(t, false)
		caller := newTestCaller()
		peer := uuid.New()
		chatA, chatB := uuid.New(), uuid.New()

		mock.ExpectQuery(getMessagesForUnsendBatchPattern).
			WithArgs(ids).
			WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
				AddRow(eligibleID, chatA, caller.UuidUserId, peer, "text", nil, false, live).
				AddRow(tc.row(keptID, chatB, caller.UuidUserId, uuid.New())...))
		// No chat lookup and no write may follow the message lookup.

		out, err := svc.UnsendMessageHandler(context.Background(), &UnsendMessagePayload{
			MessageIDs: uuidStrings(ids),
		}, caller, true)

		requireInvalidBatch(t, err)
		assert.Empty(t, out.AcknowledgedIDs)
		assert.Empty(t, out.NewlyUnsentIDs)
		assert.Equal(t, uuid.Nil, out.ChatID)
		assert.Equal(t, uuid.Nil, out.RecipientID)
		assert.Nil(t, out.PermanentlyRejectedIDs, "a failed request carries no rejected list")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// AcknowledgeReadReceiptBatch: rows from any chats, each checked on its own
// ──────────────────────────────────────────────────────────────────────────────
//
// No one-chat rule and no chat lookup. The UPDATE is scoped by the caller's
// sender_id and the IDs. As in AcknowledgeDeliveryBatch, the chat-wide cleanup
// then runs once for the chat of every row the call ACKs now or retries relay
// cleanup for. Only rows the caller sent get that far, so these are always the
// caller's own chats.

func TestAcknowledgeReadReceiptBatch_AcceptsRowsFromDifferentChats(t *testing.T) {
	now := time.Now().UTC()
	live := now.Add(24 * time.Hour)
	readAt := now.Add(-time.Minute)

	// The kept row is in chat B and an eligible row is in chat A: the caller's
	// chats with two different people. No row is on the recipient's primary
	// device yet, so no row gets a relay delete of its own. The chat of every
	// row the UPDATE ACKs is still swept, once.
	// Columns: id, chat_id, sender_id, recipient_id, file_id,
	// delivered_to_recipient_primary, synced_to_sender_primary,
	// read_by_recipient, read_at, read_acked_by_sender,
	// created_at, expires_at
	cases := []struct {
		name    string
		row     func(id, chatID, caller, peer uuid.UUID) []any
		updated bool // the kept row is ACKed by the same UPDATE as the eligible row
		done    bool // the kept row is confirmed with no write
	}{
		{"caller sent it and the recipient read it, so it is ACKed too", func(id, chatID, caller, peer uuid.UUID) []any {
			return []any{id, chatID, caller, peer, nil, false, true, true, &readAt, false, now, live}
		}, true, false},
		{"caller sent it and already ACKed the read, so it is confirmed again", func(id, chatID, caller, peer uuid.UUID) []any {
			return []any{id, chatID, caller, peer, nil, false, true, true, &readAt, true, now, live}
		}, false, true},
		{"caller received it, so ack-read skips it", func(id, chatID, caller, peer uuid.UUID) []any {
			return []any{id, chatID, peer, caller, nil, false, true, true, &readAt, false, now, live}
		}, false, false},
		{"caller sent it and it is not read yet, so ack-read skips it", func(id, chatID, caller, peer uuid.UUID) []any {
			return []any{id, chatID, caller, peer, nil, false, true, false, nil, false, now, live}
		}, false, false},
	}

	for _, tc := range cases {
		for _, keptFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/keptFirst=%v", tc.name, keptFirst), func(t *testing.T) {
				mock, svc := newChatMockSvc(t, false)
				// The sweeps run in a goroutine, and their order is not part of the contract.
				mock.MatchExpectationsInOrder(false)
				caller := newTestCaller()
				chatA, chatB := uuid.New(), uuid.New()
				missingID, eligibleID, keptID := uuid.New(), uuid.New(), uuid.New()
				ids := []uuid.UUID{missingID, eligibleID, keptID}
				if keptFirst {
					ids = []uuid.UUID{missingID, keptID, eligibleID}
				}

				mock.ExpectQuery(readAckUpfrontPattern).
					WithArgs(ids).
					WillReturnRows(pgxmock.NewRows(readAckExtendedColumns()).
						AddRow(eligibleID, chatA, caller.UuidUserId, uuid.New(), nil, false, true, true, &readAt, false, now, live).
						AddRow(tc.row(keptID, chatB, caller.UuidUserId, uuid.New())...))

				// One UPDATE covers both chats, in request order. No chat lookup.
				var updateIDs []uuid.UUID
				var wantConfirmed []string
				updated := pgxmock.NewRows(markReadAckedReturningColumns())
				for _, id := range ids {
					switch {
					case id == eligibleID:
						updateIDs = append(updateIDs, id)
						updated.AddRow(id, chatA, caller.UuidUserId, nil, false, true, true, true, now)
						wantConfirmed = append(wantConfirmed, id.String())
					case id == keptID && tc.updated:
						updateIDs = append(updateIDs, id)
						updated.AddRow(id, chatB, caller.UuidUserId, nil, false, true, true, true, now)
						wantConfirmed = append(wantConfirmed, id.String())
					case id == keptID && tc.done:
						wantConfirmed = append(wantConfirmed, id.String())
					}
				}
				mock.ExpectQuery(`(?s)UPDATE messages\s+SET\s+read_acked_by_sender = TRUE\s+WHERE.*expires_at > now\(\)`).
					WithArgs(caller.UuidUserId, updateIDs).
					WillReturnRows(updated)
				// One sweep for each chat with a row ACKed now: chat A always,
				// chat B only when its kept row is ACKed too.
				sweepChats := []uuid.UUID{chatA}
				if tc.updated {
					sweepChats = append(sweepChats, chatB)
				}
				for _, chatID := range sweepChats {
					mock.ExpectExec(`(?s)DELETE FROM messages\s+WHERE\s+chat_id = \$1.*read_acked_by_sender = TRUE`).
						WithArgs(chatID).
						WillReturnResult(pgxmock.NewResult("DELETE", 0))
				}

				res, err := svc.AcknowledgeReadReceiptBatch(context.Background(), &AckReadReceiptBatchPayload{
					MessageIDs: uuidStrings(ids),
				}, caller, true)

				require.NoError(t, err, "rows from two chats must not fail the request")
				assert.Equal(t, int32(len(wantConfirmed)), res.AcknowledgedCount)
				assert.Equal(t, wantConfirmed, res.AcknowledgedMessageIds, "confirmed IDs keep request order")
				assert.Equal(t, []string{missingID.String()}, res.PermanentlyRejectedMessageIds)
				require.Eventually(t, func() bool {
					return mock.ExpectationsWereMet() == nil
				}, 5*time.Second, 10*time.Millisecond, "the chat of every row ACKed now must be swept")
			})
		}
	}
}

func TestAcknowledgeReadReceiptBatch_UpdateIsScopedBySenderOnly(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	caller := newTestCaller()
	peer := uuid.New()
	chatID := uuid.New()
	msgID := uuid.New()
	now := time.Now().UTC()
	readAt := now.Add(-time.Minute)

	// 1. The message lookup is the only read. The recipient read it on a
	// secondary device, so it is not on the recipient's primary yet: it gets no
	// relay delete of its own, but its chat is still swept (step 3).
	mock.ExpectQuery(readAckUpfrontPattern).
		WithArgs([]uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(readAckExtendedColumns()).
			AddRow(msgID, chatID, caller.UuidUserId, peer, nil, false, true, true, &readAt, false, now, now.Add(24*time.Hour)))
	// 2. No chat lookup follows, and the UPDATE names no chat: the caller's
	// sender_id and the IDs are its only scope.
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET\s+read_acked_by_sender = TRUE\s+WHERE.*expires_at > now\(\)`).
		WithArgs(caller.UuidUserId, []uuid.UUID{msgID}).
		WillReturnRows(pgxmock.NewRows(markReadAckedReturningColumns()).
			AddRow(msgID, chatID, caller.UuidUserId, nil, false, true, true, true, now))
	// 3. The async sweep of the ACKed row's chat.
	mock.ExpectExec(`(?s)DELETE FROM messages\s+WHERE\s+chat_id = \$1.*read_acked_by_sender = TRUE`).
		WithArgs(chatID).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	res, err := svc.AcknowledgeReadReceiptBatch(context.Background(), &AckReadReceiptBatchPayload{
		MessageIDs: []string{msgID.String()},
	}, caller, true)

	require.NoError(t, err)
	assert.Equal(t, int32(1), res.AcknowledgedCount)
	assert.Equal(t, []string{msgID.String()}, res.AcknowledgedMessageIds)
	assert.Empty(t, res.PermanentlyRejectedMessageIds)
	require.Eventually(t, func() bool {
		return mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond, "the ACKed row's chat must be swept")
}

func TestAcknowledgeReadReceiptBatch_OnlyRejectedRowsAreAnsweredWithoutChatLookup(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	caller := newTestCaller()
	now := time.Now().UTC()
	readAt := now.Add(-time.Minute)

	foreignID := uuid.New() // another chat, caller is neither
	expiredID := uuid.New() // a third chat, caller's sent message, TTL passed
	missingID := uuid.New()

	mock.ExpectQuery(readAckUpfrontPattern).
		WithArgs([]uuid.UUID{foreignID, expiredID, missingID}).
		WillReturnRows(pgxmock.NewRows(readAckExtendedColumns()).
			AddRow(foreignID, uuid.New(), uuid.New(), uuid.New(), nil, true, true, true, &readAt, false, now, now.Add(24*time.Hour)).
			AddRow(expiredID, uuid.New(), caller.UuidUserId, uuid.New(), nil, true, true, true, &readAt, false, now, now.Add(-time.Hour)))
	// No chat lookup: rejected rows in different chats never fail the request,
	// and a foreign row answers exactly like a missing one.

	res, err := svc.AcknowledgeReadReceiptBatch(context.Background(), &AckReadReceiptBatchPayload{
		MessageIDs: []string{"bad", foreignID.String(), expiredID.String(), missingID.String()},
	}, caller, true)

	require.NoError(t, err)
	assert.Equal(t, int32(0), res.AcknowledgedCount)
	assert.Empty(t, res.AcknowledgedMessageIds)
	assert.Equal(t, []string{"bad", foreignID.String(), expiredID.String(), missingID.String()}, res.PermanentlyRejectedMessageIds)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAcknowledgeReadReceiptBatch_SweepsTheChatOfEveryAckedRowOnce(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)
	// The sweeps run in a goroutine, and their order is not part of the contract.
	mock.MatchExpectationsInOrder(false)

	safeBuf := &safeLogBuffer{}
	origOutput := log.Writer()
	log.SetOutput(safeBuf)
	defer log.SetOutput(origOutput)

	caller := newTestCaller()
	peerA := uuid.New()
	chatA, chatB, chatC, chatD, chatE, chatF, chatG := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	// Swept, once per chat:
	a1, a2 := uuid.New(), uuid.New() // chat A: ACKed now, both primary devices hold them
	b1 := uuid.New()                 // chat B: ACKed before, both primary devices hold it (cleanup retry)
	c1 := uuid.New()                 // chat C: ACKed now, not on the recipient's primary yet
	// Not swept:
	d1 := uuid.New() // chat D: ACKed before, not on the recipient's primary yet: confirmed, nothing to clean
	e1 := uuid.New() // chat E: the caller received it: skipped
	f1 := uuid.New() // chat F: the caller sent it, not read yet: skipped
	g1 := uuid.New() // chat G: someone else's message: rejected
	now := time.Now().UTC()
	live := now.Add(24 * time.Hour)
	readAt := now.Add(-time.Minute)

	ids := []uuid.UUID{a1, b1, c1, d1, e1, f1, g1, a2}
	// Columns: id, chat_id, sender_id, recipient_id, file_id,
	// delivered_to_recipient_primary, synced_to_sender_primary,
	// read_by_recipient, read_at, read_acked_by_sender,
	// created_at, expires_at
	mock.ExpectQuery(readAckUpfrontPattern).
		WithArgs(ids).
		WillReturnRows(pgxmock.NewRows(readAckExtendedColumns()).
			AddRow(a1, chatA, caller.UuidUserId, peerA, nil, true, true, true, &readAt, false, now, live).
			AddRow(b1, chatB, caller.UuidUserId, uuid.New(), nil, true, true, true, &readAt, true, now, live).
			AddRow(c1, chatC, caller.UuidUserId, uuid.New(), nil, false, true, true, &readAt, false, now, live).
			AddRow(d1, chatD, caller.UuidUserId, uuid.New(), nil, false, true, true, &readAt, true, now, live).
			AddRow(e1, chatE, uuid.New(), caller.UuidUserId, nil, true, true, true, &readAt, false, now, live).
			AddRow(f1, chatF, caller.UuidUserId, uuid.New(), nil, true, true, false, nil, false, now, live).
			AddRow(g1, chatG, uuid.New(), uuid.New(), nil, true, true, true, &readAt, false, now, live).
			AddRow(a2, chatA, caller.UuidUserId, peerA, nil, true, true, true, &readAt, false, now, live))
	mock.ExpectQuery(`(?s)UPDATE messages\s+SET\s+read_acked_by_sender = TRUE\s+WHERE.*expires_at > now\(\)`).
		WithArgs(caller.UuidUserId, []uuid.UUID{a1, c1, a2}).
		WillReturnRows(pgxmock.NewRows(markReadAckedReturningColumns()).
			AddRow(a1, chatA, caller.UuidUserId, nil, true, true, true, true, now).
			AddRow(c1, chatC, caller.UuidUserId, nil, false, true, true, true, now).
			AddRow(a2, chatA, caller.UuidUserId, nil, true, true, true, true, now))
	// As in AcknowledgeDeliveryBatch: one sweep for the chat of every row ACKed
	// now (chats A and C, ready for relay cleanup or not) and of every row whose
	// relay cleanup is retried (chat B).
	for _, chatID := range []uuid.UUID{chatA, chatB, chatC} {
		mock.ExpectExec(`(?s)DELETE FROM messages\s+WHERE\s+chat_id = \$1.*read_acked_by_sender = TRUE`).
			WithArgs(chatID).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))
	}

	res, err := svc.AcknowledgeReadReceiptBatch(context.Background(), &AckReadReceiptBatchPayload{
		MessageIDs: uuidStrings(ids),
	}, caller, true)

	require.NoError(t, err, "rows from seven chats must not fail the request")
	assert.Equal(t, uuidStrings([]uuid.UUID{a1, b1, c1, d1, a2}), res.AcknowledgedMessageIds)
	assert.Equal(t, []string{g1.String()}, res.PermanentlyRejectedMessageIds)
	require.Eventually(t, func() bool {
		return mock.ExpectationsWereMet() == nil
	}, 5*time.Second, 10*time.Millisecond, "chats A, B and C must each be swept")
	// pgxmock fails a sweep it does not expect, and the cleanup logs the
	// failure. A sweep of chat D, E, F or G, or a second one of chat A, would
	// show here.
	require.Never(t, func() bool {
		return strings.Contains(safeBuf.String(), "CleanupFullyAcknowledgedReadMessagesInChat ERROR")
	}, 200*time.Millisecond, 10*time.Millisecond, "no other sweep may run")
}

// ──────────────────────────────────────────────────────────────────────────────
// The requests carry message IDs only
// ──────────────────────────────────────────────────────────────────────────────

// TestChatBatchRequestsCarryOnlyMessageIDs pins the request shape of the four
// batch endpoints above on both transports: message IDs and nothing else, so a
// client can never name a chat.
func TestChatBatchRequestsCarryOnlyMessageIDs(t *testing.T) {
	for _, msg := range []proto.Message{
		&rpc_personal_chatv1.MarkChatReadRequest{},
		&rpc_personal_chatv1.AckAndReadBatchPayload{},
		&rpc_personal_chatv1.UnsendMessageRequest{},
		&rpc_personal_chatv1.AckReadReceiptBatchRequest{},
	} {
		desc := msg.ProtoReflect().Descriptor()
		var names []string
		for i := 0; i < desc.Fields().Len(); i++ {
			names = append(names, string(desc.Fields().Get(i).Name()))
		}
		assert.Equal(t, []string{"messageIds"}, names, "ConnectRPC %s", desc.Name())
	}

	for _, payload := range []any{
		MarkChatReadPayload{},
		AckAndReadBatchPayload{},
		UnsendMessagePayload{},
		AckReadReceiptBatchPayload{},
	} {
		typ := reflect.TypeOf(payload)
		var tags []string
		for i := 0; i < typ.NumField(); i++ {
			tags = append(tags, strings.Split(typ.Field(i).Tag.Get("json"), ",")[0])
		}
		assert.Equal(t, []string{"messageIds"}, tags, "REST %s", typ.Name())
	}
}
