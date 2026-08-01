package personal_contact

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	coreProfile                 *personal_profile.UserCoreProfile
	contactProfiles             map[uuid.UUID]*personal_profile.ContactProfileView
	blockListProfiles           map[uuid.UUID]*personal_profile.ContactProfileView
	userBlocks                  []personal_profile.UserBlock
	getUserBlocksErr            error
	getUserBlocksCalls          int
	lastBlockerID               uuid.UUID
	getContactableProfilesCalls int
	lastContactableViewerID     uuid.UUID
	lastContactableTargetIDs    []uuid.UUID
	getBlockListProfilesErr     error
	getBlockListProfilesCalls   int
	lastBlockListViewerID       uuid.UUID
	lastBlockListTargetIDs      []uuid.UUID
	blockStatus                 *personal_profile.BlockStatusResult
	isEitherBlocked             int32
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

func (m *mockContactProfileProvider) GetContactableProfilesForViewer(_ context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error) {
	m.getContactableProfilesCalls++
	m.lastContactableViewerID = viewerID
	m.lastContactableTargetIDs = append([]uuid.UUID(nil), targetIDs...)
	if m.contactProfiles == nil {
		return map[uuid.UUID]*personal_profile.ContactProfileView{}, nil
	}
	return m.contactProfiles, nil
}

func (m *mockContactProfileProvider) GetBlockListProfilesForViewer(_ context.Context, viewerID uuid.UUID, targetIDs []uuid.UUID) (map[uuid.UUID]*personal_profile.ContactProfileView, error) {
	m.getBlockListProfilesCalls++
	m.lastBlockListViewerID = viewerID
	m.lastBlockListTargetIDs = append([]uuid.UUID(nil), targetIDs...)
	if m.getBlockListProfilesErr != nil {
		return nil, m.getBlockListProfilesErr
	}
	if m.blockListProfiles == nil {
		return map[uuid.UUID]*personal_profile.ContactProfileView{}, nil
	}
	return m.blockListProfiles, nil
}

func (m *mockContactProfileProvider) FindContactableUserByUsername(context.Context, uuid.UUID, string) (*personal_profile.ContactLookupResult, error) {
	return &personal_profile.ContactLookupResult{Exists: false}, nil
}

func (m *mockContactProfileProvider) CreateUserBlock(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func (m *mockContactProfileProvider) GetUserBlocks(_ context.Context, blockerID uuid.UUID) ([]personal_profile.UserBlock, error) {
	m.getUserBlocksCalls++
	m.lastBlockerID = blockerID
	if m.getUserBlocksErr != nil {
		return nil, m.getUserBlocksErr
	}
	return m.userBlocks, nil
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

func TestGetBlocks_Empty(t *testing.T) {
	blockerID := uuid.New()
	profile := &mockContactProfileProvider{}
	service, _ := newMockContactService(t, profile)

	res, err := service.GetBlocks(context.Background(), kit.UserId{UuidUserId: blockerID})

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.BlockedUsers)
	assert.Equal(t, 1, profile.getUserBlocksCalls)
	assert.Equal(t, blockerID, profile.lastBlockerID)
	assert.Zero(t, profile.getContactableProfilesCalls)
	assert.Zero(t, profile.getBlockListProfilesCalls)
}

func TestGetBlocks_PropagatesBlockRowError(t *testing.T) {
	blockerID := uuid.New()
	wantErr := errors.New("block rows unavailable")
	profile := &mockContactProfileProvider{getUserBlocksErr: wantErr}
	service, _ := newMockContactService(t, profile)

	res, err := service.GetBlocks(context.Background(), kit.UserId{UuidUserId: blockerID})

	assert.Nil(t, res)
	require.ErrorIs(t, err, wantErr)
	assert.Zero(t, profile.getBlockListProfilesCalls)
}

func TestGetBlocks_PropagatesProfileEnrichmentError(t *testing.T) {
	blockerID := uuid.New()
	blockedID := uuid.New()
	wantErr := errors.New("block-list profiles unavailable")
	profile := &mockContactProfileProvider{
		userBlocks:              []personal_profile.UserBlock{{BlockedUserID: blockedID, CreatedAt: time.Now()}},
		getBlockListProfilesErr: wantErr,
	}
	service, _ := newMockContactService(t, profile)

	res, err := service.GetBlocks(context.Background(), kit.UserId{UuidUserId: blockerID})

	assert.Nil(t, res)
	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, 1, profile.getBlockListProfilesCalls)
}

func TestGetBlocks_EnrichesProfilesAndPreservesBlockTimes(t *testing.T) {
	blockerID := uuid.New()
	firstBlockedID := uuid.New()
	secondBlockedID := uuid.New()
	firstBlockedAt := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	secondBlockedAt := time.Date(2025, time.December, 31, 12, 0, 0, 0, time.UTC)
	bio := "First bio"
	avatarURL := "https://example.com/avatar"
	avatarFileID := "avatar-file-id"
	profile := &mockContactProfileProvider{
		userBlocks: []personal_profile.UserBlock{
			{BlockedUserID: firstBlockedID, CreatedAt: firstBlockedAt},
			{BlockedUserID: secondBlockedID, CreatedAt: secondBlockedAt},
		},
		blockListProfiles: map[uuid.UUID]*personal_profile.ContactProfileView{
			firstBlockedID: {
				ID:           firstBlockedID,
				Name:         "First User",
				Username:     "FIRST123",
				Bio:          &bio,
				AvatarURL:    &avatarURL,
				AvatarFileId: &avatarFileID,
				ProfileType:  "public",
			},
			secondBlockedID: {
				ID:          secondBlockedID,
				Name:        "Second User",
				Username:    "SECOND123",
				ProfileType: "personal",
			},
		},
	}
	service, _ := newMockContactService(t, profile)

	res, err := service.GetBlocks(context.Background(), kit.UserId{UuidUserId: blockerID})

	require.NoError(t, err)
	require.Len(t, res.BlockedUsers, 2)
	assert.Equal(t, firstBlockedID.String(), res.BlockedUsers[0].Id)
	assert.Equal(t, "First User", res.BlockedUsers[0].Name)
	assert.Equal(t, "FIRST123", res.BlockedUsers[0].Username)
	assert.Equal(t, &bio, res.BlockedUsers[0].Bio)
	assert.Equal(t, &avatarURL, res.BlockedUsers[0].AvatarUrl)
	assert.Equal(t, &avatarFileID, res.BlockedUsers[0].AvatarFileId)
	assert.Equal(t, "public", res.BlockedUsers[0].ProfileType)
	assert.Equal(t, firstBlockedAt, res.BlockedUsers[0].BlockedAt.AsTime())
	assert.Equal(t, secondBlockedID.String(), res.BlockedUsers[1].Id)
	assert.Equal(t, secondBlockedAt, res.BlockedUsers[1].BlockedAt.AsTime())
	assert.Nil(t, res.BlockedUsers[1].Bio)
	assert.Nil(t, res.BlockedUsers[1].AvatarUrl)
	assert.Nil(t, res.BlockedUsers[1].AvatarFileId)
	assert.Equal(t, blockerID, profile.lastBlockerID)
	assert.Equal(t, blockerID, profile.lastBlockListViewerID)
	assert.Equal(t, []uuid.UUID{firstBlockedID, secondBlockedID}, profile.lastBlockListTargetIDs)
}

func TestGetBlocks_ReciprocalBlockKeepsIdentityAndHidesSensitiveFields(t *testing.T) {
	blockerID := uuid.New()
	blockedID := uuid.New()
	blockedAt := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	profile := &mockContactProfileProvider{
		userBlocks: []personal_profile.UserBlock{
			{BlockedUserID: blockedID, CreatedAt: blockedAt},
		},
		blockListProfiles: map[uuid.UUID]*personal_profile.ContactProfileView{
			blockedID: {
				ID:          blockedID,
				Name:        "Reciprocal User",
				Username:    "RECIPROCAL1",
				ProfileType: "public",
			},
		},
	}
	service, _ := newMockContactService(t, profile)

	res, err := service.GetBlocks(context.Background(), kit.UserId{UuidUserId: blockerID})

	require.NoError(t, err)
	require.Len(t, res.BlockedUsers, 1)
	assert.Equal(t, blockedID.String(), res.BlockedUsers[0].Id)
	assert.Equal(t, "Reciprocal User", res.BlockedUsers[0].Name)
	assert.Equal(t, "RECIPROCAL1", res.BlockedUsers[0].Username)
	assert.Equal(t, "public", res.BlockedUsers[0].ProfileType)
	assert.Equal(t, blockedAt, res.BlockedUsers[0].BlockedAt.AsTime())
	assert.Nil(t, res.BlockedUsers[0].Bio)
	assert.Nil(t, res.BlockedUsers[0].AvatarUrl)
	assert.Nil(t, res.BlockedUsers[0].AvatarFileId)
	assert.Equal(t, 1, profile.getBlockListProfilesCalls)
	assert.Equal(t, blockerID, profile.lastBlockListViewerID)
	assert.Equal(t, []uuid.UUID{blockedID}, profile.lastBlockListTargetIDs)
}

func TestGetBlocks_OmitsProfilesExcludedByEnrichment(t *testing.T) {
	blockerID := uuid.New()
	visibleID := uuid.New()
	omittedID := uuid.New()
	profile := &mockContactProfileProvider{
		userBlocks: []personal_profile.UserBlock{
			{BlockedUserID: visibleID, CreatedAt: time.Now()},
			{BlockedUserID: omittedID, CreatedAt: time.Now()},
		},
		blockListProfiles: map[uuid.UUID]*personal_profile.ContactProfileView{
			visibleID: {ID: visibleID, Name: "Visible", Username: "VISIBLE123", ProfileType: "public"},
		},
	}
	service, _ := newMockContactService(t, profile)

	res, err := service.GetBlocks(context.Background(), kit.UserId{UuidUserId: blockerID})

	require.NoError(t, err)
	require.Len(t, res.BlockedUsers, 1)
	assert.Equal(t, visibleID.String(), res.BlockedUsers[0].Id)
	assert.Equal(t, []uuid.UUID{visibleID, omittedID}, profile.lastBlockListTargetIDs)
}

func TestContactHTTPGetBlocksUsesAuthenticatedUser(t *testing.T) {
	userID := uuid.New()
	blockedID := uuid.New()
	profile := &mockContactProfileProvider{
		userBlocks: []personal_profile.UserBlock{{BlockedUserID: blockedID, CreatedAt: time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)}},
		blockListProfiles: map[uuid.UUID]*personal_profile.ContactProfileView{
			blockedID: {ID: blockedID, Name: "Blocked User", Username: "BLOCKED1", ProfileType: "public"},
		},
	}
	service, _ := newMockContactService(t, profile)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/personal/contacts/blocks/get", nil)
	recorder := httptest.NewRecorder()
	ctx := e.NewContext(req, recorder)
	ctx.Set("userId", userID.String())
	ctx.Set("uuidUserId", userID)

	err := newContactHandler(service).GetBlocks(ctx)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		BlockedUsers []struct {
			ID string `json:"id"`
		} `json:"blockedUsers"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Len(t, payload.BlockedUsers, 1)
	assert.Equal(t, blockedID.String(), payload.BlockedUsers[0].ID)
	assert.Equal(t, userID, profile.lastBlockerID)
}

func TestContactHTTPGetBlocksRequiresAuthentication(t *testing.T) {
	profile := &mockContactProfileProvider{}
	service, _ := newMockContactService(t, profile)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/personal/contacts/blocks/get", nil)
	recorder := httptest.NewRecorder()
	ctx := e.NewContext(req, recorder)

	err := newContactHandler(service).GetBlocks(ctx)

	var processedErr kit.ProcessedError
	require.ErrorAs(t, err, &processedErr)
	assert.Equal(t, http.StatusUnauthorized, processedErr.Status())
	assert.Zero(t, profile.getUserBlocksCalls)
}

func TestContactConnectServerGetBlocksUsesAuthenticatedUser(t *testing.T) {
	userID := uuid.New()
	blockedID := uuid.New()
	profile := &mockContactProfileProvider{
		userBlocks: []personal_profile.UserBlock{{BlockedUserID: blockedID, CreatedAt: time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)}},
		blockListProfiles: map[uuid.UUID]*personal_profile.ContactProfileView{
			blockedID: {ID: blockedID, Name: "Blocked User", Username: "BLOCKED1", ProfileType: "public"},
		},
	}
	service, _ := newMockContactService(t, profile)
	server := &contactConnectServer{contactService: service}
	ctx := context.WithValue(context.Background(), kit.CtxSessionData, kit.SessionData{
		UserID:     userID.String(),
		UUIDUserID: userID,
	})

	res, err := server.GetBlocks(ctx, connect.NewRequest(&rpc_personal_contactv1.GetBlocksRequest{}))

	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Msg.BlockedUsers, 1)
	assert.Equal(t, blockedID.String(), res.Msg.BlockedUsers[0].Id)
	assert.Equal(t, userID, profile.lastBlockerID)
}

func TestContactConnectServerGetBlocksRequiresAuthentication(t *testing.T) {
	profile := &mockContactProfileProvider{}
	service, _ := newMockContactService(t, profile)
	server := &contactConnectServer{contactService: service}

	res, err := server.GetBlocks(context.Background(), connect.NewRequest(&rpc_personal_contactv1.GetBlocksRequest{}))

	assert.Nil(t, res)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	assert.Zero(t, profile.getUserBlocksCalls)
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
		"/api/personal/contacts/blocks/get",
		"/api/personal/contacts/check-existence",
		"/api/personal/contacts/create",
		"/api/personal/contacts/delete",
		"/api/personal/contacts/requests/get",
		"/api/personal/contacts/requests/accept",
		"/api/personal/contacts/requests/reject",
		"/api/personal/contacts/requests/undo",
		"/api/personal/contacts/update-nickname",
		"/api/personal/contacts/remove-nickname",
		"/api/personal/contacts/blocks/create",
	} {
		assert.Truef(t, paths[path], "missing route %s", path)
	}
}
