package services

import (
	"chatbasket-api-legacy/internal/db/auth"
	"chatbasket-api-legacy/model"
	"chatbasket-api-legacy/utils"
	"context"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Signup handles user registration: Validates email, creates user (unverified), sends verification OTP.
func (s *AuthService) Signup(ctx context.Context, payload *model.SignupPayload) (*model.StatusOkay, *model.ApiError) {
	// 1. Check if email already exists
	user, err := s.AuthQueries.GetAuthUserByEmail(ctx, payload.Email)
	if err == nil {
		// User exists - check if verified
		if user.IsEmailVerified {
			// Verified user cannot signup again
			return nil, &model.ApiError{Code: http.StatusConflict, Message: "Email already registered", Type: "conflict"}
		}

		// Unverified user - delete and recreate fresh
		err = s.AuthQueries.DeleteAuthUser(ctx, user.ID)
		if err != nil {
			return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to clean up unverified account", Type: "internal_server_error"}
		}
		log.Printf("Deleted unverified account for email: %s", payload.Email)
		// Continue to create new user below
	} else if err != pgx.ErrNoRows {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to query email: " + utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	// 2. Hash Password
	hashedPassword, appErr := utils.HashPassword(payload.Password)
	if appErr != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to hash password: " + appErr.Message, Type: appErr.Type}
	}

	// 3. Create User (Unverified)
	userID, err := uuid.NewV7()
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to generate UUID", Type: "internal_server_error"}
	}

	_, err = s.AuthQueries.CreateAuthUser(ctx, auth.CreateAuthUserParams{
		ID:              userID,
		Name:            payload.Name,
		Email:           payload.Email,
		PasswordHash:    hashedPassword,
		IsEmailVerified: false,
	})
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to create user: " + utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	// 4. Send Verification OTP via Utils
	if apiErr := utils.SendVerificationOTPFlow(ctx, s.AuthQueries, userID, payload.Email, "email_verification"); apiErr != nil {
		log.Printf("Signup OTP Send Warning: %v", apiErr.Message)
	}

	return &model.StatusOkay{Status: true, Message: "User created, OTP sent to email"}, nil
}

// AccountVerification verifies the OTP and activates the account/session.
func (s *AuthService) AccountVerification(ctx context.Context, payload *model.AuthVerificationPayload) (*model.SessionResponse, *model.ApiError) {
	// 1. Get User
	user, err := s.AuthQueries.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.ApiError{Code: http.StatusUnauthorized, Message: "Email is not registered", Type: "unauthorized"}
		}
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to query email: " + utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	// 2. Verify OTP via Utils
	if valid, apiErr := utils.VerifyOTPFlow(ctx, s.AuthQueries, user.ID, payload.Secret, "email_verification"); apiErr != nil || !valid {
		if apiErr != nil {
			return nil, apiErr
		}
		return nil, &model.ApiError{Code: http.StatusUnauthorized, Message: "Invalid OTP", Type: "invalid_otp"}
	}

	// 3. Mark Email Verified (if not already)
	if !user.IsEmailVerified {
		err = s.AuthQueries.UpdateAuthUserEmailVerified(ctx, auth.UpdateAuthUserEmailVerifiedParams{
			ID:              user.ID,
			IsEmailVerified: true,
		})
		if err != nil {
			return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to update verification status", Type: "internal_server_error"}
		}
	}

	// 4. Create Session via Utils
	sessionRes, apiErr := utils.CreateSessionFlow(ctx, s.AuthQueries, user.ID, payload.Platform, "", s.AuthSecret)
	if apiErr != nil {
		return nil, apiErr
	}

	return &model.SessionResponse{
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
func (s *AuthService) Login(ctx context.Context, payload *model.LoginPayload) (*model.StatusOkay, *model.ApiError) {
	// 1. Get User
	user, err := s.AuthQueries.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Generic message for security
			return nil, &model.ApiError{Code: http.StatusUnauthorized, Message: "Invalid credentials", Type: "unauthorized"}
		}
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to query email", Type: "internal_server_error"}
	}

	// 2. Check if email is verified (treat unverified as non-existent)
	if !user.IsEmailVerified {
		// Generic message for security - don't reveal user exists but is unverified
		return nil, &model.ApiError{Code: http.StatusUnauthorized, Message: "Invalid credentials", Type: "unauthorized"}
	}

	// 3. Validate Password
	match, appErr := utils.VerifyPassword(payload.Password, user.PasswordHash)
	if appErr != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to verify password: " + appErr.Message, Type: "internal_server_error"}
	}
	if !match {
		return nil, &model.ApiError{Code: http.StatusUnauthorized, Message: "Invalid credentials", Type: "unauthorized"}
	}

	// 4. Send Login OTP via Utils
	if apiErr := utils.SendVerificationOTPFlow(ctx, s.AuthQueries, user.ID, user.Email, "login"); apiErr != nil {
		return nil, apiErr
	}

	return &model.StatusOkay{Status: true, Message: "OTP sent to email"}, nil
}

// LoginVerification verifies Login OTP and creates session.
func (s *AuthService) LoginVerification(ctx context.Context, payload *model.AuthVerificationPayload) (*model.SessionResponse, *model.ApiError) {
	// 1. Get User
	user, err := s.AuthQueries.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.ApiError{Code: http.StatusUnauthorized, Message: "Email is not registered", Type: "unauthorized"}
		}
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Database error", Type: "internal_server_error"}
	}

	// 2. Verify OTP via Utils
	if valid, apiErr := utils.VerifyOTPFlow(ctx, s.AuthQueries, user.ID, payload.Secret, "login"); apiErr != nil || !valid {
		if apiErr != nil {
			return nil, apiErr
		}
		return nil, &model.ApiError{Code: http.StatusUnauthorized, Message: "Invalid OTP", Type: "invalid_otp"}
	}

	// 3. Create Session via Utils
	sessionRes, apiErr := utils.CreateSessionFlow(ctx, s.AuthQueries, user.ID, payload.Platform, "", s.AuthSecret)
	if apiErr != nil {
		return nil, apiErr
	}

	return &model.SessionResponse{
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
func (s *AuthService) ResendOTP(ctx context.Context, payload *model.ResendOTPPayload) (*model.StatusOkay, *model.ApiError) {
	// Validate type
	if payload.Type != "signup" && payload.Type != "login" {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "Invalid type. Must be 'signup' or 'login'", Type: "bad_request"}
	}

	// Get user
	user, err := s.AuthQueries.GetAuthUserByEmail(ctx, payload.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.ApiError{Code: http.StatusUnauthorized, Message: "Invalid email", Type: "unauthorized"}
		}
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to query email", Type: "internal_server_error"}
	}

	// Determine OTP type based on flow
	var otpType string
	if payload.Type == "signup" {
		// For signup: user must be unverified
		if user.IsEmailVerified {
			return nil, &model.ApiError{Code: http.StatusConflict, Message: "Email already verified", Type: "conflict"}
		}
		otpType = "email_verification"
	} else {
		// For login: user must be verified
		if !user.IsEmailVerified {
			return nil, &model.ApiError{Code: http.StatusUnauthorized, Message: "Invalid email", Type: "unauthorized"}
		}
		otpType = "login"
	}

	// Send OTP
	if apiErr := utils.SendVerificationOTPFlow(ctx, s.AuthQueries, user.ID, user.Email, otpType); apiErr != nil {
		return nil, apiErr
	}

	return &model.StatusOkay{Status: true, Message: "OTP sent to email"}, nil
}

