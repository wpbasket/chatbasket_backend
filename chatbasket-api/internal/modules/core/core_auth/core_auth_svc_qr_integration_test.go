package core_auth_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"chatbasket-api/internal/modules/core/core_auth"
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

	globalSvc := services.NewGlobalService()
	authSvc := core_auth.NewAuthService(globalSvc, pool, []byte("test-secret"))

	return pool, authSvc
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
	qrTokenStr := initResp.QRToken
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
	assert.NotEmpty(t, cbResp.SessionID)
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
	_, err = svc.QRCallback(ctx, initResp.QRToken, "web")
	assert.ErrorContains(t, err, "expired or not approved") // Should fail because status is PENDING
}

func TestQRLoginFlow_Concurrent_Integration(t *testing.T) {
	pool, svc := setupIntegrationDB(t)

	// In a real scenario, hundreds of users might be doing this at the exact same time.
	// We will spawn 50 concurrent login flows to ensure there's no DB race conditions or crosstalk.
	concurrency := 50
	errChan := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
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
			qrToken := initResp.QRToken

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

	for i := 0; i < concurrency; i++ {
		err := <-errChan
		require.NoError(t, err, "A concurrent worker failed")
	}
}
