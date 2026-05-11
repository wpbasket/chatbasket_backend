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

// allowedUpdateOTPTypes is the closed set of update_type values accepted by
// RequestUpdateOTP. It doubles as an allowlist (defends against an attacker
// pushing a free-form string into payload.UpdateType) and as the lookup key
// into the shared `otpCopies` wording table in core_auth_svc_flows.go — so a
// new flow becomes a one-line addition in *both* places, with the compiler
// catching any drift.
var allowedUpdateOTPTypes = map[string]struct{}{
	"password_update": {},
	"email_update":    {},
}

// RequestUpdateOTP sends OTP for update operations (password, email, etc.)
// Step 1 of two-step update flow.
//
// The OTP email is rendered through the same `buildOTPEmail` helper used by
// the login / signup / password-reset flows, so password_update and
// email_update mails get the same branded, multipart/alternative,
// spam-hardened layout — not the bare "<h1>OTP</h1>" template that used to
// land in users' Spam folders.
func (s *AuthService) RequestUpdateOTP(ctx context.Context, payload *RequestUpdateOTPPayload, userID uuid.UUID) (*kit.StatusOkay, error) {
	// Validate update_type against the known set before doing anything
	// expensive (DB writes, OTP generation, mail send).
	if _, ok := allowedUpdateOTPTypes[payload.UpdateType]; !ok {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "Unsupported update_type")
	}

	// Get user from database
	user, err := s.PostgresQuerier.GetAuthUserByID(ctx, userID)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to get user: "+err.Error())
	}

	// Generate OTP
	if err := s.CheckOTPSendRateLimit(ctx, userID); err != nil {
		return nil, err
	}
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

	// Render the branded transactional template and send through the relay
	// with a stable RefID — used by the relay as X-Entity-Ref-ID, which
	// helps deliverability and downstream tracing.
	subject, htmlBody, textBody := buildOTPEmail(payload.UpdateType, otp)
	refID := "otp-" + payload.UpdateType
	if err := clients.SendEmailExt([]string{user.Email}, subject, htmlBody, textBody, refID); err != nil {
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
	updateID, err := kit.StringToUUID(payload.UpdateID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "Invalid update ID")
	}

	// 2. Lockout check
	if err := s.CheckOTPVerifyRateLimit(ctx, userID); err != nil {
		return nil, err
	}

	// 3. Get verification code by user ID
	record, err := s.PostgresQuerier.GetVerificationCode(ctx, core_auth_store.GetVerificationCodeParams{
		ID:   userID,
		Type: "password_update",
	})
	if err != nil {
		return nil, kit.NewError(http.StatusNotFound, "not_found", "Verification code not found or expired")
	}

	// 4. Verify update_id matches
	if record.UpdateID == nil || *record.UpdateID != updateID {
		return nil, kit.NewError(http.StatusUnauthorized, "flow_error", "Request session invalid")
	}

	// 5. Check expiry (3 minutes)
	if IsExpiredOTP(record.CreatedAt, 3) {
		// Delete expired code
		_ = s.PostgresQuerier.DeleteVerificationCode(ctx, userID)
		return nil, kit.NewError(http.StatusUnauthorized, "otp_expired", "OTP has expired")
	}

	// 6. Verify OTP
	match, err := VerifyOTP(payload.Otp, record.CodeHash)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to verify OTP: "+err.Error())
	}
	if !match {
		_ = s.RecordVerifyError(ctx, userID)
		return nil, kit.NewError(http.StatusUnauthorized, "invalid_otp", "Invalid OTP")
	}

	// 7. Reset lockout on success
	_ = s.ResetVerifyErrors(ctx, userID)

	// 8. Hash new password
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
	if err := s.CheckOTPSendRateLimit(ctx, userID); err != nil {
		return nil, err
	}
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

	// Send OTP to the *new* email through the same branded template used by
	// every other OTP flow. RefID lets the relay tag the outgoing message,
	// improving deliverability vs. the previous bare-bones template.
	subject, htmlBody, textBody := buildOTPEmail("email_update", otp)
	if err := clients.SendEmailExt([]string{payload.NewEmail}, subject, htmlBody, textBody, "otp-email_update"); err != nil {
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
	updateID, err := kit.StringToUUID(payload.UpdateID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "Invalid update ID")
	}

	// 2. Lockout check
	if err := s.CheckOTPVerifyRateLimit(ctx, userID); err != nil {
		return nil, err
	}

	// 3. Get verification code by user ID
	record, err := s.PostgresQuerier.GetVerificationCode(ctx, core_auth_store.GetVerificationCodeParams{
		ID:   userID,
		Type: "email_update",
	})
	if err != nil {
		return nil, kit.NewError(http.StatusNotFound, "not_found", "Verification code not found or expired")
	}

	// 4. Verify update_id matches
	if record.UpdateID == nil || *record.UpdateID != updateID {
		return nil, kit.NewError(http.StatusUnauthorized, "flow_error", "Request session invalid")
	}

	// 5. Check expiry (3 minutes)
	if IsExpiredOTP(record.CreatedAt, 3) {
		// Delete expired code
		_ = s.PostgresQuerier.DeleteVerificationCode(ctx, userID)
		return nil, kit.NewError(http.StatusUnauthorized, "otp_expired", "OTP has expired")
	}

	// 6. Verify OTP
	match, err := VerifyOTP(payload.Otp, record.CodeHash)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to verify OTP: "+err.Error())
	}
	if !match {
		_ = s.RecordVerifyError(ctx, userID)
		return nil, kit.NewError(http.StatusUnauthorized, "invalid_otp", "Invalid OTP")
	}

	// 7. Reset lockout on success
	_ = s.ResetVerifyErrors(ctx, userID)

	// 8. Update email (new email is stored in record.Email)
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

// CheckOTPSendRateLimit enforces cooldowns and caps for sending OTPs.
func (s *AuthService) CheckOTPSendRateLimit(ctx context.Context, userID uuid.UUID) error {
	const (
		maxHourly    = 5
		maxDaily     = 15
		cooldown     = 5 * time.Second // 5 seconds (Safety buffer)
		hourlyWindow = 1 * time.Hour
		dailyWindow  = 24 * time.Hour
	)

	limiter, err := s.PostgresQuerier.GetAuthRateLimiter(ctx, userID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}

	now := time.Now()

	// Default values for a new or reset record
	hourlyCount := int32(1)
	dailyCount := int32(1)
	dailyResetAt := now

	if err == nil {
		lastSend := kit.DerefTime(limiter.LastOtpSendAt)

		// 1. Cooldown check (2 minutes)
		if !lastSend.IsZero() && now.Sub(lastSend) < cooldown {
			return kit.NewError(http.StatusTooManyRequests, "cooldown_active", "Please wait 2 minutes before requesting another code.")
		}

		// 2. Daily check (15 per 24h)
		dailyReset := kit.DerefTime(limiter.DailyResetAt)
		if dailyReset.IsZero() || now.Sub(dailyReset) > dailyWindow {
			// Window expired or never set, reset daily counter
			dailyCount = 1
			dailyResetAt = now
		} else {
			if limiter.OtpDailyCount >= maxDaily {
				return kit.NewError(http.StatusTooManyRequests, "daily_limit_exceeded", "Daily OTP limit reached. Please try again tomorrow.")
			}
			dailyCount = limiter.OtpDailyCount + 1
			dailyResetAt = dailyReset
		}

		// 3. Hourly check (5 per hour)
		// We use the last send time to determine the hourly window
		if !lastSend.IsZero() && now.Sub(lastSend) < hourlyWindow {
			if limiter.OtpHourlyCount >= maxHourly {
				return kit.NewError(http.StatusTooManyRequests, "hourly_limit_exceeded", "Too many requests. Please try again in an hour.")
			}
			hourlyCount = limiter.OtpHourlyCount + 1
		} else {
			hourlyCount = 1
		}
	}

	_, err = s.PostgresQuerier.UpsertAuthRateLimiterSend(ctx, core_auth_store.UpsertAuthRateLimiterSendParams{
		AuthUserID:     userID,
		OtpHourlyCount: hourlyCount,
		OtpDailyCount:  dailyCount,
		LastOtpSendAt:  &now,
		DailyResetAt:   &dailyResetAt,
	})
	return err
}

// CheckOTPVerifyRateLimit prevents brute-force attacks by blocking verification after 3 failures.
func (s *AuthService) CheckOTPVerifyRateLimit(ctx context.Context, userID uuid.UUID) error {
	const (
		maxErrors    = 3
		lockDuration = 15 * time.Minute
	)

	limiter, err := s.PostgresQuerier.GetAuthRateLimiter(ctx, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil // No record yet, no errors
		}
		return err
	}

	if limiter.OtpVerifyErrors >= maxErrors {
		lastVerify := kit.DerefTime(limiter.LastVerifyAttemptAt)
		if !lastVerify.IsZero() && time.Since(lastVerify) < lockDuration {
			return kit.NewError(http.StatusTooManyRequests, "brute_force_lockout", "Too many failed attempts. Please try again in 15 minutes.")
		}
		// If lockout expired, we don't return error here (the next failed attempt will reset it or success will clear it)
	}

	return nil
}

// RecordVerifyError increments the error counter after a failed OTP verification.
func (s *AuthService) RecordVerifyError(ctx context.Context, userID uuid.UUID) error {
	limiter, err := s.PostgresQuerier.GetAuthRateLimiter(ctx, userID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}

	errors := int32(1)
	if err == nil {
		errors = limiter.OtpVerifyErrors + 1
	}

	now := time.Now()
	_, err = s.PostgresQuerier.UpsertAuthRateLimiterVerify(ctx, core_auth_store.UpsertAuthRateLimiterVerifyParams{
		AuthUserID:          userID,
		OtpVerifyErrors:     errors,
		LastVerifyAttemptAt: &now,
	})
	return err
}

// ResetVerifyErrors clears the error counter after a successful OTP verification.
func (s *AuthService) ResetVerifyErrors(ctx context.Context, userID uuid.UUID) error {
	return s.PostgresQuerier.ResetVerifyErrors(ctx, userID)
}


