package core_auth

import (
	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-api/internal/platform/kit"
	"context"
	"net/http"
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
	m.limiter.OtpDailyCount = arg.OtpDailyCount
	if arg.LastOtpSendAt != nil {
		m.limiter.LastOtpSendAt = arg.LastOtpSendAt
	}
	m.limiter.DailyResetAt = arg.DailyResetAt
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
		dailyReset := time.Now().Add(-1 * time.Hour)
		mock.limiter = core_auth_store.AuthRateLimiter{
			OtpHourlyCount: 5,
			OtpDailyCount:  5,
			LastOtpSendAt:  &lastSend,
			DailyResetAt:   &dailyReset,
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

	t.Run("Daily_Limit_Enforcement", func(t *testing.T) {
		// Reset mock to have 15 sends in last 2 hours
		lastSend := time.Now().Add(-1 * time.Hour)
		dailyReset := time.Now().Add(-2 * time.Hour)
		mock.limiter = core_auth_store.AuthRateLimiter{
			OtpHourlyCount: 5,
			OtpDailyCount:  15,
			LastOtpSendAt:  &lastSend,
			DailyResetAt:   &dailyReset,
		}

		err := svc.CheckOTPSendRateLimit(ctx, userID)
		if err == nil {
			t.Fatal("Expected daily limit error, got nil")
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
		if pe.Kind() != "verification_locked" {
			t.Errorf("Expected verification_locked, got %v", pe.Kind())
		}

		// Simulate lockout expiry (16 minutes later)
		oldVerify := time.Now().Add(-16 * time.Minute)
		mock.limiter.LastVerifyAttemptAt = &oldVerify
		err = svc.CheckOTPVerifyRateLimit(ctx, userID)
		if err != nil {
			t.Errorf("Expected success after lockout expiry, got %v", err)
		}
	})

	t.Run("Daily_Limit_Reset", func(t *testing.T) {
		// Simulate reaching limit yesterday
		yesterday := time.Now().Add(-25 * time.Hour)
		dailyReset := time.Now().Add(-25 * time.Hour)
		mock.limiter = core_auth_store.AuthRateLimiter{
			OtpHourlyCount: 5,
			OtpDailyCount:  15,
			LastOtpSendAt:  &yesterday,
			DailyResetAt:   &dailyReset, // Expired reset (>24h)
		}

		err := svc.CheckOTPSendRateLimit(ctx, userID)
		if err != nil {
			t.Fatalf("Expected reset success, got error: %v", err)
		}
		if mock.limiter.OtpDailyCount != 1 {
			t.Errorf("Daily count should have reset to 1, got %v", mock.limiter.OtpDailyCount)
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
