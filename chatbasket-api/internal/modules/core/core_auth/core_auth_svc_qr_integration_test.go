package core_auth_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"chatbasket-api/internal/modules/core/core_auth"
	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-api/internal/platform/services"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupIntegrationDB(t *testing.T) (*pgxpool.Pool, *core_auth.AuthService) {
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

	// 2. Browser sets Offer
	offerSDP := "real-browser-sdp"
	sigResp1, err := svc.QRSignal(ctx, &core_auth.QRSignalPayload{
		QRToken: qrTokenStr,
		Role:    "browser",
		SDP:     offerSDP,
	})
	require.NoError(t, err)
	assert.Equal(t, "OFFER_SAVED", sigResp1.Status)

	// 3. Mobile gets Offer
	sigResp2, err := svc.QRSignal(ctx, &core_auth.QRSignalPayload{
		QRToken: qrTokenStr,
		Role:    "mobile",
		SDP:     "",
	})
	require.NoError(t, err)
	assert.Equal(t, offerSDP, sigResp2.SDP)

	// 4. Mobile sets Answer
	answerSDP := "real-mobile-sdp"
	sigResp3, err := svc.QRSignal(ctx, &core_auth.QRSignalPayload{
		QRToken: qrTokenStr,
		Role:    "mobile",
		SDP:     answerSDP,
	})
	require.NoError(t, err)
	assert.Equal(t, "ANSWER_SAVED", sigResp3.Status)

	// 5. Browser gets Answer
	sigResp4, err := svc.QRSignal(ctx, &core_auth.QRSignalPayload{
		QRToken: qrTokenStr,
		Role:    "browser",
		SDP:     "",
	})
	require.NoError(t, err)
	assert.Equal(t, answerSDP, sigResp4.SDP)

	// 6. Mobile Approves Login
	testUserID := createTestUser(t, pool)
	appResp, err := svc.QRApprove(ctx, testUserID, qrTokenStr)
	require.NoError(t, err)
	assert.True(t, appResp.Status)

	// Verify DB status is APPROVED
	qrUUID := uuid.MustParse(qrTokenStr)
	store := core_auth_store.New(pool)
	req, err := store.GetQRLoginRequest(ctx, qrUUID)
	require.NoError(t, err)
	assert.Equal(t, "APPROVED", req.Status)

	// 7. Browser Callback (Exchange)
	cbResp, err := svc.QRCallback(ctx, qrTokenStr, "web")
	require.NoError(t, err)
	require.NotNil(t, cbResp)
	assert.NotEmpty(t, cbResp.SessionID)
	assert.Equal(t, testUserID, cbResp.AuthUser.ID)

	// Verify DB status is EXCHANGED
	// Actually GetQRLoginRequest only fetches non-exchanged tokens or valid tokens. Let's do raw query.
	var finalStatus string
	err = pool.QueryRow(ctx, "SELECT status FROM qr_login_requests WHERE id = $1", qrUUID).Scan(&finalStatus)
	require.NoError(t, err)
	assert.Equal(t, "EXCHANGED", finalStatus)
}

func TestQREdgeCases_Integration(t *testing.T) {
	_, svc := setupIntegrationDB(t)
	ctx := context.Background()

	// 1. Invalid UUID
	_, err := svc.QRSignal(ctx, &core_auth.QRSignalPayload{
		QRToken: "invalid-uuid-string",
		Role:    "browser",
		SDP:     "sdp",
	})
	assert.Error(t, err)

	// 2. Not Found / Expired
	missingID := uuid.New().String()
	_, err = svc.QRSignal(ctx, &core_auth.QRSignalPayload{
		QRToken: missingID,
		Role:    "browser",
		SDP:     "sdp",
	})
	assert.ErrorContains(t, err, "expired")

	// 3. Callback on non-approved token
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
			if err != nil { errChan <- err; return }
			qrToken := initResp.QRToken

			// 2. Browser Offer
			offerSDP := "offer-" + qrToken
			_, err = svc.QRSignal(ctx, &core_auth.QRSignalPayload{QRToken: qrToken, Role: "browser", SDP: offerSDP})
			if err != nil { errChan <- err; return }

			// 3. Mobile Gets Offer
			getOff, err := svc.QRSignal(ctx, &core_auth.QRSignalPayload{QRToken: qrToken, Role: "mobile", SDP: ""})
			if err != nil || getOff.SDP != offerSDP { errChan <- fmt.Errorf("mobile offer mismatch"); return }

			// 4. Mobile Answer
			answerSDP := "answer-" + qrToken
			_, err = svc.QRSignal(ctx, &core_auth.QRSignalPayload{QRToken: qrToken, Role: "mobile", SDP: answerSDP})
			if err != nil { errChan <- err; return }

			// 5. Browser Gets Answer
			getAns, err := svc.QRSignal(ctx, &core_auth.QRSignalPayload{QRToken: qrToken, Role: "browser", SDP: ""})
			if err != nil || getAns.SDP != answerSDP { errChan <- fmt.Errorf("browser answer mismatch"); return }

			// 6. Mobile Approve
			_, err = svc.QRApprove(ctx, testUserID, qrToken)
			if err != nil { errChan <- err; return }

			// 7. Browser Callback
			cbResp, err := svc.QRCallback(ctx, qrToken, "web")
			if err != nil { errChan <- err; return }

			// CRITICAL: Ensure this specific browser received the exact User ID of its paired mobile app!
			if cbResp.AuthUser.ID != testUserID {
				errChan <- fmt.Errorf("CROSSTALK DETECTED: expected user %s but got %s", testUserID, cbResp.AuthUser.ID)
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
