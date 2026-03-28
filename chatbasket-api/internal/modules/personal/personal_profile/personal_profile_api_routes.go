package personal_profile

import (
	"chatbasket-api/internal/platform/middleware"

	"github.com/labstack/echo/v5"
)

// Register initializes the Profile module dependencies and registers its routes.
func Register(personalGroup *echo.Group, profileService *profileService, authProvider middleware.AuthSessionProvider) {
	handler := newProfileHandler(profileService)
	
	// Apply Auth Middleware to all personal routes
	// We use the authProvider for session verification
	personalGroup.Use(middleware.AuthSessionMiddleware(authProvider, true)) 

	// Profile Routes
	profile := personalGroup.Group("/profile")
	profile.GET("/get-profile", handler.GetProfile)
	profile.POST("/create-profile", handler.CreateUserProfile)
	profile.POST("/upload-avatar", handler.UploadProfilePicture)
	profile.DELETE("/remove-avatar", handler.RemoveProfilePicture)
	profile.POST("/update-profile", handler.UpdateProfile)

}

