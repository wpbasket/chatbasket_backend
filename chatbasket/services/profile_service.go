package services

import (
	"chatbasket/model"
	"chatbasket/utils"
	"context"
	"time"

	"github.com/appwrite/sdk-for-go/id"
	"github.com/appwrite/sdk-for-go/query"
	"github.com/labstack/echo/v4"
)

func (ps *GlobalService) Logout(ctx context.Context, payload *model.LogoutPayload, userId, sessionId string) (*model.StatusOkay, error) {

	if payload.AllSessions {
		_, err := ps.Appwrite.Users.DeleteSessions(userId)
		if err != nil {
			return nil, echo.NewHTTPError(401, "Failed to Logout  from all sessions: "+err.Error())
		}
	} else {
		_, err := ps.Appwrite.Users.DeleteSession(userId, sessionId)
		if err != nil {
			return nil, echo.NewHTTPError(401, "Failed to Logout from this session: "+err.Error())
		}
	}

	return &model.StatusOkay{Status: true, Message: "Logged out successfully"}, nil
}

func (ps *GlobalService) CheckIfUserNameAvailable(ctx context.Context, payload *model.CheckIfUserNameAvailablePayload) (*model.StatusOkay, error) {

	userRes, err := ps.Appwrite.Database.ListDocuments(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		ps.Appwrite.Database.WithListDocumentsQueries([]string{
			query.Equal("username", payload.Username),
			query.Limit(1),
		}),
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query username: "+err.Error())
	}
	if userRes.Total == 1 {
		return &model.StatusOkay{Status: false, Message: "Username already exists"}, nil
	} else {
		return &model.StatusOkay{Status: true, Message: "Username is available"}, nil
	}

}

func (ps *GlobalService) CreateUserProfile(ctx context.Context, payload *model.CreateUserProfilePayload, userId string) (*model.PrivateUser, error) {

	user, err := ps.Appwrite.Users.Get(userId)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query user data: "+err.Error())
	}
	userEmail := user.Email
	dbUserPayload := model.CreateUserProfile{
		Id:               userId,
		Username:         payload.Username,
		Name:             payload.Name,
		Email:            userEmail,
		Bio:              payload.Bio,
		Avatar:           payload.Avatar,
		ProfileVisibleTo: payload.ProfileVisibleTo,
	}

	doc, err := ps.Appwrite.Database.CreateDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		userId,
		dbUserPayload,
	)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to save user in database: "+err.Error())
	}
	var privateUser model.PrivateUser
	if err := doc.Decode(&privateUser); err != nil {
		return nil, echo.NewHTTPError(500, "Failed to parse user data: "+err.Error())
	}

	return &privateUser, nil
}

func (ps *GlobalService) GetProfile(ctx context.Context, userId string) (*model.PrivateUser, error) {

	user, err := ps.Appwrite.Database.GetDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		userId,
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query user data: "+err.Error())
	}

	var privateUser model.PrivateUser
	if err := user.Decode(&privateUser); err != nil {
		return nil, echo.NewHTTPError(401, "Failed to parse user data: "+err.Error())
	}

	return &privateUser, nil

}

func (ps *GlobalService) UpdateEmail(ctx context.Context, payload *model.UpdateEmailPayload, userId string) (*model.StatusOkay, error) {

	res, err := ps.Appwrite.Database.ListDocuments(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		ps.Appwrite.Database.WithListDocumentsQueries(
			[]string{
				query.Equal("email", payload.Email),
				query.Limit(1),
			},
		),
	)

	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query email: "+err.Error())
	}

	if res.Total == 1 {
		return &model.StatusOkay{Status: false, Message: "Email already exists"}, nil
	}

	messageId := id.Unique()
	subject := "Otp for email verification"
	otp, err := utils.GenerateOTP()
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to generate OTP: "+err.Error())
	}
	content := "Hello,\n\nYour One-Time Password (OTP) for updating your email address is: <b>" + otp + "</b>\n\nPlease enter this code in the app to verify your email address. This code is valid for 5 minutes.\n\nThank you,\nChatBasket"

	_, err = ps.Appwrite.Message.CreateEmail(
		messageId,
		subject,
		content,
		ps.Appwrite.Message.WithCreateEmailTargets([]string{payload.Email}),
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to send email: "+err.Error())
	}

	_, err = ps.Appwrite.Message.Delete(messageId)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to delete email: "+err.Error())
	}

	doc, err := ps.Appwrite.Database.GetDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.TempOtpCollectionID,
		userId,
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query otp data: "+err.Error())
	}
	if doc.Id == userId {
		_, err = ps.Appwrite.Database.DeleteDocument(
			ps.Appwrite.DatabaseID,
			ps.Appwrite.TempOtpCollectionID,
			userId,
		)
		if err != nil {
			return nil, echo.NewHTTPError(401, "Failed to delete existing otp: "+err.Error())
		}
	}
	hashedOtp, err := utils.HashOTP(otp)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to hash OTP: "+err.Error())
	}

	tempOtpPayload := model.TempOtpPayload{
		Email: payload.Email,
		Otp:   hashedOtp,
	}

	_, err = ps.Appwrite.Database.CreateDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.TempOtpCollectionID,
		userId,
		tempOtpPayload,
	)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to save otp in database: "+err.Error())
	}

	return &model.StatusOkay{Status: true, Message: "Otp sent to new email for verification"}, nil
}

func (ps *GlobalService) UpdateEmailVerification(ctx context.Context, payload *model.UpdateEmailVerification, userId string) (*model.StatusOkay, error) {

	// Retrieve the temporary OTP document from the database
	doc, err := ps.Appwrite.Database.GetDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.TempOtpCollectionID,
		userId,
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query otp data: "+err.Error())
	}
	var tempOtp model.TempOtp
	if err := doc.Decode(&tempOtp); err != nil {
		return nil, echo.NewHTTPError(401, "Failed to parse otp data: "+err.Error())
	}

	if tempOtp.Email != payload.Email {
		return nil, echo.NewHTTPError(401, "Email does not match with the sent OTP email")
	}

	match, err := utils.VerifyOTP(payload.Otp, tempOtp.Otp)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to verify OTP: "+err.Error())
	}
	if !match {
		return nil, echo.NewHTTPError(401, "Invalid OTP")
	}

	// check if tempOtp has expired or not time limit is till 5 minutes after created at

	createdAtTime, err := time.Parse(time.RFC3339, tempOtp.CreatedAt)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to parse OTP creation time: "+err.Error())
	}

	expired := utils.IsExpiredOTP(createdAtTime, 5)
	if expired {
		return nil, echo.NewHTTPError(401, "OTP has expired")
	}

	// Update user's email in Appwrite Auth
	_, err = ps.Appwrite.Users.UpdateEmail(userId, payload.Email)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to update user email in Appwrite Auth: "+err.Error())
	}

	// Update user's email verification status in Appwrite Auth

	_, err = ps.Appwrite.Users.UpdateEmailVerification(userId, true)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to update user email verification status: "+err.Error())
	}

	// Update user's email in the user collection document
	_, err = ps.Appwrite.Database.UpdateDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		userId,
		ps.Appwrite.Database.WithUpdateDocumentData(
			map[string]any{
				"email": payload.Email,
			}),
	)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to update user email in database: "+err.Error())
	}

	// Delete the temporary OTP document
	_, err = ps.Appwrite.Database.DeleteDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.TempOtpCollectionID,
		userId,
	)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to delete temporary OTP: "+err.Error())
	}

	return &model.StatusOkay{Status: true, Message: "Email updated successfully"}, nil
}

func (ps *GlobalService) UpdatePassword(ctx context.Context, payload *model.UpdatePassword, userId string) (*model.StatusOkay, error) {
	doc, err := ps.Appwrite.Users.Get(userId)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query user data: "+err.Error())
	}

	match, err := utils.VerifyOTP(payload.OldPassword, doc.Password)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to verify old password: "+err.Error())
	}
	if !match {
		return nil, echo.NewHTTPError(401, "Old password does not match")
	}

	_, err = ps.Appwrite.Users.UpdatePassword(userId, payload.NewPassword)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to update password in Appwrite Auth: "+err.Error())
	}

	return &model.StatusOkay{Status: true, Message: "Password updated successfully"}, nil
}
