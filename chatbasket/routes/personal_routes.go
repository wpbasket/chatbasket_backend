package routes

import (
	"chatbasket/middleware"
	"chatbasket/personal/personalhandler"
	"chatbasket/personal/personalservice"
	"chatbasket/services"

	"github.com/labstack/echo/v4"
)

// RegisterPersonalRoutes registers all personal domain routes
func RegisterPersonalRoutes(e *echo.Echo, globalService *services.GlobalService) {
	perSvc := personalservice.New(globalService)

	// Personal Profile Routes
	personalProfileGroup := e.Group("/personal/profile")
	personalProfileGroup.Use(middleware.AppwriteSessionMiddleware(true))
	personalProfileHandler := personalhandler.NewProfileHandler(perSvc)
	personalProfileGroup.GET("/get-profile", personalProfileHandler.GetProfile)
	personalProfileGroup.POST("/create-profile", personalProfileHandler.CreateUserProfile)
	personalProfileGroup.POST("/logout", personalProfileHandler.Logout)
	personalProfileGroup.POST("/upload-avatar", personalProfileHandler.UploadProfilePicture)
	personalProfileGroup.DELETE("/remove-avatar", personalProfileHandler.RemoveProfilePicture)
	personalProfileGroup.POST("/update-profile", personalProfileHandler.UpdateProfile)
	personalProfileGroup.POST("/token/register", personalProfileHandler.RegisterOrUpdateToken)

	// Personal Contacts Routes
	personalContactsGroup := e.Group("/personal/contacts")
	personalContactsGroup.Use(middleware.AppwriteSessionMiddleware(true))
	persContactsHandler := personalhandler.NewContactHandler(perSvc)
	personalContactsGroup.GET("/get", persContactsHandler.GetContacts)
	personalContactsGroup.POST("/check-existence", persContactsHandler.CheckContactExistance)
	personalContactsGroup.POST("/create", persContactsHandler.CreateContact)
	personalContactsGroup.POST("/delete", persContactsHandler.DeleteContact)
	personalContactsGroup.GET("/requests/get", persContactsHandler.GetContactRequests)
	personalContactsGroup.POST("/requests/accept", persContactsHandler.AcceptContactRequest)
	personalContactsGroup.POST("/requests/reject", persContactsHandler.RejectContactRequest)
	personalContactsGroup.POST("/requests/undo", persContactsHandler.UndoContactRequest)
	personalContactsGroup.POST("/update-nickname", persContactsHandler.UpdateContactNickname)
	personalContactsGroup.POST("/remove-nickname", persContactsHandler.RemoveContactNickname)
}
