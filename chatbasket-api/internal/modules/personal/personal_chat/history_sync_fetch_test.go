package personal_chat

// Unit tests for FetchHistorySync — the pointer-pattern fetch the primary uses
// to pull a secondary's request body AFTER receiving an id-only SSE event.
// pgxmock-based: no database needed (the integration file covers live-DB paths).

import (
	"context"
	"errors"
	"testing"
	"time"

	"chatbasket-api/internal/modules/personal/personal_chat/internal/personal_chat_store"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFetchSvc(t *testing.T, auth *historySyncMockAuthProvider) (*chatService, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })
	if auth == nil {
		auth = &historySyncMockAuthProvider{publicKey: "requester-pub-key"}
	}
	svc := &chatService{
		PostgresQuerier: personal_chat_store.New(mock),
		AuthProvider:    auth,
	}
	return svc, mock
}

func TestFetchHistorySync_Success(t *testing.T) {
	svc, mock := setupFetchSvc(t, nil)
	defer mock.Close()

	userID := uuid.New()
	reqID := uuid.New()
	secSessionID := uuid.New()

	mock.ExpectQuery("FROM history_sync").
		WithArgs(reqID, userID).
		WillReturnRows(pgxmock.NewRows([]string{"session_id", "chats_json", "expires_at"}).
			AddRow(secSessionID, []byte(`"cipher-body"`), time.Now().Add(10*time.Minute)))

	cipher, key, err := svc.FetchHistorySync(context.Background(), userID, reqID)
	require.NoError(t, err)
	// JSONB storage wraps the cipher in quotes — fetch must strip them.
	assert.Equal(t, "cipher-body", cipher)
	assert.Equal(t, "requester-pub-key", key)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchHistorySync_Expired(t *testing.T) {
	svc, mock := setupFetchSvc(t, nil)
	defer mock.Close()

	userID := uuid.New()
	reqID := uuid.New()

	mock.ExpectQuery("FROM history_sync").
		WithArgs(reqID, userID).
		WillReturnRows(pgxmock.NewRows([]string{"session_id", "chats_json", "expires_at"}).
			AddRow(uuid.New(), []byte(`"cipher-body"`), time.Now().Add(-time.Minute)))

	_, _, err := svc.FetchHistorySync(context.Background(), userID, reqID)
	require.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchHistorySync_NotFound(t *testing.T) {
	svc, mock := setupFetchSvc(t, nil)
	defer mock.Close()

	userID := uuid.New()
	reqID := uuid.New()

	mock.ExpectQuery("FROM history_sync").
		WithArgs(reqID, userID).
		WillReturnError(errors.New("no rows in result set"))

	_, _, err := svc.FetchHistorySync(context.Background(), userID, reqID)
	require.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchHistorySync_KeyLookupFails(t *testing.T) {
	svc, mock := setupFetchSvc(t, &historySyncMockAuthProvider{keyErr: errors.New("kms down")})
	defer mock.Close()

	userID := uuid.New()
	reqID := uuid.New()

	mock.ExpectQuery("FROM history_sync").
		WithArgs(reqID, userID).
		WillReturnRows(pgxmock.NewRows([]string{"session_id", "chats_json", "expires_at"}).
			AddRow(uuid.New(), []byte(`"cipher-body"`), time.Now().Add(10*time.Minute)))

	_, _, err := svc.FetchHistorySync(context.Background(), userID, reqID)
	require.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
