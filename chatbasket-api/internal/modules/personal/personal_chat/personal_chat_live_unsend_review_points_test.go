package personal_chat

import (
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liveUnsendFixture inserts an unsend-ready chat: one sender, one recipient,
// messageCount unread text messages, and the last message as the chat preview.
type liveUnsendFixture struct {
	chatID      uuid.UUID
	senderID    uuid.UUID
	recipientID uuid.UUID
	messageIDs  []uuid.UUID
}

func insertLiveUnsendFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, emailPrefix string, messageCount int) liveUnsendFixture {
	t.Helper()
	require.Greater(t, messageCount, 0)

	f := liveUnsendFixture{chatID: uuid.New()}
	f.senderID, f.recipientID = insertLiveUsers(t, ctx, pool, "UnsendLive", emailPrefix)

	for i := 0; i < messageCount; i++ {
		f.messageIDs = append(f.messageIDs, uuid.New())
	}
	lastMessageID := f.messageIDs[len(f.messageIDs)-1]

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
			0, $4,
			$5, now(), $2,
			'hello', 'hello',
			'text', 'text',
			now(), now()
		)`, f.chatID, f.senderID, f.recipientID, int32(messageCount), lastMessageID)
	require.NoError(t, err)

	for i, id := range f.messageIDs {
		_, err := pool.Exec(ctx, `
			INSERT INTO messages (
				id, chat_id, sender_id, recipient_id, content, message_type,
				sender_session_id,
				delivered_to_recipient, delivered_to_recipient_primary, synced_to_sender_primary,
				read_by_recipient, read_acked_by_sender,
				deleted_by_sender, deleted_by_recipient,
				expires_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, 'text', $6, TRUE, TRUE, TRUE, FALSE, FALSE, FALSE, FALSE,
				now() + interval '1 day', now(), now()
			)`, id, f.chatID, f.senderID, f.recipientID, fmt.Sprintf("live message %d", i), uuid.New())
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM message_sync_actions WHERE user_id IN ($1, $2)`, f.senderID, f.recipientID)
		_, _ = pool.Exec(bg, `DELETE FROM messages WHERE chat_id = $1`, f.chatID)
		_, _ = pool.Exec(bg, `DELETE FROM chats WHERE id = $1`, f.chatID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id IN ($1, $2)`, f.senderID, f.recipientID)
		_, _ = pool.Exec(bg, `DELETE FROM auth_users WHERE id IN ($1, $2)`, f.senderID, f.recipientID)
	})

	return f
}

// TestUnsendMessage_LiveNeonConcurrentDoubleUnsend proves review point 4 on a
// real database. The chat row is locked first, so both unsend requests complete
// their upfront eligibility lookup and then block. When the lock is released
// they run one after the other, exactly like two devices unsending at the same
// time. The UPDATE predicates do not exclude rows that are already
// message_type='unsent', so the second request matches the same rows again.
//
// The UPDATE predicates must exclude rows that are already unsent, so exactly
// one request reports the IDs as newly unsent, the recipient's unread counter
// ends at 1 (the extra never-unsent message), and exactly one recipient
// 'unsend' sync action is inserted.
func TestUnsendMessage_LiveNeonConcurrentDoubleUnsend(t *testing.T) {
	for _, tc := range []struct {
		name    string
		targets int
	}{
		{name: "single-row branch", targets: 1},
		{name: "batch branch", targets: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, pool := newLivePool(t, 90*time.Second)

			// One extra unread message is never unsent, so a correct run must
			// leave the recipient with exactly 1 unread message.
			f := insertLiveUnsendFixture(t, ctx, pool, "unsend-race-", tc.targets+1)
			targetIDs := f.messageIDs[:tc.targets]
			targetIDStrings := make([]string, len(targetIDs))
			for i, id := range targetIDs {
				targetIDStrings[i] = id.String()
			}

			store := personal_chat_store.New(pool)
			svc := &chatService{
				Pool:            pool,
				PostgresQuerier: store,
				PostgresQueries: store,
				PendingUploads:  &mockPendingUploadsChat{},
			}

			// Hold the chat row lock so both requests finish the pre-unsend
			// lookup before either one can write.
			holdTx, err := pool.Begin(ctx)
			require.NoError(t, err)
			defer holdTx.Rollback(context.Background())
			var lockedChatID uuid.UUID
			require.NoError(t, holdTx.QueryRow(ctx, `SELECT id FROM chats WHERE id = $1 FOR NO KEY UPDATE`, f.chatID).Scan(&lockedChatID))

			raceCtx, cancelRace := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancelRace()

			type unsendResult struct {
				out unsendOutcome
				err error
			}
			results := make(chan unsendResult, 2)
			call := func() {
				out, err := svc.UnsendMessageHandler(raceCtx, &UnsendMessagePayload{MessageIDs: targetIDStrings}, kit.UserId{
					UuidUserId:   f.senderID,
					StringUserId: f.senderID.String(),
				}, true)
				results <- unsendResult{out: out, err: err}
			}
			go call()
			go call()

			// Both requests are blocked on the chat row lock, which proves both
			// consumed the pre-unsend message state.
			require.Eventually(t, func() bool {
				var waiting int
				err := pool.QueryRow(ctx, `
					SELECT count(*)
					FROM pg_stat_activity
					WHERE datname = current_database()
					  AND state = 'active'
					  AND wait_event_type = 'Lock'
					  AND query ILIKE '%for no key update%'`).Scan(&waiting)
				return err == nil && waiting >= 2
			}, 30*time.Second, 100*time.Millisecond, "both unsend requests should block on the chat row lock")

			require.NoError(t, holdTx.Commit(ctx))

			got := make([]unsendResult, 0, 2)
			for i := 0; i < 2; i++ {
				select {
				case r := <-results:
					got = append(got, r)
				case <-time.After(45 * time.Second):
					t.Fatal("timed out waiting for the concurrent unsend requests")
				}
			}
			for i, r := range got {
				require.NoErrorf(t, r.err, "unsend request %d failed", i)
			}

			var finalUnread int32
			require.NoError(t, pool.QueryRow(ctx, `SELECT p2_unread_count FROM chats WHERE id = $1`, f.chatID).Scan(&finalUnread))

			rows, err := pool.Query(ctx, `
				SELECT payload FROM message_sync_actions
				WHERE user_id = $1 AND action_type = 'unsend'
				ORDER BY created_at`, f.recipientID)
			require.NoError(t, err)
			var payloads []SyncActionPayload
			for rows.Next() {
				var raw []byte
				require.NoError(t, rows.Scan(&raw))
				var payload SyncActionPayload
				require.NoError(t, json.Unmarshal(raw, &payload))
				payloads = append(payloads, payload)
			}
			require.NoError(t, rows.Err())
			rows.Close()

			t.Logf("observed: p2_unread_count=%d, recipient 'unsend' sync actions=%d", finalUnread, len(payloads))

			newlyUnsentRequests := 0
			for i, r := range got {
				assert.ElementsMatchf(t, targetIDStrings, r.out.AcknowledgedIDs, "request %d must acknowledge the requested IDs", i)
				if len(r.out.NewlyUnsentIDs) > 0 {
					newlyUnsentRequests++
					assert.ElementsMatchf(t, targetIDStrings, r.out.NewlyUnsentIDs, "request %d reported these as newly unsent", i)
				}
			}
			assert.Equal(t, 1, newlyUnsentRequests, "exactly one request may report the IDs as newly unsent")
			assert.Equal(t, int32(1), finalUnread, "only the one never-unsent message remains unread")
			require.Len(t, payloads, 1, "exactly one recipient sync action")
			assert.ElementsMatch(t, targetIDStrings, payloads[0].MessageIDs)
			assert.Equal(t, f.chatID.String(), payloads[0].ChatID)
		})
	}
}

// TestUnsendSQL_LiveNeonRepeatedUpdateIsIdempotent proves both UPDATE
// statements ignore rows that are already unsent, so a repeated unsend cannot
// report the same row again (review point 4).
func TestUnsendSQL_LiveNeonRepeatedUpdateIsIdempotent(t *testing.T) {
	ctx, pool := newLivePool(t, 60*time.Second)
	f := insertLiveUnsendFixture(t, ctx, pool, "unsend-repeat-", 2)
	store := personal_chat_store.New(pool)

	// Batch query (the path ported to single-pass).
	firstBatch, err := store.UpdateMessagesToUnsentBatch(ctx, []uuid.UUID{f.messageIDs[0]})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{f.messageIDs[0]}, firstBatch)
	secondBatch, err := store.UpdateMessagesToUnsentBatch(ctx, []uuid.UUID{f.messageIDs[0]})
	require.NoError(t, err)
	assert.Empty(t, secondBatch,
		"batch UPDATE must not match an already-unsent row")

	// Single-row query (unchanged by the port).
	firstRows, err := store.UpdateMessageToUnsent(ctx, f.messageIDs[1])
	require.NoError(t, err)
	require.EqualValues(t, 1, firstRows)
	secondRows, err := store.UpdateMessageToUnsent(ctx, f.messageIDs[1])
	require.NoError(t, err)
	assert.EqualValues(t, 0, secondRows,
		"single-row UPDATE must not match an already-unsent row")

	var unsent int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM messages
		WHERE chat_id = $1 AND message_type = 'unsent'`, f.chatID).Scan(&unsent))
	assert.Equal(t, 2, unsent)
}

// TestUnsendSQL_LiveNeonBatchQueriesHandleSingleRow checks point 6's claim that
// the batch UPDATE and batch preview UPDATE behave correctly with exactly one
// ID, so the dedicated single-row statements are not required for correctness.
func TestUnsendSQL_LiveNeonBatchQueriesHandleSingleRow(t *testing.T) {
	ctx, pool := newLivePool(t, 60*time.Second)
	f := insertLiveUnsendFixture(t, ctx, pool, "unsend-single-batch-", 1)
	store := personal_chat_store.New(pool)

	updated, err := store.UpdateMessagesToUnsentBatch(ctx, []uuid.UUID{f.messageIDs[0]})
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{f.messageIDs[0]}, updated)

	require.NoError(t, store.UpdateChatUnsendPreviewBatch(ctx, personal_chat_store.UpdateChatUnsendPreviewBatchParams{
		ID:         f.chatID,
		MessageIds: []uuid.UUID{f.messageIDs[0]},
	}))

	var content, messageType string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT p1_last_message_content, p1_last_message_type
		FROM chats WHERE id = $1`, f.chatID).Scan(&content, &messageType))
	assert.Equal(t, "Message unsent", content)
	assert.Equal(t, "unsent", messageType)
}
