package commonservice

import (
	"chatbasket-api/common/commonmodel"
	"chatbasket-api/internal/db/auth"
	"chatbasket-api/model"
	"chatbasket-api/utils"
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// RequestUpdateOTP sends OTP for update operations (password, email, etc.)
// Step 1 of two-step update flow
func (s *Service) RequestUpdateOTP(ctx context.Context, payload *commonmodel.RequestUpdateOTPPayload, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	// Get user UUID from middleware
	userUUID := userId.UuidUserId

	// Get user from database
	user, err := s.AuthQueries.GetAuthUserByID(ctx, userUUID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to get user: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Generate OTP
	otp, err := utils.GenerateOTP()
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to generate OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Hash OTP
	hashedOTP, err := utils.HashOTP(otp)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to hash OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Generate update_id
	updateID := uuid.New()

	// Store verification code with update_id
	_, err = s.AuthQueries.CreateVerificationCodeWithUpdateID(ctx, auth.CreateVerificationCodeWithUpdateIDParams{
		ID:       userUUID,
		UpdateID: pgtype.UUID{Bytes: updateID, Valid: true},
		Email:    user.Email,
		CodeHash: hashedOTP,
		Type:     payload.UpdateType, // "password_update" or "email_update"
	})
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to store verification code: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Send OTP email
	subject := "OTP for " + payload.UpdateType
	body := "<p>Hello,<br>Your OTP for " + payload.UpdateType + " is:<br><h1>" + otp + "</h1></p><p>This code is valid for 3 minutes.<br>ChatBasket</p>"
	if appErr := utils.SendEmail([]string{user.Email}, subject, body); appErr != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to send email: " + appErr.Message,
			Type:    "internal_server_error",
		}
	}

	return &model.StatusOkay{
		Status:  true,
		Message: updateID.String(), // Return update_id as message
	}, nil
}

// ConfirmPasswordUpdate verifies OTP and updates password
// Step 2 of two-step password update flow
func (s *Service) ConfirmPasswordUpdate(ctx context.Context, payload *commonmodel.ConfirmPasswordUpdatePayload, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	// Get user UUID from middleware
	userUUID := userId.UuidUserId

	// Parse update_id
	updateID, err := uuid.Parse(payload.UpdateID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid update ID",
			Type:    "bad_request",
		}
	}

	// Get verification code by user ID
	record, err := s.AuthQueries.GetVerificationCode(ctx, auth.GetVerificationCodeParams{
		ID:   userUUID,
		Type: "password_update",
	})
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusNotFound,
			Message: "Verification code not found or expired",
			Type:    "not_found",
		}
	}

	// Verify update_id matches
	if !record.UpdateID.Valid || record.UpdateID.Bytes != updateID {
		return nil, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Request session invalid",
			Type:    "flow_error",
		}
	}

	// Check expiry (3 minutes)
	if utils.IsExpiredOTP(record.CreatedAt.Time, 3) {
		// Delete expired code
		_ = s.AuthQueries.DeleteVerificationCode(ctx, userUUID)
		return nil, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "OTP has expired",
			Type:    "otp_expired",
		}
	}

	// Verify OTP
	match, err := utils.VerifyOTP(payload.Otp, record.CodeHash)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to verify OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if !match {
		return nil, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Invalid OTP",
			Type:    "invalid_otp",
		}
	}

	// Hash new password
	hashedPassword, appErr := utils.HashPassword(payload.NewPassword)
	if appErr != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to hash password: " + appErr.Message,
			Type:    "internal_server_error",
		}
	}

	// Update password
	err = s.AuthQueries.UpdateAuthUserPassword(ctx, auth.UpdateAuthUserPasswordParams{
		ID:           userUUID,
		PasswordHash: hashedPassword,
	})
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to update password: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Delete verification code
	_ = s.AuthQueries.DeleteVerificationCode(ctx, userUUID)

	return &model.StatusOkay{
		Status:  true,
		Message: "Password updated successfully",
	}, nil
}

// RequestEmailUpdate sends OTP to new email for verification
// Step 1 of two-step email update flow
func (s *Service) RequestEmailUpdate(ctx context.Context, payload *commonmodel.RequestEmailUpdatePayload, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	// Get user UUID from middleware
	userUUID := userId.UuidUserId

	// Get user from database
	user, err := s.AuthQueries.GetAuthUserByID(ctx, userUUID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to get user: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Verify password
	match, appErr := utils.VerifyPassword(payload.Password, user.PasswordHash)
	if appErr != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to verify password: " + appErr.Message,
			Type:    "internal_server_error",
		}
	}
	if !match {
		return nil, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Invalid password",
			Type:    "invalid_password",
		}
	}

	// Check if new email already exists
	exists, err := s.AuthQueries.CheckEmailExists(ctx, payload.NewEmail)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to check email: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if exists {
		return nil, &model.ApiError{
			Code:    http.StatusConflict,
			Message: "Email already registered",
			Type:    "conflict",
		}
	}

	// Generate OTP
	otp, err := utils.GenerateOTP()
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to generate OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Hash OTP
	hashedOTP, err := utils.HashOTP(otp)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to hash OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Generate update_id
	updateID := uuid.New()

	// Store verification code with update_id and new email
	_, err = s.AuthQueries.CreateVerificationCodeWithUpdateID(ctx, auth.CreateVerificationCodeWithUpdateIDParams{
		ID:       userUUID,
		UpdateID: pgtype.UUID{Bytes: updateID, Valid: true},
		Email:    payload.NewEmail, // Store new email in verification code
		CodeHash: hashedOTP,
		Type:     "email_update",
	})
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to store verification code: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Send OTP to new email
	subject := "OTP for email update"
	body := "<p>Hello,<br>Your OTP for email update is:<br><h1>" + otp + "</h1></p><p>This code is valid for 3 minutes.<br>ChatBasket</p>"
	if appErr := utils.SendEmail([]string{payload.NewEmail}, subject, body); appErr != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to send email: " + appErr.Message,
			Type:    "internal_server_error",
		}
	}

	return &model.StatusOkay{
		Status:  true,
		Message: updateID.String(), // Return update_id as message
	}, nil
}

// ConfirmEmailUpdate verifies OTP and updates email
// Step 2 of two-step email update flow
func (s *Service) ConfirmEmailUpdate(ctx context.Context, payload *commonmodel.ConfirmEmailUpdatePayload, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	// Get user UUID from middleware
	userUUID := userId.UuidUserId

	// Parse update_id
	updateID, err := uuid.Parse(payload.UpdateID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid update ID",
			Type:    "bad_request",
		}
	}

	// Get verification code by user ID
	record, err := s.AuthQueries.GetVerificationCode(ctx, auth.GetVerificationCodeParams{
		ID:   userUUID,
		Type: "email_update",
	})
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusNotFound,
			Message: "Verification code not found or expired",
			Type:    "not_found",
		}
	}

	// Verify update_id matches
	if !record.UpdateID.Valid || record.UpdateID.Bytes != updateID {
		return nil, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Request session invalid",
			Type:    "flow_error",
		}
	}

	// Check expiry (3 minutes)
	if utils.IsExpiredOTP(record.CreatedAt.Time, 3) {
		// Delete expired code
		_ = s.AuthQueries.DeleteVerificationCode(ctx, userUUID)
		return nil, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "OTP has expired",
			Type:    "otp_expired",
		}
	}

	// Verify OTP
	match, err := utils.VerifyOTP(payload.Otp, record.CodeHash)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to verify OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if !match {
		return nil, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "Invalid OTP",
			Type:    "invalid_otp",
		}
	}

	// Update email (new email is stored in record.Email)
	err = s.AuthQueries.UpdateAuthUserEmail(ctx, auth.UpdateAuthUserEmailParams{
		ID:              userUUID,
		Email:           record.Email, // Use email from verification code
		IsEmailVerified: true,
	})
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to update email: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Delete verification code
	_ = s.AuthQueries.DeleteVerificationCode(ctx, userUUID)

	return &model.StatusOkay{
		Status:  true,
		Message: "Email updated successfully",
	}, nil
}
