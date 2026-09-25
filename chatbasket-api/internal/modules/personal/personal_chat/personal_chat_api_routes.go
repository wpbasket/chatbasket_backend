package personal_chat

import (
	"net/http"
	"time"

	rpc_personal_chatv1connect "chatbasket-api/gen/proto/personal/personal_chat/rpc_personal_chatv1connect"
	"chatbasket-api/internal/modules/personal/personal_sse"
	"chatbasket-api/internal/platform/middleware"

	"github.com/labstack/echo/v5"
)

// Register initializes the Chat module dependencies and registers its routes.
func Register(personalGroup *echo.Group, chatSvc *chatService, personalSseManager *personal_sse.Manager) {
	handler := newChatHandler(chatSvc, personalSseManager)

	// Normal chat calls: max 5MB upload, max 30 seconds.
	// History-sync upload and download are big and slow, so they skip
	// these group limits and set their own limits on their routes below.
	chat := personalGroup.Group(
		"/chat",
		middleware.BodyLimitWithSkipper(5242880, func(c *echo.Context) bool {
			return c.Request().URL.Path == "/api/personal/chat/history-sync/upload"
		}),
		middleware.ContextTimeoutWithSkipper(30*time.Second, func(c *echo.Context) bool {
			p := c.Request().URL.Path
			return p == "/api/personal/chat/history-sync/upload" || p == "/api/personal/chat/history-sync"
		}),
	)

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

	// History Sync.
	// Upload carries a full database copy: up to 90MB and 10 minutes.
	// Download also may take up to 10 minutes.
	chat.POST("/history-sync/request", handler.RequestHistorySync)
	chat.POST("/history-sync/upload", handler.UploadHistorySync,
		middleware.BodyLimit(94371840),
		middleware.ContextTimeout(10*time.Minute),
	)
	chat.GET("/history-sync", handler.DownloadHistorySync,
		middleware.ContextTimeout(10*time.Minute),
	)
	chat.GET("/history-sync/fetch", handler.FetchHistorySync)
	chat.POST("/history-sync/ack", handler.AcknowledgeHistorySync)

	// Same rules for the RPC version: 5MB and 30 seconds for all calls,
	// except UploadHistorySync (90MB, 10 minutes) and
	// DownloadHistorySync (10 minutes).
	connectServer := newChatConnectServer(chatSvc, personalSseManager)
	path, connectHandler := rpc_personal_chatv1connect.NewChatServiceHandler(connectServer)
	personalGroup.Any(
		path+"*",
		echo.WrapHandler(http.StripPrefix("/api/personal", connectHandler)),
		middleware.DynamicBodyLimit(5242880, middleware.BodyLimitOverrides{
			rpc_personal_chatv1connect.ChatServiceUploadHistorySyncProcedure: 94371840,
		}),
		middleware.DynamicContextTimeout(30*time.Second, middleware.ContextTimeoutOverrides{
			rpc_personal_chatv1connect.ChatServiceUploadHistorySyncProcedure:   10 * time.Minute,
			rpc_personal_chatv1connect.ChatServiceDownloadHistorySyncProcedure: 10 * time.Minute,
		}),
	)
}
