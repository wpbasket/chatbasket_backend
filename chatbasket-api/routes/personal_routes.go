package routes

import (
	"chatbasket-api/middleware"
	"chatbasket-api/personal/personalhandler"
	"chatbasket-api/personal/personalservice"
	"chatbasket-api/services"

	"github.com/labstack/echo/v4"
)

// RegisterPersonalRoutes registers all personal domain routes
func RegisterPersonalRoutes(e *echo.Echo, globalService *services.GlobalService, authService *services.AuthService, authSecret []byte) {
	perSvc := personalservice.New(globalService, authSecret)

	// Personal Profile Routes
	personalProfileGroup := e.Group("/personal/profile")
	personalProfileGroup.Use(middleware.AuthSessionMiddleware(authService, true))
	personalProfileHandler := personalhandler.NewProfileHandler(perSvc)
	personalProfileGroup.GET("/get-profile", personalProfileHandler.GetProfile)
	personalProfileGroup.POST("/create-profile", personalProfileHandler.CreateUserProfile)
	personalProfileGroup.POST("/upload-avatar", personalProfileHandler.UploadProfilePicture)
	personalProfileGroup.DELETE("/remove-avatar", personalProfileHandler.RemoveProfilePicture)
	personalProfileGroup.POST("/update-profile", personalProfileHandler.UpdateProfile)

	// Personal Contacts Routes
	personalContactsGroup := e.Group("/personal/contacts")
	personalContactsGroup.Use(middleware.AuthSessionMiddleware(authService, true))
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
	personalContactsGroup.POST("/block", persContactsHandler.BlockUser)
	personalContactsGroup.POST("/remove-nickname", persContactsHandler.RemoveContactNickname)
	// Personal Settings Routes
	persSettingGroup := e.Group("/personal/settings")
	persSettingGroup.Use(middleware.AuthSessionMiddleware(authService, true))
	persSettingHandler := personalhandler.NewSettingHandler(perSvc)
	persSettingGroup.POST("/session/central", persSettingHandler.UpdateSessionCentral)
	persSettingGroup.POST("/session/notification-token", persSettingHandler.UpdateSessionNotificationToken)

	// Personal Chat Routes
	personalChatGroup := e.Group("/personal/chat")
	personalChatGroup.Use(middleware.AuthSessionMiddleware(authService, true))
	persChatHandler := personalhandler.NewChatHandler(perSvc)
	personalChatGroup.POST("/check-eligibility", persChatHandler.CheckEligibility)
	personalChatGroup.POST("/create", persChatHandler.CreateChat)
	personalChatGroup.POST("/send", persChatHandler.SendMessage)
	personalChatGroup.GET("/messages", persChatHandler.GetMessages)
	personalChatGroup.POST("/ack", persChatHandler.AcknowledgeDelivery)
	personalChatGroup.GET("/list", persChatHandler.GetUserChats)
	personalChatGroup.POST("/upload", persChatHandler.UploadFileForMessage)
	personalChatGroup.GET("/file-url", persChatHandler.GetFileURL)
	personalChatGroup.POST("/mark-read", persChatHandler.MarkChatRead)
	personalChatGroup.POST("/unsend", persChatHandler.UnsendMessage)
	personalChatGroup.POST("/delete-for-me", persChatHandler.DeleteMessageForMe)
	personalChatGroup.GET("/sync-actions", persChatHandler.GetSyncActions)
	personalChatGroup.POST("/sync-actions/ack", persChatHandler.AcknowledgeSyncAction)
	personalChatGroup.GET("/pending", persChatHandler.GetPendingMessages)
}
