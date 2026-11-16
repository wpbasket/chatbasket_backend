package personal_chat

import (
	"chatbasket-api/gen/proto/personal/personal_chat"
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readReceiptMessageIDs(receipts []*rpc_personal_chatv1.MessageReadReceipt) []string {
	ids := make([]string, 0, len(receipts))
	for _, receipt := range receipts {
		ids = append(ids, receipt.MessageId)
	}
	return ids
}

// insertReadOrderMessage inserts one message for the read-order live tests.
func insertReadOrderMessage(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	id, chatID, senderID, recipientID uuid.UUID,
	read bool,
	readAt *time.Time,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		INSERT INTO messages (
			id, chat_id, sender_id, recipient_id, content, message_type,
			sender_session_id,
			delivered_to_recipient, delivered_to_recipient_primary, synced_to_sender_primary,
			read_by_recipient, read_at, read_acked_by_sender,
			deleted_by_sender, deleted_by_recipient,
			expires_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, 'hello', 'text',
			$5,
			TRUE, TRUE, FALSE,
			$6, $7, FALSE,
			FALSE, FALSE,
			now() + interval '1 day', now(), now()
		)`, id, chatID, senderID, recipientID, uuid.New(), read, readAt)
	require.NoError(t, err)
}

// newReadOrderFixture creates a chat where u2 is the recipient and inserts:
//   - new1, new2, new3: unread messages (inserted in this order)
//   - already: already read by u2
//   - senderOwned: authored by u2, so ineligible for a read receipt
//
// The returned IDs are deliberately requested in a different order by each test.
func newReadOrderFixture(t *testing.T) (
	ctx context.Context,
	pool *pgxpool.Pool,
	chatID, u1, u2 uuid.UUID,
	new1, new2, new3, already, senderOwned uuid.UUID,
	originalReadAt time.Time,
) {
	t.Helper()
	ctx, pool = newLivePool(t, 60*time.Second)
	chatID = uuid.New()
	u1, u2 = insertLiveUsers(t, ctx, pool, "ReadOrder", "ro-")

	_, err := pool.Exec(ctx, `
		INSERT INTO chats (id, participant_1_id, participant_2_id, p2_unread_count, created_at, updated_at)
		VALUES ($1, $2, $3, 3, now(), now())`, chatID, u1, u2)
	require.NoError(t, err)

	new1, new2, new3 = uuid.New(), uuid.New(), uuid.New()
	already, senderOwned = uuid.New(), uuid.New()
	originalReadAt = time.Now().Add(-time.Hour).Truncate(time.Microsecond)

	insertReadOrderMessage(t, ctx, pool, new1, chatID, u1, u2, false, nil)
	insertReadOrderMessage(t, ctx, pool, new2, chatID, u1, u2, false, nil)
	insertReadOrderMessage(t, ctx, pool, new3, chatID, u1, u2, false, nil)
	insertReadOrderMessage(t, ctx, pool, already, chatID, u1, u2, true, &originalReadAt)
	insertReadOrderMessage(t, ctx, pool, senderOwned, chatID, u2, u1, false, nil)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE chat_id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM chats WHERE id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, u1, u2)
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_users WHERE id IN ($1, $2)`, u1, u2)
	})

	return ctx, pool, chatID, u1, u2, new1, new2, new3, already, senderOwned, originalReadAt
}

// TestMarkChatRead_ReceiptsFollowRequestOrder pins point 5 on a real database.
// The UPDATE ... RETURNING rows arrive in insertion order (new1, new2, new3),
// while the request asks for new3 first, then the already-read row. The
// response must follow the request, not the database.
func TestMarkChatRead_ReceiptsFollowRequestOrder(t *testing.T) {
	ctx, pool, chatID, u1, u2, new1, new2, new3, already, senderOwned, _ := newReadOrderFixture(t)
	missingID := uuid.New()

	store := personal_chat_store.New(pool)
	svc := &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	out, err := svc.MarkChatRead(ctx, &MarkChatReadPayload{MessageIDs: []string{
		new3.String(),
		already.String(),
		senderOwned.String(),
		new1.String(),
		missingID.String(),
		new2.String(),
		new1.String(), // duplicate must not appear twice
	}}, kit.UserId{UuidUserId: u2, StringUserId: u2.String()}, true)
	require.NoError(t, err)

	require.Equal(t, chatID, out.ChatID)
	assert.Equal(t, u1, out.OtherUserID)
	assert.Equal(t, []string{new3.String(), already.String(), new1.String(), new2.String()},
		readReceiptMessageIDs(out.Receipts), "response receipts must follow request order")
	assert.Equal(t, []string{new3.String(), new1.String(), new2.String()},
		readReceiptMessageIDs(out.NewlyRead),
		"SSE must follow request order and carry only the rows this call actually read")
	assert.Equal(t, []string{missingID.String()}, out.PermanentlyRejectedIDs)
}

// TestAcknowledgeAndReadBatch_ReceiptsFollowRequestOrder pins point 5 for the
// combined delivery+read endpoint on a real database. It has the same mixed
// assembly as MarkChatRead: newly read rows from RETURNING followed by
// already-read rows from the request.
func TestAcknowledgeAndReadBatch_ReceiptsFollowRequestOrder(t *testing.T) {
	ctx, pool, chatID, u1, u2, new1, new2, new3, already, senderOwned, _ := newReadOrderFixture(t)
	missingID := uuid.New()

	store := personal_chat_store.New(pool)
	svc := &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	out, err := svc.AcknowledgeAndReadBatch(ctx, &AckAndReadBatchPayload{MessageIDs: []string{
		new3.String(),
		already.String(),
		senderOwned.String(),
		new1.String(),
		missingID.String(),
		new2.String(),
		new1.String(), // duplicate must not appear twice
	}}, kit.UserId{UuidUserId: u2, StringUserId: u2.String()}, true)
	require.NoError(t, err)

	require.Equal(t, chatID, out.ChatID)
	assert.Equal(t, u1, out.OtherUserID)
	assert.Equal(t, []string{new3.String(), already.String(), new1.String(), new2.String()},
		readReceiptMessageIDs(out.Receipts), "response receipts must follow request order")
	assert.Equal(t, []string{new3.String(), new1.String(), new2.String()},
		readReceiptMessageIDs(out.NewlyRead),
		"SSE must follow request order and carry only the rows this call actually read")
	assert.Equal(t, []string{missingID.String()}, out.PermanentlyRejectedIDs)
}
