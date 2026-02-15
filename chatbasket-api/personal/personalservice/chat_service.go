package personalservice

import (
	"chatbasket-api/internal/db/personal"
	"chatbasket-api/model"
	personalmodel "chatbasket-api/personal/personalmodel"
	"chatbasket-api/utils"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
)

const (
	EligibilityAllowed            = "allowed"
	EligibilityNotInContacts      = "not_in_contacts"
	EligibilityRecipientPrivate   = "recipient_private"
	EligibilityBlocked            = "blocked" // Legacy, keep for safety
	EligibilityBlockedByRecipient = "blocked_by_recipient"
	EligibilityBlockedByMe        = "blocked_by_me"
	EligibilityAdminBlocked       = "admin_blocked"
	EligibilityNoPrimaryDevice    = "no_primary_device"
	EligibilityRecipientNotFound  = "recipient_not_found"
)

const (
	DefaultMessageTTL   = 30 * 24 * time.Hour
	StorageFullTTL      = 7 * 24 * time.Hour
	MaxDeliveryAttempts = 5
)

func messagingEligibilityError(eligibility string) *model.ApiError {
	errType := "messaging_not_allowed"
	switch eligibility {
	case EligibilityNotInContacts:
		errType = "messaging_not_allowed_not_in_contacts"
	case EligibilityRecipientPrivate:
		errType = "messaging_not_allowed_recipient_private"
	case EligibilityBlocked, EligibilityBlockedByRecipient:
		errType = "messaging_not_allowed_blocked_by_recipient"
	case EligibilityBlockedByMe:
		errType = "messaging_not_allowed_blocked_by_me"
	case EligibilityAdminBlocked:
		errType = "messaging_not_allowed_admin_blocked"
	case EligibilityNoPrimaryDevice:
		errType = "messaging_not_allowed_no_primary_device"
	case EligibilityRecipientNotFound:
		errType = "messaging_not_allowed_recipient_not_found"
	}

	return &model.ApiError{
		Code:    http.StatusForbidden,
		Message: eligibility,
		Type:    errType,
	}
}

func (ps *Service) CreateOrGetChat(ctx context.Context, user1ID, user2ID uuid.UUID) (*personal.Chat, *model.ApiError) {
	chatID := uuid.New()

	chat, err := ps.PersonalQueries.CreateChat(ctx, personal.CreateChatParams{
		ID:      chatID,
		Column2: user1ID,
		Column3: user2ID,
	})

	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "chat_creation_failed",
		}
	}

	return &chat, nil
}

func (ps *Service) CheckMessagingEligibility(ctx context.Context, senderID, recipientID uuid.UUID) (string, *model.ApiError) {
	fmt.Printf("[DEBUG] CheckMessagingEligibility: sender=%s, recipient=%s\n", senderID, recipientID)

	status, err := ps.PersonalQueries.CanSendMessage(ctx, personal.CanSendMessageParams{
		Column1: senderID,
		Column2: recipientID,
	})

	if err != nil {
		fmt.Printf("[DEBUG] CanSendMessage query error: %v\n", err)
		return "", &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "eligibility_check_failed",
		}
	}

	fmt.Printf("[DEBUG] CanSendMessage status: %s\n", status)

	if status != EligibilityAllowed {
		return status, nil
	}

	senderPrimary, err := ps.AuthQueries.GetUserPrimarySession(ctx, senderID)
	if err != nil {
		if err == pgx.ErrNoRows {
			fmt.Printf("[DEBUG] Sender has no primary device\n")
			return EligibilityNoPrimaryDevice, nil
		}
		return "", &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "failed to check sender primary device",
			Type:    "internal_server_error",
		}
	}

	if senderPrimary.ID == uuid.Nil {
		fmt.Printf("[DEBUG] Sender primary device is nil\n")
		return EligibilityNoPrimaryDevice, nil
	}

	recipientPrimary, err := ps.AuthQueries.GetUserPrimarySession(ctx, recipientID)
	if err != nil {
		if err == pgx.ErrNoRows {
			fmt.Printf("[DEBUG] Recipient has no primary device\n")
			return EligibilityNoPrimaryDevice, nil
		}
		return "", &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "failed to check recipient primary device",
			Type:    "internal_server_error",
		}
	}

	if recipientPrimary.ID == uuid.Nil {
		fmt.Printf("[DEBUG] Recipient primary device is nil\n")
		return EligibilityNoPrimaryDevice, nil
	}

	fmt.Printf("[DEBUG] All checks passed - allowed\n")
	return EligibilityAllowed, nil
}

type SendMessageParams struct {
	SenderID    uuid.UUID
	RecipientID uuid.UUID
	Content     string
	MessageType string
}

func (ps *Service) SendMessage(ctx context.Context, params SendMessageParams) (*personal.Message, *model.ApiError) {
	eligibility, apiErr := ps.CheckMessagingEligibility(ctx, params.SenderID, params.RecipientID)
	if apiErr != nil {
		return nil, apiErr
	}

	if eligibility != EligibilityAllowed {
		return nil, messagingEligibilityError(eligibility)
	}

	chat, apiErr := ps.CreateOrGetChat(ctx, params.SenderID, params.RecipientID)
	if apiErr != nil {
		return nil, apiErr
	}

	messageID := uuid.New()
	expiresAt := time.Now().Add(DefaultMessageTTL)

	message, err := ps.PersonalQueries.CreateMessage(ctx, personal.CreateMessageParams{
		ID:          messageID,
		ChatID:      chat.ID,
		SenderID:    params.SenderID,
		RecipientID: params.RecipientID,
		Content:     params.Content,
		MessageType: params.MessageType,
		ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})

	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "message_send_failed",
		}
	}

	return &message, nil
}

func (ps *Service) AcknowledgeDelivery(ctx context.Context, messageID uuid.UUID, acknowledgedBy string) *model.ApiError {
	var err error

	if acknowledgedBy == "recipient" {
		err = ps.PersonalQueries.MarkMessageDeliveredToRecipient(ctx, messageID)
	} else {
		err = ps.PersonalQueries.MarkMessageSyncedToSenderPrimary(ctx, messageID)
	}

	if err != nil {
		return &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "ack_failed",
		}
	}

	message, err := ps.PersonalQueries.GetMessageByID(ctx, messageID)
	if err != nil {
		return nil
	}

	if message.DeliveredToRecipient && message.SyncedToSenderPrimary {
		if err := ps.CleanupMessageFile(ctx, messageID); err != nil {
		}

		err = ps.PersonalQueries.DeleteMessage(ctx, messageID)
		if err != nil {
			return &model.ApiError{
				Code:    http.StatusInternalServerError,
				Message: "failed to cleanup delivered message",
				Type:    "cleanup_failed",
			}
		}
	}

	return nil
}

func (ps *Service) GetPendingMessages(ctx context.Context, userID uuid.UUID, limit int32) ([]personal.Message, *model.ApiError) {
	messages, err := ps.PersonalQueries.GetPendingMessagesForRecipient(ctx, personal.GetPendingMessagesForRecipientParams{
		RecipientID: userID,
		Limit:       limit,
	})

	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "fetch_failed",
		}
	}

	return messages, nil
}

func (ps *Service) GetChatMessages(ctx context.Context, chatID uuid.UUID, limit, offset int32) ([]personal.Message, *model.ApiError) {
	messages, err := ps.PersonalQueries.GetChatMessages(ctx, personal.GetChatMessagesParams{
		ChatID: chatID,
		Limit:  limit,
		Offset: offset,
	})

	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "fetch_failed",
		}
	}

	return messages, nil
}

func (ps *Service) IsChatParticipant(ctx context.Context, chatID, userID uuid.UUID) (bool, *model.ApiError) {
	isParticipant, err := ps.PersonalQueries.IsChatParticipant(ctx, personal.IsChatParticipantParams{
		Column1: chatID,
		Column2: userID,
	})

	if err != nil {
		return false, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "verification_failed",
		}
	}

	return isParticipant, nil
}

func (ps *Service) CleanupExpiredMessages(ctx context.Context) error {
	return ps.PersonalQueries.DeleteExpiredMessages(ctx)
}

func (ps *Service) DropPendingMessagesBetweenUsers(ctx context.Context, user1ID, user2ID uuid.UUID) error {
	return ps.PersonalQueries.DeletePendingMessagesBetweenUsers(ctx, personal.DeletePendingMessagesBetweenUsersParams{
		SenderID:    user1ID,
		RecipientID: user2ID,
	})
}

func (ps *Service) GetUserChats(ctx context.Context, userID uuid.UUID) ([]personal.GetUserChatsRow, *model.ApiError) {
	chats, err := ps.PersonalQueries.GetUserChats(ctx, userID)

	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "fetch_failed",
		}
	}

	return chats, nil
}

func (ps *Service) GetChatByParticipants(ctx context.Context, user1ID, user2ID uuid.UUID) (*personal.Chat, *model.ApiError) {
	chat, err := ps.PersonalQueries.GetChatByParticipants(ctx, personal.GetChatByParticipantsParams{
		Column1: user1ID,
		Column2: user2ID,
	})

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.ApiError{
				Code:    http.StatusNotFound,
				Message: "chat not found",
				Type:    "chat_not_found",
			}
		}
		return nil, &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: utils.GetPostgresError(err).Message,
			Type:    "fetch_failed",
		}
	}

	return &chat, nil
}

// New service methods to match handler patterns
func (ps *Service) CheckEligibilityHandler(ctx context.Context, payload *personalmodel.CheckEligibilityPayload, userId model.UserId) (*personalmodel.MessagingEligibilityResponse, *model.ApiError) {
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid recipient ID",
			Type:    "invalid_recipient",
		}
	}

	if userId.UuidUserId == recipientID {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Cannot check eligibility with yourself",
			Type:    "invalid_recipient",
		}
	}

	eligibility, apiErr := ps.CheckMessagingEligibility(ctx, userId.UuidUserId, recipientID)
	if apiErr != nil {
		return nil, apiErr
	}

	resp := &personalmodel.MessagingEligibilityResponse{
		Allowed: eligibility == EligibilityAllowed,
		Reason:  "",
	}
	if !resp.Allowed {
		resp.Reason = eligibility
	}

	return resp, nil
}

func (ps *Service) CreateChatHandler(ctx context.Context, payload *personalmodel.CreateChatPayload, userId model.UserId) (*personalmodel.ChatResponse, *model.ApiError) {
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid recipient ID",
			Type:    "invalid_recipient",
		}
	}

	if userId.UuidUserId == recipientID {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Cannot create chat with yourself",
			Type:    "invalid_recipient",
		}
	}

	eligibility, apiErr := ps.CheckMessagingEligibility(ctx, userId.UuidUserId, recipientID)
	if apiErr != nil {
		return nil, apiErr
	}

	if eligibility != EligibilityAllowed {
		return nil, messagingEligibilityError(eligibility)
	}

	chat, apiErr := ps.CreateOrGetChat(ctx, userId.UuidUserId, recipientID)
	if apiErr != nil {
		return nil, apiErr
	}

	return &personalmodel.ChatResponse{
		ChatID:      chat.ID.String(),
		OtherUserID: recipientID.String(),
		CreatedAt:   chat.CreatedAt.Time,
		UpdatedAt:   chat.UpdatedAt.Time,
	}, nil
}

func (ps *Service) SendMessageHandler(ctx context.Context, payload *personalmodel.SendMessagePayload, userId model.UserId) (*personalmodel.MessageResponse, *model.ApiError) {
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid recipient ID",
			Type:    "invalid_recipient",
		}
	}

	if userId.UuidUserId == recipientID {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Cannot send message to yourself",
			Type:    "invalid_recipient",
		}
	}

	message, apiErr := ps.SendMessage(ctx, SendMessageParams{
		SenderID:    userId.UuidUserId,
		RecipientID: recipientID,
		Content:     payload.Content,
		MessageType: payload.MessageType,
	})

	if apiErr != nil {
		return nil, apiErr
	}

	return &personalmodel.MessageResponse{
		MessageID:   message.ID.String(),
		ChatID:      message.ChatID.String(),
		SenderID:    message.SenderID.String(),
		RecipientID: message.RecipientID.String(),
		Content:     message.Content,
		MessageType: message.MessageType,
		CreatedAt:   message.CreatedAt.Time,
		ExpiresAt:   message.ExpiresAt.Time,
	}, nil
}

func (ps *Service) GetMessagesHandler(ctx context.Context, payload *personalmodel.GetMessagesPayload, userId model.UserId) (*personalmodel.GetMessagesResponse, *model.ApiError) {
	chatID, err := uuid.Parse(payload.ChatID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid chat ID",
			Type:    "invalid_chat_id",
		}
	}

	isParticipant, apiErr := ps.IsChatParticipant(ctx, chatID, userId.UuidUserId)
	if apiErr != nil {
		return nil, apiErr
	}

	if !isParticipant {
		return nil, &model.ApiError{
			Code:    http.StatusForbidden,
			Message: "not_chat_participant",
			Type:    "chat_access_denied",
		}
	}

	limit := payload.Limit
	if limit == 0 {
		limit = 50
	}
	offset := payload.Offset

	messages, apiErr := ps.GetChatMessages(ctx, chatID, limit, offset)
	if apiErr != nil {
		return nil, apiErr
	}

	messageResponses := make([]personalmodel.MessageResponse, len(messages))
	for i, msg := range messages {
		messageResponses[i] = personalmodel.MessageResponse{
			MessageID:   msg.ID.String(),
			ChatID:      msg.ChatID.String(),
			SenderID:    msg.SenderID.String(),
			RecipientID: msg.RecipientID.String(),
			Content:     msg.Content,
			MessageType: msg.MessageType,
			CreatedAt:   msg.CreatedAt.Time,
			ExpiresAt:   msg.ExpiresAt.Time,
		}
	}

	return &personalmodel.GetMessagesResponse{
		Messages: messageResponses,
		Count:    len(messageResponses),
	}, nil
}

func (ps *Service) AcknowledgeDeliveryHandler(ctx context.Context, payload *personalmodel.AcknowledgeDeliveryPayload, userId model.UserId) (*personalmodel.AcknowledgeDeliveryResponse, *model.ApiError) {
	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid message ID",
			Type:    "invalid_message_id",
		}
	}

	// Check if success is false, return early without processing
	if !payload.Success {
		return &personalmodel.AcknowledgeDeliveryResponse{
			Acknowledged: false,
		}, nil
	}

	// Pass acknowledged_by string directly to AcknowledgeDelivery
	apiErr := ps.AcknowledgeDelivery(ctx, messageID, payload.AcknowledgedBy)
	if apiErr != nil {
		return nil, apiErr
	}

	return &personalmodel.AcknowledgeDeliveryResponse{
		Acknowledged: true,
	}, nil
}

func (ps *Service) GetUserChatsHandler(ctx context.Context, userId model.UserId) (*personalmodel.GetUserChatsResponse, *model.ApiError) {
	chats, apiErr := ps.GetUserChats(ctx, userId.UuidUserId)
	if apiErr != nil {
		return nil, apiErr
	}

	// Helper for privacy logic (same as in contact_service.go)
	shouldExposeAvatar := func(globalRestrictProfile, exceptionGlobalProfile, globalRestrictAvatar, exceptionGlobalAvatar, userRestrictProfile, userRestrictAvatar bool) bool {
		if globalRestrictProfile {
			return exceptionGlobalProfile
		}
		if globalRestrictAvatar {
			return exceptionGlobalAvatar
		}
		if userRestrictProfile {
			return false
		}
		if userRestrictAvatar {
			return false
		}
		return true
	}

	chatResponses := make([]personalmodel.ChatResponse, len(chats))
	for i, chat := range chats {
		var avatarURL *string
		if shouldExposeAvatar(chat.GlobalRestrictProfile, chat.ExceptionGlobalProfile, chat.GlobalRestrictAvatar, chat.ExceptionGlobalAvatar, chat.UserRestrictProfile, chat.UserRestrictAvatar) {
			url, apiErr := ps.buildAvatarURL(ctx, chat.AvatarFileID, chat.AvatarTokenID, chat.AvatarTokenSecret, chat.AvatarTokenExpiry, chat.OtherUserID)
			if apiErr != nil {
				return nil, apiErr
			}
			avatarURL = url
		}

		var lastMessageContent *string
		var lastMessageCreatedAt *time.Time
		var lastMessageType *string
		var lastMessageSenderID *string

		if chat.LastMessageContent != nil {
			lastMessageContent = chat.LastMessageContent

			if chat.LastMessageCreatedAt.Valid {
				t := chat.LastMessageCreatedAt.Time
				lastMessageCreatedAt = &t
			}

			lastMessageType = chat.LastMessageType

			if chat.LastMessageSenderID.Valid {
				// Convert [16]byte to slice for FromBytes
				uid, err := uuid.FromBytes(chat.LastMessageSenderID.Bytes[:])
				if err == nil {
					s := uid.String()
					lastMessageSenderID = &s
				}
			}
		}

		chatResponses[i] = personalmodel.ChatResponse{
			ChatID:               chat.ID.String(),
			OtherUserID:          chat.OtherUserID.String(),
			OtherUserName:        chat.OtherUserName,
			OtherUserUsername:    chat.OtherUserUsername,
			AvatarURL:            avatarURL,
			CreatedAt:            chat.CreatedAt.Time,
			UpdatedAt:            chat.UpdatedAt.Time,
			LastMessageContent:   lastMessageContent,
			LastMessageCreatedAt: lastMessageCreatedAt,
			LastMessageType:      lastMessageType,
			LastMessageSenderID:  lastMessageSenderID,
			UnreadCount:          int(chat.UnreadCount),
		}
	}

	return &personalmodel.GetUserChatsResponse{
		Chats: chatResponses,
		Count: len(chatResponses),
	}, nil
}

func (ps *Service) UploadFileForMessageHandler(ctx context.Context, c echo.Context, userId model.UserId) (*personalmodel.UploadFileResponse, *model.ApiError) {
	recipientIDStr := c.FormValue("recipient_id")
	if recipientIDStr == "" {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "recipient_id is required",
			Type:    "invalid_request",
		}
	}

	recipientID, err := uuid.Parse(recipientIDStr)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid recipient ID",
			Type:    "invalid_recipient",
		}
	}

	if userId.UuidUserId == recipientID {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Cannot send file to yourself",
			Type:    "invalid_recipient",
		}
	}

	messageType := c.FormValue("message_type")
	if messageType == "" {
		messageType = "file"
	}

	caption := c.FormValue("caption")

	file, err := c.FormFile("file")
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "No file provided",
			Type:    "invalid_request",
		}
	}

	message, apiErr := ps.UploadFileForMessage(ctx, UploadFileForMessageParams{
		SenderID:    userId.UuidUserId,
		RecipientID: recipientID,
		FileHeader:  file,
		MessageType: messageType,
		Caption:     caption,
	})

	if apiErr != nil {
		return nil, apiErr
	}

	fileURL, apiErr := ps.GetMessageFileURL(ctx, message.ID, userId.UuidUserId)
	if apiErr != nil {
		return nil, apiErr
	}

	return &personalmodel.UploadFileResponse{
		MessageID: message.ID.String(),
		FileURL:   fileURL,
		FileName:  message.FileName,
		FileSize:  message.FileSize,
		CreatedAt: message.CreatedAt.Time,
		ExpiresAt: message.ExpiresAt.Time,
	}, nil
}

func (ps *Service) GetFileURLHandler(ctx context.Context, payload *personalmodel.GetFileURLPayload, userId model.UserId) (*personalmodel.GetFileURLResponse, *model.ApiError) {
	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		return nil, &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid message ID",
			Type:    "invalid_request",
		}
	}

	fileURL, apiErr := ps.GetMessageFileURL(ctx, messageID, userId.UuidUserId)
	if apiErr != nil {
		return nil, apiErr
	}

	return &personalmodel.GetFileURLResponse{
		FileURL: fileURL,
	}, nil
}

func (ps *Service) MarkChatRead(ctx context.Context, userID uuid.UUID, chatID uuid.UUID) *model.ApiError {
	// Verify user is participant
	isParticipant, err := ps.PersonalQueries.IsChatParticipant(ctx, personal.IsChatParticipantParams{
		Column1: chatID,
		Column2: userID,
	})
	if err != nil {
		return &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to check chat participation",
			Type:    "server_error",
		}
	}
	if !isParticipant {
		return &model.ApiError{
			Code:    http.StatusForbidden,
			Message: "User is not a participant of this chat",
			Type:    "forbidden",
		}
	}

	// Mark messages as read
	err = ps.PersonalQueries.MarkChatMessagesAsRead(ctx, personal.MarkChatMessagesAsReadParams{
		ChatID:      chatID,
		RecipientID: userID,
	})
	if err != nil {
		return &model.ApiError{
			Code:    http.StatusInternalServerError,
			Message: "Failed to mark messages as read",
			Type:    "server_error",
		}
	}

	return nil
}

func (ps *Service) MarkChatReadHandler(ctx context.Context, payload *personalmodel.MarkChatReadPayload, userId model.UserId) *model.ApiError {
	chatID, err := uuid.Parse(payload.ChatID)
	if err != nil {
		return &model.ApiError{
			Code:    http.StatusBadRequest,
			Message: "Invalid chat ID",
			Type:    "invalid_request",
		}
	}

	return ps.MarkChatRead(ctx, userId.UuidUserId, chatID)
}
