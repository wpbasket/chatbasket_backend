package personalServices

import (
	"chatbasket/model"
	"context"
	"net/http"
)

// Template: mirror public settings methods for personal mode. Implement later.

func (ps *Service) UpdatePassword(ctx context.Context, payload *model.UpdatePassword, userId string) (*model.StatusOkay, *model.ApiError) {
	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal UpdatePassword not implemented", Type: "not_implemented"}
}

func (ps *Service) UpdateEmail(ctx context.Context, payload *model.UpdateEmailPayload, userId string) (*model.StatusOkay, *model.ApiError) {
	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal UpdateEmail not implemented", Type: "not_implemented"}
}

func (ps *Service) UpdateEmailVerification(ctx context.Context, payload *model.UpdateEmailVerification, userId string) (*model.StatusOkay, *model.ApiError) {
	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal UpdateEmailVerification not implemented", Type: "not_implemented"}
}

func (ps *Service) SendOtp(ctx context.Context, payload *model.SendOtpPayload, userId string) (*model.StatusOkay, *model.ApiError) {
	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal SendOtp not implemented", Type: "not_implemented"}
}

func (ps *Service) VerifyOtp(ctx context.Context, payload *model.OtpVerificationPayload, userId string) (*model.StatusOkay, *model.ApiError) {
	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal VerifyOtp not implemented", Type: "not_implemented"}
}
