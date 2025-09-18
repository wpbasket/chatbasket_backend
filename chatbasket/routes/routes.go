package routes

import (
	"chatbasket/appwriteinternal"
	"chatbasket/handler"
	"chatbasket/middleware"
	"chatbasket/publicHandler"
	"chatbasket/publicServices"
	"chatbasket/services"
	"os"

	"github.com/labstack/echo/v4"
)

func RegisterRoutes(
	e *echo.Echo,
	// add more services as needed...
) {

	as := appwriteinternal.NewAppwriteService(
		os.Getenv("APPWRITE_ENDPOINT"),
		os.Getenv("APPWRITE_PROJECT_ID"),
		os.Getenv("APPWRITE_API_KEY"),
		os.Getenv("APPWRITE_DATABASE_ID"),
		os.Getenv("APPWRITE_USERS_COLLECTION_ID"),
		os.Getenv("APPWRITE_POSTS_COLLECTION_ID"),
		os.Getenv("APPWRITE_COMMENTS_COLLECTION_ID"),
		os.Getenv("APPWRITE_BLOCK_COLLECTION_ID"),
		os.Getenv("APPWRITE_LIKES_COLLECTION_ID"),
		os.Getenv("APPWRITE_FOLLOW_COLLECTION_ID"),
		os.Getenv("APPWRITE_REFRESH_TOKENS_COLLECTION_ID"),		
		os.Getenv("APPWRITE_FOLLOW_REQUESTS_COLLECTION_ID"),
		os.Getenv("APPWRITE_TEMP_OTP_COLLECTION_ID"),
		os.Getenv("APPWRITE_FILE_USERPROFILEPIC_BUCKET_ID"),	
	)
	
	globalService := services.NewGlobalService(as)
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
	publicSettingGroup.POST("/send-otp",publicSettingHandler.SendOtp)
	publicSettingGroup.POST("/verify-otp",publicSettingHandler.VerifyOtp)

}
