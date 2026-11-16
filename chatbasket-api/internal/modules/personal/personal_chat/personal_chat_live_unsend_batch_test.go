package personal_chat

import (
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnsendMessage_LiveNeonBatchSinglePass exercises the multi-message unsend
// against a real PostgreSQL database: both rows must be tombstoned, the chat
// preview and unread counter must be updated once, and the recipient must get
// exactly one sync action carrying both message IDs.
func TestUnsendMessage_LiveNeonBatchSinglePass(t *testing.T) {
	ctx, pool := newLivePool(t, 60*time.Second)

	chatID := uuid.New()
	u1, u2 := insertLiveUsers(t, ctx, pool, "UnsendLive", "unsendlive-")

	m1, m2 := uuid.New(), uuid.New()

	_, err := pool.Exec(ctx, `
		INSERT INTO chats (
			id, participant_1_id, participant_2_id,
			p1_unread_count, p2_unread_count,
			last_message_id, last_message_created_at, last_message_sender_id,
			p1_last_message_content, p2_last_message_content,
			p1_last_message_type, p2_last_message_type,
			created_at, updated_at
		) VALUES (
			$1, $2, $3,
			0, 2,
			$4, now(), $2,
			'hello 2', 'hello 2',
			'text', 'text',
			now(), now()
		)`, chatID, u1, u2, m2)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO messages (
			id, chat_id, sender_id, recipient_id, content, message_type,
			sender_session_id,
			delivered_to_recipient, delivered_to_recipient_primary, synced_to_sender_primary,
			read_by_recipient, read_acked_by_sender,
			deleted_by_sender, deleted_by_recipient,
			expires_at, created_at, updated_at
		) VALUES
			($1, $3, $4, $5, 'hello 1', 'text', $6, TRUE, TRUE, TRUE, FALSE, FALSE, FALSE, FALSE, now() + interval '1 day', now(), now()),
			($2, $3, $4, $5, 'hello 2', 'text', $7, TRUE, TRUE, TRUE, FALSE, FALSE, FALSE, FALSE, now() + interval '1 day', now(), now())`,
		m1, m2, chatID, u1, u2, uuid.New(), uuid.New())
	require.NoError(t, err)

	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM message_sync_actions WHERE user_id IN ($1, $2)`, u1, u2)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE chat_id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM chats WHERE id = $1`, chatID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, u1, u2)
		_, _ = pool.Exec(context.Background(), `DELETE FROM auth_users WHERE id IN ($1, $2)`, u1, u2)
	}()

	store := personal_chat_store.New(pool)
	svc := &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	out, err := svc.UnsendMessageHandler(ctx, &UnsendMessagePayload{
		MessageIDs: []string{m1.String(), m2.String()},
	}, kit.UserId{UuidUserId: u1, StringUserId: u1.String()}, true)
	require.NoError(t, err)
	assert.Equal(t, chatID, out.ChatID)
	assert.Equal(t, u2, out.RecipientID)
	assert.ElementsMatch(t, []string{m1.String(), m2.String()}, out.NewlyUnsentIDs)
	assert.ElementsMatch(t, []string{m1.String(), m2.String()}, out.AcknowledgedIDs)
	assert.Empty(t, out.PermanentlyRejectedIDs)

	// Both message rows are tombstoned.
	var tombstoned int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM messages
		WHERE chat_id = $1 AND message_type = 'unsent' AND content = 'Message unsent'`, chatID).Scan(&tombstoned))
	assert.Equal(t, 2, tombstoned)

	// The recipient's unread counter is decremented by the batch size, and the
	// last-message preview is replaced.
	var p2Unread int32
	var p1Preview, p2Preview, p1Type, p2Type string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT p2_unread_count, p1_last_message_content, p2_last_message_content,
		       p1_last_message_type, p2_last_message_type
		FROM chats WHERE id = $1`, chatID).
		Scan(&p2Unread, &p1Preview, &p2Preview, &p1Type, &p2Type))
	assert.Equal(t, int32(0), p2Unread)
	assert.Equal(t, "Message unsent", p1Preview)
	assert.Equal(t, "Message unsent", p2Preview)
	assert.Equal(t, "unsent", p1Type)
	assert.Equal(t, "unsent", p2Type)

	// Exactly one recipient sync action carrying both IDs.
	rows, err := pool.Query(ctx, `
		SELECT payload FROM message_sync_actions
		WHERE user_id = $1 AND action_type = 'unsend'`, u2)
	require.NoError(t, err)
	defer rows.Close()

	var payloads []SyncActionPayload
	for rows.Next() {
		var raw []byte
		require.NoError(t, rows.Scan(&raw))
		var payload SyncActionPayload
		require.NoError(t, json.Unmarshal(raw, &payload))
		payloads = append(payloads, payload)
	}
	require.NoError(t, rows.Err())
	require.Len(t, payloads, 1)
	assert.ElementsMatch(t, []string{m1.String(), m2.String()}, payloads[0].MessageIDs)
	assert.Equal(t, chatID.String(), payloads[0].ChatID)
}
