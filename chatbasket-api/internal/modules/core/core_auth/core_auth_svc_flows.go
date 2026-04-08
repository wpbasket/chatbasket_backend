package core_auth

import (
	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/kit"
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SendVerificationOTPFlow generates OTP, hashes it, stores in DB, and sends email.
func (s *AuthService) SendVerificationOTPFlow(ctx context.Context, userID uuid.UUID, email, otpType string) error {
	// Generate OTP
	otp, err := GenerateOTP()
	if err != nil {
		log.Printf("Failed to generate OTP: %v", err)
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate OTP: "+err.Error())
	}

	hashedOTP, err := HashOTP(otp)
	if err != nil {
		log.Printf("Failed to hash OTP: %v", err)
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to hash OTP")
	}

	// Store in DB (UPSERT on id=userID handles strict 1:1)
	// ExpiresAt column removed; logic relies on CreatedAt + 3 mins
	_, err = s.PostgresQuerier.CreateVerificationCode(ctx, core_auth_store.CreateVerificationCodeParams{
		ID:       userID, // Using UserID as the PK directly
		Email:    email,
		CodeHash: hashedOTP,
		Type:     otpType,
	})
	if err != nil {
		log.Printf("Failed to store OTP: %v", err)
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to save OTP")
	}

	// Send Email
	subject := "Your Verification Code"
	body := fmt.Sprintf("<p>Your %s code is: <b>%s</b></p><p>Expires in 3 minutes.</p>", otpType, otp)
	switch otpType {
	case "login":
		subject = "Your Login Code"
		body = fmt.Sprintf("<p>Your login code is: <b>%s</b></p><p>Expires in 3 minutes.</p>", otp)
	case "email_verification":
		subject = "Your Verification Code"
		body = fmt.Sprintf("<p>Your email verification code is: <b>%s</b></p><p>Expires in 3 minutes.</p>", otp)
	case "password_reset":
		subject = "Reset Your Password"
		body = fmt.Sprintf("<p>Your password reset code is: <b>%s</b></p><p>Expires in 3 minutes.</p>", otp)
	}

	if err := clients.SendEmail([]string{email}, subject, body); err != nil {
		log.Printf("Failed to send OTP email: %v", err.Error())
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to send email: "+err.Error())
	}
	return nil
}

// VerifyOTPFlow retrieves code by UserID, checks created_at expiry (3 mins), validates hash, and consumes code.
func (s *AuthService) VerifyOTPFlow(ctx context.Context, userID uuid.UUID, secret, otpType string) (bool, error) {
	record, err := s.PostgresQuerier.GetVerificationCode(ctx, core_auth_store.GetVerificationCodeParams{
		ID:   userID,
		Type: otpType,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			// No code found
			return false, kit.NewError(http.StatusUnauthorized, "unauthorized", "Invalid OTP")
		}
		return false, kit.NewError(http.StatusInternalServerError, "internal_server_error", "DB Error")
	}

	// Check Expiry (Created At + 3 Minutes)
	// Passing 3 as validity duration in minutes
	if IsExpiredOTP(record.CreatedAt, 3) {
		return false, kit.NewError(http.StatusUnauthorized, "unauthorized", "OTP has expired")
	}

	// Verify Hash
	match, err := VerifyOTP(secret, record.CodeHash)
	if err != nil {
		return false, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Hash verification error")
	}

	if match {
		// Consume the code
		_ = s.PostgresQuerier.DeleteVerificationCode(ctx, record.ID)
		return true, nil
	}

	return false, kit.NewError(http.StatusUnauthorized, "unauthorized", "Invalid OTP")
}

type SessionResult struct {
	Token             string
	ExpiresAt         string
	IsPrimary         bool
	PrimaryDeviceName string
}

// CreateSessionFlow generates token, hashes, stores session, returns token + expiry string.
func (s *AuthService) CreateSessionFlow(ctx context.Context, userID uuid.UUID, platform, deviceToken *string) (*SessionResult, error) {
	tokenEnv := uuid.New().String()
	tokenHash, err := kit.ComputeHMAC(tokenEnv, s.AuthSecret, true, new(userID.String()))
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Hash error")
	}

	// 3 years expiry
	expiresAt := time.Now().Add(3 * 365 * 24 * time.Hour)
	sid, _ := uuid.NewV7()

	// Logic: If user has NO primary device, and this is a native platform, make this the primary device.
	isPrimary := false
	primaryDeviceName := ""

	// Check for existing primary device
	existingPrimary, err := s.PostgresQuerier.GetCentralSession(ctx, userID)
	if err == nil {
		// Existing primary device found
		if existingPrimary.DeviceName != nil {
			primaryDeviceName = *existingPrimary.DeviceName
		}
	} else {
		// No primary device found, auto-promote ANY platform for now (Temporary Fix for Web Messaging)
		isPrimary = true
	}

	_, err = s.PostgresQuerier.CreateSession(ctx, core_auth_store.CreateSessionParams{
		ID:          sid,
		AuthUserID:  userID,
		TokenHash:   tokenHash,
		ExpiresAt:   expiresAt,
		Platform:    platform,
		DeviceToken: deviceToken,
		DeviceName:  nil, // To be set later via settings
		IsCentral:   isPrimary,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to create session: "+err.Error())
	}

	return &SessionResult{
		Token:             tokenEnv,
		ExpiresAt:         expiresAt.Format(time.RFC3339),
		IsPrimary:         isPrimary,
		PrimaryDeviceName: primaryDeviceName,
	}, nil
}
