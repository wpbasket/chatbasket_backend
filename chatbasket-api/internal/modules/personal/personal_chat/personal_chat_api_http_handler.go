package personal_chat

import (
	"net/http"

	rpc_personal_chatv1 "chatbasket-api/gen/proto/personal/personal_chat"
	rpc_personal_ssev1 "chatbasket-api/gen/proto/personal/personal_sse"
	"chatbasket-api/internal/modules/personal/personal_sse"
	"chatbasket-api/internal/platform/kit"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type chatHandler struct {
	Service            *chatService
	personalSseManager *personal_sse.Manager
}

func newChatHandler(service *chatService, personalSseManager *personal_sse.Manager) *chatHandler {
	return &chatHandler{Service: service, personalSseManager: personalSseManager}
}

// ──────────────────────────────────────────────────────────────────────────────
// Auth helpers
// ──────────────────────────────────────────────────────────────────────────────

func extractIsPrimary(c *echo.Context) bool {
	isPrimary, _ := c.Get("isPrimary").(bool)
	return isPrimary
}

// ──────────────────────────────────────────────────────────────────────────────
// REST Handlers
// ──────────────────────────────────────────────────────────────────────────────

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
	sessionUUID, err := kit.ExtractSessionUUID(c)
	if err != nil {
		return err
	}

	var payload SendMessagePayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	if err := validateMessageContentLength(payload.Content); err != nil {
		return err
	}

	resp, svcErr := h.Service.SendMessageHandler(c.Request().Context(), &payload, userID, isPrimary, sessionUUID)
	if svcErr != nil {
		return svcErr
	}

	// SSE Broadcast: SendMessageSseEvent to the recipient and to EVERY sender stream.
	// Sibling tabs share the session and the shared local store; the echo lets them
	// mirror the confirmed row into their own memory (the authoring tab dedupes it).
	if h.personalSseManager != nil {
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

		// To every sender stream: is_from_me = true
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
		go h.personalSseManager.BroadcastToUser(userID.UuidUserId, senderSseEvent)
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *chatHandler) GetMessages(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}
	isPrimary := extractIsPrimary(c)

	var payload GetMessagesPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request params")
	}

	sessionCreatedAt, err := kit.ExtractSessionCreatedAt(c)
	if err != nil {
		return err
	}

	res, svcErr := h.Service.GetMessagesHandler(c.Request().Context(), &payload, userID, sessionCreatedAt, isPrimary)
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

	var payload AcknowledgeDeliveryPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	isPrimary := extractIsPrimary(c)
	resp, deliveryGroups, svcErr := h.Service.AcknowledgeDeliveryHandler(c.Request().Context(), &payload, userID, isPrimary)
	if svcErr != nil {
		return svcErr
	}

	// SSE Broadcast per distinct (SenderID, ChatID) group asynchronously without blocking endpoint response
	// Only broadcast newly delivered messages (strictly excluding already-delivered and non-existent messages)
	if payload.AcknowledgedBy == "recipient" && h.personalSseManager != nil && len(deliveryGroups) > 0 {
		go func(groups []AckDeliveryGroup) {
			for _, group := range groups {
				if group.SenderID == uuid.Nil || group.ChatID == uuid.Nil || len(group.MessageIDs) == 0 {
					continue
				}
				sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
					Timestamp: timestamppb.Now(),
					Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
						ChatModule: &rpc_personal_chatv1.ChatSsePayload{
							Event: &rpc_personal_chatv1.ChatSsePayload_AcknowledgeDeliverySseEvent{
								AcknowledgeDeliverySseEvent: &rpc_personal_chatv1.AcknowledgeDeliverySsePayload{
									ChatId:      group.ChatID.String(),
									MessageIds:  group.MessageIDs,
									DeliveredAt: timestamppb.New(group.LatestCreatedAt),
								},
							},
						},
					},
				}
				h.personalSseManager.BroadcastToUser(group.SenderID, sseEvent)
			}
		}(deliveryGroups)
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *chatHandler) AcknowledgeDeliveryBatch(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}
	isPrimary := extractIsPrimary(c)

	var payload AckDeliveryBatchPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	res, ackErr := h.Service.AcknowledgeDeliveryBatch(c.Request().Context(), payload.MessageIDs, payload.AcknowledgedBy, isPrimary, userID)
	if ackErr != nil {
		return ackErr
	}

	// SSE Broadcast per distinct (SenderID, ChatID) group asynchronously without blocking endpoint response
	if payload.AcknowledgedBy == "recipient" && h.personalSseManager != nil && len(res.DeliveryGroups) > 0 {
		go func(groups []AckDeliveryGroup) {
			for _, group := range groups {
				if group.SenderID == uuid.Nil || group.ChatID == uuid.Nil || len(group.MessageIDs) == 0 {
					continue
				}
				sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
					Timestamp: timestamppb.Now(),
					Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
						ChatModule: &rpc_personal_chatv1.ChatSsePayload{
							Event: &rpc_personal_chatv1.ChatSsePayload_AcknowledgeDeliverySseEvent{
								AcknowledgeDeliverySseEvent: &rpc_personal_chatv1.AcknowledgeDeliverySsePayload{
									ChatId:      group.ChatID.String(),
									MessageIds:  group.MessageIDs,
									DeliveredAt: timestamppb.New(group.LatestCreatedAt),
								},
							},
						},
					},
				}
				h.personalSseManager.BroadcastToUser(group.SenderID, sseEvent)
			}
		}(res.DeliveryGroups)
	}

	return c.JSON(http.StatusOK, AckDeliveryBatchResponse{
		AcknowledgedCount:             int(res.AcknowledgedCount),
		AcknowledgedMessageIds:        res.AcknowledgedMessageIDs,
		PermanentlyRejectedMessageIds: res.PermanentlyRejectedIDs,
	})
}

func (h *chatHandler) AcknowledgeReadReceiptBatch(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}
	isPrimary := extractIsPrimary(c)

	var payload AckReadReceiptBatchPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}
	if len(payload.MessageIDs) > MaxMessageIDsPerRequest {
		return ErrTooManyMessageIDs
	}

	res, err := h.Service.AcknowledgeReadReceiptBatch(c.Request().Context(), &payload, userID, isPrimary)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, AckReadReceiptBatchResponse{
		AcknowledgedCount:             int(res.AcknowledgedCount),
		AcknowledgedMessageIds:        res.AcknowledgedMessageIds,
		PermanentlyRejectedMessageIds: res.PermanentlyRejectedMessageIds,
	})
}

func (h *chatHandler) AcknowledgeAndReadBatch(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}
	isPrimary := extractIsPrimary(c)

	var payload AckAndReadBatchPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}
	if len(payload.MessageIDs) > MaxMessageIDsPerRequest {
		return ErrTooManyMessageIDs
	}

	out, err := h.Service.AcknowledgeAndReadBatch(c.Request().Context(), &payload, userID, isPrimary)
	if err != nil {
		return err
	}

	// SSE Broadcast: MarkChatReadSseEvent to other participant asynchronously without blocking endpoint response
	// Only broadcast newly read messages (strictly excluding already-true and non-existent messages)
	if h.personalSseManager != nil && out.OtherUserID != uuid.Nil && len(out.NewlyRead) > 0 {
		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_MarkChatReadSseEvent{
						MarkChatReadSseEvent: &rpc_personal_chatv1.MarkChatReadSsePayload{
							ChatId:       out.ChatID.String(),
							ReaderId:     userID.StringUserId,
							ReadAt:       formatChatTimestamp(out.ReadAt),
							ReadMessages: out.NewlyRead,
						},
					},
				},
			},
		}
		go h.personalSseManager.BroadcastToUser(out.OtherUserID, sseEvent)
	}

	return c.JSON(http.StatusOK, &rpc_personal_chatv1.AckAndReadBatchResponse{
		AcknowledgedCount:             int32(len(out.Receipts)),
		ReadMessages:                  out.Receipts,
		PermanentlyRejectedMessageIds: out.PermanentlyRejectedIDs,
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
	if len(payload.MessageIDs) > MaxMessageIDsPerRequest {
		return ErrTooManyMessageIDs
	}

	out, svcErr := h.Service.MarkChatRead(c.Request().Context(), &payload, userID, isPrimary)
	if svcErr != nil {
		return svcErr
	}

	// SSE Broadcast: MarkChatReadSseEvent to other participant asynchronously without blocking endpoint response
	// Only broadcast newly read messages (strictly excluding already-true and non-existent messages)
	if h.personalSseManager != nil && out.OtherUserID != uuid.Nil && len(out.NewlyRead) > 0 {
		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_MarkChatReadSseEvent{
						MarkChatReadSseEvent: &rpc_personal_chatv1.MarkChatReadSsePayload{
							ChatId:       out.ChatID.String(),
							ReaderId:     userID.StringUserId,
							ReadAt:       formatChatTimestamp(out.ReadAt),
							ReadMessages: out.NewlyRead,
						},
					},
				},
			},
		}
		go h.personalSseManager.BroadcastToUser(out.OtherUserID, sseEvent)
	}

	return c.JSON(http.StatusOK, MarkChatReadResponse{
		Status:                        true,
		ReadAt:                        formatChatTimestamp(out.ReadAt),
		ReadMessages:                  out.Receipts,
		PermanentlyRejectedMessageIds: out.PermanentlyRejectedIDs,
	})
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
	if len(payload.MessageIDs) > MaxMessageIDsPerRequest {
		return ErrTooManyMessageIDs
	}

	out, svcErr := h.Service.UnsendMessageHandler(c.Request().Context(), &payload, userID, isPrimary)
	if svcErr != nil {
		return svcErr
	}

	// SSE Broadcast: UnsendMessageSseEvent to the recipient and to EVERY sender stream
	// Only broadcast newly unsent messages (strictly excluding already-true, non-existent, and ineligible messages)
	if h.personalSseManager != nil && out.ChatID != uuid.Nil && len(out.NewlyUnsentIDs) > 0 {
		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_UnsendMessageSseEvent{
						UnsendMessageSseEvent: &rpc_personal_chatv1.UnsendMessageSsePayload{
							ChatId:     out.ChatID.String(),
							MessageIds: out.NewlyUnsentIDs,
							SenderId:   userID.StringUserId,
						},
					},
				},
			},
		}
		go func(rID uuid.UUID, sID uuid.UUID, evt *rpc_personal_ssev1.PersonalSseEvent) {
			if rID != uuid.Nil {
				h.personalSseManager.BroadcastToUser(rID, evt)
			}
			h.personalSseManager.BroadcastToUser(sID, evt)
		}(out.RecipientID, userID.UuidUserId, sseEvent)
	}

	return c.JSON(http.StatusOK, UnsendMessageResponse{
		AcknowledgedCount:             len(out.AcknowledgedIDs),
		AcknowledgedMessageIds:        out.AcknowledgedIDs,
		PermanentlyRejectedMessageIds: out.PermanentlyRejectedIDs,
	})
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
	if len(payload.MessageIDs) > MaxMessageIDsPerRequest {
		return ErrTooManyMessageIDs
	}

	groups, svcErr := h.Service.DeleteMessageForMeHandler(c.Request().Context(), &payload, userID, isPrimary)
	if svcErr != nil {
		return svcErr
	}

	// Smart SSE Broadcast: Asynchronously broadcast without blocking the endpoint response
	if h.personalSseManager != nil && len(groups) > 0 {
		go func(targetUserID uuid.UUID, grps []DeleteForMeGroup) {
			for _, g := range grps {
				chatIDStr := ""
				if g.ChatID != uuid.Nil {
					chatIDStr = g.ChatID.String()
				}
				sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
					Timestamp: timestamppb.Now(),
					Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
						ChatModule: &rpc_personal_chatv1.ChatSsePayload{
							Event: &rpc_personal_chatv1.ChatSsePayload_DeleteMessageForMeSseEvent{
								DeleteMessageForMeSseEvent: &rpc_personal_chatv1.DeleteMessageForMeSsePayload{
									ChatId:     chatIDStr,
									MessageIds: g.MessageIDs,
								},
							},
						},
					},
				}
				h.personalSseManager.BroadcastToUser(targetUserID, sseEvent)
			}
		}(userID.UuidUserId, groups)
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
	sessionUUID, err := kit.ExtractSessionUUID(c)
	if err != nil {
		return err
	}
	var payload ConfirmChatUploadPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "Invalid confirm payload")
	}

	if err := validateMessageContentLength(payload.Content); err != nil {
		return err
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
	var replyToUUID *uuid.UUID
	if payload.ReplyToMessageID != nil && *payload.ReplyToMessageID != "" {
		parsed, err := uuid.Parse(*payload.ReplyToMessageID)
		if err != nil {
			return kit.NewError(http.StatusBadRequest, "invalid_reply_to_message_id", "Invalid reply to message id")
		}
		replyToUUID = &parsed
	}
	message, svcErr := h.Service.ConfirmChatUpload(c.Request().Context(), ConfirmChatUploadParams{
		MessageID:             messageID,
		SenderID:              userID,
		RecipientID:           recipientID,
		FileID:                payload.FileID,
		Content:               payload.Content,
		MessageType:           payload.MessageType,
		IsPrimary:             isPrimary,
		SessionUUID:           sessionUUID,
		RecipientKeysRevision: payload.RecipientKeysRevision,
		SenderKeysRevision:    payload.SenderKeysRevision,
		ReplyToMessageID:      replyToUUID,
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
		ReplyToMessageID:   uuidPtrToStringPtr(message.ReplyToMessageID),
	}
	// SSE Broadcast: ConfirmFileMessageUploadSseEvent to the recipient and to EVERY sender stream
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
			ReadByRecipient:       message.ReadByRecipient,
			ReadAckedBySender:     message.ReadAckedBySender,
			ReadAt:                kit.OptionalTimestamp(message.ReadAt),
			ReplyToMessageId:      uuidPtrToStringPtr(message.ReplyToMessageID),
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

		go h.personalSseManager.BroadcastToUser(recipientUUID, recipientSseEvent)
		go h.personalSseManager.BroadcastToUser(userID.UuidUserId, senderSseEvent)
	}
	return c.JSON(http.StatusOK, confirmResponse)
}
