package personal_chat

import (
	"context"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	"net/http"
	"time"

	rpc_common_modelv1 "chatbasket-api/gen/proto/common/model"
	rpc_personal_chatv1 "chatbasket-api/gen/proto/personal/personal_chat"
	rpc_personal_chatv1connect "chatbasket-api/gen/proto/personal/personal_chat/rpc_personal_chatv1connect"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/websocket"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

type chatConnectServer struct {
	rpc_personal_chatv1connect.UnimplementedChatServiceHandler
	chatService *chatService
	hub         *websocket.WSHub
}

func newChatConnectServer(service *chatService, hub *websocket.WSHub) rpc_personal_chatv1connect.ChatServiceHandler {
	return &chatConnectServer{
		chatService: service,
		hub:         hub,
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

	// ——— WS Broadcast: new_message ————————————————————————————————————————————————————————————————
	if s.hub != nil {
		sessionID, _ := kit.GetConnectRpcSessionID(ctx)
		recipientUUID, _ := uuid.Parse(res.RecipientId)

		log.Printf("[WS Broadcast] SendMessage: msgID=%s chatID=%s sender=%s recipient=%s sessionId=%s",
			res.MessageId, res.ChatId, userID.StringUserId, res.RecipientId, sessionID)

		// For recipient: is_from_me = false
		recipientPayload := proto.Clone(res).(*rpc_personal_chatv1.Message)
		recipientPayload.IsFromMe = false
		log.Printf("[WS Broadcast] SendMessage: pushing new_message to RECIPIENT=%s (is_from_me=false)", recipientUUID)
		go s.hub.BroadcastToUser(recipientUUID, websocket.WSEvent{
			Type:    WSEventNewMessage,
			Payload: recipientPayload,
		})

		// For sender's OTHER devices: is_from_me = true (sync)
		log.Printf("[WS Broadcast] SendMessage: pushing new_message to SENDER=%s other devices (excluding session=%s)",
			userID.StringUserId, sessionID)
		go s.hub.BroadcastToUserExcept(userID.UuidUserId, sessionID, websocket.WSEvent{
			Type:    WSEventNewMessage,
			Payload: res,
		})
	} else {
		log.Printf("[WS Broadcast] SendMessage: WSHub is NIL, skipping broadcast")
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

	res, err := s.chatService.AcknowledgeDeliveryHandler(ctx, &AcknowledgeDeliveryPayload{
		MessageID:      req.Msg.MessageId,
		AcknowledgedBy: req.Msg.AcknowledgedBy,
		Success:        req.Msg.Success,
	}, userID, sessionID)
	if err != nil {
		return nil, kit.ParseIntoRpcError(err)
	}

	// ——— WS Broadcast: delivery_ack ————————————————————————————————————————————————————————————————
	if s.hub != nil && res.Acknowledged && req.Msg.AcknowledgedBy == "recipient" {
		log.Printf("[WS Broadcast] AckDelivery: msgID=%s acknowledged_by=%s user=%s → looking up sender",
			req.Msg.MessageId, req.Msg.AcknowledgedBy, userID.StringUserId)
		msgUUID, parseErr := uuid.Parse(req.Msg.MessageId)
		if parseErr == nil {
			msg, lookupErr := s.chatService.PostgresQueries.GetMessageByID(ctx, msgUUID)
			if lookupErr == nil {
				log.Printf("[WS Broadcast] AckDelivery: pushing delivery_ack to SENDER=%s for msgID=%s chatID=%s",
					msg.SenderID, req.Msg.MessageId, msg.ChatID)
				go s.hub.BroadcastToUser(msg.SenderID, websocket.WSEvent{
					Type: WSEventDeliveryAck,
					Payload: DeliveryAckEventPayload{
						MessageIDs: []string{req.Msg.MessageId},
						ChatID:     msg.ChatID.String(),
					},
				})
			} else {
				log.Printf("[WS Broadcast] AckDelivery: GetMessageByID FAILED for msgID=%s: %v", req.Msg.MessageId, lookupErr)
			}
		} else {
			log.Printf("[WS Broadcast] AckDelivery: parse msgID FAILED: %s → %v", req.Msg.MessageId, parseErr)
		}
	} else if s.hub != nil {
		log.Printf("[WS Broadcast] AckDelivery: SKIPPED (acknowledged=%v, acknowledged_by=%s)",
			res.Acknowledged, req.Msg.AcknowledgedBy)
	}

	return connect.NewResponse(res), nil
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

	// ——— WS Broadcast: read_receipt ————————————————————————————————————————————————————————————————
	if s.hub != nil {
		log.Printf("[WS Broadcast] MarkChatRead: chatID=%s reader=%s → looking up other participant",
			req.Msg.ChatId, userID.StringUserId)
		chatUUID, _ := uuid.Parse(req.Msg.ChatId)
		chat, chatErr := s.chatService.PostgresQueries.GetChatByID(ctx, chatUUID)
		if chatErr == nil {
			var otherUserID uuid.UUID
			if chat.Participant1ID == userID.UuidUserId {
				otherUserID = chat.Participant2ID
			} else {
				otherUserID = chat.Participant1ID
			}

			readAt := time.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
			log.Printf("[WS Broadcast] MarkChatRead: pushing read_receipt to OTHER_USER=%s for chatID=%s read_at=%s",
				otherUserID, req.Msg.ChatId, readAt)
			go s.hub.BroadcastToUser(otherUserID, websocket.WSEvent{
				Type: WSEventReadReceipt,
				Payload: ReadReceiptEventPayload{
					ChatID:   req.Msg.ChatId,
					ReaderID: userID.StringUserId,
					ReadAt:   readAt,
				},
			})
		} else {
			log.Printf("[WS Broadcast] MarkChatRead: GetChatByID FAILED for chatID=%s: %v", req.Msg.ChatId, chatErr)
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

	// ——— WS Broadcast: unsend —————————————————————————————————————————————————————————————————————
	if s.hub != nil {
		log.Printf("[WS Broadcast] Unsend: chatID=%s sender=%s messageIDs=%v → looking up recipient",
			req.Msg.ChatId, userID.StringUserId, req.Msg.MessageIds)
		chatUUID, _ := uuid.Parse(req.Msg.ChatId)
		chat, chatErr := s.chatService.PostgresQueries.GetChatByID(ctx, chatUUID)
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
					ChatID:     req.Msg.ChatId,
					MessageIDs: req.Msg.MessageIds,
					SenderID:   userID.StringUserId,
				},
			}

			// Notify recipient (all devices)
			log.Printf("[WS Broadcast] Unsend: pushing unsend to RECIPIENT=%s", recipientID)
			go s.hub.BroadcastToUser(recipientID, unsendEvent)

			// Notify sender's other devices (for sync)
			sessionID, _ := kit.GetConnectRpcSessionID(ctx)
			log.Printf("[WS Broadcast] Unsend: pushing unsend to SENDER=%s other devices (excluding session=%s)",
				userID.StringUserId, sessionID)
			go s.hub.BroadcastToUserExcept(userID.UuidUserId, sessionID, unsendEvent)
		} else {
			log.Printf("[WS Broadcast] Unsend: GetChatByID FAILED for chatID=%s: %v", req.Msg.ChatId, chatErr)
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

	// ——— WS Broadcast: delete_for_me ——————————————————————————————————————————————————————————————
	if s.hub != nil {
		sessionID, _ := kit.GetConnectRpcSessionID(ctx)

		// Resolve chat_id from the first message so the frontend can clear the preview
		var chatID string
		if len(req.Msg.MessageIds) > 0 {
			if msgUUID, parseErr := uuid.Parse(req.Msg.MessageIds[0]); parseErr == nil {
				if msg, lookupErr := s.chatService.PostgresQueries.GetMessageByID(ctx, msgUUID); lookupErr == nil {
					chatID = msg.ChatID.String()
				}
			}
		}

		log.Printf("[WS Broadcast] DeleteForMe: user=%s messageIDs=%v chatID=%s → pushing to other devices (excluding session=%s)",
			userID.StringUserId, req.Msg.MessageIds, chatID, sessionID)
		go s.hub.BroadcastToUserExcept(userID.UuidUserId, sessionID, websocket.WSEvent{
			Type: WSEventDeleteForMe,
			Payload: DeleteForMeEventPayload{
				MessageIDs: req.Msg.MessageIds,
				ChatID:     chatID,
			},
		})
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

	if s.hub != nil {
		sessionID, _ := kit.GetConnectRpcSessionID(ctx)
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
		go s.hub.BroadcastToUser(recipientUUID, websocket.WSEvent{Type: WSEventNewMessage, Payload: recipientPayload})
		go s.hub.BroadcastToUserExcept(userID.UuidUserId, sessionID, websocket.WSEvent{Type: WSEventNewMessage, Payload: protoMsg})
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
		func(uid uuid.UUID, sid uuid.UUID) bool {
			return s.hub.IsSessionActive(uid, sid)
		},
	)
	if svcErr != nil {
		return nil, kit.ParseIntoRpcError(svcErr)
	}

	if primarySessionID != uuid.Nil {
		wsPayload := HistorySyncRequestedPayload{
			RequestID:          requestID,
			RequesterSessionID: sessionUUIDVal,
			RequesterPublicKey: requesterPubKey,
			ChatsCipher:        req.Msg.ChatsCipher,
		}
		go s.hub.BroadcastToUserSession(userID.UuidUserId, primarySessionID.String(), websocket.WSEvent{
			Type:    WSEventHistorySyncRequested,
			Payload: wsPayload,
		})
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

	// Notify the requesting secondary device that it's ready
	wsPayload := HistorySyncReadyPayload{
		RequestID: requestID,
	}
	go s.hub.BroadcastToUserSession(userID.UuidUserId, requesterSessionID.String(), websocket.WSEvent{
		Type:    WSEventHistorySyncReady,
		Payload: wsPayload,
	})

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
