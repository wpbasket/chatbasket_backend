package personalServices

import (
	"chatbasket/model"
	"context"
	"mime/multipart"
	"net/http"
)

// Template: mirror public profile methods for personal mode. Implement later.

func (ps *Service) Logout(ctx context.Context, payload *model.LogoutPayload, userId, sessionId string) (*model.StatusOkay, *model.ApiError) {
	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal Logout not implemented", Type: "not_implemented"}
}

func (ps *Service) CheckIfUserNameAvailable(ctx context.Context, payload *model.CheckIfUserNameAvailablePayload) (*model.StatusOkay, *model.ApiError) {
	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal CheckIfUserNameAvailable not implemented", Type: "not_implemented"}
}

func (ps *Service) CreateUserProfile(ctx context.Context, payload *model.CreateUserProfilePayload, userId string) (*model.PrivateUser, *model.ApiError) {
	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal CreateUserProfile not implemented", Type: "not_implemented"}
}

func (ps *Service) GetProfile(ctx context.Context, userId string) (*model.PrivateUser, *model.ApiError) {
	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal GetProfile not implemented", Type: "not_implemented"}
}

func (ps *Service) UploadUserProfilePicture(ctx context.Context, fh *multipart.FileHeader, userId string) (*model.UploadUserProfilePictureResponse, *model.ApiError) {
	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal UploadUserProfilePicture not implemented", Type: "not_implemented"}
}

func (ps *Service) RemoveUserProfilePicture(ctx context.Context, userId string) (*model.StatusOkay, *model.ApiError) {
	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal RemoveUserProfilePicture not implemented", Type: "not_implemented"}
}

func (ps *Service) UpdateUserProfile(ctx context.Context, payload *model.UpdateUserProfilePayload, userId string) (*model.PrivateUser, *model.ApiError) {
	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal UpdateUserProfile not implemented", Type: "not_implemented"}
}
