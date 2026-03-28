package routes

import (
	"chatbasket-api-legacy/middleware"
	"chatbasket-api-legacy/personal/personalhandler"
	"chatbasket-api-legacy/personal/personalservice"
	"chatbasket-api-legacy/personal/personalutils"
	"chatbasket-api-legacy/services"
	"time"

	"github.com/labstack/echo/v5"
)

// RegisterPersonalRoutes registers all personal domain routes
func RegisterPersonalRoutes(api *echo.Group, globalService *services.GlobalService, authService *services.AuthService, authSecret []byte) {
	perSvc := personalservice.New(globalService, authSecret)
	// Start background cleanup job (every 1 hour)
	personalutils.StartMessageCleanupJob(perSvc, 1*time.Hour)

	// Personal Profile Routes
	personalProfileGroup := api.Group("/personal/profile")
	personalProfileGroup.Use(middleware.AuthSessionMiddleware(authService, true))
	personalProfileHandler := personalhandler.NewProfileHandler(perSvc)
	personalProfileGroup.GET("/get-profile", personalProfileHandler.GetProfile)
	personalProfileGroup.POST("/create-profile", personalProfileHandler.CreateUserProfile)
	personalProfileGroup.POST("/upload-avatar", personalProfileHandler.UploadProfilePicture)
	personalProfileGroup.DELETE("/remove-avatar", personalProfileHandler.RemoveProfilePicture)
	personalProfileGroup.POST("/update-profile", personalProfileHandler.UpdateProfile)

	// Personal Contacts Routes
	personalContactsGroup := api.Group("/personal/contacts")
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
	persSettingGroup := api.Group("/personal/settings")
	persSettingGroup.Use(middleware.AuthSessionMiddleware(authService, true))
	persSettingHandler := personalhandler.NewSettingHandler(perSvc)
	persSettingGroup.POST("/session/central", persSettingHandler.UpdateSessionCentral)
	persSettingGroup.POST("/session/notification-token", persSettingHandler.UpdateSessionNotificationToken)

	// Personal Chat Routes
	personalChatGroup := api.Group("/personal/chat")
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
	personalChatGroup.GET("/ws", persChatHandler.WebSocketUpgrade)
}

