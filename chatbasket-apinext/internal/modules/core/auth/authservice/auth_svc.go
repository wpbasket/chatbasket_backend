package authservice

import (
	"chatbasket-apinext/internal/modules/core/auth/authkit"
	"chatbasket-apinext/internal/modules/core/auth/authmodels"
	"chatbasket-apinext/internal/platform/kit"
	"chatbasket-apinext/internal/platform/services"
	"chatbasket-apinext/internal/store/postgresgen"
	"context"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AuthService handles the business logic for the Auth module.
type AuthService struct {
	GlobalService   *services.GlobalService
	PostgresQuerier postgresgen.Querier  // For regular queries (interface)
	PostgresQueries *postgresgen.Queries // For transactions (concrete type with WithTx)
	AuthSecret      []byte
}

// NewAuthService creates a new AuthService instance.
func NewAuthService(globalService *services.GlobalService, authSecret []byte) *AuthService {
	return &AuthService{
		GlobalService:   globalService,
		PostgresQuerier: globalService.PostgresQuerier,
		PostgresQueries: globalService.PostgresQueries,
		AuthSecret:      authSecret,
	}
}

// Signup handles user registration: Validates email, creates user (unverified), sends verification OTP.
func (s *AuthService) Signup(ctx context.Context, payload *authmodels.SignupPayload) (*kit.StatusOkay, *kit.ApiError) {
	// 1. Check if email already exists
	user, err := s.PostgresQuerier.GetAuthUserByEmail(ctx, payload.Email)
	if err == nil {
		// User exists - check if verified
		if user.IsEmailVerified {
			// Verified user cannot signup again
			return nil, &kit.ApiError{Code: http.StatusConflict, Message: "Email already registered", Type: "conflict"}
		}

		// Unverified user - delete and recreate fresh
		err = s.PostgresQuerier.DeleteAuthUser(ctx, user.ID)
		if err != nil {
			return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to clean up unverified account", Type: "internal_server_error"}
		}
		log.Printf("Deleted unverified account for email: %s", payload.Email)
		// Continue to create new user below
	} else if err != pgx.ErrNoRows {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to query email: " + kit.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	// 2. Hash Password
	// IMPORTANT: We use the early-generated UUID for credential binding in the enhanced password kit
	userID, err := uuid.NewV7()
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to generate UUID", Type: "internal_server_error"}
	}

	hashedPassword, err := authkit.HashPassword(payload.Password, s.AuthSecret, userID.String())
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to hash password: " + err.Error(), Type: "hashing_error"}
	}

	// 3. Create User (Unverified)
	_, err = s.PostgresQuerier.CreateAuthUser(ctx, postgresgen.CreateAuthUserParams{
		ID:              userID,
		Name:            payload.Name,
		Email:           payload.Email,
		PasswordHash:    hashedPassword,
		IsEmailVerified: false,
	})
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to create user: " + kit.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	// 4. Send Verification OTP via Utils
	if apiErr := s.SendVerificationOTPFlow(ctx, userID, payload.Email, "email_verification"); apiErr != nil {
		log.Printf("Signup OTP Send Warning: %v", apiErr.Message)
	}

	return &kit.StatusOkay{Status: true, Message: "User created, OTP sent to email"}, nil
}

// AccountVerification verifies the OTP and activates the account/session.
func (s *AuthService) AccountVerification(ctx context.Context, payload *authmodels.AuthVerificationPayload) (*authmodels.SessionResponse, *kit.ApiError) {
	// 1. Get User
	user, err := s.PostgresQuerier.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Email is not registered", Type: "unauthorized"}
		}
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to query email: " + kit.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	// 2. Verify OTP via Utils
	if valid, apiErr := s.VerifyOTPFlow(ctx, user.ID, payload.Secret, "email_verification"); apiErr != nil || !valid {
		if apiErr != nil {
			return nil, apiErr
		}
		return nil, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Invalid OTP", Type: "invalid_otp"}
	}

	// 3. Mark Email Verified (if not already)
	if !user.IsEmailVerified {
		err = s.PostgresQuerier.UpdateAuthUserEmailVerified(ctx, postgresgen.UpdateAuthUserEmailVerifiedParams{
			ID:              user.ID,
			IsEmailVerified: true,
		})
		if err != nil {
			return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to update verification status", Type: "internal_server_error"}
		}
	}

	// 4. Create Session via Utils
	sessionRes, apiErr := s.CreateSessionFlow(ctx, user.ID, payload.Platform, "")
	if apiErr != nil {
		return nil, apiErr
	}

	return &authmodels.SessionResponse{
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
func (s *AuthService) Login(ctx context.Context, payload *authmodels.LoginPayload) (*kit.StatusOkay, *kit.ApiError) {
	// 1. Get User
	user, err := s.PostgresQuerier.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Generic message for security
			return nil, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Invalid credentials", Type: "unauthorized"}
		}
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to query email", Type: "internal_server_error"}
	}

	// 2. Check if email is verified (treat unverified as non-existent)
	if !user.IsEmailVerified {
		// Generic message for security - don't reveal user exists but is unverified
		return nil, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Invalid credentials", Type: "unauthorized"}
	}

	// 3. Validate Password
	match, err := authkit.VerifyPassword(payload.Password, user.PasswordHash, s.AuthSecret, user.ID.String())
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to verify password: " + err.Error(), Type: "internal_server_error"}
	}
	if !match {
		return nil, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Invalid credentials", Type: "unauthorized"}
	}

	// 4. Send Login OTP via Utils
	if apiErr := s.SendVerificationOTPFlow(ctx, user.ID, user.Email, "login"); apiErr != nil {
		return nil, apiErr
	}

	return &kit.StatusOkay{Status: true, Message: "OTP sent to email"}, nil
}

// LoginVerification verifies Login OTP and creates session.
func (s *AuthService) LoginVerification(ctx context.Context, payload *authmodels.AuthVerificationPayload) (*authmodels.SessionResponse, *kit.ApiError) {
	// 1. Get User
	user, err := s.PostgresQuerier.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Email is not registered", Type: "unauthorized"}
		}
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Database error", Type: "internal_server_error"}
	}

	// 2. Verify OTP via Utils
	if valid, apiErr := s.VerifyOTPFlow(ctx, user.ID, payload.Secret, "login"); apiErr != nil || !valid {
		if apiErr != nil {
			return nil, apiErr
		}
		return nil, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Invalid OTP", Type: "invalid_otp"}
	}

	// 3. Create Session via Utils
	sessionRes, apiErr := s.CreateSessionFlow(ctx, user.ID, payload.Platform, "")
	if apiErr != nil {
		return nil, apiErr
	}

	return &authmodels.SessionResponse{
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
func (s *AuthService) ResendOTP(ctx context.Context, payload *authmodels.ResendOTPPayload) (*kit.StatusOkay, *kit.ApiError) {
	// Validate type
	if payload.Type != "signup" && payload.Type != "login" {
		return nil, &kit.ApiError{Code: http.StatusBadRequest, Message: "Invalid type. Must be 'signup' or 'login'", Type: "bad_request"}
	}

	// Get user
	user, err := s.PostgresQuerier.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Invalid email", Type: "unauthorized"}
		}
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to query email", Type: "internal_server_error"}
	}

	// Determine OTP type based on flow
	var otpType string
	if payload.Type == "signup" {
		// For signup: user must be unverified
		if user.IsEmailVerified {
			return nil, &kit.ApiError{Code: http.StatusConflict, Message: "Email already verified", Type: "conflict"}
		}
		otpType = "email_verification"
	} else {
		// For login: user must be verified
		if !user.IsEmailVerified {
			return nil, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Invalid email", Type: "unauthorized"}
		}
		otpType = "login"
	}

	// Send OTP
	if apiErr := s.SendVerificationOTPFlow(ctx, user.ID, user.Email, otpType); apiErr != nil {
		return nil, apiErr
	}

	return &kit.StatusOkay{Status: true, Message: "OTP sent to email"}, nil
}
