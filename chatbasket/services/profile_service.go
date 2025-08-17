package services

import (
	"chatbasket/model"
	"chatbasket/utils"
	"context"
	"github.com/appwrite/sdk-for-go/query"
	"mime/multipart"
	"os"
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
	if userRes.Total == 0 {
		return &model.StatusOkay{Status: true, Message: "Username is available"}, nil
	} else {
		return &model.StatusOkay{Status: false, Message: "Username is not available"}, nil
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
	dbUserPayload := model.CreateOrUpdateUserProfile{
		Username:         payload.Username,
		Name:             payload.Name,
		Email:            userEmail,
		Bio:              payload.Bio,
		ProfileVisibleTo: payload.ProfileVisibleTo,
	}

	check, err := ps.Appwrite.Database.ListDocuments(
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

	usernameCheck, err := ps.Appwrite.Database.ListDocuments(
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

	if usernameCheck.Total > 0 {
		return nil, &model.ApiError{
			Code:    409,
			Message: "Username already exists",
			Type:    "conflict_username",
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
	var resUser model.User
	if err := doc.Decode(&resUser); err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to parse created user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	return model.ToPrivateUser(&resUser), nil
}

func (ps *GlobalService) GetProfile(ctx context.Context, userId string) (*model.PrivateUser, *model.ApiError) {

	getEmail, err := ps.Appwrite.Users.Get(userId)
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

	var responseUser model.Documents[model.User]

	if err := user.Decode(&responseUser); err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to parse user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	finalResponse := responseUser.Documents[0]

	return model.ToPrivateUser(&finalResponse), nil

}

func (ps *GlobalService) UploadUserProfilePicture(ctx context.Context, fh *multipart.FileHeader, userId string) (*model.UploadUserProfilePictureResponse, *model.ApiError) {
	fileTemp, err := utils.ConvertToInputFile(fh)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to process file: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Clean up temp file after upload
	defer func() {
		if fileTemp.Path != "" {
			os.Remove(fileTemp.Path)
		}
	}()

	resUser, err := ps.Appwrite.Database.GetDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		userId,
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	var user model.User
	if err := resUser.Decode(&user); err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to parse user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	if user.Avatar == userId {

		listFilesRes, err := ps.Appwrite.Storage.ListFiles(
			ps.Appwrite.ProfilePicBucketID,
			ps.Appwrite.Storage.WithListFilesQueries([]string{
				query.Equal("$id", userId),
				query.Limit(1),
			}),
		)

		if err != nil {
			return nil, &model.ApiError{
				Code:    500,
				Message: "Failed to query profile picture: " + err.Error(),
				Type:    "internal_server_error",
			}
		}

		if listFilesRes.Total == 1 {
			_, err := ps.Appwrite.Storage.DeleteFile(
				ps.Appwrite.ProfilePicBucketID,
				userId,
			)
			if err != nil {
				return nil, &model.ApiError{
					Code:    500,
					Message: "Failed to delete old profile picture: " + err.Error(),
					Type:    "internal_server_error",
				}
			}
		}

	}

	fileId := userId

	uploadRes, err := ps.Appwrite.Storage.CreateFile(
		ps.Appwrite.ProfilePicBucketID,
		fileId,
		fileTemp,
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to upload file: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	personalToken, err := ps.Appwrite.Tokens.CreateFileToken(
		ps.Appwrite.ProfilePicBucketID,
		fileId,
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to create personal token: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	publicToken, err := ps.Appwrite.Tokens.CreateFileToken(
		ps.Appwrite.ProfilePicBucketID,
		fileId,
	)

	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to create public token: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// fileUrl := "https://fra.cloud.appwrite.io/v1/storage/buckets/685bc613002edcfee6bb/files/" + "" + "/view?project=6858ed4d0005c859ea03&token=" + ""
	return &model.UploadUserProfilePictureResponse{
		FileId:       uploadRes.Id,
		Name:         uploadRes.Name,
		AvatarTokens: []string{personalToken.Id, publicToken.Id, personalToken.Secret, publicToken.Secret},
	}, nil
}

func (ps *GlobalService) UpdateUserProfile(ctx context.Context, payload *model.UpdateUserProfilePayload, userId string) (*model.PrivateUser, *model.ApiError) {

	_, err := ps.Appwrite.Users.Get(userId)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	updatePayload := model.UpdateUserProfile{
		Username:         payload.Username,
		Name:             payload.Name,
		Bio:              payload.Bio,
		Avatar:           payload.Avatar,
		AvatarTokens:     payload.AvatarTokens,
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
