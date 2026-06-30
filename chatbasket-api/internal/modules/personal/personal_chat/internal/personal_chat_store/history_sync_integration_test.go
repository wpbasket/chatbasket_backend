package personal_chat_store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDB(t *testing.T) (*pgxpool.Pool, *personal_chat_store.Queries) {
	_ = godotenv.Load("../../../../../../.env")
	_ = godotenv.Load("../../../../../../../.env")
	_ = godotenv.Load("../../../../../../../../.env")

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

	queries := personal_chat_store.New(pool)
	return pool, queries
}

func createUserAndSession(t *testing.T, pool *pgxpool.Pool) (uuid.UUID, uuid.UUID) {
	ctx := context.Background()
	userID := uuid.New()
	email := userID.String() + "@test.com"

	_, err := pool.Exec(ctx,
		"INSERT INTO auth_users (id, email, password_hash, name, is_email_verified, created_at, updated_at, keys_revision) VALUES ($1, $2, 'hash', 'Test User', true, now(), now(), 0)",
		userID, email)
	require.NoError(t, err)

	hmacStr := (userID.String() + userID.String())[:64]
	_, err = pool.Exec(ctx,
		"INSERT INTO users (id, name, profile_type, hmac_sha256_hex_username, b64_cipher_chacha20poly1305_username, created_at, updated_at) VALUES ($1, 'Test User', 'personal', $2, 'dummy-b64', now(), now())",
		userID, hmacStr)
	require.NoError(t, err)

	sessionID := uuid.New()
	publicKey := "test-public-key-44-chars-base64-encoded!!!"
	_, err = pool.Exec(ctx,
		"INSERT INTO sessions (id, auth_user_id, token_hash, e2ee_public_key, expires_at, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, now(), now())",
		sessionID, userID, "token-hash-"+sessionID.String(), &publicKey, time.Now().Add(24*time.Hour))
	require.NoError(t, err)

	return userID, sessionID
}

func TestHistorySyncStore_Integration(t *testing.T) {
	pool, queries := setupDB(t)
	ctx := context.Background()

	userID, sessionID := createUserAndSession(t, pool)

	reqID := uuid.New()
	expiresAt := time.Now().Add(1 * time.Hour)

	// 1. Upsert request
	chatsCipher := []byte(`"encrypted-chats"`)
	returnedID, err := queries.UpsertHistorySync(ctx, personal_chat_store.UpsertHistorySyncParams{
		ID:        reqID,
		UserID:    userID,
		SessionID: sessionID,
		ChatsJson: chatsCipher,
		ExpiresAt: expiresAt,
	})
	require.NoError(t, err)
	assert.Equal(t, reqID, returnedID)

	// 2. Get pending
	pending, err := queries.GetPendingHistorySyncForUser(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, pending, 1)
	assert.Equal(t, reqID, pending[0].ID)
	assert.Equal(t, chatsCipher, pending[0].ChatsJson)

	// 3. Get Meta
	meta, err := queries.GetHistorySyncMeta(ctx, reqID)
	require.NoError(t, err)
	assert.Equal(t, userID, meta.UserID)
	assert.Equal(t, sessionID, meta.SessionID)

	// 4. Upload payload
	payloadCipher := []byte(`"encrypted-payload"`)
	rows, err := queries.UploadHistorySyncPayload(ctx, personal_chat_store.UploadHistorySyncPayloadParams{
		Payload: payloadCipher,
		ID:      reqID,
		UserID:  userID,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)

	// 5. Get for download
	downloadPayload, err := queries.GetHistorySyncForDownload(ctx, personal_chat_store.GetHistorySyncForDownloadParams{
		ID:        reqID,
		SessionID: sessionID,
	})
	require.NoError(t, err)
	assert.Equal(t, payloadCipher, downloadPayload)

	// 6. Check it still exists after download (SQLC only SELECTs)
	_, err = queries.GetHistorySyncMeta(ctx, reqID)
	assert.NoError(t, err)
}
