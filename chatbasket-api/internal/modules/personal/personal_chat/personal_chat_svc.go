package personal_chat

import (
	rpc_common_modelv1 "chatbasket-api/gen/proto/common/model"
	rpc_personal_chatv1 "chatbasket-api/gen/proto/personal/personal_chat"
	"chatbasket-api/internal/modules/core/pending_uploads"
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/services"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	GetContactableUserIDs(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) ([]uuid.UUID, error)
}

type personalContactPersonalChatProvider interface {
	IsAlreadyContact(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (bool, error)
	GetMessagingBlockStatus(ctx context.Context, user1ID uuid.UUID, user2ID uuid.UUID) (int32, error)
}

// ──────────────────────────────────────────────────────────────────────────────
// Chat Service
// ──────────────────────────────────────────────────────────────────────────────

type chatService struct {
	GlobalService   *services.GlobalService
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
		GlobalService:   globalService,
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

func (s *chatService) CheckEligibilityHandler(ctx context.Context, payload *CheckEligibilityPayload, userID kit.UserId) (*rpc_personal_chatv1.CheckEligibilityResponse, error) {
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_recipient", "Invalid recipient ID")
	}
	eligibility, _, _, eligErr := s.CheckMessagingEligibility(ctx, userID, recipientID)
	if eligErr != nil {
		return nil, eligErr
	}
	resp := &rpc_personal_chatv1.CheckEligibilityResponse{
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

func (s *chatService) CreateChatHandler(ctx context.Context, payload *CreateChatPayload, userID kit.UserId) (*rpc_personal_chatv1.CreateChatResponse, error) {
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
	contactProfile, _ := s.ProfileProvider.GetContactableProfilesForViewer(ctx, userID.UuidUserId, []uuid.UUID{recipientID})
	// Privacy exclusion: if the contactable profile lookup omits the user
	// (admin-blocked / private profile / user-blocked either way), the map
	// lookup below misses and the empty initials stand. The wire shape the
	// frontend relies on is name/username/profile_type as `""` and
	// bio/avatar_url/avatar_file_id as `null` — see
	// profileService.GetContactableProfilesForViewer for the contract.
	otherName := ""
	otherUsername := ""
	var otherBio *string
	var avatarURL *string
	var avatarFileID *string
	otherProfileType := ""
	var otherUserKeysRevision int32
	if cp, ok := contactProfile[recipientID]; ok && cp != nil {
		otherName = cp.Name
		otherUsername = cp.Username
		otherBio = cp.Bio
		avatarURL = cp.AvatarURL
		avatarFileID = cp.AvatarFileId
		otherProfileType = cp.ProfileType
		otherUserKeysRevision = cp.KeysRevision
	}
	return &rpc_personal_chatv1.CreateChatResponse{
		ChatId:                   chat.ID.String(),
		OtherUserId:              recipientID.String(),
		OtherUserName:            otherName,
		OtherUserUsername:        otherUsername,
		OtherUserBio:             otherBio,
		AvatarUrl:                avatarURL,
		AvatarFileId:             avatarFileID,
		CreatedAt:                timestamppb.New(chat.CreatedAt),
		UpdatedAt:                timestamppb.New(chat.UpdatedAt),
		OtherUserLastReadAt:      timestamppb.New(otherReadAt),
		OtherUserLastDeliveredAt: timestamppb.New(otherDeliveredAt),
		LastMessageIsFromMe:      false,
		OtherUserKeysRevision:    otherUserKeysRevision,
		ProfileType:              otherProfileType,
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
	details := &rpc_common_modelv1.StaleKeysErrorDetails{StaleSide: string(side)}
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
	return kit.NewErrorWithDetails(http.StatusConflict, "keys_stale", fmt.Sprintf("keys_revision is stale (side: %s)", side), details)
}

func (s *chatService) SendMessage(ctx context.Context, params SendMessageParams) (*personal_chat_store.Message, error) {
	// Idempotency check:
	if s.PostgresQueries != nil && params.MessageID != uuid.Nil {
		if exists, err := s.PostgresQueries.CheckMessageExists(ctx, params.MessageID); err == nil && exists {
			log.Printf("[E2EE] SendMessage: IDEMPOTENT RETRY — message %s already exists. Fetching existing message.", params.MessageID)
			if existingMsg, err := s.PostgresQueries.GetMessageByID(ctx, params.MessageID); err == nil {
				return &existingMsg, nil
			}
		}
	}
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
	expiresAt := time.Now().Add(DefaultMessageTTL)
	message, dbErr := s.PostgresQueries.CreateMessage(ctx, personal_chat_store.CreateMessageParams{
		ID:                          params.MessageID,
		ChatID:                      chat.ID,
		SenderID:                    params.SenderID.UuidUserId,
		RecipientID:                 params.RecipientID,
		Content:                     params.Content,
		MessageType:                 params.MessageType,
		ExpiresAt:                   expiresAt,
		SyncedToSenderPrimary:       params.IsPrimary,
		DeliveredToRecipientPrimary: false,
	})
	if dbErr != nil {
		// Handle PK duplicate key violation (race condition: both WS and REST passed CheckMessageExists)
		pgErr := kit.GetPostgresError(dbErr)
		if pgErr.PgError != nil && pgErr.PgError.Code == "23505" {
			log.Printf("[E2EE] SendMessage: PK CONFLICT (race) — message %s inserted concurrently. Fetching existing.", params.MessageID)
			if existingMsg, err := s.PostgresQueries.GetMessageByID(ctx, params.MessageID); err == nil {
				return &existingMsg, nil
			}
		}
		return nil, kit.NewError(http.StatusInternalServerError, "message_send_failed", pgErr.Message)
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

func (s *chatService) SendMessageHandler(ctx context.Context, payload *SendMessagePayload, userID kit.UserId, isPrimary bool) (*rpc_personal_chatv1.Message, error) {
	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_message_id", "Invalid message id")
	}
	recipientID, err := uuid.Parse(payload.RecipientID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_recipient", "Invalid recipient id")
	}
	if userID.UuidUserId == recipientID {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_recipient", "Cannot send message to yourself")
	}
	message, sendErr := s.SendMessage(ctx, SendMessageParams{
		MessageID:             messageID,
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
	return &rpc_personal_chatv1.Message{
		MessageId:             message.ID.String(),
		ChatId:                message.ChatID.String(),
		RecipientId:           message.RecipientID.String(),
		SenderKeysRevision:    s.getSenderKeysRevision(ctx, message.SenderID),
		Content:               message.Content,
		MessageType:           message.MessageType,
		DeliveredToRecipient:  message.DeliveredToRecipient,
		SyncedToSenderPrimary: message.SyncedToSenderPrimary,
		CreatedAt:             timestamppb.New(message.CreatedAt),
		ExpiresAt:             timestamppb.New(message.ExpiresAt),
		IsFromMe:              true,
		FileId:                message.FileID,
		FileName:              message.FileName,
		FileSize:              message.FileSize,
		FileMimeType:          message.FileMimeType,
		ReadByRecipient:       message.ReadByRecipient,
		ReadAckedBySender:     message.ReadAckedBySender,
		ReadAt:                kit.OptionalTimestamp(message.ReadAt),
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Delivery Acknowledgment
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) AcknowledgeDelivery(ctx context.Context, messageID uuid.UUID, acknowledgedBy string, sessionId string, userID kit.UserId) error {
	message, err := s.PostgresQueries.GetMessageByID(ctx, messageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	isCentral, centralErr := s.AuthProvider.IsSessionCentral(ctx, userID.UuidUserId, sessionId)
	if centralErr != nil {
		if pe, ok := centralErr.(kit.ProcessedError); ok && pe.Status() == http.StatusUnauthorized {
			return kit.NewError(http.StatusForbidden, "session_invalid", "Forbidden: Session not found or invalid")
		}
		return centralErr
	}

	if acknowledgedBy == "recipient" {
		if message.RecipientID != userID.UuidUserId {
			return kit.NewError(http.StatusForbidden, "forbidden", "Forbidden: You are not the recipient of this message")
		}
		if err := s.PostgresQueries.MarkMessageDeliveredToRecipient(ctx, messageID); err != nil {
			return kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
		}
		_ = s.PostgresQueries.UpdateChatLastDeliveredAt(ctx, personal_chat_store.UpdateChatLastDeliveredAtParams{
			ChatID:          message.ChatID,
			ParticipantID:   userID.UuidUserId,
			LastDeliveredAt: message.CreatedAt,
		})
		if isCentral {
			if err := s.PostgresQueries.MarkMessageDeliveredToRecipientPrimary(ctx, messageID); err != nil {
				return kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
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
			return kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
		}
	}

	updatedMessage, err := s.PostgresQueries.GetMessageByID(ctx, messageID)
	if err == nil {
		recipientPrimaryDelivered := updatedMessage.DeliveredToRecipientPrimary
		senderPrimarySynced := updatedMessage.SyncedToSenderPrimary
		if recipientPrimaryDelivered && senderPrimarySynced {
			s.deleteMessageFromRelay(ctx, updatedMessage)
		}
		if err := s.PostgresQueries.CleanupFullyAcknowledgedReadMessagesInChat(ctx, updatedMessage.ChatID); err != nil {
			log.Printf("[ACK] CleanupFullyAcknowledgedReadMessagesInChat ERROR: %v", err)
		}
	}
	return nil
}

func (s *chatService) AcknowledgeDeliveryHandler(ctx context.Context, payload *AcknowledgeDeliveryPayload, userID kit.UserId, sessionId string) (*rpc_personal_chatv1.AcknowledgeDeliveryResponse, error) {
	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_message_id", "Invalid message ID")
	}
	if !payload.Success {
		return &rpc_personal_chatv1.AcknowledgeDeliveryResponse{Acknowledged: false}, nil
	}
	if err := s.AcknowledgeDelivery(ctx, messageID, payload.AcknowledgedBy, sessionId, userID); err != nil {
		return nil, err
	}
	return &rpc_personal_chatv1.AcknowledgeDeliveryResponse{Acknowledged: true}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Message Queries
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) GetChatMessages(ctx context.Context, chatID uuid.UUID, userID kit.UserId, limit int32, afterCreatedAt *time.Time, afterMessageID *uuid.UUID, sessionCreatedAt time.Time) ([]personal_chat_store.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	messages, err := s.PostgresQueries.GetChatMessages(ctx, personal_chat_store.GetChatMessagesParams{
		ChatID:           chatID,
		Limit:            limit,
		UserID:           userID.UuidUserId,
		SessionCreatedAt: sessionCreatedAt,
		AfterCreatedAt:   afterCreatedAt,
		AfterMessageID:   afterMessageID,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
	}
	return messages, nil
}

func (s *chatService) buildMessageResponse(ctx context.Context, msg personal_chat_store.Message, userID kit.UserId, isPrimary bool, senderKeyRevision int32) MessageResponse {
	isFromMe := msg.SenderID == userID.UuidUserId

	var isConsumed bool
	switch {
	case !isPrimary:
		// Secondary device: consumed if DB payload was already stripped
		isConsumed = (msg.Content == "")
	case isFromMe:
		// Primary sender phone: consumed if already synced to primary phone
		isConsumed = msg.SyncedToSenderPrimary
	default:
		// Primary recipient phone: consumed if already delivered to primary phone
		isConsumed = msg.DeliveredToRecipientPrimary
	}

	var content string
	var fileID, fileName, fileMimeType *string
	var fileSize *int64
	var viewURL, downloadURL string

	if isConsumed {
		// Stripped payload on the wire for bandwidth saving / already consumed
		content = ""
		fileID = nil
		fileName = nil
		fileSize = nil
		fileMimeType = nil
		viewURL = ""
		downloadURL = ""
	} else {
		// Full encrypted payload
		content = msg.Content
		fileID = msg.FileID
		fileName = msg.FileName
		fileSize = msg.FileSize
		fileMimeType = msg.FileMimeType
		if msg.FileID != nil && *msg.FileID != "" {
			var fileErr error
			viewURL, downloadURL, fileErr = s.GenerateMessageFileURLs(ctx, msg, userID)
			if fileErr != nil {
				log.Printf("[buildMessageResponse] Failed to generate URLs for message %s: %v", msg.ID, fileErr)
			}
		}
	}

	return MessageResponse{
		MessageID:                   msg.ID.String(),
		ChatID:                      msg.ChatID.String(),
		IsFromMe:                    isFromMe,
		RecipientID:                 msg.RecipientID.String(),
		SenderKeysRevision:          senderKeyRevision,
		Content:                     content,
		MessageType:                 msg.MessageType,
		DeliveredToRecipient:        msg.DeliveredToRecipient,
		DeliveredToRecipientPrimary: msg.DeliveredToRecipientPrimary,
		SyncedToSenderPrimary:       msg.SyncedToSenderPrimary,
		CreatedAt:                   msg.CreatedAt,
		ExpiresAt:                   msg.ExpiresAt,
		FileID:                      fileID,
		FileName:                    fileName,
		FileSize:                    fileSize,
		FileMimeType:                fileMimeType,
		ViewURL:                     viewURL,
		DownloadURL:                 downloadURL,
		ReadByRecipient:             msg.ReadByRecipient,
		ReadAckedBySender:           msg.ReadAckedBySender,
		ReadAt:                      msg.ReadAt,
		IsConsumed:                  isConsumed,
	}
}

func (s *chatService) GetMessagesHandler(ctx context.Context, payload *GetMessagesPayload, userID kit.UserId, sessionCreatedAt time.Time, isPrimary bool) (*rpc_personal_chatv1.GetMessagesResponse, error) {
	chatID, err := uuid.Parse(payload.ChatID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_chat_id", "Invalid chat ID")
	}

	// 1. Fetch chat metadata FIRST (combines existence check, participant authorization & other user timestamps in 1 query)
	chat, chatErr := s.PostgresQueries.GetChatByID(ctx, chatID)
	if chatErr != nil {
		if errors.Is(chatErr, pgx.ErrNoRows) {
			return nil, kit.NewError(http.StatusForbidden, "forbidden", "You are not a participant in this chat")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", "Failed to fetch chat")
	}
	if chat.Participant1ID != userID.UuidUserId && chat.Participant2ID != userID.UuidUserId {
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "You are not a participant in this chat")
	}

	hasCreatedAt := payload.AfterCreatedAt != nil
	hasMsgID := payload.AfterMessageID != nil && *payload.AfterMessageID != ""
	if hasCreatedAt != hasMsgID {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_cursor", "Both after_created_at and after_message_id must be provided together")
	}
	var afterMsgUUID *uuid.UUID
	if hasMsgID {
		parsed, parseErr := uuid.Parse(*payload.AfterMessageID)
		if parseErr != nil {
			return nil, kit.NewError(http.StatusBadRequest, "invalid_cursor", "Invalid after_message_id format")
		}
		afterMsgUUID = &parsed
	}

	// 2. Fetch paginated chat messages bounded by session epoch and keyset seek
	messages, err := s.GetChatMessages(ctx, chatID, userID, payload.Limit, payload.AfterCreatedAt, afterMsgUUID, sessionCreatedAt)
	if err != nil {
		return nil, err
	}

	// 3. Memoize sender key revisions for the chat participants (eliminates N+1 lookups)
	revisionMap := make(map[uuid.UUID]int32, 2)
	getRevision := func(senderID uuid.UUID) int32 {
		if rev, ok := revisionMap[senderID]; ok {
			return rev
		}
		rev := s.getSenderKeysRevision(ctx, senderID)
		revisionMap[senderID] = rev
		return rev
	}

	// 4. Build message responses directly
	msgResponses := make([]*rpc_personal_chatv1.Message, 0, len(messages))
	for _, msg := range messages {
		rev := getRevision(msg.SenderID)
		messageResponse := s.buildMessageResponse(ctx, msg, userID, isPrimary, rev)
		msgResponses = append(msgResponses, &rpc_personal_chatv1.Message{
			MessageId:                   messageResponse.MessageID,
			ChatId:                      messageResponse.ChatID,
			RecipientId:                 messageResponse.RecipientID,
			SenderKeysRevision:          messageResponse.SenderKeysRevision,
			Content:                     messageResponse.Content,
			MessageType:                 messageResponse.MessageType,
			DeliveredToRecipient:        messageResponse.DeliveredToRecipient,
			DeliveredToRecipientPrimary: messageResponse.DeliveredToRecipientPrimary,
			SyncedToSenderPrimary:       messageResponse.SyncedToSenderPrimary,
			CreatedAt:                   timestamppb.New(messageResponse.CreatedAt),
			ExpiresAt:                   timestamppb.New(messageResponse.ExpiresAt),
			IsFromMe:                    messageResponse.IsFromMe,
			FileId:                      messageResponse.FileID,
			FileName:                    messageResponse.FileName,
			FileSize:                    messageResponse.FileSize,
			FileMimeType:                messageResponse.FileMimeType,
			ViewUrl:                     messageResponse.ViewURL,
			DownloadUrl:                 messageResponse.DownloadURL,
			ReadByRecipient:             messageResponse.ReadByRecipient,
			ReadAckedBySender:           messageResponse.ReadAckedBySender,
			ReadAt:                      kit.OptionalTimestamp(messageResponse.ReadAt),
			IsConsumed:                  messageResponse.IsConsumed,
		})
	}

	nextCreatedAt, nextMessageID := extractNextCursor(messages)

	var otherReadAt, otherDeliveredAt time.Time
	if chat.Participant1ID == userID.UuidUserId {
		otherReadAt = kit.DerefTime(chat.P2LastReadAt)
		otherDeliveredAt = kit.DerefTime(chat.P2LastDeliveredAt)
	} else {
		otherReadAt = kit.DerefTime(chat.P1LastReadAt)
		otherDeliveredAt = kit.DerefTime(chat.P1LastDeliveredAt)
	}
	return &rpc_personal_chatv1.GetMessagesResponse{
		Messages:                 msgResponses,
		Count:                    int32(len(msgResponses)),
		OtherUserLastReadAt:      timestamppb.New(otherReadAt),
		OtherUserLastDeliveredAt: timestamppb.New(otherDeliveredAt),
		NextCreatedAt:            nextCreatedAt,
		NextMessageId:            nextMessageID,
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Chat List
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) GetUserChatsLite(ctx context.Context, userID uuid.UUID) ([]personal_chat_store.GetUserChatsLiteRow, error) {
	return s.PostgresQueries.GetUserChatsLite(ctx, userID)
}

func (s *chatService) GetUserChatsHandler(ctx context.Context, userID kit.UserId, sessionCreatedAt time.Time) (*rpc_personal_chatv1.GetUserChatsResponse, error) {
	chats, err := s.GetUserChatsLite(ctx, userID.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
	}
	// Collect unique target IDs
	targetIDs := make([]uuid.UUID, 0, len(chats))
	seen := make(map[uuid.UUID]struct{})
	for _, chat := range chats {
		var otherUserID uuid.UUID
		if chat.Participant1ID == userID.UuidUserId {
			otherUserID = chat.Participant2ID
		} else {
			otherUserID = chat.Participant1ID
		}
		if _, exists := seen[otherUserID]; !exists {
			targetIDs = append(targetIDs, otherUserID)
			seen[otherUserID] = struct{}{}
		}
	}

	// Batch fetch profiles to avoid N+1 queries
	contactProfiles, _ := s.ProfileProvider.GetContactableProfilesForViewer(ctx, userID.UuidUserId, targetIDs)

	chatResponses := make([]*rpc_personal_chatv1.Chat, 0, len(chats))
	for _, chat := range chats {
		var otherUserID uuid.UUID
		if chat.Participant1ID == userID.UuidUserId {
			otherUserID = chat.Participant2ID
		} else {
			otherUserID = chat.Participant1ID
		}
		// Privacy exclusion: if the contactable profile lookup omitted this
		// user (admin-blocked / private profile / user-blocked either way),
		// the map lookup below misses and the empty initials stand. The
		// wire shape — name/username/profile_type as `""` and
		// bio/avatar_url/avatar_file_id as `null` — is the privacy
		// contract the frontend relies on; see
		// profileService.GetContactableProfilesForViewer.
		otherName := ""
		otherUsername := ""
		var otherBio *string
		var avatarURL *string
		var avatarFileID *string
		otherProfileType := ""
		if cp, ok := contactProfiles[otherUserID]; ok && cp != nil {
			otherName = cp.Name
			otherUsername = cp.Username
			otherBio = cp.Bio
			avatarURL = cp.AvatarURL
			avatarFileID = cp.AvatarFileId
			otherProfileType = cp.ProfileType
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
		if cp, ok := contactProfiles[otherUserID]; ok && cp != nil {
			otherUserKeysRevision = cp.KeysRevision
		}

		if lastMessageCreatedAt != nil && lastMessageCreatedAt.Before(sessionCreatedAt) {
			lastMessageContent = nil
			lastMessageType = nil
		}

		lastMessageIsUnsent := false
		lastMessageStatus := "sent"
		chatResponses = append(chatResponses, &rpc_personal_chatv1.Chat{
			ChatId:                   chat.ID.String(),
			OtherUserId:              otherUserID.String(),
			OtherUserName:            otherName,
			OtherUserUsername:        otherUsername,
			OtherUserBio:             otherBio,
			AvatarUrl:                avatarURL,
			AvatarFileId:             avatarFileID,
			CreatedAt:                timestamppb.New(chat.CreatedAt),
			UpdatedAt:                timestamppb.New(chat.UpdatedAt),
			OtherUserLastReadAt:      timestamppb.New(otherUserLastReadAt),
			OtherUserLastDeliveredAt: timestamppb.New(otherUserLastDeliveredAt),
			LastMessageContent:       lastMessageContent,
			LastMessageCreatedAt:     kit.OptionalTimestamp(lastMessageCreatedAt),
			LastMessageType:          lastMessageType,

			LastMessageIsFromMe:   lastMessageSenderID != nil && *lastMessageSenderID == userID.StringUserId,
			LastMessageStatus:     lastMessageStatus,
			LastMessageIsUnsent:   lastMessageIsUnsent,
			LastMessageId:         lastMessageID,
			UnreadCount:           chat.UnreadCount,
			OtherUserKeysRevision: otherUserKeysRevision,
			ProfileType:           otherProfileType,
		})
	}
	return &rpc_personal_chatv1.GetUserChatsResponse{
		Chats: chatResponses,
		Count: int32(len(chatResponses)),
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

func (s *chatService) MarkChatRead(ctx context.Context, payload *MarkChatReadPayload, userID kit.UserId, isPrimary bool) ([]*rpc_personal_chatv1.MessageReadReceipt, error) {
	chatID, err := uuid.Parse(payload.ChatID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_request", "Invalid chat ID")
	}

	chat, err := s.PostgresQueries.GetChatByID(ctx, chatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, kit.NewError(http.StatusNotFound, "not_found", "chat not found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
	}
	if chat.Participant1ID != userID.UuidUserId && chat.Participant2ID != userID.UuidUserId {
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "You are not a participant in this chat")
	}

	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		if u, err := uuid.Parse(idStr); err == nil {
			msgUUIDs = append(msgUUIDs, u)
		}
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to begin tx")
	}
	defer tx.Rollback(ctx)
	qtx := s.PostgresQueries.WithTx(tx)

	if err := qtx.ResetChatReadStatus(ctx, personal_chat_store.ResetChatReadStatusParams{
		ID:             chatID,
		Participant1ID: userID.UuidUserId,
	}); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "mark_read_failed", kit.GetPostgresError(err).Message)
	}

	var readRows []personal_chat_store.MarkMessagesAsReadRow
	if len(msgUUIDs) > 0 {
		var markErr error
		readRows, markErr = qtx.MarkMessagesAsRead(ctx, personal_chat_store.MarkMessagesAsReadParams{
			ChatID:      chatID,
			RecipientID: userID.UuidUserId,
			MessageIds:  msgUUIDs,
		})
		if markErr != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "mark_read_failed", kit.GetPostgresError(markErr).Message)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to commit")
	}

	// Async relay cleanup: delete fully acknowledged, read messages with no pending files in this chat without blocking response
	go func(cID uuid.UUID) {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.PostgresQueries.CleanupFullyAcknowledgedReadMessagesInChat(cleanupCtx, cID); err != nil {
			log.Printf("[MarkChatRead] Async CleanupFullyAcknowledgedReadMessagesInChat ERROR for chat %s: %v", cID, err)
		}
	}(chatID)

	readReceipts := make([]*rpc_personal_chatv1.MessageReadReceipt, 0, len(msgUUIDs))
	readMap := make(map[uuid.UUID]bool, len(readRows))
	for _, r := range readRows {
		readMap[r.ID] = true
		receipt := &rpc_personal_chatv1.MessageReadReceipt{
			MessageId: r.ID.String(),
		}
		if r.ReadAt != nil {
			receipt.ReadAt = timestamppb.New(*r.ReadAt)
		}
		readReceipts = append(readReceipts, receipt)
	}

	var remainingIDs []uuid.UUID
	for _, id := range msgUUIDs {
		if !readMap[id] {
			remainingIDs = append(remainingIDs, id)
		}
	}

	if len(remainingIDs) > 0 {
		existingRows, getErr := s.PostgresQueries.GetMessagesByIds(ctx, remainingIDs)
		if getErr != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "lookup_failed", kit.GetPostgresError(getErr).Message)
		}

		existingMap := make(map[uuid.UUID]personal_chat_store.GetMessagesByIdsRow, len(existingRows))
		for _, row := range existingRows {
			existingMap[row.ID] = row
		}

		now := time.Now()
		for _, id := range remainingIDs {
			row, exists := existingMap[id]
			if !exists {
				// Message already deleted from relay table (Stage 2 complete) -> Idempotent Confirmation
				readReceipts = append(readReceipts, &rpc_personal_chatv1.MessageReadReceipt{
					MessageId: id.String(),
					ReadAt:    timestamppb.New(now),
				})
				continue
			}
			// Message exists in DB: confirm if caller is recipient in the same chat AND message is read
			if row.RecipientID == userID.UuidUserId && row.ChatID == chatID && row.ReadByRecipient {
				rAt := now
				if row.ReadAt != nil {
					rAt = *row.ReadAt
				}
				readReceipts = append(readReceipts, &rpc_personal_chatv1.MessageReadReceipt{
					MessageId: id.String(),
					ReadAt:    timestamppb.New(rAt),
				})
			}
		}
	}

	return readReceipts, nil
}

func (s *chatService) AcknowledgeAndReadBatch(ctx context.Context, payload *AckAndReadBatchPayload, userID kit.UserId, isPrimary bool) (*rpc_personal_chatv1.AckAndReadBatchResponse, uuid.UUID, error) {
	if payload == nil || len(payload.MessageIDs) == 0 {
		return &rpc_personal_chatv1.AckAndReadBatchResponse{
			AcknowledgedCount: 0,
			ReadMessages:      []*rpc_personal_chatv1.MessageReadReceipt{},
		}, uuid.Nil, nil
	}

	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		if u, err := uuid.Parse(idStr); err == nil {
			msgUUIDs = append(msgUUIDs, u)
		}
	}
	if len(msgUUIDs) == 0 {
		return &rpc_personal_chatv1.AckAndReadBatchResponse{
			AcknowledgedCount: 0,
			ReadMessages:      []*rpc_personal_chatv1.MessageReadReceipt{},
		}, uuid.Nil, nil
	}

	// 1. Upfront Pre-Validation: Validate chat consistency BEFORE opening any write transaction
	existingRows, getErr := s.PostgresQueries.GetMessagesByIds(ctx, msgUUIDs)
	if getErr != nil {
		return nil, uuid.Nil, kit.NewError(http.StatusInternalServerError, "lookup_failed", kit.GetPostgresError(getErr).Message)
	}

	var targetChatID uuid.UUID
	var latestCreatedAt time.Time
	var validIDs []uuid.UUID
	existingMap := make(map[uuid.UUID]personal_chat_store.GetMessagesByIdsRow, len(existingRows))

	for _, row := range existingRows {
		existingMap[row.ID] = row
		if row.RecipientID == userID.UuidUserId {
			if targetChatID == uuid.Nil {
				targetChatID = row.ChatID
			} else if targetChatID != row.ChatID {
				// Reject immediately without starting write transaction or wasting DB resources
				return nil, uuid.Nil, kit.NewError(http.StatusBadRequest, "invalid_batch", "All messages in an in-chat batch must belong to the same chat")
			}
			validIDs = append(validIDs, row.ID)
		}
	}

	// 2. Transaction Phase: Update delivery and mark read for the target chat
	var readRows []personal_chat_store.MarkMessagesAsReadRow
	if len(validIDs) > 0 && targetChatID != uuid.Nil {
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return nil, uuid.Nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to begin tx")
		}
		defer tx.Rollback(ctx)
		qtx := s.PostgresQueries.WithTx(tx)

		if isPrimary {
			rows, batchErr := qtx.MarkMessagesDeliveredToRecipientPrimaryBatch(ctx, personal_chat_store.MarkMessagesDeliveredToRecipientPrimaryBatchParams{
				RecipientID: userID.UuidUserId,
				MessageIds:  validIDs,
			})
			if batchErr != nil {
				return nil, uuid.Nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(batchErr).Message)
			}
			for _, r := range rows {
				if r.CreatedAt.After(latestCreatedAt) {
					latestCreatedAt = r.CreatedAt
				}
			}
		} else {
			rows, batchErr := qtx.MarkMessagesDeliveredToRecipientBatch(ctx, personal_chat_store.MarkMessagesDeliveredToRecipientBatchParams{
				RecipientID: userID.UuidUserId,
				MessageIds:  validIDs,
			})
			if batchErr != nil {
				return nil, uuid.Nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(batchErr).Message)
			}
			for _, r := range rows {
				if r.CreatedAt.After(latestCreatedAt) {
					latestCreatedAt = r.CreatedAt
				}
			}
		}

		if latestCreatedAt.IsZero() {
			latestCreatedAt = time.Now()
		}

		_ = qtx.UpdateChatLastDeliveredAt(ctx, personal_chat_store.UpdateChatLastDeliveredAtParams{
			ChatID:          targetChatID,
			ParticipantID:   userID.UuidUserId,
			LastDeliveredAt: latestCreatedAt,
		})

		if err := qtx.ResetChatReadStatus(ctx, personal_chat_store.ResetChatReadStatusParams{
			ID:             targetChatID,
			Participant1ID: userID.UuidUserId,
		}); err != nil {
			return nil, uuid.Nil, kit.NewError(http.StatusInternalServerError, "mark_read_failed", kit.GetPostgresError(err).Message)
		}

		rRows, markErr := qtx.MarkMessagesAsRead(ctx, personal_chat_store.MarkMessagesAsReadParams{
			ChatID:      targetChatID,
			RecipientID: userID.UuidUserId,
			MessageIds:  validIDs,
		})
		if markErr != nil {
			return nil, uuid.Nil, kit.NewError(http.StatusInternalServerError, "mark_read_failed", kit.GetPostgresError(markErr).Message)
		}
		readRows = rRows

		if err := tx.Commit(ctx); err != nil {
			return nil, uuid.Nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to commit tx")
		}

		// Async relay cleanup for this specific chat: strip/delete delivered messages from both primaries
		go func(cID uuid.UUID, ids []uuid.UUID) {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			for _, msgID := range ids {
				msg, err := s.PostgresQueries.GetMessageByID(cleanupCtx, msgID)
				if err == nil && msg.DeliveredToRecipientPrimary && msg.SyncedToSenderPrimary {
					s.deleteMessageFromRelay(cleanupCtx, msg)
				}
			}
			if err := s.PostgresQueries.CleanupFullyAcknowledgedReadMessagesInChat(cleanupCtx, cID); err != nil {
				log.Printf("[AckAndReadBatch] Async CleanupFullyAcknowledgedReadMessagesInChat ERROR for chat %s: %v", cID, err)
			}
		}(targetChatID, validIDs)
	}

	// 3. Build Read Receipts: Newly read, Already read, Purged from DB
	readReceipts := make([]*rpc_personal_chatv1.MessageReadReceipt, 0, len(msgUUIDs))
	readMap := make(map[uuid.UUID]bool, len(readRows))
	for _, r := range readRows {
		readMap[r.ID] = true
		receipt := &rpc_personal_chatv1.MessageReadReceipt{
			MessageId: r.ID.String(),
		}
		if r.ReadAt != nil {
			receipt.ReadAt = timestamppb.New(*r.ReadAt)
		}
		readReceipts = append(readReceipts, receipt)
	}

	now := time.Now()
	for _, id := range msgUUIDs {
		if readMap[id] {
			continue
		}
		row, exists := existingMap[id]
		if !exists {
			// Message already deleted from relay table (Stage 2 complete) -> Idempotent Confirmation
			readReceipts = append(readReceipts, &rpc_personal_chatv1.MessageReadReceipt{
				MessageId: id.String(),
				ReadAt:    timestamppb.New(now),
			})
			continue
		}
		// Message exists in DB: confirm if caller is recipient and message is read
		if row.RecipientID == userID.UuidUserId && row.ReadByRecipient {
			rAt := now
			if row.ReadAt != nil {
				rAt = *row.ReadAt
			}
			readReceipts = append(readReceipts, &rpc_personal_chatv1.MessageReadReceipt{
				MessageId: id.String(),
				ReadAt:    timestamppb.New(rAt),
			})
		}
	}

	return &rpc_personal_chatv1.AckAndReadBatchResponse{
		AcknowledgedCount: int32(len(readReceipts)),
		ReadMessages:      readReceipts,
	}, targetChatID, nil
}

func (s *chatService) AcknowledgeReadReceiptBatch(ctx context.Context, payload *AckReadReceiptBatchPayload, userID kit.UserId, isPrimary bool) (*rpc_personal_chatv1.AckReadReceiptBatchResponse, error) {
	if !isPrimary {
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "Forbidden: Only primary device can ACK read receipts")
	}

	chatID, err := uuid.Parse(payload.ChatID)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_chat_id", "Invalid chat ID")
	}

	chat, err := s.PostgresQueries.GetChatByID(ctx, chatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, kit.NewError(http.StatusNotFound, "not_found", "chat not found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
	}
	if chat.Participant1ID != userID.UuidUserId && chat.Participant2ID != userID.UuidUserId {
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "You are not a participant in this chat")
	}

	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		if u, err := uuid.Parse(idStr); err == nil {
			msgUUIDs = append(msgUUIDs, u)
		}
	}

	if len(msgUUIDs) == 0 {
		return &rpc_personal_chatv1.AckReadReceiptBatchResponse{
			AcknowledgedCount:      0,
			AcknowledgedMessageIds: []string{},
		}, nil
	}

	ackedIDs, err := s.PostgresQueries.MarkMessagesReadAckedBySender(ctx, personal_chat_store.MarkMessagesReadAckedBySenderParams{
		ChatID:     chatID,
		SenderID:   userID.UuidUserId,
		MessageIds: msgUUIDs,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "mark_read_ack_failed", kit.GetPostgresError(err).Message)
	}

	ackedMap := make(map[uuid.UUID]bool, len(ackedIDs))
	for _, id := range ackedIDs {
		ackedMap[id] = true
	}

	var remainingIDs []uuid.UUID
	for _, id := range msgUUIDs {
		if !ackedMap[id] {
			remainingIDs = append(remainingIDs, id)
		}
	}

	var finalAckedIDs []string
	for _, id := range ackedIDs {
		finalAckedIDs = append(finalAckedIDs, id.String())
	}

	if len(remainingIDs) > 0 {
		existingRows, getErr := s.PostgresQueries.GetMessagesByIds(ctx, remainingIDs)
		if getErr != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "fetch_messages_failed", kit.GetPostgresError(getErr).Message)
		}

		existingMap := make(map[uuid.UUID]personal_chat_store.GetMessagesByIdsRow, len(existingRows))
		for _, row := range existingRows {
			existingMap[row.ID] = row
		}

		for _, id := range remainingIDs {
			row, exists := existingMap[id]
			if !exists {
				// Message already deleted from relay table (Stage 2 complete) -> Idempotent Success
				finalAckedIDs = append(finalAckedIDs, id.String())
				continue
			}
			// If message exists in DB and caller is not the sender of this message
			if row.SenderID != userID.UuidUserId {
				return nil, kit.NewError(http.StatusForbidden, "forbidden", "You are not the sender of message "+id.String())
			}
			if row.ChatID == chatID && row.ReadAckedBySender {
				finalAckedIDs = append(finalAckedIDs, id.String())
			}
		}
	}

	// Async relay cleanup: delete fully acknowledged, read messages in this chat
	if len(ackedIDs) > 0 {
		go func(cID uuid.UUID) {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.PostgresQueries.CleanupFullyAcknowledgedReadMessagesInChat(cleanupCtx, cID); err != nil {
				log.Printf("[AcknowledgeReadReceiptBatch] Async CleanupFullyAcknowledgedReadMessagesInChat ERROR for chat %s: %v", cID, err)
			}
		}(chatID)
	}

	return &rpc_personal_chatv1.AckReadReceiptBatchResponse{
		AcknowledgedCount:      int32(len(finalAckedIDs)),
		AcknowledgedMessageIds: finalAckedIDs,
	}, nil
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
			if msg.DeliveredToRecipientPrimary {
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

func (s *chatService) GetSyncActionsHandler(ctx context.Context, payload *GetSyncActionsPayload, userID kit.UserId) (*rpc_personal_chatv1.GetSyncActionsResponse, error) {
	limit := payload.Limit
	if limit <= 0 {
		limit = 50
	}
	actions, err := s.GetSyncActions(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	respActions := make([]*rpc_personal_chatv1.SyncAction, 0, len(actions))
	for _, a := range actions {
		var payloadObj SyncActionPayload
		_ = json.Unmarshal(a.Payload, &payloadObj)
		respActions = append(respActions, &rpc_personal_chatv1.SyncAction{
			Id:         a.ID.String(),
			UserId:     a.UserID.String(),
			ActionType: a.ActionType,
			Payload: &rpc_personal_chatv1.SyncActionPayload{
				MessageIds: payloadObj.MessageIDs,
				ChatId:     kit.OptionalString(payloadObj.ChatID),
			},
			DeliveredToPrimary: a.DeliveredToPrimary,
			CreatedAt:          timestamppb.New(a.CreatedAt),
		})
	}
	return &rpc_personal_chatv1.GetSyncActionsResponse{
		Actions: respActions,
		Count:   int32(len(respActions)),
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

func (s *chatService) GetPendingMessagesHandler(ctx context.Context, payload *GetPendingMessagesPayload, userID kit.UserId, sessionCreatedAt time.Time, isPrimary bool) (*rpc_personal_chatv1.GetPendingMessagesResponse, error) {
	limit := payload.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	hasRecipCreatedAt := payload.AfterRecipientCreatedAt != nil
	hasRecipMsgID := payload.AfterRecipientMessageID != nil && *payload.AfterRecipientMessageID != ""
	if hasRecipCreatedAt != hasRecipMsgID {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_cursor", "Both after_recipient_created_at and after_recipient_message_id must be provided together")
	}
	var afterRecipientMsgUUID *uuid.UUID
	if hasRecipMsgID {
		parsed, parseErr := uuid.Parse(*payload.AfterRecipientMessageID)
		if parseErr != nil {
			return nil, kit.NewError(http.StatusBadRequest, "invalid_cursor", "Invalid after_recipient_message_id format")
		}
		afterRecipientMsgUUID = &parsed
	}

	hasSenderCreatedAt := payload.AfterSenderCreatedAt != nil
	hasSenderMsgID := payload.AfterSenderMessageID != nil && *payload.AfterSenderMessageID != ""
	if hasSenderCreatedAt != hasSenderMsgID {
		return nil, kit.NewError(http.StatusBadRequest, "invalid_cursor", "Both after_sender_created_at and after_sender_message_id must be provided together")
	}
	var afterSenderMsgUUID *uuid.UUID
	if hasSenderMsgID {
		parsed, parseErr := uuid.Parse(*payload.AfterSenderMessageID)
		if parseErr != nil {
			return nil, kit.NewError(http.StatusBadRequest, "invalid_cursor", "Invalid after_sender_message_id format")
		}
		afterSenderMsgUUID = &parsed
	}

	baseLimit := limit
	maxTotalCapacity := int(baseLimit * 2)

	// 1. Fetch pending received messages up to maxTotalCapacity, looping internally through filtered/blocked rows
	var recipientMsgsFiltered []personal_chat_store.Message
	var lastRawRecipientMsg *personal_chat_store.Message
	curAfterRecipCreatedAt := payload.AfterRecipientCreatedAt
	curAfterRecipUUID := afterRecipientMsgUUID

	const maxFilterIterations = 10
	for iteration := 0; iteration < maxFilterIterations && len(recipientMsgsFiltered) < maxTotalCapacity; iteration++ {
		rawBatch, err := s.PostgresQueries.GetPendingMessagesForRecipient(ctx, personal_chat_store.GetPendingMessagesForRecipientParams{
			RecipientID:      userID.UuidUserId,
			Limit:            int32(maxTotalCapacity),
			SessionCreatedAt: sessionCreatedAt,
			AfterCreatedAt:   curAfterRecipCreatedAt,
			AfterMessageID:   curAfterRecipUUID,
		})
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
		}
		if len(rawBatch) == 0 {
			break
		}

		lastRawRecipientMsg = &rawBatch[len(rawBatch)-1]
		curAfterRecipCreatedAt = &lastRawRecipientMsg.CreatedAt
		curAfterRecipUUID = &lastRawRecipientMsg.ID

		senderIDsMap := make(map[uuid.UUID]struct{})
		for _, m := range rawBatch {
			senderIDsMap[m.SenderID] = struct{}{}
		}
		senderIDs := make([]uuid.UUID, 0, len(senderIDsMap))
		for id := range senderIDsMap {
			senderIDs = append(senderIDs, id)
		}

		// Batch check contactable user IDs to enforce both admin and peer blocks
		contactableIDs, err := s.ProfileProvider.GetContactableUserIDs(ctx, userID.UuidUserId, senderIDs)
		if err != nil {
			return nil, err
		}
		// Convert slice to map for O(1) checks
		contactableSet := make(map[uuid.UUID]struct{}, len(contactableIDs))
		for _, id := range contactableIDs {
			contactableSet[id] = struct{}{}
		}
		for _, m := range rawBatch {
			if _, isContactable := contactableSet[m.SenderID]; isContactable {
				recipientMsgsFiltered = append(recipientMsgsFiltered, m)
			}
		}

		if len(rawBatch) < maxTotalCapacity {
			break
		}
	}

	// 2. Fetch pending sender sync messages up to maxTotalCapacity
	senderMsgs, err := s.PostgresQueries.GetPendingSenderSyncMessages(ctx, personal_chat_store.GetPendingSenderSyncMessagesParams{
		SenderID:         userID.UuidUserId,
		Limit:            int32(maxTotalCapacity),
		SessionCreatedAt: sessionCreatedAt,
		AfterCreatedAt:   payload.AfterSenderCreatedAt,
		AfterMessageID:   afterSenderMsgUUID,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_failed", kit.GetPostgresError(err).Message)
	}

	// 3. Dynamic capacity borrowing: fill unused slots from the smaller stream into the larger stream
	lenRecip := len(recipientMsgsFiltered)
	lenSender := len(senderMsgs)

	var takeRecip, takeSender int
	if lenRecip <= int(baseLimit) && lenSender <= int(baseLimit) {
		takeRecip = lenRecip
		takeSender = lenSender
	} else if lenRecip <= int(baseLimit) {
		takeRecip = lenRecip
		takeSender = lenSender
		if takeSender > (maxTotalCapacity - takeRecip) {
			takeSender = maxTotalCapacity - takeRecip
		}
	} else if lenSender <= int(baseLimit) {
		takeSender = lenSender
		takeRecip = lenRecip
		if takeRecip > (maxTotalCapacity - takeSender) {
			takeRecip = maxTotalCapacity - takeSender
		}
	} else {
		takeRecip = int(baseLimit)
		takeSender = int(baseLimit)
	}

	var combinedMsgs []personal_chat_store.Message
	if takeRecip > 0 {
		combinedMsgs = append(combinedMsgs, recipientMsgsFiltered[:takeRecip]...)
	}
	if takeSender > 0 {
		combinedMsgs = append(combinedMsgs, senderMsgs[:takeSender]...)
	}

	// 4. Sort chronologically by CreatedAt ASC, id ASC
	if len(combinedMsgs) > 1 {
		sort.Slice(combinedMsgs, func(i, j int) bool {
			if combinedMsgs[i].CreatedAt.Equal(combinedMsgs[j].CreatedAt) {
				return combinedMsgs[i].ID.String() < combinedMsgs[j].ID.String()
			}
			return combinedMsgs[i].CreatedAt.Before(combinedMsgs[j].CreatedAt)
		})
	}

	// 4. Memoize sender key revisions across pending messages
	revisionMap := make(map[uuid.UUID]int32, 4)
	getRevision := func(senderID uuid.UUID) int32 {
		if rev, ok := revisionMap[senderID]; ok {
			return rev
		}
		rev := s.getSenderKeysRevision(ctx, senderID)
		revisionMap[senderID] = rev
		return rev
	}

	// 5. Build response payload
	msgResponses := make([]*rpc_personal_chatv1.Message, 0, len(combinedMsgs))
	for _, msg := range combinedMsgs {
		rev := getRevision(msg.SenderID)
		mr := s.buildMessageResponse(ctx, msg, userID, isPrimary, rev)
		msgResponses = append(msgResponses, &rpc_personal_chatv1.Message{
			MessageId:                   mr.MessageID,
			ChatId:                      mr.ChatID,
			RecipientId:                 mr.RecipientID,
			SenderKeysRevision:          mr.SenderKeysRevision,
			Content:                     mr.Content,
			MessageType:                 mr.MessageType,
			DeliveredToRecipient:        mr.DeliveredToRecipient,
			DeliveredToRecipientPrimary: mr.DeliveredToRecipientPrimary,
			SyncedToSenderPrimary:       mr.SyncedToSenderPrimary,
			CreatedAt:                   timestamppb.New(mr.CreatedAt),
			ExpiresAt:                   timestamppb.New(mr.ExpiresAt),
			IsFromMe:                    mr.IsFromMe,
			FileId:                      mr.FileID,
			FileName:                    mr.FileName,
			FileSize:                    mr.FileSize,
			FileMimeType:                mr.FileMimeType,
			ViewUrl:                     mr.ViewURL,
			DownloadUrl:                 mr.DownloadURL,
			ReadByRecipient:             mr.ReadByRecipient,
			ReadAckedBySender:           mr.ReadAckedBySender,
			ReadAt:                      kit.OptionalTimestamp(mr.ReadAt),
			IsConsumed:                  mr.IsConsumed,
		})
	}

	var nextRecipientCreatedAt *timestamppb.Timestamp
	var nextRecipientMessageID *string

	if takeRecip < len(recipientMsgsFiltered) {
		lastTaken := recipientMsgsFiltered[takeRecip-1]
		idStr := lastTaken.ID.String()
		nextRecipientCreatedAt = timestamppb.New(lastTaken.CreatedAt)
		nextRecipientMessageID = &idStr
	} else if lastRawRecipientMsg != nil {
		idStr := lastRawRecipientMsg.ID.String()
		nextRecipientCreatedAt = timestamppb.New(lastRawRecipientMsg.CreatedAt)
		nextRecipientMessageID = &idStr
	}

	nextSenderCreatedAt, nextSenderMessageID := extractNextCursor(senderMsgs[:takeSender])

	return &rpc_personal_chatv1.GetPendingMessagesResponse{
		Messages:               msgResponses,
		Count:                  int32(len(msgResponses)),
		NextRecipientCreatedAt: nextRecipientCreatedAt,
		NextRecipientMessageId: nextRecipientMessageID,
		NextSenderCreatedAt:    nextSenderCreatedAt,
		NextSenderMessageId:    nextSenderMessageID,
	}, nil
}

func extractNextCursor(msgs []personal_chat_store.Message) (*timestamppb.Timestamp, *string) {
	if n := len(msgs); n > 0 {
		last := msgs[n-1]
		idStr := last.ID.String()
		return timestamppb.New(last.CreatedAt), &idStr
	}
	return nil, nil
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

	if message.ReadByRecipient && message.ReadAckedBySender {
		if err := qtx.DeleteMessage(ctx, messageID); err != nil {
			log.Printf("[Relay-Cleanup] ERROR: Failed to delete message %s: %v", messageID, err)
			return
		}
	} else {
		if err := qtx.StripDeliveredMessagePayload(ctx, messageID); err != nil {
			log.Printf("[Relay-Cleanup] ERROR: Failed to strip payload for message %s: %v", messageID, err)
			return
		}
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

// CleanupChatMessagesForBlockAsync deletes all messages in the chat between blocker and blocked asynchronously in the background.
// It registers any attached files to pending_uploads, deletes the database rows in a transaction, and triggers R2 deletions.
func (s *chatService) CleanupChatMessagesForBlockAsync(ctx context.Context, blockerID, blockedID uuid.UUID) {
	// Use background context with timeout for background processing
	bgCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// 1. Get the chat between participants
	chat, err := s.PostgresQueries.GetChatByParticipants(bgCtx, personal_chat_store.GetChatByParticipantsParams{
		Column1: blockerID,
		Column2: blockedID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			// No chat exists, nothing to clean up
			return
		}
		log.Printf("[Block-Cleanup] ERROR: Failed to fetch chat for block cleanup: %v", err)
		return
	}

	// 2. Fetch all messages in the chat containing a file
	messages, err := s.PostgresQueries.GetMessagesWithFilesByChatID(bgCtx, chat.ID)
	if err != nil {
		log.Printf("[Block-Cleanup] ERROR: Failed to fetch messages with files for chat %s: %v", chat.ID, err)
		return
	}

	// Start a transaction for registering to pending_uploads and deleting messages
	tx, err := s.Pool.Begin(bgCtx)
	if err != nil {
		log.Printf("[Block-Cleanup] ERROR: Failed to start transaction for chat %s: %v", chat.ID, err)
		return
	}
	defer tx.Rollback(bgCtx)
	qtx := s.PostgresQueries.WithTx(tx)

	var fileIDs []string
	for _, msg := range messages {
		if msg.FileID != nil && *msg.FileID != "" {
			fileID := *msg.FileID
			_, objectKey := clients.ParseFilePrefix(fileID)
			client := s.R2Pool.GetClient(fileID)
			bucket := client.ChatBucket()

			if err := s.PendingUploads.RegisterTx(bgCtx, tx, fileID, bucket, objectKey, time.Now().UTC()); err != nil {
				log.Printf("[Block-Cleanup] ERROR: Failed to register file %s in pending_uploads: %v", fileID, err)
				return
			}
			fileIDs = append(fileIDs, fileID)
		}
	}

	// Delete all messages in the chat from the messages table
	if err := qtx.DeleteMessagesByChatID(bgCtx, chat.ID); err != nil {
		log.Printf("[Block-Cleanup] ERROR: Failed to delete messages for chat %s: %v", chat.ID, err)
		return
	}

	if err := tx.Commit(bgCtx); err != nil {
		log.Printf("[Block-Cleanup] ERROR: Failed to commit transaction for chat %s: %v", chat.ID, err)
		return
	}

	// 3. Post-commit: Asynchronously delete files from Cloudflare R2
	if len(fileIDs) > 0 {
		log.Printf("[Block-Cleanup] Triggering concurrent R2 deletion of %d files for chat %s", len(fileIDs), chat.ID)
		r2Errors := kit.DeleteFilesBatch(bgCtx, fileIDs, s.DeleteChatFile)
		for i, fileID := range fileIDs {
			if r2Errors[i] == nil {
				if err := s.PendingUploads.Remove(context.Background(), fileID); err != nil {
					log.Printf("[Block-Cleanup] WARNING: Failed to remove file %s from pending_uploads: %v", fileID, err)
				}
			} else {
				log.Printf("[Block-Cleanup] WARNING: Async file %s R2 delete failed: %v", fileID, r2Errors[i])
			}
		}
	}
}

var (
	cleanupDBBatchSize   = int32(5000)
	cleanupThrottleSleep = 50 * time.Millisecond
)

// deleteBatchUntilDone runs a bounded DELETE query repeatedly in batches of 5000 rows.
// It enforces two layers of budget protection:
// 1. Soft Active Work Limit (90%): Stops starting new delete query cycles if elapsed time passes 90% of the budget.
// 2. Hard Stop Timeout (100%): Cancels the active query using a context timeout if execution overruns the total budget.
// This prevents long-running locked transactions, ensures predictable completion times, and saves the remaining 10% safety buffer.
func deleteBatchUntilDone(ctx context.Context, deleteFn func(context.Context, int32) (int64, error), budget time.Duration, label string) bool {
	start := time.Now()
	var total int64

	// Allocate 90% of the budget for initiating new active batches. Keep 10% as a safety margin.
	workBudget := (budget * 90) / 100

	// Loop only while we are within the 90% soft active work budget limit
	for time.Since(start) < workBudget {
		// Calculate the hard remaining limit left in the total budget (100%)
		timeLeft := budget - time.Since(start)
		if timeLeft <= 0 {
			break
		}

		// Cancel the active batch immediately if it exceeds the hard timeout limit
		queryCtx, cancel := context.WithTimeout(ctx, timeLeft)
		rows, err := deleteFn(queryCtx, cleanupDBBatchSize)
		cancel()

		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.Printf("[CleanupJob] Context expired during %s batch, stopping", label)
			} else {
				log.Printf("[CleanupJob] ERROR: Failed to delete %s batch: %v", label, err)
			}
			return false
		}
		total += rows
		if rows == 0 {
			log.Printf("[CleanupJob] Finished %s cleanup, deleted %d rows", label, total)
			return true // Finished deleting all target records for this query
		}

		// Throttle execution by pausing between batch runs to allow autovacuum and concurrent traffic to execute.
		select {
		case <-ctx.Done():
			log.Printf("[CleanupJob] Context cancelled during %s throttle sleep", label)
			return false
		case <-time.After(cleanupThrottleSleep):
		}
	}
	log.Printf("[CleanupJob] Active work budget limit reached for %s, deleted %d rows total (exited within safety buffer)", label, total)
	return false // Exited due to budget limit exhaustion
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
		// Concurrent R2 deletes — 50-way parallelism (kit.MaxConcurrentDeletes).
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
			if err := s.PostgresQueries.StripDeliveredMessagePayload(ctx, msg.ID); err != nil {
				log.Printf("[CleanupJob] WARNING: Failed to strip delivered message payload %s: %v", msg.ID, err)
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
		if len(blockedMessages) < batchSize {
			break
		}
		lastBlockedID = blockedMessages[len(blockedMessages)-1].ID
	}

	log.Printf("[CleanupJob] Cleanup process completed successfully")
	return nil
}

// CleanupDatabaseOnly executes the 6 database-only cleanups sequentially.
// It enforces a 55-minute worker lifetime to guarantee clean shutdown before the next hourly run,
// and schedules all jobs within a 50-minute time budget. Leftover time from early completions is
// dynamically shared equally among subsequent tasks.
func (s *chatService) CleanupDatabaseOnly(ctx context.Context) error {
	log.Printf("[DatabaseCleanupJob] Starting bounded DB-only cleanup sweep (no R2 files)")

	// 1. Establish a 55-minute hard shutdown boundary to prevent overlapping runs.
	jobCtx, cancel := context.WithTimeout(ctx, 55*time.Minute)
	defer cancel()

	// 2. Track the start time and the 50-minute active allocation budget.
	jobStart := time.Now()
	const totalBudget = 50 * time.Minute

	// Helper function to dynamically track remaining active budget.
	getRemainingBudget := func() time.Duration {
		elapsed := time.Since(jobStart)
		if elapsed >= totalBudget {
			return 0
		}
		return totalBudget - elapsed
	}

	// Ordered list of database cleanup tasks to run sequentially.
	jobs := []struct {
		deleteFn func(context.Context, int32) (int64, error)
		label    string
	}{
		{s.PostgresQueries.DeleteExpiredMessagesWithoutFilesBatch, "expired no-file messages"},
		{s.PostgresQueries.DeleteFullyAcknowledgedMessagesWithoutFilesBatch, "fully-acknowledged no-file messages"},
		{s.PostgresQueries.DeleteBlockedUserMessagesWithoutFilesBatch, "blocked-user no-file messages"},
		{s.PostgresQueries.CleanupSyncActionsForBlockedUsersBatch, "blocked-user sync actions"},
		{s.PostgresQueries.DeleteOldSyncActionsBatch, "old sync actions"},
		{s.PostgresQueries.DeleteExpiredHistorySyncBatch, "expired history sync"},
	}

	for i := 0; i < len(jobs); i++ {
		// Stop immediately before starting the next job if context has been cancelled or hard timed out.
		select {
		case <-jobCtx.Done():
			log.Printf("[DatabaseCleanupJob] Job cancelled/timed out after %d/%d jobs, stopping remaining cleanups", i, len(jobs))
			return jobCtx.Err()
		default:
		}

		// Calculate job-specific budget: remaining budget divided equally among remaining jobs.
		// If Job A completes early, its unused time increases the budget for subsequent jobs.
		remainingJobsCount := int64(len(jobs) - i)
		currentRemainingBudget := getRemainingBudget()
		if currentRemainingBudget <= 0 {
			log.Printf("[DatabaseCleanupJob] Time budget fully depleted after %d/%d jobs, skipping remaining", i, len(jobs))
			break
		}
		jobBudget := currentRemainingBudget / time.Duration(remainingJobsCount)

		log.Printf("[DatabaseCleanupJob] Allocating budget %v for %s", jobBudget, jobs[i].label)
		deleteBatchUntilDone(jobCtx, jobs[i].deleteFn, jobBudget, jobs[i].label)
	}

	log.Printf("[DatabaseCleanupJob] Cleanup sweep completed (elapsed: %v)", time.Since(jobStart))
	return nil
}
