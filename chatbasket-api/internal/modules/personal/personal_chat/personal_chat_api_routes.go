package personal_chat

import (
	"chatbasket-api/internal/platform/middleware"
	"chatbasket-api/internal/platform/websocket"

	"github.com/labstack/echo/v5"
)

// Register initializes the Chat module dependencies and registers its routes.
func Register(personalGroup *echo.Group, chatSvc *chatService, hub *websocket.WSHub, authProvider middleware.AuthSessionProvider) {
	handler := newChatHandler(chatSvc, hub)

	// Chat Routes
	chat := personalGroup.Group("/chat")
	chat.Use(middleware.AuthSessionMiddleware(authProvider, true))

	// Chat management
	chat.POST("/check-eligibility", handler.CheckEligibility)
	chat.POST("/create", handler.CreateChat)
	chat.GET("/list", handler.GetUserChats)

	// Messaging
	chat.POST("/send", handler.SendMessage)
	chat.GET("/messages", handler.GetMessages)
	chat.GET("/pending", handler.GetPendingMessages)
	chat.POST("/ack", handler.AcknowledgeDelivery)

	// File messaging
	chat.POST("/upload", handler.UploadFileForMessage)
	chat.GET("/file-url", handler.GetFileURL)

	// Read / Unsend / Delete
	chat.POST("/mark-read", handler.MarkChatRead)
	chat.POST("/unsend", handler.UnsendMessage)
	chat.POST("/delete-for-me", handler.DeleteMessageForMe)

	// Sync actions
	chat.GET("/sync-actions", handler.GetSyncActions)
	chat.POST("/sync-actions/ack", handler.AcknowledgeSyncAction)

	// WebSocket
	chat.GET("/ws", handler.WebSocketUpgrade)
}

