package personalServices

import (
	"chatbasket/model"
	"chatbasket/personalModel"
	"chatbasket/personalUtils"
	"chatbasket/utils"
	"context"

	// "mime/multipart"
	"net/http"

	"github.com/appwrite/sdk-for-go/id"
	"github.com/appwrite/sdk-for-go/query"
	"github.com/google/uuid"
)

// Template: mirror public profile methods for personal mode. Implement later.

func (ps *Service) Logout(ctx context.Context, payload *personalmodel.LogoutPayload, userId, sessionId string) (*model.StatusOkay, *model.ApiError) {
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

func (ps *Service) CreateUserProfile(ctx context.Context, payload *personalmodel.CreateUserProfilePayload, userId string, email string) (*personalmodel.PrivateUser, *model.ApiError) {
	resUser, err := ps.Appwrite.Database.ListDocuments(
		ps.Appwrite.PersonalDatabaseID,
		ps.Appwrite.PersonalUsersCollectionID,
		ps.Appwrite.Database.WithListDocumentsQueries(
			[]string{
				query.Equal("$id", userId),
				query.Limit(1),
			},
		),
	)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to query user", Type: "internal_server_error"}
	}
	if resUser.Total == 1 {
		return nil, &model.ApiError{Code: http.StatusConflict, Message: "User already exists", Type: "conflict"}
	}
	generatedUsername, err := personalutils.GenerateRandomUsername()
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Username generation failed", Type: "internal_server_error"}
	}
	sha256Username, err := utils.HashUsername(generatedUsername, ps.Appwrite.PersonalUsernameKey)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Username hashing failed", Type: "internal_server_error"}
	}
	b64CipherChacha20Poly1305Username, err := utils.EncryptUsername(generatedUsername, ps.Appwrite.PersonalUsernameKey, userId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Username encryption failed", Type: "internal_server_error"}
	}
	dbPayload := personalmodel.CreateUserProfileDbPayload{
		HmacSha256HexUsername:             sha256Username,
		B64CipherChacha20Poly1305Username: b64CipherChacha20Poly1305Username,
		Name:                              payload.Name,
		Bio:                               payload.Bio,
		ProfileType:                       payload.ProfileType,
	}
	doc, err := ps.Appwrite.Database.CreateDocument(
		ps.Appwrite.PersonalDatabaseID,
		ps.Appwrite.PersonalUsersCollectionID,
		id.Custom(userId),
		dbPayload,
	)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to create user", Type: "internal_server_error"}
	}

    aloneUsernameDbPayload:=personalmodel.AloneUsernameDbPayload{
		Username: generatedUsername,
	}
	_, err = ps.Appwrite.Database.CreateDocument(
		ps.Appwrite.PersonalDatabaseID,
		ps.Appwrite.AloneUsernameCollectionID,
		id.Custom(uuid.New().String()),
		aloneUsernameDbPayload,
	)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to create alone username", Type: "internal_server_error"}
	}

	var responseUser personalmodel.User
	if err := doc.Decode(&responseUser); err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "Failed to parse user", Type: "internal_server_error"}
	}
	return personalmodel.ToPrivateUser(&responseUser, generatedUsername, email), nil
}

func (ps *Service) GetProfile(ctx context.Context, userId string, email string) (*personalmodel.PrivateUser, *model.ApiError) {
	profile, err := ps.Appwrite.Database.ListDocuments(
		ps.Appwrite.PersonalDatabaseID,
		ps.Appwrite.PersonalUsersCollectionID,
		ps.Appwrite.Database.WithListDocumentsQueries(
			[]string{
				query.Equal("$id", userId),
				query.Limit(1),
			},
		),
	)

	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "personal GetProfile failed", Type: "internal_server_error"}
	}

	if profile.Total == 0 {
		return nil, &model.ApiError{Code: http.StatusNotFound, Message: "personal GetProfile not found", Type: "not_found"}
	}
	var responseUser model.Documents[personalmodel.User]
	if err := profile.Decode(&responseUser); err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to parse user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	decodeUsername, err := utils.DecryptUsername(responseUser.Documents[0].B64CipherChacha20Poly1305Username, ps.Appwrite.PersonalUsernameKey)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "personal GetProfile failed", Type: "internal_server_error"}
	}

	return personalmodel.ToPrivateUser(&responseUser.Documents[0], decodeUsername, email), nil
}






// func (ps *Service) UploadUserProfilePicture(ctx context.Context, fh *multipart.FileHeader, userId string) (*model.UploadUserProfilePictureResponse, *model.ApiError) {
// 	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal UploadUserProfilePicture not implemented", Type: "not_implemented"}
// }

// func (ps *Service) RemoveUserProfilePicture(ctx context.Context, userId string) (*model.StatusOkay, *model.ApiError) {
// 	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal RemoveUserProfilePicture not implemented", Type: "not_implemented"}
// }

// func (ps *Service) UpdateUserProfile(ctx context.Context, payload *model.UpdateUserProfilePayload, userId string) (*model.PrivateUser, *model.ApiError) {
// 	return nil, &model.ApiError{Code: http.StatusNotImplemented, Message: "personal UpdateUserProfile not implemented", Type: "not_implemented"}
// }
