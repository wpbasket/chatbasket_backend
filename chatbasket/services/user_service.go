package services

import (
	"context"
	"encoding/json"

	"github.com/appwrite/sdk-for-go/id"
	"github.com/appwrite/sdk-for-go/permission"
	"github.com/appwrite/sdk-for-go/query"
	"github.com/appwrite/sdk-for-go/role"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"chatbasket/appwrite"
	"chatbasket/model"
)

type UserService struct {
	Appwrite *appwrite.AppwriteService
}

func NewUserService(app *appwrite.AppwriteService) *UserService {
	return &UserService{Appwrite: app}
}

func (us *UserService) Signup(ctx context.Context, payload *model.SignupPayload) (*model.SignupIntialResponse, error) {
	// 🔍 Step 1: Check if email already exists
	emailRes, err := us.Appwrite.Database.ListDocuments(
		us.Appwrite.DatabaseID,
		us.Appwrite.UsersCollectionID,
		us.Appwrite.Database.WithListDocumentsQueries([]string{
			query.Equal("email", payload.Email),
			query.Limit(1),
		}),
	)

	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to query email")
	}
	if len(emailRes.Documents) > 0 {
		return nil, echo.NewHTTPError(409, "Email already in use")
	}

	// ✅ Step 2: Create account in Appwrite Auth
	userID := id.Custom(uuid.NewString())
	_, err = us.Appwrite.Account.Create(
		userID,
		payload.Email,
		payload.Password,
	)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Appwrite account creation failed")
	}

	// Step 3: Send OTP (CreateEmailToken)
	_, err = us.Appwrite.Account.CreateEmailToken(userID, payload.Email)
	if err != nil {
		return nil, echo.NewHTTPError(500, "Failed to send OTP to email")
	}

	// 👤 Step 4: Return success response
	return &model.SignupIntialResponse{Status: "success"}, nil
}

func (us *UserService) AccountVerification(ctx context.Context, payload *model.AccountVerificationPayload) (*model.SessionResponse, error) {

	// Step1: Verify user
	userRes, err := us.Appwrite.Users.List(
		us.Appwrite.Users.WithListQueries([]string{
			query.Equal("email", payload.Email),
			query.Limit(1),
		}),
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query email")
	}
	if userRes.Total == 0 {
		return nil, echo.NewHTTPError(401, "Email is not registered")
	}
	userID := userRes.Users[0].Id

	// Step2: Verify otp
	session, err := us.Appwrite.Account.CreateSession(userID, payload.Secret)
	if err != nil {
		return nil, echo.NewHTTPError(401, "OTP verification failed")
	}

	// Step 2: Verify account
	_, err = us.Appwrite.Users.UpdateEmailVerification(userID, true)
	if err != nil {
		return nil, echo.NewHTTPError(401, "OTP verification failed")
	}

	// 🗃️ Step 4: Store additional user metadata in database
	dbPayload := model.AppwriteUserPayload{
		Username:         "",
		Name:             payload.Name,
		Email:            payload.Email,
		Bio:              "",
		Avatar:           "",
		Followers:        0,
		Following:        0,
		ProfileVisibleTo: "public",
		IsAdminBlocked:   false,
		AdminBlockReason: "",
	}

	doc, errr := us.Appwrite.Database.CreateDocument(
		us.Appwrite.DatabaseID,
		us.Appwrite.UsersCollectionID,
		userID,
		dbPayload,
		us.Appwrite.Database.WithCreateDocumentPermissions([]string{
			permission.Read(role.Any()),
			permission.Update(role.User(userID,"true")),
			permission.Delete(role.User(userID,"true")),
		}),
	)
	if errr != nil {
		return nil, echo.NewHTTPError(500, "Failed to save user in database")
	}

	// 📦 Convert to internal user model
	var user model.User
	userJSON, _ := json.Marshal(doc)
	if err := json.Unmarshal(userJSON, &user); err != nil {
		return nil, echo.NewHTTPError(500, "Failed to parse user data")
	}

	// ✅ Step 4: Convert to private user and return session response
	privateUser := model.ToPrivateUser(&user)

	return &model.SessionResponse{
		User:          privateUser,
		SessionID:     session.Id,
		SessionExpiry: session.Expire,
	}, nil
}

func (us *UserService) Login(ctx context.Context, payload *model.LoginPayload) (*model.LoginIntialResponse, error) {

	// Step1: verify user
	userRes, err := us.Appwrite.Users.List(
		us.Appwrite.Users.WithListQueries([]string{
			query.Equal("email", payload.Email),
			query.Limit(1),
		}),
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query email")
	}
	if (userRes.Total) == 0 {
		return nil, echo.NewHTTPError(401, "Email is not registered")
	}
	if userRes.Users[0].Password != payload.Password {
		return nil, echo.NewHTTPError(401, "Invalid password")
	}

	// Step2: Generate otp to create session
	_, err = us.Appwrite.Account.CreateEmailToken(userRes.Users[0].Id, payload.Email)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to send OTP to email")
	}

	return &model.LoginIntialResponse{Status: "success"}, nil
}

func (us *UserService) LoginVerification(ctx context.Context, payload *model.LoginVerificationPayload) (*model.SessionResponse, error) {
	// 🔍 Step 1: Find user by email
	userRes, err := us.Appwrite.Users.List(
		us.Appwrite.Users.WithListQueries([]string{
			query.Equal("email", payload.Email),
			query.Limit(1),
		}),
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query email")
	}
	if userRes.Total == 0 {
		return nil, echo.NewHTTPError(401, "Email is not registered")
	}
	userId := userRes.Users[0].Id

	// 🔑 Step 2: Verify OTP and create session
	session, err := us.Appwrite.Account.CreateSession(userId, payload.Secret)
	if err != nil {
		return nil, echo.NewHTTPError(401, "OTP verification failed")
	}

	// 🧾 Step 3: Get user profile from database
	doc, err := us.Appwrite.Database.GetDocument(
		us.Appwrite.DatabaseID,
		us.Appwrite.UsersCollectionID,
		userId,
	)
	if err != nil {
		return nil, echo.NewHTTPError(401, "Failed to query user data")
	}

	// 📦 Convert to internal user model
	var user model.User
	userJSON, _ := json.Marshal(doc)
	if err := json.Unmarshal(userJSON, &user); err != nil {
		return nil, echo.NewHTTPError(500, "Failed to parse user data")
	}

	// ✅ Step 4: Convert to private user and return session response
	privateUser := model.ToPrivateUser(&user)

	return &model.SessionResponse{
		User:          privateUser,
		SessionID:     session.Id,
		SessionExpiry: session.Expire,
	}, nil
}
