package core_auth

import (
	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// rateLimiterMock satisfies core_auth_store.Querier for testing purposes.
type rateLimiterMock struct {
	core_auth_store.Querier
	limiter core_auth_store.AuthRateLimiter
	err     error
}

func (m *rateLimiterMock) GetAuthRateLimiter(ctx context.Context, authUserID uuid.UUID) (core_auth_store.AuthRateLimiter, error) {
	return m.limiter, m.err
}

func (m *rateLimiterMock) UpsertAuthRateLimiterSend(ctx context.Context, arg core_auth_store.UpsertAuthRateLimiterSendParams) (core_auth_store.AuthRateLimiter, error) {
	m.limiter.OtpHourlyCount = arg.OtpHourlyCount
	m.limiter.Otp24hCount = arg.Otp24hCount
	if arg.LastOtpSendAt != nil {
		m.limiter.LastOtpSendAt = arg.LastOtpSendAt
	}
	m.limiter.Otp24hWindowStartAt = arg.Otp24hWindowStartAt
	return m.limiter, nil
}

func (m *rateLimiterMock) UpsertAuthRateLimiterVerify(ctx context.Context, arg core_auth_store.UpsertAuthRateLimiterVerifyParams) (core_auth_store.AuthRateLimiter, error) {
	m.limiter.OtpVerifyErrors = arg.OtpVerifyErrors
	if arg.LastVerifyAttemptAt != nil {
		m.limiter.LastVerifyAttemptAt = arg.LastVerifyAttemptAt
	}
	return m.limiter, nil
}

func (m *rateLimiterMock) ResetVerifyErrors(ctx context.Context, authUserID uuid.UUID) error {
	m.limiter.OtpVerifyErrors = 0
	m.limiter.LastVerifyAttemptAt = nil
	return nil
}

func TestOTP_RateLimiting(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	mock := &rateLimiterMock{
		err: pgx.ErrNoRows, // Start with no record
	}
	svc := &AuthService{
		PostgresQuerier: mock,
	}

	t.Run("Cooldown_Enforcement", func(t *testing.T) {
		// 1. First send (Success)
		err := svc.CheckOTPSendRateLimit(ctx, userID)
		if err != nil {
			t.Fatalf("First send failed: %v", err)
		}
		mock.err = nil // Record now exists

		// 2. Immediate second send (Should fail)
		err = svc.CheckOTPSendRateLimit(ctx, userID)
		if err == nil {
			t.Fatal("Expected cooldown error, got nil")
		}

		pe, ok := err.(kit.ProcessedError)
		if !ok || pe.Status() != http.StatusTooManyRequests || pe.Kind() != "cooldown_active" {
			t.Errorf("Wrong error type: status=%v, kind=%v", pe.Status(), pe.Kind())
		}
	})

	t.Run("Hourly_Limit_Enforcement", func(t *testing.T) {
		// Reset mock to have 5 sends in last 10 minutes
		lastSend := time.Now().Add(-10 * time.Minute)
		otp24hWindowStart := time.Now().Add(-1 * time.Hour)
		mock.limiter = core_auth_store.AuthRateLimiter{
			OtpHourlyCount:      5,
			Otp24hCount:         5,
			LastOtpSendAt:       &lastSend,
			Otp24hWindowStartAt: &otp24hWindowStart,
		}
		mock.err = nil

		err := svc.CheckOTPSendRateLimit(ctx, userID)
		if err == nil {
			t.Fatal("Expected hourly limit error, got nil")
		}

		pe, _ := err.(kit.ProcessedError)
		if pe.Kind() != "hourly_limit_exceeded" {
			t.Errorf("Expected hourly_limit_exceeded, got %v", pe.Kind())
		}
	})

	t.Run("Rolling24h_Limit_Enforcement", func(t *testing.T) {
		// Reset mock to have 10 sends inside the current rolling 24h window.
		lastSend := time.Now().Add(-1 * time.Hour)
		otp24hWindowStart := time.Now().Add(-2 * time.Hour)
		mock.limiter = core_auth_store.AuthRateLimiter{
			OtpHourlyCount:      5,
			Otp24hCount:         10,
			LastOtpSendAt:       &lastSend,
			Otp24hWindowStartAt: &otp24hWindowStart,
		}

		err := svc.CheckOTPSendRateLimit(ctx, userID)
		if err == nil {
			t.Fatal("Expected 24h limit error, got nil")
		}

		pe, _ := err.(kit.ProcessedError)
		if pe.Kind() != "daily_limit_exceeded" {
			t.Errorf("Expected daily_limit_exceeded, got %v", pe.Kind())
		}
		expectedRetryAt := otp24hWindowStart.Add(24 * time.Hour).UTC().Format(time.RFC3339)
		if !strings.Contains(pe.Error(), expectedRetryAt) {
			t.Errorf("Expected retry time %q in error message, got %q", expectedRetryAt, pe.Error())
		}
	})

	t.Run("Rolling24h_Limit_Allows_10th_Send", func(t *testing.T) {
		// Reset mock to have 9 sends in the current rolling 24h window. The 10th send is still allowed.
		lastSend := time.Now().Add(-2 * time.Hour)
		otp24hWindowStart := time.Now().Add(-2 * time.Hour)
		mock.limiter = core_auth_store.AuthRateLimiter{
			OtpHourlyCount:      1,
			Otp24hCount:         9,
			LastOtpSendAt:       &lastSend,
			Otp24hWindowStartAt: &otp24hWindowStart,
		}

		err := svc.CheckOTPSendRateLimit(ctx, userID)
		if err != nil {
			t.Fatalf("Expected 10th send success, got error: %v", err)
		}
		if mock.limiter.Otp24hCount != 10 {
			t.Errorf("24h count should be 10 after allowed 10th send, got %v", mock.limiter.Otp24hCount)
		}
	})

	t.Run("Rolling24h_Does_Not_Reset_Before_24Hours", func(t *testing.T) {
		// otp_24h_window_start_at can be on the previous UTC calendar day, but the rolling
		// 24h window must not reset until 24 hours pass.
		nowUTC := time.Now().UTC()
		todayMidnightUTC := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)
		prevDayLate := todayMidnightUTC.Add(-1 * time.Second) // yesterday 23:59:59 UTC
		lastSend := time.Now().Add(-2 * time.Hour)            // >5s ago, >1h ago to avoid cooldown/hourly
		mock.limiter = core_auth_store.AuthRateLimiter{
			OtpHourlyCount:      2, // below hourly cap so we isolate the daily check
			Otp24hCount:         10,
			LastOtpSendAt:       &lastSend,
			Otp24hWindowStartAt: &prevDayLate,
		}

		err := svc.CheckOTPSendRateLimit(ctx, userID)
		if err == nil {
			t.Fatal("Expected 24h limit error before rolling 24h window expired, got nil")
		}
		pe, _ := err.(kit.ProcessedError)
		if pe.Kind() != "daily_limit_exceeded" {
			t.Errorf("Expected daily_limit_exceeded, got %v", pe.Kind())
		}
	})

	t.Run("Verify_Lockout_Enforcement", func(t *testing.T) {
		// 3 failed attempts in last minute
		lastVerify := time.Now().Add(-1 * time.Minute)
		mock.limiter = core_auth_store.AuthRateLimiter{
			OtpVerifyErrors:     3,
			LastVerifyAttemptAt: &lastVerify,
		}

		err := svc.CheckOTPVerifyRateLimit(ctx, userID)
		if err == nil {
			t.Fatal("Expected verification locked error, got nil")
		}

		pe, _ := err.(kit.ProcessedError)
		// Production code emits "brute_force_lockout" from CheckOTPVerifyRateLimit.
		if pe.Kind() != "brute_force_lockout" {
			t.Errorf("Expected brute_force_lockout, got %v", pe.Kind())
		}

		// Simulate lockout expiry (16 minutes later)
		oldVerify := time.Now().Add(-16 * time.Minute)
		mock.limiter.LastVerifyAttemptAt = &oldVerify
		err = svc.CheckOTPVerifyRateLimit(ctx, userID)
		if err != nil {
			t.Errorf("Expected success after lockout expiry, got %v", err)
		}
	})

	t.Run("Rolling24h_Limit_Reset", func(t *testing.T) {
		// Simulate reaching limit yesterday
		yesterday := time.Now().Add(-25 * time.Hour)
		otp24hWindowStart := time.Now().Add(-25 * time.Hour)
		mock.limiter = core_auth_store.AuthRateLimiter{
			OtpHourlyCount:      5,
			Otp24hCount:         10,
			LastOtpSendAt:       &yesterday,
			Otp24hWindowStartAt: &otp24hWindowStart, // Expired reset (>24h)
		}

		err := svc.CheckOTPSendRateLimit(ctx, userID)
		if err != nil {
			t.Fatalf("Expected reset success, got error: %v", err)
		}
		if mock.limiter.Otp24hCount != 1 {
			t.Errorf("24h count should have reset to 1, got %v", mock.limiter.Otp24hCount)
		}
	})

	t.Run("Verify_Success_Resets_Errors", func(t *testing.T) {
		// 2 failures
		mock.limiter = core_auth_store.AuthRateLimiter{
			OtpVerifyErrors: 2,
		}

		err := svc.ResetVerifyErrors(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		if mock.limiter.OtpVerifyErrors != 0 {
			t.Errorf("Expected 0 errors, got %v", mock.limiter.OtpVerifyErrors)
		}
	})

	t.Run("Verify_Mistake_Counter_Increment", func(t *testing.T) {
		mock.limiter = core_auth_store.AuthRateLimiter{
			OtpVerifyErrors: 1,
		}

		err := svc.RecordVerifyError(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		if mock.limiter.OtpVerifyErrors != 2 {
			t.Errorf("Expected 2 errors, got %v", mock.limiter.OtpVerifyErrors)
		}
	})
}
