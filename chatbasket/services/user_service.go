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
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query email: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	if emailRes.Total == 1 {
		return nil, &model.ApiError{
			Code:    409,
			Message: "Email already registered",
			Type:    "conflict",
		}
	}
	pass := "00" + payload.Password

	hashedPassword, err := utils.HashOTP(pass)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to hash password: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// ✅ Step 2: Create account in Appwrite Auth
	newUuid, err := uuid.NewV7()
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to generate UUID: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	
	userID := id.Custom(newUuid.String())
	_, err = us.Appwrite.Users.CreateArgon2User(
		userID,
		payload.Email,
		hashedPassword,
		us.Appwrite.Users.WithCreateArgon2UserName(payload.Name),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Appwrite account creation failed: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Step 3: Send OTP (CreateEmailToken)
	messageId := id.Custom(uuid.NewString())
	subject := "Otp for email verification"
	otp, err := utils.GenerateOTP()
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to generate OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	content := "<p>Hello,<br>Please enter this code in the app to verify your email address. This code is valid for 3 minutes.Your One-Time Password (OTP) for verifying your email address is:<br><h1>" + otp + "</h1></p><p>Thank you,<br>ChatBasket</p>"

	_, err = us.Appwrite.Message.CreateEmail(
		messageId,
		subject,
		content,
		us.Appwrite.Message.WithCreateEmailUsers([]string{userID}),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to send email: " + err.Error(),
			Type:    "internal_server_error",
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
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query otp data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	if doc.Total == 1 {
		_, err = us.Appwrite.Database.DeleteDocument(
			us.Appwrite.DatabaseID,
			us.Appwrite.TempOtpCollectionID,
			userID,
		)
		if err != nil {
			return nil, &model.ApiError{
				Code:    500,
				Message: "Failed to delete existing otp: " + err.Error(),
				Type:    "internal_server_error",
			}
		}
	}

	hashedOtp, err := utils.HashOTP(otp)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to hash OTP: " + err.Error(),
			Type:    "internal_server_error",
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
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to save otp in database: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// 👤 Step 4: Return success response
	return &model.StatusOkay{Status: true, Message: "OTP sent to email"}, nil
}

func (us *GlobalService) AccountVerification(ctx context.Context, payload *model.AuthVerificationPayload) (*model.SessionResponse, *model.ApiError) {

	// Step1: Verify user
	userRes, err := us.Appwrite.Users.List(
		us.Appwrite.Users.WithListQueries([]string{
			query.Equal("email", payload.Email),
			query.Limit(1),
		}),
	)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to query email: " + err.Error(), Type: "internal_server_error"}
	}
	if userRes.Total == 0 {
		return nil, &model.ApiError{Code: 401, Message: "Email is not registered", Type: "unauthorized"}
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
		return nil, &model.ApiError{Code: 500, Message: "Failed to query otp data: " + err.Error(), Type: "internal_server_error"}
	}
	var tempOtp model.TempOtp
	if err := doc.Decode(&tempOtp); err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to parse otp data: " + err.Error(), Type: "internal_server_error"}
	}

	if tempOtp.Email != payload.Email {
		return nil, &model.ApiError{Code: 401, Message: "Email does not match with the sent OTP email", Type: "unauthorized"}
	}

	match, err := utils.VerifyOTP(payload.Secret, tempOtp.Otp)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to verify OTP: " + err.Error(), Type: "internal_server_error"}
	}
	if !match {
		return nil, &model.ApiError{Code: 401, Message: "Invalid OTP", Type: "unauthorized"}
	}

	// check if tempOtp has expired or not time limit is till 3 minutes after created at

	createdAtTime, err := time.Parse(time.RFC3339, tempOtp.CreatedAt)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to parse OTP creation time: " + err.Error(), Type: "internal_server_error"}
	}

	expired := utils.IsExpiredOTP(createdAtTime, 3)
	if expired {
		return nil, &model.ApiError{Code: 401, Message: "OTP has expired", Type: "unauthorized"}
	}

	// Step3: Verify account using OTP and create session
	session, err := us.Appwrite.Users.CreateSession(userId)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "OTP verification failed: " + err.Error(), Type: "internal_server_error"}
	}
	_, err = us.Appwrite.Users.UpdateEmailVerification(userId, true)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to update email verification status: " + err.Error(), Type: "internal_server_error"}
	}

	_, err = us.Appwrite.Message.Delete(tempOtp.MessageId)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to delete message: " + err.Error(), Type: "internal_server_error"}
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

func (us *GlobalService) Login(ctx context.Context, payload *model.LoginPayload) (*model.StatusOkay, *model.ApiError) {

	// Step1: verify user
	userRes, err := us.Appwrite.Users.List(
		us.Appwrite.Users.WithListQueries([]string{
			query.Equal("email", payload.Email),
			query.Limit(1),
		}),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query email: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if (userRes.Total) == 0 {
		return nil, &model.ApiError{
			Code:    401,
			Message: "Email is not registered",
			Type:    "unauthorized",
		}
	}

	if userRes.Users[0].Email != payload.Email {
		return nil, &model.ApiError{
			Code:    401,
			Message: "Email is not registered",
			Type:    "unauthorized",
		}

	}

	passWord := userRes.Users[0].Password
	payloadPass := "00" + payload.Password
	match, err := utils.VerifyOTP(payloadPass, passWord)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to verify password: " + err.Error(),
			Type:    "internal_server_error",
		}

	}
	if !match {
		return nil, &model.ApiError{
			Code:    401,
			Message: "Invalid credentials",
			Type:    "unauthorized",
		}

	}

	// Step2: Generate otp to create session
	messageId := id.Custom(uuid.NewString())
	subject := "Otp for login verification"
	otp, err := utils.GenerateOTP()
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to generate OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	content := "<p>Hello,<br>Please enter this code in the app to verify your login. This code is valid for 3 minutes.Your One-Time Password (OTP) for login verification is:<br><h1>" + otp + "</h1></p><p>Thank you,<br>ChatBasket</p>"
	userId := userRes.Users[0].Id


	emailTarget,err := us.Appwrite.Users.ListTargets(userId)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to list targets: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	emailT := emailTarget.Targets[0].Id
	// if emailTarget.Total==2{
	// 	emailT=emailTarget.Targets[0].Id
	// }
	  
	_, err = us.Appwrite.Message.CreateEmail(
		messageId,
		subject,
		content,
		us.Appwrite.Message.WithCreateEmailTargets([]string{emailT}),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to send email: " + err.Error(),
			Type:    "internal_server_error",
		}
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
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query otp data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if doc.Total == 1 {
		_, err = us.Appwrite.Database.DeleteDocument(
			us.Appwrite.DatabaseID,
			us.Appwrite.TempOtpCollectionID,
			userId,
		)
		if err != nil {
			return nil, &model.ApiError{
				Code:    500,
				Message: "Failed to delete existing otp: " + err.Error(),
				Type:    "internal_server_error",
			}
		}
	}
	hashedOtp, err := utils.HashOTP(otp)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to hash OTP: " + err.Error(),
			Type:    "internal_server_error",
		}

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
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to save otp in database: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	return &model.StatusOkay{Status: true, Message: "OTP sent to email"}, nil
}

func (us *GlobalService) LoginVerification(ctx context.Context, payload *model.AuthVerificationPayload) (*model.SessionResponse, *model.ApiError) {
	// 🔍 Step 1: Find user by email
	userRes, err := us.Appwrite.Users.List(
		us.Appwrite.Users.WithListQueries([]string{
			query.Equal("email", payload.Email),
			query.Limit(1),
		}),
	)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to query email: " + err.Error(), Type: "internal_server_error"}
	}
	if userRes.Total == 0 {
		return nil, &model.ApiError{Code: 401, Message: "Email is not registered", Type: "unauthorized"}
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
		return nil, &model.ApiError{Code: 500, Message: "Failed to query otp data: " + err.Error(), Type: "internal_server_error"}
	}
	var tempOtp model.TempOtp
	if err := doc.Decode(&tempOtp); err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to parse otp data: " + err.Error(), Type: "internal_server_error"}
	}

	if tempOtp.Email != payload.Email {
		return nil, &model.ApiError{Code: 401, Message: "Email does not match with the sent OTP email", Type: "unauthorized"}
	}

	match, err := utils.VerifyOTP(payload.Secret, tempOtp.Otp)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to verify OTP: " + err.Error(), Type: "internal_server_error"}
	}
	if !match {
		return nil, &model.ApiError{Code: 401, Message: "Invalid OTP", Type: "unauthorized"}
	}

	// check if tempOtp has expired or not time limit is till 3 minutes after created at

	createdAtTime, err := time.Parse(time.RFC3339, tempOtp.CreatedAt)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to parse OTP creation time: " + err.Error(), Type: "internal_server_error"}
	}

	expired := utils.IsExpiredOTP(createdAtTime, 3)
	if expired {
		return nil, &model.ApiError{Code: 401, Message: "OTP has expired", Type: "unauthorized"}
	}

	// 🔑 Step 3:  create session
	session, err := us.Appwrite.Users.CreateSession(userId)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "OTP verification failed: " + err.Error(), Type: "internal_server_error"}
	}

	// if email not verified verfiy it
	if !userRes.Users[0].EmailVerification {
		_, err := us.Appwrite.Users.UpdateEmailVerification(userId, true)
		if err != nil {
			return nil, &model.ApiError{Code: 500, Message: "Failed to update email verification status: " + err.Error(), Type: "internal_server_error"}

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
