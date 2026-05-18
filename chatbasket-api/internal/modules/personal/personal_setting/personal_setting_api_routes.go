package personal_setting

import (
	"chatbasket-api/internal/platform/websocket"

	"github.com/labstack/echo/v5"
)

// Register initializes the Setting module dependencies and registers its routes.
func Register(personalGroup *echo.Group, settingService *settingService, hub *websocket.WSHub) {
	handler := newSettingHandler(settingService, hub)

	// Settings Routes
	settings := personalGroup.Group("/settings")
	settings.POST("/session/central", handler.updateSessionCentral)
	settings.POST("/session/notification-token", handler.updateSessionNotificationToken)
}
