package personal_setting

import (
	"net/http"

	rpc_personal_settingv1connect "chatbasket-api/gen/proto/personal/personal_setting/rpc_personal_settingv1connect"
	"chatbasket-api/internal/modules/personal/personal_sse"

	"github.com/labstack/echo/v5"
)

// Register initializes the Setting module dependencies and registers its routes.
func Register(personalGroup *echo.Group, settingService *settingService, personalSseManager *personal_sse.Manager) {
	handler := newSettingHandler(settingService, personalSseManager)

	// Settings HTTP Routes
	settings := personalGroup.Group("/settings")
	settings.POST("/session/central", handler.updateSessionCentral)
	settings.POST("/session/notification-token", handler.updateSessionNotificationToken)

	// Connect RPC Routes
	connectServer := newSettingConnectServer(settingService, personalSseManager)
	path, connectHandler := rpc_personal_settingv1connect.NewSettingServiceHandler(
		connectServer,
	)
	personalGroup.Any(path+"*", echo.WrapHandler(http.StripPrefix("/api/personal", connectHandler)))
}

