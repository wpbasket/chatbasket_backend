package personal_chat

import (
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/kit"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
// Provider Interfaces
// â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

// coreAuthChatProvider defines the minimal set of methods required from the Auth module.
// *core_auth.AuthService satisfies this interface directly â€” no adapter needed.
type coreAuthChatProvider interface {
	IsSessionCentral(ctx context.Context, userID uuid.UUID, sessionToken string) (bool, error)
	GetUserPrimarySessionID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}

// personalProfilePersonalChatProvider defines the minimal set of methods required from the Profile module.
// *personal_profile.profileService satisfies this interface directly â€” no adapter needed.
type personalProfilePersonalChatProvider interface {
	GetVisibleProfilesForContactViewer(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error)
	GetUserCoreProfile(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error)
	IsUserAdminBlocked(ctx context.Context, userID uuid.UUID) (bool, error)
}

// ————————————————————————————————————————————————————————————————————————————————
// Chat Service
// ————————————————————————————————————————————————————————————————————————————————

type chatService struct {
	Pool            *pgxpool.Pool
	PostgresQuerier personal_chat_store.Querier
	PostgresQueries *personal_chat_store.Queries
	AuthProvider    coreAuthChatProvider
	ProfileProvider personalProfilePersonalChatProvider
	AppwriteStorage *clients.AppwriteStorageService
}

func NewChatService(
	pool *pgxpool.Pool,
	authProvider coreAuthChatProvider,
	profileProvider personalProfilePersonalChatProvider,
	appwriteStorage *clients.AppwriteStorageService,
) *chatService {
	store := personal_chat_store.New(pool)
	return &chatService{
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		AuthProvider:    authProvider,
		ProfileProvider: profileProvider,
		AppwriteStorage: appwriteStorage,
	}
}

// ————————————————————————————————————————————————————————————————————————————————
// Core Messaging Functions
// ————————————————————————————————————————————————————————————————————————————————

func (s *chatService) CheckMessagingEligibility(ctx context.Context, senderID kit.UserId, recipientID uuid.UUID) (string, error) {
	// 1. Self-check (Legacy feature)
	if senderID.UuidUserId == recipientID {
		return "", kit.NewError(http.StatusBadRequest, "invalid_recipient", "Cannot check eligibility with yourself")
	}

	// 2. Fetch recipient exists + profile stats (Required for all subsequent checks)
	coreProfile, err := s.ProfileProvider.GetUserCoreProfile(ctx, recipientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EligibilityRecipientNotFound, nil
		}
		if pe, ok := err.(kit.ProcessedError); ok && pe.Status() == http.StatusNotFound {
			return EligibilityRecipientNotFound, nil
		}
		return "", kit.NewError(http.StatusInternalServerError, "eligibility_check_failed", "failed to fetch recipient profile: "+err.Error())
	}

	// 3. Contact check (Legacy Priority #2)
	// We MUST check if they are in contacts BEFORE checking privacy/blocks to match legacy logic order
	// This prevents leaking private status to non-contacts.
	status, err := s.PostgresQueries.CanSendMessageLite(ctx, personal_chat_store.CanSendMessageLiteParams{
		Column1: senderID.UuidUserId,
		Column2: recipientID,
	})
	if err != nil {
		return "", kit.NewError(http.StatusInternalServerError, "eligibility_check_failed", kit.GetPostgresError(err).Message)
	}

	if status == EligibilityNotInContacts {
		return EligibilityNotInContacts, nil
	}

	// 4. Recipient Private check (Legacy Priority #3)
	if coreProfile.ProfileType == "private" {
		return EligibilityRecipientPrivate, nil
	}

	// 5. Block checks (Legacy Priority #4)
	if status != EligibilityAllowed {
		return status, nil // (blocked_by_recipient, blocked_by_me)
	}

	// 6. Admin-block checks (Legacy Priority #5)
	senderBlocked, err := s.ProfileProvider.IsUserAdminBlocked(ctx, senderID.UuidUserId)
	if err != nil {
		return "", kit.NewError(http.StatusInternalServerError, "eligibility_check_failed", "failed to check sender admin status")
	}
	if senderBlocked {
		return EligibilityAdminBlocked, nil
	}

	recipientBlocked, err := s.ProfileProvider.IsUserAdminBlocked(ctx, recipientID)
	if err != nil {
		return "", kit.NewError(http.StatusInternalServerError, "eligibility_check_failed", "failed to check recipient admin status")
	}
	if recipientBlocked {
		return EligibilityAdminBlocked, nil
	}

	// 7. Primary-device check (Legacy Priority #6)
	senderPrimaryID, err := s.AuthProvider.GetUserPrimarySessionID(ctx, senderID.UuidUserId)
	if err != nil {
		if pe, ok := err.(kit.ProcessedError); ok && pe.Status() == http.StatusNotFound {
			return EligibilityNoPrimaryDevice, nil
		}
		return "", kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to check sender primary device")
	}
	if senderPrimaryID == uuid.Nil {
		return EligibilityNoPrimaryDevice, nil
	}

	recipientPrimaryID, err := s.AuthProvider.GetUserPrimarySessionID(ctx, recipientID)
	if err != nil {
		if pe, ok := err.(kit.ProcessedError); ok && pe.Status() == http.StatusNotFound {
			return EligibilityNoPrimaryDevice, nil
		}
		return "", kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to check recipient primary device")
	}
	if recipientPrimaryID == uuid.Nil {
		return EligibilityNoPrimaryDevice, nil
	}

	return EligibilityAllowed, nil
}

func (s *chatService) CheckEligibilityHandler(ctx context.Context, payload *CheckEligibilityPayload, userID kit.UserId) (*MessagingEligibilityResponse, error) {
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_recipient", "Invalid recipient ID")
	}

	eligibility, eligErr := s.CheckMessagingEligibility(ctx, userID, recipientID)
	if eligErr != nil {
		return nil, eligErr
	}

	resp := &MessagingEligibilityResponse{
		Allowed: eligibility == EligibilityAllowed,
		Reason:  "",
	}
	if !resp.Allowed {
		resp.Reason = eligibility
	}

	return resp, nil
}

func (s *chatService) CreateOrGetChat(ctx context.Context, user1ID, user2ID uuid.UUID) (*personal_chat_store.Chat, error) {
	chatID := uuid.New()

	chat, err := s.PostgresQueries.CreateChat(ctx, personal_chat_store.CreateChatParams{
		ID:      chatID,
		Column2: user1ID,
		Column3: user2ID,
	})

	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "chat_creation_failed", kit.GetPostgresError(err).Message)
	}

	return &chat, nil
}

func (s *chatService) CreateChatHandler(ctx context.Context, payload *CreateChatPayload, userID kit.UserId) (*ChatResponse, error) {
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_recipient", "Invalid recipient ID")
	}

	if userID.UuidUserId == recipientID {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_recipient", "Cannot create chat with yourself")
	}

	eligibility, eligErr := s.CheckMessagingEligibility(ctx, userID, recipientID)
	if eligErr != nil {
		return nil, eligErr
	}

	if eligibility != EligibilityAllowed {
		return nil, messagingEligibilityError(eligibility)
	}

	chat, chatErr := s.CreateOrGetChat(ctx, userID.UuidUserId, recipientID)
	if chatErr != nil {
		return nil, chatErr
	}

	var otherReadAt, otherDeliveredAt time.Time
	if chat.Participant1ID == userID.UuidUserId {
		otherReadAt = kit.DerefTime(chat.P2LastReadAt)
		otherDeliveredAt = kit.DerefTime(chat.P2LastDeliveredAt)
	} else {
		otherReadAt = kit.DerefTime(chat.P1LastReadAt)
		otherDeliveredAt = kit.DerefTime(chat.P1LastDeliveredAt)
	}

	return &ChatResponse{
		ChatID:                   chat.ID.String(),
		OtherUserID:              recipientID.String(),
		CreatedAt:                chat.CreatedAt,
		UpdatedAt:                chat.UpdatedAt,
		OtherUserLastReadAt:      otherReadAt,
		OtherUserLastDeliveredAt: otherDeliveredAt,
		LastMessageIsFromMe:      false,
	}, nil
}

func (s *chatService) GetChatByParticipants(ctx context.Context, user1ID, user2ID uuid.UUID) (*personal_chat_store.Chat, error) {
	chat, err := s.PostgresQueries.GetChatByParticipants(ctx, personal_chat_store.GetChatByParticipantsParams{
		Column1: user1ID,
		Column2: user2ID,
	})

	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, kit.NewError(http.StatusNotFound, "chat_not_found", "chat not found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
	}

	return &chat, nil
}

func (s *chatService) SendMessage(ctx context.Context, params SendMessageParams) (*personal_chat_store.Message, error) {
	eligibility, err := s.CheckMessagingEligibility(ctx, params.SenderID, params.RecipientID)
	if err != nil {
		return nil, err
	}

	if eligibility != EligibilityAllowed {
		return nil, messagingEligibilityError(eligibility)
	}

	chat, chatErr := s.CreateOrGetChat(ctx, params.SenderID.UuidUserId, params.RecipientID)
	if chatErr != nil {
		return nil, chatErr
	}

	messageID := uuid.New()
	expiresAt := time.Now().Add(DefaultMessageTTL)

	message, dbErr := s.PostgresQueries.CreateMessage(ctx, personal_chat_store.CreateMessageParams{
		ID:                          messageID,
		ChatID:                      chat.ID,
		SenderID:                    params.SenderID.UuidUserId,
		RecipientID:                 params.RecipientID,
		Content:                     params.Content,
		MessageType:                 params.MessageType,
		ExpiresAt:                   expiresAt,
		SyncedToSenderPrimary:       params.IsPrimary,
		DeliveredToRecipientPrimary: new(bool),
	})

	if dbErr != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "message_send_failed", kit.GetPostgresError(dbErr).Message)
	}

	// Update chat status (Last Message + Unread Count)
	_ = s.PostgresQueries.UpdateChatStatus(ctx, personal_chat_store.UpdateChatStatusParams{
		ID:                   chat.ID,
		P1LastMessageContent: &message.Content,
		LastMessageCreatedAt: &message.CreatedAt,
		P1LastMessageType:    &message.MessageType,
		LastMessageSenderID:  &message.SenderID,
		LastMessageID:        &message.ID,
	})

	return &message, nil
}

func (s *chatService) SendMessageHandler(ctx context.Context, payload *SendMessagePayload, userID kit.UserId, isPrimary bool) (*MessageResponse, error) {
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_recipient", "Invalid recipient id")
	}

	if userID.UuidUserId == recipientID {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_recipient", "Cannot send message to yourself")
	}

	message, sendErr := s.SendMessage(ctx, SendMessageParams{
		SenderID:    userID,
		RecipientID: recipientID,
		Content:     payload.Content,
		MessageType: payload.MessageType,
		IsPrimary:   isPrimary,
	})

	if sendErr != nil {
		return nil, sendErr
	}

	return &MessageResponse{
		MessageID:             message.ID.String(),
		ChatID:                message.ChatID.String(),
		RecipientID:           message.RecipientID.String(),
		Content:               message.Content,
		MessageType:           message.MessageType,
		DeliveredToRecipient:  message.DeliveredToRecipient,
		SyncedToSenderPrimary: message.SyncedToSenderPrimary,
		CreatedAt:             message.CreatedAt,
		ExpiresAt:             message.ExpiresAt,
		IsFromMe:              true,
		FileID:                message.FileID,
		FileName:              message.FileName,
		FileSize:              message.FileSize,
		FileMimeType:          message.FileMimeType,
	}, nil
}

// ————————————————————————————————————————————————————————————————————————————————
// Delivery Acknowledgment
// ————————————————————————————————————————————————————————————————————————————————

func (s *chatService) AcknowledgeDelivery(ctx context.Context, messageID uuid.UUID, acknowledgedBy string, sessionId string, userID kit.UserId) error {
	var err error

	// 1. Fetch message details first
	message, err := s.PostgresQueries.GetMessageByID(ctx, messageID)
	if err != nil {
		return kit.NewError(http.StatusNotFound, "not_found", "Message not found: "+messageID.String())
	}

	debugLog := log.New(os.Stderr, "[DEBUG-ACK] ", log.LstdFlags)
	// Check if session is Primary (Central)
	isCentral, centralErr := s.AuthProvider.IsSessionCentral(ctx, userID.UuidUserId, sessionId)
	debugLog.Printf("IsSessionCentral result: isCentral=%v, userID=%s, sessionId=%s", isCentral, userID.UuidUserId, sessionId)
	if centralErr != nil {
		debugLog.Printf("IsSessionCentral ERROR: %v", centralErr)
		if pe, ok := centralErr.(kit.ProcessedError); ok && pe.Status() == http.StatusUnauthorized {
			return kit.NewError(http.StatusForbidden, "session_invalid", "Forbidden: Session not found or invalid")
		}
		return centralErr
	}

	if acknowledgedBy == "recipient" {
		debugLog.Printf("Processing recipient ACK for message %s", messageID)
		// 1. Basic Delivery
		err = s.PostgresQueries.MarkMessageDeliveredToRecipient(ctx, messageID)
		if err != nil {
			debugLog.Printf("MarkMessageDeliveredToRecipient ERROR: %v", err)
			return kit.NewError(http.StatusInternalServerError, "ack_failed_basic", kit.GetPostgresError(err).Message)
		}

		// Update persistent chat delivery timestamp
		debugLog.Printf("Updating persistent chat %s delivery timestamp to %v for participant %s", message.ChatID, message.CreatedAt, userID.UuidUserId)
		_ = s.PostgresQueries.UpdateChatLastDeliveredAt(ctx, personal_chat_store.UpdateChatLastDeliveredAtParams{
			ChatID:          message.ChatID,
			ParticipantID:   userID.UuidUserId,
			LastDeliveredAt: message.CreatedAt,
		})

		// 2. Primary Delivery: ONLY Primary device
		if isCentral {
			debugLog.Printf("Session is CENTRAL. Marking message %s as Primary Delivered.", messageID)
			err = s.PostgresQueries.MarkMessageDeliveredToRecipientPrimary(ctx, messageID)
			if err != nil {
				debugLog.Printf("MarkMessageDeliveredToRecipientPrimary ERROR: %v", err)
			}

			// Sequential ACK for older messages
			debugLog.Printf("Executing Sequential ACK for older messages in chat %s up to %v", message.ChatID, message.CreatedAt)
			err = s.PostgresQueries.MarkOlderMessagesAsDeliveredToRecipientPrimary(ctx, personal_chat_store.MarkOlderMessagesAsDeliveredToRecipientPrimaryParams{
				ChatID:      message.ChatID,
				RecipientID: userID.UuidUserId,
				CreatedAt:   message.CreatedAt,
			})
			if err != nil {
				debugLog.Printf("MarkOlderMessagesAsDeliveredToRecipientPrimary ERROR: %v", err)
			}
		} else {
			debugLog.Printf("Session is NOT CENTRAL. Skipping Primary Delivery update.")
		}

	} else {
		// Sender Sync ACK
		debugLog.Printf("Processing sender sync ACK for message %s", messageID)
		// STRICT Security: ONLY Primary device can MARK as synced
		if !isCentral {
			debugLog.Printf("REJECTED: Sender sync ACK from non-central device.")
			return kit.NewError(http.StatusForbidden, "forbidden", "Forbidden: Only primary device can ACK sender sync")
		}

		// Verify Ownership
		if message.SenderID != userID.UuidUserId {
			debugLog.Printf("REJECTED: Ownership mismatch. message.SenderID=%s, userID=%s", message.SenderID, userID.UuidUserId)
			return kit.NewError(http.StatusForbidden, "forbidden", "Forbidden: You are not the sender of this message")
		}

		err = s.PostgresQueries.MarkMessageSyncedToSenderPrimary(ctx, messageID)
		if err != nil {
			debugLog.Printf("MarkMessageSyncedToSenderPrimary ERROR: %v", err)
		}
	}

	if err != nil {
		return kit.NewError(http.StatusInternalServerError, "ack_failed", kit.GetPostgresError(err).Message)
	}

	// Per-Message Relay Cleanup
	updatedMessage, err := s.PostgresQueries.GetMessageByID(ctx, messageID)
	if err == nil {
		recipientPrimaryDelivered := updatedMessage.DeliveredToRecipientPrimary != nil && *updatedMessage.DeliveredToRecipientPrimary
		senderPrimarySynced := updatedMessage.SyncedToSenderPrimary

		if recipientPrimaryDelivered && senderPrimarySynced {
			s.deleteMessageFromRelay(ctx, updatedMessage)
		}

		// Sequential Cleanup
		debugLog.Printf("Executing Sequential Bulk Cleanup for older messages in chat %s up to %v", updatedMessage.ChatID, updatedMessage.CreatedAt)
		cleanupErr := s.PostgresQueries.CleanupOlderFullyAcknowledgedMessages(ctx, personal_chat_store.CleanupOlderFullyAcknowledgedMessagesParams{
			ChatID:    updatedMessage.ChatID,
			CreatedAt: updatedMessage.CreatedAt,
		})
		if cleanupErr != nil {
			debugLog.Printf("CleanupOlderFullyAcknowledgedMessages ERROR: %v", cleanupErr)
		}
	}

	return nil
}

func (s *chatService) AcknowledgeDeliveryHandler(ctx context.Context, payload *AcknowledgeDeliveryPayload, userID kit.UserId, sessionId string) (*AcknowledgeDeliveryResponse, error) {
	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_message_id", "Invalid message ID")
	}

	// Check if success is false, return early without processing
	if !payload.Success {
		return &AcknowledgeDeliveryResponse{
			Acknowledged: false,
		}, nil
	}

	ackErr := s.AcknowledgeDelivery(ctx, messageID, payload.AcknowledgedBy, sessionId, userID)
	if ackErr != nil {
		return nil, ackErr
	}

	return &AcknowledgeDeliveryResponse{
		Acknowledged: true,
	}, nil
}

// ————————————————————————————————————————————————————————————————————————————————
// Message Queries
// ————————————————————————————————————————————————————————————————————————————————

func (s *chatService) GetChatMessages(ctx context.Context, chatID uuid.UUID, userID kit.UserId, limit, offset int32) ([]personal_chat_store.Message, error) {
	messages, err := s.PostgresQueries.GetChatMessages(ctx, personal_chat_store.GetChatMessagesParams{
		ChatID:   chatID,
		Limit:    limit,
		Offset:   offset,
		SenderID: userID.UuidUserId,
	})

	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
	}

	return messages, nil
}

// buildMessageResponse maps a DB message to a MessageResponse with optional file URLs.
func (s *chatService) buildMessageResponse(ctx context.Context, msg personal_chat_store.Message, userID kit.UserId) MessageResponse {
	viewURL, downloadURL := "", ""
	if msg.FileID != nil && *msg.FileID != "" {
		var fileErr error
		viewURL, downloadURL, fileErr = s.GenerateMessageFileURLs(ctx, msg, userID)
		if fileErr != nil {
			log.Printf("[buildMessageResponse] Failed to generate URLs for message %s: %v", msg.ID, fileErr)
		}
	}

	deliveredToRecipientPrimary := false
	if msg.DeliveredToRecipientPrimary != nil {
		deliveredToRecipientPrimary = *msg.DeliveredToRecipientPrimary
	}

	return MessageResponse{
		MessageID:                   msg.ID.String(),
		ChatID:                      msg.ChatID.String(),
		IsFromMe:                    msg.SenderID == userID.UuidUserId,
		RecipientID:                 msg.RecipientID.String(),
		Content:                     msg.Content,
		MessageType:                 msg.MessageType,
		DeliveredToRecipient:        msg.DeliveredToRecipient,
		DeliveredToRecipientPrimary: deliveredToRecipientPrimary,
		SyncedToSenderPrimary:       msg.SyncedToSenderPrimary,
		CreatedAt:                   msg.CreatedAt,
		ExpiresAt:                   msg.ExpiresAt,
		FileID:                      msg.FileID,
		FileName:                    msg.FileName,
		FileSize:                    msg.FileSize,
		FileMimeType:                msg.FileMimeType,
		ViewURL:                     viewURL,
		DownloadURL:                 downloadURL,
	}
}

func (s *chatService) GetMessagesHandler(ctx context.Context, payload *GetMessagesPayload, userID kit.UserId) (*GetMessagesResponse, error) {
	chatID, err := uuid.Parse(payload.ChatID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_chat_id", "Invalid chat ID")
	}

	isParticipant, partErr := s.IsChatParticipant(ctx, chatID, userID.UuidUserId)
	if partErr != nil {
		return nil, partErr
	}

	if !isParticipant {
		return nil, kit.NewError(http.StatusForbidden, "chat_access_denied", "not_chat_participant")
	}

	limit := payload.Limit
	if limit == 0 {
		limit = 50
	}

	messages, msgErr := s.GetChatMessages(ctx, chatID, userID, limit, payload.Offset)
	if msgErr != nil {
		return nil, msgErr
	}

	messageResponses := make([]MessageResponse, len(messages))
	for i, msg := range messages {
		messageResponses[i] = s.buildMessageResponse(ctx, msg, userID)
	}

	// Fetch other user's last read/delivered from chat
	chat, _ := s.PostgresQueries.GetChatByID(ctx, chatID)
	otherUserLastReadAt := time.Time{}
	otherUserLastDeliveredAt := time.Time{}
	if chat.ID != uuid.Nil {
		if chat.Participant1ID == userID.UuidUserId {
			otherUserLastReadAt = kit.DerefTime(chat.P2LastReadAt)
			otherUserLastDeliveredAt = kit.DerefTime(chat.P2LastDeliveredAt)
		} else {
			otherUserLastReadAt = kit.DerefTime(chat.P1LastReadAt)
			otherUserLastDeliveredAt = kit.DerefTime(chat.P1LastDeliveredAt)
		}
	}

	return &GetMessagesResponse{
		Messages:                 messageResponses,
		Count:                    len(messageResponses),
		OtherUserLastReadAt:      otherUserLastReadAt,
		OtherUserLastDeliveredAt: otherUserLastDeliveredAt,
	}, nil
}

func (s *chatService) IsChatParticipant(ctx context.Context, chatID uuid.UUID, userID uuid.UUID) (bool, error) {
	isParticipant, err := s.PostgresQueries.IsChatParticipant(ctx, personal_chat_store.IsChatParticipantParams{
		Column1: chatID,
		Column2: userID,
	})
	if err != nil {
		return false, kit.NewError(http.StatusInternalServerError, "server_error", "Failed to check chat participation")
	}
	return isParticipant, nil
}

func (s *chatService) GetPendingMessagesHandler(ctx context.Context, payload *GetPendingMessagesPayload, userID kit.UserId) (*GetMessagesResponse, error) {
	limit := payload.Limit
	if limit <= 0 {
		limit = 50
	}

	// 1. Fetch undelivered messages (where user is recipient)
	messagesRecv, err := s.PostgresQueries.GetPendingMessagesForRecipient(ctx, personal_chat_store.GetPendingMessagesForRecipientParams{
		RecipientID: userID.UuidUserId,
		Limit:       limit,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	// 2. Fetch unsynced messages (where user is sender but primary doesn't have it yet)
	messagesSent, err := s.PostgresQueries.GetPendingSenderSyncMessages(ctx, personal_chat_store.GetPendingSenderSyncMessagesParams{
		SenderID: userID.UuidUserId,
		Limit:    limit,
	})
	if err != nil {
		log.Printf("[GetPendingMessages] Warning: Failed to fetch sender-sync messages: %v", err)
	}

	totalCount := len(messagesRecv) + len(messagesSent)
	messageResponses := make([]MessageResponse, 0, totalCount)

	for _, m := range messagesRecv {
		messageResponses = append(messageResponses, s.buildMessageResponse(ctx, m, userID))
	}
	for _, m := range messagesSent {
		messageResponses = append(messageResponses, s.buildMessageResponse(ctx, m, userID))
	}

	return &GetMessagesResponse{
		Messages: messageResponses,
		Count:    len(messageResponses),
	}, nil
}


// Chat List

func (s *chatService) GetUserChatsLite(ctx context.Context, userID uuid.UUID) ([]personal_chat_store.GetUserChatsLiteRow, error) {
	chats, err := s.PostgresQueries.GetUserChatsLite(ctx, userID)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
	}
	return chats, nil
}

func (s *chatService) GetUserChatsHandler(ctx context.Context, userID kit.UserId) (*GetUserChatsResponse, error) {
	// Step 1: Fetch slim chat rows (zero cross-module JOINs)
	chats, err := s.GetUserChatsLite(ctx, userID.UuidUserId)
	if err != nil {
		return nil, err
	}

	// Step 2: Collect unique other_user_id values
	seen := make(map[uuid.UUID]struct{}, len(chats))
	targetIDs := make([]uuid.UUID, 0, len(chats))
	for _, chat := range chats {
		otherID := chat.OtherUserID
		if _, exists := seen[otherID]; !exists {
			seen[otherID] = struct{}{}
			targetIDs = append(targetIDs, otherID)
		}
	}

	// Step 3: Batch-resolve profiles via provider (handles decryption, privacy, avatar refresh)
	profilesByID, err := s.ProfileProvider.GetVisibleProfilesForContactViewer(ctx, userID.UuidUserId, targetIDs)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", "failed to resolve chat profiles: "+err.Error())
	}

	// Step 4: Map enriched profiles onto ChatResponse
	chatResponses := make([]ChatResponse, 0, len(chats))
	for _, chat := range chats {
		otherID := chat.OtherUserID

		profile := profilesByID[otherID]

		// CRITICAL FIDELITY: The legacy SQL uses an INNER JOIN on the users table.
		// If a user has been deleted but the chat remains (orphaned data),
		// the legacy backend automatically filters it out from the list.
		if profile == nil {
			continue
		}

		otherUserName := profile.Name
		otherUserUsername := profile.Username
		avatarURL := profile.AvatarURL


		var lastMessageContent, lastMessageType, lastMessageSenderID, lastMessageID *string
		var lastMessageCreatedAt *time.Time

		// Check if a last message exists using LastMessageID (now a pointer)
		if chat.LastMessageID != nil {
			if chat.LastMessageContent != nil {
				if strVal, ok := chat.LastMessageContent.(string); ok {
					lastMessageContent = &strVal
				}
			}

			if chat.LastMessageType != nil {
				if strVal, ok := chat.LastMessageType.(string); ok {
					lastMessageType = &strVal
				}
			}

			lastMessageCreatedAt = chat.LastMessageCreatedAt

			if chat.LastMessageSenderID != nil {
				senderStr := chat.LastMessageSenderID.String()
				lastMessageSenderID = &senderStr
			}

			idStr := chat.LastMessageID.String()
			lastMessageID = &idStr
		}

		chatResponses = append(chatResponses, ChatResponse{
			ChatID:                   chat.ID.String(),
			OtherUserID:              otherID.String(),
			OtherUserName:            otherUserName,
			OtherUserUsername:        otherUserUsername,
			AvatarURL:                avatarURL,
			CreatedAt:                chat.CreatedAt,
			UpdatedAt:                chat.UpdatedAt,
			OtherUserLastReadAt:      chat.OtherUserLastReadAt,
			OtherUserLastDeliveredAt: chat.OtherUserLastDeliveredAt,
			LastMessageContent:       lastMessageContent,
			LastMessageCreatedAt:     lastMessageCreatedAt,
			LastMessageType:          lastMessageType,
			LastMessageSenderID:      lastMessageSenderID,
			LastMessageIsFromMe:      chat.LastMessageSenderID != nil && *chat.LastMessageSenderID == userID.UuidUserId,
			LastMessageStatus:        chat.LastMessageStatus,
			LastMessageIsUnsent:      lastMessageType != nil && *lastMessageType == "unsent",
			LastMessageID:            lastMessageID,
			UnreadCount:              int(chat.UnreadCount),
		})
	}

	return &GetUserChatsResponse{
		Chats: chatResponses,
		Count: len(chatResponses),
	}, nil
}

// ————————————————————————————————————————————————————————————————————————————————
// Mark Read
// ————————————————————————————————————————————————————————————————————————————————

func (s *chatService) MarkChatRead(ctx context.Context, userID kit.UserId, chatID uuid.UUID, isPrimary bool) error {
	// Verify user is participant
	isParticipant, err := s.PostgresQueries.IsChatParticipant(ctx, personal_chat_store.IsChatParticipantParams{
		Column1: chatID,
		Column2: userID.UuidUserId,
	})
	if err != nil {
		return kit.NewError(http.StatusInternalServerError, "server_error", "Failed to check chat participation")
	}
	if !isParticipant {
		return kit.NewError(http.StatusForbidden, "forbidden", "User is not a participant of this chat")
	}

	log.Printf("[MarkChatRead] Resetting read status for chat %s, user %s", chatID, userID.UuidUserId)
	err = s.PostgresQueries.ResetChatReadStatus(ctx, personal_chat_store.ResetChatReadStatusParams{
		ID:             chatID,
		Participant1ID: userID.UuidUserId,
	})
	if err != nil {
		log.Printf("[MarkChatRead] ERROR: Failed to reset read status: %v", err)
		return kit.NewError(http.StatusInternalServerError, "server_error", "Failed to mark chat as read")
	}

	// Also update persistent delivery timestamp (if it's read, it's delivered)
	_ = s.PostgresQueries.UpdateChatLastDeliveredAt(ctx, personal_chat_store.UpdateChatLastDeliveredAtParams{
		ChatID:          chatID,
		ParticipantID:   userID.UuidUserId,
		LastDeliveredAt: time.Now(),
	})

	// Proactive Delivery: If primary, mark all messages as primary-delivered
	if isPrimary {
		log.Printf("[MarkChatRead] Primary session detected. Marking all messages in chat %s as primary-delivered for user %s", chatID, userID.UuidUserId)
		err = s.PostgresQueries.MarkChatMessagesAsReadPrimary(ctx, personal_chat_store.MarkChatMessagesAsReadPrimaryParams{
			ChatID:      chatID,
			RecipientID: userID.UuidUserId,
		})
		if err != nil {
			log.Printf("[MarkChatRead] ERROR: Failed to mark messages as read primary: %v", err)
		}
	}

	return nil
}

func (s *chatService) MarkChatReadHandler(ctx context.Context, payload *MarkChatReadPayload, userID kit.UserId, isPrimary bool) error {
	chatID, err := uuid.Parse(payload.ChatID)
	if err != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_request", "Invalid chat ID")
	}

	return s.MarkChatRead(ctx, userID, chatID, isPrimary)
}

// ————————————————————————————————————————————————————————————————————————————————
// Unsend
// ————————————————————————————————————————————————————————————————————————————————

func (s *chatService) UnsendMessage(ctx context.Context, chatID uuid.UUID, messageIDs []uuid.UUID, senderID kit.UserId, isPrimary bool) error {
	log.Printf("[UnsendMessage] START: Processing %d messages for sender %s in chat %s (isPrimary=%v)", len(messageIDs), senderID.UuidUserId, chatID, isPrimary)

	// Start transaction
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		log.Printf("[UnsendMessage] ERROR: Failed to begin transaction: %v", err)
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to start unsend transaction")
	}
	defer tx.Rollback(ctx)

	qtx := s.PostgresQueries.WithTx(tx)

	// Fetch chat details
	chat, err := qtx.GetChatByID(ctx, chatID)
	if err != nil {
		log.Printf("[UnsendMessage] ERROR: Chat %s not found: %v", chatID, err)
		return kit.NewError(http.StatusNotFound, "not_found", "chat not found")
	}

	// Identify recipient
	var recipientID uuid.UUID
	if chat.Participant1ID == senderID.UuidUserId {
		recipientID = chat.Participant2ID
	} else if chat.Participant2ID == senderID.UuidUserId {
		recipientID = chat.Participant1ID
	} else {
		log.Printf("[UnsendMessage] ERROR: User %s is not a participant in chat %s", senderID.UuidUserId, chatID)
		return kit.NewError(http.StatusForbidden, "forbidden", "unauthorized: not a participant in this chat")
	}

	messagesToUnsend := make([]personal_chat_store.Message, 0, len(messageIDs))

	for _, msgID := range messageIDs {
		msg, err := qtx.GetMessageByID(ctx, msgID)
		if err != nil {
			// FALLBACK: message already deleted
			log.Printf("[UnsendMessage] Message %s not found (likely already deleted). Falling back to sync-only.", msgID)

			// Notify Sender devices (ONLY if action initiated by Secondary)
			if !isPrimary {
				log.Printf("[UnsendMessage] [Fallback] Initiated by Secondary. Creating sync action for Sender.")
				senderPayload, _ := json.Marshal(SyncActionPayload{MessageIDs: []string{msgID.String()}, ChatID: chatID.String()})
				_, _ = qtx.CreateSyncAction(ctx, personal_chat_store.CreateSyncActionParams{
					ID:         uuid.New(),
					UserID:     senderID.UuidUserId,
					ActionType: "unsend",
					Payload:    senderPayload,
				})
			}

			// Notify all Recipient devices
			log.Printf("[UnsendMessage] [Fallback] Creating sync action for Recipient %s.", recipientID)
			recipientPayload, _ := json.Marshal(SyncActionPayload{MessageIDs: []string{msgID.String()}, ChatID: chatID.String()})
			_, _ = qtx.CreateSyncAction(ctx, personal_chat_store.CreateSyncActionParams{
				ID:         uuid.New(),
				UserID:     recipientID,
				ActionType: "unsend",
				Payload:    recipientPayload,
			})
		} else {
			// Security: Only sender can unsend
			if msg.SenderID != senderID.UuidUserId {
				log.Printf("[UnsendMessage] ERROR: Unauthorized unsend attempt for msg %s by user %s", msgID, senderID.UuidUserId)
				return kit.NewError(http.StatusForbidden, "forbidden", "unauthorized: you can only unsend your own messages")
			}

			// Prevent duplicate unsend
			if msg.MessageType == "unsent" {
				log.Printf("[UnsendMessage] Message %s is already unsent. Skipping.", msgID)
				continue
			}

			messagesToUnsend = append(messagesToUnsend, msg)

			// Soft delete from relay (Tombstone)
			if err := qtx.UpdateMessageToUnsent(ctx, msg.ID); err != nil {
				log.Printf("[UnsendMessage] ERROR: Failed to create tombstone for msg %s: %v", msg.ID, err)
				return kit.NewError(http.StatusInternalServerError, "server_error", "failed to create message tombstone")
			}

			// Sync actions for unsent messages
			if !isPrimary {
				senderPayload, _ := json.Marshal(SyncActionPayload{MessageIDs: []string{msgID.String()}, ChatID: chatID.String()})
				_, _ = qtx.CreateSyncAction(ctx, personal_chat_store.CreateSyncActionParams{
					ID:         uuid.New(),
					UserID:     senderID.UuidUserId,
					ActionType: "unsend",
					Payload:    senderPayload,
				})
			}

			recipientPayload, _ := json.Marshal(SyncActionPayload{MessageIDs: []string{msgID.String()}, ChatID: chatID.String()})
			_, _ = qtx.CreateSyncAction(ctx, personal_chat_store.CreateSyncActionParams{
				ID:         uuid.New(),
				UserID:     recipientID,
				ActionType: "unsend",
				Payload:    recipientPayload,
			})
		}
	}

	// Update chat preview + unread after unsend
	if len(messagesToUnsend) > 0 {
		// Update preview to "unsent" text
		lastMsgID := messagesToUnsend[len(messagesToUnsend)-1].ID
		_ = qtx.UpdateChatUnsendPreview(ctx, personal_chat_store.UpdateChatUnsendPreviewParams{
			ID:            chatID,
			LastMessageID: &lastMsgID,
		})

		// Decrement unread only for recipient
		_ = qtx.UpdateChatUnsendDecrement(ctx, personal_chat_store.UpdateChatUnsendDecrementParams{
			RecipientID: recipientID,
			Amount:      int32(len(messagesToUnsend)),
			ID:          chatID,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("[UnsendMessage] ERROR: Failed to commit transaction: %v", err)
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to commit unsend")
	}

	// Async file cleanup (after commit)
	for _, m := range messagesToUnsend {
		if m.FileID != nil && *m.FileID != "" {
			log.Printf("[UnsendMessage] Async deleting file %s", *m.FileID)
			go s.DeleteChatFile(context.Background(), *m.FileID)
		}
		if m.ThumbnailFileID != nil && *m.ThumbnailFileID != "" {
			log.Printf("[UnsendMessage] Async deleting thumbnail %s", *m.ThumbnailFileID)
			go s.DeleteChatFile(context.Background(), *m.ThumbnailFileID)
		}
	}

	return nil
}

func (s *chatService) UnsendMessageHandler(ctx context.Context, payload *UnsendMessagePayload, userID kit.UserId, isPrimary bool) error {
	chatID, err := uuid.Parse(payload.ChatID)
	if err != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_request", "Invalid chat ID")
	}

	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return kit.NewError(http.StatusBadRequest, "invalid_request", "Invalid message ID in list")
		}
		msgUUIDs = append(msgUUIDs, id)
	}

	return s.UnsendMessage(ctx, chatID, msgUUIDs, userID, isPrimary)
}

// ————————————————————————————————————————————————————————————————————————————————
// Delete For Me
// ————————————————————————————————————————————————————————————————————————————————

func (s *chatService) DeleteMessageForMe(ctx context.Context, messageIDs []uuid.UUID, userID kit.UserId, isPrimary bool) error {
	log.Printf("[DeleteMessageForMe] Processing %d messages for user %s (isPrimary=%v)", len(messageIDs), userID.UuidUserId, isPrimary)

	for _, msgID := range messageIDs {
		msg, err := s.PostgresQueries.GetMessageByID(ctx, msgID)
		if err != nil {
			// Message already deleted (e.g., by relay cleanup). Still try to clear the chat preview
			// by scanning the user's chats for a matching last_message_id — matches legacy behaviour.
			log.Printf("[DeleteMessageForMe] Message %s not found (likely relay-cleaned). Attempting preview clear.", msgID)
			chats, chatsErr := s.PostgresQueries.GetChatsByUserID(ctx, userID.UuidUserId)
			if chatsErr == nil {
				for _, chat := range chats {
					if chat.LastMessageID != nil && *chat.LastMessageID == msgID {
						_ = s.PostgresQueries.ClearLastMessageForParticipant(ctx, personal_chat_store.ClearLastMessageForParticipantParams{
							UserID:    userID.UuidUserId,
							ChatID:    chat.ID,
							MessageID: &msgID,
						})
						break
					}
				}
			}
			continue
		}

		// Clear preview for the participant who is deleting
		msgID := msg.ID
		_ = s.PostgresQueries.ClearLastMessageForParticipant(ctx, personal_chat_store.ClearLastMessageForParticipantParams{
			UserID:    userID.UuidUserId,
			ChatID:    msg.ChatID,
			MessageID: &msgID,
		})

		// Mark as deleted by the appropriate party
		if msg.SenderID == userID.UuidUserId {
			_ = s.PostgresQueries.MarkMessageDeletedBySender(ctx, personal_chat_store.MarkMessageDeletedBySenderParams{
				ID:       msgID,
				SenderID: userID.UuidUserId,
			})
		} else if msg.RecipientID == userID.UuidUserId {
			_ = s.PostgresQueries.MarkMessageDeletedByRecipient(ctx, personal_chat_store.MarkMessageDeletedByRecipientParams{
				ID:          msgID,
				RecipientID: userID.UuidUserId,
			})
		}

		// Create sync action if non-primary
		if !isPrimary {
			payload, _ := json.Marshal(SyncActionPayload{MessageIDs: []string{msgID.String()}, ChatID: msg.ChatID.String()})
			_, _ = s.PostgresQueries.CreateSyncAction(ctx, personal_chat_store.CreateSyncActionParams{
				ID:         uuid.New(),
				UserID:     userID.UuidUserId,
				ActionType: "delete_for_me",
				Payload:    payload,
			})
		}

		// Check if both parties have deleted — instant relay cleanup
		updatedMsg, err := s.PostgresQueries.GetMessageByID(ctx, msgID)
		if err == nil && updatedMsg.DeletedBySender && updatedMsg.DeletedByRecipient {
			log.Printf("[DeleteMessageForMe] Both parties deleted msg %s, cleaning up from relay", msgID)
			s.deleteMessageFromRelay(ctx, updatedMsg)
		}
	}

	return nil
}

func (s *chatService) DeleteMessageForMeHandler(ctx context.Context, payload *DeleteMessageForMePayload, userID kit.UserId, isPrimary bool) error {
	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return kit.NewError(http.StatusBadRequest, "invalid_request", "Invalid message ID in list")
		}
		msgUUIDs = append(msgUUIDs, id)
	}

	return s.DeleteMessageForMe(ctx, msgUUIDs, userID, isPrimary)
}

// ————————————————————————————————————————————————————————————————————————————————
// Sync Actions
// ————————————————————————————————————————————————————————————————————————————————

func (s *chatService) GetSyncActions(ctx context.Context, userID kit.UserId, limit int32) ([]personal_chat_store.MessageSyncAction, error) {
	actions, err := s.PostgresQueries.GetPendingSyncActions(ctx, personal_chat_store.GetPendingSyncActionsParams{
		UserID: userID.UuidUserId,
		Limit:  limit,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
	}
	return actions, nil
}

func (s *chatService) GetSyncActionsHandler(ctx context.Context, payload *GetSyncActionsPayload, userID kit.UserId) (*GetSyncActionsResponse, error) {
	limit := payload.Limit
	if limit <= 0 {
		limit = 50
	}

	actions, err := s.GetSyncActions(ctx, userID, limit)
	if err != nil {
		log.Printf("[PersonalChat] GetSyncActions failed for user %s: %v", userID.UuidUserId, err)
		return nil, err
	}

	respActions := make([]SyncActionResponse, 0, len(actions))
	for _, a := range actions {
		var payloadObj SyncActionPayload
		_ = json.Unmarshal(a.Payload, &payloadObj)

		respActions = append(respActions, SyncActionResponse{
			ID:                 a.ID.String(),
			UserID:             a.UserID.String(),
			ActionType:         a.ActionType,
			Payload:            payloadObj,
			DeliveredToPrimary: a.DeliveredToPrimary,
			CreatedAt:          a.CreatedAt,
		})
	}

	return &GetSyncActionsResponse{
		Actions: respActions,
		Count:   len(respActions),
	}, nil
}

func (s *chatService) AcknowledgeSyncAction(ctx context.Context, actionID uuid.UUID, isPrimary bool) error {
	if isPrimary {
		err := s.PostgresQueries.ConsumeSyncAction(ctx, actionID)
		if err != nil {
			return kit.NewError(http.StatusInternalServerError, "ack_failed", kit.GetPostgresError(err).Message)
		}
	}
	return nil
}

func (s *chatService) AcknowledgeSyncActionHandler(ctx context.Context, payload *AcknowledgeSyncActionPayload, isPrimary bool) error {
	actionID, err := uuid.Parse(payload.ActionID)
	if err != nil {
		return kit.NewError(http.StatusBadRequest, "invalid_request", "Invalid action ID")
	}

	return s.AcknowledgeSyncAction(ctx, actionID, isPrimary)
}

// â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
// Relay Cleanup
// â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

func (s *chatService) deleteMessageFromRelay(ctx context.Context, message personal_chat_store.Message) {
	messageID := message.ID
	log.Printf("[Relay-Cleanup] Message %s fully delivered and synced. Deleting from server.", messageID)

	// Capture file info before deleting message row
	fileID := ""
	if message.FileID != nil {
		fileID = *message.FileID
	}
	thumbID := ""
	if message.ThumbnailFileID != nil {
		thumbID = *message.ThumbnailFileID
	}

	// Hard delete from messages table
	err := s.PostgresQueries.DeleteMessage(ctx, messageID)
	if err != nil {
		log.Printf("[Relay-Cleanup] ERROR: Failed to delete row for %s: %v", messageID, err)
		return
	}

	// Async cleanup of files
	if fileID != "" || thumbID != "" {
		go func(fID, tID string) {
			if fID != "" {
				s.DeleteChatFile(context.Background(), fID)
			}
			if tID != "" {
				s.DeleteChatFile(context.Background(), tID)
			}
		}(fileID, thumbID)
	}
}

func (s *chatService) CleanupExpiredMessages(ctx context.Context) error {
	log.Printf("[CleanupJob] Starting cleanup of expired and orphaned (fully delivered) messages/files")

	// 1. Fetch expired messages that have files (batched to avoid memory issues)
	const batchSize = 100
	for {
		expiredMessages, err := s.PostgresQueries.GetExpiredMessagesWithFiles(ctx, batchSize)
		if err != nil {
			log.Printf("[CleanupJob] ERROR: Failed to fetch expired messages with files: %v", err)
			break
		}

		if len(expiredMessages) == 0 {
			break
		}

		log.Printf("[CleanupJob] Found %d expired/orphaned messages with files to clean up", len(expiredMessages))

		for _, msg := range expiredMessages {
			if msg.FileID != nil && *msg.FileID != "" {
				s.DeleteChatFile(ctx, *msg.FileID)
			}
			if msg.ThumbnailFileID != nil && *msg.ThumbnailFileID != "" {
				s.DeleteChatFile(ctx, *msg.ThumbnailFileID)
			}
		}

		if len(expiredMessages) < batchSize {
			break
		}
	}

	// 2. Perform bulk database deletion of ALL expired messages
	err := s.PostgresQueries.DeleteExpiredMessages(ctx)
	if err != nil {
		log.Printf("[CleanupJob] ERROR: Failed to bulk delete expired messages from DB: %v", err)
		return fmt.Errorf("failed to delete expired messages: %w", err)
	}

	// Also clean up old sync actions (Improvement over legacy)
	_ = s.PostgresQueries.DeleteOldSyncActions(ctx)

	log.Printf("[CleanupJob] Cleanup of expired/orphaned messages completed successfully")
	return nil
}

func (s *chatService) DropPendingMessagesBetweenUsers(ctx context.Context, user1ID, user2ID uuid.UUID) error {
	return s.PostgresQueries.DeletePendingMessagesBetweenUsers(ctx, personal_chat_store.DeletePendingMessagesBetweenUsersParams{
		SenderID:    user1ID,
		RecipientID: user2ID,
	})
}

