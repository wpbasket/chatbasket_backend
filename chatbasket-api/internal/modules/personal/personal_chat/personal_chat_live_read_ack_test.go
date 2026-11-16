package personal_chat

import (
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcknowledgeReadReceiptBatch_LiveDBAcksRowsFromDifferentChats sends one
// read-receipt ACK to a real database with rows from three of the caller's
// chats and one row from a chat the caller is not in. The same request ACKs the
// caller's rows in every chat. As in AcknowledgeDeliveryBatch, the chat of each
// row ACKed now is swept, also when that row itself is not ready for cleanup.
// The other people's chat is never swept, even though one of its IDs is in the
// request.
func TestAcknowledgeReadReceiptBatch_LiveDBAcksRowsFromDifferentChats(t *testing.T) {
	ctx, pool := newLivePool(t, 90*time.Second)

	alice, bob := insertLiveUsers(t, ctx, pool, "ReadAck", "readack-")
	carol, dave := insertLiveUsers(t, ctx, pool, "ReadAck2", "readack2-")
	users := []uuid.UUID{alice, bob, carol, dave}
	chatAB, chatAC, chatAD, chatBC := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	chats := []uuid.UUID{chatAB, chatAC, chatAD, chatBC}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM messages WHERE chat_id = ANY($1)`, chats)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM chats WHERE id = ANY($1)`, chats)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM users WHERE id = ANY($1)`, users)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM auth_users WHERE id = ANY($1)`, users)
	})

	for _, c := range []struct{ id, a, b uuid.UUID }{
		{chatAB, alice, bob}, {chatAC, alice, carol}, {chatAD, alice, dave}, {chatBC, bob, carol},
	} {
		p1, p2 := c.a, c.b
		if bytesCompareUUID(p1, p2) > 0 {
			p1, p2 = p2, p1
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO chats (id, participant_1_id, participant_2_id, created_at, updated_at)
			VALUES ($1, $2, $3, now(), now())`, c.id, p1, p2)
		require.NoError(t, err)
	}

	type seed struct {
		id, chatID, sender, recipient uuid.UUID
		onRecipientPrimary            bool // delivered_to_recipient_primary
		read, readAcked               bool
	}
	newSeed := func(chatID, sender, recipient uuid.UUID, onRecipientPrimary, read, readAcked bool) seed {
		return seed{uuid.New(), chatID, sender, recipient, onRecipientPrimary, read, readAcked}
	}
	// Every row is on the sender's primary device. In the request:
	ackAB := newSeed(chatAB, alice, bob, true, true, false)        // read and on both primary devices: deleted after the ACK
	ackAC := newSeed(chatAC, alice, carol, true, true, false)      // read and on both primary devices: deleted after the ACK
	unreadAC := newSeed(chatAC, alice, carol, true, false, false)  // not read yet: skipped
	receivedAC := newSeed(chatAC, carol, alice, true, true, false) // alice received it: skipped
	ackAD := newSeed(chatAD, alice, dave, false, true, false)      // read on dave's other device, not on his primary yet: ACKed, no cleanup of its own
	foreignBC := newSeed(chatBC, bob, carol, true, true, false)    // alice is neither sender nor recipient: rejected
	// Not in the request: finished rows that only a sweep of their chat removes.
	doneAB := newSeed(chatAB, alice, bob, true, true, true)
	doneAC := newSeed(chatAC, alice, carol, true, true, true)
	doneAD := newSeed(chatAD, alice, dave, true, true, true)
	doneBC := newSeed(chatBC, bob, carol, true, true, true)
	seeds := []seed{ackAB, ackAC, unreadAC, receivedAC, ackAD, foreignBC, doneAB, doneAC, doneAD, doneBC}

	n := len(seeds)
	ids, chatIDs, senders, recipients := make([]uuid.UUID, n), make([]uuid.UUID, n), make([]uuid.UUID, n), make([]uuid.UUID, n)
	onPrimary, isRead, readAcked := make([]bool, n), make([]bool, n), make([]bool, n)
	for i, s := range seeds {
		ids[i], chatIDs[i], senders[i], recipients[i] = s.id, s.chatID, s.sender, s.recipient
		onPrimary[i], isRead[i], readAcked[i] = s.onRecipientPrimary, s.read, s.readAcked
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO messages (
			id, chat_id, sender_id, recipient_id, content, message_type,
			sender_session_id, delivered_to_recipient, delivered_to_recipient_primary,
			synced_to_sender_primary,
			read_by_recipient, read_at, read_acked_by_sender,
			expires_at, created_at, updated_at
		)
		SELECT v.id, v.chat_id, v.sender_id, v.recipient_id, 'cipher', 'text',
		       $8::uuid, TRUE, v.on_primary,
		       TRUE,
		       v.is_read, CASE WHEN v.is_read THEN now() END, v.read_acked,
		       now() + interval '1 day', now(), now()
		FROM unnest($1::uuid[], $2::uuid[], $3::uuid[], $4::uuid[], $5::bool[], $6::bool[], $7::bool[])
		     AS v(id, chat_id, sender_id, recipient_id, on_primary, is_read, read_acked)`,
		ids, chatIDs, senders, recipients, onPrimary, isRead, readAcked, uuid.New())
	require.NoError(t, err)

	store := personal_chat_store.New(pool)
	svc := &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  &mockPendingUploadsChat{},
	}

	missingID := uuid.New()
	res, err := svc.AcknowledgeReadReceiptBatch(ctx, &AckReadReceiptBatchPayload{
		MessageIDs: []string{
			ackAB.id.String(), ackAC.id.String(), unreadAC.id.String(), receivedAC.id.String(),
			ackAD.id.String(), foreignBC.id.String(), missingID.String(),
		},
	}, kit.UserId{UuidUserId: alice, StringUserId: alice.String()}, true)
	require.NoError(t, err, "rows from four chats must not fail the request")
	assert.Equal(t, int32(3), res.AcknowledgedCount)
	assert.Equal(t, []string{ackAB.id.String(), ackAC.id.String(), ackAD.id.String()}, res.AcknowledgedMessageIds)
	assert.Equal(t, []string{foreignBC.id.String(), missingID.String()}, res.PermanentlyRejectedMessageIds)

	// Relay cleanup runs in the background: finished rows are deleted, and the
	// sweep of the chat of every row ACKed now deletes that chat's finished row,
	// in chat AD too.
	require.Eventually(t, func() bool {
		var left int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE id = ANY($1)`,
			[]uuid.UUID{ackAB.id, ackAC.id, doneAB.id, doneAC.id, doneAD.id}).Scan(&left); err != nil {
			return false
		}
		return left == 0
	}, 30*time.Second, 100*time.Millisecond, "relay cleanup must delete the finished rows and sweep the chat of every row ACKed now")

	require.Never(t, func() bool {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1)`, doneBC.id).Scan(&exists); err != nil {
			return false
		}
		return !exists
	}, 1500*time.Millisecond, 100*time.Millisecond, "a chat the caller is not in must never be swept, even when one of its IDs is in the request")

	readAckedNow := func(id uuid.UUID) bool {
		var acked bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT read_acked_by_sender FROM messages WHERE id = $1`, id).Scan(&acked))
		return acked
	}
	assert.True(t, readAckedNow(ackAD.id), "the row in chat AD is ACKed and stays: its recipient's primary has no copy yet")
	assert.False(t, readAckedNow(unreadAC.id), "an unread row is skipped")
	assert.False(t, readAckedNow(receivedAC.id), "a row the caller received is skipped")
	assert.False(t, readAckedNow(foreignBC.id), "someone else's row is never touched")
}
