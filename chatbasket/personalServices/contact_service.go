package personalServices

import (
	"chatbasket/model"
	"chatbasket/personalModel"
	"chatbasket/utils"
	"context"
	"net/http"
	"time"
)

func (ps *Service) GetContacts(ctx context.Context, userId model.UserId) (*personalmodel.GetContactsResponse, *model.ApiError) {
	myContacts, err := ps.Queries.GetUserContacts(ctx, userId.UuidUserId)
	if err != nil {
		return nil, &model.ApiError{Code: http.StatusInternalServerError, Message: utils.GetPostgresError(err).Message, Type: "internal_server_error"}
	}

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
	for _, c := range myContacts {
		myContactsMap[c.ID.String()] = struct{}{}
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
			ID:               c.ID.String(),
			Name:             c.Name,
			Username:         username,
			Bio:              c.Bio,
			CreatedAt:        createdAt,
			AvatarURL:        avatarURL,
			IsMutual:         isMutual,
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

		var avatarURL *string
		if shouldExposeAvatar(p.GlobalRestrictProfile, p.ExceptionGlobalProfile, p.GlobalRestrictAvatar, p.ExceptionGlobalAvatar, p.UserRestrictProfile, p.UserRestrictAvatar) {
			url, apiErr := ps.buildAvatarURL(ctx, p.AvatarFileID, p.AvatarTokenID, p.AvatarTokenSecret, p.AvatarTokenExpiry, p.ID)
			if apiErr != nil {
				return nil, apiErr
			}
			avatarURL = url
		}

		_, isMutual := myContactsMap[p.ID.String()]

		peopleWhoAddedYou = append(peopleWhoAddedYou, personalmodel.Contact{
			ID:               p.ID.String(),
			Name:             p.Name,
			Username:         username,
			Bio:              p.Bio,
			CreatedAt:        createdAt,
			AvatarURL:        avatarURL,
			IsMutual:         isMutual,
		})
	}

	return &personalmodel.GetContactsResponse{
		Contacts:          contacts,
		PeopleWhoAddedYou: peopleWhoAddedYou,
	}, nil
}