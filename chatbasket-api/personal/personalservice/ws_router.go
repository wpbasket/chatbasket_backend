package personalservice

import (
	"chatbasket-api/model"
	personalmodel "chatbasket-api/personal/personalmodel"
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
)

// WSRouter handles client→server WebSocket messages
type WSRouter struct {
	service *Service
	hub     *WSHub
}

// NewWSRouter creates a new WebSocket message router
func NewWSRouter(service *Service, hub *WSHub) *WSRouter {
	return &WSRouter{
		service: service,
		hub:     hub,
	}
}

// HandleMessage is the main dispatcher for client→server WS messages
func (r *WSRouter) HandleMessage(ctx context.Context, conn *WSConn, event WSClientEvent) WSResponseEvent {
	log.Printf("[WS Router] Handling message type=%s ref=%s from user=%s", event.Type, event.Ref, conn.UserID)

	var payload any
	var err *WSError

	switch event.Type {
	case "send_message":
		payload, err = r.handleSendMessage(ctx, conn, event.Payload)
	case "ack_delivery":
		payload, err = r.handleAckDelivery(ctx, conn, event.Payload)
	case "ack_delivery_batch":
		payload, err = r.handleAckDeliveryBatch(ctx, conn, event.Payload)
	case "mark_read":
		payload, err = r.handleMarkRead(ctx, conn, event.Payload)
	case "unsend":
		payload, err = r.handleUnsend(ctx, conn, event.Payload)
	case "delete_for_me":
		payload, err = r.handleDeleteForMe(ctx, conn, event.Payload)
	case "ack_sync_action":
		payload, err = r.handleAckSyncAction(ctx, conn, event.Payload)
	default:
		err = &WSError{Code: 400, Message: "Unknown message type: " + event.Type}
	}

	return WSResponseEvent{
		Type:    event.Type + "_response",
		Ref:     event.Ref,
		Payload: payload,
		Error:   err,
	}
}

func (r *WSRouter) handleSendMessage(ctx context.Context, conn *WSConn, rawPayload json.RawMessage) (any, *WSError) {
	var payload personalmodel.SendMessagePayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &WSError{Code: 400, Message: "Invalid payload: " + err.Error()}
	}

	userId := model.UserId{
		StringUserId: conn.UserID.String(),
		UuidUserId:   conn.UserID,
	}

	resp, apiErr := r.service.SendMessageHandler(ctx, &payload, userId, conn.IsPrimary)
	if apiErr != nil {
		return nil, &WSError{Code: apiErr.Code, Message: apiErr.Message}
	}

	// WS Broadcast: new_message to recipient and sender's other devices
	if r.hub != nil {
		recipientUUID, _ := uuid.Parse(resp.RecipientID)

		// To recipient: is_from_me = false
		recipientPayload := *resp
		recipientPayload.IsFromMe = false
		go r.hub.BroadcastToUser(recipientUUID, WSEvent{
			Type:    WSEventNewMessage,
			Payload: recipientPayload,
		})

		// To sender's other devices: is_from_me = true
		go r.hub.BroadcastToUserExcept(conn.UserID, conn.SessionID, WSEvent{
			Type:    WSEventNewMessage,
			Payload: resp,
		})
	}

	return resp, nil
}

func (r *WSRouter) handleAckDelivery(ctx context.Context, conn *WSConn, rawPayload json.RawMessage) (any, *WSError) {
	var payload personalmodel.AcknowledgeDeliveryPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &WSError{Code: 400, Message: "Invalid payload: " + err.Error()}
	}

	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		return nil, &WSError{Code: 400, Message: "Invalid message_id"}
	}

	// 1. Fetch message info BEFORE acknowledgment (it might be deleted from relay during ACK)
	message, msgErr := r.service.PersonalQueries.GetMessageByID(ctx, messageID)
	if msgErr != nil {
		log.Printf("[WS Router] handleAckDelivery: Message %s not found in relay, skipping broadcast", messageID)
	}

	apiErr := r.service.AcknowledgeDelivery(ctx, messageID, payload.AcknowledgedBy, conn.SessionID, conn.UserID)
	if apiErr != nil {
		return nil, &WSError{Code: apiErr.Code, Message: apiErr.Message}
	}

	// 2. WS Broadcast: delivery_ack to sender (if acknowledged_by == "recipient")
	if payload.AcknowledgedBy == "recipient" && r.hub != nil && msgErr == nil {
		log.Printf("[WS Router] handleAckDelivery: Broadcasting delivery_ack for msg %s to sender %s", messageID, message.SenderID)
		go r.hub.BroadcastToUser(message.SenderID, WSEvent{
			Type: WSEventDeliveryAck,
			Payload: personalmodel.DeliveryAckEventPayload{
				ChatID:      message.ChatID.String(),
				MessageIDs:  []string{messageID.String()},
				DeliveredAt: message.CreatedAt.Time, // This matches the chat-level metadata update
			},
		})
	}

	return personalmodel.AcknowledgeDeliveryResponse{Acknowledged: true}, nil
}

func (r *WSRouter) handleAckDeliveryBatch(ctx context.Context, conn *WSConn, rawPayload json.RawMessage) (any, *WSError) {
	var payload personalmodel.AckDeliveryBatchPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &WSError{Code: 400, Message: "Invalid payload: " + err.Error()}
	}

	var chatID uuid.UUID
	var senderID uuid.UUID
	var latestCreatedAt time.Time
	acknowledgedCount := 0

	// 1. Fetch info and perform acknowledgments
	for i, msgIDStr := range payload.MessageIDs {
		messageID, err := uuid.Parse(msgIDStr)
		if err != nil {
			continue
		}

		// Fetch message info BEFORE it's potentially deleted
		if msg, msgErr := r.service.PersonalQueries.GetMessageByID(ctx, messageID); msgErr == nil {
			if i == 0 {
				chatID = msg.ChatID
				senderID = msg.SenderID
			}
			if msg.CreatedAt.Time.After(latestCreatedAt) {
				latestCreatedAt = msg.CreatedAt.Time
			}
		}

		apiErr := r.service.AcknowledgeDelivery(ctx, messageID, payload.AcknowledgedBy, conn.SessionID, conn.UserID)
		if apiErr == nil {
			acknowledgedCount++
		}
	}

	// 2. WS Broadcast: single delivery_ack with all message_ids
	if payload.AcknowledgedBy == "recipient" && r.hub != nil && chatID != uuid.Nil && senderID != uuid.Nil {
		if latestCreatedAt.IsZero() {
			latestCreatedAt = time.Now()
		}
		log.Printf("[WS Router] handleAckDeliveryBatch: Broadcasting batch delivery_ack (%d msgs) to sender %s, delivered_at=%v", acknowledgedCount, senderID, latestCreatedAt)
		go r.hub.BroadcastToUser(senderID, WSEvent{
			Type: WSEventDeliveryAck,
			Payload: personalmodel.DeliveryAckEventPayload{
				ChatID:      chatID.String(),
				MessageIDs:  payload.MessageIDs,
				DeliveredAt: latestCreatedAt,
			},
		})
	}

	return personalmodel.AckDeliveryBatchResponse{AcknowledgedCount: acknowledgedCount}, nil
}

func (r *WSRouter) handleMarkRead(ctx context.Context, conn *WSConn, rawPayload json.RawMessage) (any, *WSError) {
	var payload personalmodel.MarkChatReadPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &WSError{Code: 400, Message: "Invalid payload: " + err.Error()}
	}

	userId := model.UserId{
		StringUserId: conn.UserID.String(),
		UuidUserId:   conn.UserID,
	}

	apiErr := r.service.MarkChatReadHandler(ctx, &payload, userId, conn.IsPrimary)
	if apiErr != nil {
		return nil, &WSError{Code: apiErr.Code, Message: apiErr.Message}
	}

	// WS Broadcast: read_receipt to other participant
	if r.hub != nil {
		chatUUID, _ := uuid.Parse(payload.ChatID)
		chat, err := r.service.PersonalQueries.GetChatByID(ctx, chatUUID)
		if err == nil {
			var otherUserID uuid.UUID
			if chat.Participant1ID == conn.UserID {
				otherUserID = chat.Participant2ID
			} else {
				otherUserID = chat.Participant1ID
			}

			go r.hub.BroadcastToUser(otherUserID, WSEvent{
				Type: WSEventReadReceipt,
				Payload: personalmodel.ReadReceiptEventPayload{
					ChatID:   payload.ChatID,
					ReaderID: conn.UserID.String(),
					ReadAt:   chat.UpdatedAt.Time.Format("2006-01-02T15:04:05.999999999Z07:00"),
				},
			})
		}
	}

	return model.StatusOkay{Status: true, Message: "success"}, nil
}

func (r *WSRouter) handleUnsend(ctx context.Context, conn *WSConn, rawPayload json.RawMessage) (any, *WSError) {
	var payload personalmodel.UnsendMessagePayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &WSError{Code: 400, Message: "Invalid payload: " + err.Error()}
	}

	userId := model.UserId{
		StringUserId: conn.UserID.String(),
		UuidUserId:   conn.UserID,
	}

	apiErr := r.service.UnsendMessageHandler(ctx, &payload, userId, conn.IsPrimary)
	if apiErr != nil {
		return nil, &WSError{Code: apiErr.Code, Message: apiErr.Message}
	}

	// WS Broadcast: unsend to recipient and sender's other devices
	if r.hub != nil {
		chatUUID, _ := uuid.Parse(payload.ChatID)
		chat, err := r.service.PersonalQueries.GetChatByID(ctx, chatUUID)
		if err == nil {
			var recipientID uuid.UUID
			if chat.Participant1ID == conn.UserID {
				recipientID = chat.Participant2ID
			} else {
				recipientID = chat.Participant1ID
			}

			unsendEvent := WSEvent{
				Type: WSEventUnsend,
				Payload: personalmodel.UnsendEventPayload{
					ChatID:     payload.ChatID,
					MessageIDs: payload.MessageIDs,
					SenderID:   conn.UserID.String(),
				},
			}

			go r.hub.BroadcastToUser(recipientID, unsendEvent)
			go r.hub.BroadcastToUserExcept(conn.UserID, conn.SessionID, unsendEvent)
		}
	}

	return model.StatusOkay{Status: true, Message: "success"}, nil
}

func (r *WSRouter) handleDeleteForMe(ctx context.Context, conn *WSConn, rawPayload json.RawMessage) (any, *WSError) {
	var payload personalmodel.DeleteMessageForMePayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &WSError{Code: 400, Message: "Invalid payload: " + err.Error()}
	}

	userId := model.UserId{
		StringUserId: conn.UserID.String(),
		UuidUserId:   conn.UserID,
	}

	apiErr := r.service.DeleteMessageForMeHandler(ctx, &payload, userId, conn.IsPrimary)
	if apiErr != nil {
		return nil, &WSError{Code: apiErr.Code, Message: apiErr.Message}
	}

	// WS Broadcast: delete_for_me to sender's other devices
	if r.hub != nil {
		var chatID string
		if len(payload.MessageIDs) > 0 {
			if msgUUID, parseErr := uuid.Parse(payload.MessageIDs[0]); parseErr == nil {
				if msg, lookupErr := r.service.PersonalQueries.GetMessageByID(ctx, msgUUID); lookupErr == nil {
					chatID = msg.ChatID.String()
				}
			}
		}

		go r.hub.BroadcastToUserExcept(conn.UserID, conn.SessionID, WSEvent{
			Type: WSEventDeleteForMe,
			Payload: personalmodel.DeleteForMeEventPayload{
				MessageIDs: payload.MessageIDs,
				ChatID:     chatID,
			},
		})
	}

	return model.StatusOkay{Status: true, Message: "success"}, nil
}

func (r *WSRouter) handleAckSyncAction(ctx context.Context, conn *WSConn, rawPayload json.RawMessage) (any, *WSError) {
	var payload personalmodel.AcknowledgeSyncActionPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &WSError{Code: 400, Message: "Invalid payload: " + err.Error()}
	}

	apiErr := r.service.AcknowledgeSyncActionHandler(ctx, &payload, conn.IsPrimary)
	if apiErr != nil {
		return nil, &WSError{Code: apiErr.Code, Message: apiErr.Message}
	}

	return model.StatusOkay{Status: true, Message: "success"}, nil
}
