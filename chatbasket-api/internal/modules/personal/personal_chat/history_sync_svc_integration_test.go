package personal_chat

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	rpc_personal_chatv1 "chatbasket-api/gen/proto/personal/personal_chat"
	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"
	"chatbasket-api/internal/platform/kit"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupIntegrationDB(t *testing.T) *pgxpool.Pool {
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
	return pool
}

type historySyncMockAuthProvider struct {
	primarySessionID uuid.UUID
	publicKey        string
	primaryErr       error
	keyErr           error
}

func (m *historySyncMockAuthProvider) IsSessionCentral(ctx context.Context, userID uuid.UUID, sessionToken string) (bool, error) {
	return false, nil
}
func (m *historySyncMockAuthProvider) GetUserPrimarySessionID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return m.primarySessionID, m.primaryErr
}
func (m *historySyncMockAuthProvider) GetSessionE2EEPublicKey(ctx context.Context, sessionID uuid.UUID) (*string, error) {
	if m.keyErr != nil {
		return nil, m.keyErr
	}
	if m.publicKey == "" {
		return nil, nil
	}
	return &m.publicKey, nil
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
	publicKey := "test-public-key-44-chars-base-64-encoded-key"
	_, err = pool.Exec(ctx,
		"INSERT INTO sessions (id, auth_user_id, token_hash, e2ee_public_key, expires_at, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, now(), now())",
		sessionID, userID, "token-hash-"+sessionID.String(), &publicKey, time.Now().Add(24*time.Hour))
	require.NoError(t, err)

	return userID, sessionID
}

func TestHistorySync_FullIntegration_EdgeCases(t *testing.T) {
	pool := setupIntegrationDB(t)
	ctx := context.Background()

	userID, sessionID := createUserAndSession(t, pool)
	primarySessionID := sessionID
	pubKey := "test-public-key-44-chars-base-64-encoded-key"

	mockAuth := &historySyncMockAuthProvider{
		primarySessionID: primarySessionID,
		publicKey:        pubKey,
	}

	// Create service
	svc := &chatService{
		PostgresQuerier: personal_chat_store.New(pool),
		AuthProvider:    mockAuth,
	}

	chatsCipher := "\"encrypted-chats\""

	// 1. RequestHistorySync Success
	reqID, pSession, key, err := svc.RequestHistorySync(ctx, userID, sessionID, chatsCipher, pubKey)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, reqID)
	assert.Equal(t, primarySessionID, pSession)
	assert.Equal(t, pubKey, key)

	// 2. UploadHistorySync Edge Cases
	// a. Not your request
	wrongUserID := uuid.New()
	_, err = svc.UploadHistorySync(ctx, wrongUserID, reqID, "\"payload\"")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Not your request")
	// 2. UploadHistorySync - Not your request
	otherUserID := uuid.New()
	_, err = svc.UploadHistorySync(ctx, otherUserID, reqID, "payload_cipher_test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Not your request")

	// 3. UploadHistorySync - Success
	payloadCipher := "{\"type\":\"text\",\"text\":\"encrypted-messages\"}"
	retSessionID, err := svc.UploadHistorySync(ctx, userID, reqID, payloadCipher)
	require.NoError(t, err)
	assert.Equal(t, sessionID, retSessionID)

	// 4. DownloadHistorySync - Success
	downPayload, err := svc.DownloadHistorySync(ctx, userID, sessionID, reqID)
	require.NoError(t, err)
	if assert.NotNil(t, downPayload) {
		assert.JSONEq(t, payloadCipher, *downPayload)
	}

	// 5. AcknowledgeHistorySync - Success
	err = svc.AcknowledgeHistorySync(ctx, userID, sessionID, reqID)
	require.NoError(t, err)

	// Verify row is deleted from database
	_, err = svc.DownloadHistorySync(ctx, userID, sessionID, reqID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired or not found")
}

func TestHistorySync_Timeouts(t *testing.T) {
	pool := setupIntegrationDB(t)
	ctx := context.Background()

	userID, sessionID := createUserAndSession(t, pool)

	mockAuth := &historySyncMockAuthProvider{
		primarySessionID: sessionID,
		publicKey:        "test-public-key-44-chars-base-64-encoded-key",
	}

	svc := &chatService{
		PostgresQuerier: personal_chat_store.New(pool),
		AuthProvider:    mockAuth,
	}

	// Insert expired record manually to bypass the service's TTL setting
	reqID, _ := uuid.NewV7()
	_, err := svc.PostgresQuerier.UpsertHistorySync(ctx, personal_chat_store.UpsertHistorySyncParams{
		ID:        reqID,
		UserID:    userID,
		SessionID: sessionID,
		ChatsJson: []byte("\"chats\""),
		ExpiresAt: time.Now().Add(-1 * time.Minute), // Expired!
	})
	require.NoError(t, err)

	_, err = svc.UploadHistorySync(ctx, userID, reqID, "payload")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")

	_, err = svc.DownloadHistorySync(ctx, userID, sessionID, reqID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestHistorySync_UpsertReplace(t *testing.T) {
	pool := setupIntegrationDB(t)
	ctx := context.Background()

	userID, sessionID := createUserAndSession(t, pool)
	pubKey := "test-public-key-44-chars-base-64-encoded-key"

	mockAuth := &historySyncMockAuthProvider{
		primarySessionID: sessionID,
		publicKey:        pubKey,
	}

	svc := &chatService{
		PostgresQuerier: personal_chat_store.New(pool),
		AuthProvider:    mockAuth,
	}

	reqID, _, _, err := svc.RequestHistorySync(ctx, userID, sessionID, "\"chats1\"", pubKey)
	require.NoError(t, err)

	// Verify the request exists in DB
	meta, err := svc.PostgresQuerier.GetHistorySyncMeta(ctx, reqID)
	require.NoError(t, err)
	assert.Equal(t, userID, meta.UserID)
	assert.Equal(t, sessionID, meta.SessionID)
}

func TestHistorySync_EchoHandler_Ack(t *testing.T) {
	pool := setupIntegrationDB(t)
	ctx := context.Background()

	userID, sessionID := createUserAndSession(t, pool)
	pubKey := "test-public-key-44-chars-base-64-encoded-key"

	mockAuth := &historySyncMockAuthProvider{
		primarySessionID: sessionID,
		publicKey:        pubKey,
	}

	svc := &chatService{
		PostgresQuerier: personal_chat_store.New(pool),
		AuthProvider:    mockAuth,
	}

	reqID, _, _, err := svc.RequestHistorySync(ctx, userID, sessionID, "\"chats1\"", pubKey)
	require.NoError(t, err)

	handler := &chatHandler{Service: svc}

	e := echo.New()
	body := fmt.Sprintf(`{"request_id":"%s"}`, reqID.String())
	req := httptest.NewRequest(http.MethodPost, "/chat/history-sync/ack", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("userId", userID.String())
	c.Set("uuidUserId", userID)
	c.Set("sessionUUID", sessionID)

	err = handler.AcknowledgeHistorySync(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify row deleted
	_, err = svc.PostgresQuerier.GetHistorySyncMeta(ctx, reqID)
	assert.Error(t, err)
}

func TestHistorySync_ConnectHandler_Ack(t *testing.T) {
	pool := setupIntegrationDB(t)
	ctx := context.Background()

	userID, sessionID := createUserAndSession(t, pool)
	pubKey := "test-public-key-44-chars-base-64-encoded-key"

	mockAuth := &historySyncMockAuthProvider{
		primarySessionID: sessionID,
		publicKey:        pubKey,
	}

	svc := &chatService{
		PostgresQuerier: personal_chat_store.New(pool),
		AuthProvider:    mockAuth,
	}

	reqID, _, _, err := svc.RequestHistorySync(ctx, userID, sessionID, "\"chats1\"", pubKey)
	require.NoError(t, err)

	connectServer := &chatConnectServer{chatService: svc}
	reqCtx := context.WithValue(ctx, kit.CtxSessionData, kit.SessionData{
		UserID:      userID.String(),
		UUIDUserID:  userID,
		SessionUUID: sessionID,
	})

	res, err := connectServer.AcknowledgeHistorySync(reqCtx, connect.NewRequest(&rpc_personal_chatv1.AcknowledgeHistorySyncRequest{
		RequestId: reqID.String(),
	}))
	require.NoError(t, err)
	assert.True(t, res.Msg.Status)

	// Verify row deleted
	_, err = svc.PostgresQuerier.GetHistorySyncMeta(ctx, reqID)
	assert.Error(t, err)
}

