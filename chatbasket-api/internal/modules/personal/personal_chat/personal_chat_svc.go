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
	RegisterBatchTx(ctx context.Context, tx pgx.Tx, fileIDs, buckets, r2Keys []string, expiresAt time.Time) error
}

type personalProfilePersonalChatProvider interface {
	GetContactableProfilesForViewer(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error)
	GetUserCoreProfile(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error)
	IsUserLockedForDeletion(ctx context.Context, userID uuid.UUID) (bool, error)
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
	GlobalService      *services.GlobalService
	Pool               *pgxpool.Pool
	MaintenancePool    *pgxpool.Pool
	PostgresQuerier    personal_chat_store.Querier
	PostgresQueries    *personal_chat_store.Queries
	MaintenanceQuerier personal_chat_store.Querier
	MaintenanceQueries *personal_chat_store.Queries
	PendingUploads     pendingUploadsChatProvider
	AuthProvider       coreAuthChatProvider
	ProfileProvider    personalProfilePersonalChatProvider
	ContactProvider    personalContactPersonalChatProvider
	R2Pool             *clients.R2ClientPool
}

func NewChatService(
	globalService *services.GlobalService,
	pool *pgxpool.Pool,
	maintenancePool *pgxpool.Pool,
	authProvider coreAuthChatProvider,
	profileProvider personalProfilePersonalChatProvider,
	contactProvider personalContactPersonalChatProvider,
	pendingUploads pendingUploadsChatProvider,
	r2Pool *clients.R2ClientPool,
) *chatService {
	store := personal_chat_store.New(pool)
	maintenanceStore := personal_chat_store.New(maintenancePool)
	return &chatService{
		GlobalService:      globalService,
		Pool:               pool,
		MaintenancePool:    maintenancePool,
		PostgresQuerier:    store,
		PostgresQueries:    store,
		MaintenanceQuerier: maintenanceStore,
		MaintenanceQueries: maintenanceStore,
		PendingUploads:     pendingUploads,
		AuthProvider:       authProvider,
		ProfileProvider:    profileProvider,
		ContactProvider:    contactProvider,
		R2Pool:             r2Pool,
	}
}

func (s *chatService) accountClient(accountName string) *clients.R2Client {
	return s.R2Pool.GetClientByAccount(accountName)
}

func (s *chatService) CheckMessagingEligibility(ctx context.Context, senderID kit.UserId, recipientID uuid.UUID) (string, *string, int32, error) {
	if senderID.UuidUserId == recipientID {
		return "", nil, 0, kit.NewError(http.StatusBadRequest, "invalid_recipient", "Cannot check eligibility with yourself")
	}
	// Strict account-deletion block: the deleter holds users.id FOR UPDATE
	// until commit; probe both sides with NOWAIT so a send involving a
	// deleting user fails instantly (never waits, never slips in).
	// Plain SELECTs do not conflict with row locks, so this explicit probe
	// is required — GetUserCoreProfile below would pass straight through.
	if locked, err := s.ProfileProvider.IsUserLockedForDeletion(ctx, senderID.UuidUserId); err != nil {
		return "", nil, 0, kit.NewError(http.StatusInternalServerError, "eligibility_check_failed", "failed to check sender deletion status: "+err.Error())
	} else if locked {
		return "", nil, 0, kit.NewError(http.StatusForbidden, "messaging_not_allowed", "This account is not available for messaging.")
	}
	if locked, err := s.ProfileProvider.IsUserLockedForDeletion(ctx, recipientID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EligibilityRecipientNotFound, nil, 0, nil
		}
		return "", nil, 0, kit.NewError(http.StatusInternalServerError, "eligibility_check_failed", "failed to check recipient deletion status: "+err.Error())
	} else if locked {
		return "", nil, 0, kit.NewError(http.StatusForbidden, "messaging_not_allowed", "This account is not available for messaging.")
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
	case 1, 2:
		// Privacy: identical reason either way — the sender must not learn
		// whether they blocked the recipient or the recipient blocked them.
		// Direction stays in server logs only (SendMessage logs eligibility).
		return EligibilityUserBlocked, nil, 0, nil
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
		ChatId:                chat.ID.String(),
		OtherUserId:           recipientID.String(),
		OtherUserName:         otherName,
		OtherUserUsername:     otherUsername,
		OtherUserBio:          otherBio,
		AvatarUrl:             avatarURL,
		AvatarFileId:          avatarFileID,
		CreatedAt:             timestamppb.New(chat.CreatedAt),
		UpdatedAt:             timestamppb.New(chat.UpdatedAt),
		OtherUserKeysRevision: otherUserKeysRevision,
		ProfileType:           otherProfileType,
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

// normalizeReplyToMessageID treats the all-zero UUID as absent.
// uuid.Parse accepts 00000000-... so this must run after validation and
// before insert on every write path (SendMessage, ConfirmChatUpload) to
// avoid persisting a dangling reply target.
func normalizeReplyToMessageID(replyToID *uuid.UUID) *uuid.UUID {
	if replyToID != nil && *replyToID == uuid.Nil {
		return nil
	}
	return replyToID
}

func (s *chatService) validateReplyToMessage(
	ctx context.Context,
	replyToID *uuid.UUID,
	messageID uuid.UUID,
	senderID uuid.UUID,
	recipientID uuid.UUID,
) error {
	if replyToID == nil || *replyToID == uuid.Nil {
		return nil
	}
	if *replyToID == messageID {
		return kit.NewError(http.StatusBadRequest, "invalid_reply_to_message_id", "Cannot reply to the message itself")
	}
	if s.PostgresQueries == nil {
		return nil
	}
	orig, err := s.PostgresQueries.GetMessageByID(ctx, *replyToID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Best-effort: relay is an ephemeral buffer; older messages expire or get pruned,
			// while the client's local SQLite DB and encrypted quote snapshot retain them.
			return nil
		}
		log.Printf("[validateReplyToMessage] GetMessageByID failed for %s: %v", replyToID, err)
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to verify reply to message")
	}

	// Require unordered pair match: {orig.SenderID, orig.RecipientID} == {new.SenderID, new.RecipientID}
	// Allows normal replies and self-replies within the same 1:1 conversation.
	isSamePair := (orig.SenderID == senderID && orig.RecipientID == recipientID) ||
		(orig.SenderID == recipientID && orig.RecipientID == senderID)
	if !isSamePair {
		return kit.NewError(http.StatusBadRequest, "invalid_reply_to_message_id", "Invalid reply to message id")
	}
	return nil
}

func (s *chatService) SendMessage(ctx context.Context, params SendMessageParams) (*personal_chat_store.Message, error) {
	// Idempotency check:
	if s.PostgresQueries != nil && params.MessageID != uuid.Nil {
		// Tier 1: Check active messages table
		if exists, err := s.PostgresQueries.CheckMessageExists(ctx, params.MessageID); err == nil && exists {
			log.Printf("[E2EE] SendMessage: IDEMPOTENT RETRY — message %s already exists. Fetching existing message.", params.MessageID)
			if existingMsg, err := s.PostgresQueries.GetMessageByID(ctx, params.MessageID); err == nil {
				return &existingMsg, nil
			}
		}
		// Tier 2: Check message_history table (purged tombstone)
		if recorded, err := s.PostgresQueries.CheckMessageHistoryExists(ctx, params.MessageID); err == nil && recorded {
			log.Printf("[E2EE] SendMessage: DUPLICATE BLOCKED — message %s was already delivered and purged from relay.", params.MessageID)
			return nil, kit.NewError(http.StatusConflict, "message_already_purged", "Message was already delivered and purged from relay")
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
	if err := s.validateReplyToMessage(ctx, params.ReplyToMessageID, params.MessageID, params.SenderID.UuidUserId, params.RecipientID); err != nil {
		return nil, err
	}
	params.ReplyToMessageID = normalizeReplyToMessageID(params.ReplyToMessageID)
	chat, chatErr := s.CreateOrGetChat(ctx, params.SenderID.UuidUserId, params.RecipientID)
	if chatErr != nil {
		return nil, chatErr
	}
	expiresAt := time.Now().Add(DefaultMessageTTL)

	var qtx = s.PostgresQueries
	var tx pgx.Tx
	if s.Pool != nil {
		var beginErr error
		tx, beginErr = s.Pool.Begin(ctx)
		if beginErr != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to start send transaction")
		}
		defer tx.Rollback(ctx)
		qtx = s.PostgresQueries.WithTx(tx)
	}

	message, dbErr := qtx.CreateMessage(ctx, personal_chat_store.CreateMessageParams{
		ID:                          params.MessageID,
		ChatID:                      chat.ID,
		SenderID:                    params.SenderID.UuidUserId,
		RecipientID:                 params.RecipientID,
		Content:                     params.Content,
		MessageType:                 params.MessageType,
		ExpiresAt:                   expiresAt,
		SyncedToSenderPrimary:       params.IsPrimary,
		SenderSessionID:             params.SessionUUID,
		DeliveredToRecipientPrimary: false,
		ReplyToMessageID:            params.ReplyToMessageID,
	})
	if dbErr != nil {
		// Handle PK duplicate key violation (race condition: both WS and REST passed CheckMessageExists)
		pgErr := kit.GetPostgresError(dbErr)
		if pgErr.PgError != nil && pgErr.PgError.Code == "23505" {
			log.Printf("[E2EE] SendMessage: PK CONFLICT (race) — message %s inserted concurrently. Fetching existing.", params.MessageID)
			if tx != nil {
				_ = tx.Rollback(ctx)
			}
			if existingMsg, err := s.PostgresQueries.GetMessageByID(ctx, params.MessageID); err == nil {
				return &existingMsg, nil
			}
		}
		return nil, kit.NewError(http.StatusInternalServerError, "message_send_failed", pgErr.Message)
	}

	if err := qtx.RecordMessageHistory(ctx, personal_chat_store.RecordMessageHistoryParams{
		ID:        message.ID,
		CreatedAt: message.CreatedAt,
	}); err != nil {
		log.Printf("[E2EE] SendMessage: failed to record message history for %s: %v", message.ID, err)
		return nil, kit.NewError(http.StatusInternalServerError, "message_history_record_failed", err.Error())
	}

	_ = qtx.UpdateChatStatus(ctx, personal_chat_store.UpdateChatStatusParams{
		ID:                   chat.ID,
		P1LastMessageContent: &message.Content,
		LastMessageCreatedAt: &message.CreatedAt,
		P1LastMessageType:    &message.MessageType,
		LastMessageSenderID:  &message.SenderID,
		LastMessageID:        &message.ID,
	})

	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to commit send transaction")
		}
	}

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

func (s *chatService) SendMessageHandler(ctx context.Context, payload *SendMessagePayload, userID kit.UserId, isPrimary bool, sessionUUID uuid.UUID) (*rpc_personal_chatv1.Message, error) {
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
	var replyToUUID *uuid.UUID
	if payload.ReplyToMessageID != nil && *payload.ReplyToMessageID != "" {
		parsed, err := uuid.Parse(*payload.ReplyToMessageID)
		if err != nil {
			return nil, kit.NewError(http.StatusBadRequest, "invalid_reply_to_message_id", "Invalid reply to message id")
		}
		replyToUUID = &parsed
	}
	message, sendErr := s.SendMessage(ctx, SendMessageParams{
		MessageID:             messageID,
		SenderID:              userID,
		RecipientID:           recipientID,
		Content:               payload.Content,
		MessageType:           payload.MessageType,
		IsPrimary:             isPrimary,
		SessionUUID:           sessionUUID,
		RecipientKeysRevision: payload.RecipientKeysRevision,
		SenderKeysRevision:    payload.SenderKeysRevision,
		ReplyToMessageID:      replyToUUID,
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
		ReplyToMessageId:      uuidPtrToStringPtr(message.ReplyToMessageID),
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Permanently Rejected Message IDs
// ──────────────────────────────────────────────────────────────────────────────

// isPermanentlyRejected reports whether a found message row can never be
// acted on by the caller, whatever the endpoint: the caller neither sent nor
// received it, or its TTL has passed. Missing rows and malformed IDs are the
// other permanently rejected cases.
//
// The client keeps ONE sticky flag per message for every endpoint, and a
// flagged id is never sent on any ACK, read or unsend call again. So only
// conditions that hold for every endpoint belong here. A row that fails just
// one endpoint's own rule (wrong side, other session, already read, not yet
// synced to the primary) is skipped by that endpoint and must not be rejected.
func isPermanentlyRejected(callerID, senderID, recipientID uuid.UUID, expiresAt, now time.Time) bool {
	if senderID != callerID && recipientID != callerID {
		return true
	}
	return !expiresAt.IsZero() && now.After(expiresAt)
}

// permanentlyRejectedIDs lists every malformed request ID and every request ID
// in rejected, in request order and spelled as in the request, with each UUID
// listed once. The result is never nil, so the REST responses built from the
// structs in personal_chat_mdl.go always carry an array. Handlers that return
// the proto message itself omit an empty list, like any repeated field.
func permanentlyRejectedIDs(rawIDs []string, rejected map[uuid.UUID]bool) []string {
	out := make([]string, 0)
	seen := make(map[string]bool)
	for _, raw := range rawIDs {
		key := raw
		if id, err := uuid.Parse(raw); err == nil {
			if !rejected[id] {
				continue
			}
			key = id.String()
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, raw)
	}
	return out
}

// ──────────────────────────────────────────────────────────────────────────────
// Delivery Acknowledgment
// ──────────────────────────────────────────────────────────────────────────────

// validateAcknowledgedBy rejects anything that is not an exact supported
// direction. Without this, an unknown value would silently take the sender
// branch in AcknowledgeDelivery.
func validateAcknowledgedBy(value string) error {
	if value != "recipient" && value != "sender" {
		return kit.NewError(http.StatusBadRequest, "invalid_request", "acknowledgedBy must be 'recipient' or 'sender'")
	}
	return nil
}

func (s *chatService) AcknowledgeDelivery(ctx context.Context, messageID uuid.UUID, acknowledgedBy string, isPrimary bool, userID kit.UserId) error {
	if err := validateAcknowledgedBy(acknowledgedBy); err != nil {
		return err
	}
	_, err := s.AcknowledgeDeliveryBatch(ctx, []string{messageID.String()}, acknowledgedBy, isPrimary, userID)
	return err
}

func (s *chatService) AcknowledgeDeliveryHandler(ctx context.Context, payload *AcknowledgeDeliveryPayload, userID kit.UserId, isPrimary bool) (*rpc_personal_chatv1.AcknowledgeDeliveryResponse, []AckDeliveryGroup, error) {
	if err := validateAcknowledgedBy(payload.AcknowledgedBy); err != nil {
		return nil, nil, err
	}
	if _, err := uuid.Parse(payload.MessageID); err != nil {
		return nil, nil, kit.NewError(http.StatusBadRequest, "invalid_message_id", "Invalid message ID")
	}
	if !payload.Success {
		return &rpc_personal_chatv1.AcknowledgeDeliveryResponse{Acknowledged: false}, nil, nil
	}
	batchRes, err := s.AcknowledgeDeliveryBatch(ctx, []string{payload.MessageID}, payload.AcknowledgedBy, isPrimary, userID)
	if err != nil {
		return nil, nil, err
	}
	isAcked := batchRes.AcknowledgedCount > 0
	return &rpc_personal_chatv1.AcknowledgeDeliveryResponse{Acknowledged: isAcked}, batchRes.DeliveryGroups, nil
}

func (s *chatService) AcknowledgeDeliveryBatch(ctx context.Context, messageIDs []string, acknowledgedBy string, isPrimary bool, userID kit.UserId) (*AckDeliveryBatchResult, error) {
	if err := validateAcknowledgedBy(acknowledgedBy); err != nil {
		return nil, err
	}
	if len(messageIDs) > MaxMessageIDsPerRequest {
		return nil, ErrTooManyMessageIDs
	}
	if len(messageIDs) == 0 {
		return &AckDeliveryBatchResult{
			AcknowledgedCount:      0,
			AcknowledgedMessageIDs: []string{},
			DeliveryGroups:         nil,
			PermanentlyRejectedIDs: []string{},
		}, nil
	}

	validUUIDs := make([]uuid.UUID, 0, len(messageIDs))
	seenMap := make(map[uuid.UUID]bool, len(messageIDs))
	for _, idStr := range messageIDs {
		parsed, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		if !seenMap[parsed] {
			seenMap[parsed] = true
			validUUIDs = append(validUUIDs, parsed)
		}
	}

	if len(validUUIDs) == 0 {
		return &AckDeliveryBatchResult{
			AcknowledgedCount:      0,
			AcknowledgedMessageIDs: []string{},
			DeliveryGroups:         nil,
			PermanentlyRejectedIDs: permanentlyRejectedIDs(messageIDs, nil),
		}, nil
	}

	if acknowledgedBy == "sender" && !isPrimary {
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "Forbidden: Only primary device can ACK sender sync")
	}

	// 1. Upfront GET query: inspect message state before any mutations
	existingMsgs, err := s.PostgresQueries.GetMessagesForDeliveryAckBatch(ctx, validUUIDs)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Internal server error")
	}
	existingMap := make(map[uuid.UUID]personal_chat_store.GetMessagesForDeliveryAckBatchRow, len(existingMsgs))
	for _, em := range existingMsgs {
		existingMap[em.ID] = em
	}

	now := time.Now()
	var eligibleUUIDs []uuid.UUID
	confirmedMap := make(map[uuid.UUID]bool, len(validUUIDs))
	rejectedMap := make(map[uuid.UUID]bool)
	var retryRelayCleanup []personal_chat_store.Message

	for _, uid := range validUUIDs {
		em, exists := existingMap[uid]
		if !exists {
			// Missing from the relay (deleted, cleaned up, or never existed):
			// permanently rejected.
			rejectedMap[uid] = true
			continue
		}

		// Tenant isolation and TTL expiration: rows the caller neither sent nor
		// received, and expired rows, are permanently rejected. Expiry is checked
		// before the side check below. Rejected rows do not participate in DB
		// write, ACK response, SSE, or relay cleanup.
		if isPermanentlyRejected(userID.UuidUserId, em.SenderID, em.RecipientID, em.ExpiresAt, now) {
			rejectedMap[uid] = true
			continue
		}

		// Side check: the caller's own message on the other side is only skipped.
		if acknowledgedBy == "recipient" && em.RecipientID != userID.UuidUserId {
			continue
		}
		if acknowledgedBy == "sender" && em.SenderID != userID.UuidUserId {
			continue
		}

		// Check if message is already acknowledged for this role and device tier
		alreadyAcked := false
		if acknowledgedBy == "recipient" {
			if isPrimary {
				alreadyAcked = em.DeliveredToRecipientPrimary
			} else {
				alreadyAcked = em.DeliveredToRecipient
			}
		} else {
			alreadyAcked = em.SyncedToSenderPrimary
		}

		if alreadyAcked {
			// Idempotent retry: confirm in response so client stops retrying
			confirmedMap[uid] = true

			// Retry relay cleanup if both primary delivery and primary sync are already true
			if em.DeliveredToRecipientPrimary && em.SyncedToSenderPrimary {
				retryRelayCleanup = append(retryRelayCleanup, personal_chat_store.Message{
					ID:                          em.ID,
					ChatID:                      em.ChatID,
					SenderID:                    em.SenderID,
					FileID:                      em.FileID,
					DeliveredToRecipientPrimary: em.DeliveredToRecipientPrimary,
					SyncedToSenderPrimary:       em.SyncedToSenderPrimary,
					ReadByRecipient:             em.ReadByRecipient,
					ReadAckedBySender:           em.ReadAckedBySender,
					CreatedAt:                   em.CreatedAt,
				})
			}
		} else {
			eligibleUUIDs = append(eligibleUUIDs, uid)
		}
	}

	type updatedRowInfo struct {
		id                          uuid.UUID
		chatID                      uuid.UUID
		senderID                    uuid.UUID
		fileID                      *string
		deliveredToRecipientPrimary bool
		syncedToSenderPrimary       bool
		readByRecipient             bool
		readAckedBySender           bool
		createdAt                   time.Time
	}

	var updatedRows []updatedRowInfo

	// 2. Execute batch UPDATE only for eligible UUIDs
	if len(eligibleUUIDs) > 0 {
		if acknowledgedBy == "recipient" {
			if isPrimary {
				rows, err := s.PostgresQueries.MarkMessagesDeliveredToRecipientPrimaryBatch(ctx, personal_chat_store.MarkMessagesDeliveredToRecipientPrimaryBatchParams{
					MessageIds:  eligibleUUIDs,
					RecipientID: userID.UuidUserId,
				})
				if err != nil {
					return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Internal server error")
				}
				for _, r := range rows {
					updatedRows = append(updatedRows, updatedRowInfo{
						id:                          r.ID,
						chatID:                      r.ChatID,
						senderID:                    r.SenderID,
						fileID:                      r.FileID,
						deliveredToRecipientPrimary: r.DeliveredToRecipientPrimary,
						syncedToSenderPrimary:       r.SyncedToSenderPrimary,
						readByRecipient:             r.ReadByRecipient,
						readAckedBySender:           r.ReadAckedBySender,
						createdAt:                   r.CreatedAt,
					})
					confirmedMap[r.ID] = true
				}
			} else {
				rows, err := s.PostgresQueries.MarkMessagesDeliveredToRecipientBatch(ctx, personal_chat_store.MarkMessagesDeliveredToRecipientBatchParams{
					MessageIds:  eligibleUUIDs,
					RecipientID: userID.UuidUserId,
				})
				if err != nil {
					return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Internal server error")
				}
				for _, r := range rows {
					updatedRows = append(updatedRows, updatedRowInfo{
						id:                          r.ID,
						chatID:                      r.ChatID,
						senderID:                    r.SenderID,
						fileID:                      r.FileID,
						deliveredToRecipientPrimary: r.DeliveredToRecipientPrimary,
						syncedToSenderPrimary:       r.SyncedToSenderPrimary,
						readByRecipient:             r.ReadByRecipient,
						readAckedBySender:           r.ReadAckedBySender,
						createdAt:                   r.CreatedAt,
					})
					confirmedMap[r.ID] = true
				}
			}
		} else {
			// acknowledgedBy == "sender" (isPrimary already validated above)
			rows, err := s.PostgresQueries.MarkMessagesSyncedToSenderPrimaryBatch(ctx, personal_chat_store.MarkMessagesSyncedToSenderPrimaryBatchParams{
				MessageIds: eligibleUUIDs,
				SenderID:   userID.UuidUserId,
			})
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Internal server error")
			}
			for _, r := range rows {
				updatedRows = append(updatedRows, updatedRowInfo{
					id:                          r.ID,
					chatID:                      r.ChatID,
					senderID:                    r.SenderID,
					fileID:                      r.FileID,
					deliveredToRecipientPrimary: r.DeliveredToRecipientPrimary,
					syncedToSenderPrimary:       r.SyncedToSenderPrimary,
					readByRecipient:             r.ReadByRecipient,
					readAckedBySender:           r.ReadAckedBySender,
					createdAt:                   r.CreatedAt,
				})
				confirmedMap[r.ID] = true
			}
		}
	}

	acknowledgedMessageIDs := make([]string, 0)
	for _, rawID := range messageIDs {
		parsed, err := uuid.Parse(rawID)
		if err == nil && confirmedMap[parsed] {
			acknowledgedMessageIDs = append(acknowledgedMessageIDs, rawID)
		}
	}

	// Recipient: update last_delivered_at in chats for newly updated messages
	if acknowledgedBy == "recipient" && len(updatedRows) > 0 {
		chatMaxCreatedAt := make(map[uuid.UUID]time.Time)
		for _, r := range updatedRows {
			if existing, ok := chatMaxCreatedAt[r.chatID]; !ok || r.createdAt.After(existing) {
				chatMaxCreatedAt[r.chatID] = r.createdAt
			}
		}
		for cID, maxTime := range chatMaxCreatedAt {
			_ = s.PostgresQueries.UpdateChatLastDeliveredAt(ctx, personal_chat_store.UpdateChatLastDeliveredAtParams{
				ChatID:          cID,
				ParticipantID:   userID.UuidUserId,
				LastDeliveredAt: maxTime,
			})
		}
	}

	// Async cleanup: relay deletion and chat read cleanup
	if len(updatedRows) > 0 || len(retryRelayCleanup) > 0 {
		var toRelayDelete []personal_chat_store.Message
		chatSet := make(map[uuid.UUID]bool)

		for _, r := range updatedRows {
			chatSet[r.chatID] = true
			if r.deliveredToRecipientPrimary && r.syncedToSenderPrimary {
				toRelayDelete = append(toRelayDelete, personal_chat_store.Message{
					ID:                          r.id,
					ChatID:                      r.chatID,
					SenderID:                    r.senderID,
					FileID:                      r.fileID,
					DeliveredToRecipientPrimary: r.deliveredToRecipientPrimary,
					SyncedToSenderPrimary:       r.syncedToSenderPrimary,
					ReadByRecipient:             r.readByRecipient,
					ReadAckedBySender:           r.readAckedBySender,
					CreatedAt:                   r.createdAt,
				})
			}
		}

		for _, m := range retryRelayCleanup {
			chatSet[m.ChatID] = true
			toRelayDelete = append(toRelayDelete, m)
		}

		uniqueChatIDs := make([]uuid.UUID, 0, len(chatSet))
		for cID := range chatSet {
			uniqueChatIDs = append(uniqueChatIDs, cID)
		}

		go func(candidates []personal_chat_store.Message, chats []uuid.UUID) {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, c := range candidates {
				s.deleteMessageFromRelay(cleanupCtx, c)
			}
			for _, cID := range chats {
				if err := s.PostgresQueries.CleanupFullyAcknowledgedReadMessagesInChat(cleanupCtx, cID); err != nil {
					log.Printf("[ACK-Batch] Async CleanupFullyAcknowledgedReadMessagesInChat ERROR for chat %s: %v", cID, err)
				}
			}
		}(toRelayDelete, uniqueChatIDs)
	}

	// Build delivery groups for SSE broadcast (only for recipient ACK)
	var deliveryGroups []AckDeliveryGroup
	if acknowledgedBy == "recipient" && len(updatedRows) > 0 {
		type groupKey struct {
			senderID uuid.UUID
			chatID   uuid.UUID
		}
		groupsMap := make(map[groupKey]*AckDeliveryGroup)
		var groupOrder []groupKey

		for _, r := range updatedRows {
			k := groupKey{senderID: r.senderID, chatID: r.chatID}
			g, exists := groupsMap[k]
			if !exists {
				g = &AckDeliveryGroup{
					SenderID:        r.senderID,
					ChatID:          r.chatID,
					MessageIDs:      nil,
					LatestCreatedAt: r.createdAt,
				}
				groupsMap[k] = g
				groupOrder = append(groupOrder, k)
			}
			g.MessageIDs = append(g.MessageIDs, r.id.String())
			if r.createdAt.After(g.LatestCreatedAt) {
				g.LatestCreatedAt = r.createdAt
			}
		}

		for _, k := range groupOrder {
			deliveryGroups = append(deliveryGroups, *groupsMap[k])
		}

		// The UPDATE ... RETURNING rows arrive in database order, which decides
		// both the group order and the order inside each group. Rebuild both from
		// validUUIDs (parsed, de-duplicated, in request order). Membership,
		// SenderID, ChatID and LatestCreatedAt are unchanged; only positions move.
		groupIndexByMessageID := make(map[string]int, len(updatedRows))
		for i := range deliveryGroups {
			for _, messageID := range deliveryGroups[i].MessageIDs {
				groupIndexByMessageID[messageID] = i
			}
			deliveryGroups[i].MessageIDs = nil
		}

		orderedGroups := make([]AckDeliveryGroup, 0, len(deliveryGroups))
		orderedIndexByGroup := make([]int, len(deliveryGroups))
		for i := range orderedIndexByGroup {
			orderedIndexByGroup[i] = -1
		}
		for _, id := range validUUIDs {
			key := id.String()
			groupIndex, ok := groupIndexByMessageID[key]
			if !ok {
				continue
			}
			if orderedIndexByGroup[groupIndex] < 0 {
				orderedIndexByGroup[groupIndex] = len(orderedGroups)
				orderedGroups = append(orderedGroups, deliveryGroups[groupIndex])
			}
			target := orderedIndexByGroup[groupIndex]
			orderedGroups[target].MessageIDs = append(orderedGroups[target].MessageIDs, key)
		}
		deliveryGroups = orderedGroups
	}

	return &AckDeliveryBatchResult{
		AcknowledgedCount:      int32(len(acknowledgedMessageIDs)),
		AcknowledgedMessageIDs: acknowledgedMessageIDs,
		DeliveryGroups:         deliveryGroups,
		PermanentlyRejectedIDs: permanentlyRejectedIDs(messageIDs, rejectedMap),
	}, nil
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
		ReplyToMessageID:            uuidPtrToStringPtr(msg.ReplyToMessageID),
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
			ReplyToMessageId:            messageResponse.ReplyToMessageID,
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
		var otherUserKeysRevision int32
		if cp, ok := contactProfiles[otherUserID]; ok && cp != nil {
			otherUserKeysRevision = cp.KeysRevision
		}

		chatResponses = append(chatResponses, &rpc_personal_chatv1.Chat{
			ChatId:                chat.ID.String(),
			OtherUserId:           otherUserID.String(),
			OtherUserName:         otherName,
			OtherUserUsername:     otherUsername,
			OtherUserBio:          otherBio,
			AvatarUrl:             avatarURL,
			AvatarFileId:          avatarFileID,
			CreatedAt:             timestamppb.New(chat.CreatedAt),
			UpdatedAt:             timestamppb.New(chat.UpdatedAt),
			OtherUserKeysRevision: otherUserKeysRevision,
			ProfileType:           otherProfileType,
		})
	}
	return &rpc_personal_chatv1.GetUserChatsResponse{
		Chats: chatResponses,
		Count: int32(len(chatResponses)),
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Mark Read
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) MarkChatRead(ctx context.Context, payload *MarkChatReadPayload, userID kit.UserId, isPrimary bool) (markChatReadOutcome, error) {
	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	seenMap := make(map[uuid.UUID]bool, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		if u, err := uuid.Parse(idStr); err == nil && !seenMap[u] {
			seenMap[u] = true
			msgUUIDs = append(msgUUIDs, u)
		}
	}

	// 1. Upfront Pre-Validation via dedicated query: Validate chat consistency, tenant isolation, and TTL expiration
	var targetChatID uuid.UUID
	var validIDs []uuid.UUID
	existingMap := make(map[uuid.UUID]personal_chat_store.GetMessagesForMarkChatReadRow)
	rejectedMap := make(map[uuid.UUID]bool)
	now := time.Now()

	if len(msgUUIDs) > 0 {
		existingRows, getErr := s.PostgresQueries.GetMessagesForMarkChatRead(ctx, msgUUIDs)
		if getErr != nil {
			return markChatReadOutcome{}, kit.NewError(http.StatusInternalServerError, "lookup_failed", "Failed to fetch messages")
		}
		rowsByID := make(map[uuid.UUID]personal_chat_store.GetMessagesForMarkChatReadRow, len(existingRows))
		for _, row := range existingRows {
			rowsByID[row.ID] = row
		}

		for _, id := range msgUUIDs {
			row, exists := rowsByID[id]
			// Rule 5: Missing rows, rows the caller neither sent nor received, and
			// expired rows are permanently rejected upfront. They take no part in
			// the chat checks below, so they never fail the request.
			if !exists || isPermanentlyRejected(userID.UuidUserId, row.SenderID, row.RecipientID, row.ExpiresAt, now) {
				rejectedMap[id] = true
				continue
			}

			// Rule 1: Chat consistency - if any message belongs to a different chat, reject entire request!
			if targetChatID == uuid.Nil {
				targetChatID = row.ChatID
			} else if targetChatID != row.ChatID {
				return markChatReadOutcome{}, kit.NewError(http.StatusBadRequest, "invalid_batch", "All messages in an in-chat batch must belong to the same chat")
			}

			existingMap[row.ID] = row

			// Rule 2: Participant check - only messages where caller is recipient are valid
			if row.RecipientID == userID.UuidUserId && !row.ReadByRecipient {
				validIDs = append(validIDs, row.ID)
			}
		}
	}

	// Derived from the DB rows only: the request carries no chat ID.
	// If no message survives the checks above, return smoothly with 0 DB mutations
	if targetChatID == uuid.Nil {
		return markChatReadOutcome{
			Receipts:               []*rpc_personal_chatv1.MessageReadReceipt{},
			NewlyRead:              []*rpc_personal_chatv1.MessageReadReceipt{},
			PermanentlyRejectedIDs: permanentlyRejectedIDs(payload.MessageIDs, rejectedMap),
		}, nil
	}

	chat, err := s.PostgresQueries.GetChatByID(ctx, targetChatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return markChatReadOutcome{}, kit.NewError(http.StatusNotFound, "not_found", "chat not found")
		}
		return markChatReadOutcome{}, kit.NewError(http.StatusInternalServerError, "fetch_failed", "Failed to fetch chat")
	}
	if chat.Participant1ID != userID.UuidUserId && chat.Participant2ID != userID.UuidUserId {
		return markChatReadOutcome{}, kit.NewError(http.StatusForbidden, "forbidden", "You are not a participant in this chat")
	}
	var otherUserID uuid.UUID
	if chat.Participant1ID == userID.UuidUserId {
		otherUserID = chat.Participant2ID
	} else {
		otherUserID = chat.Participant1ID
	}

	// Fallback only for callers without a pool (tests). The real value is read
	// back from ResetChatReadStatus below — `chat` was fetched before the write.
	readAt := chat.UpdatedAt

	// 2. Transaction Phase: Reset unread count and mark messages as read
	var readRows []personal_chat_store.MarkMessagesAsReadRow
	if s.Pool != nil {
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return markChatReadOutcome{}, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to begin tx")
		}
		defer tx.Rollback(ctx)
		qtx := s.PostgresQueries.WithTx(tx)

		// Lock the chat before message update to avoid deadlocks
		if _, lockErr := qtx.GetChatByIDForNoKeyUpdate(ctx, targetChatID); lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return markChatReadOutcome{}, kit.NewError(http.StatusNotFound, "not_found", "chat not found")
			}
			return markChatReadOutcome{}, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Internal server error")
		}

		resetChat, err := qtx.ResetChatReadStatus(ctx, personal_chat_store.ResetChatReadStatusParams{
			ID:             targetChatID,
			Participant1ID: userID.UuidUserId,
		})
		if err != nil {
			return markChatReadOutcome{}, kit.NewError(http.StatusInternalServerError, "mark_read_failed", "Failed to reset read status")
		}
		// Use the timestamp this statement actually wrote instead of the pre-tx
		// snapshot, otherwise the response and the SSE event carry the old value.
		readAt = resetChat

		if len(validIDs) > 0 {
			rRows, markErr := qtx.MarkMessagesAsRead(ctx, personal_chat_store.MarkMessagesAsReadParams{
				ChatID:      targetChatID,
				RecipientID: userID.UuidUserId,
				MessageIds:  validIDs,
			})
			if markErr != nil {
				return markChatReadOutcome{}, kit.NewError(http.StatusInternalServerError, "mark_read_failed", "Failed to mark messages as read")
			}
			readRows = rRows
		}

		if err := tx.Commit(ctx); err != nil {
			return markChatReadOutcome{}, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to commit")
		}

		// Async relay cleanup: delete fully acknowledged, read messages with no pending files in this chat without blocking response
		go func(cID uuid.UUID) {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.PostgresQueries.CleanupFullyAcknowledgedReadMessagesInChat(cleanupCtx, cID); err != nil {
				log.Printf("[MarkChatRead] Async CleanupFullyAcknowledgedReadMessagesInChat ERROR for chat %s: %v", cID, err)
			}
		}(targetChatID)
	}

	// 3. Build Read Receipts:
	// - newlyReadReceipts: ONLY messages newly marked read in DB in this call (powers SSE broadcast)
	// - readReceipts: newlyReadReceipts + already-read messages (powers endpoint response for caller idempotency)
	newlyReadReceipts := make([]*rpc_personal_chatv1.MessageReadReceipt, 0, len(readRows))
	readMap := make(map[uuid.UUID]bool, len(readRows))
	for _, r := range readRows {
		readMap[r.ID] = true
		receipt := &rpc_personal_chatv1.MessageReadReceipt{
			MessageId: r.ID.String(),
		}
		if r.ReadAt != nil {
			receipt.ReadAt = timestamppb.New(*r.ReadAt)
		}
		newlyReadReceipts = append(newlyReadReceipts, receipt)
	}

	newlyReadSet := make(map[string]bool, len(newlyReadReceipts))
	for _, receipt := range newlyReadReceipts {
		newlyReadSet[receipt.MessageId] = true
	}

	readReceipts := make([]*rpc_personal_chatv1.MessageReadReceipt, 0, len(newlyReadReceipts))
	readReceipts = append(readReceipts, newlyReadReceipts...)

	for _, id := range msgUUIDs {
		if readMap[id] {
			continue
		}
		row, exists := existingMap[id]
		if !exists {
			// Rule 4: Rejected rows (missing, neither sent nor received by the
			// caller, or expired) get no receipt and no SSE; they are listed in
			// PermanentlyRejectedIDs instead.
			continue
		}
		// Confirm if caller is recipient in the same chat AND message is already read
		if row.RecipientID == userID.UuidUserId && row.ChatID == targetChatID && row.ReadByRecipient {
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

	// The UPDATE ... RETURNING rows arrive in database order and the already-read
	// rows are appended afterwards. Rebuild both Receipts and the SSE NewlyRead
	// list from msgUUIDs (parsed, de-duplicated, in request order) so every
	// payload follows the caller's order.
	receiptByID := make(map[string]*rpc_personal_chatv1.MessageReadReceipt, len(readReceipts))
	for _, receipt := range readReceipts {
		receiptByID[receipt.MessageId] = receipt
	}
	orderedReceipts := make([]*rpc_personal_chatv1.MessageReadReceipt, 0, len(readReceipts))
	for _, id := range msgUUIDs {
		if receipt, ok := receiptByID[id.String()]; ok {
			orderedReceipts = append(orderedReceipts, receipt)
		}
	}
	readReceipts = orderedReceipts

	orderedNewlyRead := make([]*rpc_personal_chatv1.MessageReadReceipt, 0, len(newlyReadReceipts))
	for _, receipt := range orderedReceipts {
		if newlyReadSet[receipt.MessageId] {
			orderedNewlyRead = append(orderedNewlyRead, receipt)
		}
	}
	newlyReadReceipts = orderedNewlyRead

	return markChatReadOutcome{
		Receipts:               readReceipts,
		NewlyRead:              newlyReadReceipts,
		ChatID:                 targetChatID,
		OtherUserID:            otherUserID,
		ReadAt:                 readAt,
		PermanentlyRejectedIDs: permanentlyRejectedIDs(payload.MessageIDs, rejectedMap),
	}, nil
}

func (s *chatService) AcknowledgeAndReadBatch(ctx context.Context, payload *AckAndReadBatchPayload, userID kit.UserId, isPrimary bool) (ackAndReadBatchOutcome, error) {
	if payload == nil || len(payload.MessageIDs) == 0 {
		return ackAndReadBatchOutcome{Receipts: []*rpc_personal_chatv1.MessageReadReceipt{}}, nil
	}

	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	seenMap := make(map[uuid.UUID]bool, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		if u, err := uuid.Parse(idStr); err == nil && !seenMap[u] {
			seenMap[u] = true
			msgUUIDs = append(msgUUIDs, u)
		}
	}
	if len(msgUUIDs) == 0 {
		return ackAndReadBatchOutcome{
			Receipts:               []*rpc_personal_chatv1.MessageReadReceipt{},
			PermanentlyRejectedIDs: permanentlyRejectedIDs(payload.MessageIDs, nil),
		}, nil
	}

	// 1. Upfront Pre-Validation via dedicated query: inspect state before any mutations
	existingRows, getErr := s.PostgresQueries.GetMessagesForAckAndReadBatch(ctx, msgUUIDs)
	if getErr != nil {
		return ackAndReadBatchOutcome{}, kit.NewError(http.StatusInternalServerError, "lookup_failed", "Failed to fetch messages")
	}

	existingMap := make(map[uuid.UUID]personal_chat_store.GetMessagesForAckAndReadBatchRow, len(existingRows))
	for _, row := range existingRows {
		existingMap[row.ID] = row
	}

	now := time.Now()
	var targetChatID uuid.UUID
	var eligibleIDs []uuid.UUID
	var deliveryEligibleIDs []uuid.UUID
	var alreadyReadReceipts []*rpc_personal_chatv1.MessageReadReceipt
	var retryCleanupIDs []uuid.UUID
	rejectedMap := make(map[uuid.UUID]bool)

	for _, uid := range msgUUIDs {
		em, exists := existingMap[uid]
		// Rules 3 & 5: Missing rows, rows the caller neither sent nor received, and
		// expired rows are permanently rejected. They do not participate in the
		// chat check, DB write, ACK response, SSE, or relay cleanup.
		if !exists || isPermanentlyRejected(userID.UuidUserId, em.SenderID, em.RecipientID, em.ExpiresAt, now) {
			rejectedMap[uid] = true
			continue
		}

		// Rule 1: Chat consistency - if any message belongs to a different chat, reject entire request!
		if targetChatID == uuid.Nil {
			targetChatID = em.ChatID
		} else if targetChatID != em.ChatID {
			return ackAndReadBatchOutcome{}, kit.NewError(http.StatusBadRequest, "invalid_batch", "All messages in an in-chat batch must belong to the same chat")
		}

		// Rule 4: Tenant isolation: caller must be recipient.
		if em.RecipientID != userID.UuidUserId {
			continue
		}

		if em.ReadByRecipient {
			// Rule 1: Already True: confirm in response for idempotency, no DB write, no SSE.
			rAt := now
			if em.ReadAt != nil {
				rAt = *em.ReadAt
			}
			alreadyReadReceipts = append(alreadyReadReceipts, &rpc_personal_chatv1.MessageReadReceipt{
				MessageId: uid.String(),
				ReadAt:    timestamppb.New(rAt),
			})

			// Retry relay cleanup if both primary delivery and primary sync are already true
			if em.DeliveredToRecipientPrimary && em.SyncedToSenderPrimary {
				retryCleanupIDs = append(retryCleanupIDs, uid)
			}
		} else {
			// Rule 2: Eligible: needs DB write to mark read
			eligibleIDs = append(eligibleIDs, uid)

			// Upfront filter for ACK (delivery) part: only include if not already delivered for this device tier
			if isPrimary {
				if !em.DeliveredToRecipientPrimary {
					deliveryEligibleIDs = append(deliveryEligibleIDs, uid)
				}
			} else {
				if !em.DeliveredToRecipient {
					deliveryEligibleIDs = append(deliveryEligibleIDs, uid)
				}
			}
		}
	}

	if len(eligibleIDs) == 0 && len(alreadyReadReceipts) == 0 {
		return ackAndReadBatchOutcome{
			Receipts:               []*rpc_personal_chatv1.MessageReadReceipt{},
			PermanentlyRejectedIDs: permanentlyRejectedIDs(payload.MessageIDs, rejectedMap),
		}, nil
	}

	// 2. Transaction Phase: Update delivery, reset chat unread status, and mark read for the target chat
	var readRows []personal_chat_store.MarkMessagesAsReadRow
	var readAt time.Time
	var otherUserID uuid.UUID
	if targetChatID != uuid.Nil && s.Pool != nil {
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return ackAndReadBatchOutcome{}, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to begin tx")
		}
		defer tx.Rollback(ctx)
		qtx := s.PostgresQueries.WithTx(tx)

		// Lock the chat before any message update to avoid deadlocks
		chat, lockErr := qtx.GetChatByIDForNoKeyUpdate(ctx, targetChatID)
		if lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return ackAndReadBatchOutcome{}, kit.NewError(http.StatusNotFound, "not_found", "chat not found")
			}
			return ackAndReadBatchOutcome{}, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Internal server error")
		}
		if chat.Participant1ID != userID.UuidUserId && chat.Participant2ID != userID.UuidUserId {
			return ackAndReadBatchOutcome{}, kit.NewError(http.StatusForbidden, "forbidden", "You are not a participant in this chat")
		}
		if chat.Participant1ID == userID.UuidUserId {
			otherUserID = chat.Participant2ID
		} else {
			otherUserID = chat.Participant1ID
		}

		if len(deliveryEligibleIDs) > 0 {
			var latestCreatedAt time.Time
			if isPrimary {
				rows, batchErr := qtx.MarkMessagesDeliveredToRecipientPrimaryBatch(ctx, personal_chat_store.MarkMessagesDeliveredToRecipientPrimaryBatchParams{
					RecipientID: userID.UuidUserId,
					MessageIds:  deliveryEligibleIDs,
				})
				if batchErr != nil {
					return ackAndReadBatchOutcome{}, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Internal server error")
				}
				for _, r := range rows {
					if r.CreatedAt.After(latestCreatedAt) {
						latestCreatedAt = r.CreatedAt
					}
				}
			} else {
				rows, batchErr := qtx.MarkMessagesDeliveredToRecipientBatch(ctx, personal_chat_store.MarkMessagesDeliveredToRecipientBatchParams{
					RecipientID: userID.UuidUserId,
					MessageIds:  deliveryEligibleIDs,
				})
				if batchErr != nil {
					return ackAndReadBatchOutcome{}, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Internal server error")
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
		}

		resetChat, err := qtx.ResetChatReadStatus(ctx, personal_chat_store.ResetChatReadStatusParams{
			ID:             targetChatID,
			Participant1ID: userID.UuidUserId,
		})
		if err != nil {
			return ackAndReadBatchOutcome{}, kit.NewError(http.StatusInternalServerError, "mark_read_failed", "Failed to reset read status")
		}
		readAt = resetChat

		if len(eligibleIDs) > 0 {
			rRows, markErr := qtx.MarkMessagesAsRead(ctx, personal_chat_store.MarkMessagesAsReadParams{
				ChatID:      targetChatID,
				RecipientID: userID.UuidUserId,
				MessageIds:  eligibleIDs,
			})
			if markErr != nil {
				return ackAndReadBatchOutcome{}, kit.NewError(http.StatusInternalServerError, "mark_read_failed", "Failed to mark messages as read")
			}
			readRows = rRows
		}

		if err := tx.Commit(ctx); err != nil {
			return ackAndReadBatchOutcome{}, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to commit tx")
		}

		// Async relay cleanup for this specific chat
		cleanupIDs := make([]uuid.UUID, 0, len(eligibleIDs)+len(retryCleanupIDs))
		cleanupIDs = append(cleanupIDs, eligibleIDs...)
		cleanupIDs = append(cleanupIDs, retryCleanupIDs...)
		if len(cleanupIDs) > 0 {
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
			}(targetChatID, cleanupIDs)
		}
	}

	if readAt.IsZero() {
		readAt = now
	}

	// 3. Build Read Receipts:
	// - newlyReadReceipts: ONLY messages newly marked read in DB in this call (powers SSE broadcast)
	// - readReceipts: newlyReadReceipts + already-read messages (powers endpoint response for caller idempotency)
	newlyReadReceipts := make([]*rpc_personal_chatv1.MessageReadReceipt, 0, len(readRows))
	for _, r := range readRows {
		receipt := &rpc_personal_chatv1.MessageReadReceipt{
			MessageId: r.ID.String(),
		}
		if r.ReadAt != nil {
			receipt.ReadAt = timestamppb.New(*r.ReadAt)
		}
		newlyReadReceipts = append(newlyReadReceipts, receipt)
	}

	newlyReadSet := make(map[string]bool, len(newlyReadReceipts))
	for _, receipt := range newlyReadReceipts {
		newlyReadSet[receipt.MessageId] = true
	}

	readReceipts := make([]*rpc_personal_chatv1.MessageReadReceipt, 0, len(newlyReadReceipts)+len(alreadyReadReceipts))
	readReceipts = append(readReceipts, newlyReadReceipts...)
	readReceipts = append(readReceipts, alreadyReadReceipts...)

	// The UPDATE ... RETURNING rows arrive in database order and the already-read
	// rows are appended afterwards. Rebuild both Receipts and the SSE NewlyRead
	// list from msgUUIDs (parsed, de-duplicated, in request order) so every
	// payload follows the caller's order.
	receiptByID := make(map[string]*rpc_personal_chatv1.MessageReadReceipt, len(readReceipts))
	for _, receipt := range readReceipts {
		receiptByID[receipt.MessageId] = receipt
	}
	orderedReceipts := make([]*rpc_personal_chatv1.MessageReadReceipt, 0, len(readReceipts))
	for _, id := range msgUUIDs {
		if receipt, ok := receiptByID[id.String()]; ok {
			orderedReceipts = append(orderedReceipts, receipt)
		}
	}
	readReceipts = orderedReceipts

	orderedNewlyRead := make([]*rpc_personal_chatv1.MessageReadReceipt, 0, len(newlyReadReceipts))
	for _, receipt := range orderedReceipts {
		if newlyReadSet[receipt.MessageId] {
			orderedNewlyRead = append(orderedNewlyRead, receipt)
		}
	}
	newlyReadReceipts = orderedNewlyRead

	return ackAndReadBatchOutcome{
		Receipts:               readReceipts,
		NewlyRead:              newlyReadReceipts,
		ChatID:                 targetChatID,
		OtherUserID:            otherUserID,
		ReadAt:                 readAt,
		PermanentlyRejectedIDs: permanentlyRejectedIDs(payload.MessageIDs, rejectedMap),
	}, nil
}

// AcknowledgeReadReceiptBatch is the sender primary's Stage 2 ACK. The request
// carries message IDs only, and unlike MarkChatRead, AcknowledgeAndReadBatch
// and UnsendMessage there is no one-chat rule: the IDs may come from any chats.
// Each row is checked on its own, and the UPDATE is scoped by the caller's
// sender_id. As in AcknowledgeDeliveryBatch, the chat-wide cleanup then runs
// once for the chat of every row it ACKs now or retries relay cleanup for.
func (s *chatService) AcknowledgeReadReceiptBatch(ctx context.Context, payload *AckReadReceiptBatchPayload, userID kit.UserId, isPrimary bool) (*rpc_personal_chatv1.AckReadReceiptBatchResponse, error) {
	if !isPrimary {
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "Forbidden: Only primary device can ACK read receipts")
	}

	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	seenMap := make(map[uuid.UUID]bool, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		if u, err := uuid.Parse(idStr); err == nil && !seenMap[u] {
			seenMap[u] = true
			msgUUIDs = append(msgUUIDs, u)
		}
	}

	if len(msgUUIDs) == 0 {
		return &rpc_personal_chatv1.AckReadReceiptBatchResponse{
			AcknowledgedCount:             0,
			AcknowledgedMessageIds:        []string{},
			PermanentlyRejectedMessageIds: permanentlyRejectedIDs(payload.MessageIDs, nil),
		}, nil
	}

	// 1. Upfront GET query: inspect message state before any mutations
	existingMsgs, err := s.PostgresQueries.GetMessagesForReadAckBatch(ctx, msgUUIDs)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "fetch_messages_failed", kit.GetPostgresError(err).Message)
	}

	existingMap := make(map[uuid.UUID]personal_chat_store.GetMessagesForReadAckBatchRow, len(existingMsgs))
	for _, em := range existingMsgs {
		existingMap[em.ID] = em
	}

	now := time.Now()
	var eligibleUUIDs []uuid.UUID
	confirmedMap := make(map[uuid.UUID]bool, len(msgUUIDs))
	rejectedMap := make(map[uuid.UUID]bool)
	var retryRelayCleanup []personal_chat_store.Message

	for _, uid := range msgUUIDs {
		em, exists := existingMap[uid]
		if !exists {
			// Rule 3: Non-existent rows are permanently rejected.
			rejectedMap[uid] = true
			continue
		}

		// Rule 5: Rows the caller neither sent nor received, and expired rows, are
		// permanently rejected. Expiry is checked before the side and read checks
		// below. Rejected rows do not participate in DB write, ACK response, or
		// relay cleanup.
		if isPermanentlyRejected(userID.UuidUserId, em.SenderID, em.RecipientID, em.ExpiresAt, now) {
			rejectedMap[uid] = true
			continue
		}

		// Rule 4: Eligibility isolation:
		// Caller must be sender and message must have been read by recipient.
		// Strictly skip non-eligible rows without failing the batch.
		if em.SenderID != userID.UuidUserId || !em.ReadByRecipient {
			continue
		}

		// Rule 1: Check if message is already acknowledged by sender
		if em.ReadAckedBySender {
			// Idempotent retry: confirm in response so client stops retrying
			confirmedMap[uid] = true

			// Retry relay cleanup if delivered to primary and synced to primary
			if em.DeliveredToRecipientPrimary && em.SyncedToSenderPrimary {
				retryRelayCleanup = append(retryRelayCleanup, personal_chat_store.Message{
					ID:                          em.ID,
					ChatID:                      em.ChatID,
					SenderID:                    em.SenderID,
					FileID:                      em.FileID,
					DeliveredToRecipientPrimary: em.DeliveredToRecipientPrimary,
					SyncedToSenderPrimary:       em.SyncedToSenderPrimary,
					ReadByRecipient:             em.ReadByRecipient,
					ReadAckedBySender:           em.ReadAckedBySender,
					CreatedAt:                   em.CreatedAt,
				})
			}
		} else {
			// Rule 2: Eligible for read receipt ACK
			eligibleUUIDs = append(eligibleUUIDs, uid)
		}
	}

	var toRelayDelete []personal_chat_store.Message

	// Chats to sweep after this call, as in AcknowledgeDeliveryBatch: the chat
	// of every row ACKed now and of every row whose relay cleanup is retried,
	// each once. Only rows the caller sent get this far, so these are always
	// the caller's own chats.
	var sweepChatIDs []uuid.UUID
	sweepChats := make(map[uuid.UUID]bool)
	addSweepChat := func(chatID uuid.UUID) {
		if !sweepChats[chatID] {
			sweepChats[chatID] = true
			sweepChatIDs = append(sweepChatIDs, chatID)
		}
	}
	for _, m := range retryRelayCleanup {
		addSweepChat(m.ChatID)
	}

	// 2. Execute batch UPDATE only for eligible UUIDs
	if len(eligibleUUIDs) > 0 {
		ackedRows, err := s.PostgresQueries.MarkMessagesReadAckedBySender(ctx, personal_chat_store.MarkMessagesReadAckedBySenderParams{
			SenderID:   userID.UuidUserId,
			MessageIds: eligibleUUIDs,
		})
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "mark_read_ack_failed", kit.GetPostgresError(err).Message)
		}

		for _, r := range ackedRows {
			confirmedMap[r.ID] = true
			addSweepChat(r.ChatID)
			if r.DeliveredToRecipientPrimary && r.SyncedToSenderPrimary {
				toRelayDelete = append(toRelayDelete, personal_chat_store.Message{
					ID:                          r.ID,
					ChatID:                      r.ChatID,
					SenderID:                    r.SenderID,
					FileID:                      r.FileID,
					DeliveredToRecipientPrimary: r.DeliveredToRecipientPrimary,
					SyncedToSenderPrimary:       r.SyncedToSenderPrimary,
					ReadByRecipient:             r.ReadByRecipient,
					ReadAckedBySender:           r.ReadAckedBySender,
					CreatedAt:                   r.CreatedAt,
				})
			}
		}
	}

	// 3. Assemble response preserving payload order. Never nil, so the REST
	// response always carries an array, also when nothing is confirmed.
	finalAckedIDs := make([]string, 0, len(confirmedMap))
	for _, idStr := range payload.MessageIDs {
		if u, err := uuid.Parse(idStr); err == nil && confirmedMap[u] {
			finalAckedIDs = append(finalAckedIDs, idStr)
		}
	}

	// 4. Async relay cleanup, as in AcknowledgeDeliveryBatch: delete each newly
	// acknowledged or retried message that both primary devices hold, then
	// sweep each chat in sweepChatIDs. The rows may come from several chats.
	if len(sweepChatIDs) > 0 {
		allCandidates := append(toRelayDelete, retryRelayCleanup...)
		go func(candidates []personal_chat_store.Message, chatIDs []uuid.UUID) {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, c := range candidates {
				s.deleteMessageFromRelay(cleanupCtx, c)
			}
			for _, cID := range chatIDs {
				if err := s.PostgresQueries.CleanupFullyAcknowledgedReadMessagesInChat(cleanupCtx, cID); err != nil {
					log.Printf("[AcknowledgeReadReceiptBatch] Async CleanupFullyAcknowledgedReadMessagesInChat ERROR for chat %s: %v", cID, err)
				}
			}
		}(allCandidates, sweepChatIDs)
	}

	return &rpc_personal_chatv1.AckReadReceiptBatchResponse{
		AcknowledgedCount:             int32(len(finalAckedIDs)),
		AcknowledgedMessageIds:        finalAckedIDs,
		PermanentlyRejectedMessageIds: permanentlyRejectedIDs(payload.MessageIDs, rejectedMap),
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Unsend (DB transaction first; attached files are deleted from R2 after commit)
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) UnsendMessageHandler(ctx context.Context, payload *UnsendMessagePayload, userID kit.UserId, isPrimary bool) (unsendOutcome, error) {
	if payload == nil || len(payload.MessageIDs) == 0 {
		return unsendOutcome{AcknowledgedIDs: []string{}, PermanentlyRejectedIDs: []string{}}, nil
	}
	if len(payload.MessageIDs) > MaxMessageIDsPerRequest {
		return unsendOutcome{}, ErrTooManyMessageIDs
	}

	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	seenMap := make(map[uuid.UUID]bool, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		if id, err := uuid.Parse(idStr); err == nil && !seenMap[id] {
			seenMap[id] = true
			msgUUIDs = append(msgUUIDs, id)
		}
	}
	if len(msgUUIDs) == 0 {
		return unsendOutcome{
			AcknowledgedIDs:        []string{},
			PermanentlyRejectedIDs: permanentlyRejectedIDs(payload.MessageIDs, nil),
		}, nil
	}

	// The database returns rows in arbitrary order. Rebuild the confirmed ID
	// lists from msgUUIDs (already parsed, de-duplicated and in request order)
	// so the response, SSE event and sync-action payload all follow the
	// caller's order.
	orderConfirmedIDs := func(newlyUnsent, alreadyUnsent []string) (orderedNewlyUnsent, orderedAcknowledged []string) {
		newlySet := make(map[string]bool, len(newlyUnsent))
		for _, id := range newlyUnsent {
			newlySet[id] = true
		}
		alreadySet := make(map[string]bool, len(alreadyUnsent))
		for _, id := range alreadyUnsent {
			alreadySet[id] = true
		}
		orderedNewlyUnsent = make([]string, 0, len(newlyUnsent))
		orderedAcknowledged = make([]string, 0, len(newlyUnsent)+len(alreadyUnsent))
		for _, id := range msgUUIDs {
			key := id.String()
			if newlySet[key] {
				orderedNewlyUnsent = append(orderedNewlyUnsent, key)
				orderedAcknowledged = append(orderedAcknowledged, key)
			} else if alreadySet[key] {
				orderedAcknowledged = append(orderedAcknowledged, key)
			}
		}
		return orderedNewlyUnsent, orderedAcknowledged
	}

	// 1. Upfront Batch Lookup: Look up actual messages from DB first
	existingRows, getErr := s.PostgresQueries.GetMessagesForUnsendBatch(ctx, msgUUIDs)
	if getErr != nil {
		return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "lookup_failed", "Failed to fetch messages")
	}
	rowsByID := make(map[uuid.UUID]personal_chat_store.GetMessagesForUnsendBatchRow, len(existingRows))
	for _, row := range existingRows {
		rowsByID[row.ID] = row
	}

	// 2. Permanently reject missing rows, rows the caller neither sent nor
	// received, and expired rows. Derive targetChatID strictly from the remaining
	// DB messages and enforce single chat consistency on them only.
	var targetChatID uuid.UUID
	now := time.Now()
	rejectedMap := make(map[uuid.UUID]bool)

	for _, id := range msgUUIDs {
		row, exists := rowsByID[id]
		if !exists || isPermanentlyRejected(userID.UuidUserId, row.SenderID, row.RecipientID, row.ExpiresAt, now) {
			rejectedMap[id] = true
			continue
		}
		if targetChatID == uuid.Nil {
			targetChatID = row.ChatID
		} else if targetChatID != row.ChatID {
			return unsendOutcome{}, kit.NewError(http.StatusBadRequest, "invalid_batch", "All messages in an in-chat batch must belong to the same chat")
		}
	}

	// If no message survives the checks above, nothing to unsend; return 0 acknowledged smoothly
	if targetChatID == uuid.Nil {
		return unsendOutcome{
			AcknowledgedIDs:        []string{},
			PermanentlyRejectedIDs: permanentlyRejectedIDs(payload.MessageIDs, rejectedMap),
		}, nil
	}

	// 3. Participant check: Validate chat existence and that caller is participant
	chat, err := s.PostgresQueries.GetChatByID(ctx, targetChatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return unsendOutcome{}, kit.NewError(http.StatusNotFound, "not_found", "chat not found")
		}
		return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "fetch_failed", "Failed to fetch chat")
	}
	if chat.Participant1ID != userID.UuidUserId && chat.Participant2ID != userID.UuidUserId {
		return unsendOutcome{}, kit.NewError(http.StatusForbidden, "forbidden", "You are not a participant in this chat")
	}
	var recipientID uuid.UUID
	if chat.Participant1ID == userID.UuidUserId {
		recipientID = chat.Participant2ID
	} else {
		recipientID = chat.Participant1ID
	}

	// 4. Partition messages:
	// - Already unsent (sender == caller && message_type == "unsent"): Idempotent, returned in response, excluded from DB write and SSE. Participates in file cleanup if file still attached.
	// - Eligible: sender == caller && !read_by_recipient && message_type != "unsent" && not expired
	// - Permanently rejected (missing, neither sent nor received by the caller, or expired): left out of the DB write, SSE, acknowledged IDs and file cleanup; listed in PermanentlyRejectedIDs.
	// - Non-eligible (the caller received it, or read_by_recipient == true): the caller's own rows that fail the unsend rule; left out of the DB write, SSE and response, and not rejected.
	alreadyUnsentIDs := make([]string, 0)
	eligibleRows := make([]personal_chat_store.GetMessagesForUnsendBatchRow, 0, len(existingRows))
	retryFileRows := make([]personal_chat_store.GetMessagesForUnsendBatchRow, 0)

	for _, row := range existingRows {
		// Skip permanently rejected rows (neither sent nor received, or expired)
		if rejectedMap[row.ID] {
			continue
		}
		if row.SenderID != userID.UuidUserId {
			continue // ineligible: caller can only unsend own messages
		}
		if row.ReadByRecipient {
			continue // ineligible: already read messages cannot be unsent
		}
		if row.MessageType == "unsent" {
			alreadyUnsentIDs = append(alreadyUnsentIDs, row.ID.String())
			// Still participate in file cleanup if file still attached
			if row.FileID != nil && *row.FileID != "" {
				retryFileRows = append(retryFileRows, row)
			}
			continue
		}
		eligibleRows = append(eligibleRows, row)
	}

	// If no messages require unsend or file cleanup, return immediately (0 DB operations, 0 SSE events)
	if len(eligibleRows) == 0 && len(retryFileRows) == 0 {
		_, orderedAcknowledged := orderConfirmedIDs(nil, alreadyUnsentIDs)
		return unsendOutcome{
			AcknowledgedIDs:        orderedAcknowledged,
			ChatID:                 targetChatID,
			RecipientID:            recipientID,
			PermanentlyRejectedIDs: permanentlyRejectedIDs(payload.MessageIDs, rejectedMap),
		}, nil
	}

	// 5. DB Transaction for unsend updates and file registration
	qtx := s.PostgresQueries
	var tx pgx.Tx
	if s.Pool != nil {
		var err error
		tx, err = s.Pool.Begin(ctx)
		if err != nil {
			return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to start unsend transaction")
		}
		defer tx.Rollback(ctx)
		qtx = s.PostgresQueries.WithTx(tx)
	}

	// Lock chat row to prevent deadlocks with concurrent mark-read or message sending
	_, err = qtx.GetChatByIDForNoKeyUpdate(ctx, targetChatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return unsendOutcome{}, kit.NewError(http.StatusNotFound, "not_found", "chat not found")
		}
		return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Internal server error")
	}

	var (
		filesToCleanup      []string
		msgIDsToClearFiles  []uuid.UUID
		fileRegistrationIDs []string
		fileBuckets         []string
		fileObjectKeys      []string
		seenFileIDs         = make(map[string]bool)
	)

	collectFileForCleanup := func(msgID uuid.UUID, fileID string) {
		msgIDsToClearFiles = append(msgIDsToClearFiles, msgID)
		if !seenFileIDs[fileID] {
			seenFileIDs[fileID] = true
			filesToCleanup = append(filesToCleanup, fileID)

			bucket := ""
			objectKey := fileID
			if s.R2Pool != nil {
				_, objectKey = clients.ParseFilePrefix(fileID)
				if client := s.R2Pool.GetClient(fileID); client != nil {
					bucket = client.ChatBucket()
				}
			}
			fileRegistrationIDs = append(fileRegistrationIDs, fileID)
			fileBuckets = append(fileBuckets, bucket)
			fileObjectKeys = append(fileObjectKeys, objectKey)
		}
	}

	// Collect retry file cleanups for already-unsent messages
	for _, msg := range retryFileRows {
		if msg.FileID != nil && *msg.FileID != "" {
			collectFileForCleanup(msg.ID, *msg.FileID)
		}
	}

	newlyUnsentIDs := make([]string, 0, len(eligibleRows))
	var newlyUnsentUUIDs []uuid.UUID

	if len(eligibleRows) == 1 {
		msg := eligibleRows[0]
		rowsAffected, updateErr := qtx.UpdateMessageToUnsent(ctx, msg.ID)
		if updateErr != nil {
			return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "server_error", "failed to create message tombstone")
		}
		if rowsAffected == 0 {
			// Concurrently modified: check if it already became unsent
			latest, getErr := qtx.GetMessageByID(ctx, msg.ID)
			if getErr == nil && latest.MessageType == "unsent" {
				alreadyUnsentIDs = append(alreadyUnsentIDs, msg.ID.String())
			}
		} else {
			newlyUnsentIDs = append(newlyUnsentIDs, msg.ID.String())
			newlyUnsentUUIDs = append(newlyUnsentUUIDs, msg.ID)
			if msg.FileID != nil && *msg.FileID != "" {
				collectFileForCleanup(msg.ID, *msg.FileID)
			}
		}
	} else if len(eligibleRows) > 1 {
		eligibleMsgIDs := make([]uuid.UUID, len(eligibleRows))
		fileMap := make(map[uuid.UUID]*string, len(eligibleRows))
		for i, r := range eligibleRows {
			eligibleMsgIDs[i] = r.ID
			fileMap[r.ID] = r.FileID
		}
		updatedUUIDs, updateErr := qtx.UpdateMessagesToUnsentBatch(ctx, eligibleMsgIDs)
		if updateErr != nil {
			return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "server_error", "failed to create message tombstone")
		}
		newlyUnsentUUIDs = updatedUUIDs
		for _, id := range updatedUUIDs {
			newlyUnsentIDs = append(newlyUnsentIDs, id.String())
			if fid := fileMap[id]; fid != nil && *fid != "" {
				collectFileForCleanup(id, *fid)
			}
		}
		if len(updatedUUIDs) < len(eligibleRows) {
			updatedSet := make(map[uuid.UUID]bool, len(updatedUUIDs))
			for _, id := range updatedUUIDs {
				updatedSet[id] = true
			}
			missingIDs := make([]uuid.UUID, 0, len(eligibleRows)-len(updatedUUIDs))
			for _, msg := range eligibleRows {
				if !updatedSet[msg.ID] {
					missingIDs = append(missingIDs, msg.ID)
				}
			}
			// One batch re-check classifies every row that lost the race between
			// the upfront lookup and the UPDATE; never fall back to GetMessageByID per ID.
			latestRows, getErr := qtx.GetMessagesForUnsendBatch(ctx, missingIDs)
			if getErr == nil {
				latestByID := make(map[uuid.UUID]personal_chat_store.GetMessagesForUnsendBatchRow, len(latestRows))
				for _, latest := range latestRows {
					latestByID[latest.ID] = latest
				}
				for _, msg := range eligibleRows {
					if updatedSet[msg.ID] {
						continue
					}
					if latest, ok := latestByID[msg.ID]; ok && latest.MessageType == "unsent" {
						alreadyUnsentIDs = append(alreadyUnsentIDs, msg.ID.String())
					}
				}
			}
		}
	}

	newlyUnsentIDs, confirmedIDs := orderConfirmedIDs(newlyUnsentIDs, alreadyUnsentIDs)

	if len(newlyUnsentIDs) > 0 {
		if !isPrimary {
			senderPayload, _ := json.Marshal(SyncActionPayload{MessageIDs: newlyUnsentIDs, ChatID: targetChatID.String()})
			_, err := qtx.CreateSyncAction(ctx, personal_chat_store.CreateSyncActionParams{
				ID:         uuid.New(),
				UserID:     userID.UuidUserId,
				ActionType: "unsend",
				Payload:    senderPayload,
			})
			if err != nil {
				log.Printf("[UnsendMessage] CreateSyncAction (sender) error: %v", err)
				return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "server_error", "failed to create sync action for sender")
			}
		}
		recipientPayload, _ := json.Marshal(SyncActionPayload{MessageIDs: newlyUnsentIDs, ChatID: targetChatID.String()})
		_, err := qtx.CreateSyncAction(ctx, personal_chat_store.CreateSyncActionParams{
			ID:         uuid.New(),
			UserID:     recipientID,
			ActionType: "unsend",
			Payload:    recipientPayload,
		})
		if err != nil {
			log.Printf("[UnsendMessage] CreateSyncAction (recipient) error: %v", err)
			return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "server_error", "failed to create sync action for recipient")
		}
		if len(newlyUnsentUUIDs) == 1 {
			if err := qtx.UpdateChatUnsendPreview(ctx, personal_chat_store.UpdateChatUnsendPreviewParams{
				ID:            targetChatID,
				LastMessageID: &newlyUnsentUUIDs[0],
			}); err != nil {
				log.Printf("[UnsendMessage] UpdateChatUnsendPreview error: %v", err)
				return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "server_error", "failed to update chat unsend preview")
			}
		} else {
			if err := qtx.UpdateChatUnsendPreviewBatch(ctx, personal_chat_store.UpdateChatUnsendPreviewBatchParams{
				ID:         targetChatID,
				MessageIds: newlyUnsentUUIDs,
			}); err != nil {
				log.Printf("[UnsendMessage] UpdateChatUnsendPreviewBatch error: %v", err)
				return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "server_error", "failed to update chat unsend preview")
			}
		}
		if err := qtx.UpdateChatUnsendDecrement(ctx, personal_chat_store.UpdateChatUnsendDecrementParams{
			ID:          targetChatID,
			RecipientID: recipientID,
			Amount:      int32(len(newlyUnsentIDs)),
		}); err != nil {
			log.Printf("[UnsendMessage] UpdateChatUnsendDecrement error: %v", err)
			return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "server_error", "failed to update chat unsend decrement")
		}
	}

	// Batch register files in pending_uploads and batch clear file fields on messages
	if len(fileRegistrationIDs) > 0 {
		if s.PendingUploads != nil {
			if err := s.PendingUploads.RegisterBatchTx(ctx, tx, fileRegistrationIDs, fileBuckets, fileObjectKeys, time.Now().UTC()); err != nil {
				log.Printf("[UnsendMessage] ERROR: Failed to register batch files in pending_uploads: %v", err)
				return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "server_error", "failed to register pending upload batch")
			}
		}
		if err := qtx.ClearMessagesFileFieldsBatch(ctx, msgIDsToClearFiles); err != nil {
			log.Printf("[UnsendMessage] ERROR: Failed to batch clear message file fields: %v", err)
			return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "server_error", "failed to clear file fields")
		}
	}

	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return unsendOutcome{}, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to commit unsend")
		}
	}

	// Post-commit: Asynchronously delete files from Cloudflare R2 and remove from pending_uploads
	if len(filesToCleanup) > 0 {
		go func(fIDs []string) {
			bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			r2Errors := kit.DeleteFilesBatch(bgCtx, fIDs, s.DeleteChatFile)
			for i, fID := range fIDs {
				if r2Errors[i] == nil {
					if s.PendingUploads != nil {
						if err := s.PendingUploads.Remove(context.Background(), fID); err != nil {
							log.Printf("[UnsendMessage] WARNING: Failed to remove file %s from pending_uploads: %v", fID, err)
						} else {
							log.Printf("[UnsendMessage] Inline R2 delete + pending_uploads removal succeeded for chat file %s", fID)
						}
					}
				} else {
					log.Printf("[UnsendMessage] WARNING: Async file %s R2 delete failed: %v (sweeper will retry)", fID, r2Errors[i])
				}
			}
		}(filesToCleanup)
	}

	return unsendOutcome{
		AcknowledgedIDs:        confirmedIDs,
		ChatID:                 targetChatID,
		RecipientID:            recipientID,
		NewlyUnsentIDs:         newlyUnsentIDs,
		PermanentlyRejectedIDs: permanentlyRejectedIDs(payload.MessageIDs, rejectedMap),
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Delete For Me
// ──────────────────────────────────────────────────────────────────────────────

func (s *chatService) DeleteMessageForMe(ctx context.Context, messageIDs []uuid.UUID, userID kit.UserId, isPrimary bool) ([]DeleteForMeGroup, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}

	uniqueIDs := make([]uuid.UUID, 0, len(messageIDs))
	seen := make(map[uuid.UUID]bool, len(messageIDs))
	for _, id := range messageIDs {
		if !seen[id] {
			seen[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}

	// 1. Batch fetch messages matching IDs
	existingRows, err := s.PostgresQueries.GetMessagesForDeleteForMeBatch(ctx, uniqueIDs)
	if err != nil {
		log.Printf("[DeleteMessageForMe] GetMessagesForDeleteForMeBatch error: %v", err)
		return nil, kit.NewError(http.StatusInternalServerError, "server_error", "failed to fetch messages for delete")
	}

	existingMap := make(map[uuid.UUID]personal_chat_store.GetMessagesForDeleteForMeBatchRow, len(existingRows))
	for _, row := range existingRows {
		existingMap[row.ID] = row
	}

	now := time.Now().UTC()

	// 2. Classify messages into categories:
	// - Category 1 (Newly Eligible): in DB, caller is participant, not expired, deleted_by_* == false -> DB updates + sync action (secondary) + SSE
	// - Category 2 (Already True): in DB, caller is participant, deleted_by_* == true -> strictly rejected
	// - Category 3 (Ineligible / Foreign): in DB, caller is NOT a participant -> strictly rejected
	// - Category 4 (Non-Existent / Purged): not in DB -> sync action (secondary only) + SSE (both primary & secondary)
	// - Category 5 (Expired in DB): in DB, caller is participant, expired -> 0 DB message writes, sync action (secondary only) + SSE
	var eligibleMsgIDs []uuid.UUID
	var senderMsgIDs []uuid.UUID
	var recipientMsgIDs []uuid.UUID
	var senderCandidatesToClean []personal_chat_store.Message
	var recipientCandidatesToClean []personal_chat_store.Message

	var expiredMsgIDs []uuid.UUID
	var nonExistentMsgIDs []uuid.UUID
	var nonExistentSSEIDs []string

	chatMap := make(map[uuid.UUID][]string)
	var chatOrder []uuid.UUID

	for _, id := range uniqueIDs {
		row, exists := existingMap[id]
		if !exists {
			// Category 4: Non-existent / Purged from relay
			nonExistentMsgIDs = append(nonExistentMsgIDs, id)
			nonExistentSSEIDs = append(nonExistentSSEIDs, id.String())
			continue
		}

		isSender := row.SenderID == userID.UuidUserId
		isRecipient := row.RecipientID == userID.UuidUserId

		if !isSender && !isRecipient {
			// Category 3: Ineligible / Foreign (caller is neither sender nor recipient) -> strictly reject / omit
			continue
		}

		msgStore := personal_chat_store.Message{
			ID:                          row.ID,
			ChatID:                      row.ChatID,
			SenderID:                    row.SenderID,
			RecipientID:                 row.RecipientID,
			FileID:                      row.FileID,
			DeliveredToRecipientPrimary: row.DeliveredToRecipientPrimary,
			SyncedToSenderPrimary:       row.SyncedToSenderPrimary,
			ReadByRecipient:             row.ReadByRecipient,
			ReadAckedBySender:           row.ReadAckedBySender,
		}

		if (isSender && row.DeletedBySender) || (isRecipient && row.DeletedByRecipient) {
			// Category 2: Already True -> skip DB updates, sync actions, and SSE.
			// BUT: participate in relay cleanup if delivery/sync criteria are satisfied!
			if isSender && row.DeliveredToRecipientPrimary {
				senderCandidatesToClean = append(senderCandidatesToClean, msgStore)
			} else if isRecipient && row.SyncedToSenderPrimary {
				recipientCandidatesToClean = append(recipientCandidatesToClean, msgStore)
			}
			continue
		}

		// Check expiry: for expired messages, do NOT perform DB write for delete-for-me (no deleted_by_*, no chat preview clear),
		// but STILL create a sync action if caller is secondary (!isPrimary), and emit SSE.
		isExpired := !row.ExpiresAt.IsZero() && now.After(row.ExpiresAt)
		if isExpired {
			expiredMsgIDs = append(expiredMsgIDs, row.ID)
			if _, ok := chatMap[row.ChatID]; !ok {
				chatMap[row.ChatID] = nil
				chatOrder = append(chatOrder, row.ChatID)
			}
			chatMap[row.ChatID] = append(chatMap[row.ChatID], row.ID.String())
			continue
		}

		if isSender {
			// Category 1: Newly Eligible (Sender)
			senderMsgIDs = append(senderMsgIDs, row.ID)
			eligibleMsgIDs = append(eligibleMsgIDs, row.ID)
			if _, ok := chatMap[row.ChatID]; !ok {
				chatMap[row.ChatID] = nil
				chatOrder = append(chatOrder, row.ChatID)
			}
			chatMap[row.ChatID] = append(chatMap[row.ChatID], row.ID.String())

			if row.DeliveredToRecipientPrimary {
				senderCandidatesToClean = append(senderCandidatesToClean, msgStore)
			}
		} else { // isRecipient
			// Category 1: Newly Eligible (Recipient)
			recipientMsgIDs = append(recipientMsgIDs, row.ID)
			eligibleMsgIDs = append(eligibleMsgIDs, row.ID)
			if _, ok := chatMap[row.ChatID]; !ok {
				chatMap[row.ChatID] = nil
				chatOrder = append(chatOrder, row.ChatID)
			}
			chatMap[row.ChatID] = append(chatMap[row.ChatID], row.ID.String())

			if row.SyncedToSenderPrimary {
				recipientCandidatesToClean = append(recipientCandidatesToClean, msgStore)
			}
		}
	}

	// 3. DB Transaction Phase:
	// Clear chat preview, mark soft deleted, and insert secondary sync actions atomically in one transaction.
	// If any step fails, the entire transaction rolls back, preserving database and sync consistency on retry.
	needsTx := len(eligibleMsgIDs) > 0 || (!isPrimary && (len(nonExistentMsgIDs) > 0 || len(expiredMsgIDs) > 0))
	qtx := s.PostgresQueries
	var tx pgx.Tx
	if needsTx && s.Pool != nil {
		var beginErr error
		tx, beginErr = s.Pool.Begin(ctx)
		if beginErr != nil {
			log.Printf("[DeleteMessageForMe] Begin tx error: %v", beginErr)
			return nil, kit.NewError(http.StatusInternalServerError, "server_error", "failed to start transaction")
		}
		defer tx.Rollback(ctx)
		qtx = s.PostgresQueries.WithTx(tx)
	}

	if len(eligibleMsgIDs) > 0 {
		_, err = qtx.ClearLastMessageForParticipantBatch(ctx, personal_chat_store.ClearLastMessageForParticipantBatchParams{
			UserID:     userID.UuidUserId,
			MessageIds: eligibleMsgIDs,
		})
		if err != nil {
			log.Printf("[DeleteMessageForMe] ClearLastMessageForParticipantBatch error: %v", err)
			return nil, kit.NewError(http.StatusInternalServerError, "server_error", "failed to clear last message preview")
		}
	}

	if len(senderMsgIDs) > 0 {
		_, err = qtx.MarkMessagesDeletedBySenderBatch(ctx, personal_chat_store.MarkMessagesDeletedBySenderBatchParams{
			MessageIds: senderMsgIDs,
			SenderID:   userID.UuidUserId,
		})
		if err != nil {
			log.Printf("[DeleteMessageForMe] MarkMessagesDeletedBySenderBatch error: %v", err)
			return nil, kit.NewError(http.StatusInternalServerError, "server_error", "failed to mark messages deleted by sender")
		}
	}

	if len(recipientMsgIDs) > 0 {
		_, err = qtx.MarkMessagesDeletedByRecipientBatch(ctx, personal_chat_store.MarkMessagesDeletedByRecipientBatchParams{
			MessageIds:  recipientMsgIDs,
			RecipientID: userID.UuidUserId,
		})
		if err != nil {
			log.Printf("[DeleteMessageForMe] MarkMessagesDeletedByRecipientBatch error: %v", err)
			return nil, kit.NewError(http.StatusInternalServerError, "server_error", "failed to mark messages deleted by recipient")
		}
	}

	// 4. If secondary session, insert sync actions in batch atomically within the same transaction:
	// - For eligible messages whose DB update succeeded
	// - For non-existent messages (purged from relay)
	// - For expired messages in DB
	// (Already-true and ineligible messages are strictly omitted)
	if !isPrimary {
		var syncActionMsgIDs []uuid.UUID
		syncActionMsgIDs = append(syncActionMsgIDs, senderMsgIDs...)
		syncActionMsgIDs = append(syncActionMsgIDs, recipientMsgIDs...)
		syncActionMsgIDs = append(syncActionMsgIDs, nonExistentMsgIDs...)
		syncActionMsgIDs = append(syncActionMsgIDs, expiredMsgIDs...)

		if len(syncActionMsgIDs) > 0 {
			_, err = qtx.CreateSyncActionsBatch(ctx, personal_chat_store.CreateSyncActionsBatchParams{
				UserID:     userID.UuidUserId,
				MessageIds: syncActionMsgIDs,
			})
			if err != nil {
				log.Printf("[DeleteMessageForMe] CreateSyncActionsBatch error: %v", err)
				return nil, kit.NewError(http.StatusInternalServerError, "server_error", "failed to create sync action")
			}
		}
	}

	// Commit transaction before relay cleanup or SSE broadcast
	if tx != nil {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			log.Printf("[DeleteMessageForMe] Commit tx error: %v", commitErr)
			return nil, kit.NewError(http.StatusInternalServerError, "server_error", "failed to commit transaction")
		}
	}

	// 5. Asynchronously clean up relay ONLY AFTER transaction committed successfully
	var messagesToClean []personal_chat_store.Message
	messagesToClean = append(messagesToClean, senderCandidatesToClean...)
	messagesToClean = append(messagesToClean, recipientCandidatesToClean...)

	if len(messagesToClean) > 0 {
		go func(toClean []personal_chat_store.Message) {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			for _, msg := range toClean {
				s.deleteMessageFromRelay(cleanupCtx, msg)
			}
		}(messagesToClean)
	}

	// 6. Group for smart SSE broadcast (only reached if transaction succeeded):
	// - Eligible messages grouped by ChatID
	// - Non-existent messages grouped with ChatID: uuid.Nil
	var groups []DeleteForMeGroup
	for _, cID := range chatOrder {
		var validIDsInChat []string
		for _, mIDStr := range chatMap[cID] {
			validIDsInChat = append(validIDsInChat, mIDStr)
		}
		if len(validIDsInChat) > 0 {
			groups = append(groups, DeleteForMeGroup{
				ChatID:     cID,
				MessageIDs: validIDsInChat,
			})
		}
	}

	if len(nonExistentSSEIDs) > 0 {
		groups = append(groups, DeleteForMeGroup{
			ChatID:     uuid.Nil,
			MessageIDs: nonExistentSSEIDs,
		})
	}

	return groups, nil
}

func (s *chatService) DeleteMessageForMeHandler(ctx context.Context, payload *DeleteMessageForMePayload, userID kit.UserId, isPrimary bool) ([]DeleteForMeGroup, error) {
	msgUUIDs := make([]uuid.UUID, 0, len(payload.MessageIDs))
	for _, idStr := range payload.MessageIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, kit.NewError(http.StatusBadRequest, "invalid_request", "Invalid message ID")
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

	// 5. Memoize sender key revisions across pending messages
	revisionMap := make(map[uuid.UUID]int32, 4)
	getRevision := func(senderID uuid.UUID) int32 {
		if rev, ok := revisionMap[senderID]; ok {
			return rev
		}
		rev := s.getSenderKeysRevision(ctx, senderID)
		revisionMap[senderID] = rev
		return rev
	}

	// 6. Build response payload
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
			ReplyToMessageId:            mr.ReplyToMessageID,
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

	// Never touch the relay row or the R2 object until both primary devices
	// hold the message. Callers also gate this, but the check lives here too so
	// a mistake in one caller cannot destroy the only remaining copy.
	if !message.DeliveredToRecipientPrimary || !message.SyncedToSenderPrimary {
		log.Printf("[Relay-Cleanup] Skipping message %s: primary delivery/sync incomplete", messageID)
		return
	}

	log.Printf("[Relay-Cleanup] Message %s fully delivered and synced. Deleting from server.", messageID)
	fileID := ""
	if message.FileID != nil {
		fileID = *message.FileID
	}

	if s.Pool == nil {
		log.Printf("[Relay-Cleanup] WARNING: Pool is nil, skipping relay cleanup for message %s", messageID)
		return
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		log.Printf("[Relay-Cleanup] ERROR: Failed to start transaction for message %s: %v", messageID, err)
		return
	}
	defer tx.Rollback(ctx)
	qtx := s.PostgresQueries.WithTx(tx)

	if fileID != "" {
		if s.R2Pool == nil {
			log.Printf("[Relay-Cleanup] WARNING: R2 pool is nil, skipping file registration for %s", fileID)
		} else {
			_, objectKey := clients.ParseFilePrefix(fileID)
			client := s.R2Pool.GetClient(fileID)
			bucket := client.ChatBucket()
			if err := s.PendingUploads.RegisterTx(ctx, tx, fileID, bucket, objectKey, time.Now().UTC()); err != nil {
				log.Printf("[Relay-Cleanup] ERROR: Failed to register file %s in pending_uploads: %v", fileID, err)
				return
			}
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

const (
	messageCleanupOuterBudget  = 55 * time.Minute
	messageCleanupJobBudget    = 25 * time.Minute
	messageCleanupJobGap       = 5 * time.Minute
	messageCleanupBatchSize    = 100
	messageCleanupBatchTimeout = 2 * time.Minute
)

// CleanupExpiredMessages cleans up blocked-user messages with files (Job 1) and expired messages with files (Job 2).
// It enforces an outer 55-minute budget for the 1-hour cycle, with:
//   - Job 1 (Blocked users): 25 minutes fixed budget.
//   - 5-minute gap / buffer window between Job 1 and Job 2.
//   - Job 2 (Expired messages): 25 minutes fixed budget + any unused time transferred from Job 1.
//   - Ongoing batch preservation: An in-flight R2/DB batch is not cancelled by the job budget,
//     but each batch has a 2-minute hard ceiling so a hung database or R2 call cannot park the
//     worker forever. Starting new batches is prohibited once the job budget is reached.
func (s *chatService) CleanupExpiredMessages(ctx context.Context) error {
	outerBudget := messageCleanupOuterBudget
	jobGap := messageCleanupJobGap
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d > 0 && d < outerBudget {
			outerBudget = d
			jobGap = (d * 5) / 55
		}
	}

	log.Printf("[CleanupJob] Starting cleanup of expired and orphaned messages/files (outer budget: %v, job gap: %v)", outerBudget, jobGap)
	overallStart := time.Now()

	job1Budget := messageCleanupJobBudget
	if outerBudget < messageCleanupOuterBudget {
		job1Budget = (outerBudget * 25) / 55
	}
	job2BaseBudget := messageCleanupJobBudget
	if outerBudget < messageCleanupOuterBudget {
		job2BaseBudget = (outerBudget * 25) / 55
	}

	// --------------------------------------------------------------------------
	// JOB 1: Messages with Files for Blocked Users
	// --------------------------------------------------------------------------
	log.Printf("[CleanupJob] Starting Job 1: Blocked-user messages with files (budget: %v)", job1Budget)
	job1Start := time.Now()
	lastBlockedID := uuid.Nil
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if time.Since(job1Start) >= job1Budget {
			log.Printf("[CleanupJob] Job 1 budget reached (%v elapsed >= %v budget), stopping before new batch", time.Since(job1Start), job1Budget)
			break
		}

		batchCtx, batchCancel := context.WithTimeout(context.WithoutCancel(ctx), messageCleanupBatchTimeout)
		blockedMessages, err := s.MaintenanceQueries.GetMessagesWithFilesForBlockedUsers(batchCtx, personal_chat_store.GetMessagesWithFilesForBlockedUsersParams{
			Limit:  int32(messageCleanupBatchSize),
			LastID: lastBlockedID,
		})
		if err != nil {
			batchCancel()
			log.Printf("[CleanupJob] ERROR: Failed to fetch blocked-user messages with files: %v", err)
			break
		}
		if len(blockedMessages) == 0 {
			batchCancel()
			break
		}

		// Concurrent R2 deletes — in-flight batch is NOT cancelled on budget hit
		fileIDs := make([]string, 0, len(blockedMessages))
		msgByFileID := make(map[string]uuid.UUID, len(blockedMessages))
		for _, msg := range blockedMessages {
			if msg.FileID != nil && *msg.FileID != "" {
				fileIDs = append(fileIDs, *msg.FileID)
				msgByFileID[*msg.FileID] = msg.ID
			}
		}
		r2Errors := kit.DeleteFilesBatch(batchCtx, fileIDs, s.DeleteChatFile)
		for i, fileID := range fileIDs {
			if r2Errors[i] != nil {
				log.Printf("[CleanupJob] WARNING: Chat file %s R2 delete failed: %v", fileID, r2Errors[i])
				continue
			}
			if err := s.MaintenanceQueries.DeleteMessage(batchCtx, msgByFileID[fileID]); err != nil {
				log.Printf("[CleanupJob] WARNING: Failed to delete blocked-user message record %s: %v", msgByFileID[fileID], err)
			}
		}
		// The batch is complete; release its deadline before deciding whether
		// to start another batch.
		batchCancel()

		if len(blockedMessages) < messageCleanupBatchSize {
			break
		}
		lastBlockedID = blockedMessages[len(blockedMessages)-1].ID

		// Prohibit triggering new batch if Job 1 budget reached after batch completion
		if time.Since(job1Start) >= job1Budget {
			log.Printf("[CleanupJob] Job 1 budget reached after batch completion (%v elapsed >= %v budget), stopping", time.Since(job1Start), job1Budget)
			break
		}
	}

	job1Elapsed := time.Since(job1Start)
	unusedJob1Budget := time.Duration(0)
	if job1Elapsed < job1Budget {
		unusedJob1Budget = job1Budget - job1Elapsed
		log.Printf("[CleanupJob] Job 1 finished early in %v; unused budget %v transferred to Job 2", job1Elapsed, unusedJob1Budget)
	} else {
		log.Printf("[CleanupJob] Job 1 completed in %v", job1Elapsed)
	}

	// --------------------------------------------------------------------------
	// 5-MINUTE GAP / SAFETY BUFFER WINDOW BETWEEN JOB 1 AND JOB 2
	// --------------------------------------------------------------------------
	gapDuration := jobGap
	if job1Elapsed > job1Budget {
		spillover := job1Elapsed - job1Budget
		if spillover < gapDuration {
			gapDuration -= spillover
		} else {
			gapDuration = 0
		}
	}

	if gapDuration >= 1*time.Millisecond {
		log.Printf("[CleanupJob] Waiting gap of %v before starting Job 2...", gapDuration)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(gapDuration):
		}
	}

	// --------------------------------------------------------------------------
	// JOB 2: Expired Messages with Files
	// --------------------------------------------------------------------------
	// Job 2 gets 25 minutes + remaining time from Job 1's budget (bounded by outerBudget)
	job2Budget := job2BaseBudget + unusedJob1Budget
	remainingOuterBudget := outerBudget - time.Since(overallStart)
	if job2Budget > remainingOuterBudget {
		job2Budget = remainingOuterBudget
	}
	if job2Budget <= 0 {
		log.Printf("[CleanupJob] Outer budget depleted before Job 2 (%v elapsed >= %v outer budget), skipping Job 2", time.Since(overallStart), outerBudget)
		return nil
	}

	log.Printf("[CleanupJob] Starting Job 2: Expired messages with files (budget: %v)", job2Budget)
	job2Start := time.Now()
	lastExpiredID := uuid.Nil
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if time.Since(job2Start) >= job2Budget {
			log.Printf("[CleanupJob] Job 2 budget reached (%v elapsed >= %v budget), stopping before new batch", time.Since(job2Start), job2Budget)
			break
		}

		batchCtx, batchCancel := context.WithTimeout(context.WithoutCancel(ctx), messageCleanupBatchTimeout)
		expiredMessages, err := s.MaintenanceQueries.GetExpiredMessagesWithFiles(batchCtx, personal_chat_store.GetExpiredMessagesWithFilesParams{
			Limit:  int32(messageCleanupBatchSize),
			LastID: lastExpiredID,
		})
		if err != nil {
			batchCancel()
			log.Printf("[CleanupJob] ERROR: Failed to fetch expired messages with files: %v", err)
			break
		}
		if len(expiredMessages) == 0 {
			batchCancel()
			break
		}

		// Concurrent R2 deletes — 50-way parallelism (kit.MaxConcurrentDeletes).
		// In-flight batch runs to completion without being cancelled on budget hit.
		fileIDs := make([]string, 0, len(expiredMessages))
		msgByFileID := make(map[string]personal_chat_store.Message, len(expiredMessages))
		for _, msg := range expiredMessages {
			if msg.FileID != nil && *msg.FileID != "" {
				fileIDs = append(fileIDs, *msg.FileID)
				msgByFileID[*msg.FileID] = msg
			}
		}
		r2Errors := kit.DeleteFilesBatch(batchCtx, fileIDs, s.DeleteChatFile)
		expiredIDsToDelete := make([]uuid.UUID, 0, len(fileIDs))
		for i, fileID := range fileIDs {
			if r2Errors[i] != nil {
				// R2 failed (real error) — skip DB delete, retry next cycle
				log.Printf("[CleanupJob] WARNING: Chat file %s R2 delete failed: %v", fileID, r2Errors[i])
				continue
			}
			// R2 succeeded (or file was already gone)
			msg := msgByFileID[fileID]
			if !msg.ExpiresAt.After(time.Now().UTC()) {
				// TTL supersedes relay delivery. The row is no longer deliverable; remove it entirely.
				expiredIDsToDelete = append(expiredIDsToDelete, msg.ID)
				continue
			}
			if err := s.MaintenanceQueries.StripDeliveredMessagePayload(batchCtx, msg.ID); err != nil {
				log.Printf("[CleanupJob] WARNING: Failed to strip delivered message payload %s: %v", msg.ID, err)
			}
		}
		if len(expiredIDsToDelete) > 0 {
			if _, err := s.MaintenanceQueries.DeleteExpiredMessagesWithFilesBatch(batchCtx, expiredIDsToDelete); err != nil {
				log.Printf("[CleanupJob] WARNING: Failed to delete expired message rows: %v", err)
			}
		}
		// The batch is complete; release its deadline before deciding whether
		// to start another batch.
		batchCancel()

		if len(expiredMessages) < messageCleanupBatchSize {
			break
		}
		lastExpiredID = expiredMessages[len(expiredMessages)-1].ID

		// Prohibit triggering new batch if Job 2 budget reached after batch completion
		if time.Since(job2Start) >= job2Budget {
			log.Printf("[CleanupJob] Job 2 budget reached after batch completion (%v elapsed >= %v budget), stopping", time.Since(job2Start), job2Budget)
			break
		}
	}

	log.Printf("[CleanupJob] Cleanup process completed successfully (total elapsed: %v)", time.Since(overallStart))
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

	// Ordered list of database cleanup tasks to run sequentially on the maintenance pool.
	jobs := []struct {
		deleteFn func(context.Context, int32) (int64, error)
		label    string
	}{
		{s.MaintenanceQueries.DeleteExpiredMessagesWithoutFilesBatch, "expired no-file messages"},
		{s.MaintenanceQueries.DeleteFullyAcknowledgedMessagesWithoutFilesBatch, "fully-acknowledged no-file messages"},
		{s.MaintenanceQueries.DeleteBlockedUserMessagesWithoutFilesBatch, "blocked-user no-file messages"},
		{s.MaintenanceQueries.CleanupSyncActionsForBlockedUsersBatch, "blocked-user sync actions"},
		{s.MaintenanceQueries.DeleteOldSyncActionsBatch, "old sync actions"},
		{s.MaintenanceQueries.DeleteExpiredHistorySyncBatch, "expired history sync"},
	}

	for i := range jobs {
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

// LockUserChatsTx satisfies core_auth.personalChatAuthCleanupProvider.
// Acquires exclusive row-level locks on all chats involving the user to prevent concurrent message writes during account deletion.
func (s *chatService) LockUserChatsTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	qtx := s.PostgresQueries.WithTx(tx)
	_, err := qtx.LockUserChatsForUpdate(ctx, userID)
	return err
}

// RegisterChatCleanupFilesTx satisfies core_auth.personalChatAuthCleanupProvider.
// Single round-trip UNION ALL scan per page: one index-backed branch per
// side (sent rides idx_messages_files_sender_id, received rides
// idx_messages_files_recipient_id — never OR, which could not use an
// index and scanned while holding chat row locks). The side column drives
// independent keyset cursors; rows merge in memory (dedup by file_id).
// Rows are born-expired in pending_uploads (expires_at = now()) so the
// background sweeper retries them if the async R2 goroutine dies.
func (s *chatService) RegisterChatCleanupFilesTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID) ([]string, error) {
	qtx := s.PostgresQueries.WithTx(tx)
	const batchSize = 100
	seen := make(map[string]struct{})
	var fileIDs []string
	lastSentID := uuid.Nil
	lastReceivedID := uuid.Nil

	for {
		batch, err := qtx.GetMessagesWithFilesForUserUnion(ctx, personal_chat_store.GetMessagesWithFilesForUserUnionParams{
			UserID:         userID,
			LastSentID:     lastSentID,
			LastReceivedID: lastReceivedID,
			Limit:          int32(batchSize),
		})
		if err != nil {
			log.Printf("[Chat RegisterChatCleanupFilesTx] ERROR: Failed to fetch messages with files: %v", err)
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		var batchFileIDs []string
		var batchBuckets []string
		var batchKeys []string

		for i := range batch {
			row := &batch[i]
			// Advance the cursor of whichever side produced this row.
			if row.Side == "sent" {
				lastSentID = row.ID
			} else {
				lastReceivedID = row.ID
			}
			if row.FileID == nil || *row.FileID == "" {
				continue
			}
			fileID := *row.FileID
			if _, exists := seen[fileID]; exists {
				continue
			}
			seen[fileID] = struct{}{}

			_, objectKey := clients.ParseFilePrefix(fileID)
			client := s.R2Pool.GetClient(fileID)
			bucket := client.ChatBucket()

			batchFileIDs = append(batchFileIDs, fileID)
			batchBuckets = append(batchBuckets, bucket)
			batchKeys = append(batchKeys, objectKey)
			fileIDs = append(fileIDs, fileID)
		}

		if len(batchFileIDs) > 0 {
			if err := s.PendingUploads.RegisterBatchTx(ctx, tx, batchFileIDs, batchBuckets, batchKeys, time.Now().UTC()); err != nil {
				log.Printf("[Chat RegisterChatCleanupFilesTx] ERROR: Failed to batch register %d files in pending_uploads: %v", len(batchFileIDs), err)
				return nil, err
			}
		}

		// Each branch returns up to batchSize rows: a side returning
		// fewer than batchSize has no more pages; stop when both are done.
		sentCount, receivedCount := 0, 0
		for i := range batch {
			if batch[i].Side == "sent" {
				sentCount++
			} else {
				receivedCount++
			}
		}
		if sentCount < batchSize && receivedCount < batchSize {
			break
		}
	}

	return fileIDs, nil
}

// DeleteChatR2FilesAsync satisfies core_auth.personalChatAuthCleanupProvider.
// Performs asynchronous Cloudflare R2 deletions for chat files and removes entries from pending_uploads on success.
func (s *chatService) DeleteChatR2FilesAsync(ctx context.Context, chatFileIDs []string) {
	if len(chatFileIDs) == 0 {
		return
	}
	r2Errors := kit.DeleteFilesBatch(ctx, chatFileIDs, s.DeleteChatFile)
	for i, fileID := range chatFileIDs {
		if r2Errors[i] == nil {
			if remErr := s.PendingUploads.Remove(ctx, fileID); remErr != nil {
				log.Printf("[Chat DeleteChatR2FilesAsync] WARNING: Failed to remove file %s from pending_uploads: %v", fileID, remErr)
			}
		} else {
			log.Printf("[Chat DeleteChatR2FilesAsync] WARNING: Async file %s R2 delete failed: %v (sweeper will retry)", fileID, r2Errors[i])
		}
	}
}

func uuidPtrToStringPtr(u *uuid.UUID) *string {
	if u == nil {
		return nil
	}
	s := u.String()
	return &s
}

