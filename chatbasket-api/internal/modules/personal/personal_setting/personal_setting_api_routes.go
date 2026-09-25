package personal_setting

import (
	"net/http"
	"time"

	rpc_personal_settingv1connect "chatbasket-api/gen/proto/personal/personal_setting/rpc_personal_settingv1connect"
	"chatbasket-api/internal/modules/personal/personal_sse"
	"chatbasket-api/internal/platform/middleware"

	"github.com/labstack/echo/v5"
)

// Register initializes the Setting module dependencies and registers its routes.
func Register(personalGroup *echo.Group, settingService *settingService, personalSseManager *personal_sse.Manager) {
	handler := newSettingHandler(settingService, personalSseManager)

	// Settings calls are small: max 5MB upload, max 30 seconds.
	settings := personalGroup.Group("/settings", middleware.BodyLimit(5242880), middleware.ContextTimeout(30*time.Second))
	settings.POST("/session/central", handler.updateSessionCentral)
	settings.POST("/session/notification-token", handler.updateSessionNotificationToken)

	// Same rules for the RPC version: max 5MB upload, max 30 seconds.
	connectServer := newSettingConnectServer(settingService, personalSseManager)
	path, connectHandler := rpc_personal_settingv1connect.NewSettingServiceHandler(
		connectServer,
	)
	personalGroup.Any(
		path+"*",
		echo.WrapHandler(http.StripPrefix("/api/personal", connectHandler)),
		middleware.BodyLimit(5242880),
		middleware.ContextTimeout(30*time.Second),
	)
}

