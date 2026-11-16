package personal_chat

import (
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// capturedArgument is a pgxmock.Argument that accepts any value and records it,
// so tests can assert the exact argument the service passed to the database.
type capturedArgument struct {
	value any
}

func (c *capturedArgument) Match(actual any) bool {
	c.value = actual
	return true
}

// payload decodes the captured JSONB argument into a SyncActionPayload.
func (c *capturedArgument) payload(t *testing.T) SyncActionPayload {
	t.Helper()
	raw, ok := c.value.([]byte)
	require.True(t, ok, "payload argument must be []byte, got %T", c.value)
	var payload SyncActionPayload
	require.NoError(t, json.Unmarshal(raw, &payload))
	return payload
}

func batchAckColumns() []string {
	return []string{
		"id", "chat_id", "sender_id",
		"file_id",
		"delivered_to_recipient_primary", "synced_to_sender_primary",
		"read_by_recipient", "read_acked_by_sender",
		"created_at",
	}
}

func getMessagesForMarkChatReadColumns() []string {
	return []string{
		"id", "chat_id", "sender_id", "recipient_id",
		"read_by_recipient", "read_at", "read_acked_by_sender",
		"created_at", "expires_at",
	}
}

func getMessagesForUnsendBatchColumns() []string {
	return []string{
		"id", "chat_id", "sender_id", "recipient_id",
		"message_type", "file_id", "read_by_recipient",
		"expires_at",
	}
}

func getMessagesForAckAndReadBatchColumns() []string {
	return []string{
		"id", "chat_id", "sender_id", "recipient_id",
		"file_id", "delivered_to_recipient", "delivered_to_recipient_primary",
		"synced_to_sender_primary",
		"read_by_recipient", "read_acked_by_sender", "read_at",
		"created_at", "expires_at",
	}
}

func deliveryAckColumns() []string {
	return []string{
		"id", "chat_id", "sender_id", "recipient_id",
		"file_id",
		"delivered_to_recipient", "delivered_to_recipient_primary", "synced_to_sender_primary",
		"read_by_recipient", "read_at", "read_acked_by_sender",
		"created_at", "expires_at",
	}
}

func chatColumns() []string {
	return []string{
		"id", "participant_1_id", "participant_2_id",
		"p1_unread_count", "p2_unread_count",
		"p1_last_read_at", "p2_last_read_at",
		"p1_last_delivered_at", "p2_last_delivered_at",
		"last_message_created_at", "last_message_sender_id", "last_message_id",
		"p1_last_message_content", "p2_last_message_content",
		"p1_last_message_type", "p2_last_message_type",
		"created_at", "updated_at",
	}
}

// TestMarkChatRead_RejectsMixedChat verifies Rule 1:
// When MarkChatRead is called with message IDs belonging to different chats,
// the entire request is rejected with 400 Bad Request (invalid_batch).

const getMessagesForMarkChatReadPattern = `(?s)SELECT id, chat_id, sender_id, recipient_id, read_by_recipient, read_at, read_acked_by_sender, created_at, expires_at FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`

const getMessagesForUnsendBatchPattern = `(?s)SELECT id, chat_id, sender_id, recipient_id, message_type, file_id, read_by_recipient, expires_at FROM messages WHERE id = ANY\(\$1::uuid\[\]\)`

const deliveryAckPattern = `(?s)SELECT\s+id,\s+chat_id,\s+sender_id,\s+recipient_id,\s+file_id,\s+delivered_to_recipient,\s+delivered_to_recipient_primary,\s+synced_to_sender_primary,\s+read_by_recipient,\s+read_at,\s+read_acked_by_sender,\s+created_at,\s+expires_at\s+FROM messages\s+WHERE id = ANY\(\$1::uuid\[\]\)`

// TestAcknowledgeDeliveryBatch_MultiSenderGrouping verifies that when a batch of messages
// spanning multiple chats and distinct senders is acknowledged by a recipient:
// 1. Exactly 1 set-based batch UPDATE query is executed for all message IDs.
// 2. Chat last_delivered_at is updated once per distinct chat.
// 3. DeliveryGroups partitions messages strictly by (SenderID, ChatID).
// 4. Sender A's group contains ONLY Sender A's message IDs for Chat 1.
// 5. Sender B's group contains ONLY Sender B's message IDs for Chat 2 (neither omitted nor mixed).

func oversizedMessageIDStrings(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = uuid.New().String()
	}
	return ids
}

// newChatMockSvc creates a pgxmock pool and a chatService for unit tests. The mock
// is closed via t.Cleanup, so callers must not defer mock.Close() themselves.
// When pendingUploads is true the service is wired with a mockPendingUploadsChat.
func newChatMockSvc(t *testing.T, pendingUploads bool) (pgxmock.PgxPoolIface, *chatService) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	store := personal_chat_store.New(mock)
	svc := &chatService{
		PostgresQuerier: store,
		PostgresQueries: store,
	}
	if pendingUploads {
		svc.PendingUploads = &mockPendingUploadsChat{}
	}
	return mock, svc
}

// newLivePool connects to the live test database with the given timeout. The
// context and pool are cleaned up via t.Cleanup, so callers must not defer
// cancel() or pool.Close() themselves.
func newLivePool(t *testing.T, timeout time.Duration) (context.Context, *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, getTestDSN(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return ctx, pool
}

// insertLiveUsers inserts two ordered (u1 < u2) live-test users into auth_users
// and users and returns them. Callers still insert their own chats/messages.
func insertLiveUsers(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name, emailPrefix string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	u1, u2 := uuid.New(), uuid.New()
	if bytesCompareUUID(u1, u2) > 0 {
		u1, u2 = u2, u1
	}
	h1 := sha256.Sum256([]byte(u1.String()))
	h2 := sha256.Sum256([]byte(u2.String()))

	_, err := pool.Exec(ctx, `
		INSERT INTO auth_users (id, name, email, password_hash, is_email_verified, keys_revision, created_at, updated_at)
		VALUES ($1, $2, $3, 'hash', true, 1, now(), now()),
		       ($4, $5, $6, 'hash', true, 1, now(), now())`,
		u1, name+" U1", fmt.Sprintf("%su1-%s@test.com", emailPrefix, u1),
		u2, name+" U2", fmt.Sprintf("%su2-%s@test.com", emailPrefix, u2))
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, name, profile_type, hmac_sha256_hex_username, b64_cipher_chacha20poly1305_username, created_at, updated_at)
		VALUES ($1, $2, 'personal', $3, 'cipher1', now(), now()),
		       ($4, $5, 'personal', $6, 'cipher2', now(), now())`,
		u1, name+" U1", hex.EncodeToString(h1[:]),
		u2, name+" U2", hex.EncodeToString(h2[:]))
	require.NoError(t, err)
	return u1, u2
}
