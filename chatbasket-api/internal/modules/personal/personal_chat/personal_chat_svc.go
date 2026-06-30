package personal_chat

import (
	"chatbasket-api/internal/modules/core/pending_uploads"
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/services"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ──────────────────────────────────────────────────────────────────────────────
// Provider Interfaces (matches core_auth pattern)
// ──────────────────────────────────────────────────────────────────────────────

type coreAuthChatProvider interface {
	IsSessionCentral(ctx context.Context, userID uuid.UUID, sessionToken string) (bool, error)
	GetUserPrimarySessionID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	GetSessionE2EEPublicKey(ctx context.Context, sessionID uuid.UUID) (*string, error)
}

type pendingUploadsChatProvider interface {
	Register(ctx context.Context, fileID, bucket, r2Key string, expiresAt time.Time) error
	Lookup(ctx context.Context, fileID string) (pending_uploads.PendingUpload, error)
	Remove(ctx context.Context, fileID string) error
	LookupTx(ctx context.Context, tx pgx.Tx, fileID string) (pending_uploads.PendingUpload, error)
	RemoveTx(ctx context.Context, tx pgx.Tx, fileID string) error
	RegisterTx(ctx context.Context, tx pgx.Tx, fileID, bucket, r2Key string, expiresAt time.Time) error
}

type personalProfilePersonalChatProvider interface {
	GetContactableProfilesForViewer(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error)
	GetUserCoreProfile(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error)
	GetE2EEPublicKey(ctx context.Context, targetUserID uuid.UUID) (*string, int32, error)
	GetActiveSessionKeysForUser(ctx context.Context, userID uuid.UUID) ([]string, error)
	IsUserAdminBlocked(ctx context.Context, userID uuid.UUID) (bool, error)
}

type personalContactPersonalChatProvider interface {
	IsAlreadyContact(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (bool, error)
	GetMessagingBlockStatus(ctx context.Context, user1ID uuid.UUID, user2ID uuid.UUID) (int32, error)
}

// ──────────────────────────────────────────────────────────────────────────────
// Chat Service
// ──────────────────────────────────────────────────────────────────────────────

type chatService struct {
	GlobalService    *services.GlobalService
	Pool            *pgxpool.Pool
	PostgresQuerier personal_chat_store.Querier
	PostgresQueries *personal_chat_store.Queries
	PendingUploads  pendingUploadsChatProvider
	AuthProvider    coreAuthChatProvider
	ProfileProvider personalProfilePersonalChatProvider
	ContactProvider personalContactPersonalChatProvider
	R2Pool          *clients.R2ClientPool
}

func NewChatService(
	globalService *services.GlobalService,
	pool *pgxpool.Pool,
	authProvider coreAuthChatProvider,
	profileProvider personalProfilePersonalChatProvider,
	contactProvider personalContactPersonalChatProvider,
	pendingUploads pendingUploadsChatProvider,
	r2Pool *clients.R2ClientPool,
) *chatService {
	store := personal_chat_store.New(pool)
	return &chatService{
		GlobalService:    globalService,
		Pool:            pool,
		PostgresQuerier: store,
		PostgresQueries: store,
		PendingUploads:  pendingUploads,
		AuthProvider:    authProvider,
		ProfileProvider: profileProvider,
		ContactProvider: contactProvider,
		R2Pool:          r2Pool,
	}
}

func (s *chatService) accountClient(accountName string) *clients.R2Client {
	return s.R2Pool.GetClientByAccount(accountName)
}

func (s *chatService) CheckMessagingEligibility(ctx context.Context, senderID kit.UserId, recipientID uuid.UUID) (string, *string, int32, error) {
	if senderID.UuidUserId == recipientID {
		return "", nil, 0, kit.NewError(http.StatusBadRequest, "invalid_recipient", "Cannot check eligibility with yourself")
	}
	coreProfile, err := s.ProfileProvider.GetUserCoreProfile(ctx, recipientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EligibilityRecipientNotFound, nil, 0, nil
		}
		if pe, ok := err.(kit.ProcessedError); ok && pe.Status() == http.StatusNotFound {
			return EligibilityRecipientNotFound, nil, 0, nil
		}
		return "", nil, 0, kit.NewError(http.StatusInternalServerError, "eligibility_check_failed", "failed to fetch recipient profile: "+err.Error())
	}
	isContact, err := s.ContactProvider.IsAlreadyContact(ctx, senderID.UuidUserId, recipientID)
	if err != nil {
		return "", nil, 0, kit.NewError(http.StatusInternalServerError, "eligibility_check_failed", "failed to verify contact relationship: "+err.Error())
	}
	if !isContact {
		return EligibilityNotInContacts, nil, 0, nil
	}
	if coreProfile.ProfileType == "private" {
		return EligibilityRecipientPrivate, nil, 0, nil
	}
	blockStatus, err := s.ContactProvider.GetMessagingBlockStatus(ctx, senderID.UuidUserId, recipientID)
	if err != nil {
		return "", nil, 0, kit.NewError(http.StatusInternalServerError, "eligibility_check_failed", "failed to verify block status: "+err.Error())
	}
	switch blockStatus {
	case 1:
		return EligibilityBlockedByMe, nil, 0, nil
	case 2:
		return EligibilityBlockedByRecipient, nil, 0, nil
	}
	senderBlocked, err := s.ProfileProvider.IsUserAdminBlocked(ctx, senderID.UuidUserId)
	if err != nil {
		return "", nil, 0, kit.NewError(http.StatusInternalServerError, "eligibility_check_failed", "failed to check sender admin status")
	}
	if senderBlocked {
		return EligibilityAdminBlocked, nil, 0, nil
	}
	recipientBlocked, err := s.ProfileProvider.IsUserAdminBlocked(ctx, recipientID)
	if err != nil {
		return "", nil, 0, kit.NewError(http.StatusInternalServerError, "eligibility_check_failed", "failed to check recipient admin status")
	}
	if recipientBlocked {
		return EligibilityAdminBlocked, nil, 0, nil
	}
	senderPrimaryID, err := s.AuthProvider.GetUserPrimarySessionID(ctx, senderID.UuidUserId)
	if err != nil {
		if pe, ok := err.(kit.ProcessedError); ok && pe.Status() == http.StatusNotFound {
			return EligibilityNoPrimaryDevice, nil, 0, nil
		}
		return "", nil, 0, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to check sender primary device")
	}
	if senderPrimaryID == uuid.Nil {
		return EligibilityNoPrimaryDevice, nil, 0, nil
	}
	recipientPrimaryID, err := s.AuthProvider.GetUserPrimarySessionID(ctx, recipientID)
	if err != nil {
		if pe, ok := err.(kit.ProcessedError); ok && pe.Status() == http.StatusNotFound {
			return EligibilityNoPrimaryDevice, nil, 0, nil
		}
		return "", nil, 0, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to check recipient primary device")
	}
	if recipientPrimaryID == uuid.Nil {
		return EligibilityNoPrimaryDevice, nil, 0, nil
	}
	recipientKey, recipientKeysRevision, keyErr := s.ProfileProvider.GetE2EEPublicKey(ctx, recipientID)
	if keyErr != nil || recipientKeysRevision == 0 {
		return EligibilityNoE2EE, nil, 0, nil
	}
	return EligibilityAllowed, recipientKey, recipientKeysRevision, nil
}

func (s *chatService) CheckEligibilityHandler(ctx context.Context, payload *CheckEligibilityPayload, userID kit.UserId) (*MessagingEligibilityResponse, error) {
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_recipient", "Invalid recipient ID")
	}
	eligibility, _, _, eligErr := s.CheckMessagingEligibility(ctx, userID, recipientID)
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

// createOrGetChatTx is the tx-bound variant used by ConfirmChatUpload.
func (s *chatService) createOrGetChatTx(ctx context.Context, qtx *personal_chat_store.Queries, user1ID, user2ID uuid.UUID) (*personal_chat_store.Chat, error) {
	chatID := uuid.New()
	chat, err := qtx.CreateChat(ctx, personal_chat_store.CreateChatParams{
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
	eligibility, _, _, eligErr := s.CheckMessagingEligibility(ctx, userID, recipientID)
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

func (s *chatService) checkRevisionStaleness(ctx context.Context, senderID kit.UserId, recipientID uuid.UUID, senderKeysRevision, recipientKeysRevision, recipientActualRevision int32) error {
	senderActualRevision := s.getSenderKeysRevision(ctx, senderID.UuidUserId)
	senderStale := senderKeysRevision != 0 && senderKeysRevision != senderActualRevision
	recipientStale := recipientKeysRevision != 0 && recipientKeysRevision != recipientActualRevision
	if !senderStale && !recipientStale {
		return nil
	}
	var side StaleSide
	if senderStale && recipientStale {
		side = StaleSideBoth
	} else if senderStale {
		side = StaleSideSender
	} else {
		side = StaleSideRecipient
	}
	details := StaleKeysErrorDetails{StaleSide: side}
	if senderStale {
		details.SenderKeysRevision = senderActualRevision
		senderKeys, _ := s.ProfileProvider.GetActiveSessionKeysForUser(ctx, senderID.UuidUserId)
		details.SenderActiveKeys = senderKeys
		log.Printf("[E2EE] ⚠️ Sender keys revision MISMATCH for user %s — client used %d, DB has %d", senderID.UuidUserId, senderKeysRevision, senderActualRevision)
	}
	if recipientStale {
		details.RecipientKeysRevision = recipientActualRevision
		recipientKeys, _ := s.ProfileProvider.GetActiveSessionKeysForUser(ctx, recipientID)
		details.RecipientActiveKeys = recipientKeys
		log.Printf("[E2EE] ⚠️ Recipient keys revision MISMATCH for user %s — client used %d, DB has %d", recipientID, recipientKeysRevision, recipientActualRevision)
	}
	return NewStaleKeysError(details)
}

func (s *chatService) SendMessage(ctx context.Context, params SendMessageParams) (*personal_chat_store.Message, error) {
	eligibility, _, recipientKeysRevision, err := s.CheckMessagingEligibility(ctx, params.SenderID, params.RecipientID)
	if err != nil {
		return nil, err
	}
	if eligibility != EligibilityAllowed {
		log.Printf("[E2EE] SendMessage: BLOCKED — eligibility=%s for sender %s → recipient %s", eligibility, params.SenderID.UuidUserId, params.RecipientID)
		return nil, messagingEligibilityError(eligibility)
	}
	log.Printf("[E2EE] SendMessage: eligibility OK — sender %s → recipient %s, client senderRev=%d recipientRev=%d, DB recipientRev=%d",
		params.SenderID.UuidUserId, params.RecipientID, params.SenderKeysRevision, params.RecipientKeysRevision, recipientKeysRevision)
	if staleErr := s.checkRevisionStaleness(ctx, params.SenderID, params.RecipientID, params.SenderKeysRevision, params.RecipientKeysRevision, recipientKeysRevision); staleErr != nil {
		log.Printf("[E2EE] SendMessage: STALE KEYS — sender %s recipient %s: %v", params.SenderID.UuidUserId, params.RecipientID, staleErr)
		return nil, staleErr
	}
	log.Printf("[E2EE] SendMessage: revision check PASSED — storing message from %s → %s", params.SenderID.UuidUserId, params.RecipientID)
	if utf8.RuneCountInString(params.Content) > 5000 {
		return nil, kit.NewError(http.StatusBadRequest, "content_too_long", "Message content cannot exceed 5000 characters")
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

func (s *chatService) getSenderKeysRevision(ctx context.Context, senderID uuid.UUID) int32 {
	_, revision, err := s.ProfileProvider.GetE2EEPublicKey(ctx, senderID)
	if err != nil {
		log.Printf("[E2EE] Failed to fetch sender keys revision for user %s: %v", senderID, err)
		return 0
	}
	return revision
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
		SenderID:              userID,
		RecipientID:           recipientID,
		Content:               payload.Content,
		MessageType:           payload.MessageType,
		IsPrimary:             isPrimary,
		RecipientKeysRevision: payload.RecipientKeysRevision,
		SenderKeysRevision:    payload.SenderKeysRevision,
	})
	if sendErr != nil {
		return nil, sendErr
	}
	return &MessageResponse{
		MessageID:             message.ID.String(),
		ChatID:                message.ChatID.String(),
		RecipientID:           message.RecipientID.String(),
		SenderKeysRevision:    s.getSenderKeysRevision(ctx, message.SenderID),
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

// ──────────────────────────────────────────────────────────────────────────────
// Delivery Acknowledgment
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) AcknowledgeDelivery(ctx context.Context, messageID uuid.UUID, acknowledgedBy string, sessionId string, userID kit.UserId) error {
	message, err := s.PostgresQueries.GetMessageByID(ctx, messageID)
	if err != nil {
		// Idempotency: If the message is not found, it may have already been fully acknowledged and deleted from the relay.
		log.Printf("[ACK] Message %s not found (likely already fully acknowledged and deleted). Treating as success.", messageID)
		return nil
	}
	isCentral, centralErr := s.AuthProvider.IsSessionCentral(ctx, userID.UuidUserId, sessionId)
	if centralErr != nil {
		if pe, ok := centralErr.(kit.ProcessedError); ok && pe.Status() == http.StatusUnauthorized {
			return kit.NewError(http.StatusForbidden, "session_invalid", "Forbidden: Session not found or invalid")
		}
		return centralErr
	}

	if acknowledgedBy == "recipient" {
		if err := s.PostgresQueries.MarkMessageDeliveredToRecipient(ctx, messageID); err != nil {
			return kit.NewError(http.StatusInternalServerError, "ack_failed_basic", kit.GetPostgresError(err).Message)
		}
		_ = s.PostgresQueries.UpdateChatLastDeliveredAt(ctx, personal_chat_store.UpdateChatLastDeliveredAtParams{
			ChatID:          message.ChatID,
			ParticipantID:   userID.UuidUserId,
			LastDeliveredAt: message.CreatedAt,
		})
		if isCentral {
			if err := s.PostgresQueries.MarkMessageDeliveredToRecipientPrimary(ctx, messageID); err != nil {
				log.Printf("[ACK] MarkMessageDeliveredToRecipientPrimary ERROR: %v", err)
			}
			if err := s.PostgresQueries.MarkOlderMessagesAsDeliveredToRecipientPrimary(ctx, personal_chat_store.MarkOlderMessagesAsDeliveredToRecipientPrimaryParams{
				ChatID:      message.ChatID,
				RecipientID: userID.UuidUserId,
				CreatedAt:   message.CreatedAt,
			}); err != nil {
				log.Printf("[ACK] MarkOlderMessagesAsDeliveredToRecipientPrimary ERROR: %v", err)
			}
		}
	} else {
		if !isCentral {
			return kit.NewError(http.StatusForbidden, "forbidden", "Forbidden: Only primary device can ACK sender sync")
		}
		if message.SenderID != userID.UuidUserId {
			return kit.NewError(http.StatusForbidden, "forbidden", "Forbidden: You are not the sender of this message")
		}
		if err := s.PostgresQueries.MarkMessageSyncedToSenderPrimary(ctx, messageID); err != nil {
			log.Printf("[ACK] MarkMessageSyncedToSenderPrimary ERROR: %v", err)
		}
	}

	updatedMessage, err := s.PostgresQueries.GetMessageByID(ctx, messageID)
	if err == nil {
		recipientPrimaryDelivered := updatedMessage.DeliveredToRecipientPrimary != nil && *updatedMessage.DeliveredToRecipientPrimary
		senderPrimarySynced := updatedMessage.SyncedToSenderPrimary
		if recipientPrimaryDelivered && senderPrimarySynced {
			s.deleteMessageFromRelay(ctx, updatedMessage)
		}
		if err := s.PostgresQueries.CleanupOlderFullyAcknowledgedMessages(ctx, personal_chat_store.CleanupOlderFullyAcknowledgedMessagesParams{
			ChatID:    updatedMessage.ChatID,
			CreatedAt: updatedMessage.CreatedAt,
		}); err != nil {
			log.Printf("[ACK] CleanupOlderFullyAcknowledgedMessages ERROR: %v", err)
		}
	}
	return nil
}

func (s *chatService) AcknowledgeDeliveryHandler(ctx context.Context, payload *AcknowledgeDeliveryPayload, userID kit.UserId, sessionId string) (*AcknowledgeDeliveryResponse, error) {
	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_message_id", "Invalid message ID")
	}
	if !payload.Success {
		return &AcknowledgeDeliveryResponse{Acknowledged: false}, nil
	}
	if err := s.AcknowledgeDelivery(ctx, messageID, payload.AcknowledgedBy, sessionId, userID); err != nil {
		return nil, err
	}
	return &AcknowledgeDeliveryResponse{Acknowledged: true}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Message Queries
// ──────────────────────────────────────────────────────────────────────────────

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
		SenderKeysRevision:          s.getSenderKeysRevision(ctx, msg.SenderID),
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
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "You are not a participant in this chat")
	}
	messages, err := s.GetChatMessages(ctx, chatID, userID, payload.Limit, payload.Offset)
	if err != nil {
		return nil, err
	}
	msgResponses := make([]MessageResponse, 0, len(messages))
	for _, msg := range messages {
		msgResponses = append(msgResponses, s.buildMessageResponse(ctx, msg, userID))
	}
	chat, chatErr := s.PostgresQueries.GetChatByID(ctx, chatID)
	if chatErr != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", "Failed to fetch chat")
	}
	var otherReadAt, otherDeliveredAt time.Time
	if chat.Participant1ID == userID.UuidUserId {
		otherReadAt = kit.DerefTime(chat.P2LastReadAt)
		otherDeliveredAt = kit.DerefTime(chat.P2LastDeliveredAt)
	} else {
		otherReadAt = kit.DerefTime(chat.P1LastReadAt)
		otherDeliveredAt = kit.DerefTime(chat.P1LastDeliveredAt)
	}
	return &GetMessagesResponse{
		Messages:                 msgResponses,
		Count:                    len(msgResponses),
		OtherUserLastReadAt:      otherReadAt,
		OtherUserLastDeliveredAt: otherDeliveredAt,
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Chat List
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) GetUserChatsLite(ctx context.Context, userID uuid.UUID) ([]personal_chat_store.GetUserChatsLiteRow, error) {
	return s.PostgresQueries.GetUserChatsLite(ctx, userID)
}

func (s *chatService) GetUserChatsHandler(ctx context.Context, userID kit.UserId) (*GetUserChatsResponse, error) {
	chats, err := s.GetUserChatsLite(ctx, userID.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
	}
	chatResponses := make([]ChatResponse, 0, len(chats))
	for _, chat := range chats {
		var otherUserID uuid.UUID
		if chat.Participant1ID == userID.UuidUserId {
			otherUserID = chat.Participant2ID
		} else {
			otherUserID = chat.Participant1ID
		}
		contactProfile, _ := s.ProfileProvider.GetContactableProfilesForViewer(ctx, userID.UuidUserId, []uuid.UUID{otherUserID})
		otherName := ""
		var avatarURL *string
		var avatarFileID *string
		if cp, ok := contactProfile[otherUserID]; ok && cp != nil {
			otherName = cp.Name
			avatarURL = cp.AvatarURL
			avatarFileID = cp.AvatarFileId
		}
		var lastMessageContent *string
		var lastMessageType *string
		var lastMessageCreatedAt *time.Time
		var lastMessageSenderID *string
		var lastMessageID *string
		var otherUserLastReadAt, otherUserLastDeliveredAt time.Time
		var otherUserKeysRevision int32
		if chat.Participant1ID == userID.UuidUserId {
			lastMessageContent = chat.P2LastMessageContent
			lastMessageType = chat.P2LastMessageType
			lastMessageCreatedAt = chat.LastMessageCreatedAt
			otherUserLastReadAt = kit.DerefTime(chat.P2LastReadAt)
			otherUserLastDeliveredAt = kit.DerefTime(chat.P2LastDeliveredAt)
			if chat.LastMessageSenderID != nil {
				s := chat.LastMessageSenderID.String()
				lastMessageSenderID = &s
			}
			if chat.LastMessageID != nil {
				s := chat.LastMessageID.String()
				lastMessageID = &s
			}
		} else {
			lastMessageContent = chat.P1LastMessageContent
			lastMessageType = chat.P1LastMessageType
			lastMessageCreatedAt = chat.LastMessageCreatedAt
			otherUserLastReadAt = kit.DerefTime(chat.P1LastReadAt)
			otherUserLastDeliveredAt = kit.DerefTime(chat.P1LastDeliveredAt)
			if chat.LastMessageSenderID != nil {
				s := chat.LastMessageSenderID.String()
				lastMessageSenderID = &s
			}
			if chat.LastMessageID != nil {
				s := chat.LastMessageID.String()
				lastMessageID = &s
			}
		}
		if cp, ok := contactProfile[otherUserID]; ok && cp != nil {
			otherUserKeysRevision = cp.KeysRevision
		}
		lastMessageIsUnsent := false
		lastMessageStatus := "sent"
		chatResponses = append(chatResponses, ChatResponse{
			ChatID:                   chat.ID.String(),
			OtherUserID:              otherUserID.String(),
			OtherUserName:            otherName,
			AvatarURL:                avatarURL,
			AvatarFileId:             avatarFileID,
			CreatedAt:                chat.CreatedAt,
			UpdatedAt:                chat.UpdatedAt,
			OtherUserLastReadAt:      otherUserLastReadAt,
			OtherUserLastDeliveredAt: otherUserLastDeliveredAt,
			LastMessageContent:       lastMessageContent,
			LastMessageCreatedAt:     lastMessageCreatedAt,
			LastMessageType:          lastMessageType,
			LastMessageSenderID:      lastMessageSenderID,
			LastMessageIsFromMe:      lastMessageSenderID != nil && *lastMessageSenderID == userID.StringUserId,
			LastMessageStatus:        lastMessageStatus,
			LastMessageIsUnsent:      lastMessageIsUnsent,
			LastMessageID:            lastMessageID,
			UnreadCount:              int(chat.UnreadCount),
			OtherUserKeysRevision:    otherUserKeysRevision,
		})
	}
	return &GetUserChatsResponse{
		Chats: chatResponses,
		Count: len(chatResponses),
	}, nil
}

func (s *chatService) IsChatParticipant(ctx context.Context, chatID, userID uuid.UUID) (bool, error) {
	isParticipant, err := s.PostgresQueries.IsChatParticipant(ctx, personal_chat_store.IsChatParticipantParams{
		Column1: chatID,
		Column2: userID,
	})
	if err != nil {
		return false, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
	}
	if isParticipant {
		return true, nil
	}
	_, _ = s.PostgresQueries.IsChatParticipant(ctx, personal_chat_store.IsChatParticipantParams{
		Column1: chatID,
		Column2: uuid.Nil,
	})
	chat, err := s.PostgresQueries.GetChatByID(ctx, chatID)
	if err != nil {
		return false, kit.NewError(http.StatusNotFound, "not_found", "chat not found")
	}
	return chat.Participant1ID == userID || chat.Participant2ID == userID, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Mark Read
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) MarkChatRead(ctx context.Context, userID kit.UserId, chatID uuid.UUID, isPrimary bool) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to begin tx")
	}
	defer tx.Rollback(ctx)
	qtx := s.PostgresQueries.WithTx(tx)
	if err := qtx.ResetChatReadStatus(ctx, personal_chat_store.ResetChatReadStatusParams{
		ID:             chatID,
		Participant1ID: userID.UuidUserId,
	}); err != nil {
		return kit.NewError(http.StatusInternalServerError, "mark_read_failed", kit.GetPostgresError(err).Message)
	}
	if isPrimary {
		if err := qtx.MarkChatMessagesAsReadPrimary(ctx, personal_chat_store.MarkChatMessagesAsReadPrimaryParams{
			ChatID:      chatID,
			RecipientID: userID.UuidUserId,
		}); err != nil {
			return kit.NewError(http.StatusInternalServerError, "mark_read_failed", kit.GetPostgresError(err).Message)
		}
	} else {
		if err := qtx.MarkChatMessagesAsRead(ctx, personal_chat_store.MarkChatMessagesAsReadParams{
			ChatID:      chatID,
			RecipientID: userID.UuidUserId,
		}); err != nil {
			return kit.NewError(http.StatusInternalServerError, "mark_read_failed", kit.GetPostgresError(err).Message)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to commit")
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

// ──────────────────────────────────────────────────────────────────────────────
// Unsend (no R2 file operations needed - DB only)
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) UnsendMessage(ctx context.Context, chatID uuid.UUID, messageIDs []uuid.UUID, senderID kit.UserId, isPrimary bool) error {
	log.Printf("[UnsendMessage] START: Processing %d messages for sender %s in chat %s (isPrimary=%v)", len(messageIDs), senderID.UuidUserId, chatID, isPrimary)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to start unsend transaction")
	}
	defer tx.Rollback(ctx)
	qtx := s.PostgresQueries.WithTx(tx)
	chat, err := qtx.GetChatByID(ctx, chatID)
	if err != nil {
		return kit.NewError(http.StatusNotFound, "not_found", "chat not found")
	}
	var recipientID uuid.UUID
	if chat.Participant1ID == senderID.UuidUserId {
		recipientID = chat.Participant2ID
	} else if chat.Participant2ID == senderID.UuidUserId {
		recipientID = chat.Participant1ID
	} else {
		return kit.NewError(http.StatusForbidden, "forbidden", "unauthorized: not a participant in this chat")
	}
	messagesToUnsend := make([]personal_chat_store.Message, 0, len(messageIDs))
	for _, msgID := range messageIDs {
		msg, err := qtx.GetMessageByID(ctx, msgID)
		if err != nil {
			log.Printf("[UnsendMessage] Message %s not found. Falling back to sync-only.", msgID)
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
		} else {
			if msg.SenderID != senderID.UuidUserId {
				return kit.NewError(http.StatusForbidden, "forbidden", "unauthorized: you can only unsend your own messages")
			}
			if msg.MessageType == "unsent" {
				continue
			}
			messagesToUnsend = append(messagesToUnsend, msg)
			if err := qtx.UpdateMessageToUnsent(ctx, msg.ID); err != nil {
				return kit.NewError(http.StatusInternalServerError, "server_error", "failed to create message tombstone")
			}
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
		_ = qtx.UpdateChatUnsendPreview(ctx, personal_chat_store.UpdateChatUnsendPreviewParams{
			ID:            chatID,
			LastMessageID: &msgID,
		})
		_ = qtx.UpdateChatUnsendDecrement(ctx, personal_chat_store.UpdateChatUnsendDecrementParams{
			ID:          chatID,
			RecipientID: recipientID,
			Amount:      1,
		})
	}
	if err := tx.Commit(ctx); err != nil {
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to commit unsend")
	}
	for _, m := range messagesToUnsend {
		if m.FileID != nil && *m.FileID != "" {
			go func(fileID string, msgID uuid.UUID) {
				if err := s.DeleteChatFile(context.Background(), fileID); err == nil {
					_ = s.PostgresQueries.ClearMessageFileFields(context.Background(), msgID)
				} else {
					log.Printf("[UnsendMessage] WARNING: Chat file %s R2 delete failed: %v (relying on sweeper)", fileID, err)
				}
			}(*m.FileID, m.ID)
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

// ──────────────────────────────────────────────────────────────────────────────
// Delete For Me
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) DeleteMessageForMe(ctx context.Context, messageIDs []uuid.UUID, userID kit.UserId, isPrimary bool) error {
	for _, msgID := range messageIDs {
		msg, err := s.PostgresQueries.GetMessageByID(ctx, msgID)
		if err == nil {
			_ = s.PostgresQueries.ClearLastMessageForParticipant(ctx, personal_chat_store.ClearLastMessageForParticipantParams{
				UserID:    userID.UuidUserId,
				ChatID:    msg.ChatID,
				MessageID: &msgID,
			})
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
		}
	}
	if !isPrimary {
		for _, msgID := range messageIDs {
			payload, _ := json.Marshal(SyncActionPayload{MessageIDs: []string{msgID.String()}})
			_, err := s.PostgresQueries.CreateSyncAction(ctx, personal_chat_store.CreateSyncActionParams{
				ID:         uuid.New(),
				UserID:     userID.UuidUserId,
				ActionType: "delete_for_me",
				Payload:    payload,
			})
			if err != nil {
				return kit.NewError(http.StatusInternalServerError, "server_error", "failed to create sync action")
			}
		}
	}
	for _, msgID := range messageIDs {
		msg, err := s.PostgresQueries.GetMessageByID(ctx, msgID)
		if err != nil {
			continue
		}
		shouldDeleteNow := false
		if msg.SenderID == userID.UuidUserId {
			if msg.DeliveredToRecipientPrimary != nil && *msg.DeliveredToRecipientPrimary {
				shouldDeleteNow = true
			}
		} else if msg.RecipientID == userID.UuidUserId {
			if msg.SyncedToSenderPrimary {
				shouldDeleteNow = true
			}
		}
		if shouldDeleteNow {
			s.deleteMessageFromRelay(ctx, msg)
		}
	}
	return nil
}

func (s *chatService) DeleteMessageForMeHandler(ctx context.Context, payload *DeleteMessageForMePayload, userID kit.UserId, isPrimary bool) error {
	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return kit.NewError(http.StatusBadRequest, "invalid_request", "Invalid message ID")
		}
		msgUUIDs = append(msgUUIDs, id)
	}
	return s.DeleteMessageForMe(ctx, msgUUIDs, userID, isPrimary)
}

// ──────────────────────────────────────────────────────────────────────────────
// Sync Actions
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) GetSyncActions(ctx context.Context, userID kit.UserId, limit int32) ([]personal_chat_store.MessageSyncAction, error) {
	return s.PostgresQueries.GetPendingSyncActions(ctx, personal_chat_store.GetPendingSyncActionsParams{
		UserID: userID.UuidUserId,
		Limit:  limit,
	})
}

func (s *chatService) GetSyncActionsHandler(ctx context.Context, payload *GetSyncActionsPayload, userID kit.UserId) (*GetSyncActionsResponse, error) {
	limit := payload.Limit
	if limit <= 0 {
		limit = 50
	}
	actions, err := s.GetSyncActions(ctx, userID, limit)
	if err != nil {
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
		if err := s.PostgresQueries.ConsumeSyncAction(ctx, actionID); err != nil {
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

// ──────────────────────────────────────────────────────────────────────────────
// Pending Messages
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) GetPendingMessagesHandler(ctx context.Context, payload *GetPendingMessagesPayload, userID kit.UserId) (*GetMessagesResponse, error) {
	limit := payload.Limit
	if limit <= 0 {
		limit = 100
	}
	messages, err := s.PostgresQueries.GetPendingMessagesForRecipient(ctx, personal_chat_store.GetPendingMessagesForRecipientParams{
		RecipientID: userID.UuidUserId,
		Limit:       limit,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
	}
	msgResponses := make([]MessageResponse, 0, len(messages))
	for _, msg := range messages {
		msgResponses = append(msgResponses, s.buildMessageResponse(ctx, msg, userID))
	}
	return &GetMessagesResponse{
		Messages: msgResponses,
		Count:    len(msgResponses),
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Relay Cleanup
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) deleteMessageFromRelay(ctx context.Context, message personal_chat_store.Message) {
	messageID := message.ID
	log.Printf("[Relay-Cleanup] Message %s fully delivered and synced. Deleting from server.", messageID)
	fileID := ""
	if message.FileID != nil {
		fileID = *message.FileID
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		log.Printf("[Relay-Cleanup] ERROR: Failed to start transaction for message %s: %v", messageID, err)
		return
	}
	defer tx.Rollback(ctx)
	qtx := s.PostgresQueries.WithTx(tx)

	if fileID != "" {
		_, objectKey := clients.ParseFilePrefix(fileID)
		client := s.R2Pool.GetClient(fileID)
		bucket := client.ChatBucket()
		if err := s.PendingUploads.RegisterTx(ctx, tx, fileID, bucket, objectKey, time.Now().UTC()); err != nil {
			log.Printf("[Relay-Cleanup] ERROR: Failed to register file %s in pending_uploads: %v", fileID, err)
			return
		}
	}

	if err := qtx.DeleteMessage(ctx, messageID); err != nil {
		log.Printf("[Relay-Cleanup] ERROR: Failed to delete message %s: %v", messageID, err)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("[Relay-Cleanup] ERROR: Failed to commit transaction for message %s: %v", messageID, err)
		return
	}

	if fileID != "" {
		go func(fID string) {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := s.DeleteChatFile(bgCtx, fID); err == nil {
				if err := s.PendingUploads.Remove(context.Background(), fID); err != nil {
					log.Printf("[Relay-Cleanup] WARNING: Failed to remove file %s from pending_uploads: %v", fID, err)
				} else {
					log.Printf("[Relay-Cleanup] Inline R2 delete + pending_uploads removal succeeded for chat file %s", fID)
				}
			} else {
				log.Printf("[Relay-Cleanup] WARNING: Inline chat file %s R2 delete failed: %v (sweeper will retry)", fID, err)
			}
		}(fileID)
	}
}

func (s *chatService) CleanupExpiredMessages(ctx context.Context) error {
	log.Printf("[CleanupJob] Starting cleanup of expired and orphaned messages/files")
	const batchSize = 100
	lastExpiredID := uuid.Nil
	for {
		expiredMessages, err := s.PostgresQueries.GetExpiredMessagesWithFiles(ctx, personal_chat_store.GetExpiredMessagesWithFilesParams{
			Limit:  int32(batchSize),
			LastID: lastExpiredID,
		})
		if err != nil {
			log.Printf("[CleanupJob] ERROR: Failed to fetch expired messages with files: %v", err)
			break
		}
		if len(expiredMessages) == 0 {
			break
		}
		// Concurrent R2 deletes — 20-way parallelism (kit.MaxConcurrentDeletes).
		// Returns errors[i] indexed by fileID position. nil = R2 success or "not found" (idempotent).
		fileIDs := make([]string, 0, len(expiredMessages))
		msgByFileID := make(map[string]personal_chat_store.Message, len(expiredMessages))
		for _, msg := range expiredMessages {
			if msg.FileID != nil && *msg.FileID != "" {
				fileIDs = append(fileIDs, *msg.FileID)
				msgByFileID[*msg.FileID] = msg
			}
		}
		r2Errors := kit.DeleteFilesBatch(ctx, fileIDs, s.DeleteChatFile)
		for i, fileID := range fileIDs {
			if r2Errors[i] != nil {
				// R2 failed (real error) — skip DB delete, retry next cycle
				log.Printf("[CleanupJob] WARNING: Chat file %s R2 delete failed: %v", fileID, r2Errors[i])
				continue
			}
			// R2 succeeded (or file was already gone)
			msg := msgByFileID[fileID]
			if msg.MessageType == "unsent" {
				// Safe to clear file fields while keeping the tombstone
				if err := s.PostgresQueries.ClearMessageFileFields(ctx, msg.ID); err != nil {
					log.Printf("[CleanupJob] WARNING: Failed to clear expired unsent message file fields %s: %v", msg.ID, err)
				}
			} else {
				// Safe to delete DB row completely
				if err := s.PostgresQueries.DeleteMessage(ctx, msg.ID); err != nil {
					log.Printf("[CleanupJob] WARNING: Failed to delete expired message record %s: %v", msg.ID, err)
				}
			}
		}
		// Also delete messages without files (no R2 step needed)
		for _, msg := range expiredMessages {
			if msg.FileID == nil || *msg.FileID == "" {
				if msg.MessageType == "unsent" {
					if err := s.PostgresQueries.ClearMessageFileFields(ctx, msg.ID); err != nil {
						log.Printf("[CleanupJob] WARNING: Failed to clear expired unsent message file fields %s: %v", msg.ID, err)
					}
				} else {
					if err := s.PostgresQueries.DeleteMessage(ctx, msg.ID); err != nil {
						log.Printf("[CleanupJob] WARNING: Failed to delete expired message record %s: %v", msg.ID, err)
					}
				}
			}
		}
		if len(expiredMessages) < batchSize {
			break
		}
		lastExpiredID = expiredMessages[len(expiredMessages)-1].ID
	}
	lastBlockedID := uuid.Nil
	for {
		blockedMessages, err := s.PostgresQueries.GetMessagesWithFilesForBlockedUsers(ctx, personal_chat_store.GetMessagesWithFilesForBlockedUsersParams{
			Limit:  int32(batchSize),
			LastID: lastBlockedID,
		})
		if err != nil {
			log.Printf("[CleanupJob] ERROR: Failed to fetch blocked-user messages with files: %v", err)
			break
		}
		if len(blockedMessages) == 0 {
			break
		}
		// Concurrent R2 deletes (same pattern as above)
		fileIDs := make([]string, 0, len(blockedMessages))
		msgByFileID := make(map[string]uuid.UUID, len(blockedMessages))
		for _, msg := range blockedMessages {
			if msg.FileID != nil && *msg.FileID != "" {
				fileIDs = append(fileIDs, *msg.FileID)
				msgByFileID[*msg.FileID] = msg.ID
			}
		}
		r2Errors := kit.DeleteFilesBatch(ctx, fileIDs, s.DeleteChatFile)
		for i, fileID := range fileIDs {
			if r2Errors[i] != nil {
				log.Printf("[CleanupJob] WARNING: Chat file %s R2 delete failed: %v", fileID, r2Errors[i])
				continue
			}
			if err := s.PostgresQueries.DeleteMessage(ctx, msgByFileID[fileID]); err != nil {
				log.Printf("[CleanupJob] WARNING: Failed to delete blocked-user message record %s: %v", msgByFileID[fileID], err)
			}
		}
		for _, msg := range blockedMessages {
			if msg.FileID == nil || *msg.FileID == "" {
				if err := s.PostgresQueries.DeleteMessage(ctx, msg.ID); err != nil {
					log.Printf("[CleanupJob] WARNING: Failed to delete blocked-user message record %s: %v", msg.ID, err)
				}
			}
		}
		if len(blockedMessages) < batchSize {
			break
		}
		lastBlockedID = blockedMessages[len(blockedMessages)-1].ID
	}
	log.Printf("[CleanupJob] Starting final bulk sweep for safe records (no R2 files)")
	// Safe bulk deletes: messages WITHOUT files. Messages WITH files were already
	// handled by the batched loop above (R2 deleted first, then DB row).
	// Three cases (matches original DeleteExpiredMessages + blocked-user cleanup):
	//   1. Expired by TTL (expires_at < now())
	//   2. Fully acknowledged by both primary devices (delivered + synced)
	//   3. In a blocked-user chat
	if err := s.PostgresQueries.DeleteExpiredMessagesWithoutFiles(ctx); err != nil {
		log.Printf("[CleanupJob] ERROR: Failed to bulk delete expired messages without files: %v", err)
	}
	if err := s.PostgresQueries.DeleteFullyAcknowledgedMessagesWithoutFiles(ctx); err != nil {
		log.Printf("[CleanupJob] ERROR: Failed to bulk delete fully-acknowledged messages without files: %v", err)
	}
	if err := s.PostgresQueries.DeleteBlockedUserMessagesWithoutFiles(ctx); err != nil {
		log.Printf("[CleanupJob] ERROR: Failed to bulk delete blocked-user messages without files: %v", err)
	}
	// Safe sync action bulk deletes (sync actions have no R2 files).
	if err := s.PostgresQueries.CleanupSyncActionsForBlockedUsers(ctx); err != nil {
		log.Printf("[CleanupJob] ERROR: Failed to bulk delete blocked-user sync actions: %v", err)
	}
	_ = s.PostgresQueries.DeleteOldSyncActions(ctx)

	// Clean up expired history sync requests (lazy expiry 410 enforces TTL instantly, sweep reclaims space)
	if err := s.PostgresQueries.DeleteExpiredHistorySync(ctx); err != nil {
		log.Printf("[CleanupJob] ERROR: Failed to delete expired history syncs: %v", err)
	}

	log.Printf("[CleanupJob] Cleanup process completed successfully")
	return nil
}
