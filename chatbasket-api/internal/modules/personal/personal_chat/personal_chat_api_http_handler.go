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
	userID, err := kit.ExtractUserID(c)
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
	userID, err := kit.ExtractUserID(c)
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
	userID, err := kit.ExtractUserID(c)
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

	// ——— WS Broadcast: new_message ————————————————————————————————————————————————————————————————
	if h.hub != nil {
		sessionId := extractSessionId(c)
		recipientUUID, _ := uuid.Parse(resp.RecipientID)

		log.Printf("[WS Broadcast] SendMessage: msgID=%s chatID=%s sender=%s recipient=%s sessionId=%s",
			resp.MessageID, resp.ChatID, userID.StringUserId, resp.RecipientID, sessionId)

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
			userID.StringUserId, sessionId)
		go h.hub.BroadcastToUserExcept(userID.UuidUserId, sessionId, websocket.WSEvent{
			Type:    WSEventNewMessage,
			Payload: resp,
		})
	} else {
		log.Printf("[WS Broadcast] SendMessage: WSHub is NIL, skipping broadcast")
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *chatHandler) GetMessages(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	var payload GetMessagesPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request params")
	}

	sessionCreatedAt, err := kit.ExtractSessionCreatedAt(c)
	if err != nil {
		return err
	}

	res, svcErr := h.Service.GetMessagesHandler(c.Request().Context(), &payload, userID, sessionCreatedAt)
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, res)
}

func (h *chatHandler) AcknowledgeDelivery(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
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

	// ——— WS Broadcast: delivery_ack ———————————————————————————————————————————————————————————————
	if h.hub != nil && resp.Acknowledged && payload.AcknowledgedBy == "recipient" {
		log.Printf("[WS Broadcast] AckDelivery: msgID=%s acknowledged_by=%s user=%s → looking up sender",
			payload.MessageID, payload.AcknowledgedBy, userID.StringUserId)
		msgUUID, parseErr := uuid.Parse(payload.MessageID)
		if parseErr == nil {
			msg, lookupErr := h.Service.PostgresQueries.GetMessageByID(c.Request().Context(), msgUUID)
			if lookupErr == nil {
				log.Printf("[WS Broadcast] AckDelivery: pushing delivery_ack to SENDER=%s for msgID=%s chatID=%s",
					msg.SenderID, payload.MessageID, msg.ChatID)
				go h.hub.BroadcastToUser(msg.SenderID, websocket.WSEvent{
					Type: WSEventDeliveryAck,
					Payload: DeliveryAckEventPayload{
						MessageIDs: []string{payload.MessageID},
						ChatID:     msg.ChatID.String(),
					},
				})
			} else {
				log.Printf("[WS Broadcast] AckDelivery: GetMessageByID FAILED for msgID=%s: %v", payload.MessageID, lookupErr)
			}
		} else {
			log.Printf("[WS Broadcast] AckDelivery: parse msgID FAILED: %s → %v", payload.MessageID, parseErr)
		}
	} else if h.hub != nil {
		log.Printf("[WS Broadcast] AckDelivery: SKIPPED (acknowledged=%v, acknowledged_by=%s)",
			resp.Acknowledged, payload.AcknowledgedBy)
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *chatHandler) GetUserChats(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	sessionCreatedAt, err := kit.ExtractSessionCreatedAt(c)
	if err != nil {
		return err
	}

	res, svcErr := h.Service.GetUserChatsHandler(c.Request().Context(), userID, sessionCreatedAt)
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, res)
}

func (h *chatHandler) GetPendingMessages(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	payload := GetPendingMessagesPayload{}
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	sessionCreatedAt, err := kit.ExtractSessionCreatedAt(c)
	if err != nil {
		return err
	}

	res, svcErr := h.Service.GetPendingMessagesHandler(c.Request().Context(), &payload, userID, sessionCreatedAt)
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, res)
}


func (h *chatHandler) GetFileURL(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	var payload GetFileURLPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_request", "invalid request payload")
	}

	res, svcErr := h.Service.GetFileURLHandler(c.Request().Context(), &payload, userID)
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, res)
}

func (h *chatHandler) MarkChatRead(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
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

	// ——— WS Broadcast: read_receipt ———————————————————————————————————————————————————————————————
	if h.hub != nil {
		log.Printf("[WS Broadcast] MarkChatRead: chatID=%s reader=%s → looking up other participant",
			payload.ChatID, userID.StringUserId)
		chatUUID, _ := uuid.Parse(payload.ChatID)
		chat, chatErr := h.Service.PostgresQueries.GetChatByID(c.Request().Context(), chatUUID)
		if chatErr == nil {
			var otherUserID uuid.UUID
			if chat.Participant1ID == userID.UuidUserId {
				otherUserID = chat.Participant2ID
			} else {
				otherUserID = chat.Participant1ID
			}

			readAt := time.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
			log.Printf("[WS Broadcast] MarkChatRead: pushing read_receipt to OTHER_USER=%s for chatID=%s read_at=%s",
				otherUserID, payload.ChatID, readAt)
			go h.hub.BroadcastToUser(otherUserID, websocket.WSEvent{
				Type: WSEventReadReceipt,
				Payload: ReadReceiptEventPayload{
					ChatID:   payload.ChatID,
					ReaderID: userID.StringUserId,
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
	userID, err := kit.ExtractUserID(c)
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

	// ——— WS Broadcast: unsend —————————————————————————————————————————————————————————————————————
	if h.hub != nil {
		log.Printf("[WS Broadcast] Unsend: chatID=%s sender=%s messageIDs=%v → looking up recipient",
			payload.ChatID, userID.StringUserId, payload.MessageIDs)
		chatUUID, _ := uuid.Parse(payload.ChatID)
		chat, chatErr := h.Service.PostgresQueries.GetChatByID(c.Request().Context(), chatUUID)
		if chatErr == nil {
			var recipientID uuid.UUID
			if chat.Participant1ID == userID.UuidUserId {
				recipientID = chat.Participant2ID
			} else {
				recipientID = chat.Participant1ID
			}

			unsendEvent := websocket.WSEvent{
				Type: WSEventUnsend,
				Payload: UnsendEventPayload{
					ChatID:     payload.ChatID,
					MessageIDs: payload.MessageIDs,
					SenderID:   userID.StringUserId,
				},
			}

			// Notify recipient (all devices)
			log.Printf("[WS Broadcast] Unsend: pushing unsend to RECIPIENT=%s", recipientID)
			go h.hub.BroadcastToUser(recipientID, unsendEvent)

			// Notify sender's other devices (for sync)
			sessionId := extractSessionId(c)
			log.Printf("[WS Broadcast] Unsend: pushing unsend to SENDER=%s other devices (excluding session=%s)",
				userID.StringUserId, sessionId)
			go h.hub.BroadcastToUserExcept(userID.UuidUserId, sessionId, unsendEvent)
		} else {
			log.Printf("[WS Broadcast] Unsend: GetChatByID FAILED for chatID=%s: %v", payload.ChatID, chatErr)
		}
	}

	return c.JSON(http.StatusOK, kit.StatusOkay{Status: true, Message: "success"})
}

func (h *chatHandler) DeleteMessageForMe(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
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

	// ——— WS Broadcast: delete_for_me ——————————————————————————————————————————————————————————————
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

		log.Printf("[WS Broadcast] DeleteForMe: user=%s messageIDs=%v chatID=%s → pushing to other devices (excluding session=%s)",
			userID.StringUserId, payload.MessageIDs, chatID, sessionId)
		go h.hub.BroadcastToUserExcept(userID.UuidUserId, sessionId, websocket.WSEvent{
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
	userID, err := kit.ExtractUserID(c)
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
func (h *chatHandler) PresignUpload(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}
	var payload struct {
		RecipientID           string `json:"recipient_id"`
		MessageType           string `json:"message_type"`
		RecipientKeysRevision int32  `json:"recipient_keys_revision"`
		SenderKeysRevision    int32  `json:"sender_keys_revision"`
	}
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "Invalid presign payload: "+err.Error())
	}
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_recipient", "Invalid recipient id")
	}
	res, svcErr := h.Service.PresignChatUpload(c.Request().Context(), PresignChatUploadParams{
		SenderID:              userID,
		RecipientID:           recipientID,
		MessageType:           payload.MessageType,
		RecipientKeysRevision: payload.RecipientKeysRevision,
		SenderKeysRevision:    payload.SenderKeysRevision,
	})
	if svcErr != nil {
		return svcErr
	}
	return c.JSON(http.StatusOK, res)
}

func (h *chatHandler) ConfirmUpload(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}
	isPrimary := extractIsPrimary(c)
	var payload ConfirmChatUploadPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "Invalid confirm payload")
	}
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_recipient", "Invalid recipient id")
	}
	if userID.UuidUserId == recipientID {
		return kit.NewError(http.StatusBadRequest, "invalid_recipient", "Cannot send file to yourself")
	}
	message, svcErr := h.Service.ConfirmChatUpload(c.Request().Context(), ConfirmChatUploadParams{
		SenderID:              userID,
		RecipientID:           recipientID,
		FileID:                payload.FileID,
		Content:               payload.Content,
		MessageType:           payload.MessageType,
		IsPrimary:             isPrimary,
		RecipientKeysRevision: payload.RecipientKeysRevision,
		SenderKeysRevision:    payload.SenderKeysRevision,
	})
	if svcErr != nil {
		return svcErr
	}
	viewURL, downloadURL, _ := h.Service.GenerateMessageFileURLs(c.Request().Context(), *message, userID)
	senderKeysRevision := h.Service.getSenderKeysRevision(c.Request().Context(), message.SenderID)
	msgInfo := &MessageResponse{
		MessageID:             message.ID.String(),
		ChatID:                message.ChatID.String(),
		RecipientID:           message.RecipientID.String(),
		SenderKeysRevision:    senderKeysRevision,
		Content:               message.Content,
		MessageType:           message.MessageType,
		DeliveredToRecipient:  false,
		SyncedToSenderPrimary: message.SyncedToSenderPrimary,
		CreatedAt:             message.CreatedAt,
		ExpiresAt:             message.ExpiresAt,
		IsFromMe:              true,
		FileID:                message.FileID,
		FileName:              message.FileName,
		FileSize:              message.FileSize,
		FileMimeType:          message.FileMimeType,
		ViewURL:               viewURL,
		DownloadURL:           downloadURL,
	}
	confirmResponse := &ConfirmChatUploadResponse{
		MessageID:          message.ID.String(),
		ChatID:             message.ChatID.String(),
		RecipientID:        message.RecipientID.String(),
		SenderKeysRevision: senderKeysRevision,
		FileID:             *message.FileID,
		MessageType:        message.MessageType,
		ViewURL:            viewURL,
		DownloadURL:        downloadURL,
		CreatedAt:          message.CreatedAt,
		ExpiresAt:          message.ExpiresAt,
	}
	if h.hub != nil {
		sessionId := extractSessionId(c)
		recipientUUID, _ := uuid.Parse(msgInfo.RecipientID)
		recipientPayload := *msgInfo
		recipientPayload.IsFromMe = false
		go h.hub.BroadcastToUser(recipientUUID, websocket.WSEvent{Type: WSEventNewMessage, Payload: recipientPayload})
		go h.hub.BroadcastToUserExcept(userID.UuidUserId, sessionId, websocket.WSEvent{Type: WSEventNewMessage, Payload: msgInfo})
	}
	return c.JSON(http.StatusOK, confirmResponse)
}

