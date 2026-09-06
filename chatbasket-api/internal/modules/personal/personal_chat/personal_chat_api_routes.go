package personal_chat

import (
	rpc_personal_chatv1connect "chatbasket-api/gen/proto/personal/personal_chat/rpc_personal_chatv1connect"
	"chatbasket-api/internal/modules/personal/personal_sse"
	"chatbasket-api/internal/platform/middleware"

	"github.com/labstack/echo/v5"
	"net/http"
)

// Register initializes the Chat module dependencies and registers its routes.
func Register(personalGroup *echo.Group, chatSvc *chatService, personalSseManager *personal_sse.Manager) {
	handler := newChatHandler(chatSvc, personalSseManager)

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
	chat.POST("/ack-read-batch", handler.AcknowledgeReadReceiptBatch)
	chat.POST("/ack-and-read-batch", handler.AcknowledgeAndReadBatch)

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
	chat.POST("/history-sync/ack", handler.AcknowledgeHistorySync)

	// Connect RPC Routes
	connectServer := newChatConnectServer(chatSvc, personalSseManager)
	path, connectHandler := rpc_personal_chatv1connect.NewChatServiceHandler(connectServer)
	personalGroup.Any(path+"*", echo.WrapHandler(http.StripPrefix("/api/personal", connectHandler)))
}
