package profileapi

import (
	"chatbasket-apinext/internal/modules/personal/profile/profileservice"
	"chatbasket-apinext/internal/platform/middleware"

	"github.com/labstack/echo/v5"
)

// Register initializes the Profile module dependencies and registers its routes.
func Register(personalGroup *echo.Group, profileService *profileservice.ProfileService, authProvider middleware.AuthSessionProvider) {
	handler := NewProfileHandler(profileService)
	
	// Apply Auth Middleware to all personal routes
	// We use the authProvider for session verification
	personalGroup.Use(middleware.AuthSessionMiddleware(authProvider, true)) 

	// Profile Routes
	profile := personalGroup.Group("/profile")
	profile.POST("/create", handler.CreateUserProfile)
	profile.GET("", handler.GetProfile)
	profile.POST("/update", handler.UpdateProfile)
	profile.POST("/picture/upload", handler.UploadProfilePicture)
	profile.POST("/picture/remove", handler.RemoveProfilePicture)

}
