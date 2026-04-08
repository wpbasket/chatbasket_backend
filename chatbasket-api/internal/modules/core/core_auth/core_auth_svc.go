package core_auth

import (
	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/services"
	"context"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthService handles the business logic for the Auth module.
type AuthService struct {
	GlobalService   *services.GlobalService
	PostgresQuerier core_auth_store.Querier  // For regular queries (interface)
	PostgresQueries *core_auth_store.Queries // For transactions (concrete type with WithTx)
	AuthSecret      []byte
}

// NewAuthService creates a new AuthService instance.
func NewAuthService(globalService *services.GlobalService, pool *pgxpool.Pool, authSecret []byte) *AuthService {
	store := core_auth_store.New(pool)
	return &AuthService{
		GlobalService:   globalService,
		PostgresQuerier: store,
		PostgresQueries: store,
		AuthSecret:      authSecret,
	}
}

// Signup handles user registration: Validates email, creates user (unverified), sends verification OTP.
func (s *AuthService) Signup(ctx context.Context, payload *SignupPayload) (*kit.StatusOkay, error) {
	// 1. Check if email already exists
	user, err := s.PostgresQuerier.GetAuthUserByEmail(ctx, payload.Email)
	if err == nil {
		// User exists - check if verified
		if user.IsEmailVerified {
			// Verified user cannot signup again
			return nil, kit.NewError(http.StatusConflict, "conflict", "Email already registered")
		}

		// Unverified user - delete and recreate fresh
		err = s.PostgresQuerier.DeleteAuthUser(ctx, user.ID)
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to clean up unverified account")
		}
		log.Printf("Deleted unverified account for email: %s", payload.Email)
		// Continue to create new user below
	} else if err != pgx.ErrNoRows {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to query email: "+kit.GetPostgresError(err).Message)
	}

	// 2. Hash Password
	// IMPORTANT: We use the early-generated UUID for credential binding in the enhanced password kit
	userID, err := uuid.NewV7()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate UUID")
	}

	hashedPassword, err := HashPassword(payload.Password, s.AuthSecret, userID.String())
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to hash password: "+err.Error())
	}

	// 3. Create User (Unverified)
	_, err = s.PostgresQuerier.CreateAuthUser(ctx, core_auth_store.CreateAuthUserParams{
		ID:              userID,
		Name:            payload.Name,
		Email:           payload.Email,
		PasswordHash:    hashedPassword,
		IsEmailVerified: false,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to create user: "+kit.GetPostgresError(err).Message)
	}

	// 4. Send Verification OTP via Utils
	if err := s.SendVerificationOTPFlow(ctx, userID, payload.Email, "email_verification"); err != nil {
		log.Printf("Signup OTP Send Warning: %v", err.Error())
	}

	return &kit.StatusOkay{Status: true, Message: "User created, OTP sent to email"}, nil
}

// AccountVerification verifies the OTP and activates the account/session.
func (s *AuthService) AccountVerification(ctx context.Context, payload *AuthVerificationPayload) (*SessionResponse, error) {
	// 1. Get User
	user, err := s.PostgresQuerier.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusUnauthorized, "unauthorized", "Email is not registered")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to query email: "+kit.GetPostgresError(err).Message)
	}

	// 2. Verify OTP via Utils
	if valid, err := s.VerifyOTPFlow(ctx, user.ID, payload.Secret, "email_verification"); err != nil || !valid {
		if err != nil {
			return nil, err
		}
		return nil, kit.NewError(http.StatusUnauthorized, "invalid_otp", "Invalid OTP")
	}

	// 3. Mark Email Verified (if not already)
	if !user.IsEmailVerified {
		err = s.PostgresQuerier.UpdateAuthUserEmailVerified(ctx, core_auth_store.UpdateAuthUserEmailVerifiedParams{
			ID:              user.ID,
			IsEmailVerified: true,
		})
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to update verification status")
		}
	}

	// 4. Create Session via Utils
	sessionRes, err := s.CreateSessionFlow(ctx, user.ID, new(payload.Platform), nil)
	if err != nil {
		return nil, err
	}

	return &SessionResponse{
		UserId:            user.ID.String(),
		Name:              user.Name,
		Email:             user.Email,
		SessionID:         sessionRes.Token,
		SessionExpiry:     sessionRes.ExpiresAt,
		IsPrimary:         sessionRes.IsPrimary,
		PrimaryDeviceName: sessionRes.PrimaryDeviceName,
	}, nil
}

// Login validates password and sends OTP (2FA flow).
func (s *AuthService) Login(ctx context.Context, payload *LoginPayload) (*kit.StatusOkay, error) {
	// 1. Get User
	user, err := s.PostgresQuerier.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Generic message for security
			return nil, kit.NewError(http.StatusUnauthorized, "unauthorized", "Invalid credentials")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to query email")
	}

	// 2. Check if email is verified (treat unverified as non-existent)
	if !user.IsEmailVerified {
		// Generic message for security - don't reveal user exists but is unverified
		return nil, kit.NewError(http.StatusUnauthorized, "unauthorized", "Invalid credentials")
	}

	// 3. Validate Password
	match, err := VerifyPassword(payload.Password, user.PasswordHash, s.AuthSecret, user.ID.String())
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to verify password: "+err.Error())
	}
	if !match {
		return nil, kit.NewError(http.StatusUnauthorized, "unauthorized", "Invalid credentials")
	}

	// 4. Send Login OTP via Utils
	if err := s.SendVerificationOTPFlow(ctx, user.ID, user.Email, "login"); err != nil {
		log.Printf("Login OTP Send Warning: %v", err.Error())
	}

	return &kit.StatusOkay{Status: true, Message: "Login successful, OTP sent to email"}, nil
}

// LoginVerification verifies Login OTP and creates session.
func (s *AuthService) LoginVerification(ctx context.Context, payload *AuthVerificationPayload) (*SessionResponse, error) {
	// 1. Get User
	user, err := s.PostgresQuerier.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusUnauthorized, "unauthorized", "Email is not registered")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Database error")
	}

	// 2. Verify OTP via Utils
	if valid, err := s.VerifyOTPFlow(ctx, user.ID, payload.Secret, "login"); err != nil || !valid {
		if err != nil {
			return nil, err
		}
		return nil, kit.NewError(http.StatusUnauthorized, "invalid_otp", "Invalid OTP")
	}

	// 3. Create Session via Utils
	sessionRes, err := s.CreateSessionFlow(ctx, user.ID, new(payload.Platform), nil)
	if err != nil {
		return nil, err
	}

	return &SessionResponse{
		UserId:            user.ID.String(),
		Name:              user.Name,
		Email:             user.Email,
		SessionID:         sessionRes.Token,
		SessionExpiry:     sessionRes.ExpiresAt,
		IsPrimary:         sessionRes.IsPrimary,
		PrimaryDeviceName: sessionRes.PrimaryDeviceName,
	}, nil
}

// ResendOTP handles OTP resend for both signup and login flows.
func (s *AuthService) ResendOTP(ctx context.Context, payload *ResendOTPPayload) (*kit.StatusOkay, error) {
	// Validate type
	if payload.Type != "signup" && payload.Type != "login" {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "Invalid type. Must be 'signup' or 'login'")
	}

	// Get user
	user, err := s.PostgresQuerier.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusUnauthorized, "unauthorized", "Invalid email")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to query email")
	}

	// Determine OTP type based on flow
	var otpType string
	if payload.Type == "signup" {
		// For signup: user must be unverified
		if user.IsEmailVerified {
			return nil, kit.NewError(http.StatusConflict, "conflict", "Email already verified")
		}
		otpType = "email_verification"
	} else {
		// For login: user must be verified
		if !user.IsEmailVerified {
			return nil, kit.NewError(http.StatusUnauthorized, "unauthorized", "Invalid email")
		}
		otpType = "login"
	}

	// Send OTP
	if err := s.SendVerificationOTPFlow(ctx, user.ID, user.Email, otpType); err != nil {
		return nil, err
	}

	return &kit.StatusOkay{Status: true, Message: "OTP sent to email"}, nil
}

// ForgotPassword initiates the forgot password flow by sending OTP to email.
func (s *AuthService) ForgotPassword(ctx context.Context, payload *ForgotPasswordPayload) (*kit.StatusOkay, error) {
	// 1. Get user by email
	user, err := s.PostgresQuerier.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Return conflict error for email not found (as per frontend spec)
			return nil, kit.NewError(http.StatusConflict, "conflict", "Email not found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to query email")
	}

	// 2. User must be verified to reset password
	if !user.IsEmailVerified {
		return nil, kit.NewError(http.StatusConflict, "conflict", "Email not found")
	}

	// 3. Send OTP for password reset
	if err := s.SendVerificationOTPFlow(ctx, user.ID, user.Email, "password_reset"); err != nil {
		return nil, err
	}

	// 4. Return updateId (user ID as string) for verification step
	return &kit.StatusOkay{Status: true, Message: user.ID.String()}, nil
}

// VerifyForgotPassword verifies OTP and updates the password.
func (s *AuthService) VerifyForgotPassword(ctx context.Context, payload *ForgotPasswordVerifyPayload) (*kit.StatusOkay, error) {
	// 1. Parse updateId as UUID
	userID, err := uuid.Parse(payload.UpdateID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "Invalid updateId")
	}

	// 2. Get verification code by user ID
	record, err := s.PostgresQuerier.GetVerificationCode(ctx, core_auth_store.GetVerificationCodeParams{
		ID:   userID,
		Type: "password_reset",
	})
	if err != nil {
		return nil, kit.NewError(http.StatusNotFound, "not_found", "Verification code not found or expired")
	}

	// 3. Verify updateId matches (flow validation)
	// For password_reset, we store the userID as the updateId, so we just verify the record exists
	// This ensures the OTP request came from the same flow

	// 4. Check expiry (3 minutes)
	if IsExpiredOTP(record.CreatedAt, 3) {
		// Delete expired code
		_ = s.PostgresQuerier.DeleteVerificationCode(ctx, userID)
		return nil, kit.NewError(http.StatusUnauthorized, "otp_expired", "OTP has expired")
	}

	// 5. Verify OTP
	match, err := VerifyOTP(payload.Otp, record.CodeHash)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to verify OTP: "+err.Error())
	}
	if !match {
		return nil, kit.NewError(http.StatusUnauthorized, "invalid_otp", "Invalid OTP")
	}

	// 6. Hash new password
	hashedPassword, err := HashPassword(payload.NewPassword, s.AuthSecret, userID.String())
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to hash password: "+err.Error())
	}

	// 7. Update password
	err = s.PostgresQuerier.UpdateAuthUserPassword(ctx, core_auth_store.UpdateAuthUserPasswordParams{
		ID:           userID,
		PasswordHash: hashedPassword,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to update password")
	}

	// 8. Delete verification code
	_ = s.PostgresQuerier.DeleteVerificationCode(ctx, userID)

	return &kit.StatusOkay{Status: true, Message: "Password updated successfully"}, nil
}
