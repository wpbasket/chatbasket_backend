package personal_contact

import (
	"chatbasket-api/internal/modules/personal/personal_contact/internal/personal_contact_store"
	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/services"
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// personalProfilePersonalContactProvider defines the minimal set of methods required from the Profile module.
type personalProfilePersonalContactProvider interface {
	IsUserAdminBlocked(ctx context.Context, userID uuid.UUID) (bool, error)
	GetUserCoreProfile(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error)
	GetVisibleProfilesForContactViewer(ctx context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error)
	FindContactableUserByUsername(ctx context.Context, viewerID uuid.UUID, username string) (*personal_profile.ContactLookupResult, error)
}

type contactService struct {
	GlobalService                  *services.GlobalService
	PostgresQuerier                personal_contact_store.Querier
	PostgresQueries                *personal_contact_store.Queries
	personalProfilePersonalContactProvider personalProfilePersonalContactProvider
	PersonalUsernameKey            []byte
	PersonalContactKey             []byte
}

func NewContactService(globalService *services.GlobalService, pool *pgxpool.Pool, personalProfilePersonalContactProvider personalProfilePersonalContactProvider, personalUsernameKey []byte, personalContactKey []byte) *contactService {
	store := personal_contact_store.New(pool)
	return &contactService{
		GlobalService:                  globalService,
		PostgresQuerier:                store,
		PostgresQueries:                store,
		personalProfilePersonalContactProvider: personalProfilePersonalContactProvider,
		PersonalUsernameKey:            personalUsernameKey,
		PersonalContactKey:             personalContactKey,
	}
}

func (ps *contactService) GetContacts(ctx context.Context, userId kit.UserId) (*GetContactsResponse, error) {
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
		return &GetContactsResponse{
			Contacts:          []Contact{},
			PeopleWhoAddedYou: []Contact{},
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
	profilesByID, err := ps.personalProfilePersonalContactProvider.GetVisibleProfilesForContactViewer(
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
	contacts := make([]Contact, 0, len(myContacts))
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

		contacts = append(contacts, Contact{
			ID:        c.ID.String(),
			Name:      profile.Name,
			Username:  profile.Username,
			Bio:       profile.Bio,
			Nickname:  nickname,
			CreatedAt: c.ContactCreatedAt,
			UpdatedAt: c.ContactUpdatedAt,
			AvatarURL: profile.AvatarURL,
			IsMutual:  isMutual,
		})
	}

	// Build people who added you list
	peopleWhoAddedYou := make([]Contact, 0, len(addedMe))
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

		peopleWhoAddedYou = append(peopleWhoAddedYou, Contact{
			ID:        p.ID.String(),
			Name:      profile.Name,
			Username:  profile.Username,
			Bio:       profile.Bio,
			Nickname:  myNickname,
			CreatedAt: p.ContactCreatedAt,
			UpdatedAt: p.ContactUpdatedAt,
			AvatarURL: profile.AvatarURL,
			IsMutual:  isMutual,
		})
	}

	return &GetContactsResponse{
		Contacts:          contacts,
		PeopleWhoAddedYou: peopleWhoAddedYou,
	}, nil
}

func (ps *contactService) CheckContactExistance(ctx context.Context, payload *CheckContactExistancePayload, userId kit.UserId) (*CheckContactExistanceResponse, error) {
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
		return &CheckContactExistanceResponse{Exists: false}, nil
	}

	existsResp := &CheckContactExistanceResponse{
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

func (ps *contactService) CreateContact(ctx context.Context, payload *CreateContactPayload, userId kit.UserId) (*kit.StatusOkay, error) {
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
	blockStatus, err := ps.PostgresQueries.IsEitherBlocked(ctx, personal_contact_store.IsEitherBlockedParams{
		BlockerUserID: userId.UuidUserId,
		BlockedUserID: targetUUID,
	})
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
		return &kit.StatusOkay{Status: true, Message: "already_in_contacts"}, nil
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
		return &kit.StatusOkay{Status: true, Message: "public_contact_added"}, nil
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
			return &kit.StatusOkay{Status: true, Message: "personal_contact_added"}, nil
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
				return &kit.StatusOkay{Status: true, Message: "pending_request_exists"}, nil
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
			return &kit.StatusOkay{Status: true, Message: "contact_request_sent"}, nil
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
		return &kit.StatusOkay{Status: true, Message: "contact_request_sent"}, nil
	default:
		return nil, kit.NewError(http.StatusBadRequest, "bad_request", "invalid target profile type")
	}
}

func (ps *contactService) AcceptContactRequest(ctx context.Context, payload *AcceptContactRequestPayload, userId kit.UserId) (*kit.StatusOkay, error) {
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

	result, err := ps.PostgresQueries.AcceptContactRequest(ctx, personal_contact_store.AcceptContactRequestParams{
		RequesterUserID: requesterUUID,
		ReceiverUserID:  userId.UuidUserId,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	switch result {
	case "accepted":
		return &kit.StatusOkay{Status: true, Message: "contact_request_accepted"}, nil
	case "not_found":
		return nil, kit.NewError(http.StatusNotFound, "not_found", "pending_request_not_found")
	case "processed":
		return nil, kit.NewError(http.StatusConflict, "conflict", "request_already_processed")
	default:
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "unexpected outcome")
	}
}

func (ps *contactService) RejectContactRequest(ctx context.Context, payload *RejectContactRequestPayload, userId kit.UserId) (*kit.StatusOkay, error) {
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

	result, err := ps.PostgresQueries.RejectContactRequest(ctx, personal_contact_store.RejectContactRequestParams{
		RequesterUserID: requesterUUID,
		ReceiverUserID:  userId.UuidUserId,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	switch result {
	case "declined":
		return &kit.StatusOkay{Status: true, Message: "contact_request_declined"}, nil
	case "not_found":
		return nil, kit.NewError(http.StatusNotFound, "not_found", "pending_request_not_found")
	case "processed":
		return nil, kit.NewError(http.StatusConflict, "conflict", "request_already_processed")
	default:
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "unexpected outcome")
	}
}

func (ps *contactService) DeleteContact(ctx context.Context, payload *DeleteContactPayload, userId kit.UserId) (*kit.StatusOkay, error) {
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

	removed, err := ps.PostgresQueries.DeleteContact(ctx, personal_contact_store.DeleteContactParams{
		OwnerUserID:    userId.UuidUserId,
		ContactUserIds: uniqIDs,
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

	return &kit.StatusOkay{Status: true, Message: message}, nil
}

func (ps *contactService) UndoContactRequest(ctx context.Context, payload *UndoContactRequestPayload, userId kit.UserId) (*kit.StatusOkay, error) {
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

	result, err := ps.PostgresQueries.UndoContactRequest(ctx, personal_contact_store.UndoContactRequestParams{
		RequesterUserID: userId.UuidUserId,
		ReceiverUserID:  receiverUUID,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	switch result {
	case "undone":
		return &kit.StatusOkay{Status: true, Message: "contact_request_undone"}, nil
	case "not_found":
		return nil, kit.NewError(http.StatusNotFound, "not_found", "pending_request_not_found")
	default:
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "unexpected outcome")
	}
}

func (ps *contactService) GetContactRequests(ctx context.Context, userId kit.UserId) (*GetContactRequestsResponse, error) {
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
		profilesByID, err = ps.personalProfilePersonalContactProvider.GetVisibleProfilesForContactViewer(
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
	pending := make([]PendingContactRequest, 0, len(pendingRows))
	for _, r := range pendingRows {
		profile, ok := profilesByID[r.ID]
		if !ok {
			continue
		}

		pending = append(pending, PendingContactRequest{
			ID:          r.ID.String(),
			Name:        profile.Name,
			Username:    profile.Username,
			Bio:         profile.Bio,
			Nickname:    nil, // Privacy: Receiver should not see the nickname given by requester
			RequestedAt: r.RequestCreatedAt,
			UpdatedAt:   r.RequestUpdatedAt,
			Status:      r.Status,
			AvatarURL:   profile.AvatarURL,
		})
	}

	// Build sent list
	sent := make([]SentContactRequest, 0, len(sentRows))
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

		sent = append(sent, SentContactRequest{
			ID:          r.ID.String(),
			Name:        profile.Name,
			Username:    profile.Username,
			Bio:         profile.Bio,
			Nickname:    nickname,
			RequestedAt: r.RequestCreatedAt,
			UpdatedAt:   r.RequestUpdatedAt,
			Status:      r.Status,
			AvatarURL:   profile.AvatarURL,
		})
	}

	return &GetContactRequestsResponse{
		Pending: pending,
		Sent:    sent,
	}, nil
}

func (ps *contactService) UpdateContactNickname(ctx context.Context, payload *UpdateContactNicknamePayload, userId kit.UserId) (*kit.StatusOkay, error) {
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

	return &kit.StatusOkay{Status: true, Message: "contact_nickname_updated"}, nil
}

func (ps *contactService) RemoveContactNickname(ctx context.Context, payload *RemoveContactNicknamePayload, userId kit.UserId) (*kit.StatusOkay, error) {
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

	return &kit.StatusOkay{Status: true, Message: "contact_nickname_removed"}, nil
}

func (ps *contactService) BlockUser(ctx context.Context, payload *BlockUserPayload, userId kit.UserId) (*BlockUserResponse, error) {
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
	blockStatus, err := ps.PostgresQueries.IsEitherBlocked(ctx, personal_contact_store.IsEitherBlockedParams{
		BlockerUserID: userId.UuidUserId,
		BlockedUserID: blockedUUID,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	if blockStatus == 1 {
		// Already blocked by you
		return &BlockUserResponse{Blocked: true}, nil
	}

	blockID, err := uuid.NewV7()
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to generate block ID")
	}

	err = ps.PostgresQueries.CreateUserBlock(ctx, personal_contact_store.CreateUserBlockParams{
		ID:            blockID,
		BlockerUserID: userId.UuidUserId,
		BlockedUserID: blockedUUID,
	})
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	// When blocking, we also remove the contact relationship if it exists (both ways)
	_, _ = ps.PostgresQueries.DeleteContact(ctx, personal_contact_store.DeleteContactParams{
		OwnerUserID:    userId.UuidUserId,
		ContactUserIds: []uuid.UUID{blockedUUID},
	})
	_, _ = ps.PostgresQueries.DeleteContact(ctx, personal_contact_store.DeleteContactParams{
		OwnerUserID:    blockedUUID,
		ContactUserIds: []uuid.UUID{userId.UuidUserId},
	})

	return &BlockUserResponse{Blocked: true}, nil
}

