package personal_chat

import (
	"context"
	"errors"
	"net/http"
	"time"

	rpc_common_modelv1 "chatbasket-api/gen/proto/common/model"
	rpc_personal_chatv1 "chatbasket-api/gen/proto/personal/personal_chat"
	rpc_personal_chatv1connect "chatbasket-api/gen/proto/personal/personal_chat/rpc_personal_chatv1connect"
	rpc_personal_ssev1 "chatbasket-api/gen/proto/personal/personal_sse"
	"chatbasket-api/internal/modules/personal/personal_sse"
	"chatbasket-api/internal/platform/kit"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type chatConnectServer struct {
	rpc_personal_chatv1connect.UnimplementedChatServiceHandler
	chatService        *chatService
	personalSseManager *personal_sse.Manager
}

func newChatConnectServer(service *chatService, personalSseManager *personal_sse.Manager) rpc_personal_chatv1connect.ChatServiceHandler {
	return &chatConnectServer{
		chatService:        service,
		personalSseManager: personalSseManager,
	}
}

func (s *chatConnectServer) CheckEligibility(ctx context.Context, req *connect.Request[rpc_personal_chatv1.CheckEligibilityRequest]) (*connect.Response[rpc_personal_chatv1.CheckEligibilityResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	res, err := s.chatService.CheckEligibilityHandler(ctx, &CheckEligibilityPayload{
		RecipientID: req.Msg.RecipientId,
	}, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *chatConnectServer) CreateChat(ctx context.Context, req *connect.Request[rpc_personal_chatv1.CreateChatRequest]) (*connect.Response[rpc_personal_chatv1.CreateChatResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	res, err := s.chatService.CreateChatHandler(ctx, &CreateChatPayload{
		RecipientID: req.Msg.RecipientId,
	}, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *chatConnectServer) SendMessage(ctx context.Context, req *connect.Request[rpc_personal_chatv1.SendMessageRequest]) (*connect.Response[rpc_personal_chatv1.Message], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	isPrimary, err := kit.GetConnectRpcIsPrimary(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	res, err := s.chatService.SendMessageHandler(ctx, &SendMessagePayload{
		MessageID:             req.Msg.MessageId,
		RecipientID:           req.Msg.RecipientId,
		Content:               req.Msg.Content,
		MessageType:           req.Msg.MessageType,
		RecipientKeysRevision: req.Msg.RecipientKeysRevision,
		SenderKeysRevision:    req.Msg.SenderKeysRevision,
	}, userID, isPrimary)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	// SSE Broadcast: SendMessageSseEvent to recipient and sender's other devices
	if s.personalSseManager != nil {
		sessionUUID, _ := kit.GetConnectRpcSessionUUID(ctx)
		recipientUUID, _ := uuid.Parse(res.RecipientId)

		// To recipient: is_from_me = false
		recipientPayload := proto.Clone(res).(*rpc_personal_chatv1.Message)
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
		go s.personalSseManager.BroadcastToUser(recipientUUID, recipientSseEvent)

		// To sender's other devices: is_from_me = true
		senderSseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_SendMessageSseEvent{
						SendMessageSseEvent: res,
					},
				},
			},
		}
		go s.personalSseManager.BroadcastToUserExcept(userID.UuidUserId, sessionUUID, senderSseEvent)
	}

	return connect.NewResponse(res), nil
}

func (s *chatConnectServer) GetMessages(ctx context.Context, req *connect.Request[rpc_personal_chatv1.GetMessagesRequest]) (*connect.Response[rpc_personal_chatv1.GetMessagesResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	sessionCreatedAt, err := kit.GetConnectRpcSessionCreatedAt(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	res, err := s.chatService.GetMessagesHandler(ctx, &GetMessagesPayload{
		ChatID: req.Msg.ChatId,
		Limit:  req.Msg.Limit,
		Offset: req.Msg.Offset,
	}, userID, sessionCreatedAt)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *chatConnectServer) AcknowledgeDelivery(ctx context.Context, req *connect.Request[rpc_personal_chatv1.AcknowledgeDeliveryRequest]) (*connect.Response[rpc_personal_chatv1.AcknowledgeDeliveryResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	sessionID, _ := kit.GetConnectRpcSessionID(ctx)

	messageID, parseErr := uuid.Parse(req.Msg.MessageId)
	if parseErr != nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "invalid_message_id", "Invalid message_id"))
	}

	// 1. Fetch message info BEFORE acknowledgment (it might be deleted from relay during ACK)
	message, msgErr := s.chatService.PostgresQueries.GetMessageByID(ctx, messageID)
	if errors.Is(msgErr, pgx.ErrNoRows) {
		// Idempotency: Message already fully acknowledged and deleted from relay table.
		return connect.NewResponse(&rpc_personal_chatv1.AcknowledgeDeliveryResponse{Acknowledged: true}), nil
	} else if msgErr != nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusInternalServerError, "internal_server_error", "Internal server error"))
	}

	res, err := s.chatService.AcknowledgeDeliveryHandler(ctx, &AcknowledgeDeliveryPayload{
		MessageID:      req.Msg.MessageId,
		AcknowledgedBy: req.Msg.AcknowledgedBy,
		Success:        req.Msg.Success,
	}, userID, sessionID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	// 2. SSE Broadcast: AcknowledgeDeliverySseEvent to sender (if acknowledgedBy == "recipient")
	if req.Msg.AcknowledgedBy == "recipient" && s.personalSseManager != nil {
		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_AcknowledgeDeliverySseEvent{
						AcknowledgeDeliverySseEvent: &rpc_personal_chatv1.AcknowledgeDeliverySsePayload{
							ChatId:      message.ChatID.String(),
							MessageIds:  []string{req.Msg.MessageId},
							DeliveredAt: timestamppb.New(message.CreatedAt),
						},
					},
				},
			},
		}
		go s.personalSseManager.BroadcastToUser(message.SenderID, sseEvent)
	}

	return connect.NewResponse(res), nil
}

func (s *chatConnectServer) AcknowledgeDeliveryBatch(ctx context.Context, req *connect.Request[rpc_personal_chatv1.AckDeliveryBatchPayload]) (*connect.Response[rpc_personal_chatv1.AckDeliveryBatchResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	sessionID, _ := kit.GetConnectRpcSessionID(ctx)

	var chatID uuid.UUID
	var senderID uuid.UUID
	var latestCreatedAt time.Time
	acknowledgedCount := 0

	// 1. Fetch info and perform acknowledgments
	for i, msgIDStr := range req.Msg.MessageIds {
		messageID, err := uuid.Parse(msgIDStr)
		if err != nil {
			continue
		}

		// Fetch message info BEFORE it's potentially deleted
		if msg, msgErr := s.chatService.PostgresQueries.GetMessageByID(ctx, messageID); msgErr == nil {
			if i == 0 {
				chatID = msg.ChatID
				senderID = msg.SenderID
			}
			if msg.CreatedAt.After(latestCreatedAt) {
				latestCreatedAt = msg.CreatedAt
			}
		}

		ackErr := s.chatService.AcknowledgeDelivery(ctx, messageID, req.Msg.AcknowledgedBy, sessionID, userID)
		if ackErr == nil {
			acknowledgedCount++
		}
	}

	// 2. SSE Broadcast: single delivery_ack with all message_ids (strict DB timestamp only)
	if req.Msg.AcknowledgedBy == "recipient" && s.personalSseManager != nil && chatID != uuid.Nil && senderID != uuid.Nil && !latestCreatedAt.IsZero() {
		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_AcknowledgeDeliverySseEvent{
						AcknowledgeDeliverySseEvent: &rpc_personal_chatv1.AcknowledgeDeliverySsePayload{
							ChatId:      chatID.String(),
							MessageIds:  req.Msg.MessageIds,
							DeliveredAt: timestamppb.New(latestCreatedAt),
						},
					},
				},
			},
		}
		go s.personalSseManager.BroadcastToUser(senderID, sseEvent)
	}

	return connect.NewResponse(&rpc_personal_chatv1.AckDeliveryBatchResponse{
		AcknowledgedCount: int32(acknowledgedCount),
	}), nil
}

func (s *chatConnectServer) GetUserChats(ctx context.Context, req *connect.Request[rpc_personal_chatv1.GetUserChatsRequest]) (*connect.Response[rpc_personal_chatv1.GetUserChatsResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	sessionCreatedAt, err := kit.GetConnectRpcSessionCreatedAt(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	res, err := s.chatService.GetUserChatsHandler(ctx, userID, sessionCreatedAt)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *chatConnectServer) GetPendingMessages(ctx context.Context, req *connect.Request[rpc_personal_chatv1.GetPendingMessagesRequest]) (*connect.Response[rpc_personal_chatv1.GetPendingMessagesResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	sessionCreatedAt, err := kit.GetConnectRpcSessionCreatedAt(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	isPrimary, err := kit.GetConnectRpcIsPrimary(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	res, err := s.chatService.GetPendingMessagesHandler(ctx, &GetPendingMessagesPayload{
		Limit: req.Msg.Limit,
	}, userID, sessionCreatedAt, isPrimary)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *chatConnectServer) GetFileURL(ctx context.Context, req *connect.Request[rpc_personal_chatv1.GetFileURLRequest]) (*connect.Response[rpc_personal_chatv1.GetFileURLResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	res, err := s.chatService.GetFileURLHandler(ctx, &GetFileURLPayload{
		MessageID: req.Msg.MessageId,
	}, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *chatConnectServer) MarkChatRead(ctx context.Context, req *connect.Request[rpc_personal_chatv1.MarkChatReadRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	isPrimary, err := kit.GetConnectRpcIsPrimary(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	svcErr := s.chatService.MarkChatReadHandler(ctx, &MarkChatReadPayload{
		ChatID: req.Msg.ChatId,
	}, userID, isPrimary)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	// SSE Broadcast: MarkChatReadSseEvent to other participant
	if s.personalSseManager != nil {
		chatUUID, _ := uuid.Parse(req.Msg.ChatId)
		chat, chatErr := s.chatService.PostgresQueries.GetChatByID(ctx, chatUUID)
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
								ChatId:   req.Msg.ChatId,
								ReaderId: userID.StringUserId,
								ReadAt:   chat.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
							},
						},
					},
				},
			}
			go s.personalSseManager.BroadcastToUser(otherUserID, sseEvent)
		}
	}

	return connect.NewResponse(&rpc_common_modelv1.StatusOkay{Status: true, Message: "success"}), nil
}

func (s *chatConnectServer) UnsendMessage(ctx context.Context, req *connect.Request[rpc_personal_chatv1.UnsendMessageRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	isPrimary, err := kit.GetConnectRpcIsPrimary(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	svcErr := s.chatService.UnsendMessageHandler(ctx, &UnsendMessagePayload{
		ChatID:     req.Msg.ChatId,
		MessageIDs: req.Msg.MessageIds,
	}, userID, isPrimary)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	// SSE Broadcast: UnsendMessageSseEvent to recipient and sender's other devices
	if s.personalSseManager != nil {
		chatUUID, _ := uuid.Parse(req.Msg.ChatId)
		chat, err := s.chatService.PostgresQueries.GetChatByID(ctx, chatUUID)
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
								ChatId:     req.Msg.ChatId,
								MessageIds: req.Msg.MessageIds,
								SenderId:   userID.StringUserId,
							},
						},
					},
				},
			}

			sessionUUID, _ := kit.GetConnectRpcSessionUUID(ctx)

			go s.personalSseManager.BroadcastToUser(recipientID, sseEvent)
			go s.personalSseManager.BroadcastToUserExcept(userID.UuidUserId, sessionUUID, sseEvent)
		}
	}

	return connect.NewResponse(&rpc_common_modelv1.StatusOkay{Status: true, Message: "success"}), nil
}

func (s *chatConnectServer) DeleteMessageForMe(ctx context.Context, req *connect.Request[rpc_personal_chatv1.DeleteMessageForMeRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	isPrimary, err := kit.GetConnectRpcIsPrimary(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	svcErr := s.chatService.DeleteMessageForMeHandler(ctx, &DeleteMessageForMePayload{
		MessageIDs: req.Msg.MessageIds,
	}, userID, isPrimary)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	// SSE Broadcast: DeleteMessageForMeSseEvent to sender's other devices
	if s.personalSseManager != nil {
		var chatID string
		if len(req.Msg.MessageIds) > 0 {
			if msgUUID, parseErr := uuid.Parse(req.Msg.MessageIds[0]); parseErr == nil {
				if msg, lookupErr := s.chatService.PostgresQueries.GetMessageByID(ctx, msgUUID); lookupErr == nil {
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
							MessageIds: req.Msg.MessageIds,
						},
					},
				},
			},
		}

		sessionUUID, _ := kit.GetConnectRpcSessionUUID(ctx)
		go s.personalSseManager.BroadcastToUserExcept(userID.UuidUserId, sessionUUID, sseEvent)
	}

	return connect.NewResponse(&rpc_common_modelv1.StatusOkay{Status: true, Message: "success"}), nil
}

func (s *chatConnectServer) GetSyncActions(ctx context.Context, req *connect.Request[rpc_personal_chatv1.GetSyncActionsRequest]) (*connect.Response[rpc_personal_chatv1.GetSyncActionsResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	res, err := s.chatService.GetSyncActionsHandler(ctx, &GetSyncActionsPayload{
		Limit: req.Msg.Limit,
	}, userID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *chatConnectServer) AcknowledgeSyncAction(ctx context.Context, req *connect.Request[rpc_personal_chatv1.AcknowledgeSyncActionRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	isPrimary, err := kit.GetConnectRpcIsPrimary(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	svcErr := s.chatService.AcknowledgeSyncActionHandler(ctx, &AcknowledgeSyncActionPayload{
		ActionID: req.Msg.ActionId,
	}, isPrimary)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	return connect.NewResponse(&rpc_common_modelv1.StatusOkay{Status: true, Message: "success"}), nil
}

func (s *chatConnectServer) PresignUpload(ctx context.Context, req *connect.Request[rpc_personal_chatv1.PresignChatUploadRequest]) (*connect.Response[rpc_personal_chatv1.PresignChatUploadResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	recipientID, err := uuid.Parse(req.Msg.RecipientId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "invalid_recipient", "Invalid recipient id"))
	}

	res, err := s.chatService.PresignChatUpload(ctx, PresignChatUploadParams{
		SenderID:              userID,
		RecipientID:           recipientID,
		MessageType:           req.Msg.MessageType,
		RecipientKeysRevision: req.Msg.RecipientKeysRevision,
		SenderKeysRevision:    req.Msg.SenderKeysRevision,
	})
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	return connect.NewResponse(res), nil
}

func (s *chatConnectServer) ConfirmUpload(ctx context.Context, req *connect.Request[rpc_personal_chatv1.ConfirmChatUploadRequest]) (*connect.Response[rpc_personal_chatv1.ConfirmChatUploadResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	isPrimary, err := kit.GetConnectRpcIsPrimary(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	messageID, err := uuid.Parse(req.Msg.MessageId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "invalid_message_id", "Invalid message id"))
	}

	recipientID, err := uuid.Parse(req.Msg.RecipientId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "invalid_recipient", "Invalid recipient id"))
	}

	if userID.UuidUserId == recipientID {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "invalid_recipient", "Cannot send file to yourself"))
	}

	message, svcErr := s.chatService.ConfirmChatUpload(ctx, ConfirmChatUploadParams{
		MessageID:             messageID,
		SenderID:              userID,
		RecipientID:           recipientID,
		FileID:                req.Msg.FileId,
		Content:               req.Msg.Content,
		MessageType:           req.Msg.MessageType,
		IsPrimary:             isPrimary,
		RecipientKeysRevision: req.Msg.RecipientKeysRevision,
		SenderKeysRevision:    req.Msg.SenderKeysRevision,
	})
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	viewURL, downloadURL, _ := s.chatService.GenerateMessageFileURLs(ctx, *message, userID)
	senderKeysRevision := s.chatService.getSenderKeysRevision(ctx, message.SenderID)

	confirmRes := &rpc_personal_chatv1.ConfirmChatUploadResponse{
		MessageId:          message.ID.String(),
		ChatId:             message.ChatID.String(),
		RecipientId:        message.RecipientID.String(),
		SenderKeysRevision: senderKeysRevision,
		FileId:             *message.FileID,
		MessageType:        message.MessageType,
		ViewUrl:            viewURL,
		DownloadUrl:        downloadURL,
		CreatedAt:          timestamppb.New(message.CreatedAt),
		ExpiresAt:          timestamppb.New(message.ExpiresAt),
	}

	// SSE Broadcast: ConfirmFileMessageUploadSseEvent to recipient and sender's other devices
	if s.personalSseManager != nil {
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

		sessionUUID, _ := kit.GetConnectRpcSessionUUID(ctx)
		go s.personalSseManager.BroadcastToUser(recipientUUID, recipientSseEvent)
		go s.personalSseManager.BroadcastToUserExcept(userID.UuidUserId, sessionUUID, senderSseEvent)
	}

	return connect.NewResponse(confirmRes), nil
}

func (s *chatConnectServer) RequestHistorySync(ctx context.Context, req *connect.Request[rpc_personal_chatv1.RequestHistorySyncRequest]) (*connect.Response[rpc_personal_chatv1.RequestHistorySyncResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	sessionUUIDVal, err := kit.GetConnectRpcSessionUUID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	if req.Msg.UsedPrimaryKey == "" {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "used_primary_key is required"))
	}

	requestID, primarySessionID, requesterPubKey, svcErr := s.chatService.RequestHistorySync(
		ctx,
		userID.UuidUserId,
		sessionUUIDVal,
		req.Msg.ChatsCipher,
		req.Msg.UsedPrimaryKey,
	)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	// SSE Broadcast: RequestHistorySyncSseEvent to primary device
	if s.personalSseManager != nil && primarySessionID != uuid.Nil {
		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_RequestHistorySyncSseEvent{
						RequestHistorySyncSseEvent: &rpc_personal_chatv1.RequestHistorySyncSsePayload{
							RequestId:          requestID.String(),
							RequesterPublicKey: requesterPubKey,
							ChatsCipher:        req.Msg.ChatsCipher,
						},
					},
				},
			},
		}
		go s.personalSseManager.BroadcastToUserSession(userID.UuidUserId, primarySessionID, sseEvent)
	}

	return connect.NewResponse(&rpc_personal_chatv1.RequestHistorySyncResponse{
		RequestId: requestID.String(),
	}), nil
}

func (s *chatConnectServer) UploadHistorySync(ctx context.Context, req *connect.Request[rpc_personal_chatv1.UploadHistorySyncRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	isPrimary, err := kit.GetConnectRpcIsPrimary(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if !isPrimary {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusForbidden, "forbidden", "Only primary device can upload history sync"))
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	requestID, err := uuid.Parse(req.Msg.RequestId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request_id"))
	}

	requesterSessionID, svcErr := s.chatService.UploadHistorySync(ctx, userID.UuidUserId, requestID, req.Msg.PayloadCipher)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	// SSE Broadcast: UploadHistorySyncSseEvent to requesting secondary device
	if s.personalSseManager != nil {
		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_UploadHistorySyncSseEvent{
						UploadHistorySyncSseEvent: &rpc_personal_chatv1.UploadHistorySyncSsePayload{
							RequestId: requestID.String(),
						},
					},
				},
			},
		}
		go s.personalSseManager.BroadcastToUserSession(userID.UuidUserId, requesterSessionID, sseEvent)
	}

	return connect.NewResponse(&rpc_common_modelv1.StatusOkay{Status: true}), nil
}

func (s *chatConnectServer) DownloadHistorySync(ctx context.Context, req *connect.Request[rpc_personal_chatv1.DownloadHistorySyncRequest]) (*connect.Response[rpc_personal_chatv1.DownloadHistorySyncResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	sessionUUIDVal, err := kit.GetConnectRpcSessionUUID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	requestID, err := uuid.Parse(req.Msg.RequestId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "missing or invalid request_id"))
	}

	payloadCipher, svcErr := s.chatService.DownloadHistorySync(ctx, userID.UuidUserId, sessionUUIDVal, requestID)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	payload := ""
	if payloadCipher != nil {
		payload = *payloadCipher
	}

	return connect.NewResponse(&rpc_personal_chatv1.DownloadHistorySyncResponse{
		PayloadCipher: payload,
	}), nil
}
