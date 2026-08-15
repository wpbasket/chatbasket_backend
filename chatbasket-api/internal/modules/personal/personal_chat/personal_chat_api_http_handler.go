package personal_chat

import (
	"errors"
	"net/http"
	"time"

	rpc_personal_chatv1 "chatbasket-api/gen/proto/personal/personal_chat"
	rpc_personal_ssev1 "chatbasket-api/gen/proto/personal/personal_sse"
	"chatbasket-api/internal/modules/personal/personal_sse"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/websocket"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type chatHandler struct {
	Service            *chatService
	hub                *websocket.WSHub
	personalSseManager *personal_sse.Manager
}

func newChatHandler(service *chatService, hub *websocket.WSHub, personalSseManager *personal_sse.Manager) *chatHandler {
	return &chatHandler{Service: service, hub: hub, personalSseManager: personalSseManager}
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

func extractSessionUUID(c *echo.Context) uuid.UUID {
	sessionUUID, _ := c.Get("sessionUUID").(uuid.UUID)
	return sessionUUID
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

	// SSE Broadcast: SendMessageSseEvent to recipient and sender's other devices
	if h.personalSseManager != nil {
		sessionUUID := extractSessionUUID(c)
		recipientUUID, _ := uuid.Parse(resp.RecipientId)

		// To recipient: is_from_me = false
		recipientPayload := proto.Clone(resp).(*rpc_personal_chatv1.Message)
		recipientPayload.IsFromMe = false

		recipientSseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_SendMessageSseEvent{
						SendMessageSseEvent: recipientPayload,
					},
				},
			},
		}
		go h.personalSseManager.BroadcastToUser(recipientUUID, recipientSseEvent)

		// To sender's other devices: is_from_me = true
		senderSseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_SendMessageSseEvent{
						SendMessageSseEvent: resp,
					},
				},
			},
		}
		go h.personalSseManager.BroadcastToUserExcept(userID.UuidUserId, sessionUUID, senderSseEvent)
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

	messageID, parseErr := uuid.Parse(payload.MessageID)
	if parseErr != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_message_id", "Invalid message_id")
	}

	// 1. Fetch message info BEFORE acknowledgment (it might be deleted from relay during ACK)
	message, msgErr := h.Service.PostgresQueries.GetMessageByID(c.Request().Context(), messageID)
	if errors.Is(msgErr, pgx.ErrNoRows) {
		// Idempotency: Message already fully acknowledged and deleted from relay table.
		return c.JSON(http.StatusOK, &rpc_personal_chatv1.AcknowledgeDeliveryResponse{Acknowledged: true})
	} else if msgErr != nil {
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "Internal server error")
	}

	resp, svcErr := h.Service.AcknowledgeDeliveryHandler(c.Request().Context(), &payload, userID, sessionId)
	if svcErr != nil {
		return svcErr
	}

	// 2. SSE Broadcast: AcknowledgeDeliverySseEvent to sender (if acknowledgedBy == "recipient")
	if payload.AcknowledgedBy == "recipient" && h.personalSseManager != nil {
		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_AcknowledgeDeliverySseEvent{
						AcknowledgeDeliverySseEvent: &rpc_personal_chatv1.AcknowledgeDeliverySsePayload{
							ChatId:      message.ChatID.String(),
							MessageIds:  []string{payload.MessageID},
							DeliveredAt: timestamppb.New(message.CreatedAt),
						},
					},
				},
			},
		}
		go h.personalSseManager.BroadcastToUser(message.SenderID, sseEvent)
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *chatHandler) AcknowledgeDeliveryBatch(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}
	sessionId := extractSessionId(c)

	var payload AckDeliveryBatchPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	var chatID uuid.UUID
	var senderID uuid.UUID
	var latestCreatedAt time.Time
	acknowledgedCount := 0

	// 1. Fetch info and perform acknowledgments
	for i, msgIDStr := range payload.MessageIDs {
		messageID, parseErr := uuid.Parse(msgIDStr)
		if parseErr != nil {
			continue
		}

		// Fetch message info BEFORE it's potentially deleted
		if msg, msgErr := h.Service.PostgresQueries.GetMessageByID(c.Request().Context(), messageID); msgErr == nil {
			if i == 0 {
				chatID = msg.ChatID
				senderID = msg.SenderID
			}
			if msg.CreatedAt.After(latestCreatedAt) {
				latestCreatedAt = msg.CreatedAt
			}
		}

		ackErr := h.Service.AcknowledgeDelivery(c.Request().Context(), messageID, payload.AcknowledgedBy, sessionId, userID)
		if ackErr == nil {
			acknowledgedCount++
		}
	}

	// 2. SSE Broadcast: single delivery_ack with all message_ids (strict DB timestamp only)
	if payload.AcknowledgedBy == "recipient" && h.personalSseManager != nil && chatID != uuid.Nil && senderID != uuid.Nil && !latestCreatedAt.IsZero() {
		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_AcknowledgeDeliverySseEvent{
						AcknowledgeDeliverySseEvent: &rpc_personal_chatv1.AcknowledgeDeliverySsePayload{
							ChatId:      chatID.String(),
							MessageIds:  payload.MessageIDs,
							DeliveredAt: timestamppb.New(latestCreatedAt),
						},
					},
				},
			},
		}
		go h.personalSseManager.BroadcastToUser(senderID, sseEvent)
	}

	return c.JSON(http.StatusOK, AckDeliveryBatchResponse{
		AcknowledgedCount: acknowledgedCount,
	})
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

	isPrimary := extractIsPrimary(c)

	res, svcErr := h.Service.GetPendingMessagesHandler(c.Request().Context(), &payload, userID, sessionCreatedAt, isPrimary)
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

	// SSE Broadcast: MarkChatReadSseEvent to other participant
	if h.personalSseManager != nil {
		chatUUID, _ := uuid.Parse(payload.ChatID)
		chat, chatErr := h.Service.PostgresQueries.GetChatByID(c.Request().Context(), chatUUID)
		if chatErr == nil {
			var otherUserID uuid.UUID
			if chat.Participant1ID == userID.UuidUserId {
				otherUserID = chat.Participant2ID
			} else {
				otherUserID = chat.Participant1ID
			}

			sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
				Timestamp: timestamppb.Now(),
				Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
					ChatModule: &rpc_personal_chatv1.ChatSsePayload{
						Event: &rpc_personal_chatv1.ChatSsePayload_MarkChatReadSseEvent{
							MarkChatReadSseEvent: &rpc_personal_chatv1.MarkChatReadSsePayload{
								ChatId:   payload.ChatID,
								ReaderId: userID.StringUserId,
								ReadAt:   chat.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
							},
						},
					},
				},
			}
			go h.personalSseManager.BroadcastToUser(otherUserID, sseEvent)
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

	// SSE Broadcast: UnsendMessageSseEvent to recipient and sender's other devices
	if h.personalSseManager != nil {
		chatUUID, _ := uuid.Parse(payload.ChatID)
		chat, err := h.Service.PostgresQueries.GetChatByID(c.Request().Context(), chatUUID)
		if err == nil {
			var recipientID uuid.UUID
			if chat.Participant1ID == userID.UuidUserId {
				recipientID = chat.Participant2ID
			} else {
				recipientID = chat.Participant1ID
			}

			sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
				Timestamp: timestamppb.Now(),
				Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
					ChatModule: &rpc_personal_chatv1.ChatSsePayload{
						Event: &rpc_personal_chatv1.ChatSsePayload_UnsendMessageSseEvent{
							UnsendMessageSseEvent: &rpc_personal_chatv1.UnsendMessageSsePayload{
								ChatId:     payload.ChatID,
								MessageIds: payload.MessageIDs,
								SenderId:   userID.StringUserId,
							},
						},
					},
				},
			}

			sessionUUID := extractSessionUUID(c)

			go h.personalSseManager.BroadcastToUser(recipientID, sseEvent)
			go h.personalSseManager.BroadcastToUserExcept(userID.UuidUserId, sessionUUID, sseEvent)
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

	// SSE Broadcast: DeleteMessageForMeSseEvent to sender's other devices
	if h.personalSseManager != nil {
		var chatID string
		if len(payload.MessageIDs) > 0 {
			if msgUUID, parseErr := uuid.Parse(payload.MessageIDs[0]); parseErr == nil {
				if msg, lookupErr := h.Service.PostgresQueries.GetMessageByID(c.Request().Context(), msgUUID); lookupErr == nil {
					chatID = msg.ChatID.String()
				}
			}
		}

		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_DeleteMessageForMeSseEvent{
						DeleteMessageForMeSseEvent: &rpc_personal_chatv1.DeleteMessageForMeSsePayload{
							ChatId:     chatID,
							MessageIds: payload.MessageIDs,
						},
					},
				},
			},
		}

		sessionUUID := extractSessionUUID(c)
		go h.personalSseManager.BroadcastToUserExcept(userID.UuidUserId, sessionUUID, sseEvent)
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
	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_message_id", "Invalid message id")
	}
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_recipient", "Invalid recipient id")
	}
	if userID.UuidUserId == recipientID {
		return kit.NewError(http.StatusBadRequest, "invalid_recipient", "Cannot send file to yourself")
	}
	message, svcErr := h.Service.ConfirmChatUpload(c.Request().Context(), ConfirmChatUploadParams{
		MessageID:             messageID,
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
	// SSE Broadcast: ConfirmFileMessageUploadSseEvent to recipient and sender's other devices
	if h.personalSseManager != nil {
		protoMsg := &rpc_personal_chatv1.Message{
			MessageId:             message.ID.String(),
			ChatId:                message.ChatID.String(),
			RecipientId:           message.RecipientID.String(),
			SenderKeysRevision:    senderKeysRevision,
			Content:               message.Content,
			MessageType:           message.MessageType,
			DeliveredToRecipient:  false,
			SyncedToSenderPrimary: message.SyncedToSenderPrimary,
			CreatedAt:             timestamppb.New(message.CreatedAt),
			ExpiresAt:             timestamppb.New(message.ExpiresAt),
			IsFromMe:              true,
			FileId:                message.FileID,
			FileName:              message.FileName,
			FileSize:              message.FileSize,
			FileMimeType:          message.FileMimeType,
			ViewUrl:               viewURL,
			DownloadUrl:           downloadURL,
		}

		recipientUUID, _ := uuid.Parse(protoMsg.RecipientId)
		recipientPayload := proto.Clone(protoMsg).(*rpc_personal_chatv1.Message)
		recipientPayload.IsFromMe = false

		recipientSseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_ConfirmFileMessageUploadSseEvent{
						ConfirmFileMessageUploadSseEvent: recipientPayload,
					},
				},
			},
		}

		senderSseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_ConfirmFileMessageUploadSseEvent{
						ConfirmFileMessageUploadSseEvent: protoMsg,
					},
				},
			},
		}

		sessionUUID := extractSessionUUID(c)
		go h.personalSseManager.BroadcastToUser(recipientUUID, recipientSseEvent)
		go h.personalSseManager.BroadcastToUserExcept(userID.UuidUserId, sessionUUID, senderSseEvent)
	}
	return c.JSON(http.StatusOK, confirmResponse)
}

