package personal_profile

import (
	"net/http"
	"time"

	rpc_personal_profilev1connect "chatbasket-api/gen/proto/personal/personal_profile/rpc_personal_profilev1connect"
	"chatbasket-api/internal/platform/middleware"

	"github.com/labstack/echo/v5"
)

// Register initializes the Profile module dependencies and registers its routes.
func Register(personalGroup *echo.Group, profileService *profileService) {
	handler := newProfileHandler(profileService)

	// Profile calls are small: max 5MB upload, max 30 seconds.
	profile := personalGroup.Group("/profile", middleware.BodyLimit(5242880), middleware.ContextTimeout(30*time.Second))
	profile.GET("/get-profile", handler.GetProfile)
	profile.POST("/create-profile", handler.CreateUserProfile)
	profile.POST("/presign-avatar", handler.PresignAvatar)
	profile.POST("/confirm-avatar", handler.ConfirmAvatar)
	profile.DELETE("/remove-avatar", handler.RemoveProfilePicture)
	profile.POST("/update-profile", handler.UpdateProfile)
	profile.POST("/update-e2ee-key", handler.UploadE2EEPublicKey)
	profile.GET("/get-e2ee-key", handler.GetE2EEPublicKey)

	// Same rules for the RPC version: max 5MB upload, max 30 seconds.
	connectServer := newProfileConnectServer(profileService)
	path, connectHandler := rpc_personal_profilev1connect.NewProfileServiceHandler(
		connectServer,
	)
	personalGroup.Any(
		path+"*",
		echo.WrapHandler(http.StripPrefix("/api/personal", connectHandler)),
		middleware.BodyLimit(5242880),
		middleware.ContextTimeout(30*time.Second),
	)
}
