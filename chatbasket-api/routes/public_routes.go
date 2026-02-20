package routes

import (
	"chatbasket-api/middleware"
	"chatbasket-api/public/publichandler"
	"chatbasket-api/public/publicservice"
	"chatbasket-api/services"

	"github.com/labstack/echo/v4"
)

// RegisterPublicRoutes registers all public domain routes
// RegisterPublicRoutes registers all public domain routes
func RegisterPublicRoutes(api *echo.Group, globalService *services.GlobalService, authService *services.AuthService, authSecret []byte) {
	pubSvc := publicservice.New(globalService, authSecret)

	// Public Profile Routes
	publicProfileGroup := api.Group("/public/profile")
	publicProfileGroup.Use(middleware.AuthSessionMiddleware(authService, true))
	publicProfileHandler := publichandler.NewProfileHandler(pubSvc)
	publicProfileGroup.POST("/logout", publicProfileHandler.Logout)
	publicProfileGroup.POST("/check-username", publicProfileHandler.CheckIfUserNameAvailable)
	publicProfileGroup.POST("/create-profile", publicProfileHandler.CreateUserProfile)
	publicProfileGroup.GET("/get-profile", publicProfileHandler.GetProfile)
	publicProfileGroup.POST("/upload-avatar", publicProfileHandler.UploadProfilePicture)
	publicProfileGroup.DELETE("/remove-avatar", publicProfileHandler.RemoveProfilePicture)
	publicProfileGroup.POST("/update-profile", publicProfileHandler.UpdateProfile)
}
