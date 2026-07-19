package personal_contact

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"chatbasket-api/internal/modules/core/core_auth"
	"chatbasket-api/internal/modules/personal/personal_profile"
	"chatbasket-api/internal/platform/clients"
	"chatbasket-api/internal/platform/kit"
	"chatbasket-api/internal/platform/services"

	rpc_common_modelv1 "chatbasket-api/gen/proto/common/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupContactDeleteIntegrationDB(t *testing.T) (*pgxpool.Pool, *contactService) {
	_ = godotenv.Load("../../../../.env")
	_ = godotenv.Load("../../../../../.env")

	dsn := os.Getenv("DatabaseURLTesting")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL_PG_TESTING")
	}
	if dsn == "" {
		t.Skip("DatabaseURLTesting/DATABASE_URL_PG_TESTING not set")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err, "failed to connect testing db")
	t.Cleanup(func() { pool.Close() })

	globalSvc := services.NewGlobalService("https://chatbasket.live")
	authSvc := core_auth.NewAuthService(globalSvc, pool, []byte("test-secret"))
	profileSvc := personal_profile.NewProfileService(globalSvc, pool, authSvc, []byte("test-username-key-32bytes-long!!"), nil, (*clients.R2ClientPool)(nil))
	contactSvc := NewContactService(globalSvc, pool, profileSvc, []byte("test-username-key-32bytes-long!!"), []byte("test-contact-key-32bytes-long!!"))

	return pool, contactSvc
}

func createContactTestUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	ctx := context.Background()
	userID := uuid.New()
	email := userID.String() + "@test.com"

	hmacHex := fmt.Sprintf("%064x", userID)[0:64]

	_, err := pool.Exec(ctx,
		"INSERT INTO auth_users (id, email, password_hash, name, is_email_verified, created_at, updated_at, keys_revision) VALUES ($1, $2, 'hash', 'Test User', true, now(), now(), 0)",
		userID, email)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		"INSERT INTO users (id, name, profile_type, hmac_sha256_hex_username, b64_cipher_chacha20poly1305_username, created_at, updated_at) VALUES ($1, 'Test User', 'public', $2, $3, now(), now())",
		userID, hmacHex, "encrypted-"+userID.String())
	require.NoError(t, err)

	return userID
}

func addContact(t *testing.T, pool *pgxpool.Pool, ownerID, contactID uuid.UUID) {
	ctx := context.Background()
	_, err := pool.Exec(ctx,
		"INSERT INTO user_contacts (owner_user_id, contact_user_id, created_at, updated_at) VALUES ($1, $2, now(), now()) ON CONFLICT DO NOTHING",
		ownerID, contactID)
	require.NoError(t, err)
}

func countContacts(t *testing.T, pool *pgxpool.Pool, ownerID, contactID uuid.UUID) int {
	ctx := context.Background()
	var count int
	err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM user_contacts WHERE owner_user_id = $1 AND contact_user_id = $2",
		ownerID, contactID).Scan(&count)
	require.NoError(t, err)
	return count
}

func TestDeleteContact_Integration_DeleteUnblocked(t *testing.T) {
	pool, contactSvc := setupContactDeleteIntegrationDB(t)
	ctx := context.Background()

	owner := createContactTestUser(t, pool)
	contact := createContactTestUser(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", owner) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", contact) })

	addContact(t, pool, owner, contact)

	res, err := contactSvc.DeleteContact(ctx, &DeleteContactPayload{ContactUserId: []string{contact.String()}}, kit.UserId{UuidUserId: owner})
	require.NoError(t, err)
	assert.True(t, res.Status)
	assert.Equal(t, 0, countContacts(t, pool, owner, contact))
}

func TestDeleteContact_Integration_SkipAdminBlockedTarget(t *testing.T) {
	pool, contactSvc := setupContactDeleteIntegrationDB(t)
	ctx := context.Background()

	owner := createContactTestUser(t, pool)
	blocked := createContactTestUser(t, pool)
	normal := createContactTestUser(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", owner) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", blocked) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", normal) })

	addContact(t, pool, owner, blocked)
	addContact(t, pool, owner, normal)
	_, err := pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id = $1", blocked)
	require.NoError(t, err)

	res, err := contactSvc.DeleteContact(ctx, &DeleteContactPayload{ContactUserId: []string{blocked.String(), normal.String()}}, kit.UserId{UuidUserId: owner})
	require.NoError(t, err)
	assert.True(t, res.Status)
	assert.Equal(t, 1, countContacts(t, pool, owner, blocked))
	assert.Equal(t, 0, countContacts(t, pool, owner, normal))
}

func TestDeleteContact_Integration_SkipUserBlockedTarget(t *testing.T) {
	pool, contactSvc := setupContactDeleteIntegrationDB(t)
	ctx := context.Background()

	owner := createContactTestUser(t, pool)
	blocked := createContactTestUser(t, pool)
	normal := createContactTestUser(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", owner) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", blocked) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", normal) })

	addContact(t, pool, owner, blocked)
	addContact(t, pool, owner, normal)
	_, err := pool.Exec(ctx,
		"INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id) VALUES ($1, $2, $3)",
		uuid.New(), owner, blocked)
	require.NoError(t, err)

	res, err := contactSvc.DeleteContact(ctx, &DeleteContactPayload{ContactUserId: []string{blocked.String(), normal.String()}}, kit.UserId{UuidUserId: owner})
	require.NoError(t, err)
	assert.True(t, res.Status)
	assert.Equal(t, 1, countContacts(t, pool, owner, blocked))
	assert.Equal(t, 0, countContacts(t, pool, owner, normal))
}

func TestDeleteContact_Integration_PartialDeletion(t *testing.T) {
	pool, contactSvc := setupContactDeleteIntegrationDB(t)
	ctx := context.Background()

	owner := createContactTestUser(t, pool)
	normal := createContactTestUser(t, pool)
	adminBlocked := createContactTestUser(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", owner) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", normal) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", adminBlocked) })

	addContact(t, pool, owner, normal)
	addContact(t, pool, owner, adminBlocked)

	_, err := pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id = $1", adminBlocked)
	require.NoError(t, err)

	res, err := contactSvc.DeleteContact(ctx, &DeleteContactPayload{ContactUserId: []string{normal.String(), adminBlocked.String()}}, kit.UserId{UuidUserId: owner})
	require.NoError(t, err)
	assert.True(t, res.Status)
	assert.Equal(t, 0, countContacts(t, pool, owner, normal))
	assert.Equal(t, 1, countContacts(t, pool, owner, adminBlocked))
}

func TestDeleteContact_Integration_RequesterAdminBlocked(t *testing.T) {
	pool, contactSvc := setupContactDeleteIntegrationDB(t)
	ctx := context.Background()

	owner := createContactTestUser(t, pool)
	contact := createContactTestUser(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", owner) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", contact) })

	addContact(t, pool, owner, contact)
	_, err := pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id = $1", owner)
	require.NoError(t, err)

	_, err = contactSvc.DeleteContact(ctx, &DeleteContactPayload{ContactUserId: []string{contact.String()}}, kit.UserId{UuidUserId: owner})
	require.Error(t, err)
	var processed kit.ProcessedError
	require.True(t, errors.As(err, &processed))
	assert.Equal(t, http.StatusForbidden, processed.Status())
	assert.Equal(t, "self_admin_blocked", processed.Error())
}

func TestDeleteContact_Integration_AllBlocked(t *testing.T) {
	pool, contactSvc := setupContactDeleteIntegrationDB(t)
	ctx := context.Background()

	owner := createContactTestUser(t, pool)
	blocked1 := createContactTestUser(t, pool)
	blocked2 := createContactTestUser(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", owner) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", blocked1) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", blocked2) })

	addContact(t, pool, owner, blocked1)
	addContact(t, pool, owner, blocked2)

	_, err := pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id IN ($1, $2)", blocked1, blocked2)
	require.NoError(t, err)

	_, err = contactSvc.DeleteContact(ctx, &DeleteContactPayload{ContactUserId: []string{blocked1.String(), blocked2.String()}}, kit.UserId{UuidUserId: owner})
	require.Error(t, err)
	var processed kit.ProcessedError
	require.True(t, errors.As(err, &processed))
	assert.Equal(t, http.StatusForbidden, processed.Status())
	assert.Equal(t, "all_contacts_blocked", processed.Error())
}

func TestDeleteContact_Integration_EmptyPayload(t *testing.T) {
	_, contactSvc := setupContactDeleteIntegrationDB(t)

	_, err := contactSvc.DeleteContact(context.Background(), &DeleteContactPayload{ContactUserId: []string{}}, kit.UserId{UuidUserId: uuid.New()})
	require.Error(t, err)
	var processed kit.ProcessedError
	require.True(t, errors.As(err, &processed))
	assert.Equal(t, http.StatusBadRequest, processed.Status())
}

func TestDeleteContact_Integration_SelfDelete(t *testing.T) {
	pool, contactSvc := setupContactDeleteIntegrationDB(t)
	ctx := context.Background()

	owner := createContactTestUser(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", owner) })

	_, err := contactSvc.DeleteContact(ctx, &DeleteContactPayload{ContactUserId: []string{owner.String()}}, kit.UserId{UuidUserId: owner})
	require.Error(t, err)
	var processed kit.ProcessedError
	require.True(t, errors.As(err, &processed))
	assert.Equal(t, http.StatusConflict, processed.Status())
}

func TestDeleteContact_Integration_DeduplicatesBeforeBlockCheck(t *testing.T) {
	pool, contactSvc := setupContactDeleteIntegrationDB(t)
	ctx := context.Background()

	owner := createContactTestUser(t, pool)
	normal := createContactTestUser(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", owner) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", normal) })

	addContact(t, pool, owner, normal)

	// Pass the same contact twice; DeleteContact should deduplicate and issue one batch call.
	res, err := contactSvc.DeleteContact(ctx, &DeleteContactPayload{ContactUserId: []string{normal.String(), normal.String()}}, kit.UserId{UuidUserId: owner})
	require.NoError(t, err)
	assert.True(t, res.Status)
	assert.Equal(t, 0, countContacts(t, pool, owner, normal))
}

func TestDeleteContact_Integration_SingleBlockedDetail(t *testing.T) {
	pool, contactSvc := setupContactDeleteIntegrationDB(t)
	ctx := context.Background()

	owner := createContactTestUser(t, pool)
	blocked := createContactTestUser(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", owner) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", blocked) })

	addContact(t, pool, owner, blocked)

	// Block the user
	_, err := pool.Exec(ctx,
		"INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id) VALUES ($1, $2, $3)",
		uuid.New(), owner, blocked)
	require.NoError(t, err)

	_, err = contactSvc.DeleteContact(ctx, &DeleteContactPayload{ContactUserId: []string{blocked.String()}}, kit.UserId{UuidUserId: owner})
	require.Error(t, err)

	var detailed kit.DetailedProcessedError
	require.True(t, errors.As(err, &detailed))
	assert.Equal(t, http.StatusForbidden, detailed.Status())
	assert.Equal(t, "blocked", detailed.Error())

	flags, ok := detailed.Details().(*rpc_common_modelv1.BlockStatusFlags)
	require.True(t, ok, "details should be *rpc_common_modelv1.BlockStatusFlags")
	assert.True(t, flags.IsTargetUserBlockedByRequester)
	assert.False(t, flags.IsRequesterUserBlockedByTarget)
}

