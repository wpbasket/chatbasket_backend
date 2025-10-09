package publicServices

import (
	"chatbasket/model"
	"chatbasket/services"
	"context"
	"mime/multipart"
	"time"

	"github.com/appwrite/sdk-for-go/query"
)

func (ps *Service) Logout(ctx context.Context, payload *model.LogoutPayload, userId, sessionId string) (*model.StatusOkay, *model.ApiError) {

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

func (ps *Service) CheckIfUserNameAvailable(ctx context.Context, payload *model.CheckIfUserNameAvailablePayload) (*model.StatusOkay, *model.ApiError) {

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

func (ps *Service) CreateUserProfile(ctx context.Context, payload *model.CreateUserProfilePayload, userId string) (*model.PrivateUser, *model.ApiError) {

	user, err := ps.Appwrite.Users.Get(userId)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	userEmail := user.Email
	dbUserPayload := model.CreateUserProfileDbPayload{
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

	return model.ToPrivateUser(&resUser, ""), nil
}

func (ps *Service) GetProfile(ctx context.Context, userId string) (*model.PrivateUser, *model.ApiError) {

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
	avatarData := model.AppwriteFileData{
		FileId:     finalResponse.AvatarFileId,
		FileTokens: finalResponse.AvatarFileTokens,
	}


	if len(finalResponse.AvatarFileTokens) >=3 {

		if finalResponse.AvatarFileTokens[2]<time.Now().Format("2006-01-02 15:04:05") {
			exp:=time.Now().AddDate(1, 0, 0).Format("2006-01-02 15:04:05")
			tok,err := ps.Appwrite.Tokens.CreateFileToken(ps.Appwrite.ProfilePicBucketID, finalResponse.AvatarFileId, ps.Appwrite.Tokens.WithCreateFileTokenExpire(exp))
			if err != nil {
				return nil, &model.ApiError{
					Code:    500,
					Message: "Failed to create personal token: " + err.Error(),
					Type:    "internal_server_error",
				}
			}
			_,err = ps.Appwrite.Database.UpdateDocument(
				ps.Appwrite.DatabaseID,
				ps.Appwrite.UsersCollectionID,
				userId,
				ps.Appwrite.Database.WithUpdateDocumentData(model.UploadUserProfilePictureDbPayload{
					AvatarFileTokens: []string{tok.Id,tok.Secret,exp},
				}),
			)
			if err != nil {
				return nil, &model.ApiError{
					Code:    500,
					Message: "Failed to update user data: " + err.Error(),
					Type:    "internal_server_error",
				}
			}
			avatarData.FileTokens = []string{tok.Id,tok.Secret,exp}

		}
	}


	avatarUri := model.BuildAvatarURI(&avatarData, 3)

	return model.ToPrivateUser(&finalResponse, avatarUri), nil

}

func (ps *Service) UploadUserProfilePicture(ctx context.Context, fh *multipart.FileHeader, userId string) (*model.UploadUserProfilePictureResponse, *model.ApiError) {
	// Fetch user to determine if an existing avatar (same fileId) should be deleted
	resUser, err := ps.Appwrite.Database.GetDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		userId,
	)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to query user data: " + err.Error(), Type: "internal_server_error"}
	}
	var user model.User
	if err := resUser.Decode(&user); err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to parse user data: " + err.Error(), Type: "internal_server_error"}
	}
	deleteExisting := user.AvatarFileId == userId

	result, apiErr := ps.UploadFileFromMultipart(
		ps.Appwrite.ProfilePicBucketID,
		userId,
		fh,
		services.UploadOptions{DeleteExisting: deleteExisting, GenerateTokens: true},
	)
	if apiErr != nil {
		return nil, apiErr
	}

	avatarTokens := []string{}
	if len(result.TokenIDs) == 1 && len(result.TokenSecrets) == 1  && result.Expire!="" {
		avatarTokens = []string{result.TokenIDs[0], result.TokenSecrets[0],result.Expire}
	}
	
	updatePayload := model.UploadUserProfilePictureDbPayload{
		AvatarFileId: result.FileId,
		AvatarFileTokens: avatarTokens,
	}
	_,err = ps.Appwrite.Database.UpdateDocument(ps.Appwrite.DatabaseID,ps.Appwrite.UsersCollectionID,userId,ps.Appwrite.Database.WithUpdateDocumentData(updatePayload))
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to update user data: " + err.Error(), Type: "internal_server_error"}
	}


	return &model.UploadUserProfilePictureResponse{
		AvatarFileId:     result.FileId,
		Name:             result.Name,
		AvatarFileTokens: avatarTokens,
	}, nil
}

func (ps *Service) RemoveUserProfilePicture(ctx context.Context, userId string) (*model.StatusOkay, *model.ApiError) {
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

	if user.AvatarFileId != userId {
		return nil, &model.ApiError{
			Code:    404,
			Message: "No profile picture found to remove",
			Type:    "not_found",
		}
	}

	// Delete the file access token
	tok, err := ps.Appwrite.Tokens.List(ps.Appwrite.ProfilePicBucketID, user.AvatarFileId)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query token data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if tok.Total > 0 {
		for _, token := range tok.Tokens {
			_, err := ps.Appwrite.Tokens.Delete(token.Id)
			if err != nil {
				return nil, &model.ApiError{
					Code:    500,
					Message: "Failed to delete token data: " + err.Error(),
					Type:    "internal_server_error",
				}
			}
		}
	}

	// Delete the file from storage
	_, err = ps.Appwrite.Storage.DeleteFile(
		ps.Appwrite.ProfilePicBucketID,
		userId,
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to delete profile picture from storage: " + err.Error(),
			Type:    "not_found",
		}
	}

	dataToUpdateInUserProfile := model.RemoveProfilePictureDbPayload{
		AvatarFileId:     nil,
		AvatarFileTokens: nil,
	}

	_, err = ps.Appwrite.Database.UpdateDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		userId,
		ps.Appwrite.Database.WithUpdateDocumentData(dataToUpdateInUserProfile),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to update user profile in database: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	return &model.StatusOkay{Status: true, Message: "Profile picture removed successfully"}, nil
}

func (ps *Service) UpdateUserProfile(ctx context.Context, payload *model.UpdateUserProfilePayload, userId string) (*model.PrivateUser, *model.ApiError) {

	_, err := ps.Appwrite.Users.Get(userId)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	updatePayload := model.UpdateUserProfileDbPayload{
		Username:         payload.Username,
		Name:             payload.Name,
		Bio:              payload.Bio,
		AvatarFileId:     payload.AvatarFileId,
		AvatarFileTokens: payload.AvatarFileTokens,
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

	var updatedUser model.User
	if err := doc.Decode(&updatedUser); err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to parse updated user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	avatarData := model.AppwriteFileData{
		FileId:     updatedUser.AvatarFileId,
		FileTokens: updatedUser.AvatarFileTokens,
	}
	avatarUri := model.BuildAvatarURI(&avatarData, 3)

	return model.ToPrivateUser(&updatedUser, avatarUri), nil
}
