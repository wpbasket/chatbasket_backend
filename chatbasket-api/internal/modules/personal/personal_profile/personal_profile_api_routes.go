package personal_profile

import (
	"chatbasket-api/internal/platform/middleware"
	"chatbasket-api/internal/platform/websocket"

	rpc_personal_profilev1connect "chatbasket-api/gen/proto/personal/personal_profile/rpc_personal_profilev1connect"

	"github.com/labstack/echo/v5"
	"net/http"
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

	// Connect RPC Routes (inherits personalGroup's middleware for REST fallback)
	connectServer := newProfileConnectServer(profileService)
	path, connectHandler := rpc_personal_profilev1connect.NewProfileServiceHandler(
		connectServer,
	)
	personalGroup.Any(path+"*", echo.WrapHandler(http.StripPrefix("/api/personal", connectHandler)))
}
