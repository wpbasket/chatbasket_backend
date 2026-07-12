package personal_chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCleanupDatabaseOnlyIntegration(t *testing.T) {
	dbURL := getTestDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	// 1. Connect to database
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to Neon DB: %v", err)
	}
	defer pool.Close()

	t.Log("Connected to database successfully for integration testing.")

	// 2. Generate UUIDs for mock data
	blockerID := uuid.New()
	blockedID := uuid.New()
	chatID := uuid.New()
	userBlockID := uuid.New()

	msgExpiredID := uuid.New()
	msgAckedID := uuid.New()
	msgBlockedID := uuid.New()

	syncActionBlockedID := uuid.New()
	syncActionOldID := uuid.New()
	historySyncID := uuid.New()
	sessionID := uuid.New()

	// Compute unique 64-character hex usernames using SHA-256
	blockerSum := sha256.Sum256([]byte(blockerID.String()))
	blockedSum := sha256.Sum256([]byte(blockedID.String()))
	blockerUsername := hex.EncodeToString(blockerSum[:])
	blockedUsername := hex.EncodeToString(blockedSum[:])

	// Order participants to satisfy chats_ordered_pair check constraint
	p1ID := blockerID
	p2ID := blockedID
	if p1ID.String() > p2ID.String() {
		p1ID, p2ID = p2ID, p1ID
	}

	// 3. Insert mock data
	t.Log("Inserting mock users, block, chat, and messages...")

	// Create auth users first (for foreign keys)
	blockerEmail := fmt.Sprintf("blocker-%s@test.com", blockerID.String())
	blockedEmail := fmt.Sprintf("blocked-%s@test.com", blockedID.String())
	_, err = pool.Exec(ctx, `
		INSERT INTO auth_users (id, name, email, password_hash, is_email_verified, keys_revision, created_at, updated_at) 
		VALUES ($1, 'Test Blocker', $2, 'hash', true, 1, now(), now()),
		       ($3, 'Test Blocked', $4, 'hash', true, 1, now(), now())`,
		blockerID, blockerEmail, blockedID, blockedEmail)
	if err != nil {
		t.Fatalf("Failed to insert auth_users: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM auth_users WHERE id IN ($1, $2)", blockerID, blockedID)

	// Create blocker and blocked users
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, name, profile_type, hmac_sha256_hex_username, b64_cipher_chacha20poly1305_username, created_at, updated_at) 
		VALUES ($1, 'Test Blocker', 'personal', $2, 'cipher1', now(), now()),
		       ($3, 'Test Blocked', 'personal', $4, 'cipher2', now(), now())`,
		blockerID, blockerUsername, blockedID, blockedUsername)
	if err != nil {
		t.Fatalf("Failed to insert users: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM users WHERE id IN ($1, $2)", blockerID, blockedID)

	// Create session for history_sync reference
	tokenHash := fmt.Sprintf("hash-%s", sessionID.String())
	_, err = pool.Exec(ctx, `
		INSERT INTO sessions (id, auth_user_id, token_hash, is_central, expires_at, created_at, updated_at) 
		VALUES ($1, $2, $3, false, now() + INTERVAL '1 day', now(), now())`,
		sessionID, blockerID, tokenHash)
	if err != nil {
		t.Fatalf("Failed to insert session: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM sessions WHERE id = $1", sessionID)

	// Create chat between blocker and blocked (using ordered IDs)
	_, err = pool.Exec(ctx, `
		INSERT INTO chats (id, participant_1_id, participant_2_id, p1_unread_count, p2_unread_count, created_at, updated_at) 
		VALUES ($1, $2, $3, 0, 0, now(), now())`,
		chatID, p1ID, p2ID)
	if err != nil {
		t.Fatalf("Failed to insert chat: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM chats WHERE id = $1", chatID)

	// Create blocker->blocked block
	_, err = pool.Exec(ctx, `
		INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id, created_at, updated_at) 
		VALUES ($1, $2, $3, now(), now())`,
		userBlockID, blockerID, blockedID)
	if err != nil {
		t.Fatalf("Failed to insert block: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM user_blocks WHERE id = $1", userBlockID)

	// Create Message A: Expired (no file_id)
	_, err = pool.Exec(ctx, `
		INSERT INTO messages (id, chat_id, sender_id, recipient_id, content, message_type, delivered_to_recipient, delivered_to_recipient_primary, synced_to_sender_primary, expires_at, created_at, updated_at) 
		VALUES ($1, $2, $3, $4, 'Expired Msg', 'text', true, true, true, now() - INTERVAL '1 day', now(), now())`,
		msgExpiredID, chatID, blockerID, blockedID)
	if err != nil {
		t.Fatalf("Failed to insert expired message: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM messages WHERE id = $1", msgExpiredID)

	// Create Message B: Fully Acknowledged (no file_id, expires in future)
	_, err = pool.Exec(ctx, `
		INSERT INTO messages (id, chat_id, sender_id, recipient_id, content, message_type, delivered_to_recipient, delivered_to_recipient_primary, synced_to_sender_primary, expires_at, created_at, updated_at) 
		VALUES ($1, $2, $3, $4, 'Acked Msg', 'text', true, true, true, now() + INTERVAL '1 day', now(), now())`,
		msgAckedID, chatID, blockerID, blockedID)
	if err != nil {
		t.Fatalf("Failed to insert acknowledged message: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM messages WHERE id = $1", msgAckedID)

	// Create Message C: Blocked User Chat Message (no file_id, expires in future)
	_, err = pool.Exec(ctx, `
		INSERT INTO messages (id, chat_id, sender_id, recipient_id, content, message_type, delivered_to_recipient, delivered_to_recipient_primary, synced_to_sender_primary, expires_at, created_at, updated_at) 
		VALUES ($1, $2, $3, $4, 'Blocked Msg', 'text', false, false, false, now() + INTERVAL '1 day', now(), now())`,
		msgBlockedID, chatID, blockerID, blockedID)
	if err != nil {
		t.Fatalf("Failed to insert blocked chat message: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM messages WHERE id = $1", msgBlockedID)

	// Create Sync Action A: Blocked User Chat Sync Action
	syncActionBlockedPayload := fmt.Sprintf(`{"chatId": "%s"}`, chatID.String())
	_, err = pool.Exec(ctx, `
		INSERT INTO message_sync_actions (id, user_id, action_type, payload, delivered_to_primary, created_at, updated_at) 
		VALUES ($1, $2, 'unsend', $3::jsonb, false, now(), now())`,
		syncActionBlockedID, blockerID, syncActionBlockedPayload)
	if err != nil {
		t.Fatalf("Failed to insert blocked sync action: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM message_sync_actions WHERE id = $1", syncActionBlockedID)

	// Create Sync Action B: Old Sync Action (>30 days old)
	syncActionOldPayload := `{"chatId": "00000000-0000-0000-0000-000000000000"}`
	_, err = pool.Exec(ctx, `
		INSERT INTO message_sync_actions (id, user_id, action_type, payload, delivered_to_primary, created_at, updated_at) 
		VALUES ($1, $2, 'unsend', $3::jsonb, false, now(), now())`,
		syncActionOldID, blockerID, syncActionOldPayload)
	if err != nil {
		t.Fatalf("Failed to insert old sync action: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM message_sync_actions WHERE id = $1", syncActionOldID)

	// Bypassing triggers to manually update the created_at of Old Sync Action to 31 days ago
	_, err = pool.Exec(ctx, "ALTER TABLE message_sync_actions DISABLE TRIGGER sync_actions_timestamps_trigger")
	if err != nil {
		t.Fatalf("Failed to disable trigger: %v", err)
	}
	_, err = pool.Exec(ctx, "UPDATE message_sync_actions SET created_at = now() - INTERVAL '31 days' WHERE id = $1", syncActionOldID)
	if err != nil {
		pool.Exec(ctx, "ALTER TABLE message_sync_actions ENABLE TRIGGER sync_actions_timestamps_trigger")
		t.Fatalf("Failed to update created_at for old sync action: %v", err)
	}
	_, err = pool.Exec(ctx, "ALTER TABLE message_sync_actions ENABLE TRIGGER sync_actions_timestamps_trigger")
	if err != nil {
		t.Fatalf("Failed to re-enable trigger: %v", err)
	}

	// Create Expired History Sync
	_, err = pool.Exec(ctx, `
		INSERT INTO history_sync (id, user_id, session_id, chats_json, payload, expires_at, created_at, updated_at) 
		VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, now() - INTERVAL '1 hour', now(), now())`,
		historySyncID, blockerID, sessionID)
	if err != nil {
		t.Fatalf("Failed to insert expired history sync: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM history_sync WHERE id = $1", historySyncID)

	// 4. Initialize Chat Service
	s := NewChatService(nil, pool, nil, nil, nil, nil, nil)

	// Update statistics so query planner uses expression indexes for bulk tables
	_, err = pool.Exec(ctx, "ANALYZE message_sync_actions")
	if err != nil {
		t.Logf("Warning: ANALYZE failed: %v", err)
	}

	// 5. Execute database-only cleanup
	t.Log("Executing CleanupDatabaseOnly...")
	err = s.CleanupDatabaseOnly(ctx)
	if err != nil {
		t.Fatalf("CleanupDatabaseOnly failed: %v", err)
	}

	// 6. Verify Deletions
	t.Log("Verifying row deletions...")

	var count int

	// Check Expired Message
	err = pool.QueryRow(ctx, "SELECT count(*) FROM messages WHERE id = $1", msgExpiredID).Scan(&count)
	if err != nil {
		t.Errorf("Failed to check expired message: %v", err)
	} else if count != 0 {
		t.Error("Expired message was not deleted!")
	}

	// Check Acknowledged Message
	err = pool.QueryRow(ctx, "SELECT count(*) FROM messages WHERE id = $1", msgAckedID).Scan(&count)
	if err != nil {
		t.Errorf("Failed to check acknowledged message: %v", err)
	} else if count != 0 {
		t.Error("Acknowledged message was not deleted!")
	}

	// Check Blocked Message
	err = pool.QueryRow(ctx, "SELECT count(*) FROM messages WHERE id = $1", msgBlockedID).Scan(&count)
	if err != nil {
		t.Errorf("Failed to check blocked message: %v", err)
	} else if count != 0 {
		t.Error("Blocked message was not deleted!")
	}

	// Check Blocked Sync Action
	err = pool.QueryRow(ctx, "SELECT count(*) FROM message_sync_actions WHERE id = $1", syncActionBlockedID).Scan(&count)
	if err != nil {
		t.Errorf("Failed to check blocked sync action: %v", err)
	} else if count != 0 {
		t.Error("Blocked sync action was not deleted!")
	}

	// Check Old Sync Action
	err = pool.QueryRow(ctx, "SELECT count(*) FROM message_sync_actions WHERE id = $1", syncActionOldID).Scan(&count)
	if err != nil {
		t.Errorf("Failed to check old sync action: %v", err)
	} else if count != 0 {
		t.Error("Old sync action was not deleted!")
	}

	// Check Expired History Sync
	err = pool.QueryRow(ctx, "SELECT count(*) FROM history_sync WHERE id = $1", historySyncID).Scan(&count)
	if err != nil {
		t.Errorf("Failed to check expired history sync: %v", err)
	} else if count != 0 {
		t.Error("Expired history sync was not deleted!")
	}

	t.Log("All deletions verified successfully!")
}

func TestDeleteBatchUntilDoneTimingAndBudgets(t *testing.T) {
	ctx := context.Background()

	// Edge Case 1: Normal execution (finishes cleanly under budget)
	t.Run("NormalExecutionUnderBudget", func(t *testing.T) {
		callCount := 0
		deleteFn := func(c context.Context, limit int32) (int64, error) {
			callCount++
			if callCount == 1 {
				return 5, nil
			}
			return 0, nil
		}

		oldThrottle := cleanupThrottleSleep
		cleanupThrottleSleep = 1 * time.Millisecond
		defer func() { cleanupThrottleSleep = oldThrottle }()

		finished := deleteBatchUntilDone(ctx, deleteFn, 100*time.Millisecond, "test-normal")
		if !finished {
			t.Error("Expected finished to be true since rows hit 0")
		}
		if callCount != 2 {
			t.Errorf("Expected deleteFn to be called twice, got %d", callCount)
		}
	})

	// Edge Case 2: Exceeding 90% soft limit stops scheduling new queries
	t.Run("ExceedsSoftLimitAndStops", func(t *testing.T) {
		oldThrottle := cleanupThrottleSleep
		cleanupThrottleSleep = 1 * time.Millisecond
		defer func() { cleanupThrottleSleep = oldThrottle }()

		deleteFn := func(c context.Context, limit int32) (int64, error) {
			// Simulate a query that takes 40ms
			select {
			case <-c.Done():
				return 0, c.Err()
			case <-time.After(40 * time.Millisecond):
			}
			return 10, nil // Always returns rows deleted so it wants to keep looping
		}

		// Budget is 50ms. Work budget is 45ms.
		// Iteration 1: elapsed = 0 (which is < 45ms). Runs query for 40ms.
		// Throttle sleep: 1ms. Total elapsed = 41ms.
		// Iteration 2: elapsed = 41ms (which is < 45ms). Runs query for 40ms.
		// Total elapsed = 81ms.
		// Iteration 3: elapsed = 81ms (which is >= 45ms). Stops!
		finished := deleteBatchUntilDone(ctx, deleteFn, 50*time.Millisecond, "test-soft-limit")
		if finished {
			t.Error("Expected finished to be false because work budget was exceeded")
		}
	})

	// Edge Case 3: Query-level context timeout (100% hard limit)
	t.Run("QueryHardTimeoutTriggered", func(t *testing.T) {
		oldThrottle := cleanupThrottleSleep
		cleanupThrottleSleep = 1 * time.Millisecond
		defer func() { cleanupThrottleSleep = oldThrottle }()

		deleteFn := func(c context.Context, limit int32) (int64, error) {
			// Query runs for 200ms, which is longer than the budget
			select {
			case <-c.Done():
				return 0, c.Err()
			case <-time.After(200 * time.Millisecond):
			}
			return 10, nil
		}

		// Budget is 50ms. Work budget is 45ms.
		// Iteration 1: starts at elapsed = 0.
		// Time left passed to queryCtx is 50ms.
		// Query starts, sleeps. Since it takes 200ms, queryCtx will time out after 50ms.
		// deleteFn will return context.DeadlineExceeded error.
		start := time.Now()
		finished := deleteBatchUntilDone(ctx, deleteFn, 50*time.Millisecond, "test-hard-timeout")
		duration := time.Since(start)

		if finished {
			t.Error("Expected finished to be false due to hard timeout error")
		}
		// The test should take roughly 50ms (+/- scheduling overhead), not 200ms!
		if duration >= 150*time.Millisecond {
			t.Errorf("Hard timeout failed, execution took too long: %v", duration)
		}
	})

	// Edge Case 4: Extremely small budget (microsecond budget) exits immediately
	t.Run("MicrosecondBudgetExitsImmediately", func(t *testing.T) {
		deleteFn := func(c context.Context, limit int32) (int64, error) {
			return 10, nil
		}

		finished := deleteBatchUntilDone(ctx, deleteFn, 1*time.Microsecond, "test-micro-budget")
		if finished {
			t.Error("Expected finished to be false since budget is immediately exhausted")
		}
	})
}

func TestCleanupDatabaseOnlyCancellation(t *testing.T) {
	dbURL := getTestDSN(t)

	// Create an already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel it immediately

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to Neon DB: %v", err)
	}
	defer pool.Close()

	s := NewChatService(nil, pool, nil, nil, nil, nil, nil)

	// Since ctx is cancelled, CleanupDatabaseOnly must immediately abort and return context.Canceled
	err = s.CleanupDatabaseOnly(ctx)
	if err == nil {
		t.Error("Expected CleanupDatabaseOnly to fail with context.Canceled, but it succeeded")
	} else if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
}

func getTestDSN(t *testing.T) string {
	dsn := os.Getenv("DatabaseURLTesting")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL_PG_TESTING")
	}
	if dsn == "" {
		t.Skip("DatabaseURLTesting/DATABASE_URL_PG_TESTING environment variable not set, skipping database test")
	}
	return dsn
}

