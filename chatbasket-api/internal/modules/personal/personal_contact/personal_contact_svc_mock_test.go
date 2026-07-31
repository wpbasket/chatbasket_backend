package personal_contact

import (
	"context"
	"errors"
	"testing"
	"time"

	rpc_common_modelv1 "chatbasket-api/gen/proto/common/model"
	rpc_personal_contactv1 "chatbasket-api/gen/proto/personal/personal_contact"
	"chatbasket-api/internal/modules/personal/personal_contact/internal/personal_contact_store"
	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/kit"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockContactProfileProvider struct {
	coreProfile     *personal_profile.UserCoreProfile
	contactProfiles map[uuid.UUID]*personal_profile.ContactProfileView
	blockStatus     *personal_profile.BlockStatusResult
	isEitherBlocked int32
}

func (m *mockContactProfileProvider) IsUserAdminBlocked(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}

func (m *mockContactProfileProvider) GetUserCoreProfile(_ context.Context, userID uuid.UUID) (*personal_profile.UserCoreProfile, error) {
	if m.coreProfile != nil {
		return m.coreProfile, nil
	}
	return &personal_profile.UserCoreProfile{ID: userID, ProfileType: "public"}, nil
}

func (m *mockContactProfileProvider) GetContactableProfilesForViewer(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error) {
	if m.contactProfiles == nil {
		return map[uuid.UUID]*personal_profile.ContactProfileView{}, nil
	}
	return m.contactProfiles, nil
}

func (m *mockContactProfileProvider) FindContactableUserByUsername(context.Context, uuid.UUID, string) (*personal_profile.ContactLookupResult, error) {
	return &personal_profile.ContactLookupResult{Exists: false}, nil
}

func (m *mockContactProfileProvider) CreateUserBlock(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func (m *mockContactProfileProvider) IsEitherBlocked(context.Context, uuid.UUID, uuid.UUID) (int32, error) {
	return m.isEitherBlocked, nil
}

func (m *mockContactProfileProvider) IsBlockedBetweenUsers(context.Context, uuid.UUID, uuid.UUID) (*personal_profile.BlockStatusResult, error) {
	if m.blockStatus != nil {
		return m.blockStatus, nil
	}
	return &personal_profile.BlockStatusResult{}, nil
}

func (m *mockContactProfileProvider) IsBlockedBetweenUsersBatch(_ context.Context, _ uuid.UUID, targetIDs []uuid.UUID) ([]*personal_profile.BlockStatusResult, error) {
	results := make([]*personal_profile.BlockStatusResult, len(targetIDs))
	for i, id := range targetIDs {
		results[i] = &personal_profile.BlockStatusResult{TargetID: id}
	}
	return results, nil
}

func newMockContactService(t *testing.T, profile *mockContactProfileProvider) (*contactService, pgxmock.PgxPoolIface) {
	t.Helper()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })

	store := personal_contact_store.New(mock)
	return &contactService{
		PostgresQuerier:                        store,
		PostgresQueries:                        store,
		personalProfilePersonalContactProvider: profile,
		PersonalContactKey:                     make([]byte, 32),
	}, mock
}

func TestCreateContact_PublicProfile(t *testing.T) {
	ownerID := uuid.New()
	contactID := uuid.New()
	now := time.Now()
	profile := &mockContactProfileProvider{
		coreProfile: &personal_profile.UserCoreProfile{ID: contactID, ProfileType: "public"},
		contactProfiles: map[uuid.UUID]*personal_profile.ContactProfileView{
			contactID: {
				ID:          contactID,
				Name:        "Contact",
				Username:    "CONTACT123",
				ProfileType: "public",
			},
		},
	}
	service, mock := newMockContactService(t, profile)
	nickname := "Best contact"
	encryptedNickname, err := service.EncryptNickname(nickname, ownerID, contactID)
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(ownerID, contactID).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`INSERT INTO user_contacts`).
		WithArgs(ownerID, contactID, pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectQuery(`SELECT\s+uc\.contact_user_id AS id`).
		WithArgs(ownerID, contactID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "nickname", "contact_created_at", "contact_updated_at"}).
			AddRow(contactID, &encryptedNickname, now, now))
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(contactID, ownerID).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

	res, err := service.CreateContact(context.Background(), &CreateContactPayload{
		ContactUserId: contactID.String(),
		Nickname:      &nickname,
	}, kit.UserId{UuidUserId: ownerID})

	require.NoError(t, err)
	assert.Equal(t, "public_contact_added", res.Message)
	require.NotNil(t, res.Contact)
	assert.Equal(t, contactID.String(), res.Contact.Id)
	assert.Equal(t, nickname, res.Contact.GetNickname())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateContact_PersonalProfileCreatesRequest(t *testing.T) {
	ownerID := uuid.New()
	contactID := uuid.New()
	profile := &mockContactProfileProvider{
		coreProfile: &personal_profile.UserCoreProfile{ID: contactID, ProfileType: "personal"},
	}
	service, mock := newMockContactService(t, profile)

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(ownerID, contactID).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(contactID, ownerID).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT status::text FROM contact_requests`).
		WithArgs(ownerID, contactID).
		WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec(`INSERT INTO contact_requests`).
		WithArgs(pgxmock.AnyArg(), ownerID, contactID, pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	res, err := service.CreateContact(context.Background(), &CreateContactPayload{
		ContactUserId: contactID.String(),
	}, kit.UserId{UuidUserId: ownerID})

	require.NoError(t, err)
	assert.Equal(t, "contact_request_sent", res.Message)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactRequestActions(t *testing.T) {
	ownerID := uuid.New()
	requesterID := uuid.New()

	tests := []struct {
		name    string
		outcome string
		args    []any
		call    func(*contactService) (*rpc_common_modelv1.StatusOkay, error)
		want    string
	}{
		{
			name:    "accept",
			outcome: "accepted",
			args:    []any{requesterID, ownerID},
			call: func(service *contactService) (*rpc_common_modelv1.StatusOkay, error) {
				return service.AcceptContactRequest(context.Background(), &AcceptContactRequestPayload{ContactUserId: requesterID.String()}, kit.UserId{UuidUserId: ownerID})
			},
			want: "contact_request_accepted",
		},
		{
			name:    "reject",
			outcome: "declined",
			args:    []any{requesterID, ownerID},
			call: func(service *contactService) (*rpc_common_modelv1.StatusOkay, error) {
				return service.RejectContactRequest(context.Background(), &RejectContactRequestPayload{ContactUserId: requesterID.String()}, kit.UserId{UuidUserId: ownerID})
			},
			want: "contact_request_declined",
		},
		{
			name:    "undo",
			outcome: "undone",
			args:    []any{ownerID, requesterID},
			call: func(service *contactService) (*rpc_common_modelv1.StatusOkay, error) {
				return service.UndoContactRequest(context.Background(), &UndoContactRequestPayload{ContactUserId: requesterID.String()}, kit.UserId{UuidUserId: ownerID})
			},
			want: "contact_request_undone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mock := newMockContactService(t, &mockContactProfileProvider{})
			mock.ExpectQuery(`WITH (updated|deleted) AS`).
				WithArgs(tt.args...).
				WillReturnRows(pgxmock.NewRows([]string{"outcome"}).AddRow(tt.outcome))

			res, err := tt.call(service)

			require.NoError(t, err)
			assert.Equal(t, tt.want, res.Message)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUpdateAndRemoveContactNickname(t *testing.T) {
	ownerID := uuid.New()
	contactID := uuid.New()

	t.Run("update", func(t *testing.T) {
		service, mock := newMockContactService(t, &mockContactProfileProvider{})
		mock.ExpectQuery(`UPDATE user_contacts`).
			WithArgs(ownerID, contactID, pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"updated"}).AddRow(true))

		nickname := "Renamed"
		res, err := service.UpdateContactNickname(context.Background(), &UpdateContactNicknamePayload{
			ContactUserId: contactID.String(),
			Nickname:      &nickname,
		}, kit.UserId{UuidUserId: ownerID})

		require.NoError(t, err)
		assert.Equal(t, "contact_nickname_updated", res.Message)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("remove", func(t *testing.T) {
		service, mock := newMockContactService(t, &mockContactProfileProvider{})
		mock.ExpectQuery(`UPDATE user_contacts`).
			WithArgs(ownerID, contactID, (*string)(nil)).
			WillReturnRows(pgxmock.NewRows([]string{"updated"}).AddRow(true))

		res, err := service.RemoveContactNickname(context.Background(), &RemoveContactNicknamePayload{
			ContactUserId: contactID.String(),
		}, kit.UserId{UuidUserId: ownerID})

		require.NoError(t, err)
		assert.Equal(t, "contact_nickname_removed", res.Message)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGetContacts_DecryptsNickname(t *testing.T) {
	ownerID := uuid.New()
	contactID := uuid.New()
	now := time.Now()
	profile := &mockContactProfileProvider{
		contactProfiles: map[uuid.UUID]*personal_profile.ContactProfileView{
			contactID: {
				ID:          contactID,
				Name:        "Contact",
				Username:    "CONTACT123",
				ProfileType: "public",
			},
		},
	}
	service, mock := newMockContactService(t, profile)
	nickname := "Saved name"
	encryptedNickname, err := service.EncryptNickname(nickname, ownerID, contactID)
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT\s+uc\.contact_user_id AS id`).
		WithArgs(ownerID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "nickname", "contact_created_at", "contact_updated_at"}).
			AddRow(contactID, &encryptedNickname, now, now))
	mock.ExpectQuery(`SELECT\s+uc\.owner_user_id AS id`).
		WithArgs(ownerID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "nickname", "contact_created_at", "contact_updated_at"}))

	res, err := service.GetContacts(context.Background(), kit.UserId{UuidUserId: ownerID})

	require.NoError(t, err)
	require.Len(t, res.Contacts, 1)
	assert.Equal(t, nickname, res.Contacts[0].GetNickname())
	assert.Empty(t, res.PeopleWhoAddedYou)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactConnectServerCreateContactRejectsEmptyPayload(t *testing.T) {
	userID := uuid.New()
	ctx := context.WithValue(context.Background(), kit.CtxSessionData, kit.SessionData{
		UserID:     userID.String(),
		UUIDUserID: userID,
	})
	server := &contactConnectServer{contactService: &contactService{}}

	_, err := server.CreateContact(ctx, connect.NewRequest(&rpc_personal_contactv1.CreateContactRequest{}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	var processed kit.ProcessedError
	assert.False(t, errors.As(err, &processed))
}

func TestContactRoutesRegister(t *testing.T) {
	e := echo.New()
	group := e.Group("/api/personal")

	Register(group, &contactService{})

	paths := make(map[string]bool)
	for _, route := range e.Router().Routes() {
		paths[route.Path] = true
	}

	for _, path := range []string{
		"/api/personal/contacts/get",
		"/api/personal/contacts/check-existence",
		"/api/personal/contacts/create",
		"/api/personal/contacts/delete",
		"/api/personal/contacts/requests/get",
		"/api/personal/contacts/requests/accept",
		"/api/personal/contacts/requests/reject",
		"/api/personal/contacts/requests/undo",
		"/api/personal/contacts/update-nickname",
		"/api/personal/contacts/remove-nickname",
		"/api/personal/contacts/block",
	} {
		assert.Truef(t, paths[path], "missing route %s", path)
	}
}
