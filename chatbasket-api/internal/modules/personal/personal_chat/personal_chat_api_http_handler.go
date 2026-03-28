package personal_chat

import (
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/websocket"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type chatHandler struct {
	Service *chatService
	hub     *websocket.WSHub
}

func newChatHandler(service *chatService, hub *websocket.WSHub) *chatHandler {
	return &chatHandler{Service: service, hub: hub}
}

// â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
// Auth helpers
// â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

func extractUserID(c *echo.Context) (uuid.UUID, error) {
	uuidUserId, ok := c.Get("uuidUserId").(uuid.UUID)
	if !ok {
		return uuid.Nil, kit.NewError(http.StatusUnauthorized, "unauthorized", "User id is missing or invalid")
	}
	return uuidUserId, nil
}

func extractSessionId(c *echo.Context) string {
	sessionId, _ := c.Get("sessionId").(string)
	return sessionId
}

func extractIsPrimary(c *echo.Context) bool {
	isPrimary, _ := c.Get("isPrimary").(bool)
	return isPrimary
}

// â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
// REST Handlers
// â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

func (h *chatHandler) CheckEligibility(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}

	var payload CheckEligibilityPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	res, svcErr := h.Service.CheckEligibilityHandler(c.Request().Context(), &payload, userID)
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, res)
}

func (h *chatHandler) CreateChat(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}

	var payload CreateChatPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	res, svcErr := h.Service.CreateChatHandler(c.Request().Context(), &payload, userID)
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, res)
}

func (h *chatHandler) SendMessage(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}
	isPrimary := extractIsPrimary(c)

	var payload SendMessagePayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	resp, svcErr := h.Service.SendMessageHandler(c.Request().Context(), &payload, userID, isPrimary)
	if svcErr != nil {
		return svcErr
	}

	// â”€â”€ WS Broadcast: new_message â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	if h.hub != nil {
		sessionId := extractSessionId(c)
		recipientUUID, _ := uuid.Parse(resp.RecipientID)

		log.Printf("[WS Broadcast] SendMessage: msgID=%s chatID=%s sender=%s recipient=%s sessionId=%s",
			resp.MessageID, resp.ChatID, userID, resp.RecipientID, sessionId)

		// For recipient: is_from_me = false
		recipientPayload := *resp
		recipientPayload.IsFromMe = false
		log.Printf("[WS Broadcast] SendMessage: pushing new_message to RECIPIENT=%s (is_from_me=false)", recipientUUID)
		go h.hub.BroadcastToUser(recipientUUID, websocket.WSEvent{
			Type:    WSEventNewMessage,
			Payload: recipientPayload,
		})

		// For sender's OTHER devices: is_from_me = true (sync)
		log.Printf("[WS Broadcast] SendMessage: pushing new_message to SENDER=%s other devices (excluding session=%s)",
			userID, sessionId)
		go h.hub.BroadcastToUserExcept(userID, sessionId, websocket.WSEvent{
			Type:    WSEventNewMessage,
			Payload: resp,
		})
	} else {
		log.Printf("[WS Broadcast] SendMessage: WSHub is NIL, skipping broadcast")
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *chatHandler) GetMessages(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}

	var payload GetMessagesPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request params")
	}

	res, svcErr := h.Service.GetMessagesHandler(c.Request().Context(), &payload, userID)
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, res)
}

func (h *chatHandler) AcknowledgeDelivery(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}
	sessionId := extractSessionId(c)

	var payload AcknowledgeDeliveryPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	resp, svcErr := h.Service.AcknowledgeDeliveryHandler(c.Request().Context(), &payload, userID, sessionId)
	if svcErr != nil {
		return svcErr
	}

	// â”€â”€ WS Broadcast: delivery_ack â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	if h.hub != nil && resp.Acknowledged && payload.AcknowledgedBy == "recipient" {
		log.Printf("[WS Broadcast] AckDelivery: msgID=%s acknowledged_by=%s user=%s â†’ looking up sender",
			payload.MessageID, payload.AcknowledgedBy, userID)
		msgUUID, parseErr := uuid.Parse(payload.MessageID)
		if parseErr == nil {
			msg, lookupErr := h.Service.PostgresQueries.GetMessageByID(c.Request().Context(), msgUUID)
			if lookupErr == nil {
				log.Printf("[WS Broadcast] AckDelivery: pushing delivery_ack to SENDER=%s for msgID=%s chatID=%s deliveredAt=%v",
					msg.SenderID, payload.MessageID, msg.ChatID, msg.CreatedAt)
				go h.hub.BroadcastToUser(msg.SenderID, websocket.WSEvent{
					Type: WSEventDeliveryAck,
					Payload: DeliveryAckEventPayload{
						MessageIDs:  []string{payload.MessageID},
						ChatID:      msg.ChatID.String(),
						DeliveredAt: kit.DerefTime(msg.CreatedAt),
					},
				})
			} else {
				log.Printf("[WS Broadcast] AckDelivery: GetMessageByID FAILED for msgID=%s: %v", payload.MessageID, lookupErr)
			}
		} else {
			log.Printf("[WS Broadcast] AckDelivery: parse msgID FAILED: %s â†’ %v", payload.MessageID, parseErr)
		}
	} else if h.hub != nil {
		log.Printf("[WS Broadcast] AckDelivery: SKIPPED (acknowledged=%v, acknowledged_by=%s)",
			resp.Acknowledged, payload.AcknowledgedBy)
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *chatHandler) GetUserChats(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}

	res, svcErr := h.Service.GetUserChatsHandler(c.Request().Context(), userID)
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, res)
}

func (h *chatHandler) GetPendingMessages(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}

	payload := GetPendingMessagesPayload{}
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	res, svcErr := h.Service.GetPendingMessagesHandler(c.Request().Context(), &payload, userID)
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, res)
}

func (h *chatHandler) UploadFileForMessage(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}
	isPrimary := extractIsPrimary(c)

	recipientIDStr := c.FormValue("recipient_id")
	if recipientIDStr == "" {
		return kit.NewError(http.StatusBadRequest, "invalid_request", "recipient_id is required")
	}

	recipientID, err := uuid.Parse(recipientIDStr)
	if err != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_recipient", "Invalid recipient ID")
	}

	if userID == recipientID {
		return kit.NewError(http.StatusBadRequest, "invalid_recipient", "Cannot send file to yourself")
	}

	messageType := c.FormValue("message_type")
	if messageType == "" {
		messageType = "file"
	}

	caption := c.FormValue("caption")

	file, err := c.FormFile("file")
	if err != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_request", "No file provided")
	}

	message, svcErr := h.Service.UploadFileForMessage(c.Request().Context(), UploadFileForMessageParams{
		SenderID:    userID,
		RecipientID: recipientID,
		FileHeader:  file,
		MessageType: messageType,
		Caption:     caption,
		IsPrimary:   isPrimary,
	})
	if svcErr != nil {
		return svcErr
	}

	viewURL, downloadURL, _ := h.Service.GenerateMessageFileURLs(c.Request().Context(), *message, userID)

	msgInfo := &MessageResponse{
		MessageID:             message.ID.String(),
		ChatID:                message.ChatID.String(),
		RecipientID:           message.RecipientID.String(),
		Content:               message.Content,
		MessageType:           message.MessageType,
		DeliveredToRecipient:  false,
		SyncedToSenderPrimary: message.SyncedToSenderPrimary,
		CreatedAt:             kit.DerefTime(message.CreatedAt),
		ExpiresAt:             message.ExpiresAt,
		IsFromMe:              true,
		FileID:                message.FileID,
		FileName:              message.FileName,
		FileSize:              message.FileSize,
		FileMimeType:          message.FileMimeType,
		ViewURL:               viewURL,
		DownloadURL:           downloadURL,
	}

	uploadResponse := &UploadFileResponse{
		MessageID:    message.ID.String(),
		FileID:       *message.FileID,
		MessageType:  message.MessageType,
		FileMimeType: message.FileMimeType,
		ViewURL:      viewURL,
		DownloadURL:  downloadURL,
		FileName:     message.FileName,
		FileSize:     message.FileSize,
		CreatedAt:    kit.DerefTime(message.CreatedAt),
		ExpiresAt:    message.ExpiresAt,
	}

	// â”€â”€ WS Broadcast: new_message (File Upload) â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	if h.hub != nil {
		sessionId := extractSessionId(c)
		recipientUUID, _ := uuid.Parse(msgInfo.RecipientID)

		log.Printf("[WS Broadcast] UploadFileForMessage: msgID=%s chatID=%s sender=%s recipient=%s sessionId=%s",
			msgInfo.MessageID, msgInfo.ChatID, userID, msgInfo.RecipientID, sessionId)

		// For recipient: is_from_me = false
		recipientPayload := *msgInfo
		recipientPayload.IsFromMe = false
		log.Printf("[WS Broadcast] UploadFileForMessage: pushing new_message to RECIPIENT=%s", recipientUUID)
		go h.hub.BroadcastToUser(recipientUUID, websocket.WSEvent{
			Type:    WSEventNewMessage,
			Payload: recipientPayload,
		})

		// For sender's OTHER devices: is_from_me = true
		log.Printf("[WS Broadcast] UploadFileForMessage: pushing new_message to SENDER=%s other devices", userID)
		go h.hub.BroadcastToUserExcept(userID, sessionId, websocket.WSEvent{
			Type:    WSEventNewMessage,
			Payload: msgInfo,
		})
	} else {
		log.Printf("[WS Broadcast] UploadFileForMessage: WSHub is NIL or msgInfo missing, skipping broadcast")
	}

	return c.JSON(http.StatusOK, uploadResponse)
}

func (h *chatHandler) GetFileURL(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}

	payload := GetFileURLPayload{
		MessageID: c.QueryParam("message_id"),
	}

	res, svcErr := h.Service.GetFileURLHandler(c.Request().Context(), &payload, userID)
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, res)
}

func (h *chatHandler) MarkChatRead(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}
	isPrimary := extractIsPrimary(c)

	var payload MarkChatReadPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	svcErr := h.Service.MarkChatReadHandler(c.Request().Context(), &payload, userID, isPrimary)
	if svcErr != nil {
		return svcErr
	}

	// â”€â”€ WS Broadcast: read_receipt â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	if h.hub != nil {
		log.Printf("[WS Broadcast] MarkChatRead: chatID=%s reader=%s â†’ looking up other participant",
			payload.ChatID, userID)
		chatUUID, _ := uuid.Parse(payload.ChatID)
		chat, chatErr := h.Service.PostgresQueries.GetChatByID(c.Request().Context(), chatUUID)
		if chatErr == nil {
			var otherUserID uuid.UUID
			if chat.Participant1ID == userID {
				otherUserID = chat.Participant2ID
			} else {
				otherUserID = chat.Participant1ID
			}

			readAt := time.Now().UTC().Format(time.RFC3339Nano)
			log.Printf("[WS Broadcast] MarkChatRead: pushing read_receipt to OTHER_USER=%s for chatID=%s read_at=%s",
				otherUserID, payload.ChatID, readAt)
			go h.hub.BroadcastToUser(otherUserID, websocket.WSEvent{
				Type: WSEventReadReceipt,
				Payload: ReadReceiptEventPayload{
					ChatID:   payload.ChatID,
					ReaderID: userID.String(),
					ReadAt:   readAt,
				},
			})
		} else {
			log.Printf("[WS Broadcast] MarkChatRead: GetChatByID FAILED for chatID=%s: %v", payload.ChatID, chatErr)
		}
	}

	return c.JSON(http.StatusOK, kit.StatusOkay{Status: true, Message: "success"})
}

func (h *chatHandler) UnsendMessage(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}
	isPrimary := extractIsPrimary(c)

	var payload UnsendMessagePayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	svcErr := h.Service.UnsendMessageHandler(c.Request().Context(), &payload, userID, isPrimary)
	if svcErr != nil {
		return svcErr
	}

	// â”€â”€ WS Broadcast: unsend â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	if h.hub != nil {
		log.Printf("[WS Broadcast] Unsend: chatID=%s sender=%s messageIDs=%v â†’ looking up recipient",
			payload.ChatID, userID, payload.MessageIDs)
		chatUUID, _ := uuid.Parse(payload.ChatID)
		chat, chatErr := h.Service.PostgresQueries.GetChatByID(c.Request().Context(), chatUUID)
		if chatErr == nil {
			var recipientID uuid.UUID
			if chat.Participant1ID == userID {
				recipientID = chat.Participant2ID
			} else {
				recipientID = chat.Participant1ID
			}

			unsendEvent := websocket.WSEvent{
				Type: WSEventUnsend,
				Payload: UnsendEventPayload{
					ChatID:     payload.ChatID,
					MessageIDs: payload.MessageIDs,
					SenderID:   userID.String(),
				},
			}

			// Notify recipient (all devices)
			log.Printf("[WS Broadcast] Unsend: pushing unsend to RECIPIENT=%s", recipientID)
			go h.hub.BroadcastToUser(recipientID, unsendEvent)

			// Notify sender's other devices (for sync)
			sessionId := extractSessionId(c)
			log.Printf("[WS Broadcast] Unsend: pushing unsend to SENDER=%s other devices (excluding session=%s)",
				userID, sessionId)
			go h.hub.BroadcastToUserExcept(userID, sessionId, unsendEvent)
		} else {
			log.Printf("[WS Broadcast] Unsend: GetChatByID FAILED for chatID=%s: %v", payload.ChatID, chatErr)
		}
	}

	return c.JSON(http.StatusOK, kit.StatusOkay{Status: true, Message: "success"})
}

func (h *chatHandler) DeleteMessageForMe(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}
	isPrimary := extractIsPrimary(c)

	var payload DeleteMessageForMePayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	svcErr := h.Service.DeleteMessageForMeHandler(c.Request().Context(), &payload, userID, isPrimary)
	if svcErr != nil {
		return svcErr
	}

	// â”€â”€ WS Broadcast: delete_for_me â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	if h.hub != nil {
		sessionId := extractSessionId(c)

		// Resolve chat_id from the first message so the frontend can clear the preview
		var chatID string
		if len(payload.MessageIDs) > 0 {
			if msgUUID, parseErr := uuid.Parse(payload.MessageIDs[0]); parseErr == nil {
				if msg, lookupErr := h.Service.PostgresQueries.GetMessageByID(c.Request().Context(), msgUUID); lookupErr == nil {
					chatID = msg.ChatID.String()
				}
			}
		}

		log.Printf("[WS Broadcast] DeleteForMe: user=%s messageIDs=%v chatID=%s â†’ pushing to other devices (excluding session=%s)",
			userID, payload.MessageIDs, chatID, sessionId)
		go h.hub.BroadcastToUserExcept(userID, sessionId, websocket.WSEvent{
			Type: WSEventDeleteForMe,
			Payload: DeleteForMeEventPayload{
				MessageIDs: payload.MessageIDs,
				ChatID:     chatID,
			},
		})
	}

	return c.JSON(http.StatusOK, kit.StatusOkay{Status: true, Message: "success"})
}

func (h *chatHandler) GetSyncActions(c *echo.Context) error {
	userID, err := extractUserID(c)
	if err != nil {
		return err
	}

	payload := GetSyncActionsPayload{}
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	res, svcErr := h.Service.GetSyncActionsHandler(c.Request().Context(), &payload, userID)
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, res)
}

func (h *chatHandler) AcknowledgeSyncAction(c *echo.Context) error {
	isPrimary := extractIsPrimary(c)

	var payload AcknowledgeSyncActionPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	svcErr := h.Service.AcknowledgeSyncActionHandler(c.Request().Context(), &payload, isPrimary)
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, kit.StatusOkay{Status: true, Message: "success"})
}

