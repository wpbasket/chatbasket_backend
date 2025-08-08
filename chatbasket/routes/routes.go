package routes

import (
	"chatbasket/appwriteinternal"
	"chatbasket/handler"
	"chatbasket/middleware"
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

	authGroup := e.Group("/auth")
	authGroup.POST("/signup", userHandler.Signup)
	authGroup.POST("/signup-verification", userHandler.AcountVerification)
	authGroup.POST("/login", userHandler.Login)
	authGroup.POST("/login-verification", userHandler.LoginVerification)

	profileGroup := e.Group("/profile")
	profileGroup.Use(middleware.AppwriteSessionMiddleware(true))
	profileHandler := handler.NewProfileHandler(globalService)
	profileGroup.POST("/logout", profileHandler.Logout)
	profileGroup.POST("/check-username", profileHandler.CheckIfUserNameAvailable)
	profileGroup.POST("/create-profile", profileHandler.CreateUserProfile)
	profileGroup.GET("/get-profile", profileHandler.GetProfile)
	profileGroup.POST("/upload-avatar", profileHandler.UploadProfilePicture)
	profileGroup.POST("/update-profile", profileHandler.UpdateProfile)
	

	settingGroup := e.Group("/settings")
	settingGroup.Use(middleware.AppwriteSessionMiddleware(true))
	settingHandler := handler.NewSettingHandler(globalService)
	settingGroup.POST("/update-email", settingHandler.UpdateEmail)
	settingGroup.POST("/update-password", settingHandler.UpdatePassword)
	profileGroup.POST("/update-email-verification", settingHandler.UpdateEmailVerification)

}
