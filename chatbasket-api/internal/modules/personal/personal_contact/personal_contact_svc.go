package personal_contact

import (
	"chatbasket-api/internal/modules/personal/personal_contact/internal/personal_contact_store"
	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/services"
	"context"
	"net/http"
	"strings"

	rpc_common_modelv1 "chatbasket-api/gen/proto/common/model"
	rpc_personal_contactv1 "chatbasket-api/gen/proto/personal/personal_contact"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// personalProfilePersonalContactProvider defines the minimal set of methods required from the Profile module.
type personalProfilePersonalContactProvider interface {
	IsUserAdminBlocked(ctx context.Context, userID uuid.UUID) (bool, error)
	GetUserCoreProfile(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error)
	GetContactableProfilesForViewer(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error)
	FindContactableUserByUsername(ctx context.Context, viewerID uuid.UUID, username string) (*personal_profile.ContactLookupResult, error)
	CreateUserBlock(ctx context.Context, id, blockerID, blockedID uuid.UUID) error
	IsEitherBlocked(ctx context.Context, user1ID, user2ID uuid.UUID) (int32, error)
	IsBlockedBetweenUsers(ctx context.Context, requesterID, targetID uuid.UUID) (*personal_profile.BlockStatusResult, error)
	IsBlockedBetweenUsersBatch(ctx context.Context, requesterID uuid.UUID, targetIDs []uuid.UUID) ([]*personal_profile.BlockStatusResult, error)
}

// ChatCleanupProvider defines the decoupled method signature for async chat message/file cleanup on user block
type ChatCleanupProvider interface {
	CleanupChatMessagesForBlockAsync(ctx context.Context, blockerID, blockedID uuid.UUID)
}

type contactService struct {
	GlobalService                          *services.GlobalService
	PostgresQuerier                        personal_contact_store.Querier
	PostgresQueries                        *personal_contact_store.Queries
	personalProfilePersonalContactProvider personalProfilePersonalContactProvider
	chatCleanupProvider                    ChatCleanupProvider
	PersonalUsernameKey                    []byte
	PersonalContactKey                     []byte
}

func NewContactService(globalService *services.GlobalService, pool *pgxpool.Pool, personalProfilePersonalContactProvider personalProfilePersonalContactProvider, personalUsernameKey []byte, personalContactKey []byte) *contactService {
	store := personal_contact_store.New(pool)
	return &contactService{
		GlobalService:                          globalService,
		PostgresQuerier:                        store,
		PostgresQueries:                        store,
		personalProfilePersonalContactProvider: personalProfilePersonalContactProvider,
		PersonalUsernameKey:                    personalUsernameKey,
		PersonalContactKey:                     personalContactKey,
	}
}

func (ps *contactService) RegisterChatCleanupProvider(provider ChatCleanupProvider) {
	ps.chatCleanupProvider = provider
}

func (ps *contactService) checkBlockStatus(ctx context.Context, requesterID, targetID uuid.UUID) error {
	status, err := ps.personalProfilePersonalContactProvider.IsBlockedBetweenUsers(ctx, requesterID, targetID)
	if err != nil {
		return kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	if !status.IsBlocked {
		return nil
	}
	return kit.NewErrorWithDetails(http.StatusForbidden, "forbidden", "blocked", &rpc_common_modelv1.BlockStatusFlags{
		IsRequesterAdminBlocked:        status.IsRequesterAdminBlocked,
		IsTargetAdminBlocked:           status.IsTargetAdminBlocked,
		IsRequesterUserBlockedByTarget: status.IsRequesterUserBlockedByTarget,
		IsTargetUserBlockedByRequester: status.IsTargetUserBlockedByRequester,
		IsTargetProfilePrivate:         status.IsTargetProfilePrivate,
	})
}

func (ps *contactService) GetContacts(ctx context.Context, userId kit.UserId) (*rpc_personal_contactv1.GetContactsResponse, error) {
	// Fetch slim contact data
	myContacts, err := ps.PostgresQueries.GetUserContactsLite(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	addedMe, err := ps.PostgresQueries.GetUsersWhoAddedYouLite(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	if len(myContacts) == 0 && len(addedMe) == 0 {
		return &rpc_personal_contactv1.GetContactsResponse{
			Contacts:          []*rpc_personal_contactv1.Contact{},
			PeopleWhoAddedYou: []*rpc_personal_contactv1.Contact{},
		}, nil
	}

	// Collect unique target IDs
	targetIDs := make([]uuid.UUID, 0, len(myContacts)+len(addedMe))
	seen := make(map[uuid.UUID]struct{})

	for _, c := range myContacts {
		if _, exists := seen[c.ID]; !exists {
			targetIDs = append(targetIDs, c.ID)
			seen[c.ID] = struct{}{}
		}
	}
	for _, p := range addedMe {
		if _, exists := seen[p.ID]; !exists {
			targetIDs = append(targetIDs, p.ID)
			seen[p.ID] = struct{}{}
		}
	}

	// Batch fetch enriched profiles
	profilesByID, err := ps.personalProfilePersonalContactProvider.GetContactableProfilesForViewer(
		ctx,
		userId.UuidUserId,
		targetIDs,
	)
	if err != nil {
		return nil, err
	}

	// Build mutual lookup
	addedMeMap := make(map[string]struct{}, len(addedMe))
	for _, u := range addedMe {
		addedMeMap[u.ID.String()] = struct{}{}
	}

	myContactsMap := make(map[string]struct{}, len(myContacts))
	myNicknameByID := make(map[string]*string, len(myContacts))
	for _, c := range myContacts {
		id := c.ID.String()
		myContactsMap[id] = struct{}{}
		myNicknameByID[id] = c.Nickname
	}

	// Build contacts list
	contacts := make([]*rpc_personal_contactv1.Contact, 0, len(myContacts))
	for _, c := range myContacts {
		profile, ok := profilesByID[c.ID]
		if !ok {
			continue // Skip if profile not found
		}

		_, isMutual := addedMeMap[c.ID.String()]

		var nickname *string
		if c.Nickname != nil {
			decrypted, err := ps.DecryptNickname(c.Nickname, userId.UuidUserId, c.ID)
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to decrypt contact nickname")
			}
			nickname = decrypted
		}

		contacts = append(contacts, &rpc_personal_contactv1.Contact{
			Id:           c.ID.String(),
			Name:         profile.Name,
			Username:     profile.Username,
			Bio:          profile.Bio,
			Nickname:     nickname,
			CreatedAt:    timestamppb.New(c.ContactCreatedAt),
			UpdatedAt:    timestamppb.New(c.ContactUpdatedAt),
			AvatarUrl:    profile.AvatarURL,
			AvatarFileId: profile.AvatarFileId,
			IsMutual:     isMutual,
			ProfileType:  profile.ProfileType,
		})
	}

	// Build people who added you list
	peopleWhoAddedYou := make([]*rpc_personal_contactv1.Contact, 0, len(addedMe))
	for _, p := range addedMe {
		profile, ok := profilesByID[p.ID]
		if !ok {
			continue // Skip if profile not found
		}

		_, isMutual := myContactsMap[p.ID.String()]

		var myNickname *string
		if n, ok := myNicknameByID[p.ID.String()]; ok && n != nil {
			decrypted, err := ps.DecryptNickname(n, userId.UuidUserId, p.ID)
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to decrypt contact nickname")
			}
			myNickname = decrypted
		}

		peopleWhoAddedYou = append(peopleWhoAddedYou, &rpc_personal_contactv1.Contact{
			Id:           p.ID.String(),
			Name:         profile.Name,
			Username:     profile.Username,
			Bio:          profile.Bio,
			Nickname:     myNickname,
			CreatedAt:    timestamppb.New(p.ContactCreatedAt),
			UpdatedAt:    timestamppb.New(p.ContactUpdatedAt),
			AvatarUrl:    profile.AvatarURL,
			AvatarFileId: profile.AvatarFileId,
			IsMutual:     isMutual,
			ProfileType:  profile.ProfileType,
		})
	}

	return &rpc_personal_contactv1.GetContactsResponse{
		Contacts:          contacts,
		PeopleWhoAddedYou: peopleWhoAddedYou,
	}, nil
}

func (ps *contactService) CheckContactExistance(ctx context.Context, payload *CheckContactExistancePayload, userId kit.UserId) (*rpc_personal_contactv1.CheckContactExistanceResponse, error) {
	// Use profile module for username lookup
	user, err := ps.personalProfilePersonalContactProvider.FindContactableUserByUsername(
		ctx,
		userId.UuidUserId,
		payload.ContactUsername,
	)
	if err != nil {
		return nil, err
	}

	if !user.Exists {
		return &rpc_personal_contactv1.CheckContactExistanceResponse{Exists: false}, nil
	}

	if err := ps.checkBlockStatus(ctx, userId.UuidUserId, user.ID); err != nil {
		return nil, err
	}

	existsResp := &rpc_personal_contactv1.CheckContactExistanceResponse{
		Exists:      true,
		Name:        user.Name,
		ProfileType: user.ProfileType,
	}

	// Only set RecipientUserId if profile is not private
	if user.ProfileType != "private" {
		recipientId := user.ID.String()
		existsResp.RecipientUserId = &recipientId
	}

	return existsResp, nil
}

func (ps *contactService) CreateContact(ctx context.Context, payload *CreateContactPayload, userId kit.UserId) (*rpc_personal_contactv1.CreateContactResponse, error) {
	if payload == nil || payload.ContactUserId == "" {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	// Validate payload and parse target UUID
	targetUUID, err := uuid.Parse(payload.ContactUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid contactUserId")
	}
	// Prevent self-addition
	if targetUUID == userId.UuidUserId {
		return nil, kit.NewError(http.StatusConflict, "conflict", "self_addition")
	}

	// DB call to check if user is admin-blocked
	isMeAdminBlocked, err := ps.personalProfilePersonalContactProvider.IsUserAdminBlocked(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	if isMeAdminBlocked {
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "self_admin_blocked")
	}

	// DB call to get target user's core profile
	targetProfile, err := ps.personalProfilePersonalContactProvider.GetUserCoreProfile(ctx, targetUUID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusNotFound, "not_found", "user_not_found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	// Check if target is admin-blocked
	if targetProfile.IsAdminBlocked {
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "user_admin_blocked")
	}

	// DB call to check if users are mutually blocked
	blockStatus, err := ps.personalProfilePersonalContactProvider.IsEitherBlocked(ctx, userId.UuidUserId, targetUUID)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	switch blockStatus {
	case 1:
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "you_blocked_user")
	case 2:
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "user_blocked_you")
	case 0:
		// No block, continue
	default:
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "unexpected block status")
	}

	// DB call to check if already a contact
	alreadyContact, err := ps.PostgresQueries.IsAlreadyContact(ctx, personal_contact_store.IsAlreadyContactParams{
		OwnerUserID:   userId.UuidUserId,
		ContactUserID: targetUUID,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	if alreadyContact {
		contact, err := ps.buildSingleContactForOwner(ctx, userId.UuidUserId, targetUUID)
		if err != nil {
			return nil, err
		}
		return &rpc_personal_contactv1.CreateContactResponse{Status: true, Message: "already_in_contacts", Contact: contact}, nil
	}

	// Normalize optional nickname
	var nickname *string
	if payload.Nickname != nil {
		trimmed := strings.TrimSpace(*payload.Nickname)
		if trimmed != "" {
			if len([]rune(trimmed)) > 40 {
				return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid_nickname_length")
			}
			nickname = new(trimmed)
		}
	}

	// Handle based on target profile type
	switch targetProfile.ProfileType {
	case "private":
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "user_private_profile")
	case "public":
		// Encrypt nickname if provided
		var encryptedNickname *string
		if nickname != nil {
			encrypted, err := ps.EncryptNickname(*nickname, userId.UuidUserId, targetUUID)
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to encrypt nickname")
			}
			encryptedNickname = &encrypted
		}

		// DB call to add contact
		err = ps.PostgresQueries.InsertUserContact(ctx, personal_contact_store.InsertUserContactParams{
			OwnerUserID:   userId.UuidUserId,
			ContactUserID: targetUUID,
			Nickname:      encryptedNickname,
		})
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
		}
		contact, err := ps.buildSingleContactForOwner(ctx, userId.UuidUserId, targetUUID)
		if err != nil {
			return nil, err
		}
		return &rpc_personal_contactv1.CreateContactResponse{Status: true, Message: "public_contact_added", Contact: contact}, nil
	case "personal":
		targetAlreadyHasMe, err := ps.PostgresQueries.IsAlreadyContact(ctx, personal_contact_store.IsAlreadyContactParams{
			OwnerUserID:   targetUUID,
			ContactUserID: userId.UuidUserId,
		})
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
		}
		if targetAlreadyHasMe {
			// Encrypt nickname if provided
			var encryptedNickname *string
			if nickname != nil {
				encrypted, err := ps.EncryptNickname(*nickname, userId.UuidUserId, targetUUID)
				if err != nil {
					return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to encrypt nickname")
				}
				encryptedNickname = &encrypted
			}

			err = ps.PostgresQueries.InsertUserContact(ctx, personal_contact_store.InsertUserContactParams{
				OwnerUserID:   userId.UuidUserId,
				ContactUserID: targetUUID,
				Nickname:      encryptedNickname,
			})
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
			}
			contact, err := ps.buildSingleContactForOwner(ctx, userId.UuidUserId, targetUUID)
			if err != nil {
				return nil, err
			}
			return &rpc_personal_contactv1.CreateContactResponse{Status: true, Message: "personal_contact_added", Contact: contact}, nil
		}

		// DB call to check for existing request status
		requestStatus, err := ps.PostgresQueries.GetContactRequestStatus(ctx, personal_contact_store.GetContactRequestStatusParams{
			RequesterUserID: userId.UuidUserId,
			ReceiverUserID:  targetUUID,
		})
		if err != nil && err != pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
		}

		// Generate new request ID
		reqID, err := uuid.NewV7()
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to generate request ID")
		}

		// If request exists, check its status
		if err != pgx.ErrNoRows && requestStatus != "" {
			if requestStatus == "pending" {
				return &rpc_personal_contactv1.CreateContactResponse{Status: true, Message: "pending_request_exists"}, nil
			}

			// Encrypt nickname if provided
			var encryptedNickname *string
			if nickname != nil {
				encrypted, err := ps.EncryptNickname(*nickname, userId.UuidUserId, targetUUID)
				if err != nil {
					return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to encrypt nickname")
				}
				encryptedNickname = &encrypted
			}

			// If accepted or declined, delete old request and insert new one
			err = ps.PostgresQueries.DeleteAndInsertContactRequest(ctx, personal_contact_store.DeleteAndInsertContactRequestParams{
				ID:              reqID,
				RequesterUserID: userId.UuidUserId,
				ReceiverUserID:  targetUUID,
				Nickname:        encryptedNickname,
			})
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
			}
			return &rpc_personal_contactv1.CreateContactResponse{Status: true, Message: "contact_request_sent"}, nil
		}

		// Encrypt nickname if provided
		var encryptedNickname *string
		if nickname != nil {
			encrypted, err := ps.EncryptNickname(*nickname, userId.UuidUserId, targetUUID)
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to encrypt nickname")
			}
			encryptedNickname = &encrypted
		}

		// No existing request, insert new one
		err = ps.PostgresQueries.InsertContactRequest(ctx, personal_contact_store.InsertContactRequestParams{
			ID:              reqID,
			RequesterUserID: userId.UuidUserId,
			ReceiverUserID:  targetUUID,
			Nickname:        encryptedNickname,
		})
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
		}
		return &rpc_personal_contactv1.CreateContactResponse{Status: true, Message: "contact_request_sent"}, nil
	default:
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid target profile type")
	}
}

func (ps *contactService) AcceptContactRequest(ctx context.Context, payload *AcceptContactRequestPayload, userId kit.UserId) (*rpc_common_modelv1.StatusOkay, error) {
	if payload == nil || payload.ContactUserId == "" {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	requesterUUID, err := uuid.Parse(payload.ContactUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid contactUserId")
	}

	if requesterUUID == userId.UuidUserId {
		return nil, kit.NewError(http.StatusConflict, "conflict", "self_action_not_allowed")
	}

	if err := ps.checkBlockStatus(ctx, userId.UuidUserId, requesterUUID); err != nil {
		return nil, err
	}

	result, err := ps.PostgresQueries.AcceptContactRequest(ctx, personal_contact_store.AcceptContactRequestParams{
		RequesterUserID: requesterUUID,
		ReceiverUserID:  userId.UuidUserId,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	switch result {
	case "accepted":
		return &rpc_common_modelv1.StatusOkay{Status: true, Message: "contact_request_accepted"}, nil
	case "not_found":
		return nil, kit.NewError(http.StatusNotFound, "not_found", "pending_request_not_found")
	case "processed":
		return nil, kit.NewError(http.StatusConflict, "conflict", "request_already_processed")
	default:
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "unexpected outcome")
	}
}

func (ps *contactService) RejectContactRequest(ctx context.Context, payload *RejectContactRequestPayload, userId kit.UserId) (*rpc_common_modelv1.StatusOkay, error) {
	if payload == nil || payload.ContactUserId == "" {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	requesterUUID, err := uuid.Parse(payload.ContactUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid contactUserId")
	}

	if requesterUUID == userId.UuidUserId {
		return nil, kit.NewError(http.StatusConflict, "conflict", "self_action_not_allowed")
	}

	if err := ps.checkBlockStatus(ctx, userId.UuidUserId, requesterUUID); err != nil {
		return nil, err
	}

	result, err := ps.PostgresQueries.RejectContactRequest(ctx, personal_contact_store.RejectContactRequestParams{
		RequesterUserID: requesterUUID,
		ReceiverUserID:  userId.UuidUserId,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	switch result {
	case "declined":
		return &rpc_common_modelv1.StatusOkay{Status: true, Message: "contact_request_declined"}, nil
	case "not_found":
		return nil, kit.NewError(http.StatusNotFound, "not_found", "pending_request_not_found")
	case "processed":
		return nil, kit.NewError(http.StatusConflict, "conflict", "request_already_processed")
	default:
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "unexpected outcome")
	}
}

func (ps *contactService) DeleteContact(ctx context.Context, payload *DeleteContactPayload, userId kit.UserId) (*rpc_common_modelv1.StatusOkay, error) {
	if payload == nil || len(payload.ContactUserId) == 0 {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	uniqIDs := make([]uuid.UUID, 0, len(payload.ContactUserId))
	seen := make(map[uuid.UUID]struct{}, len(payload.ContactUserId))

	for _, raw := range payload.ContactUserId {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid contactUserId")
		}

		contactUUID, err := uuid.Parse(trimmed)
		if err != nil {
			return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid contactUserId")
		}

		if contactUUID == userId.UuidUserId {
			return nil, kit.NewError(http.StatusConflict, "conflict", "self_action_not_allowed")
		}

		if _, exists := seen[contactUUID]; exists {
			continue
		}
		seen[contactUUID] = struct{}{}
		uniqIDs = append(uniqIDs, contactUUID)
	}

	if len(uniqIDs) == 0 {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	statuses, err := ps.personalProfilePersonalContactProvider.IsBlockedBetweenUsersBatch(ctx, userId.UuidUserId, uniqIDs)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	blockedIDs := make(map[uuid.UUID]struct{}, len(uniqIDs))
	for _, s := range statuses {
		if s.IsRequesterAdminBlocked {
			return nil, kit.NewError(http.StatusForbidden, "forbidden", "self_admin_blocked")
		}
		if s.IsBlocked {
			blockedIDs[s.TargetID] = struct{}{}
		}
	}

	if len(blockedIDs) == len(uniqIDs) {
		if len(uniqIDs) == 1 {
			var singleStatus *rpc_common_modelv1.BlockStatusFlags
			for _, s := range statuses {
				if s.TargetID == uniqIDs[0] {
					singleStatus = &rpc_common_modelv1.BlockStatusFlags{
						IsRequesterAdminBlocked:        s.IsRequesterAdminBlocked,
						IsTargetAdminBlocked:           s.IsTargetAdminBlocked,
						IsRequesterUserBlockedByTarget: s.IsRequesterUserBlockedByTarget,
						IsTargetUserBlockedByRequester: s.IsTargetUserBlockedByRequester,
						IsTargetProfilePrivate:         s.IsTargetProfilePrivate,
					}
					break
				}
			}
			return nil, kit.NewErrorWithDetails(http.StatusForbidden, "forbidden", "blocked", singleStatus)
		}
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "all_contacts_blocked")
	}

	allowedIDs := make([]uuid.UUID, 0, len(uniqIDs)-len(blockedIDs))
	for _, id := range uniqIDs {
		if _, blocked := blockedIDs[id]; !blocked {
			allowedIDs = append(allowedIDs, id)
		}
	}

	removed, err := ps.PostgresQueries.DeleteContact(ctx, personal_contact_store.DeleteContactParams{
		OwnerUserID:    userId.UuidUserId,
		ContactUserIds: allowedIDs,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	if removed == 0 {
		return nil, kit.NewError(http.StatusNotFound, "not_found", "contact_not_found")
	}

	message := "contacts_deleted"
	if removed == 1 {
		message = "contact_deleted"
	} else if removed < int64(len(uniqIDs)) {
		message = "contacts_deleted_partial"
	}

	return &rpc_common_modelv1.StatusOkay{Status: true, Message: message}, nil
}

func (ps *contactService) UndoContactRequest(ctx context.Context, payload *UndoContactRequestPayload, userId kit.UserId) (*rpc_common_modelv1.StatusOkay, error) {
	if payload == nil || payload.ContactUserId == "" {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	receiverUUID, err := uuid.Parse(payload.ContactUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid contactUserId")
	}

	if receiverUUID == userId.UuidUserId {
		return nil, kit.NewError(http.StatusConflict, "conflict", "self_action_not_allowed")
	}

	if err := ps.checkBlockStatus(ctx, userId.UuidUserId, receiverUUID); err != nil {
		return nil, err
	}

	result, err := ps.PostgresQueries.UndoContactRequest(ctx, personal_contact_store.UndoContactRequestParams{
		RequesterUserID: userId.UuidUserId,
		ReceiverUserID:  receiverUUID,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	switch result {
	case "undone":
		return &rpc_common_modelv1.StatusOkay{Status: true, Message: "contact_request_undone"}, nil
	case "not_found":
		return nil, kit.NewError(http.StatusNotFound, "not_found", "pending_request_not_found")
	default:
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "unexpected outcome")
	}
}

func (ps *contactService) GetContactRequests(ctx context.Context, userId kit.UserId) (*rpc_personal_contactv1.GetContactRequestsResponse, error) {
	// Fetch slim request data
	pendingRows, err := ps.PostgresQueries.GetPendingContactRequestsLite(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	sentRows, err := ps.PostgresQueries.GetSentContactRequestsLite(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	// Collect unique target IDs
	targetIDs := make([]uuid.UUID, 0, len(pendingRows)+len(sentRows))
	seen := make(map[uuid.UUID]struct{})

	for _, r := range pendingRows {
		if _, exists := seen[r.ID]; !exists {
			targetIDs = append(targetIDs, r.ID)
			seen[r.ID] = struct{}{}
		}
	}
	for _, r := range sentRows {
		if _, exists := seen[r.ID]; !exists {
			targetIDs = append(targetIDs, r.ID)
			seen[r.ID] = struct{}{}
		}
	}

	// Batch fetch enriched profiles
	var profilesByID map[uuid.UUID]*personal_profile.ContactProfileView
	if len(targetIDs) > 0 {
		profilesByID, err = ps.personalProfilePersonalContactProvider.GetContactableProfilesForViewer(
			ctx,
			userId.UuidUserId,
			targetIDs,
		)
		if err != nil {
			return nil, err
		}
	} else {
		profilesByID = map[uuid.UUID]*personal_profile.ContactProfileView{}
	}

	// Build pending list
	pending := make([]*rpc_personal_contactv1.PendingContactRequest, 0, len(pendingRows))
	for _, r := range pendingRows {
		profile, ok := profilesByID[r.ID]
		if !ok {
			continue
		}

		pending = append(pending, &rpc_personal_contactv1.PendingContactRequest{
			Id:           r.ID.String(),
			Name:         profile.Name,
			Username:     profile.Username,
			Bio:          profile.Bio,
			Nickname:     nil, // Privacy: Receiver should not see the nickname given by requester
			RequestedAt:  timestamppb.New(r.RequestCreatedAt),
			UpdatedAt:    timestamppb.New(r.RequestUpdatedAt),
			Status:       r.Status,
			AvatarUrl:    profile.AvatarURL,
			AvatarFileId: profile.AvatarFileId,
		})
	}

	// Build sent list
	sent := make([]*rpc_personal_contactv1.SentContactRequest, 0, len(sentRows))
	for _, r := range sentRows {
		profile, ok := profilesByID[r.ID]
		if !ok {
			continue
		}

		var nickname *string
		if r.Nickname != nil {
			decrypted, err := ps.DecryptNickname(r.Nickname, userId.UuidUserId, r.ID)
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to decrypt sent request nickname")
			}
			nickname = decrypted
		}

		sent = append(sent, &rpc_personal_contactv1.SentContactRequest{
			Id:           r.ID.String(),
			Name:         profile.Name,
			Username:     profile.Username,
			Bio:          profile.Bio,
			Nickname:     nickname,
			RequestedAt:  timestamppb.New(r.RequestCreatedAt),
			UpdatedAt:    timestamppb.New(r.RequestUpdatedAt),
			Status:       r.Status,
			AvatarUrl:    profile.AvatarURL,
			AvatarFileId: profile.AvatarFileId,
		})
	}

	return &rpc_personal_contactv1.GetContactRequestsResponse{
		PendingRequests: pending,
		SentRequests:    sent,
	}, nil
}

func (ps *contactService) UpdateContactNickname(ctx context.Context, payload *UpdateContactNicknamePayload, userId kit.UserId) (*rpc_common_modelv1.StatusOkay, error) {
	if payload == nil || payload.ContactUserId == "" {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	contactUUID, err := uuid.Parse(payload.ContactUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid contactUserId")
	}

	if contactUUID == userId.UuidUserId {
		return nil, kit.NewError(http.StatusConflict, "conflict", "self_action_not_allowed")
	}

	if err := ps.checkBlockStatus(ctx, userId.UuidUserId, contactUUID); err != nil {
		return nil, err
	}

	var nickname *string
	if payload.Nickname != nil {
		trimmed := strings.TrimSpace(*payload.Nickname)
		if trimmed != "" {
			if len([]rune(trimmed)) > 40 {
				return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid_nickname_length")
			}
			nickname = new(trimmed)
		}
	}

	// Encrypt nickname if provided
	var encryptedNickname *string
	if nickname != nil {
		encrypted, err := ps.EncryptNickname(*nickname, userId.UuidUserId, contactUUID)
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to encrypt nickname")
		}
		encryptedNickname = &encrypted
	}

	_, err = ps.PostgresQueries.UpdateContactNickname(ctx, personal_contact_store.UpdateContactNicknameParams{
		OwnerUserID:   userId.UuidUserId,
		ContactUserID: contactUUID,
		Nickname:      encryptedNickname,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusNotFound, "not_found", "contact_not_found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	return &rpc_common_modelv1.StatusOkay{Status: true, Message: "contact_nickname_updated"}, nil
}

func (ps *contactService) RemoveContactNickname(ctx context.Context, payload *RemoveContactNicknamePayload, userId kit.UserId) (*rpc_common_modelv1.StatusOkay, error) {
	if payload == nil || payload.ContactUserId == "" {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	contactUUID, err := uuid.Parse(payload.ContactUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid contactUserId")
	}

	if contactUUID == userId.UuidUserId {
		return nil, kit.NewError(http.StatusConflict, "conflict", "self_action_not_allowed")
	}

	if err := ps.checkBlockStatus(ctx, userId.UuidUserId, contactUUID); err != nil {
		return nil, err
	}

	_, err = ps.PostgresQueries.UpdateContactNickname(ctx, personal_contact_store.UpdateContactNicknameParams{
		OwnerUserID:   userId.UuidUserId,
		ContactUserID: contactUUID,
		Nickname:      nil,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusNotFound, "not_found", "contact_not_found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	return &rpc_common_modelv1.StatusOkay{Status: true, Message: "contact_nickname_removed"}, nil
}

func (ps *contactService) BlockUser(ctx context.Context, payload *BlockUserPayload, userId kit.UserId) (*rpc_personal_contactv1.BlockUserResponse, error) {
	if payload == nil || payload.BlockedUserId == "" {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	blockedUUID, err := uuid.Parse(payload.BlockedUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid blockedUserId")
	}

	if blockedUUID == userId.UuidUserId {
		return nil, kit.NewError(http.StatusConflict, "conflict", "self_block_not_allowed")
	}

	// Check if target exists and is not admin-blocked
	targetProfile, err := ps.personalProfilePersonalContactProvider.GetUserCoreProfile(ctx, blockedUUID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, kit.NewError(http.StatusNotFound, "not_found", "user_not_found")
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	if targetProfile.IsAdminBlocked {
		return nil, kit.NewError(http.StatusForbidden, "forbidden", "user_admin_blocked")
	}

	// Check if already blocked (to match original fidelity and prevent duplicate entry errors)
	blockStatus, err := ps.personalProfilePersonalContactProvider.IsEitherBlocked(ctx, userId.UuidUserId, blockedUUID)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	if blockStatus == 1 {
		// Already blocked by you
		return &rpc_personal_contactv1.BlockUserResponse{Blocked: true}, nil
	}

	blockID, err := uuid.NewV7()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to generate block ID")
	}

	err = ps.personalProfilePersonalContactProvider.CreateUserBlock(ctx, blockID, userId.UuidUserId, blockedUUID)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	if ps.chatCleanupProvider != nil {
		go ps.chatCleanupProvider.CleanupChatMessagesForBlockAsync(context.Background(), userId.UuidUserId, blockedUUID)
	}

	return &rpc_personal_contactv1.BlockUserResponse{Blocked: true}, nil
}

// GetMessagingBlockStatus returns 0 if no block, 1 if user1 blocked user2, 2 if user2 blocked user1.
func (ps *contactService) GetMessagingBlockStatus(ctx context.Context, user1ID uuid.UUID, user2ID uuid.UUID) (int32, error) {
	return ps.personalProfilePersonalContactProvider.IsEitherBlocked(ctx, user1ID, user2ID)
}

// IsAlreadyContact returns true if ownerID has contactID in their contacts list.
func (ps *contactService) IsAlreadyContact(ctx context.Context, ownerID uuid.UUID, contactID uuid.UUID) (bool, error) {
	return ps.PostgresQueries.IsAlreadyContact(ctx, personal_contact_store.IsAlreadyContactParams{
		OwnerUserID:   ownerID,
		ContactUserID: contactID,
	})
}
