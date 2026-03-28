package personal_chat

import (
	"chatbasket-apinext/internal/platform/kit"
	"chatbasket-apinext/internal/platform/websocket"
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────────────────────────
// WS Router — dispatches client→server WebSocket messages
// ──────────────────────────────────────────────────────────────────────────────

type chatWSRouter struct {
	service *chatService
	hub     *websocket.WSHub
}

func NewChatWSRouter(service *chatService, hub *websocket.WSHub) *chatWSRouter {
	return &chatWSRouter{
		service: service,
		hub:     hub,
	}
}

// HandleRawMessage is the ReadPump callback. It deserializes the envelope, dispatches, and writes the response.
func (r *chatWSRouter) HandleRawMessage(ctx context.Context, conn *websocket.WSConn, data []byte) {
	var event websocket.WSClientEvent
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("[WS Router] Failed to parse client event: %v", err)
		return
	}

	response := r.handleMessage(ctx, conn, event)

	respBytes, err := json.Marshal(response)
	if err != nil {
		log.Printf("[WS Router] Failed to marshal response: %v", err)
		return
	}

	select {
	case conn.Send <- respBytes:
	default:
		log.Printf("[WS Router] Send buffer full for session %s, response dropped", conn.SessionID)
	}
}

// handleMessage is the main dispatcher for client→server WS messages
func (r *chatWSRouter) handleMessage(ctx context.Context, conn *websocket.WSConn, event websocket.WSClientEvent) websocket.WSResponseEvent {
	log.Printf("[WS Router] Handling message type=%s ref=%s from user=%s", event.Type, event.Ref, conn.UserID)

	var payload any
	var wsErr *websocket.WSError

	switch event.Type {
	case "send_message":
		payload, wsErr = r.handleSendMessage(ctx, conn, event.Payload)
	case "ack_delivery":
		payload, wsErr = r.handleAckDelivery(ctx, conn, event.Payload)
	case "ack_delivery_batch":
		payload, wsErr = r.handleAckDeliveryBatch(ctx, conn, event.Payload)
	case "mark_read":
		payload, wsErr = r.handleMarkRead(ctx, conn, event.Payload)
	case "unsend":
		payload, wsErr = r.handleUnsend(ctx, conn, event.Payload)
	case "delete_for_me":
		payload, wsErr = r.handleDeleteForMe(ctx, conn, event.Payload)
	case "ack_sync_action":
		payload, wsErr = r.handleAckSyncAction(ctx, conn, event.Payload)
	case "ping":
		payload = map[string]string{"type": "pong"}
	default:
		wsErr = &websocket.WSError{Code: 400, Type: "invalid_request", Message: "Unknown message type: " + event.Type}
	}

	return websocket.WSResponseEvent{
		Type:    event.Type + "_response",
		Ref:     event.Ref,
		Payload: payload,
		Error:   wsErr,
	}
}

// toWSError converts a kit.ProcessedError (or generic error) to a WSError
func toWSError(err error) *websocket.WSError {
	var pe kit.ProcessedError
	if errors.As(err, &pe) {
		return &websocket.WSError{Code: pe.Status(), Type: pe.Kind(), Message: pe.Error()}
	}
	return &websocket.WSError{Code: 500, Type: "internal_error", Message: err.Error()}
}

func (r *chatWSRouter) handleSendMessage(ctx context.Context, conn *websocket.WSConn, rawPayload json.RawMessage) (any, *websocket.WSError) {
	var payload SendMessagePayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &websocket.WSError{Code: 400, Type: "invalid_payload", Message: "Invalid payload: " + err.Error()}
	}

	resp, err := r.service.SendMessageHandler(ctx, &payload, conn.UserID, conn.IsPrimary)
	if err != nil {
		return nil, toWSError(err)
	}

	// WS Broadcast: new_message to recipient and sender's other devices
	if r.hub != nil {
		recipientUUID, _ := uuid.Parse(resp.RecipientID)

		// To recipient: is_from_me = false
		recipientPayload := *resp
		recipientPayload.IsFromMe = false
		go r.hub.BroadcastToUser(recipientUUID, websocket.WSEvent{
			Type:    WSEventNewMessage,
			Payload: recipientPayload,
		})

		// To sender's other devices: is_from_me = true
		go r.hub.BroadcastToUserExcept(conn.UserID, conn.SessionID, websocket.WSEvent{
			Type:    WSEventNewMessage,
			Payload: resp,
		})
	}

	return resp, nil
}

func (r *chatWSRouter) handleAckDelivery(ctx context.Context, conn *websocket.WSConn, rawPayload json.RawMessage) (any, *websocket.WSError) {
	var payload AcknowledgeDeliveryPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &websocket.WSError{Code: 400, Type: "invalid_payload", Message: "Invalid payload: " + err.Error()}
	}

	messageID, parseErr := uuid.Parse(payload.MessageID)
	if parseErr != nil {
		return nil, &websocket.WSError{Code: 400, Type: "invalid_payload", Message: "Invalid message_id"}
	}

	// 1. Fetch message info BEFORE acknowledgment (it might be deleted from relay during ACK)
	message, msgErr := r.service.PostgresQueries.GetMessageByID(ctx, messageID)
	if msgErr != nil {
		log.Printf("[WS Router] handleAckDelivery: Message %s not found in relay, skipping broadcast", messageID)
	}

	ackErr := r.service.AcknowledgeDelivery(ctx, messageID, payload.AcknowledgedBy, conn.SessionID, conn.UserID)
	if ackErr != nil {
		return nil, toWSError(ackErr)
	}

	// 2. WS Broadcast: delivery_ack to sender (if acknowledged_by == "recipient")
	if payload.AcknowledgedBy == "recipient" && r.hub != nil && msgErr == nil {
		log.Printf("[WS Router] handleAckDelivery: Broadcasting delivery_ack for msg %s to sender %s", messageID, message.SenderID)
		go r.hub.BroadcastToUser(message.SenderID, websocket.WSEvent{
			Type: WSEventDeliveryAck,
			Payload: DeliveryAckEventPayload{
				ChatID:      message.ChatID.String(),
				MessageIDs:  []string{messageID.String()},
				DeliveredAt: kit.DerefTime(message.CreatedAt),
			},
		})
	}

	return AcknowledgeDeliveryResponse{Acknowledged: true}, nil
}

func (r *chatWSRouter) handleAckDeliveryBatch(ctx context.Context, conn *websocket.WSConn, rawPayload json.RawMessage) (any, *websocket.WSError) {
	var payload AckDeliveryBatchPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &websocket.WSError{Code: 400, Type: "invalid_payload", Message: "Invalid payload: " + err.Error()}
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
		if msg, msgErr := r.service.PostgresQueries.GetMessageByID(ctx, messageID); msgErr == nil {
			if i == 0 {
				chatID = msg.ChatID
				senderID = msg.SenderID
			}
			if kit.DerefTime(msg.CreatedAt).After(latestCreatedAt) {
				latestCreatedAt = kit.DerefTime(msg.CreatedAt)
			}
		}

		ackErr := r.service.AcknowledgeDelivery(ctx, messageID, payload.AcknowledgedBy, conn.SessionID, conn.UserID)
		if ackErr == nil {
			acknowledgedCount++
		}
	}

	// 2. WS Broadcast: single delivery_ack with all message_ids
	if payload.AcknowledgedBy == "recipient" && r.hub != nil && chatID != uuid.Nil && senderID != uuid.Nil {
		if latestCreatedAt.IsZero() {
			latestCreatedAt = time.Now()
		}
		log.Printf("[WS Router] handleAckDeliveryBatch: Broadcasting batch delivery_ack (%d msgs) to sender %s, delivered_at=%v", acknowledgedCount, senderID, latestCreatedAt)
		go r.hub.BroadcastToUser(senderID, websocket.WSEvent{
			Type: WSEventDeliveryAck,
			Payload: DeliveryAckEventPayload{
				ChatID:      chatID.String(),
				MessageIDs:  payload.MessageIDs,
				DeliveredAt: latestCreatedAt,
			},
		})
	}

	return AckDeliveryBatchResponse{AcknowledgedCount: acknowledgedCount}, nil
}

func (r *chatWSRouter) handleMarkRead(ctx context.Context, conn *websocket.WSConn, rawPayload json.RawMessage) (any, *websocket.WSError) {
	var payload MarkChatReadPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &websocket.WSError{Code: 400, Type: "invalid_payload", Message: "Invalid payload: " + err.Error()}
	}

	svcErr := r.service.MarkChatReadHandler(ctx, &payload, conn.UserID, conn.IsPrimary)
	if svcErr != nil {
		return nil, toWSError(svcErr)
	}

	// WS Broadcast: read_receipt to other participant
	if r.hub != nil {
		chatUUID, _ := uuid.Parse(payload.ChatID)
		chat, err := r.service.PostgresQueries.GetChatByID(ctx, chatUUID)
		if err == nil {
			var otherUserID uuid.UUID
			if chat.Participant1ID == conn.UserID {
				otherUserID = chat.Participant2ID
			} else {
				otherUserID = chat.Participant1ID
			}

			go r.hub.BroadcastToUser(otherUserID, websocket.WSEvent{
				Type: WSEventReadReceipt,
				Payload: ReadReceiptEventPayload{
					ChatID:   payload.ChatID,
					ReaderID: conn.UserID.String(),
					ReadAt:   chat.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
				},
			})
		}
	}

	return kit.StatusOkay{Status: true, Message: "success"}, nil
}

func (r *chatWSRouter) handleUnsend(ctx context.Context, conn *websocket.WSConn, rawPayload json.RawMessage) (any, *websocket.WSError) {
	var payload UnsendMessagePayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &websocket.WSError{Code: 400, Type: "invalid_payload", Message: "Invalid payload: " + err.Error()}
	}

	svcErr := r.service.UnsendMessageHandler(ctx, &payload, conn.UserID, conn.IsPrimary)
	if svcErr != nil {
		return nil, toWSError(svcErr)
	}

	// WS Broadcast: unsend to recipient and sender's other devices
	if r.hub != nil {
		chatUUID, _ := uuid.Parse(payload.ChatID)
		chat, err := r.service.PostgresQueries.GetChatByID(ctx, chatUUID)
		if err == nil {
			var recipientID uuid.UUID
			if chat.Participant1ID == conn.UserID {
				recipientID = chat.Participant2ID
			} else {
				recipientID = chat.Participant1ID
			}

			unsendEvent := websocket.WSEvent{
				Type: WSEventUnsend,
				Payload: UnsendEventPayload{
					ChatID:     payload.ChatID,
					MessageIDs: payload.MessageIDs,
					SenderID:   conn.UserID.String(),
				},
			}

			go r.hub.BroadcastToUser(recipientID, unsendEvent)
			go r.hub.BroadcastToUserExcept(conn.UserID, conn.SessionID, unsendEvent)
		}
	}

	return kit.StatusOkay{Status: true, Message: "success"}, nil
}

func (r *chatWSRouter) handleDeleteForMe(ctx context.Context, conn *websocket.WSConn, rawPayload json.RawMessage) (any, *websocket.WSError) {
	var payload DeleteMessageForMePayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &websocket.WSError{Code: 400, Type: "invalid_payload", Message: "Invalid payload: " + err.Error()}
	}

	svcErr := r.service.DeleteMessageForMeHandler(ctx, &payload, conn.UserID, conn.IsPrimary)
	if svcErr != nil {
		return nil, toWSError(svcErr)
	}

	// WS Broadcast: delete_for_me to sender's other devices
	if r.hub != nil {
		var chatID string
		if len(payload.MessageIDs) > 0 {
			if msgUUID, parseErr := uuid.Parse(payload.MessageIDs[0]); parseErr == nil {
				if msg, lookupErr := r.service.PostgresQueries.GetMessageByID(ctx, msgUUID); lookupErr == nil {
					chatID = msg.ChatID.String()
				}
			}
		}

		go r.hub.BroadcastToUserExcept(conn.UserID, conn.SessionID, websocket.WSEvent{
			Type: WSEventDeleteForMe,
			Payload: DeleteForMeEventPayload{
				MessageIDs: payload.MessageIDs,
				ChatID:     chatID,
			},
		})
	}

	return kit.StatusOkay{Status: true, Message: "success"}, nil
}

func (r *chatWSRouter) handleAckSyncAction(ctx context.Context, conn *websocket.WSConn, rawPayload json.RawMessage) (any, *websocket.WSError) {
	var payload AcknowledgeSyncActionPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, &websocket.WSError{Code: 400, Type: "invalid_payload", Message: "Invalid payload: " + err.Error()}
	}

	svcErr := r.service.AcknowledgeSyncActionHandler(ctx, &payload, conn.IsPrimary)
	if svcErr != nil {
		return nil, toWSError(svcErr)
	}

	return kit.StatusOkay{Status: true, Message: "success"}, nil
}
