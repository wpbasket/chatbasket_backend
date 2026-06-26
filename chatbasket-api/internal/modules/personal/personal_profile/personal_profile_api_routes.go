package personal_profile

import (
	"chatbasket-api/internal/platform/middleware"
	"chatbasket-api/internal/platform/websocket"

	"github.com/labstack/echo/v5"
)

// Register initializes the Profile module dependencies and registers its routes.
func Register(personalGroup *echo.Group, profileService *profileService, authProvider middleware.AuthSessionProvider, hub *websocket.WSHub) {
	handler := newProfileHandler(profileService)

	// Apply Auth Middleware to all personal routes
	// We use the authProvider for session verification
	personalGroup.Use(middleware.AuthSessionMiddleware(authProvider, true, hub))

	// Profile Routes
	profile := personalGroup.Group("/profile")
	profile.GET("/get-profile", handler.GetProfile)
	profile.POST("/create-profile", handler.CreateUserProfile)
	profile.POST("/presign-avatar", handler.PresignAvatar)
	profile.POST("/confirm-avatar", handler.ConfirmAvatar)
	profile.DELETE("/remove-avatar", handler.RemoveProfilePicture)
	profile.POST("/update-profile", handler.UpdateProfile)
	profile.POST("/update-e2ee-key", handler.UploadE2EEPublicKey)
	profile.GET("/get-e2ee-key", handler.GetE2EEPublicKey)

}
