package core_auth_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"chatbasket-api/internal/modules/core/core_auth"
	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-api/internal/platform/services"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupIntegrationDB(t *testing.T) (*pgxpool.Pool, *core_auth.AuthService) {
	_ = godotenv.Load("../../../../.env")
	_ = godotenv.Load("../../../../../.env")

	dsn := os.Getenv("DatabaseURLTesting")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL_PG_TESTING")
	}
	if dsn == "" {
		t.Skip("DatabaseURLTesting/DATABASE_URL_PG_TESTING not set")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err, "failed to connect testing db")
	t.Cleanup(func() { pool.Close() })

	_, err = pool.Exec(context.Background(), "ALTER TABLE sessions ADD COLUMN IF NOT EXISTS e2ee_public_key CHAR(44)")
	require.NoError(t, err, "failed to ensure sessions.e2ee_public_key exists in testing db")

	_, err = pool.Exec(context.Background(), "ALTER TABLE auth_users ADD COLUMN IF NOT EXISTS keys_revision INT NOT NULL DEFAULT 0")
	require.NoError(t, err, "failed to ensure auth_users.keys_revision exists in testing db")

	globalSvc := services.NewGlobalService("https://chatbasket.me")
	authSvc := core_auth.NewAuthService(globalSvc, pool, pool, []byte("test-secret"))

	return pool, authSvc
}

// cleanupCountingQuerier wraps the generated store and counts cleanup batch
// calls, so early-exit tests can assert the code path without depending on
// database network latency.
type cleanupCountingQuerier struct {
	core_auth_store.Querier
	cleanupCalls int
}

func (q *cleanupCountingQuerier) CleanupExpiredQRLoginRequestsBatch(ctx context.Context, batchSize int32) (int64, error) {
	q.cleanupCalls++
	return q.Querier.CleanupExpiredQRLoginRequestsBatch(ctx, batchSize)
}

func createTestUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	id := uuid.New()
	email := id.String() + "@test.com"
	_, err := pool.Exec(context.Background(),
		"INSERT INTO auth_users (id, email, password_hash, name, is_email_verified) VALUES ($1, $2, 'hash', 'Test User', true)",
		id, email)
	require.NoError(t, err)
	return id
}

func TestQRLoginFlow_Integration(t *testing.T) {
	pool, svc := setupIntegrationDB(t)
	ctx := context.Background()

	// 1. Initiate
	initResp, err := svc.QRInitiate(ctx)
	require.NoError(t, err)
	require.NotNil(t, initResp)
	qrTokenStr := initResp.QrToken
	require.NotEmpty(t, qrTokenStr)

	// 2. Mobile Approves Login
	testUserID := createTestUser(t, pool)
	appResp, err := svc.QRApprove(ctx, testUserID, qrTokenStr)
	require.NoError(t, err)
	assert.True(t, appResp.Status)

	// Verify DB status is APPROVED (parse UUID from signed token)
	qrUUID := uuid.MustParse(strings.Split(qrTokenStr, ".")[0])
	var approvedStatus string
	err = pool.QueryRow(ctx, "SELECT status FROM qr_login_requests WHERE id = $1", qrUUID).Scan(&approvedStatus)
	require.NoError(t, err)
	assert.Equal(t, "APPROVED", approvedStatus)

	// 3. Browser Callback (Exchange)
	cbResp, err := svc.QRCallback(ctx, qrTokenStr, "web")
	require.NoError(t, err)
	require.NotNil(t, cbResp)
	assert.NotEmpty(t, cbResp.SessionId)
	assert.Equal(t, testUserID.String(), cbResp.UserId)

	// Verify DB status is EXCHANGED
	var finalStatus string
	err = pool.QueryRow(ctx, "SELECT status FROM qr_login_requests WHERE id = $1", qrUUID).Scan(&finalStatus)
	require.NoError(t, err)
	assert.Equal(t, "EXCHANGED", finalStatus)
}

func TestQREdgeCases_Integration(t *testing.T) {
	_, svc := setupIntegrationDB(t)
	ctx := context.Background()

	// 1. Callback on non-approved token
	initResp, err := svc.QRInitiate(ctx)
	require.NoError(t, err)
	_, err = svc.QRCallback(ctx, initResp.QrToken, "web")
	assert.ErrorContains(t, err, "expired or not approved") // Should fail because status is PENDING
}

func TestQRLoginFlow_Concurrent_Integration(t *testing.T) {
	pool, svc := setupIntegrationDB(t)

	// In a real scenario, hundreds of users might be doing this at the exact same time.
	// We will spawn 50 concurrent login flows to ensure there's no DB race conditions or crosstalk.
	concurrency := 50
	errChan := make(chan error, concurrency)

	for i := range concurrency {
		go func(routineIndex int) {
			ctx := context.Background()

			// Create a unique test user for this specific goroutine
			testUserID := createTestUser(t, pool)

			// 1. Initiate
			initResp, err := svc.QRInitiate(ctx)
			if err != nil {
				errChan <- err
				return
			}
			qrToken := initResp.QrToken

			// 2. Mobile Approve
			_, err = svc.QRApprove(ctx, testUserID, qrToken)
			if err != nil {
				errChan <- err
				return
			}

			// 3. Browser Callback
			cbResp, err := svc.QRCallback(ctx, qrToken, "web")
			if err != nil {
				errChan <- err
				return
			}

			// CRITICAL: Ensure this specific browser received the exact User ID of its paired mobile app!
			if cbResp.UserId != testUserID.String() {
				errChan <- fmt.Errorf("CROSSTALK DETECTED: expected user %s but got %s", testUserID.String(), cbResp.UserId)
				return
			}

			errChan <- nil // Success
		}(i)
	}

	for range concurrency {
		err := <-errChan
		require.NoError(t, err, "A concurrent worker failed")
	}
}

func TestCleanupQRLoginRequests_Integration(t *testing.T) {
	pool, svc := setupIntegrationDB(t)
	ctx := context.Background()

	expiredID := uuid.New()
	activeID := uuid.New()

	// Insert one expired QR request and one active QR request
	_, err := pool.Exec(ctx, `
		INSERT INTO qr_login_requests (id, status, expires_at, created_at, updated_at)
		VALUES
			($1, 'PENDING', now() - interval '10 minutes', now() - interval '15 minutes', now() - interval '15 minutes'),
			($2, 'PENDING', now() + interval '10 minutes', now(), now())
	`, expiredID, activeID)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM qr_login_requests WHERE id IN ($1, $2)", expiredID, activeID)
	})

	// Run cleanup
	err = svc.CleanupQRLoginRequests(ctx)
	require.NoError(t, err)

	// Verify expired record was deleted
	var expiredCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM qr_login_requests WHERE id = $1", expiredID).Scan(&expiredCount)
	require.NoError(t, err)
	assert.Equal(t, 0, expiredCount, "expired QR request should be deleted")

	// Verify active record was NOT deleted
	var activeCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM qr_login_requests WHERE id = $1", activeID).Scan(&activeCount)
	require.NoError(t, err)
	assert.Equal(t, 1, activeCount, "active QR request should be preserved")
}

// TestCleanupQRLoginRequests_Live_EarlyExit verifies against real PostgreSQL:
// 1. With 0 expired rows, cleanup completes immediately without sleeping.
// 2. With small rows (< 5000), cleanup deletes all rows and exits immediately without throttle sleep.
func TestCleanupQRLoginRequests_Live_EarlyExit(t *testing.T) {
	pool, svc := setupIntegrationDB(t)
	ctx := context.Background()

	cleanupCounter := &cleanupCountingQuerier{Querier: svc.MaintenanceQuerier}
	svc.MaintenanceQuerier = cleanupCounter

	testUser := createTestUser(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM qr_login_requests WHERE auth_user_id = $1", testUser)
		_, _ = pool.Exec(context.Background(), "DELETE FROM auth_users WHERE id = $1", testUser)
	})

	// Scenario 1: Zero expired rows -> one batch query, then early exit before any throttle sleep.
	err := svc.CleanupQRLoginRequests(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, cleanupCounter.cleanupCalls, "0-row cleanup must issue exactly one batch query and exit early")

	// Scenario 2: Small batch (5 expired rows < 5000)
	_, err = pool.Exec(ctx, `
		INSERT INTO qr_login_requests (id, auth_user_id, status, expires_at, created_at, updated_at)
		SELECT gen_random_uuid(), $1, 'PENDING', now() - interval '1 hour', now() - interval '2 hours', now() - interval '2 hours'
		FROM generate_series(1, 5)
	`, testUser)
	require.NoError(t, err)

	cleanupCounter.cleanupCalls = 0
	err = svc.CleanupQRLoginRequests(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, cleanupCounter.cleanupCalls, "small batch cleanup must issue exactly one batch query and exit early")

	var remaining int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM qr_login_requests WHERE auth_user_id = $1", testUser).Scan(&remaining)
	require.NoError(t, err)
	assert.Equal(t, 0, remaining, "all 5 expired rows must be deleted")
}

// TestCleanupQRLoginRequests_Live_BatchedBacklogAndNextRun verifies against real PostgreSQL:
//  1. A bounded run makes whole-batch progress on a large backlog and exits cleanly
//     even when the soft budget stops it before the backlog is empty.
//  2. A follow-up run with the normal budget removes whatever remains.
//
// The exact soft-cutoff point is covered deterministically by
// TestCleanupQRLoginRequests_BudgetSoftLimit. This live test only asserts
// whole-batch progress so it does not depend on Neon latency.
func TestCleanupQRLoginRequests_Live_BatchedBacklogAndNextRun(t *testing.T) {
	pool, svc := setupIntegrationDB(t)
	ctx := context.Background()

	testUser := createTestUser(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM qr_login_requests WHERE auth_user_id = $1", testUser)
		_, _ = pool.Exec(context.Background(), "DELETE FROM auth_users WHERE id = $1", testUser)
	})

	// Insert 12,000 expired rows (requires 3 batches: 5000 + 5000 + 2000)
	_, err := pool.Exec(ctx, `
		INSERT INTO qr_login_requests (id, auth_user_id, status, expires_at, created_at, updated_at)
		SELECT gen_random_uuid(), $1, 'PENDING', now() - interval '1 hour', now() - interval '2 hours', now() - interval '2 hours'
		FROM generate_series(1, 12000)
	`, testUser)
	require.NoError(t, err)

	var initialCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM qr_login_requests WHERE auth_user_id = $1", testUser).Scan(&initialCount)
	require.NoError(t, err)
	require.Equal(t, 12000, initialCount)

	// RUN 1: one-second budget. At least one 5,000-row batch fits; the run then
	// stops on the soft budget or after the final partial batch. Because batches
	// are 5,000 rows, a completed run can only delete 5,000, 10,000, or 12,000.
	run1Start := time.Now()
	err = svc.CleanupQRLoginRequestsWithBudget(ctx, time.Second)
	run1Elapsed := time.Since(run1Start)
	require.NoError(t, err, "Run 1 must finish cleanly without timeout error")
	assert.Less(t, run1Elapsed, 5*time.Second, "Run 1 must exit within its safety buffer")

	var afterRun1Count int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM qr_login_requests WHERE auth_user_id = $1", testUser).Scan(&afterRun1Count)
	require.NoError(t, err)
	deleted := initialCount - afterRun1Count
	assert.Contains(t, []int{5000, 10000, 12000}, deleted, "Run 1 must delete whole batches and make progress")

	// RUN 2: Next scheduled interval (runs with full normal budget).
	// Clears whatever Run 1 left behind and exits early when done.
	err = svc.CleanupQRLoginRequests(ctx)
	require.NoError(t, err, "Run 2 must finish cleanly")

	var afterRun2Count int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM qr_login_requests WHERE auth_user_id = $1", testUser).Scan(&afterRun2Count)
	require.NoError(t, err)
	assert.Equal(t, 0, afterRun2Count, "Run 2 must clean up all remaining rows completely")
}
