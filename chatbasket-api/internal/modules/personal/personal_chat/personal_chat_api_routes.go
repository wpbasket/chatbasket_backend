package personal_chat

import (
	rpc_personal_chatv1connect "chatbasket-api/gen/proto/personal/personal_chat/rpc_personal_chatv1connect"
	"chatbasket-api/internal/modules/personal/personal_sse"
	"chatbasket-api/internal/platform/middleware"
	"chatbasket-api/internal/platform/websocket"

	"github.com/labstack/echo/v5"
	"net/http"
)

// Register initializes the Chat module dependencies and registers its routes.
func Register(personalGroup *echo.Group, chatSvc *chatService, hub *websocket.WSHub, personalSseManager *personal_sse.Manager) {
	handler := newChatHandler(chatSvc, hub, personalSseManager)

	// Chat Routes
	chat := personalGroup.Group("/chat")

	// Chat management
	chat.POST("/check-eligibility", handler.CheckEligibility)
	chat.POST("/create", handler.CreateChat)
	chat.GET("/list", handler.GetUserChats)

	// Messaging
	chat.POST("/send", handler.SendMessage)
	chat.GET("/messages", handler.GetMessages)
	chat.GET("/pending", handler.GetPendingMessages)
	chat.POST("/ack", handler.AcknowledgeDelivery)
	chat.POST("/ack-batch", handler.AcknowledgeDeliveryBatch)

	// File messaging
	chat.POST("/presign", handler.PresignUpload)
	chat.POST("/confirm", handler.ConfirmUpload)
	chat.GET("/file-url", handler.GetFileURL)

	// Read / Unsend / Delete
	chat.POST("/mark-read", handler.MarkChatRead)
	chat.POST("/unsend", handler.UnsendMessage)
	chat.POST("/delete-for-me", handler.DeleteMessageForMe)

	// Sync actions
	chat.GET("/sync-actions", handler.GetSyncActions)
	chat.POST("/sync-actions/ack", handler.AcknowledgeSyncAction)

	// History Sync
	chat.POST("/history-sync/request", handler.RequestHistorySync)
	chat.POST("/history-sync/upload", handler.UploadHistorySync, middleware.BodyLimit(94371840)) // 90MB limit for database cipher sync
	chat.GET("/history-sync", handler.DownloadHistorySync)

	// WebSocket
	chat.GET("/ws", handler.WebSocketUpgrade)

	// Connect RPC Routes
	connectServer := newChatConnectServer(chatSvc, hub, personalSseManager)
	path, connectHandler := rpc_personal_chatv1connect.NewChatServiceHandler(connectServer)
	personalGroup.Any(path+"*", echo.WrapHandler(http.StripPrefix("/api/personal", connectHandler)))
}
