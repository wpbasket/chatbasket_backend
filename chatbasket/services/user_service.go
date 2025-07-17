package services

import (
	// "chatbasket/appwriteinternal"
	"chatbasket/model"
	"context"

	"github.com/alexedwards/argon2id"
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

func (us *GlobalService) Signup(ctx context.Context, payload *model.SignupPayload) (*model.StatusOkay, error) {
	// 🔍 Step 1: Check if email already exists
	emailRes, err := us.Appwrite.Users.List(
		us.Appwrite.Users.WithListQueries([]string{
			query.Equal("email", payload.Email),
			query.Limit(1),
		}),
	)

	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to query email: "+err.Error())
	}
	if emailRes.Total == 1 {
		return nil, echo.NewHTTPError(409, "Email already registered")
	}

	// ✅ Step 2: Create account in Appwrite Auth
	userID := id.Custom(uuid.NewString())
	_, err = us.Appwrite.Account.Create(
		userID,
		payload.Email,
		payload.Password,
		us.Appwrite.Account.WithCreateName(payload.Name),
	)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Appwrite account creation failed: "+err.Error())
	}

	// Step 3: Send OTP (CreateEmailToken)
	_, err = us.Appwrite.Account.CreateEmailToken(userID, payload.Email)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to send OTP to email: "+err.Error())
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

	// Step2: Verify account using OTP and create session
	session, err := us.Appwrite.Account.CreateSession(userId, payload.Secret)
	if err != nil {
		return nil, echo.NewHTTPError(401, "OTP verification failed: "+err.Error())
	}

	sessionId:= session.Id
	resUserid:= userId
	sessionExpiry:= session.Expire

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

	match, err := argon2id.ComparePasswordAndHash(payload.Password, userRes.Users[0].Password)
	if err != nil {
		return nil, echo.NewHTTPError(500, err.Error())
	}
	if !match {
		return nil, echo.NewHTTPError(401, "Invalid password")
	}

	// Step2: Generate otp to create session
	_, err = us.Appwrite.Account.CreateEmailToken(userRes.Users[0].Id, payload.Email)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to send OTP to email"+err.Error())
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

	// 🔑 Step 2: Verify OTP and create session
	session, err := us.Appwrite.Account.CreateSession(userId, payload.Secret)
	if err != nil {
		return nil, echo.NewHTTPError(401, "OTP verification failed"+err.Error())
	}

	sessionId:= session.Id
	resUserid:= userId
	sessionExpiry:= session.Expire


	return &model.SessionResponse{
		UserId:        resUserid,
		Name:          userName,
		Email:         userEmail,
		SessionID:     sessionId,
		SessionExpiry: sessionExpiry,
	}, nil
}
