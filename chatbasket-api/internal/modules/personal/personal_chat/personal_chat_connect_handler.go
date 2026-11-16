package personal_chat

import (
	"context"
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

	sessionUUID, err := kit.GetConnectRpcSessionUUID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	if err := validateMessageContentLength(req.Msg.Content); err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	res, err := s.chatService.SendMessageHandler(ctx, &SendMessagePayload{
		MessageID:             req.Msg.MessageId,
		RecipientID:           req.Msg.RecipientId,
		Content:               req.Msg.Content,
		MessageType:           req.Msg.MessageType,
		RecipientKeysRevision: req.Msg.RecipientKeysRevision,
		SenderKeysRevision:    req.Msg.SenderKeysRevision,
		ReplyToMessageID:      req.Msg.ReplyToMessageId,
	}, userID, isPrimary, sessionUUID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	// SSE Broadcast: SendMessageSseEvent to the recipient and to EVERY sender stream.
	// Sibling tabs of the same browser share the session and the shared local store,
	// but only the authoring tab runs confirmSentMessage(); the echo lets them mirror
	// the confirmed row into their own memory (the authoring tab dedupes it).
	if s.personalSseManager != nil {
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

		// To every sender stream: is_from_me = true
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
		go s.personalSseManager.BroadcastToUser(userID.UuidUserId, senderSseEvent)
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

	isPrimary, err := kit.GetConnectRpcIsPrimary(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	var afterCreatedAt *time.Time
	if req.Msg.AfterCreatedAt != nil {
		t := req.Msg.AfterCreatedAt.AsTime()
		afterCreatedAt = &t
	}

	res, err := s.chatService.GetMessagesHandler(ctx, &GetMessagesPayload{
		ChatID:         req.Msg.ChatId,
		Limit:          req.Msg.Limit,
		AfterCreatedAt: afterCreatedAt,
		AfterMessageID: req.Msg.AfterMessageId,
	}, userID, sessionCreatedAt, isPrimary)
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

	isPrimary, _ := kit.GetConnectRpcIsPrimary(ctx)

	resp, deliveryGroups, err := s.chatService.AcknowledgeDeliveryHandler(ctx, &AcknowledgeDeliveryPayload{
		MessageID:      req.Msg.MessageId,
		AcknowledgedBy: req.Msg.AcknowledgedBy,
		Success:        req.Msg.Success,
	}, userID, isPrimary)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	// SSE Broadcast per distinct (SenderID, ChatID) group asynchronously without blocking endpoint response
	// Only broadcast newly delivered messages (strictly excluding already-delivered and non-existent messages)
	if req.Msg.AcknowledgedBy == "recipient" && s.personalSseManager != nil && len(deliveryGroups) > 0 {
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
				s.personalSseManager.BroadcastToUser(group.SenderID, sseEvent)
			}
		}(deliveryGroups)
	}

	return connect.NewResponse(resp), nil
}

func (s *chatConnectServer) AcknowledgeDeliveryBatch(ctx context.Context, req *connect.Request[rpc_personal_chatv1.AckDeliveryBatchPayload]) (*connect.Response[rpc_personal_chatv1.AckDeliveryBatchResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	isPrimary, _ := kit.GetConnectRpcIsPrimary(ctx)

	res, ackErr := s.chatService.AcknowledgeDeliveryBatch(ctx, req.Msg.MessageIds, req.Msg.AcknowledgedBy, isPrimary, userID)
	if ackErr != nil {
		return nil, kit.ParseIntoRpcError(ackErr)
	}

	// SSE Broadcast per distinct (SenderID, ChatID) group asynchronously without blocking endpoint response
	if req.Msg.AcknowledgedBy == "recipient" && s.personalSseManager != nil && len(res.DeliveryGroups) > 0 {
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
				s.personalSseManager.BroadcastToUser(group.SenderID, sseEvent)
			}
		}(res.DeliveryGroups)
	}

	return connect.NewResponse(&rpc_personal_chatv1.AckDeliveryBatchResponse{
		AcknowledgedCount:             res.AcknowledgedCount,
		AcknowledgedMessageIds:        res.AcknowledgedMessageIDs,
		PermanentlyRejectedMessageIds: res.PermanentlyRejectedIDs,
	}), nil
}

func (s *chatConnectServer) AcknowledgeReadReceiptBatch(ctx context.Context, req *connect.Request[rpc_personal_chatv1.AckReadReceiptBatchRequest]) (*connect.Response[rpc_personal_chatv1.AckReadReceiptBatchResponse], error) {
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

	if len(req.Msg.MessageIds) > MaxMessageIDsPerRequest {
		return nil, kit.ParseIntoRpcError(ErrTooManyMessageIDs)
	}

	res, err := s.chatService.AcknowledgeReadReceiptBatch(ctx, &AckReadReceiptBatchPayload{
		MessageIDs: req.Msg.MessageIds,
	}, userID, isPrimary)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}
	return connect.NewResponse(res), nil
}

func (s *chatConnectServer) AcknowledgeAndReadBatch(ctx context.Context, req *connect.Request[rpc_personal_chatv1.AckAndReadBatchPayload]) (*connect.Response[rpc_personal_chatv1.AckAndReadBatchResponse], error) {
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

	if len(req.Msg.MessageIds) > MaxMessageIDsPerRequest {
		return nil, kit.ParseIntoRpcError(ErrTooManyMessageIDs)
	}

	out, svcErr := s.chatService.AcknowledgeAndReadBatch(ctx, &AckAndReadBatchPayload{
		MessageIDs: req.Msg.MessageIds,
	}, userID, isPrimary)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	// SSE Broadcast: MarkChatReadSseEvent to other participant asynchronously without blocking endpoint response
	// Only broadcast newly read messages (strictly excluding already-true and non-existent messages)
	if s.personalSseManager != nil && out.OtherUserID != uuid.Nil && len(out.NewlyRead) > 0 {
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
		go s.personalSseManager.BroadcastToUser(out.OtherUserID, sseEvent)
	}

	return connect.NewResponse(&rpc_personal_chatv1.AckAndReadBatchResponse{
		AcknowledgedCount:             int32(len(out.Receipts)),
		ReadMessages:                  out.Receipts,
		PermanentlyRejectedMessageIds: out.PermanentlyRejectedIDs,
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

	var afterRecipientCreatedAt *time.Time
	if req.Msg.AfterRecipientCreatedAt != nil {
		t := req.Msg.AfterRecipientCreatedAt.AsTime()
		afterRecipientCreatedAt = &t
	}

	var afterSenderCreatedAt *time.Time
	if req.Msg.AfterSenderCreatedAt != nil {
		t := req.Msg.AfterSenderCreatedAt.AsTime()
		afterSenderCreatedAt = &t
	}

	res, err := s.chatService.GetPendingMessagesHandler(ctx, &GetPendingMessagesPayload{
		Limit:                   req.Msg.Limit,
		AfterRecipientCreatedAt: afterRecipientCreatedAt,
		AfterRecipientMessageID: req.Msg.AfterRecipientMessageId,
		AfterSenderCreatedAt:    afterSenderCreatedAt,
		AfterSenderMessageID:    req.Msg.AfterSenderMessageId,
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

func (s *chatConnectServer) MarkChatRead(ctx context.Context, req *connect.Request[rpc_personal_chatv1.MarkChatReadRequest]) (*connect.Response[rpc_personal_chatv1.MarkChatReadResponse], error) {
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

	if len(req.Msg.MessageIds) > MaxMessageIDsPerRequest {
		return nil, kit.ParseIntoRpcError(ErrTooManyMessageIDs)
	}

	out, svcErr := s.chatService.MarkChatRead(ctx, &MarkChatReadPayload{
		MessageIDs: req.Msg.MessageIds,
	}, userID, isPrimary)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	// SSE Broadcast: MarkChatReadSseEvent to other participant asynchronously without blocking endpoint response
	// Only broadcast newly read messages (strictly excluding already-true and non-existent messages)
	if s.personalSseManager != nil && out.OtherUserID != uuid.Nil && len(out.NewlyRead) > 0 {
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
		go s.personalSseManager.BroadcastToUser(out.OtherUserID, sseEvent)
	}

	return connect.NewResponse(&rpc_personal_chatv1.MarkChatReadResponse{
		Status:                        true,
		ReadAt:                        formatChatTimestamp(out.ReadAt),
		ReadMessages:                  out.Receipts,
		PermanentlyRejectedMessageIds: out.PermanentlyRejectedIDs,
	}), nil
}

func (s *chatConnectServer) UnsendMessage(ctx context.Context, req *connect.Request[rpc_personal_chatv1.UnsendMessageRequest]) (*connect.Response[rpc_personal_chatv1.UnsendMessageResponse], error) {
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

	if len(req.Msg.MessageIds) > MaxMessageIDsPerRequest {
		return nil, kit.ParseIntoRpcError(ErrTooManyMessageIDs)
	}

	out, svcErr := s.chatService.UnsendMessageHandler(ctx, &UnsendMessagePayload{
		MessageIDs: req.Msg.MessageIds,
	}, userID, isPrimary)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	// SSE Broadcast: UnsendMessageSseEvent to the recipient and to EVERY sender stream.
	// Only broadcast newly unsent messages (strictly excluding already-true, non-existent, and ineligible messages)
	if s.personalSseManager != nil && out.ChatID != uuid.Nil && len(out.NewlyUnsentIDs) > 0 {
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
				s.personalSseManager.BroadcastToUser(rID, evt)
			}
			s.personalSseManager.BroadcastToUser(sID, evt)
		}(out.RecipientID, userID.UuidUserId, sseEvent)
	}

	return connect.NewResponse(&rpc_personal_chatv1.UnsendMessageResponse{
		AcknowledgedCount:             int32(len(out.AcknowledgedIDs)),
		AcknowledgedMessageIds:        out.AcknowledgedIDs,
		PermanentlyRejectedMessageIds: out.PermanentlyRejectedIDs,
	}), nil
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

	if len(req.Msg.MessageIds) > MaxMessageIDsPerRequest {
		return nil, kit.ParseIntoRpcError(ErrTooManyMessageIDs)
	}

	groups, svcErr := s.chatService.DeleteMessageForMeHandler(ctx, &DeleteMessageForMePayload{
		MessageIDs: req.Msg.MessageIds,
	}, userID, isPrimary)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	// Smart SSE Broadcast: Asynchronously broadcast without blocking the endpoint response
	if s.personalSseManager != nil && len(groups) > 0 {
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
				s.personalSseManager.BroadcastToUser(targetUserID, sseEvent)
			}
		}(userID.UuidUserId, groups)
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

	sessionUUID, err := kit.GetConnectRpcSessionUUID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if req == nil || req.Msg == nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload"))
	}

	if err := validateMessageContentLength(req.Msg.Content); err != nil {
		return nil, kit.ParseIntoRpcError(err)
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

	var replyToUUID *uuid.UUID
	if req.Msg.ReplyToMessageId != nil && *req.Msg.ReplyToMessageId != "" {
		parsed, err := uuid.Parse(*req.Msg.ReplyToMessageId)
		if err != nil {
			return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "invalid_reply_to_message_id", "Invalid reply to message id"))
		}
		replyToUUID = &parsed
	}

	message, svcErr := s.chatService.ConfirmChatUpload(ctx, ConfirmChatUploadParams{
		MessageID:             messageID,
		SenderID:              userID,
		RecipientID:           recipientID,
		FileID:                req.Msg.FileId,
		Content:               req.Msg.Content,
		MessageType:           req.Msg.MessageType,
		IsPrimary:             isPrimary,
		SessionUUID:           sessionUUID,
		RecipientKeysRevision: req.Msg.RecipientKeysRevision,
		SenderKeysRevision:    req.Msg.SenderKeysRevision,
		ReplyToMessageID:      replyToUUID,
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
		ReplyToMessageId:   uuidPtrToStringPtr(message.ReplyToMessageID),
	}

	// SSE Broadcast: ConfirmFileMessageUploadSseEvent to the recipient and to EVERY sender stream
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

		go s.personalSseManager.BroadcastToUser(recipientUUID, recipientSseEvent)
		go s.personalSseManager.BroadcastToUser(userID.UuidUserId, senderSseEvent)
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

	// Tell the main device about the new request.
	// Send only the request id, not the big cipher: Postgres drops NOTIFY
	// messages bigger than 8,000 bytes, and the cipher is bigger than that.
	// The main device reads the full cipher with FetchHistorySync.
	if s.personalSseManager != nil && primarySessionID != uuid.Nil {
		sseEvent := &rpc_personal_ssev1.PersonalSseEvent{
			Timestamp: timestamppb.Now(),
			Payload: &rpc_personal_ssev1.PersonalSseEvent_ChatModule{
				ChatModule: &rpc_personal_chatv1.ChatSsePayload{
					Event: &rpc_personal_chatv1.ChatSsePayload_RequestHistorySyncSseEvent{
						RequestHistorySyncSseEvent: &rpc_personal_chatv1.RequestHistorySyncSsePayload{
							RequestId:          requestID.String(),
							RequesterPublicKey: requesterPubKey,
						},
					},
				},
			},
		}
		go s.personalSseManager.BroadcastToUserSession(userID.UuidUserId, primarySessionID, sseEvent, false)
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

	if len(req.Msg.PayloadCipher) > 94371840 { // 90MB limit for database cipher sync
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusRequestEntityTooLarge, "payload_too_large", "history sync payload exceeds 90MB limit"))
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
		go s.personalSseManager.BroadcastToUserSession(userID.UuidUserId, requesterSessionID, sseEvent, false)
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

func (s *chatConnectServer) FetchHistorySync(ctx context.Context, req *connect.Request[rpc_personal_chatv1.FetchHistorySyncRequest]) (*connect.Response[rpc_personal_chatv1.FetchHistorySyncResponse], error) {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	isPrimary, err := kit.GetConnectRpcIsPrimary(ctx)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	if !isPrimary {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusForbidden, "forbidden", "Only primary device can fetch history sync requests"))
	}

	if req == nil || req.Msg == nil || req.Msg.RequestId == "" {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "missing or invalid request_id"))
	}

	requestID, err := uuid.Parse(req.Msg.RequestId)
	if err != nil {
		return nil, kit.ParseIntoRpcError(kit.NewError(http.StatusBadRequest, "bad_request", "missing or invalid request_id"))
	}

	chatsCipher, requesterPubKey, svcErr := s.chatService.FetchHistorySync(ctx, userID.UuidUserId, requestID)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	return connect.NewResponse(&rpc_personal_chatv1.FetchHistorySyncResponse{
		ChatsCipher:        chatsCipher,
		RequesterPublicKey: requesterPubKey,
	}), nil
}

func (s *chatConnectServer) AcknowledgeHistorySync(ctx context.Context, req *connect.Request[rpc_personal_chatv1.AcknowledgeHistorySyncRequest]) (*connect.Response[rpc_common_modelv1.StatusOkay], error) {
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

	if svcErr := s.chatService.AcknowledgeHistorySync(ctx, userID.UuidUserId, sessionUUIDVal, requestID); svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	return connect.NewResponse(&rpc_common_modelv1.StatusOkay{Status: true}), nil
}
