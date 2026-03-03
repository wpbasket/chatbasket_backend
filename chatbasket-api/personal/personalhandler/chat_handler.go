package personalhandler

import (
	"chatbasket-api/model"
	personalmodel "chatbasket-api/personal/personalmodel"
	"chatbasket-api/personal/personalservice"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type ChatHandler struct {
	service *personalservice.Service
}

func (h *ChatHandler) CheckEligibility(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.CheckEligibilityPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.CheckEligibilityHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func NewChatHandler(service *personalservice.Service) *ChatHandler {
	return &ChatHandler{service: service}
}

func (h *ChatHandler) CreateChat(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.CreateChatPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.CreateChatHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) SendMessage(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.SendMessagePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.SendMessageHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, c.Get("isPrimary").(bool))
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	// ── WS Broadcast: new_message ────────────────────────────────────────
	if hub := h.service.WSHub; hub != nil {
		sessionId, _ := c.Get("sessionId").(string)
		recipientUUID, _ := uuid.Parse(resp.RecipientID)

		log.Printf("[WS Broadcast] SendMessage: msgID=%s chatID=%s sender=%s recipient=%s sessionId=%s",
			resp.MessageID, resp.ChatID, uuidUserId, resp.RecipientID, sessionId)

		// Build the event payload — reuse the same MessageResponse the REST client gets.
		// For recipient: is_from_me = false
		recipientPayload := *resp
		recipientPayload.IsFromMe = false
		log.Printf("[WS Broadcast] SendMessage: pushing new_message to RECIPIENT=%s (is_from_me=false)", recipientUUID)
		go hub.BroadcastToUser(recipientUUID, personalservice.WSEvent{
			Type:    personalservice.WSEventNewMessage,
			Payload: recipientPayload,
		})

		// For sender's OTHER devices: is_from_me = true (sync)
		log.Printf("[WS Broadcast] SendMessage: pushing new_message to SENDER=%s other devices (excluding session=%s)",
			uuidUserId, sessionId)
		go hub.BroadcastToUserExcept(uuidUserId, sessionId, personalservice.WSEvent{
			Type:    personalservice.WSEventNewMessage,
			Payload: resp,
		})
	} else {
		log.Printf("[WS Broadcast] SendMessage: WSHub is NIL, skipping broadcast")
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) GetMessages(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.GetMessagesPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.GetMessagesHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) AcknowledgeDelivery(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.AcknowledgeDeliveryPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	sessionId, _ := c.Get("sessionId").(string)

	resp, apiErr := h.service.AcknowledgeDeliveryHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, sessionId)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	// ── WS Broadcast: delivery_ack ───────────────────────────────────────
	// Notify the message SENDER that their message has been delivered.
	// Use message_ids array for consistency with batch ACK.
	if hub := h.service.WSHub; hub != nil && resp.Acknowledged && payload.AcknowledgedBy == "recipient" {
		log.Printf("[WS Broadcast] AckDelivery: msgID=%s acknowledged_by=%s user=%s → looking up sender",
			payload.MessageID, payload.AcknowledgedBy, uuidUserId)
		msgUUID, parseErr := uuid.Parse(payload.MessageID)
		if parseErr == nil {
			msg, lookupErr := h.service.PersonalQueries.GetMessageByID(c.Request().Context(), msgUUID)
			if lookupErr == nil {
				log.Printf("[WS Broadcast] AckDelivery: pushing delivery_ack to SENDER=%s for msgID=%s chatID=%s",
					msg.SenderID, payload.MessageID, msg.ChatID)
				go hub.BroadcastToUser(msg.SenderID, personalservice.WSEvent{
					Type: personalservice.WSEventDeliveryAck,
					Payload: personalmodel.DeliveryAckEventPayload{
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
	} else if hub := h.service.WSHub; hub != nil {
		log.Printf("[WS Broadcast] AckDelivery: SKIPPED (acknowledged=%v, acknowledged_by=%s)",
			resp.Acknowledged, payload.AcknowledgedBy)
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) GetUserChats(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	resp, apiErr := h.service.GetUserChatsHandler(c.Request().Context(), model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		log.Printf("[ChatHandler] GetUserChats failed for user %s: %v", userId, apiErr)
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) GetPendingMessages(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.GetPendingMessagesPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.GetPendingMessagesHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) UploadFileForMessage(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	log.Printf("[ChatHandler] UploadFileForMessage received from user: %s", userId)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	resp, msgInfo, apiErr := h.service.UploadFileForMessageHandler(c.Request().Context(), c, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, c.Get("isPrimary").(bool))
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	// ── WS Broadcast: new_message (File Upload) ────────────────────────
	if hub := h.service.WSHub; hub != nil && msgInfo != nil {
		sessionId, _ := c.Get("sessionId").(string)
		recipientUUID, _ := uuid.Parse(msgInfo.RecipientID)

		log.Printf("[WS Broadcast] UploadFileForMessage: msgID=%s chatID=%s sender=%s recipient=%s sessionId=%s",
			msgInfo.MessageID, msgInfo.ChatID, uuidUserId, msgInfo.RecipientID, sessionId)

		// For recipient: is_from_me = false
		recipientPayload := *msgInfo
		recipientPayload.IsFromMe = false
		log.Printf("[WS Broadcast] UploadFileForMessage: pushing new_message to RECIPIENT=%s", recipientUUID)
		go hub.BroadcastToUser(recipientUUID, personalservice.WSEvent{
			Type:    personalservice.WSEventNewMessage,
			Payload: recipientPayload,
		})

		// For sender's OTHER devices: is_from_me = true
		log.Printf("[WS Broadcast] UploadFileForMessage: pushing new_message to SENDER=%s other devices", uuidUserId)
		go hub.BroadcastToUserExcept(uuidUserId, sessionId, personalservice.WSEvent{
			Type:    personalservice.WSEventNewMessage,
			Payload: msgInfo,
		})
	} else {
		log.Printf("[WS Broadcast] UploadFileForMessage: WSHub is NIL or msgInfo missing, skipping broadcast")
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) GetFileURL(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.GetFileURLPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.GetFileURLHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) MarkChatRead(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.MarkChatReadPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	isPrimary, ok := c.Get("isPrimary").(bool)
	if !ok {
		isPrimary = false
	}

	apiErr := h.service.MarkChatReadHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, isPrimary)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	// ── WS Broadcast: read_receipt ───────────────────────────────────────
	// Notify the OTHER participant that this user has read the chat.
	// The sender's client uses other_user_last_read_at to flip Yellow → Green tick.
	if hub := h.service.WSHub; hub != nil {
		log.Printf("[WS Broadcast] MarkChatRead: chatID=%s reader=%s → looking up other participant",
			payload.ChatID, uuidUserId)
		chatUUID, _ := uuid.Parse(payload.ChatID)
		chat, err := h.service.PersonalQueries.GetChatByID(c.Request().Context(), chatUUID)
		if err == nil {
			var otherUserID uuid.UUID
			if chat.Participant1ID == uuidUserId {
				otherUserID = chat.Participant2ID
			} else {
				otherUserID = chat.Participant1ID
			}

			readAt := time.Now().UTC().Format(time.RFC3339Nano)
			log.Printf("[WS Broadcast] MarkChatRead: pushing read_receipt to OTHER_USER=%s for chatID=%s read_at=%s",
				otherUserID, payload.ChatID, readAt)
			go hub.BroadcastToUser(otherUserID, personalservice.WSEvent{
				Type: personalservice.WSEventReadReceipt,
				Payload: personalmodel.ReadReceiptEventPayload{
					ChatID:   payload.ChatID,
					ReaderID: uuidUserId.String(),
					ReadAt:   readAt,
				},
			})
		} else {
			log.Printf("[WS Broadcast] MarkChatRead: GetChatByID FAILED for chatID=%s: %v", payload.ChatID, err)
		}
	}

	return c.JSON(http.StatusOK, model.StatusOkay{Status: true, Message: "success"})
}

func (h *ChatHandler) UnsendMessage(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.UnsendMessagePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	isPrimary, ok := c.Get("isPrimary").(bool)
	if !ok {
		// Default to false if missing (safe fallback) or handle error
		isPrimary = false
	}

	apiErr := h.service.UnsendMessageHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, isPrimary)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	// ── WS Broadcast: unsend ─────────────────────────────────────────────
	// Notify both participants that messages were unsent.
	if hub := h.service.WSHub; hub != nil {
		log.Printf("[WS Broadcast] Unsend: chatID=%s sender=%s messageIDs=%v → looking up recipient",
			payload.ChatID, uuidUserId, payload.MessageIDs)
		chatUUID, _ := uuid.Parse(payload.ChatID)
		chat, err := h.service.PersonalQueries.GetChatByID(c.Request().Context(), chatUUID)
		if err == nil {
			var recipientID uuid.UUID
			if chat.Participant1ID == uuidUserId {
				recipientID = chat.Participant2ID
			} else {
				recipientID = chat.Participant1ID
			}

			unsendEvent := personalservice.WSEvent{
				Type: personalservice.WSEventUnsend,
				Payload: personalmodel.UnsendEventPayload{
					ChatID:     payload.ChatID,
					MessageIDs: payload.MessageIDs,
					SenderID:   userId, // Added: frontend handleUnsend uses sender_id for unread count logic
				},
			}

			// Notify recipient (all devices)
			log.Printf("[WS Broadcast] Unsend: pushing unsend to RECIPIENT=%s", recipientID)
			go hub.BroadcastToUser(recipientID, unsendEvent)

			// Notify sender's other devices (for sync)
			sessionId, _ := c.Get("sessionId").(string)
			log.Printf("[WS Broadcast] Unsend: pushing unsend to SENDER=%s other devices (excluding session=%s)",
				uuidUserId, sessionId)
			go hub.BroadcastToUserExcept(uuidUserId, sessionId, unsendEvent)
		} else {
			log.Printf("[WS Broadcast] Unsend: GetChatByID FAILED for chatID=%s: %v", payload.ChatID, err)
		}
	}

	return c.JSON(http.StatusOK, model.StatusOkay{Status: true, Message: "success"})
}

func (h *ChatHandler) DeleteMessageForMe(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.DeleteMessageForMePayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	isPrimary, ok := c.Get("isPrimary").(bool)
	if !ok {
		isPrimary = false
	}

	apiErr := h.service.DeleteMessageForMeHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId}, isPrimary)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}

	// ── WS Broadcast: delete_for_me ──────────────────────────────────────
	// Notify the SAME user's other devices (for sync).
	if hub := h.service.WSHub; hub != nil {
		sessionId, _ := c.Get("sessionId").(string)

		// Resolve chat_id from the first message so the frontend can clear the preview
		var chatID string
		if len(payload.MessageIDs) > 0 {
			if msgUUID, parseErr := uuid.Parse(payload.MessageIDs[0]); parseErr == nil {
				if msg, lookupErr := h.service.PersonalQueries.GetMessageByID(c.Request().Context(), msgUUID); lookupErr == nil {
					chatID = msg.ChatID.String()
				}
			}
		}

		log.Printf("[WS Broadcast] DeleteForMe: user=%s messageIDs=%v chatID=%s → pushing to other devices (excluding session=%s)",
			uuidUserId, payload.MessageIDs, chatID, sessionId)
		go hub.BroadcastToUserExcept(uuidUserId, sessionId, personalservice.WSEvent{
			Type: personalservice.WSEventDeleteForMe,
			Payload: personalmodel.DeleteForMeEventPayload{
				MessageIDs: payload.MessageIDs,
				ChatID:     chatID,
			},
		})
	}

	return c.JSON(http.StatusOK, model.StatusOkay{Status: true, Message: "success"})
}

func (h *ChatHandler) GetSyncActions(c echo.Context) error {
	userId, ok := c.Get("userId").(string)
	if !ok || userId == "" {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}
	uuidUserId, okUUID := c.Get("uuidUserId").(uuid.UUID)
	if !okUUID {
		return c.JSON(http.StatusUnauthorized, &model.ApiError{
			Code:    http.StatusUnauthorized,
			Message: "User id is missing or invalid",
			Type:    "unauthorized",
		})
	}

	var payload personalmodel.GetSyncActionsPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	resp, apiErr := h.service.GetSyncActionsHandler(c.Request().Context(), &payload, model.UserId{StringUserId: userId, UuidUserId: uuidUserId})
	if apiErr != nil {
		log.Printf("[ChatHandler] GetSyncActions failed for user %s: %v", userId, apiErr)
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) AcknowledgeSyncAction(c echo.Context) error {
	var payload personalmodel.AcknowledgeSyncActionPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "invalid request payload",
			Type:    "bad_request",
		})
	}

	isPrimary, ok := c.Get("isPrimary").(bool)
	if !ok {
		isPrimary = false
	}

	apiErr := h.service.AcknowledgeSyncActionHandler(c.Request().Context(), &payload, isPrimary)
	if apiErr != nil {
		return c.JSON(apiErr.Code, apiErr)
	}
	return c.JSON(http.StatusOK, model.StatusOkay{Status: true, Message: "success"})
}
