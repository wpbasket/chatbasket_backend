package routes

import (
	"chatbasket/appwriteinternal"
	"chatbasket/handler"
	"chatbasket/middleware"
	"chatbasket/personalHandler"
	"chatbasket/personalServices"
	"chatbasket/publicHandler"
	"chatbasket/publicServices"
	"chatbasket/services"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

func RegisterRoutes(
	e *echo.Echo,
	pool *pgxpool.Pool,
	// add more services as needed...
) {

	cfg, err := loadAppwriteConfig()
	if err != nil {
		e.Logger.Fatal("failed to load appwrite config: " + err.Error())
	}

	as := appwriteinternal.NewAppwriteService(
		cfg.Endpoint,
		cfg.ProjectID,
		cfg.ApiKey,
		cfg.DatabaseID,
		cfg.UsersCollectionID,
		cfg.PostsCollectionID,
		cfg.CommentsCollectionID,
		cfg.BlockCollectionID,
		cfg.LikesCollectionID,
		cfg.FollowCollectionID,
		cfg.RefreshTokensCollectionID,
		cfg.FollowRequestsCollectionID,
		cfg.TempOtpCollectionID,
		cfg.FileUserProfilePicBucketID,
		cfg.PersonalUsersCollectionID,
		cfg.PersonalAloneUsernameCollectionID,
		cfg.PersonalDatabaseID,
		cfg.PersonalProfilePicBucketID,
		cfg.PersonalUsernameKey,
	)

	globalService := services.NewGlobalService(as, pool)
	userHandler := handler.NewUserHandler(globalService)
	// public services wrapper (shared between profile and settings)
	pubSvc := publicServices.New(globalService)

	authGroup := e.Group("/auth")
	authGroup.POST("/signup", userHandler.Signup)
	authGroup.POST("/signup-verification", userHandler.AcountVerification)
	authGroup.POST("/login", userHandler.Login)
	authGroup.POST("/login-verification", userHandler.LoginVerification)

	publicProfileGroup := e.Group("/public/profile")
	publicProfileGroup.Use(middleware.AppwriteSessionMiddleware(true))
	publicProfileHandler := publicHandler.NewProfileHandler(pubSvc)
	publicProfileGroup.POST("/logout", publicProfileHandler.Logout)
	publicProfileGroup.POST("/check-username", publicProfileHandler.CheckIfUserNameAvailable)
	publicProfileGroup.POST("/create-profile", publicProfileHandler.CreateUserProfile)
	publicProfileGroup.GET("/get-profile", publicProfileHandler.GetProfile)
	publicProfileGroup.POST("/upload-avatar", publicProfileHandler.UploadProfilePicture)
	publicProfileGroup.DELETE("/remove-avatar", publicProfileHandler.RemoveProfilePicture)
	publicProfileGroup.POST("/update-profile", publicProfileHandler.UpdateProfile)

	publicSettingGroup := e.Group("/public/settings")
	publicSettingGroup.Use(middleware.AppwriteSessionMiddleware(true))
	publicSettingHandler := publicHandler.NewSettingHandler(pubSvc)
	publicSettingGroup.POST("/update-email", publicSettingHandler.UpdateEmail)
	publicSettingGroup.POST("/update-password", publicSettingHandler.UpdatePassword)
	publicSettingGroup.POST("/update-email-verification", publicSettingHandler.UpdateEmailVerification)
	publicSettingGroup.POST("/send-otp", publicSettingHandler.SendOtp)
	publicSettingGroup.POST("/verify-otp", publicSettingHandler.VerifyOtp)

	personalProfileGroup := e.Group("/personal/profile")
	perSvc := personalServices.New(globalService)
	personalProfileGroup.Use(middleware.AppwriteSessionMiddleware(true))
	personalProfileHandler := personalHandler.NewProfileHandler(perSvc)
	personalProfileGroup.GET("/get-profile", personalProfileHandler.GetProfile)
	personalProfileGroup.POST("/create-profile", personalProfileHandler.CreateUserProfile)
	personalProfileGroup.POST("/logout", personalProfileHandler.Logout)
	personalProfileGroup.POST("/upload-avatar", personalProfileHandler.UploadProfilePicture)
	personalProfileGroup.DELETE("/remove-avatar", personalProfileHandler.RemoveProfilePicture)
	personalProfileGroup.POST("/update-profile", personalProfileHandler.UpdateProfile)

	personalContactsGroup := e.Group("/personal/contacts")
	personalContactsGroup.Use(middleware.AppwriteSessionMiddleware(true))
	persContactsHandler := personalHandler.NewContactHandler(perSvc)
	personalContactsGroup.GET("/get", persContactsHandler.GetContacts)
	personalContactsGroup.POST("/check-existence", persContactsHandler.CheckContactExistance)
	personalContactsGroup.POST("/create", persContactsHandler.CreateContact)
	personalContactsGroup.POST("/delete", persContactsHandler.DeleteContact)
	personalContactsGroup.GET("/requests/get", persContactsHandler.GetContactRequests)
	personalContactsGroup.POST("/requests/accept", persContactsHandler.AcceptContactRequest)
	personalContactsGroup.POST("/requests/reject", persContactsHandler.RejectContactRequest)
	personalContactsGroup.POST("/requests/undo", persContactsHandler.UndoContactRequest)
	personalContactsGroup.POST("/update-nickname", persContactsHandler.UpdateContactNickname)
	personalContactsGroup.POST("/remove-nickname", persContactsHandler.RemoveContactNickname)
}
