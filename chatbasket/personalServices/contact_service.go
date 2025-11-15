package personalServices

import (
	"chatbasket/db/postgresCode"
	"chatbasket/model"
	personalmodel "chatbasket/personalModel"
	"chatbasket/utils"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (ps *Service) GetContacts(ctx context.Context, userId model.UserId) (*personalmodel.GetContactsResponse, *model.ApiError) {
	/*
		DB call to get user's contacts
	*/
	myContacts, err := ps.Queries.GetUserContacts(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	/*
		DB call to get users who added you
	*/
	addedMe, err := ps.Queries.GetUsersWhoAddedYou(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	if len(myContacts) == 0 && len(addedMe) == 0 {
		return &personalmodel.GetContactsResponse{
			Contacts:          []personalmodel.Contact{},
			PeopleWhoAddedYou: []personalmodel.Contact{},
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

	contacts := make([]personalmodel.Contact, 0, len(myContacts))
	for _, c := range myContacts {
		username := ""
		if c.Username != "" {
			var err error
			username, err = utils.DecryptUsername(c.Username, ps.Appwrite.PersonalUsernameKey)
			if err != nil {
				return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "failed to decrypt contact username", Type: "internal_server_error"}
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
		if shouldExposeAvatar(c.GlobalRestrictProfile, c.ExceptionGlobalProfile, c.GlobalRestrictAvatar, c.ExceptionGlobalAvatar, c.UserRestrictProfile, c.UserRestrictAvatar) {
			url, apiErr := ps.buildAvatarURL(ctx, c.AvatarFileID, c.AvatarTokenID, c.AvatarTokenSecret, c.AvatarTokenExpiry, c.ID)
			if apiErr != nil {
				return nil, apiErr
			}
			avatarURL = url
		}

		_, isMutual := addedMeMap[c.ID.String()]

		contacts = append(contacts, personalmodel.Contact{
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

	peopleWhoAddedYou := make([]personalmodel.Contact, 0, len(addedMe))
	for _, p := range addedMe {
		username := ""
		if p.Username != "" {
			var err error
			username, err = utils.DecryptUsername(p.Username, ps.Appwrite.PersonalUsernameKey)
			if err != nil {
				return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "failed to decrypt contact username", Type: "internal_server_error"}
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
		if shouldExposeAvatar(p.GlobalRestrictProfile, p.ExceptionGlobalProfile, p.GlobalRestrictAvatar, p.ExceptionGlobalAvatar, p.UserRestrictProfile, p.UserRestrictAvatar) {
			url, apiErr := ps.buildAvatarURL(ctx, p.AvatarFileID, p.AvatarTokenID, p.AvatarTokenSecret, p.AvatarTokenExpiry, p.ID)
			if apiErr != nil {
				return nil, apiErr
			}
			avatarURL = url
		}

		_, isMutual := myContactsMap[p.ID.String()]
		var myNickname *string
		if n, ok := myNicknameByID[p.ID.String()]; ok {
			myNickname = n
		}

		peopleWhoAddedYou = append(peopleWhoAddedYou, personalmodel.Contact{
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

	return &personalmodel.GetContactsResponse{
		Contacts:          contacts,
		PeopleWhoAddedYou: peopleWhoAddedYou,
	}, nil
}

func (ps *Service) CheckContactExistance(ctx context.Context, payload *personalmodel.CheckContactExistancePayload, userId model.UserId) (*personalmodel.CheckContactExistanceResponse, *model.ApiError) {

	hashContactUsername, err := utils.HashUsername(payload.ContactUsername, ps.Appwrite.PersonalUsernameKey)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "failed to hash contact username", Type: "internal_server_error"}
	}

	/*
		DB call to get user by hashed username
	*/
	user, err := ps.Queries.GetUserByHashedUsername(ctx, hashContactUsername)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &personalmodel.CheckContactExistanceResponse{Exists: false}, nil
		}
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	if user.ID == userId.UuidUserId {
		return &personalmodel.CheckContactExistanceResponse{Exists: false}, nil
	}

	existsResp := &personalmodel.CheckContactExistanceResponse{
		Exists:      true,
		ProfileType: user.ProfileType,
	}

	recipentUserId := user.ID.String()
	// Only set RecipientUserId if profile is not private
	if user.ProfileType != "private" {
		existsResp.RecipientUserId = &recipentUserId
	}

	return existsResp, nil
}

func (ps *Service) CreateContact(ctx context.Context, payload *personalmodel.CreateContactPayload, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	// CreateContact handles adding a new contact or sending a contact request based on profile type and blocking rules.
	// Note: Contacts are one-way; adding someone as a contact does not automatically make it mutual.
	// The target user must separately add you back for mutual contacts.
	if payload == nil || payload.ContactUserId == "" {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"}
	}

	// Validate payload and parse target UUID
	targetUUID, err := uuid.Parse(payload.ContactUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid contactUserId", Type: "bad_request"}
	}
	// Prevent self-addition
	if targetUUID == userId.UuidUserId {
		return nil, &model.ApiError{Code: http.StatusConflict, Message: "self_addition", Type: "conflict"}
	}

	/*
		DB call to check if user is admin-blocked
	*/
	isMeAdminBlocked, err := ps.Queries.IsUserAdminBlocked(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}
	if isMeAdminBlocked {
		return nil, &model.ApiError{Code: http.StatusForbidden, Message: "self_admin_blocked", Type: "forbidden"}
	}

	/*
		DB call to get target user's core profile
	*/
	targetProfile, err := ps.Queries.GetUserCoreProfile(ctx, targetUUID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.ApiError{Code: http.StatusNotFound, Message: "user_not_found", Type: "not_found"}
		}
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	// Check if target is admin-blocked
	if targetProfile.IsAdminBlocked {
		return nil, &model.ApiError{Code: http.StatusForbidden, Message: "user_admin_blocked", Type: "forbidden"}
	}

	/*
		DB call to check if users are mutually blocked
	*/
	var blockStatus int32
	blockStatus, err = ps.Queries.IsEitherBlocked(ctx, postgresCode.IsEitherBlockedParams{
		BlockerUserID: userId.UuidUserId,
		BlockedUserID: targetUUID,
	})
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}
	switch blockStatus {
	case 1:
		return nil, &model.ApiError{Code: http.StatusForbidden, Message: "you_blocked_user", Type: "forbidden"}
	case 2:
		return nil, &model.ApiError{Code: http.StatusForbidden, Message: "user_blocked_you", Type: "forbidden"}
	case 0:
		// No block, continue
	default:
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "unexpected block status", Type: "internal_server_error"}
	}

	/*
		DB call to check if already a contact
	*/
	alreadyContact, err := ps.Queries.IsAlreadyContact(ctx, postgresCode.IsAlreadyContactParams{
		OwnerUserID:   userId.UuidUserId,
		ContactUserID: targetUUID,
	})
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}
	if alreadyContact {
		return &model.StatusOkay{Status: true, Message: "already_in_contacts"}, nil
	}

	// Normalize optional nickname
	var nickname *string
	if payload.Nickname != nil {
		trimmed := strings.TrimSpace(*payload.Nickname)
		if trimmed != "" {
			if len([]rune(trimmed)) > 40 {
				return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid_nickname_length", Type: "bad_request"}
			}
			v := trimmed
			nickname = &v
		}
	}

	// Handle based on target profile type
	switch targetProfile.ProfileType {
	case "private":
		return nil, &model.ApiError{Code: http.StatusForbidden, Message: "user_private_profile", Type: "forbidden"}
	case "public":
		/*
			DB call to add contact
		*/
		err = ps.Queries.InsertUserContact(ctx, postgresCode.InsertUserContactParams{
			OwnerUserID:   userId.UuidUserId,
			ContactUserID: targetUUID,
			Nickname:      nickname,
		})
		if err != nil {
			return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
		}
		return &model.StatusOkay{Status: true, Message: "public_contact_added"}, nil
	case "personal":
		/*
			DB call to check for existing request status
		*/
		requestStatus, err := ps.Queries.GetContactRequestStatus(ctx, postgresCode.GetContactRequestStatusParams{
			RequesterUserID: userId.UuidUserId,
			ReceiverUserID:  targetUUID,
		})
		if err != nil && err != pgx.ErrNoRows {
			return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
		}

		// Generate new request ID
		reqID, err := uuid.NewV7()
		if err != nil {
			return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "failed to generate request ID", Type: "internal_server_error"}
		}

		// If request exists, check its status
		if err != pgx.ErrNoRows && requestStatus != "" {
			if requestStatus == "pending" {
				// If pending, don't delete old request, just return
				return &model.StatusOkay{Status: true, Message: "pending_request_exists"}, nil
			}

			// If accepted or declined, delete old request and insert new one
			/*
				DB call to delete old request and insert new contact request
			*/
			err = ps.Queries.DeleteAndInsertContactRequest(ctx, postgresCode.DeleteAndInsertContactRequestParams{
				ID:              reqID,
				RequesterUserID: userId.UuidUserId,
				ReceiverUserID:  targetUUID,
				Nickname:        nickname,
			})
			if err != nil {
				return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
			}
			return &model.StatusOkay{Status: true, Message: "contact_request_sent"}, nil
		}

		// No existing request, insert new one
		/*
			DB call to insert contact request
		*/
		err = ps.Queries.InsertContactRequest(ctx, postgresCode.InsertContactRequestParams{
			ID:              reqID,
			RequesterUserID: userId.UuidUserId,
			ReceiverUserID:  targetUUID,
			Nickname:        nickname,
		})
		if err != nil {
			return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
		}
		return &model.StatusOkay{Status: true, Message: "contact_request_sent"}, nil
	default:
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid target profile type", Type: "bad_request"}
	}
}

func (ps *Service) AcceptContactRequest(ctx context.Context, payload *personalmodel.AcceptContactRequestPayload, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	if payload == nil || payload.ContactUserId == "" {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"}
	}

	requesterUUID, err := uuid.Parse(payload.ContactUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid contactUserId", Type: "bad_request"}
	}

	if requesterUUID == userId.UuidUserId {
		return nil, &model.ApiError{Code: http.StatusConflict, Message: "self_action_not_allowed", Type: "conflict"}
	}

	result, err := ps.Queries.AcceptContactRequest(ctx, postgresCode.AcceptContactRequestParams{
		RequesterUserID: requesterUUID,
		ReceiverUserID:  userId.UuidUserId,
	})
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	switch result {
	case "accepted":

		return &model.StatusOkay{Status: true, Message: "contact_request_accepted"}, nil
	case "not_found":
		return nil, &model.ApiError{Code: http.StatusNotFound, Message: "pending_request_not_found", Type: "not_found"}
	case "processed":
		return nil, &model.ApiError{Code: http.StatusConflict, Message: "request_already_processed", Type: "conflict"}
	default:
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "unexpected outcome", Type: "internal_server_error"}
	}
}

func (ps *Service) RejectContactRequest(ctx context.Context, payload *personalmodel.RejectContactRequestPayload, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	if payload == nil || payload.ContactUserId == "" {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"}
	}

	requesterUUID, err := uuid.Parse(payload.ContactUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid contactUserId", Type: "bad_request"}
	}

	if requesterUUID == userId.UuidUserId {
		return nil, &model.ApiError{Code: http.StatusConflict, Message: "self_action_not_allowed", Type: "conflict"}
	}

	result, err := ps.Queries.RejectContactRequest(ctx, postgresCode.RejectContactRequestParams{
		RequesterUserID: requesterUUID,
		ReceiverUserID:  userId.UuidUserId,
	})
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	switch result {
	case "declined":
		return &model.StatusOkay{Status: true, Message: "contact_request_declined"}, nil
	case "not_found":
		return nil, &model.ApiError{Code: http.StatusNotFound, Message: "pending_request_not_found", Type: "not_found"}
	case "processed":
		return nil, &model.ApiError{Code: http.StatusConflict, Message: "request_already_processed", Type: "conflict"}
	default:
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "unexpected outcome", Type: "internal_server_error"}
	}
}

func (ps *Service) DeleteContact(ctx context.Context, payload *personalmodel.DeleteContactPayload, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	if payload == nil || len(payload.ContactUserId) == 0 {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"}
	}

	uniqIDs := make([]uuid.UUID, 0, len(payload.ContactUserId))
	seen := make(map[uuid.UUID]struct{}, len(payload.ContactUserId))

	for _, raw := range payload.ContactUserId {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid contactUserId", Type: "bad_request"}
		}

		contactUUID, err := uuid.Parse(trimmed)
		if err != nil {
			return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid contactUserId", Type: "bad_request"}
		}

		if contactUUID == userId.UuidUserId {
			return nil, &model.ApiError{Code: http.StatusConflict, Message: "self_action_not_allowed", Type: "conflict"}
		}

		if _, exists := seen[contactUUID]; exists {
			continue
		}
		seen[contactUUID] = struct{}{}
		uniqIDs = append(uniqIDs, contactUUID)
	}

	if len(uniqIDs) == 0 {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"}
	}

	removed, err := ps.Queries.DeleteContact(ctx, postgresCode.DeleteContactParams{
		OwnerUserID:    userId.UuidUserId,
		ContactUserIds: uniqIDs,
	})
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	if removed == 0 {
		return nil, &model.ApiError{Code: http.StatusNotFound, Message: "contact_not_found", Type: "not_found"}
	}

	message := "contacts_deleted"
	if removed == 1 {
		message = "contact_deleted"
	} else if removed < int64(len(uniqIDs)) {
		message = "contacts_deleted_partial"
	}

	return &model.StatusOkay{Status: true, Message: message}, nil
}

func (ps *Service) UndoContactRequest(ctx context.Context, payload *personalmodel.UndoContactRequestPayload, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	if payload == nil || payload.ContactUserId == "" {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"}
	}

	receiverUUID, err := uuid.Parse(payload.ContactUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid contactUserId", Type: "bad_request"}
	}

	if receiverUUID == userId.UuidUserId {
		return nil, &model.ApiError{Code: http.StatusConflict, Message: "self_action_not_allowed", Type: "conflict"}
	}

	result, err := ps.Queries.UndoContactRequest(ctx, postgresCode.UndoContactRequestParams{
		RequesterUserID: userId.UuidUserId,
		ReceiverUserID:  receiverUUID,
	})
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	switch result {
	case "undone":
		return &model.StatusOkay{Status: true, Message: "contact_request_undone"}, nil
	case "not_found":
		return nil, &model.ApiError{Code: http.StatusNotFound, Message: "pending_request_not_found", Type: "not_found"}
	default:
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "unexpected outcome", Type: "internal_server_error"}
	}
}

func (ps *Service) GetContactRequests(ctx context.Context, userId model.UserId) (*personalmodel.GetContactRequestsResponse, *model.ApiError) {
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

	// Fetch viewer's contacts so we can reuse their own nicknames for pending requests
	myContacts, err := ps.Queries.GetUserContacts(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}
	myNicknameByID := make(map[string]*string, len(myContacts))
	for _, c := range myContacts {
		myNicknameByID[c.ID.String()] = c.Nickname
	}

	transformPending := func(rows []postgresCode.GetPendingContactRequestsRow) ([]personalmodel.PendingContactRequest, *model.ApiError) {
		requests := make([]personalmodel.PendingContactRequest, 0, len(rows))
		for _, r := range rows {
			username := ""
			if r.Username != "" {
				decoded, err := utils.DecryptUsername(r.Username, ps.Appwrite.PersonalUsernameKey)
				if err != nil {
					return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "failed to decrypt username", Type: "internal_server_error"}
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
			if shouldExposeAvatar(r.GlobalRestrictProfile, r.ExceptionGlobalProfile, r.GlobalRestrictAvatar, r.ExceptionGlobalAvatar, r.UserRestrictProfile, r.UserRestrictAvatar) {
				url, apiErr := ps.buildAvatarURL(ctx, r.AvatarFileID, r.AvatarTokenID, r.AvatarTokenSecret, r.AvatarTokenExpiry, r.ID)
				if apiErr != nil {
					return nil, apiErr
				}
				avatarURL = url
			}

			// Use viewer's own contact nickname for this user, if it exists
			var myNickname *string
			if n, ok := myNicknameByID[r.ID.String()]; ok {
				myNickname = n
			}

			requests = append(requests, personalmodel.PendingContactRequest{
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

	transformSent := func(rows []postgresCode.GetSentContactRequestsRow) ([]personalmodel.SentContactRequest, *model.ApiError) {
		records := make([]personalmodel.SentContactRequest, 0, len(rows))
		for _, r := range rows {
			username := ""
			if r.Username != "" {
				decoded, err := utils.DecryptUsername(r.Username, ps.Appwrite.PersonalUsernameKey)
				if err != nil {
					return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: "failed to decrypt username", Type: "internal_server_error"}
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
			if shouldExposeAvatar(r.GlobalRestrictProfile, r.ExceptionGlobalProfile, r.GlobalRestrictAvatar, r.ExceptionGlobalAvatar, r.UserRestrictProfile, r.UserRestrictAvatar) {
				url, apiErr := ps.buildAvatarURL(ctx, r.AvatarFileID, r.AvatarTokenID, r.AvatarTokenSecret, r.AvatarTokenExpiry, r.ID)
				if apiErr != nil {
					return nil, apiErr
				}
				avatarURL = url
			}

			records = append(records, personalmodel.SentContactRequest{
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

	pendingRows, err := ps.Queries.GetPendingContactRequests(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	sentRows, err := ps.Queries.GetSentContactRequests(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	pending, apiErr := transformPending(pendingRows)
	if apiErr != nil {
		return nil, apiErr
	}

	sent, apiErr := transformSent(sentRows)
	if apiErr != nil {
		return nil, apiErr
	}

	return &personalmodel.GetContactRequestsResponse{
		Pending: pending,
		Sent:    sent,
	}, nil
}

func (ps *Service) UpdateContactNickname(ctx context.Context, payload *personalmodel.UpdateContactNicknamePayload, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	if payload == nil || payload.ContactUserId == "" {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"}
	}

	contactUUID, err := uuid.Parse(payload.ContactUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid contactUserId", Type: "bad_request"}
	}

	if contactUUID == userId.UuidUserId {
		return nil, &model.ApiError{Code: http.StatusConflict, Message: "self_action_not_allowed", Type: "conflict"}
	}

	var nickname *string
	if payload.Nickname != nil {
		trimmed := strings.TrimSpace(*payload.Nickname)
		if trimmed != "" {
			if len([]rune(trimmed)) > 40 {
				return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid_nickname_length", Type: "bad_request"}
			}
			v := trimmed
			nickname = &v
		}
	}

	_, err = ps.Queries.UpdateContactNickname(ctx, postgresCode.UpdateContactNicknameParams{
		OwnerUserID:   userId.UuidUserId,
		ContactUserID: contactUUID,
		Nickname:      nickname,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.ApiError{Code: http.StatusNotFound, Message: "contact_not_found", Type: "not_found"}
		}
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	return &model.StatusOkay{Status: true, Message: "contact_nickname_updated"}, nil
}

func (ps *Service) RemoveContactNickname(ctx context.Context, payload *personalmodel.RemoveContactNicknamePayload, userId model.UserId) (*model.StatusOkay, *model.ApiError) {
	if payload == nil || payload.ContactUserId == "" {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid request payload", Type: "bad_request"}
	}

	contactUUID, err := uuid.Parse(payload.ContactUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusBadRequest, Message: "invalid contactUserId", Type: "bad_request"}
	}

	if contactUUID == userId.UuidUserId {
		return nil, &model.ApiError{Code: http.StatusConflict, Message: "self_action_not_allowed", Type: "conflict"}
	}

	_, err = ps.Queries.UpdateContactNickname(ctx, postgresCode.UpdateContactNicknameParams{
		OwnerUserID:   userId.UuidUserId,
		ContactUserID: contactUUID,
		Nickname:      nil,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.ApiError{Code: http.StatusNotFound, Message: "contact_not_found", Type: "not_found"}
		}
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

	return &model.StatusOkay{Status: true, Message: "contact_nickname_removed"}, nil
}
