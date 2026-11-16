package personal_chat

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreflightBootstrap_NilPoolReturnsNil(t *testing.T) {
	svc := &chatService{}
	assert.NoError(t, svc.PreflightBootstrap(context.Background()))
}

func TestMaintainMessageHistoryPartitions_NilPoolReturnsNil(t *testing.T) {
	svc := &chatService{}
	assert.NoError(t, svc.MaintainMessageHistoryPartitions(context.Background()))
}

// ─────────────────────────────────────────────────────────────────────────────
// INTEGRATION TESTS (SKIPS AUTOMATICALLY IF DATABASE URL IS NOT PROVIDED)
// ─────────────────────────────────────────────────────────────────────────────

func TestLivePartitions_PreflightBootstrapAndRouting(t *testing.T) {
	dbURL := getTestDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Ensure base table exists
	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS message_history (
			id UUID NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (id, created_at)
		) PARTITION BY RANGE (created_at);
	`)
	require.NoError(t, err)

	svc := &chatService{
		MaintenancePool: pool,
	}

	// 1. Run PreflightBootstrap
	err = svc.PreflightBootstrap(ctx)
	require.NoError(t, err)

	// 2. Insert test message into message_history with current timestamp
	testID := uuid.New()
	testCreatedAt := time.Now().UTC()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM message_history WHERE id = $1`, testID)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO message_history (id, created_at)
		VALUES ($1, $2)
	`, testID, testCreatedAt)
	require.NoError(t, err)

	// 3. Verify it routed to today's partition
	var tableOid string
	err = pool.QueryRow(ctx, `
		SELECT tableoid::regclass::text
		FROM message_history
		WHERE id = $1
	`, testID).Scan(&tableOid)
	require.NoError(t, err)

	expectedPartName := fmt.Sprintf("message_history_y%04dm%02dd%02d", testCreatedAt.Year(), testCreatedAt.Month(), testCreatedAt.Day())
	assert.Equal(t, expectedPartName, tableOid)
}

func TestLivePartitions_AdvisoryLockMutualExclusion(t *testing.T) {
	dbURL := getTestDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	conn1, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn1.Release()

	conn2, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn2.Release()

	// Worker 1 acquires advisory lock
	var locked1 bool
	err = conn1.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", partitionMaintenanceLockID).Scan(&locked1)
	require.NoError(t, err)
	require.True(t, locked1)

	// Worker 2 attempts same lock concurrently -> must be rejected
	var locked2 bool
	err = conn2.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", partitionMaintenanceLockID).Scan(&locked2)
	require.NoError(t, err)
	assert.False(t, locked2)

	// Release worker 1
	var unlocked1 bool
	err = conn1.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", partitionMaintenanceLockID).Scan(&unlocked1)
	require.NoError(t, err)
	assert.True(t, unlocked1)

	// Worker 2 can now acquire lock
	err = conn2.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", partitionMaintenanceLockID).Scan(&locked2)
	require.NoError(t, err)
	assert.True(t, locked2)

	// Clean up worker 2 lock
	_, _ = conn2.Exec(ctx, "SELECT pg_advisory_unlock($1)", partitionMaintenanceLockID)
}

func TestLivePartitions_AutoExpiryDropAndLockTimeout(t *testing.T) {
	dbURL := getTestDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Create an expired partition (e.g. year 2020)
	oldPart := "message_history_y2020m01d01"
	_, err = pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s PARTITION OF message_history
		FOR VALUES FROM ('2020-01-01T00:00:00Z') TO ('2020-01-02T00:00:00Z')
	`, oldPart))
	require.NoError(t, err)

	// Also create a non-partition backup table to verify it is NOT dropped
	backupTable := "message_history_y2020_backup"
	_, err = pool.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (id int)`, backupTable))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP TABLE IF EXISTS %s`, backupTable))
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP TABLE IF EXISTS %s`, oldPart))
	})

	svc := &chatService{
		MaintenancePool: pool,
	}

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	// 1. Hold an ACCESS SHARE lock on oldPart in another session to verify 150ms lock_timeout aborts cleanly without hanging
	lockConn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	lockTx, err := lockConn.Begin(ctx)
	require.NoError(t, err)
	var rowCount int
	require.NoError(t, lockTx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s`, oldPart)).Scan(&rowCount))

	err = svc.dropExpiredPartitions(ctx, conn, time.Now().UTC())
	require.NoError(t, err)

	var stillExists bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1)`, oldPart).Scan(&stillExists))
	assert.True(t, stillExists, "expired partition should remain when locked past 150ms lock_timeout")

	_ = lockTx.Rollback(ctx)
	lockConn.Release()

	// 2. Once lock is released, dropExpiredPartitions drops oldPart cleanly while leaving backupTable intact
	err = svc.dropExpiredPartitions(ctx, conn, time.Now().UTC())
	require.NoError(t, err)

	// Verify oldPart was dropped
	var oldPartExists bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1)`, oldPart).Scan(&oldPartExists))
	assert.False(t, oldPartExists, "expired partition should have been dropped")

	// Verify backupTable was NOT dropped
	var backupExists bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1)`, backupTable).Scan(&backupExists))
	assert.True(t, backupExists, "unrelated backup table must NOT be dropped")
}

func TestLivePartitions_MaintainMessageHistoryPartitions(t *testing.T) {
	dbURL := getTestDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Ensure base table exists
	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS message_history (
			id UUID NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (id, created_at)
		) PARTITION BY RANGE (created_at);
	`)
	require.NoError(t, err)

	svc := &chatService{
		MaintenancePool: pool,
	}

	// 1. Create an expired partition to verify it gets cleaned up by maintain
	oldPart := "message_history_y2019m05d05"
	_, err = pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s PARTITION OF message_history
		FOR VALUES FROM ('2019-05-05T00:00:00Z') TO ('2019-05-06T00:00:00Z')
	`, oldPart))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP TABLE IF EXISTS %s`, oldPart))
	})

	// 2. Run MaintainMessageHistoryPartitions
	err = svc.MaintainMessageHistoryPartitions(ctx)
	require.NoError(t, err)

	// 3. Verify expired partition was dropped
	var oldPartExists bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1)`, oldPart).Scan(&oldPartExists))
	assert.False(t, oldPartExists, "expired partition should have been dropped by maintain")

	// 4. Verify today's forward partition exists
	now := time.Now().UTC()
	todayPart := fmt.Sprintf("message_history_y%04dm%02dd%02d", now.Year(), now.Month(), now.Day())
	var todayPartExists bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1)`, todayPart).Scan(&todayPartExists))
	assert.True(t, todayPartExists, "today's partition should exist after maintain")
}
