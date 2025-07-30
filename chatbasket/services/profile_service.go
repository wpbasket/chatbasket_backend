package services

import (
	"chatbasket/model"
	"context"
	"github.com/appwrite/sdk-for-go/query"
)

func (ps *GlobalService) Logout(ctx context.Context, payload *model.LogoutPayload, userId, sessionId string) (*model.StatusOkay, *model.ApiError) {

	if payload.AllSessions {
		_, err := ps.Appwrite.Users.DeleteSessions(userId)
		if err != nil {
			return nil, &model.ApiError{
				Code:    401,
				Message: "Failed to Logout from all sessions: " + err.Error(),
				Type:    "unauthorized",
			}
		}
	} else {
		_, err := ps.Appwrite.Users.DeleteSession(userId, sessionId)
		if err != nil {
			return nil, &model.ApiError{
				Code:    401,
				Message: "Failed to Logout from session: " + err.Error(),
				Type:    "unauthorized",
			}
		}
	}

	return &model.StatusOkay{Status: true, Message: "Logged out successfully"}, nil
}

func (ps *GlobalService) CheckIfUserNameAvailable(ctx context.Context, payload *model.CheckIfUserNameAvailablePayload) (*model.StatusOkay, *model.ApiError) {

	userRes, err := ps.Appwrite.Database.ListDocuments(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		ps.Appwrite.Database.WithListDocumentsQueries([]string{
			query.Equal("username", payload.Username),
			query.Limit(1),
		}),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if userRes.Total == 1 {
		return &model.StatusOkay{Status: false, Message: "Username already exists"}, nil
	} else {
		return &model.StatusOkay{Status: true, Message: "Username is available"}, nil
	}

}

func (ps *GlobalService) CreateUserProfile(ctx context.Context, payload *model.CreateUserProfilePayload, userId string) (*model.PrivateUser, *model.ApiError) {

	user, err := ps.Appwrite.Users.Get(userId)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	userEmail := user.Email
	dbUserPayload := model.CreateUserProfile{
		Username:         payload.Username,
		Name:             payload.Name,
		Email:            userEmail,
		Bio:              payload.Bio,
		Avatar:           payload.Avatar,
		ProfileVisibleTo: payload.ProfileVisibleTo,
	}

	check,err := ps.Appwrite.Database.ListDocuments(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		ps.Appwrite.Database.WithListDocumentsQueries([]string{
			query.Equal("email", userEmail),
			query.Limit(1),
		}),
	)

	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	
	if check.Total > 0 {
		return nil, &model.ApiError{
			Code:    409,
			Message: "User profile already exists",
			Type:    "conflict",
		}
	}

	doc, err := ps.Appwrite.Database.CreateDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		userId,
		dbUserPayload,
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to create user profile in database: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	var privateUser model.PrivateUser
	if err := doc.Decode(&privateUser); err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to parse created user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	return &privateUser, nil
}

func (ps *GlobalService) GetProfile(ctx context.Context, userId string) (*model.PrivateUser, *model.ApiError) {

	getEmail,err := ps.Appwrite.Users.Get(userId)	
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query user data: " + err.Error(),
			Type:    "internal_server_error",	
		}
	}
	
	user, err := ps.Appwrite.Database.ListDocuments(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		ps.Appwrite.Database.WithListDocumentsQueries([]string{
			query.Equal("email", getEmail.Email),
			query.Limit(1),	
		}),
	)	
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	if user.Total == 0 {
		return nil, &model.ApiError{
			Code:    404,
			Message: "User profile not found",
			Type:    "not_found",
		}
	}

	var privateUser model.PrivateUser
	if err := user.Decode(&privateUser); err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to parse user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	return &privateUser, nil

}






func (ps *GlobalService) UpdateUserProfile(ctx context.Context, payload *model.CreateUserProfilePayload,  userId string) (*model.PrivateUser, *model.ApiError) {

	user, err := ps.Appwrite.Users.Get(userId)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	updatePayload := model.CreateUserProfile{
		Username:         payload.Username,
		Name:             payload.Name,
		Email:            user.Email,
		Bio:              payload.Bio,
		Avatar:           payload.Avatar,
		ProfileVisibleTo: payload.ProfileVisibleTo,
	}

	doc, err := ps.Appwrite.Database.UpdateDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		userId,
		ps.Appwrite.Database.WithUpdateDocumentData(updatePayload),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to update user profile in database: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	var privateUser model.PrivateUser
	if err := doc.Decode(&privateUser); err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to parse updated user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	return &privateUser, nil
}