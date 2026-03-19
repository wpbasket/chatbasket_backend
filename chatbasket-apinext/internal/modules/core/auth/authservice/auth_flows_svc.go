package authservice

import (
	"chatbasket-apinext/internal/modules/core/auth/authkit"
	"chatbasket-apinext/internal/platform/clients"
	"chatbasket-apinext/internal/platform/kit"
	"chatbasket-apinext/internal/store/postgresgen"
	"context"
	"fmt"
	"net/http"
	"time"
	"log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SendVerificationOTPFlow generates OTP, hashes it, stores in DB, and sends email.
func (s *AuthService) SendVerificationOTPFlow(ctx context.Context, userID uuid.UUID, email, otpType string) *kit.ApiError {
	// Generate OTP
	otp, err := authkit.GenerateOTP()
	if err != nil {
		log.Printf("Failed to generate OTP: %v", err)
		return &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to generate OTP: " + err.Error(), Type: "internal_server_error"}
	}

	hashedOTP, err := authkit.HashOTP(otp)
	if err != nil {
		log.Printf("Failed to hash OTP: %v", err)
		return &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to hash OTP", Type: "internal_server_error"}
	}

	// Store in DB (UPSERT on id=userID handles strict 1:1)
	// ExpiresAt column removed; logic relies on CreatedAt + 3 mins
	_, err = s.PostgresQuerier.CreateVerificationCode(ctx, postgresgen.CreateVerificationCodeParams{
		ID:       userID, // Using UserID as the PK directly
		Email:    email,
		CodeHash: hashedOTP,
		Type:     otpType,
	})
	if err != nil {
		log.Printf("Failed to store OTP: %v", err)
		return &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to save OTP", Type: "internal_server_error"}
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
	}

	if appErr := clients.SendEmail([]string{email}, subject, body); appErr != nil {
		log.Printf("Failed to send OTP email: %v", appErr.Message)
		return &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to send email: " + appErr.Message, Type: "internal_server_error"}
	}
	return nil
}

// VerifyOTPFlow retrieves code by UserID, checks created_at expiry (3 mins), validates hash, and consumes code.
func (s *AuthService) VerifyOTPFlow(ctx context.Context, userID uuid.UUID, secret, otpType string) (bool, *kit.ApiError) {
	record, err := s.PostgresQuerier.GetVerificationCode(ctx, postgresgen.GetVerificationCodeParams{
		ID:   userID,
		Type: otpType,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			// No code found
			return false, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Invalid OTP", Type: "unauthorized"}
		}
		return false, &kit.ApiError{Code: http.StatusInternalServerError, Message: "DB Error", Type: "internal_server_error"}
	}

	// Check Expiry (Created At + 3 Minutes)
	// Passing 3 as validity duration in minutes
	if authkit.IsExpiredOTP(record.CreatedAt.Time, 3) {
		return false, &kit.ApiError{Code: http.StatusUnauthorized, Message: "OTP has expired", Type: "unauthorized"}
	}

	// Verify Hash
	match, err := authkit.VerifyOTP(secret, record.CodeHash)
	if err != nil {
		return false, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Hash verification error", Type: "internal_server_error"}
	}

	if match {
		// Consume the code
		_ = s.PostgresQuerier.DeleteVerificationCode(ctx, record.ID)
		return true, nil
	}

	return false, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Invalid OTP", Type: "unauthorized"}
}

type SessionResult struct {
	Token             string
	ExpiresAt         string
	IsPrimary         bool
	PrimaryDeviceName string
}

// CreateSessionFlow generates token, hashes, stores session, returns token + expiry string.
func (s *AuthService) CreateSessionFlow(ctx context.Context, userID uuid.UUID, platform, deviceToken string) (*SessionResult, *kit.ApiError) {
	tokenEnv := uuid.New().String()
	tokenHash, err := kit.ComputeHMAC(tokenEnv, s.AuthSecret, true, new(userID.String()))
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Hash error", Type: "internal_server_error"}
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

	_, err = s.PostgresQuerier.CreateSession(ctx, postgresgen.CreateSessionParams{
		ID:          sid,
		AuthUserID:  userID,
		TokenHash:   tokenHash,
		ExpiresAt:   pgtype.Timestamptz{Valid: true, Time: expiresAt},
		Platform:    new(platform),
		DeviceToken: new(deviceToken),
		DeviceName:  nil, // To be set later via settings
		IsCentral:   isPrimary,
	})
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to create session: " + err.Error(), Type: "internal_server_error"}
	}

	return &SessionResult{
		Token:             tokenEnv,
		ExpiresAt:         expiresAt.Format(time.RFC3339),
		IsPrimary:         isPrimary,
		PrimaryDeviceName: primaryDeviceName,
	}, nil
}
