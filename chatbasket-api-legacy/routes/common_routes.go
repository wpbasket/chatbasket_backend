package routes

import (
	"chatbasket-api/common/commonhandler"
	"chatbasket-api/common/commonservice"
	"chatbasket-api/middleware"
	"chatbasket-api/services"

	"github.com/labstack/echo/v5"
)

// RegisterCommonRoutes registers all common authenticated routes
// These routes are shared between public and personal modes
func RegisterCommonRoutes(api *echo.Group, globalService *services.GlobalService, authService *services.AuthService, authSecret []byte) {
	commonSvc := commonservice.New(globalService, authSecret)

	// Common Settings Routes (works for both public and personal modes)
	commonSettingsGroup := api.Group("/common/settings")
	commonSettingsGroup.Use(middleware.AuthSessionMiddleware(authService, true))
	commonSettingsHandler := commonhandler.NewSettingHandler(commonSvc)
	commonSettingsGroup.POST("/update/request", commonSettingsHandler.RequestUpdateOTP)
	commonSettingsGroup.POST("/password/confirm", commonSettingsHandler.ConfirmPasswordUpdate)
	commonSettingsGroup.POST("/email/request", commonSettingsHandler.RequestEmailUpdate)
	commonSettingsGroup.POST("/email/confirm", commonSettingsHandler.ConfirmEmailUpdate)

	// Common Auth Routes (logout works for both modes)
	commonAuthGroup := api.Group("/common")
	commonAuthGroup.Use(middleware.AuthSessionMiddleware(authService, true))
	commonAuthHandler := commonhandler.NewAuthHandler(commonSvc)
	commonAuthGroup.POST("/logout", commonAuthHandler.Logout)
	commonAuthGroup.GET("/me", commonAuthHandler.GetUser)
}
