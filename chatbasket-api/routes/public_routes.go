package routes

import (
	"chatbasket-api/middleware"
	"chatbasket-api/public/publichandler"
	"chatbasket-api/public/publicservice"
	"chatbasket-api/services"

	"github.com/labstack/echo/v4"
)

// RegisterPublicRoutes registers all public domain routes
func RegisterPublicRoutes(e *echo.Echo, globalService *services.GlobalService) {
	pubSvc := publicservice.New(globalService)

	// Public Profile Routes
	publicProfileGroup := e.Group("/public/profile")
	publicProfileGroup.Use(middleware.AppwriteSessionMiddleware(true))
	publicProfileHandler := publichandler.NewProfileHandler(pubSvc)
	publicProfileGroup.POST("/logout", publicProfileHandler.Logout)
	publicProfileGroup.POST("/check-username", publicProfileHandler.CheckIfUserNameAvailable)
	publicProfileGroup.POST("/create-profile", publicProfileHandler.CreateUserProfile)
	publicProfileGroup.GET("/get-profile", publicProfileHandler.GetProfile)
	publicProfileGroup.POST("/upload-avatar", publicProfileHandler.UploadProfilePicture)
	publicProfileGroup.DELETE("/remove-avatar", publicProfileHandler.RemoveProfilePicture)
	publicProfileGroup.POST("/update-profile", publicProfileHandler.UpdateProfile)

	// Public Settings Routes
	publicSettingGroup := e.Group("/public/settings")
	publicSettingGroup.Use(middleware.AppwriteSessionMiddleware(true))
	publicSettingHandler := publichandler.NewSettingHandler(pubSvc)
	publicSettingGroup.POST("/update-email", publicSettingHandler.UpdateEmail)
	publicSettingGroup.POST("/update-password", publicSettingHandler.UpdatePassword)
	publicSettingGroup.POST("/update-email-verification", publicSettingHandler.UpdateEmailVerification)
	publicSettingGroup.POST("/send-otp", publicSettingHandler.SendOtp)
	publicSettingGroup.POST("/verify-otp", publicSettingHandler.VerifyOtp)
}
