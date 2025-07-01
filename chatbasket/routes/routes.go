package routes

import (
	"github.com/labstack/echo/v4"

	"chatbasket/handler"
	"chatbasket/services"
)

func RegisterRoutes(
	e *echo.Echo,
	userService *services.UserService,
	// add more services as needed...
) {
	// Initialize handlers
	userHandler := handler.NewUserHandler(userService)

	// ✅ User routes
	e.POST("/signup", userHandler.Signup)
	e.POST("/account-verification", userHandler.AcountVerification)
	e.POST("/login", userHandler.Login)
	e.POST("/login-verification", userHandler.LoginVerification)
	

}
