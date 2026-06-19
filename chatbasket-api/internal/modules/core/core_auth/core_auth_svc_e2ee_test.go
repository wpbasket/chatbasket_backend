package core_auth

import (
	"context"
	"errors"
	"testing"

	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
)

func setupTestAuthServiceE2EE(t *testing.T) (*AuthService, pgxmock.PgxPoolIface) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}

	store := core_auth_store.New(mock)
	svc := &AuthService{
		PostgresQuerier: store,
		PostgresQueries: store,
		Pool:            nil, // Logout tests need integration tests with real pool
		AuthSecret:      []byte("test-secret"),
	}
	return svc, mock
}

// setupTestAuthServiceE2EEWithPool creates a test auth service with a mock pool for transaction tests
func setupTestAuthServiceE2EEWithPool(t *testing.T) (*AuthService, pgxmock.PgxPoolIface) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}

	store := core_auth_store.New(mock)
	// For transaction tests, we need to use the mock as a pool
	// pgxmock.NewPool() returns an interface that implements pgxpool.Pool methods
	svc := &AuthService{
		PostgresQuerier: store,
		PostgresQueries: store,
		Pool:            (*pgxpool.Pool)(nil), // Will be set by individual tests
		AuthSecret:      []byte("test-secret"),
	}
	return svc, mock
}

// ============================================================================
// SaveSessionE2EEPublicKey Tests
// ============================================================================

func TestSaveSessionE2EEPublicKey_Success(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()
	sessionID := uuid.New()
	publicKey := "test-public-key-44-chars-base64-encoded=="

	mock.ExpectQuery(`UPDATE sessions SET e2ee_public_key = \$1, updated_at = now\(\)`).
		WithArgs(&publicKey, sessionID, userID).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(sessionID))

	err := svc.SaveSessionE2EEPublicKey(context.Background(), nil, userID, sessionID, publicKey)
	assert.NoError(t, err)
}

func TestSaveSessionE2EEPublicKey_SessionNotFound(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()
	sessionID := uuid.New()
	publicKey := "test-public-key-44-chars-base64-encoded=="

	mock.ExpectQuery(`UPDATE sessions SET e2ee_public_key = \$1, updated_at = now\(\)`).
		WithArgs(&publicKey, sessionID, userID).
		WillReturnError(pgx.ErrNoRows)

	err := svc.SaveSessionE2EEPublicKey(context.Background(), nil, userID, sessionID, publicKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found or expired")
}

func TestSaveSessionE2EEPublicKey_ExpiredSession(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()
	sessionID := uuid.New()
	publicKey := "test-public-key-44-chars-base64-encoded=="

	// Session exists but is expired - query returns no rows
	mock.ExpectQuery(`UPDATE sessions SET e2ee_public_key = \$1, updated_at = now\(\)`).
		WithArgs(&publicKey, sessionID, userID).
		WillReturnError(pgx.ErrNoRows)

	err := svc.SaveSessionE2EEPublicKey(context.Background(), nil, userID, sessionID, publicKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found or expired")
}

func TestSaveSessionE2EEPublicKey_DatabaseError(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()
	sessionID := uuid.New()
	publicKey := "test-public-key-44-chars-base64-encoded=="

	mock.ExpectQuery(`UPDATE sessions SET e2ee_public_key = \$1, updated_at = now\(\)`).
		WithArgs(&publicKey, sessionID, userID).
		WillReturnError(errors.New("database connection failed"))

	err := svc.SaveSessionE2EEPublicKey(context.Background(), nil, userID, sessionID, publicKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database connection failed")
}

// ============================================================================
// GetActiveSessionKeysForUser Tests
// ============================================================================

func TestGetActiveSessionKeysForUser_Success(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()
	session1 := uuid.New()
	session2 := uuid.New()
	key1 := "key1-44-chars-base64-encoded-public-key=="
	key2 := "key2-44-chars-base64-encoded-public-key=="

	mock.ExpectQuery(`SELECT id, e2ee_public_key FROM sessions`).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "e2ee_public_key"}).
			AddRow(session1, &key1).
			AddRow(session2, &key2))

	keys, err := svc.GetActiveSessionKeysForUser(context.Background(), userID)
	assert.NoError(t, err)
	assert.Len(t, keys, 2)
	assert.Equal(t, key1, keys[0])
	assert.Equal(t, key2, keys[1])
}

func TestGetActiveSessionKeysForUser_EmptyResult(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()

	mock.ExpectQuery(`SELECT id, e2ee_public_key FROM sessions`).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "e2ee_public_key"}))

	keys, err := svc.GetActiveSessionKeysForUser(context.Background(), userID)
	assert.NoError(t, err)
	assert.Empty(t, keys)
}

// ============================================================================
// CountActiveKeyedSessionsForUser Tests
// ============================================================================

func TestCountActiveKeyedSessionsForUser_Success(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM sessions`).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(3))

	count, err := svc.CountActiveKeyedSessionsForUser(context.Background(), nil, userID)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestCountActiveKeyedSessionsForUser_ZeroCount(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM sessions`).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))

	count, err := svc.CountActiveKeyedSessionsForUser(context.Background(), nil, userID)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

// ============================================================================
// GetKeysRevision Tests
// ============================================================================

func TestGetKeysRevision_Success(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()

	mock.ExpectQuery(`SELECT keys_revision FROM auth_users WHERE id = \$1 FOR UPDATE`).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"keys_revision"}).AddRow(5))

	revision, err := svc.GetKeysRevision(context.Background(), userID)
	assert.NoError(t, err)
	assert.Equal(t, int32(5), revision)
}

func TestGetKeysRevision_UserNotFound(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()

	mock.ExpectQuery(`SELECT keys_revision FROM auth_users WHERE id = \$1 FOR UPDATE`).
		WithArgs(userID).
		WillReturnError(pgx.ErrNoRows)

	revision, err := svc.GetKeysRevision(context.Background(), userID)
	assert.Error(t, err)
	assert.Equal(t, int32(0), revision)
}

// ============================================================================
// IncrementKeysRevision Tests
// ============================================================================

func TestIncrementKeysRevision_Success(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()

	mock.ExpectExec(`UPDATE auth_users SET keys_revision = keys_revision \+ 1 WHERE id = \$1`).
		WithArgs(userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := svc.IncrementKeysRevision(context.Background(), nil, userID)
	assert.NoError(t, err)
}

func TestIncrementKeysRevision_DatabaseError(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()

	mock.ExpectExec(`UPDATE auth_users SET keys_revision = keys_revision \+ 1 WHERE id = \$1`).
		WithArgs(userID).
		WillReturnError(errors.New("database error"))

	err := svc.IncrementKeysRevision(context.Background(), nil, userID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

// ============================================================================
// ResetKeysRevision Tests
// ============================================================================

func TestResetKeysRevision_Success(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()

	mock.ExpectExec(`UPDATE auth_users SET keys_revision = 0 WHERE id = \$1`).
		WithArgs(userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := svc.ResetKeysRevision(context.Background(), nil, userID)
	assert.NoError(t, err)
}

// ============================================================================
// Atomic Increment Race Condition Test
// ============================================================================

func TestIncrementKeysRevision_AtomicOperation(t *testing.T) {
	svc, mock := setupTestAuthServiceE2EE(t)
	defer mock.Close()

	userID := uuid.New()

	// The SQL should be atomic: UPDATE auth_users SET keys_revision = keys_revision + 1
	// This prevents race conditions where two concurrent increments could lose an update
	mock.ExpectExec(`UPDATE auth_users SET keys_revision = keys_revision \+ 1 WHERE id = \$1`).
		WithArgs(userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := svc.IncrementKeysRevision(context.Background(), nil, userID)
	assert.NoError(t, err)
	
	// Verify the query was executed (atomic increment, not read-modify-write)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}
