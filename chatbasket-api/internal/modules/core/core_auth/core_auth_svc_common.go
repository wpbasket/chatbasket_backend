package core_auth

import (
	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/kit"
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Logout handles logout from single or all sessions
// Works for both public and personal modes
func (s *AuthService) Logout(ctx context.Context, payload *LogoutPayload, userID uuid.UUID, sessionToken string) (*kit.StatusOkay, error) {
	if payload.AllSessions {
		// Logout from all sessions - delete from PostgreSQL
		err := s.PostgresQuerier.DeleteAllUserSessions(ctx, userID)
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to logout from all sessions: "+err.Error())
		}
	} else {
		// Logout from single session - delete from PostgreSQL using token hash
		tokenHash, err := kit.ComputeHMAC(sessionToken, s.AuthSecret, true, new(userID.String()))
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to hash session token: "+err.Error())
		}

		err = s.PostgresQuerier.DeleteSessionByToken(ctx, core_auth_store.DeleteSessionByTokenParams{
			TokenHash:  tokenHash,
			AuthUserID: userID,
		})
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to logout from session: "+err.Error())
		}
	}

	return &kit.StatusOkay{Status: true, Message: "Logged out successfully"}, nil
}

// GetUserWithSession retrieves user and session details (similar to login response)
func (s *AuthService) GetUserWithSession(ctx context.Context, userID uuid.UUID, sessionToken string) (*SessionResponse, error) {
	// 1. Compute HMAC
	tokenHash, err := kit.ComputeHMAC(sessionToken, s.AuthSecret, true, new(userID.String()))
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_error", "Failed to process token")
	}

	// 2. Get Session
	session, err := s.PostgresQuerier.GetSessionByToken(ctx, core_auth_store.GetSessionByTokenParams{
		TokenHash:  tokenHash,
		AuthUserID: userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusUnauthorized, "unauthorized", "Session not found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_error", "Database error: "+err.Error())
	}

	// 3. Get User
	user, err := s.PostgresQuerier.GetAuthUserByID(ctx, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusNotFound, "not_found", "User not found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_error", "Database error: "+err.Error())
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
	return &SessionResponse{
		UserId:            user.ID.String(),
		Name:              user.Name,
		Email:             user.Email,
		SessionID:         session.ID.String(),
		SessionExpiry:     session.ExpiresAt.Format(time.RFC3339),
		IsPrimary:         session.IsCentral,
		PrimaryDeviceName: centralDeviceName,
	}, nil
}

// RequestUpdateOTP sends OTP for update operations (password, email, etc.)
// Step 1 of two-step update flow
func (s *AuthService) RequestUpdateOTP(ctx context.Context, payload *RequestUpdateOTPPayload, userID uuid.UUID) (*kit.StatusOkay, error) {
	// Get user from database
	user, err := s.PostgresQuerier.GetAuthUserByID(ctx, userID)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to get user: "+err.Error())
	}

	// Generate OTP
	otp, err := GenerateOTP()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate OTP: "+err.Error())
	}

	// Hash OTP
	hashedOTP, err := HashOTP(otp)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to hash OTP: "+err.Error())
	}

	// Generate update_id
	updateID, err := uuid.NewV7()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate update ID")
	}

	// Store verification code with update_id
	_, err = s.PostgresQuerier.CreateVerificationCodeWithUpdateID(ctx, core_auth_store.CreateVerificationCodeWithUpdateIDParams{
		ID:       userID,
		UpdateID: &updateID,
		Email:    user.Email,
		CodeHash: hashedOTP,
		Type:     payload.UpdateType, // "password_update" or "email_update"
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to store verification code: "+err.Error())
	}

	// Send OTP email
	subject := "OTP for " + payload.UpdateType
	body := "<p>Hello,<br>Your OTP for " + payload.UpdateType + " is:<br><h1>" + otp + "</h1></p><p>This code is valid for 3 minutes.<br>ChatBasket</p>"
	if err := clients.SendEmail([]string{user.Email}, subject, body); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to send email: "+err.Error())
	}

	return &kit.StatusOkay{
		Status:  true,
		Message: updateID.String(), // Return update_id as message
	}, nil
}

// ConfirmPasswordUpdate verifies OTP and updates password
// Step 2 of two-step password update flow
func (s *AuthService) ConfirmPasswordUpdate(ctx context.Context, payload *ConfirmPasswordUpdatePayload, userID uuid.UUID) (*kit.StatusOkay, error) {
	// Parse update_id
	updateID, err := uuid.Parse(payload.UpdateID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "Invalid update ID")
	}

	// Get verification code by user ID
	record, err := s.PostgresQuerier.GetVerificationCode(ctx, core_auth_store.GetVerificationCodeParams{
		ID:   userID,
		Type: "password_update",
	})
	if err != nil {
		return nil, kit.NewError(http.StatusNotFound, "not_found", "Verification code not found or expired")
	}

	// Verify update_id matches
	if record.UpdateID == nil || *record.UpdateID != updateID {
		return nil, kit.NewError(http.StatusUnauthorized, "flow_error", "Request session invalid")
	}

	// Check expiry (3 minutes)
	if IsExpiredOTP(record.CreatedAt, 3) {
		// Delete expired code
		_ = s.PostgresQuerier.DeleteVerificationCode(ctx, userID)
		return nil, kit.NewError(http.StatusUnauthorized, "otp_expired", "OTP has expired")
	}

	// Verify OTP
	match, err := VerifyOTP(payload.Otp, record.CodeHash)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to verify OTP: "+err.Error())
	}
	if !match {
		return nil, kit.NewError(http.StatusUnauthorized, "invalid_otp", "Invalid OTP")
	}

	// Hash new password
	hashedPassword, err := HashPassword(payload.NewPassword, s.AuthSecret, userID.String())
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to hash password: "+err.Error())
	}

	// Update password
	err = s.PostgresQuerier.UpdateAuthUserPassword(ctx, core_auth_store.UpdateAuthUserPasswordParams{
		ID:           userID,
		PasswordHash: hashedPassword,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to update password: "+err.Error())
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
func (s *AuthService) RequestEmailUpdate(ctx context.Context, payload *RequestEmailUpdatePayload, userID uuid.UUID) (*kit.StatusOkay, error) {
	// Get user from database
	user, err := s.PostgresQuerier.GetAuthUserByID(ctx, userID)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to get user: "+err.Error())
	}

	// Verify current password
	match, err := VerifyPassword(payload.Password, user.PasswordHash, s.AuthSecret, userID.String())
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to verify password: "+err.Error())
	}
	if !match {
		return nil, kit.NewError(http.StatusUnauthorized, "invalid_password", "Invalid password")
	}

	// Check if new email already exists
	exists, err := s.PostgresQuerier.CheckEmailExists(ctx, payload.NewEmail)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to check email: "+err.Error())
	}
	if exists {
		return nil, kit.NewError(http.StatusConflict, "conflict", "Email already registered")
	}

	// Generate OTP
	otp, err := GenerateOTP()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate OTP: "+err.Error())
	}

	// Hash OTP
	hashedOTP, err := HashOTP(otp)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to hash OTP: "+err.Error())
	}

	// Generate update_id
	updateID, err := uuid.NewV7()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to generate update ID")
	}

	// Store verification code with update_id and new email
	_, err = s.PostgresQuerier.CreateVerificationCodeWithUpdateID(ctx, core_auth_store.CreateVerificationCodeWithUpdateIDParams{
		ID:       userID,
		UpdateID: &updateID,
		Email:    payload.NewEmail, // Store new email in verification code
		CodeHash: hashedOTP,
		Type:     "email_update",
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to store verification code: "+err.Error())
	}

	// Send OTP to new email
	subject := "OTP for email update"
	body := "<p>Hello,<br>Your OTP for email update is:<br><h1>" + otp + "</h1></p><p>This code is valid for 3 minutes.<br>ChatBasket</p>"
	if err := clients.SendEmail([]string{payload.NewEmail}, subject, body); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to send email: "+err.Error())
	}

	return &kit.StatusOkay{
		Status:  true,
		Message: updateID.String(), // Return update_id as message
	}, nil
}

// ConfirmEmailUpdate verifies OTP and updates email
// Step 2 of two-step email update flow
func (s *AuthService) ConfirmEmailUpdate(ctx context.Context, payload *ConfirmEmailUpdatePayload, userID uuid.UUID) (*kit.StatusOkay, error) {
	// Parse update_id
	updateID, err := uuid.Parse(payload.UpdateID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "Invalid update ID")
	}

	// Get verification code by user ID
	record, err := s.PostgresQuerier.GetVerificationCode(ctx, core_auth_store.GetVerificationCodeParams{
		ID:   userID,
		Type: "email_update",
	})
	if err != nil {
		return nil, kit.NewError(http.StatusNotFound, "not_found", "Verification code not found or expired")
	}

	// Verify update_id matches
	if record.UpdateID == nil || *record.UpdateID != updateID {
		return nil, kit.NewError(http.StatusUnauthorized, "flow_error", "Request session invalid")
	}

	// Check expiry (3 minutes)
	if IsExpiredOTP(record.CreatedAt, 3) {
		// Delete expired code
		_ = s.PostgresQuerier.DeleteVerificationCode(ctx, userID)
		return nil, kit.NewError(http.StatusUnauthorized, "otp_expired", "OTP has expired")
	}

	// Verify OTP
	match, err := VerifyOTP(payload.Otp, record.CodeHash)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to verify OTP: "+err.Error())
	}
	if !match {
		return nil, kit.NewError(http.StatusUnauthorized, "invalid_otp", "Invalid OTP")
	}

	// Update email (new email is stored in record.Email)
	err = s.PostgresQuerier.UpdateAuthUserEmail(ctx, core_auth_store.UpdateAuthUserEmailParams{
		ID:              userID,
		Email:           record.Email, // Use email from verification code
		IsEmailVerified: true,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to update email: "+err.Error())
	}

	// Delete verification code
	_ = s.PostgresQuerier.DeleteVerificationCode(ctx, userID)

	return &kit.StatusOkay{
		Status:  true,
		Message: "Email updated successfully",
	}, nil
}

