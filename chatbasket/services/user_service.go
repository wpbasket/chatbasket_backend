package services

import (
	// "chatbasket/appwriteinternal"
	"chatbasket/model"
	"chatbasket/utils"
	"context"
	"log"
	"time"

	"github.com/appwrite/sdk-for-go/id"
	"github.com/appwrite/sdk-for-go/query"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// type UserService struct {
// 	Appwrite *appwriteinternal.AppwriteService
// }

// func NewUserService(app *appwriteinternal.AppwriteService) *UserService {
// 	return &UserService{Appwrite: app}
// }

func (us *GlobalService) Signup(ctx context.Context, payload *model.SignupPayload) (*model.StatusOkay, *model.ApiError) {
	// 🔍 Step 1: Check if email already exists
	emailRes, err := us.Appwrite.Users.List(
		us.Appwrite.Users.WithListQueries([]string{
			query.Equal("email", payload.Email),
			query.Limit(1),
		}),
	)

	if err != nil {
		return nil,&model.ApiError{
			Code:500,
			Message: "Failed to query email: "+err.Error(),
			Type: "internal_server_error",    
		}
	}

	if emailRes.Total == 1 {
		return nil,&model.ApiError{
			Code:409,
			Message: "Email already registered",
			Type: "conflict",    
		}
	}

	hashedPassword, err := utils.HashOTP(payload.Password)
	if err != nil {
		return nil,&model.ApiError{
			Code:500,
			Message: "Failed to hash password: "+err.Error(),
			Type: "internal_server_error",    
		}
	}

	// ✅ Step 2: Create account in Appwrite Auth
	userID := id.Custom(uuid.NewString())
	_, err = us.Appwrite.Users.CreateArgon2User(
		userID,
		payload.Email,
		hashedPassword,
		us.Appwrite.Users.WithCreateArgon2UserName(payload.Name),
	)
	if err != nil {
		return nil,&model.ApiError{
			Code:500,
			Message: "Appwrite account creation failed: "+err.Error(),
			Type: "internal_server_error",    
		}
	}

	// Step 3: Send OTP (CreateEmailToken)
	messageId := id.Custom(uuid.NewString())
	subject := "Otp for email verification"
	otp, err := utils.GenerateOTP()
	if err != nil {
		return nil,&model.ApiError{
			Code:500,
			Message: "Failed to generate OTP: "+err.Error(),
			Type: "internal_server_error",    
		}
	}

	content := "Hello,\n\nYour One-Time Password (OTP) for verifying your email address is: <b>" + otp + "</b>\n\nPlease enter this code in the app to verify your email address. This code is valid for 3 minutes.\n\nThank you,\nChatBasket"

	_, err = us.Appwrite.Message.CreateEmail(
		messageId,
		subject,
		content,
		us.Appwrite.Message.WithCreateEmailUsers([]string{userID}),
	)
	if err != nil {
		return nil,&model.ApiError{
			Code:500,
			Message: "Failed to send email: "+err.Error(),
			Type: "internal_server_error",    
		}
	}

	doc, err := us.Appwrite.Database.ListDocuments(
		us.Appwrite.DatabaseID,
		us.Appwrite.TempOtpCollectionID,
		us.Appwrite.Database.WithListDocumentsQueries(
			[]string{
				query.Equal("userId", userID),
				query.Limit(1),
			},
		),
	)

	if err != nil {
		return nil,&model.ApiError{
			Code:500,
			Message: "Failed to query otp data: "+err.Error(),
			Type: "internal_server_error",    
		}
	}

	if doc.Total == 1 {
		_, err = us.Appwrite.Database.DeleteDocument(
			us.Appwrite.DatabaseID,
			us.Appwrite.TempOtpCollectionID,
			userID,
		)
		if err != nil {
			return nil,&model.ApiError{
				Code:500,
				Message: "Failed to delete existing otp: "+err.Error(),
				Type: "internal_server_error",    
			}
		}
	}

	hashedOtp, err := utils.HashOTP(otp)
	if err != nil {
		return nil,&model.ApiError{
			Code:500,
			Message: "Failed to hash OTP: "+err.Error(),
			Type: "internal_server_error",    
		}
	}

	tempOtpPayload := model.TempOtpPayload{
		Email:     payload.Email,
		Otp:       hashedOtp,
		UserId:    userID,
		MessageId: messageId,
	}

	_, err = us.Appwrite.Database.CreateDocument(
		us.Appwrite.DatabaseID,
		us.Appwrite.TempOtpCollectionID,
		userID,
		tempOtpPayload,
	)
	if err != nil {
		return nil,&model.ApiError{
			Code:500,
			Message: "Failed to save otp in database: "+err.Error(),
			Type: "internal_server_error",    
		}
	}

	// 👤 Step 4: Return success response
	return &model.StatusOkay{Status: true, Message: "OTP sent to email"}, nil
}






func (us *GlobalService) AccountVerification(ctx context.Context, payload *model.AuthVerificationPayload) (*model.SessionResponse, error) {

	// Step1: Verify user
	userRes, err := us.Appwrite.Users.List(
		us.Appwrite.Users.WithListQueries([]string{
			query.Equal("email", payload.Email),
			query.Limit(1),
		}),
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query email: "+err.Error())
	}
	if userRes.Total == 0 {
		return nil, echo.NewHTTPError(401, "Email is not registered")
	}
	userId := userRes.Users[0].Id
	userName := userRes.Users[0].Name
	userEmail := userRes.Users[0].Email

	// Step2: verify otp

	// Retrieve the temporary OTP document from the database
	doc, err := us.Appwrite.Database.GetDocument(
		us.Appwrite.DatabaseID,
		us.Appwrite.TempOtpCollectionID,
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

	match, err := utils.VerifyOTP(payload.Secret, tempOtp.Otp)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to verify OTP: "+err.Error())
	}
	if !match {
		return nil, echo.NewHTTPError(401, "Invalid OTP")
	}

	// check if tempOtp has expired or not time limit is till 3 minutes after created at

	createdAtTime, err := time.Parse(time.RFC3339, tempOtp.CreatedAt)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to parse OTP creation time: "+err.Error())
	}

	expired := utils.IsExpiredOTP(createdAtTime, 3)
	if expired {
		return nil, echo.NewHTTPError(401, "OTP has expired")
	}

	// Step3: Verify account using OTP and create session
	session, err := us.Appwrite.Users.CreateSession(userId)
	if err != nil {
		return nil, echo.NewHTTPError(401, "OTP verification failed: "+err.Error())
	}
	_, err = us.Appwrite.Users.UpdateEmailVerification(userId, true)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to update email verification status: "+err.Error())
	}

	_, err = us.Appwrite.Message.Delete(tempOtp.MessageId)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to delete message: "+err.Error())
	}

	_, err = us.Appwrite.Database.DeleteDocument(
		us.Appwrite.DatabaseID,
		us.Appwrite.TempOtpCollectionID,
		userId,
	)
	if err != nil {
		log.Printf("Failed to delete otp: %v", err.Error())
	}

	sessionId := session.Id
	resUserid := userId
	sessionExpiry := session.Expire

	return &model.SessionResponse{
		UserId:        resUserid,
		Name:          userName,
		Email:         userEmail,
		SessionID:     sessionId,
		SessionExpiry: sessionExpiry,
	}, nil
}







func (us *GlobalService) Login(ctx context.Context, payload *model.LoginPayload) (*model.StatusOkay, error) {

	// Step1: verify user
	userRes, err := us.Appwrite.Users.List(
		us.Appwrite.Users.WithListQueries([]string{
			query.Equal("email", payload.Email),
			query.Limit(1),
		}),
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query email: "+err.Error())
	}
	if (userRes.Total) == 0 {
		return nil, echo.NewHTTPError(401, "Email is not registered")
	}

	if userRes.Users[0].Email != payload.Email {
		return nil, echo.NewHTTPError(401, "Email does not match")
	}

	passWord := userRes.Users[0].Password

	match, err := utils.VerifyOTP(payload.Password, passWord)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to verify password: "+err.Error())
	}
	if !match {
		return nil, echo.NewHTTPError(401, "Invalid password")
	}

	// Step2: Generate otp to create session
	messageId := id.Custom(uuid.NewString())
	subject := "Otp for login verification"
	otp, err := utils.GenerateOTP()
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to generate OTP: "+err.Error())
	}
	content := "Hello,\n\nYour One-Time Password (OTP) for login verification is: <b>" + otp + "</b>\n\nPlease enter this code in the app to verify your login. This code is valid for 3 minutes.\n\nThank you,\nChatBasket"
	userId := userRes.Users[0].Id

	_, err = us.Appwrite.Message.CreateEmail(
		messageId,
		subject,
		content,
		us.Appwrite.Message.WithCreateEmailUsers([]string{userId}),
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to send email: "+err.Error())
	}

	doc, err := us.Appwrite.Database.ListDocuments(
		us.Appwrite.DatabaseID,
		us.Appwrite.TempOtpCollectionID,
		us.Appwrite.Database.WithListDocumentsQueries(
			[]string{
				query.Equal("userId", userId),
				query.Limit(1),
			},
		),
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query otp data: "+err.Error())
	}
	if doc.Total == 1 {
		_, err = us.Appwrite.Database.DeleteDocument(
			us.Appwrite.DatabaseID,
			us.Appwrite.TempOtpCollectionID,
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
		Email:     payload.Email,
		Otp:       hashedOtp,
		UserId:    userId,
		MessageId: messageId,
	}

	_, err = us.Appwrite.Database.CreateDocument(
		us.Appwrite.DatabaseID,
		us.Appwrite.TempOtpCollectionID,
		userId,
		tempOtpPayload,
	)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to save otp in database: "+err.Error())
	}

	return &model.StatusOkay{Status: true, Message: "OTP sent to email"}, nil
}





func (us *GlobalService) LoginVerification(ctx context.Context, payload *model.AuthVerificationPayload) (*model.SessionResponse, error) {
	// 🔍 Step 1: Find user by email
	userRes, err := us.Appwrite.Users.List(
		us.Appwrite.Users.WithListQueries([]string{
			query.Equal("email", payload.Email),
			query.Limit(1),
		}),
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query email: "+err.Error())
	}
	if userRes.Total == 0 {
		return nil, echo.NewHTTPError(401, "Email is not registered")
	}
	userId := userRes.Users[0].Id
	userName := userRes.Users[0].Name
	userEmail := userRes.Users[0].Email

	// 🔑 Step 2: Verify OTP
	// Retrieve the temporary OTP document from the database
	doc, err := us.Appwrite.Database.GetDocument(
		us.Appwrite.DatabaseID,
		us.Appwrite.TempOtpCollectionID,
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

	match, err := utils.VerifyOTP(payload.Secret, tempOtp.Otp)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to verify OTP: "+err.Error())
	}
	if !match {
		return nil, echo.NewHTTPError(401, "Invalid OTP")
	}

	// check if tempOtp has expired or not time limit is till 3 minutes after created at

	createdAtTime, err := time.Parse(time.RFC3339, tempOtp.CreatedAt)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to parse OTP creation time: "+err.Error())
	}

	expired := utils.IsExpiredOTP(createdAtTime, 3)
	if expired {
		return nil, echo.NewHTTPError(401, "OTP has expired")
	}

	// 🔑 Step 3:  create session
	session, err := us.Appwrite.Users.CreateSession(userId)
	if err != nil {
		return nil, echo.NewHTTPError(401, "OTP verification failed"+err.Error())
	}

	// if email not verified verfiy it
	if !userRes.Users[0].EmailVerification {
		_, err := us.Appwrite.Users.UpdateEmailVerification(userId, true)
		if err != nil {
			return nil, echo.NewHTTPError(500, "Failed to update email verification status: "+err.Error())

		}
	}

	// delete message but even it fails continue dont return nil
	_, err = us.Appwrite.Message.Delete(tempOtp.MessageId)
	if err != nil {
		log.Printf("could not delete message: %v", err.Error())
	}

	_, err = us.Appwrite.Database.DeleteDocument(
		us.Appwrite.DatabaseID,
		us.Appwrite.TempOtpCollectionID,
		userId,
	)
	if err != nil {
		log.Printf("Failed to delete otp: %v", err.Error())
	}

	sessionId := session.Id
	resUserid := userId
	sessionExpiry := session.Expire

	return &model.SessionResponse{
		UserId:        resUserid,
		Name:          userName,
		Email:         userEmail,
		SessionID:     sessionId,
		SessionExpiry: sessionExpiry,
	}, nil
}
