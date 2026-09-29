package personal_contact

import (
	"context"
	"testing"
	"time"

	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/kit"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// After a one-way contact (owner -> contact) is created, the satisfied
// same-direction pending request (requester=owner, receiver=contact) must be
// deleted. The reverse wish is separate and must not be touched.
func TestCreateContact_PublicDirectAddDeletesSatisfiedRequest(t *testing.T) {
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

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(ownerID, contactID).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`INSERT INTO user_contacts`).
		WithArgs(ownerID, contactID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	// One-way cleanup: only the satisfied same-direction request.
	mock.ExpectQuery(`DELETE FROM contact_requests AS cr`).
		WithArgs(ownerID, contactID).
		WillReturnRows(pgxmock.NewRows([]string{"outcome"}).AddRow("undone"))
	mock.ExpectQuery(`SELECT\s+uc\.contact_user_id AS id`).
		WithArgs(ownerID, contactID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "nickname", "contact_created_at", "contact_updated_at"}).
			AddRow(contactID, (*string)(nil), now, now))
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(contactID, ownerID).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

	res, err := service.CreateContact(context.Background(), &CreateContactPayload{
		ContactUserId: contactID.String(),
	}, kit.UserId{UuidUserId: ownerID})

	require.NoError(t, err)
	assert.Equal(t, "public_contact_added", res.Message)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateContact_PersonalDirectAddDeletesSatisfiedRequest(t *testing.T) {
	ownerID := uuid.New()
	contactID := uuid.New()
	now := time.Now()
	profile := &mockContactProfileProvider{
		coreProfile: &personal_profile.UserCoreProfile{ID: contactID, ProfileType: "personal"},
		contactProfiles: map[uuid.UUID]*personal_profile.ContactProfileView{
			contactID: {
				ID:          contactID,
				Name:        "Contact",
				Username:    "CONTACT123",
				ProfileType: "personal",
			},
		},
	}
	service, mock := newMockContactService(t, profile)

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(ownerID, contactID).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(contactID, ownerID).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`INSERT INTO user_contacts`).
		WithArgs(ownerID, contactID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	// One-way cleanup: only the satisfied same-direction request.
	mock.ExpectQuery(`DELETE FROM contact_requests AS cr`).
		WithArgs(ownerID, contactID).
		WillReturnRows(pgxmock.NewRows([]string{"outcome"}).AddRow("undone"))
	mock.ExpectQuery(`SELECT\s+uc\.contact_user_id AS id`).
		WithArgs(ownerID, contactID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "nickname", "contact_created_at", "contact_updated_at"}).
			AddRow(contactID, (*string)(nil), now, now))
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(contactID, ownerID).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))

	res, err := service.CreateContact(context.Background(), &CreateContactPayload{
		ContactUserId: contactID.String(),
	}, kit.UserId{UuidUserId: ownerID})

	require.NoError(t, err)
	assert.Equal(t, "personal_contact_added", res.Message)
	assert.NoError(t, mock.ExpectationsWereMet())
}
