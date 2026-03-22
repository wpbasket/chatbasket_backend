package personal_contact

import (
	"chatbasket-apinext/internal/modules/personal/personal_contact/internal/personal_contact_store"
	"chatbasket-apinext/internal/modules/personal/personal_profile"
	"chatbasket-apinext/internal/platform/kit"
	"chatbasket-apinext/internal/platform/services"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// personalProfilePersonalContactProvider defines the minimal set of methods required from the Profile module.
type personalProfilePersonalContactProvider interface {
	IsUserAdminBlocked(ctx context.Context, userID uuid.UUID) (bool, error)
	GetUserCoreProfile(ctx context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error)
	GetRefreshedAvatarURL(ctx context.Context, userID uuid.UUID, fileID, tokenID, tokenSecret *string, tokenExpiry pgtype.Timestamptz) (*string, error)
}

type contactService struct {
	GlobalService                  *services.GlobalService
	PostgresQuerier                personal_contact_store.Querier
	PostgresQueries                *personal_contact_store.Queries
	personalProfilePersonalContactProvider personalProfilePersonalContactProvider
	PersonalUsernameKey            []byte
}

func NewContactService(globalService *services.GlobalService, pool *pgxpool.Pool, personalProfilePersonalContactProvider personalProfilePersonalContactProvider, personalUsernameKey []byte) *contactService {
	store := personal_contact_store.New(pool)
	return &contactService{
		GlobalService:                  globalService,
		PostgresQuerier:                store,
		PostgresQueries:                store,
		personalProfilePersonalContactProvider: personalProfilePersonalContactProvider,
		PersonalUsernameKey:            personalUsernameKey,
	}
}

func (ps *contactService) GetContacts(ctx context.Context, userId kit.UserId) (*GetContactsResponse, error) {
	// DB call to get user's contacts
	myContacts, err := ps.PostgresQueries.GetUserContacts(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	// DB call to get users who added you
	addedMe, err := ps.PostgresQueries.GetUsersWhoAddedYou(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	if len(myContacts) == 0 && len(addedMe) == 0 {
		return &GetContactsResponse{
			Contacts:          []Contact{},
			PeopleWhoAddedYou: []Contact{},
		}, nil
	}

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

	contacts := make([]Contact, 0, len(myContacts))
	for _, c := range myContacts {
		username := ""
		if c.Username != "" {
			var err error
			username, err = personal_profile.DecryptUsername(c.Username, ps.PersonalUsernameKey)
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to decrypt contact username")
			}
		}

		createdAt := time.Time{}
		if c.ContactCreatedAt.Valid {
			createdAt = c.ContactCreatedAt.Time
		}

		updatedAt := time.Time{}
		if c.ContactUpdatedAt.Valid {
			updatedAt = c.ContactUpdatedAt.Time
		}

		var avatarURL *string
		if personal_profile.ShouldExposeAvatar(c.GlobalRestrictProfile, c.ExceptionGlobalProfile, c.GlobalRestrictAvatar, c.ExceptionGlobalAvatar, c.UserRestrictProfile, c.UserRestrictAvatar) {
			url, err := ps.personalProfilePersonalContactProvider.GetRefreshedAvatarURL(ctx, c.ID, c.AvatarFileID, c.AvatarTokenID, c.AvatarTokenSecret, c.AvatarTokenExpiry)
			if err != nil {
				return nil, err
			}
			avatarURL = url
		}

		_, isMutual := addedMeMap[c.ID.String()]

		contacts = append(contacts, Contact{
			ID:        c.ID.String(),
			Name:      c.Name,
			Username:  username,
			Bio:       c.Bio,
			Nickname:  c.Nickname,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			AvatarURL: avatarURL,
			IsMutual:  isMutual,
		})
	}

	peopleWhoAddedYou := make([]Contact, 0, len(addedMe))
	for _, p := range addedMe {
		username := ""
		if p.Username != "" {
			var err error
			username, err = personal_profile.DecryptUsername(p.Username, ps.PersonalUsernameKey)
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to decrypt contact username")
			}
		}

		createdAt := time.Time{}
		if p.ContactCreatedAt.Valid {
			createdAt = p.ContactCreatedAt.Time
		}

		updatedAt := time.Time{}
		if p.ContactUpdatedAt.Valid {
			updatedAt = p.ContactUpdatedAt.Time
		}

		var avatarURL *string
		if personal_profile.ShouldExposeAvatar(p.GlobalRestrictProfile, p.ExceptionGlobalProfile, p.GlobalRestrictAvatar, p.ExceptionGlobalAvatar, p.UserRestrictProfile, p.UserRestrictAvatar) {
			url, err := ps.personalProfilePersonalContactProvider.GetRefreshedAvatarURL(ctx, p.ID, p.AvatarFileID, p.AvatarTokenID, p.AvatarTokenSecret, p.AvatarTokenExpiry)
			if err != nil {
				return nil, err
			}
			avatarURL = url
		}

		_, isMutual := myContactsMap[p.ID.String()]
		var myNickname *string
		if n, ok := myNicknameByID[p.ID.String()]; ok {
			myNickname = n
		}

		peopleWhoAddedYou = append(peopleWhoAddedYou, Contact{
			ID:        p.ID.String(),
			Name:      p.Name,
			Username:  username,
			Bio:       p.Bio,
			Nickname:  myNickname,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			AvatarURL: avatarURL,
			IsMutual:  isMutual,
		})
	}

	return &GetContactsResponse{
		Contacts:          contacts,
		PeopleWhoAddedYou: peopleWhoAddedYou,
	}, nil
}

func (ps *contactService) CheckContactExistance(ctx context.Context, payload *CheckContactExistancePayload, userId kit.UserId) (*CheckContactExistanceResponse, error) {

	hashContactUsername, err := kit.ComputeHMAC(payload.ContactUsername, ps.PersonalUsernameKey, false, nil)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to hash contact username")
	}

	// DB call to get user by hashed username
	user, err := ps.PostgresQueries.GetUserByHashedUsername(ctx, hashContactUsername)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &CheckContactExistanceResponse{Exists: false}, nil
		}
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	if user.ID == userId.UuidUserId {
		return &CheckContactExistanceResponse{Exists: false}, nil
	}

	existsResp := &CheckContactExistanceResponse{
		Exists:      true,
		Name:        user.Name,
		ProfileType: user.ProfileType,
	}

	// Only set RecipientUserId if profile is not private
	if user.ProfileType != "private" {
		existsResp.RecipientUserId = new(user.ID.String())
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
		// DB call to add contact
		err = ps.PostgresQueries.InsertUserContact(ctx, personal_contact_store.InsertUserContactParams{
			OwnerUserID:   userId.UuidUserId,
			ContactUserID: targetUUID,
			Nickname:      nickname,
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
			err = ps.PostgresQueries.InsertUserContact(ctx, personal_contact_store.InsertUserContactParams{
				OwnerUserID:   userId.UuidUserId,
				ContactUserID: targetUUID,
				Nickname:      nickname,
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

			// If accepted or declined, delete old request and insert new one
			err = ps.PostgresQueries.DeleteAndInsertContactRequest(ctx, personal_contact_store.DeleteAndInsertContactRequestParams{
				ID:              reqID,
				RequesterUserID: userId.UuidUserId,
				ReceiverUserID:  targetUUID,
				Nickname:        nickname,
			})
			if err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
			}
			return &kit.StatusOkay{Status: true, Message: "contact_request_sent"}, nil
		}

		// No existing request, insert new one
		err = ps.PostgresQueries.InsertContactRequest(ctx, personal_contact_store.InsertContactRequestParams{
			ID:              reqID,
			RequesterUserID: userId.UuidUserId,
			ReceiverUserID:  targetUUID,
			Nickname:        nickname,
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
	// Fetch viewer's contacts so we can reuse their own nicknames for pending requests
	myContacts, err := ps.PostgresQueries.GetUserContacts(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}
	myNicknameByID := make(map[string]*string, len(myContacts))
	for _, c := range myContacts {
		myNicknameByID[c.ID.String()] = c.Nickname
	}

	transformPending := func(rows []personal_contact_store.GetPendingContactRequestsRow) ([]PendingContactRequest, error) {
		requests := make([]PendingContactRequest, 0, len(rows))
		for _, r := range rows {
			username := ""
			if r.Username != "" {
				decoded, err := personal_profile.DecryptUsername(r.Username, ps.PersonalUsernameKey)
				if err != nil {
					return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to decrypt username")
				}
				username = decoded
			}

			requestedAt := time.Time{}
			if r.RequestCreatedAt.Valid {
				requestedAt = r.RequestCreatedAt.Time
			}

			updatedAt := time.Time{}
			if r.RequestUpdatedAt.Valid {
				updatedAt = r.RequestUpdatedAt.Time
			}

			var avatarURL *string
			if personal_profile.ShouldExposeAvatar(r.GlobalRestrictProfile, r.ExceptionGlobalProfile, r.GlobalRestrictAvatar, r.ExceptionGlobalAvatar, r.UserRestrictProfile, r.UserRestrictAvatar) {
				url, err := ps.personalProfilePersonalContactProvider.GetRefreshedAvatarURL(ctx, r.ID, r.AvatarFileID, r.AvatarTokenID, r.AvatarTokenSecret, r.AvatarTokenExpiry)
				if err != nil {
					return nil, err
				}
				avatarURL = url
			}

			var myNickname *string
			if n, ok := myNicknameByID[r.ID.String()]; ok {
				myNickname = n
			}

			requests = append(requests, PendingContactRequest{
				ID:          r.ID.String(),
				Name:        r.Name,
				Username:    username,
				Bio:         r.Bio,
				Nickname:    myNickname,
				RequestedAt: requestedAt,
				UpdatedAt:   updatedAt,
				Status:      r.Status,
				AvatarURL:   avatarURL,
			})
		}
		return requests, nil
	}

	transformSent := func(rows []personal_contact_store.GetSentContactRequestsRow) ([]SentContactRequest, error) {
		records := make([]SentContactRequest, 0, len(rows))
		for _, r := range rows {
			username := ""
			if r.Username != "" {
				decoded, err := personal_profile.DecryptUsername(r.Username, ps.PersonalUsernameKey)
				if err != nil {
					return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "failed to decrypt username")
				}
				username = decoded
			}

			requestedAt := time.Time{}
			if r.RequestCreatedAt.Valid {
				requestedAt = r.RequestCreatedAt.Time
			}

			updatedAt := time.Time{}
			if r.RequestUpdatedAt.Valid {
				updatedAt = r.RequestUpdatedAt.Time
			}

			var avatarURL *string
			if personal_profile.ShouldExposeAvatar(r.GlobalRestrictProfile, r.ExceptionGlobalProfile, r.GlobalRestrictAvatar, r.ExceptionGlobalAvatar, r.UserRestrictProfile, r.UserRestrictAvatar) {
				url, err := ps.personalProfilePersonalContactProvider.GetRefreshedAvatarURL(ctx, r.ID, r.AvatarFileID, r.AvatarTokenID, r.AvatarTokenSecret, r.AvatarTokenExpiry)
				if err != nil {
					return nil, err
				}
				avatarURL = url
			}

			records = append(records, SentContactRequest{
				ID:          r.ID.String(),
				Name:        r.Name,
				Username:    username,
				Bio:         r.Bio,
				Nickname:    r.Nickname,
				RequestedAt: requestedAt,
				UpdatedAt:   updatedAt,
				Status:      r.Status,
				AvatarURL:   avatarURL,
			})
		}
		return records, nil
	}

	pendingRows, err := ps.PostgresQueries.GetPendingContactRequests(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	sentRows, err := ps.PostgresQueries.GetSentContactRequests(ctx, userId.UuidUserId)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", kit.GetPostgresError(err).Message)
	}

	pending, err := transformPending(pendingRows)
	if err != nil {
		return nil, err
	}

	sent, err := transformSent(sentRows)
	if err != nil {
		return nil, err
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

	_, err = ps.PostgresQueries.UpdateContactNickname(ctx, personal_contact_store.UpdateContactNicknameParams{
		OwnerUserID:   userId.UuidUserId,
		ContactUserID: contactUUID,
		Nickname:      nickname,
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
