package personal_contact

import (
	"context"
	"sync"
	"testing"

	"chatbasket-api/internal/platform/kit"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactRequest_Integration_AcceptCreatesContact(t *testing.T) {
	pool, contactSvc := setupContactDeleteIntegrationDB(t)
	ctx := context.Background()

	requesterID := createContactTestUser(t, pool)
	receiverID := createContactTestUser(t, pool)
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requesterID) })
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", receiverID) })

	_, err := pool.Exec(ctx, `
		INSERT INTO contact_requests (id, requester_user_id, receiver_user_id, status)
		VALUES ($1, $2, $3, 'pending')
	`, uuid.New(), requesterID, receiverID)
	require.NoError(t, err)

	res, err := contactSvc.AcceptContactRequest(ctx, &AcceptContactRequestPayload{
		ContactUserId: requesterID.String(),
	}, kit.UserId{UuidUserId: receiverID})

	require.NoError(t, err)
	assert.True(t, res.Status)
	assert.Equal(t, "contact_request_accepted", res.Message)

	var requestStatus string
	err = pool.QueryRow(ctx, `
		SELECT status
		FROM contact_requests
		WHERE requester_user_id = $1 AND receiver_user_id = $2
	`, requesterID, receiverID).Scan(&requestStatus)
	require.ErrorIs(t, err, pgx.ErrNoRows)

	var storedNickname *string
	err = pool.QueryRow(ctx, `
		SELECT nickname
		FROM user_contacts
		WHERE owner_user_id = $1 AND contact_user_id = $2
	`, requesterID, receiverID).Scan(&storedNickname)
	require.NoError(t, err)
	assert.Nil(t, storedNickname)

	// Verify nickname can be added via UpdateContactNickname after contact is created
	nickname := "My Friend"
	updateRes, err := contactSvc.UpdateContactNickname(ctx, &UpdateContactNicknamePayload{
		ContactUserId: receiverID.String(),
		Nickname:      &nickname,
	}, kit.UserId{UuidUserId: requesterID})
	require.NoError(t, err)
	assert.True(t, updateRes.Status)

	err = pool.QueryRow(ctx, `
		SELECT nickname
		FROM user_contacts
		WHERE owner_user_id = $1 AND contact_user_id = $2
	`, requesterID, receiverID).Scan(&storedNickname)
	require.NoError(t, err)
	require.NotNil(t, storedNickname)

	decryptedNickname, err := contactSvc.DecryptNickname(storedNickname, requesterID, receiverID)
	require.NoError(t, err)
	require.NotNil(t, decryptedNickname)
	assert.Equal(t, nickname, *decryptedNickname)
}

func TestContactRequest_Integration_RejectDeletesRequest(t *testing.T) {
	pool, contactSvc := setupContactDeleteIntegrationDB(t)
	ctx := context.Background()

	requesterID := createContactTestUser(t, pool)
	receiverID := createContactTestUser(t, pool)
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requesterID) })
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", receiverID) })

	_, err := pool.Exec(ctx, `
		INSERT INTO contact_requests (id, requester_user_id, receiver_user_id, status)
		VALUES ($1, $2, $3, 'pending')
	`, uuid.New(), requesterID, receiverID)
	require.NoError(t, err)

	res, err := contactSvc.RejectContactRequest(ctx, &RejectContactRequestPayload{
		ContactUserId: requesterID.String(),
	}, kit.UserId{UuidUserId: receiverID})

	require.NoError(t, err)
	assert.True(t, res.Status)
	assert.Equal(t, "contact_request_declined", res.Message)

	var requestStatus string
	err = pool.QueryRow(ctx, `
		SELECT status
		FROM contact_requests
		WHERE requester_user_id = $1 AND receiver_user_id = $2
	`, requesterID, receiverID).Scan(&requestStatus)
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestContactRequest_Integration_ConcurrentCreateKeepsOnePendingRequest(t *testing.T) {
	pool, contactSvc := setupContactDeleteIntegrationDB(t)
	ctx := context.Background()

	requesterID := createContactTestUser(t, pool)
	receiverID := createContactTestUser(t, pool)
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requesterID) })
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", receiverID) })

	_, err := pool.Exec(ctx, "UPDATE users SET profile_type = 'personal' WHERE id = $1", receiverID)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			res, err := contactSvc.CreateContact(ctx, &CreateContactPayload{
				ContactUserId: receiverID.String(),
			}, kit.UserId{UuidUserId: requesterID})
			if err == nil && res.Message != "contact_request_sent" && res.Message != "pending_request_exists" {
				err = assert.AnError
			}
			results <- err
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	for err := range results {
		require.NoError(t, err)
	}

	var pendingCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM contact_requests
		WHERE requester_user_id = $1
		  AND receiver_user_id = $2
		  AND status = 'pending'
	`, requesterID, receiverID).Scan(&pendingCount)
	require.NoError(t, err)
	assert.Equal(t, 1, pendingCount)
}
