package personal_chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"chatbasket-api/internal/platform/kit"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func deleteForMeBatchColumns() []string {
	return []string{
		"id",
		"chat_id",
		"sender_id",
		"recipient_id",
		"file_id",
		"delivered_to_recipient_primary",
		"synced_to_sender_primary",
		"read_by_recipient",
		"read_acked_by_sender",
		"deleted_by_sender",
		"deleted_by_recipient",
		"expires_at",
	}
}

var futureExpiry = time.Now().UTC().Add(24 * time.Hour)

func TestDeleteMessageForMe_BatchExecutionPrimary(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUserID := uuid.New()
	chatID := uuid.New()

	msg1Sender := uuid.New()
	msg2Recipient := uuid.New()
	allIDs := []uuid.UUID{msg1Sender, msg2Recipient}

	// 1. GetMessagesForDeleteForMeBatch
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(msg1Sender, chatID, userID.UuidUserId, otherUserID, nil, true, true, false, false, false, false, futureExpiry).
		AddRow(msg2Recipient, chatID, otherUserID, userID.UuidUserId, nil, true, true, false, false, false, false, futureExpiry)

	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(rows)

	// 2. ClearLastMessageForParticipantBatch
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(userID.UuidUserId, allIDs).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 3. MarkMessagesDeletedBySenderBatch (only for msg1Sender)
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_sender = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+sender_id = \$2\s+AND\s+deleted_by_sender = FALSE`).
		WithArgs([]uuid.UUID{msg1Sender}, userID.UuidUserId).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 4. MarkMessagesDeletedByRecipientBatch (only for msg2Recipient)
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_recipient = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+recipient_id = \$2\s+AND\s+deleted_by_recipient = FALSE`).
		WithArgs([]uuid.UUID{msg2Recipient}, userID.UuidUserId).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// When isPrimary=true, CreateSyncActionsBatch must NOT be called.

	groups, err := svc.DeleteMessageForMe(context.Background(), allIDs, userID, true)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, chatID, groups[0].ChatID)
	require.ElementsMatch(t, []string{msg1Sender.String(), msg2Recipient.String()}, groups[0].MessageIDs)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_BatchExecutionSecondary(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUserID := uuid.New()
	chatID := uuid.New()

	msg1Sender := uuid.New()
	msg2Recipient := uuid.New()
	allIDs := []uuid.UUID{msg1Sender, msg2Recipient}

	// 1. GetMessagesForDeleteForMeBatch
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(msg1Sender, chatID, userID.UuidUserId, otherUserID, nil, true, true, false, false, false, false, futureExpiry).
		AddRow(msg2Recipient, chatID, otherUserID, userID.UuidUserId, nil, true, true, false, false, false, false, futureExpiry)

	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(rows)

	// 2. ClearLastMessageForParticipantBatch
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(userID.UuidUserId, allIDs).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 3. MarkMessagesDeletedBySenderBatch
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_sender = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+sender_id = \$2\s+AND\s+deleted_by_sender = FALSE`).
		WithArgs([]uuid.UUID{msg1Sender}, userID.UuidUserId).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 4. MarkMessagesDeletedByRecipientBatch
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_recipient = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+recipient_id = \$2\s+AND\s+deleted_by_recipient = FALSE`).
		WithArgs([]uuid.UUID{msg2Recipient}, userID.UuidUserId).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 5. CreateSyncActionsBatch (since isPrimary=false)
	mock.ExpectExec(`(?s)INSERT INTO message_sync_actions.*FROM unnest\(\$2::uuid\[\]\)`).
		WithArgs(userID.UuidUserId, allIDs).
		WillReturnResult(pgxmock.NewResult("INSERT", 2))

	groups, err := svc.DeleteMessageForMe(context.Background(), allIDs, userID, false)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, chatID, groups[0].ChatID)
	require.ElementsMatch(t, []string{msg1Sender.String(), msg2Recipient.String()}, groups[0].MessageIDs)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_ConditionsNotMet_SkipsRelayCleanup(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUserID := uuid.New()
	chatID := uuid.New()

	msg1Sender := uuid.New()
	msg2Recipient := uuid.New()
	allIDs := []uuid.UUID{msg1Sender, msg2Recipient}

	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(msg1Sender, chatID, userID.UuidUserId, otherUserID, nil, false, true, false, false, false, false, futureExpiry).
		AddRow(msg2Recipient, chatID, otherUserID, userID.UuidUserId, nil, true, false, false, false, false, false, futureExpiry)

	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(rows)

	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(userID.UuidUserId, allIDs).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_sender = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+sender_id = \$2\s+AND\s+deleted_by_sender = FALSE`).
		WithArgs([]uuid.UUID{msg1Sender}, userID.UuidUserId).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_recipient = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+recipient_id = \$2\s+AND\s+deleted_by_recipient = FALSE`).
		WithArgs([]uuid.UUID{msg2Recipient}, userID.UuidUserId).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// Because conditions are NOT met (msg1 has DeliveredToRecipientPrimary=false, msg2 has SyncedToSenderPrimary=false),
	// relay cleanup MUST NOT run.
	_, err := svc.DeleteMessageForMe(context.Background(), allIDs, userID, true)
	require.NoError(t, err)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_AlreadyDeletedMessages_SkippedEntirely(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	alreadyDeletedID := uuid.New()
	allIDs := []uuid.UUID{alreadyDeletedID}
	chatID := uuid.New()

	// 1. GetMessagesForDeleteForMeBatch returns the row with deleted_by_sender = true
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(alreadyDeletedID, chatID, userID.UuidUserId, uuid.New(), nil, true, true, false, false, true, false, futureExpiry)

	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(rows)

	// Since it's already true, no DB updates, no sync action, and no SSE groups are returned.
	groups, err := svc.DeleteMessageForMe(context.Background(), allIDs, userID, true)
	require.NoError(t, err)
	require.Empty(t, groups, "already-true messages must produce 0 groups")

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_AlreadyDeletedMessages_SecondaryDoesNotCreateSyncActions(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	alreadyDeletedID := uuid.New()
	allIDs := []uuid.UUID{alreadyDeletedID}
	chatID := uuid.New()

	// 1. GetMessagesForDeleteForMeBatch returns row with deleted_by_sender = true
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(alreadyDeletedID, chatID, userID.UuidUserId, uuid.New(), nil, true, true, false, false, true, false, futureExpiry)

	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(rows)

	// Secondary session must NOT create sync actions for already-deleted messages
	groups, err := svc.DeleteMessageForMe(context.Background(), allIDs, userID, false)
	require.NoError(t, err)
	require.Empty(t, groups, "must return 0 groups")

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_SmartSseGrouping_FiltersAlreadyTrueAndIneligible(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUser1 := uuid.New()
	otherUser2 := uuid.New()

	chatID1 := uuid.New()
	chatID2 := uuid.New()

	// 1. Eligible message in chat 1 (caller is sender)
	msg1EligibleChat1 := uuid.New()
	// 2. Already true message in chat 1 (caller is sender, but already marked deleted_by_sender=true)
	msg2AlreadyTrueChat1 := uuid.New()
	// 3. Ineligible message in chat 1 (caller is not a participant)
	msg3IneligibleUnrelated := uuid.New()
	// 4. Eligible message in chat 2 (caller is recipient)
	msg4EligibleChat2 := uuid.New()

	allIDs := []uuid.UUID{msg1EligibleChat1, msg2AlreadyTrueChat1, msg3IneligibleUnrelated, msg4EligibleChat2}

	// 1. GetMessagesForDeleteForMeBatch returns all existing rows
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(msg1EligibleChat1, chatID1, callerID.UuidUserId, otherUser1, nil, true, true, false, false, false, false, futureExpiry).
		AddRow(msg2AlreadyTrueChat1, chatID1, callerID.UuidUserId, otherUser1, nil, true, true, false, false, true, false, futureExpiry).
		AddRow(msg3IneligibleUnrelated, chatID1, uuid.New(), uuid.New(), nil, true, true, false, false, false, false, futureExpiry).
		AddRow(msg4EligibleChat2, chatID2, otherUser2, callerID.UuidUserId, nil, true, true, false, false, false, false, futureExpiry)

	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(rows)

	// 2. ClearLastMessageForParticipantBatch only for newly eligible messages
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(callerID.UuidUserId, []uuid.UUID{msg1EligibleChat1, msg4EligibleChat2}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 3. MarkMessagesDeletedBySenderBatch for msg1
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_sender = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+sender_id = \$2\s+AND\s+deleted_by_sender = FALSE`).
		WithArgs([]uuid.UUID{msg1EligibleChat1}, callerID.UuidUserId).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 4. MarkMessagesDeletedByRecipientBatch for msg4
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_recipient = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+recipient_id = \$2\s+AND\s+deleted_by_recipient = FALSE`).
		WithArgs([]uuid.UUID{msg4EligibleChat2}, callerID.UuidUserId).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	groups, err := svc.DeleteMessageForMe(context.Background(), allIDs, callerID, true)
	require.NoError(t, err)

	// Verify groups: exactly 2 groups, 1 per chat, containing ONLY eligible messages
	require.Len(t, groups, 2, "must produce exactly 2 chat groups")

	require.Equal(t, chatID1, groups[0].ChatID)
	require.Equal(t, []string{msg1EligibleChat1.String()}, groups[0].MessageIDs, "chat 1 must only contain eligible msg1 (not already-true msg2, not ineligible msg3)")

	require.Equal(t, chatID2, groups[1].ChatID)
	require.Equal(t, []string{msg4EligibleChat2.String()}, groups[1].MessageIDs, "chat 2 must only contain eligible msg4")

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_AllAlreadyTrueOrIneligible_ReturnsZeroGroups(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	alreadyTrueID := uuid.New()
	ineligibleID := uuid.New()
	allIDs := []uuid.UUID{alreadyTrueID, ineligibleID}

	// 1. GetMessagesForDeleteForMeBatch returns the already-true and ineligible rows
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(alreadyTrueID, uuid.New(), callerID.UuidUserId, uuid.New(), nil, true, true, false, false, true, false, futureExpiry).
		AddRow(ineligibleID, uuid.New(), uuid.New(), uuid.New(), nil, true, true, false, false, false, false, futureExpiry)

	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(rows)

	// 2. MarkMessagesDeletedBySenderBatch and MarkMessagesDeletedByRecipientBatch MUST NOT BE CALLED
	// 3. ClearLastMessageForParticipantBatch MUST NOT BE CALLED

	groups, err := svc.DeleteMessageForMe(context.Background(), allIDs, callerID, true)
	require.NoError(t, err)
	require.Empty(t, groups, "when all messages are already true or ineligible, returned groups must be empty (preventing SSE emissions)")

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_Category4_NonExistent_Primary(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	nonExistentID := uuid.New()
	allIDs := []uuid.UUID{nonExistentID}

	// 1. GetMessagesForDeleteForMeBatch returns 0 rows (message purged from relay)
	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(pgxmock.NewRows(deleteForMeBatchColumns()))

	// For Primary session:
	// - No DB updates
	// - No sync action
	// - SSE broadcast emitted (ChatID = uuid.Nil)
	groups, err := svc.DeleteMessageForMe(context.Background(), allIDs, callerID, true)
	require.NoError(t, err)
	require.Len(t, groups, 1, "must produce 1 group for SSE")
	require.Equal(t, uuid.Nil, groups[0].ChatID)
	require.Equal(t, []string{nonExistentID.String()}, groups[0].MessageIDs)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_Category4_NonExistent_Secondary(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	nonExistentID := uuid.New()
	allIDs := []uuid.UUID{nonExistentID}

	// 1. GetMessagesForDeleteForMeBatch returns 0 rows (message purged from relay)
	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(pgxmock.NewRows(deleteForMeBatchColumns()))

	// For Secondary session:
	// - Sync action created for non-existent IDs
	mock.ExpectExec(`(?s)INSERT INTO message_sync_actions.*FROM unnest\(\$2::uuid\[\]\)`).
		WithArgs(callerID.UuidUserId, []uuid.UUID{nonExistentID}).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	// - SSE broadcast emitted (ChatID = uuid.Nil)
	groups, err := svc.DeleteMessageForMe(context.Background(), allIDs, callerID, false)
	require.NoError(t, err)
	require.Len(t, groups, 1, "must produce 1 group for SSE")
	require.Equal(t, uuid.Nil, groups[0].ChatID)
	require.Equal(t, []string{nonExistentID.String()}, groups[0].MessageIDs)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_All4Categories_MixedBatch(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUser := uuid.New()
	chatID := uuid.New()

	cat1Eligible := uuid.New()
	cat2AlreadyTrue := uuid.New()
	cat3Ineligible := uuid.New()
	cat4NonExistent := uuid.New()

	allIDs := []uuid.UUID{cat1Eligible, cat2AlreadyTrue, cat3Ineligible, cat4NonExistent}

	// 1. GetMessagesForDeleteForMeBatch returns rows for cat1, cat2, cat3 (cat4 is absent from DB)
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(cat1Eligible, chatID, callerID.UuidUserId, otherUser, nil, true, true, false, false, false, false, futureExpiry).
		AddRow(cat2AlreadyTrue, chatID, callerID.UuidUserId, otherUser, nil, true, true, false, false, true, false, futureExpiry).
		AddRow(cat3Ineligible, chatID, uuid.New(), uuid.New(), nil, true, true, false, false, false, false, futureExpiry)

	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(rows)

	// 2. ClearLastMessage only for cat1
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(callerID.UuidUserId, []uuid.UUID{cat1Eligible}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 3. Mark deleted by sender only for cat1
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_sender = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+sender_id = \$2\s+AND\s+deleted_by_sender = FALSE`).
		WithArgs([]uuid.UUID{cat1Eligible}, callerID.UuidUserId).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 4. Secondary session creates sync actions for cat1 + cat4 (NOT cat2, NOT cat3)
	mock.ExpectExec(`(?s)INSERT INTO message_sync_actions.*FROM unnest\(\$2::uuid\[\]\)`).
		WithArgs(callerID.UuidUserId, []uuid.UUID{cat1Eligible, cat4NonExistent}).
		WillReturnResult(pgxmock.NewResult("INSERT", 2))

	groups, err := svc.DeleteMessageForMe(context.Background(), allIDs, callerID, false)
	require.NoError(t, err)

	// Groups emitted for SSE:
	// - Group 1: ChatID (contains cat1)
	// - Group 2: uuid.Nil (contains cat4)
	// (cat2 and cat3 are strictly omitted)
	require.Len(t, groups, 2)
	require.Equal(t, chatID, groups[0].ChatID)
	require.Equal(t, []string{cat1Eligible.String()}, groups[0].MessageIDs)

	require.Equal(t, uuid.Nil, groups[1].ChatID)
	require.Equal(t, []string{cat4NonExistent.String()}, groups[1].MessageIDs)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_SenderBatchError_FailsTransactionAndAborts(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUser := uuid.New()
	chatID := uuid.New()
	msgID := uuid.New()

	// 1. Lookup
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(msgID, chatID, userID.UuidUserId, otherUser, nil, true, true, false, false, false, false, futureExpiry)
	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{msgID}).
		WillReturnRows(rows)

	// 2. ClearLastMessage succeeds
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(userID.UuidUserId, []uuid.UUID{msgID}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 3. MarkMessagesDeletedBySenderBatch fails with DB error
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_sender = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+sender_id = \$2\s+AND\s+deleted_by_sender = FALSE`).
		WithArgs([]uuid.UUID{msgID}, userID.UuidUserId).
		WillReturnError(errors.New("db connection failure"))

	// DeleteMessageForMe must fail atomically and return a server error
	groups, delErr := svc.DeleteMessageForMe(context.Background(), []uuid.UUID{msgID}, userID, false)
	require.Error(t, delErr, "DeleteMessageForMe must return an error when sender delete fails")
	require.Nil(t, groups, "groups must be nil on failure so SSE is not emitted")

	err := mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_RecipientBatchError_FailsTransactionAndAborts(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUser := uuid.New()
	chatID := uuid.New()
	msgID := uuid.New()

	// 1. Lookup
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(msgID, chatID, otherUser, userID.UuidUserId, nil, true, true, false, false, false, false, futureExpiry)
	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{msgID}).
		WillReturnRows(rows)

	// 2. ClearLastMessage succeeds
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(userID.UuidUserId, []uuid.UUID{msgID}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 3. MarkMessagesDeletedByRecipientBatch fails
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_recipient = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+recipient_id = \$2\s+AND\s+deleted_by_recipient = FALSE`).
		WithArgs([]uuid.UUID{msgID}, userID.UuidUserId).
		WillReturnError(errors.New("db write failure"))

	groups, delErr := svc.DeleteMessageForMe(context.Background(), []uuid.UUID{msgID}, userID, false)
	require.Error(t, delErr, "DeleteMessageForMe must return an error when recipient delete fails")
	require.Nil(t, groups, "groups must be nil on failure so SSE is not emitted")

	err := mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_SyncActionError_FailsTransactionAndAborts(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	userID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUser := uuid.New()
	chatID := uuid.New()
	msgID := uuid.New()

	// 1. Lookup
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(msgID, chatID, userID.UuidUserId, otherUser, nil, true, true, false, false, false, false, futureExpiry)
	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{msgID}).
		WillReturnRows(rows)

	// 2. ClearLastMessage succeeds
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(userID.UuidUserId, []uuid.UUID{msgID}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 3. MarkMessagesDeletedBySenderBatch succeeds
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_sender = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+sender_id = \$2\s+AND\s+deleted_by_sender = FALSE`).
		WithArgs([]uuid.UUID{msgID}, userID.UuidUserId).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 4. CreateSyncActionsBatch fails
	mock.ExpectExec(`(?s)INSERT INTO message_sync_actions.*FROM unnest\(\$2::uuid\[\]\)`).
		WithArgs(userID.UuidUserId, []uuid.UUID{msgID}).
		WillReturnError(errors.New("sync actions db error"))

	groups, delErr := svc.DeleteMessageForMe(context.Background(), []uuid.UUID{msgID}, userID, false)
	require.Error(t, delErr, "DeleteMessageForMe must return an error when CreateSyncActionsBatch fails")
	require.Nil(t, groups, "groups must be nil on failure so SSE is not emitted")

	err := mock.ExpectationsWereMet()
	require.NoError(t, err)
}

// TestDeleteMessageForMe_ExpiredMessage_Secondary_CreatesSyncAction_ZeroDBWrites verifies:
// When an expired message is deleted by a secondary session:
// 1. It must NOT execute DB writes for delete-for-me (no MarkMessagesDeletedBySender/Recipient, no ClearLastMessage).
// 2. It MUST create a sync action in message_sync_actions so the primary device syncs the deletion.
// 3. It emits the SSE group for active sessions.
func TestDeleteMessageForMe_ExpiredMessage_Secondary_CreatesSyncAction_ZeroDBWrites(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUser := uuid.New()
	chatID := uuid.New()
	expiredMsgID := uuid.New()
	pastExpiry := time.Now().UTC().Add(-2 * time.Hour)

	// 1. GetMessagesForDeleteForMeBatch returns the expired message
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(expiredMsgID, chatID, callerID.UuidUserId, otherUser, nil, true, true, false, false, false, false, pastExpiry)
	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{expiredMsgID}).
		WillReturnRows(rows)

	// ZERO calls to ClearLastMessageForParticipantBatch
	// ZERO calls to MarkMessagesDeletedBySenderBatch or MarkMessagesDeletedByRecipientBatch

	// Secondary session MUST create sync action
	mock.ExpectExec(`(?s)INSERT INTO message_sync_actions.*FROM unnest\(\$2::uuid\[\]\)`).
		WithArgs(callerID.UuidUserId, []uuid.UUID{expiredMsgID}).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	groups, err := svc.DeleteMessageForMe(context.Background(), []uuid.UUID{expiredMsgID}, callerID, false)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, chatID, groups[0].ChatID)
	require.Equal(t, []string{expiredMsgID.String()}, groups[0].MessageIDs)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

// TestDeleteMessageForMe_ExpiredMessage_Primary_ZeroDBWrites_ZeroSyncActions verifies:
// When an expired message is deleted by a primary session:
// 1. It must NOT execute DB writes for delete-for-me (no MarkMessagesDeletedBySender/Recipient, no ClearLastMessage).
// 2. It must NOT create any sync action (primary device handles its own local storage).
// 3. ZERO DB write operations are performed across the entire database.
// 4. It emits the SSE group for active secondary sessions.
func TestDeleteMessageForMe_ExpiredMessage_Primary_ZeroDBWrites_ZeroSyncActions(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUser := uuid.New()
	chatID := uuid.New()
	expiredMsgID := uuid.New()
	pastExpiry := time.Now().UTC().Add(-2 * time.Hour)

	// 1. GetMessagesForDeleteForMeBatch returns the expired message
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(expiredMsgID, chatID, callerID.UuidUserId, otherUser, nil, true, true, false, false, false, false, pastExpiry)
	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs([]uuid.UUID{expiredMsgID}).
		WillReturnRows(rows)

	// ZERO calls to ClearLastMessageForParticipantBatch
	// ZERO calls to MarkMessagesDeletedBySenderBatch or MarkMessagesDeletedByRecipientBatch
	// ZERO calls to CreateSyncActionsBatch

	groups, err := svc.DeleteMessageForMe(context.Background(), []uuid.UUID{expiredMsgID}, callerID, true)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, chatID, groups[0].ChatID)
	require.Equal(t, []string{expiredMsgID.String()}, groups[0].MessageIDs)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_MixedBatch_EligibleAndExpired(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUser := uuid.New()
	chatID := uuid.New()

	eligMsgID := uuid.New()
	expiredMsgID := uuid.New()
	pastExpiry := time.Now().UTC().Add(-2 * time.Hour)
	allIDs := []uuid.UUID{eligMsgID, expiredMsgID}

	// 1. GetMessagesForDeleteForMeBatch returns both rows
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(eligMsgID, chatID, callerID.UuidUserId, otherUser, nil, true, true, false, false, false, false, futureExpiry).
		AddRow(expiredMsgID, chatID, callerID.UuidUserId, otherUser, nil, true, true, false, false, false, false, pastExpiry)
	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(rows)

	// 2. ClearLastMessageForParticipantBatch ONLY for eligMsgID (expired excluded from DB writes)
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+last_message_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs(callerID.UuidUserId, []uuid.UUID{eligMsgID}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 3. MarkMessagesDeletedBySenderBatch ONLY for eligMsgID
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+deleted_by_sender = TRUE.*WHERE\s+id = ANY\(\$1::uuid\[\]\)\s+AND\s+sender_id = \$2\s+AND\s+deleted_by_sender = FALSE`).
		WithArgs([]uuid.UUID{eligMsgID}, callerID.UuidUserId).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// 4. CreateSyncActionsBatch for BOTH eligMsgID and expiredMsgID (since secondary caller)
	mock.ExpectExec(`(?s)INSERT INTO message_sync_actions.*FROM unnest\(\$2::uuid\[\]\)`).
		WithArgs(callerID.UuidUserId, []uuid.UUID{eligMsgID, expiredMsgID}).
		WillReturnResult(pgxmock.NewResult("INSERT", 2))

	groups, err := svc.DeleteMessageForMe(context.Background(), allIDs, callerID, false)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, chatID, groups[0].ChatID)
	require.ElementsMatch(t, []string{eligMsgID.String(), expiredMsgID.String()}, groups[0].MessageIDs)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

type safeLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeLogBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeLogBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestDeleteMessageForMe_AlreadyTrue_TriggersRelayCleanup_WhenDelivered(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUser := uuid.New()
	chatID := uuid.New()
	alreadyTrueID := uuid.New()
	allIDs := []uuid.UUID{alreadyTrueID}

	safeBuf := &safeLogBuffer{}
	origOutput := log.Writer()
	log.SetOutput(safeBuf)
	defer log.SetOutput(origOutput)

	// 1. Upfront batch query returns already-true message with DeliveredToRecipientPrimary = true
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(alreadyTrueID, chatID, callerID.UuidUserId, otherUser, nil, true, true, false, false, true, false, futureExpiry)
	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(rows)

	// ZERO calls to ClearLastMessageForParticipantBatch
	// ZERO calls to MarkMessagesDeletedBySenderBatch
	// ZERO calls to CreateSyncActionsBatch

	groups, err := svc.DeleteMessageForMe(context.Background(), allIDs, callerID, true)
	require.NoError(t, err)
	require.Empty(t, groups, "already-true messages must produce 0 SSE groups")

	// Verify relay cleanup was triggered in background goroutine for this delivered already-true message
	expectedLog := fmt.Sprintf("[Relay-Cleanup] Message %s fully delivered and synced", alreadyTrueID)
	assert.Eventually(t, func() bool {
		return strings.Contains(safeBuf.String(), expectedLog)
	}, 1*time.Second, 10*time.Millisecond, "relay cleanup MUST be triggered for already-true message when DeliveredToRecipientPrimary is true")

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestDeleteMessageForMe_AlreadyTrue_SkipsRelayCleanup_WhenNotDelivered(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUser := uuid.New()
	chatID := uuid.New()
	alreadyTrueID := uuid.New()
	allIDs := []uuid.UUID{alreadyTrueID}

	safeBuf := &safeLogBuffer{}
	origOutput := log.Writer()
	log.SetOutput(safeBuf)
	defer log.SetOutput(origOutput)

	// 1. Upfront batch query returns already-true message with DeliveredToRecipientPrimary = FALSE
	rows := pgxmock.NewRows(deleteForMeBatchColumns()).
		AddRow(alreadyTrueID, chatID, callerID.UuidUserId, otherUser, nil, false, true, false, false, true, false, futureExpiry)
	mock.ExpectQuery(`(?s)SELECT.*FROM messages\s+WHERE\s+id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(allIDs).
		WillReturnRows(rows)

	groups, err := svc.DeleteMessageForMe(context.Background(), allIDs, callerID, true)
	require.NoError(t, err)
	require.Empty(t, groups)

	// Relay cleanup must never be launched for an undelivered already-true message.
	unexpectedLog := fmt.Sprintf("[Relay-Cleanup] Message %s fully delivered and synced", alreadyTrueID)
	require.Never(t, func() bool {
		return strings.Contains(safeBuf.String(), unexpectedLog)
	}, 200*time.Millisecond, 10*time.Millisecond, "relay cleanup MUST NOT be triggered when DeliveredToRecipientPrimary is false")

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}
