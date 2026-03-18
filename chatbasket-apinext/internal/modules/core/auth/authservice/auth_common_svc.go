package authservice

import (
	"chatbasket-apinext/internal/modules/core/auth/authkit"
	"chatbasket-apinext/internal/modules/core/auth/authmodels"
	"chatbasket-apinext/internal/platform/clients"
	"chatbasket-apinext/internal/platform/kit"
	"chatbasket-apinext/internal/store/postgresgen"
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Logout handles logout from single or all sessions
// Works for both public and personal modes
func (s *AuthService) Logout(ctx context.Context, payload *authmodels.LogoutPayload, userID uuid.UUID, sessionToken string) (*kit.StatusOkay, *kit.ApiError) {
	if payload.AllSessions {
		// Logout from all sessions - delete from PostgreSQL
		err := s.PostgresQuerier.DeleteAllUserSessions(ctx, userID)
		if err != nil {
			return nil, &kit.ApiError{
				Code:    http.StatusInternalServerError,
				Message: "Failed to logout from all sessions: " + err.Error(),
				Type:    "internal_server_error",
			}
		}
	} else {
		// Logout from single session - delete from PostgreSQL using token hash
		tokenHash, err := kit.ComputeHMAC(sessionToken, s.AuthSecret)
		if err != nil {
			return nil, &kit.ApiError{
				Code:    http.StatusInternalServerError,
				Message: "Failed to hash session token: " + err.Error(),
				Type:    "internal_server_error",
			}
		}

		err = s.PostgresQuerier.DeleteSessionByToken(ctx, postgresgen.DeleteSessionByTokenParams{
			TokenHash:  tokenHash,
			AuthUserID: userID,
		})
		if err != nil {
			return nil, &kit.ApiError{
				Code:    http.StatusInternalServerError,
				Message: "Failed to logout from session: " + err.Error(),
				Type:    "internal_server_error",
			}
		}
	}

	return &kit.StatusOkay{Status: true, Message: "Logged out successfully"}, nil
}

// GetUserWithSession retrieves user and session details (similar to login response)
func (s *AuthService) GetUserWithSession(ctx context.Context, userID uuid.UUID, sessionToken string) (*authmodels.SessionResponse, *kit.ApiError) {
	// 1. Compute HMAC
	tokenHash, err := kit.ComputeHMAC(sessionToken, s.AuthSecret)
	if err != nil {
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Failed to process token", Type: "internal_error"}
	}

	// 2. Get Session
	session, err := s.PostgresQuerier.GetSessionByToken(ctx, postgresgen.GetSessionByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &kit.ApiError{Code: http.StatusUnauthorized, Message: "Session not found", Type: "unauthorized"}
		}
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Database error: " + err.Error(), Type: "internal_error"}
	}

	// 3. Get User
	user, err := s.PostgresQuerier.GetAuthUserByID(ctx, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &kit.ApiError{Code: http.StatusNotFound, Message: "User not found", Type: "not_found"}
		}
		return nil, &kit.ApiError{Code: http.StatusInternalServerError, Message: "Database error: " + err.Error(), Type: "internal_error"}
	}

	// 4. Determine Central Device Name
	centralDeviceName := ""
	if session.IsCentral {
		if session.DeviceName != nil {
			centralDeviceName = *session.DeviceName
		}
	} else {
		centralSession, err := s.PostgresQuerier.GetCentralSession(ctx, userID)
		if err == nil && centralSession.DeviceName != nil {
			centralDeviceName = *centralSession.DeviceName
		}
	}

	// 5. Construct Response
	return &authmodels.SessionResponse{
		UserId:            user.ID.String(),
		Name:              user.Name,
		Email:             user.Email,
		SessionID:         session.ID.String(),
		SessionExpiry:     session.ExpiresAt.Time.Format(time.RFC3339),
		IsPrimary:         session.IsCentral,
		PrimaryDeviceName: centralDeviceName,
	}, nil
}

// RequestUpdateOTP sends OTP for update operations (password, email, etc.)
// Step 1 of two-step update flow
func (s *AuthService) RequestUpdateOTP(ctx context.Context, payload *authmodels.RequestUpdateOTPPayload, userID uuid.UUID) (*kit.StatusOkay, *kit.ApiError) {
	// Get user from database
	user, err := s.PostgresQuerier.GetAuthUserByID(ctx, userID)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to get user: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Generate OTP
	otp, err := authkit.GenerateOTP()
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to generate OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Hash OTP
	hashedOTP, err := authkit.HashOTP(otp)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to hash OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Generate update_id
	updateID := uuid.New()

	// Store verification code with update_id
	_, err = s.PostgresQuerier.CreateVerificationCodeWithUpdateID(ctx, postgresgen.CreateVerificationCodeWithUpdateIDParams{
		ID:       userID,
		UpdateID: pgtype.UUID{Bytes: updateID, Valid: true},
		Email:    user.Email,
		CodeHash: hashedOTP,
		Type:     payload.UpdateType, // "password_update" or "email_update"
	})
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to store verification code: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Send OTP email
	subject := "OTP for " + payload.UpdateType
	body := "<p>Hello,<br>Your OTP for " + payload.UpdateType + " is:<br><h1>" + otp + "</h1></p><p>This code is valid for 3 minutes.<br>ChatBasket</p>"
	if appErr := clients.SendEmail([]string{user.Email}, subject, body); appErr != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to send email: " + appErr.Message,
			Type:    "internal_server_error",
		}
	}

	return &kit.StatusOkay{
		Status:  true,
		Message: updateID.String(), // Return update_id as message
	}, nil
}

// ConfirmPasswordUpdate verifies OTP and updates password
// Step 2 of two-step password update flow
func (s *AuthService) ConfirmPasswordUpdate(ctx context.Context, payload *authmodels.ConfirmPasswordUpdatePayload, userID uuid.UUID) (*kit.StatusOkay, *kit.ApiError) {
	// Parse update_id
	updateID, err := uuid.Parse(payload.UpdateID)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid update ID",
			Type:    "bad_request",
		}
	}

	// Get verification code by user ID
	record, err := s.PostgresQuerier.GetVerificationCode(ctx, postgresgen.GetVerificationCodeParams{
		ID:   userID,
		Type: "password_update",
	})
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusNotFound,
			Message: "Verification code not found or expired",
			Type:    "not_found",
		}
	}

	// Verify update_id matches
	if !record.UpdateID.Valid || record.UpdateID.Bytes != updateID {
		return nil, &kit.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Request session invalid",
			Type:    "flow_error",
		}
	}

	// Check expiry (3 minutes)
	if authkit.IsExpiredOTP(record.CreatedAt.Time, 3) {
		// Delete expired code
		_ = s.PostgresQuerier.DeleteVerificationCode(ctx, userID)
		return nil, &kit.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "OTP has expired",
			Type:    "otp_expired",
		}
	}

	// Verify OTP
	match, err := authkit.VerifyOTP(payload.Otp, record.CodeHash)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to verify OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if !match {
		return nil, &kit.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Invalid OTP",
			Type:    "invalid_otp",
		}
	}

	// Hash new password
	hashedPassword, err := authkit.HashPassword(payload.NewPassword, s.AuthSecret, userID.String())
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to hash password: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Update password
	err = s.PostgresQuerier.UpdateAuthUserPassword(ctx, postgresgen.UpdateAuthUserPasswordParams{
		ID:           userID,
		PasswordHash: hashedPassword,
	})
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to update password: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Delete verification code
	_ = s.PostgresQuerier.DeleteVerificationCode(ctx, userID)

	return &kit.StatusOkay{
		Status:  true,
		Message: "Password updated successfully",
	}, nil
}

// RequestEmailUpdate sends OTP to new email for verification
// Step 1 of two-step email update flow
func (s *AuthService) RequestEmailUpdate(ctx context.Context, payload *authmodels.RequestEmailUpdatePayload, userID uuid.UUID) (*kit.StatusOkay, *kit.ApiError) {
	// Get user from database
	user, err := s.PostgresQuerier.GetAuthUserByID(ctx, userID)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to get user: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Verify current password
	match, err := authkit.VerifyPassword(payload.Password, user.PasswordHash, s.AuthSecret, userID.String())
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to verify password: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if !match {
		return nil, &kit.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Invalid password",
			Type:    "invalid_password",
		}
	}

	// Check if new email already exists
	exists, err := s.PostgresQuerier.CheckEmailExists(ctx, payload.NewEmail)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to check email: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if exists {
		return nil, &kit.ApiError{
			Code:    http.StatusConflict,
			Message: "Email already registered",
			Type:    "conflict",
		}
	}

	// Generate OTP
	otp, err := authkit.GenerateOTP()
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to generate OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Hash OTP
	hashedOTP, err := authkit.HashOTP(otp)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to hash OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Generate update_id
	updateID := uuid.New()

	// Store verification code with update_id and new email
	_, err = s.PostgresQuerier.CreateVerificationCodeWithUpdateID(ctx, postgresgen.CreateVerificationCodeWithUpdateIDParams{
		ID:       userID,
		UpdateID: pgtype.UUID{Bytes: updateID, Valid: true},
		Email:    payload.NewEmail, // Store new email in verification code
		CodeHash: hashedOTP,
		Type:     "email_update",
	})
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to store verification code: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Send OTP to new email
	subject := "OTP for email update"
	body := "<p>Hello,<br>Your OTP for email update is:<br><h1>" + otp + "</h1></p><p>This code is valid for 3 minutes.<br>ChatBasket</p>"
	if appErr := clients.SendEmail([]string{payload.NewEmail}, subject, body); appErr != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to send email: " + appErr.Message,
			Type:    "internal_server_error",
		}
	}

	return &kit.StatusOkay{
		Status:  true,
		Message: updateID.String(), // Return update_id as message
	}, nil
}

// ConfirmEmailUpdate verifies OTP and updates email
// Step 2 of two-step email update flow
func (s *AuthService) ConfirmEmailUpdate(ctx context.Context, payload *authmodels.ConfirmEmailUpdatePayload, userID uuid.UUID) (*kit.StatusOkay, *kit.ApiError) {
	// Parse update_id
	updateID, err := uuid.Parse(payload.UpdateID)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid update ID",
			Type:    "bad_request",
		}
	}

	// Get verification code by user ID
	record, err := s.PostgresQuerier.GetVerificationCode(ctx, postgresgen.GetVerificationCodeParams{
		ID:   userID,
		Type: "email_update",
	})
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusNotFound,
			Message: "Verification code not found or expired",
			Type:    "not_found",
		}
	}

	// Verify update_id matches
	if !record.UpdateID.Valid || record.UpdateID.Bytes != updateID {
		return nil, &kit.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Request session invalid",
			Type:    "flow_error",
		}
	}

	// Check expiry (3 minutes)
	if authkit.IsExpiredOTP(record.CreatedAt.Time, 3) {
		// Delete expired code
		_ = s.PostgresQuerier.DeleteVerificationCode(ctx, userID)
		return nil, &kit.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "OTP has expired",
			Type:    "otp_expired",
		}
	}

	// Verify OTP
	match, err := authkit.VerifyOTP(payload.Otp, record.CodeHash)
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to verify OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if !match {
		return nil, &kit.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Invalid OTP",
			Type:    "invalid_otp",
		}
	}

	// Update email (new email is stored in record.Email)
	err = s.PostgresQuerier.UpdateAuthUserEmail(ctx, postgresgen.UpdateAuthUserEmailParams{
		ID:              userID,
		Email:           record.Email, // Use email from verification code
		IsEmailVerified: true,
	})
	if err != nil {
		return nil, &kit.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to update email: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Delete verification code
	_ = s.PostgresQuerier.DeleteVerificationCode(ctx, userID)

	return &kit.StatusOkay{
		Status:  true,
		Message: "Email updated successfully",
	}, nil
}
