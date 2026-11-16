package pending_uploads

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupExpiredPendingUploads_BudgetSoftLimit(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	svc := NewService(mock, mock, nil)

	// Batch 1: returns 100 rows
	rows := pgxmock.NewRows([]string{"file_id", "bucket_name", "r2_key", "expires_at"})
	for i := 0; i < 100; i++ {
		rows.AddRow(fmt.Sprintf("file_%03d", i), "bucket", "key", time.Now().Add(-time.Hour))
	}

	mock.ExpectQuery("SELECT file_id, bucket_name, r2_key, expires_at FROM pending_uploads").
		WithArgs("", int32(100)).
		WillReturnRows(rows)

	// Expect 100 DELETE queries for batch 1
	for i := 0; i < 100; i++ {
		mock.ExpectExec("DELETE FROM pending_uploads WHERE file_id = \\$1").
			WithArgs(fmt.Sprintf("file_%03d", i)).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))
	}

	// Budget: 1ms (85% work budget = 850µs).
	// Because elapsed time after batch 1 will be >= 850µs, it should finish batch 1
	// completely without force-killing it, and then STOP before starting batch 2!
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	err = svc.CleanupExpiredPendingUploads(ctxTimeout)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCleanupExpiredPendingUploads_EarlyExit(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	svc := NewService(mock, mock, nil)

	// Partial batch: returns 5 rows (< 100)
	rows := pgxmock.NewRows([]string{"file_id", "bucket_name", "r2_key", "expires_at"})
	for i := 0; i < 5; i++ {
		rows.AddRow(fmt.Sprintf("file_%03d", i), "bucket", "key", time.Now().Add(-time.Hour))
	}

	mock.ExpectQuery("SELECT file_id, bucket_name, r2_key, expires_at FROM pending_uploads").
		WithArgs("", int32(100)).
		WillReturnRows(rows)

	for i := 0; i < 5; i++ {
		mock.ExpectExec("DELETE FROM pending_uploads WHERE file_id = \\$1").
			WithArgs(fmt.Sprintf("file_%03d", i)).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))
	}

	err = svc.CleanupExpiredPendingUploads(context.Background())
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCleanupExpiredPendingUploads_Live_EarlyExit(t *testing.T) {
	dsn := os.Getenv("DatabaseURLTesting")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL_PG_TESTING")
	}
	if dsn == "" {
		t.Skip("skipping live test: DatabaseURLTesting not set")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	defer pool.Close()

	svc := NewService(pool, pool, nil)
	ctx := context.Background()

	testPrefix := "test_live_" + uuid.New().String()[:8]
	_, err = pool.Exec(ctx, `
		INSERT INTO pending_uploads (file_id, bucket_name, r2_key, expires_at)
		VALUES
			($1 || '_1', 'bucket', 'key1', now() - interval '1 hour'),
			($1 || '_2', 'bucket', 'key2', now() - interval '1 hour'),
			($1 || '_3', 'bucket', 'key3', now() - interval '1 hour')
	`, testPrefix)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM pending_uploads WHERE file_id LIKE $1 || '%'`, testPrefix)
	})

	err = svc.CleanupExpiredPendingUploads(ctx)
	require.NoError(t, err)

	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM pending_uploads WHERE file_id LIKE $1 || '%'`, testPrefix).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "all expired uploads must be deleted")
}

func TestCleanupExpiredPendingUploads_Live_BudgetCutoff(t *testing.T) {
	dsn := os.Getenv("DatabaseURLTesting")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL_PG_TESTING")
	}
	if dsn == "" {
		t.Skip("skipping live test: DatabaseURLTesting not set")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	defer pool.Close()

	svc := NewService(pool, pool, nil)
	ctx := context.Background()

	testPrefix := "test_budget_" + uuid.New().String()[:8]
	// Insert 250 expired uploads (3 batches: 100 + 100 + 50)
	_, err = pool.Exec(ctx, `
		INSERT INTO pending_uploads (file_id, bucket_name, r2_key, expires_at)
		SELECT $1 || '_' || lpad(g::text, 4, '0'), 'bucket', 'key', now() - interval '1 hour'
		FROM generate_series(1, 250) AS g
	`, testPrefix)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM pending_uploads WHERE file_id LIKE $1 || '%'`, testPrefix)
	})

	// Run 1: Set budget to 1ms (85% work budget = 850µs).
	// Batch 1 (100 rows) runs and completes gracefully.
	// Even though the budget expires while Batch 1 is in-flight, Batch 1 is NOT cancelled.
	// It completes all 100 deletions, and then halts before starting Batch 2.
	t.Logf("[Live Test] Starting Run 1 with tiny budget (1ms). 250 expired rows seeded.")
	t0 := time.Now()
	ctxTimeout, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	err = svc.CleanupExpiredPendingUploads(ctxTimeout)
	cancel()
	elapsedRun1 := time.Since(t0)
	require.NoError(t, err)

	var remainingAfterRun1 int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM pending_uploads WHERE file_id LIKE $1 || '%'`, testPrefix).Scan(&remainingAfterRun1)
	require.NoError(t, err)
	t.Logf("[Live Test] Run 1 completed in %v. Budget was 1ms (work budget 850µs). Remaining rows in DB: %d (Expected: 150)", elapsedRun1, remainingAfterRun1)
	assert.Equal(t, 150, remainingAfterRun1, "Batch 1 (100 rows) must be deleted completely, leaving 150 rows in backlog")

	// Run 2: Next scheduled run with normal full budget.
	t.Logf("[Live Test] Starting Run 2 with normal full budget (20m) to process remaining 150 rows.")
	err = svc.CleanupExpiredPendingUploads(ctx)
	require.NoError(t, err)

	var remainingAfterRun2 int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM pending_uploads WHERE file_id LIKE $1 || '%'`, testPrefix).Scan(&remainingAfterRun2)
	require.NoError(t, err)
	t.Logf("[Live Test] Run 2 completed. Remaining rows in DB: %d (Expected: 0)", remainingAfterRun2)
	assert.Equal(t, 0, remainingAfterRun2, "Run 2 must finish cleaning all remaining rows")
}

func TestCleanupExpiredPendingUploads_UsesMaintenanceStoreNotMainStore(t *testing.T) {
	mainMock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mainMock.Close()

	maintMock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer maintMock.Close()

	svc := NewService(mainMock, maintMock, nil)

	rows := pgxmock.NewRows([]string{"file_id", "bucket_name", "r2_key", "expires_at"}).
		AddRow("file_001", "bucket", "key", time.Now().Add(-time.Hour))

	maintMock.ExpectQuery("SELECT file_id, bucket_name, r2_key, expires_at FROM pending_uploads").
		WithArgs("", int32(100)).
		WillReturnRows(rows)
	maintMock.ExpectExec("DELETE FROM pending_uploads WHERE file_id = \\$1").
		WithArgs("file_001").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	err = svc.CleanupExpiredPendingUploads(context.Background())
	require.NoError(t, err)
	assert.NoError(t, mainMock.ExpectationsWereMet())
	assert.NoError(t, maintMock.ExpectationsWereMet())
}
