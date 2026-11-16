package personal_chat

import (
	"chatbasket-api/internal/platform/kit"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Unique 64-bit advisory lock key for partition maintenance mutual exclusion ("CBMSG_PA")
const partitionMaintenanceLockID = int64(0x4342_4d53_475f_5041)

const (
	partitionLockTimeout        = "150ms"
	partitionMaxLockRetries     = 3
	partitionAttemptTimeout     = 10 * time.Second
	partitionDropAttemptTimeout = 1 * time.Minute
)

var partitionRetryDelay = 500 * time.Millisecond

// PreflightBootstrap runs synchronous startup initialization for the chat module
// before the server starts accepting HTTP traffic. It ensures that yesterday's, today's,
// and forward daily partitions exist in PostgreSQL (creation only, no dropping).
// Uses a blocking advisory lock (bounded by ctx) so concurrent booting replicas wait
// until partitions are ready before opening HTTP traffic.
func (s *chatService) PreflightBootstrap(ctx context.Context) error {
	pool := s.MaintenancePool
	if pool == nil {
		return nil
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		log.Printf("[Chat Bootstrap] Failed to acquire connection from pool: %v", err)
		return err
	}
	defer conn.Release()

	return s.preflightBootstrapWithConn(ctx, conn)
}

func (s *chatService) preflightBootstrapWithConn(ctx context.Context, conn *pgxpool.Conn) error {
	// Blocking advisory lock bounded by ctx: if another replica is bootstrapping,
	// wait until it completes so this replica does not open traffic before partitions exist.
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", partitionMaintenanceLockID); err != nil {
		log.Printf("[Chat Bootstrap] Failed to acquire partition maintenance lock: %v", err)
		return err
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", partitionMaintenanceLockID)
	}()

	now := time.Now().UTC()
	return s.ensureForwardPartitions(ctx, conn, now)
}

func queryExistingPartitions(ctx context.Context, conn *pgxpool.Conn) ([]string, error) {
	qCtx, cancel := context.WithTimeout(ctx, partitionAttemptTimeout)
	defer cancel()

	rows, err := conn.Query(qCtx, `
		SELECT c.relname::text
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		WHERE i.inhparent = 'message_history'::regclass
		  AND c.relname LIKE 'message_history_y%'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return names, nil
}

func isLockTimeoutError(err error) bool {
	pgErr := kit.GetPostgresError(err)
	return pgErr != nil && pgErr.PgError != nil && pgErr.PgError.Code == "55P03"
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// ensureForwardPartitions pre-creates daily partitions for yesterday through now + 7 days
func (s *chatService) ensureForwardPartitions(ctx context.Context, conn *pgxpool.Conn, now time.Time) error {
	existingNames, err := queryExistingPartitions(ctx, conn)
	if err != nil {
		log.Printf("[Partition Maintenance] Failed to query existing partitions before creation: %v", err)
		return err
	}
	existing := make(map[string]struct{}, len(existingNames))
	for _, name := range existingNames {
		existing[name] = struct{}{}
	}

	for i := -1; i <= 7; i++ {
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		dayEnd := dayStart.AddDate(0, 0, 1)

		tableName := fmt.Sprintf("message_history_y%04dm%02dd%02d", dayStart.Year(), dayStart.Month(), dayStart.Day())
		if _, ok := existing[tableName]; ok {
			continue
		}

		from := dayStart.Format(time.RFC3339)
		to := dayEnd.Format(time.RFC3339)
		if err := createPartitionWithRetry(ctx, conn, tableName, from, to); err != nil {
			return err
		}
	}
	return nil
}

func createPartitionWithRetry(ctx context.Context, conn *pgxpool.Conn, tableName, from, to string) error {
	quotedName := pgx.Identifier{tableName}.Sanitize()
	createSQL := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF message_history FOR VALUES FROM ('%s') TO ('%s');",
		quotedName, from, to,
	)

	for attempt := 1; attempt <= partitionMaxLockRetries; attempt++ {
		attemptCtx, attemptCancel := context.WithTimeout(ctx, partitionAttemptTimeout)
		tx, err := conn.Begin(attemptCtx)
		if err != nil {
			attemptCancel()
			log.Printf("[Partition Maintenance] Failed to start tx for create %s: %v", tableName, err)
			return err
		}

		if _, err := tx.Exec(attemptCtx, "SET LOCAL lock_timeout = '"+partitionLockTimeout+"'"); err != nil {
			_ = tx.Rollback(attemptCtx)
			attemptCancel()
			log.Printf("[Partition Maintenance] Failed to set lock_timeout for create %s: %v", tableName, err)
			return err
		}

		if _, err := tx.Exec(attemptCtx, createSQL); err != nil {
			_ = tx.Rollback(attemptCtx)
			attemptCancel()
			pgErr := kit.GetPostgresError(err)
			if pgErr != nil && pgErr.PgError != nil && pgErr.PgError.Code == "42P07" {
				// Concurrent creation race (duplicate_table) — already exists
				return nil
			}
			if isLockTimeoutError(err) && attempt < partitionMaxLockRetries {
				log.Printf("[Partition Maintenance] Lock timeout creating %s (attempt %d/%d), retrying...", tableName, attempt, partitionMaxLockRetries)
				if sleepErr := sleepWithContext(ctx, partitionRetryDelay); sleepErr != nil {
					return sleepErr
				}
				continue
			}
			log.Printf("[Partition Maintenance] Failed to create partition %s: %v", tableName, err)
			return err
		}

		err = tx.Commit(attemptCtx)
		attemptCancel()
		if err != nil {
			log.Printf("[Partition Maintenance] Failed to commit create %s: %v", tableName, err)
			return err
		}
		return nil
	}
	return nil
}

// dropExpiredPartitions discovers and drops partitions older than 33 days (TTL + 3 days) via pg_inherits
func (s *chatService) dropExpiredPartitions(ctx context.Context, conn *pgxpool.Conn, now time.Time) error {
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -33)

	names, err := queryExistingPartitions(ctx, conn)
	if err != nil {
		log.Printf("[Partition Maintenance] Failed to query existing partitions for drop: %v", err)
		return err
	}

	var expiredPartitions []string
	for _, name := range names {
		var y, m, d int
		n, parseErr := fmt.Sscanf(name, "message_history_y%04dm%02dd%02d", &y, &m, &d)
		if parseErr == nil && n == 3 {
			partDate := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
			if partDate.Before(cutoff) {
				expiredPartitions = append(expiredPartitions, name)
			}
		}
	}

	for _, p := range expiredPartitions {
		dropPartitionWithRetry(ctx, conn, p)
	}

	return nil
}

func dropPartitionWithRetry(ctx context.Context, conn *pgxpool.Conn, tableName string) {
	quotedName := pgx.Identifier{tableName}.Sanitize()
	dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s;", quotedName)

	for attempt := 1; attempt <= partitionMaxLockRetries; attempt++ {
		log.Printf("[Partition Maintenance] Dropping expired partition: %s (attempt %d/%d)", tableName, attempt, partitionMaxLockRetries)
		attemptCtx, attemptCancel := context.WithTimeout(ctx, partitionDropAttemptTimeout)
		dropTx, err := conn.Begin(attemptCtx)
		if err != nil {
			attemptCancel()
			log.Printf("[Partition Maintenance] Failed to start tx for drop %s: %v", tableName, err)
			return
		}

		if _, err := dropTx.Exec(attemptCtx, "SET LOCAL lock_timeout = '"+partitionLockTimeout+"'"); err != nil {
			_ = dropTx.Rollback(attemptCtx)
			attemptCancel()
			log.Printf("[Partition Maintenance] Failed to set lock_timeout for drop %s: %v", tableName, err)
			return
		}

		if _, err := dropTx.Exec(attemptCtx, dropSQL); err != nil {
			_ = dropTx.Rollback(attemptCtx)
			attemptCancel()
			if isLockTimeoutError(err) && attempt < partitionMaxLockRetries {
				log.Printf("[Partition Maintenance] Lock timeout dropping %s (attempt %d/%d), retrying...", tableName, attempt, partitionMaxLockRetries)
				if sleepErr := sleepWithContext(ctx, partitionRetryDelay); sleepErr != nil {
					return
				}
				continue
			}
			log.Printf("[Partition Maintenance] Failed to drop expired partition %s (timed out / in use): %v", tableName, err)
			return
		}

		err = dropTx.Commit(attemptCtx)
		attemptCancel()
		if err != nil {
			log.Printf("[Partition Maintenance] Failed to commit drop %s: %v", tableName, err)
		}
		return
	}
}

// MaintainMessageHistoryPartitions maintains declarative partitions for message_history:
// 1. Pre-creates daily partitions for yesterday through now + 7 days so new messages always match a partition.
// 2. Drops daily partitions older than DefaultMessageTTL + 3 days (33 days).
// Protected by non-blocking PostgreSQL advisory locks (via dedicated pooled connection) and short lock_timeout retries.
func (s *chatService) MaintainMessageHistoryPartitions(ctx context.Context) error {
	pool := s.MaintenancePool
	if pool == nil {
		return nil
	}

	// 1. Checkout a dedicated physical connection from pgxpool so session advisory lock is strictly pinned
	conn, err := pool.Acquire(ctx)
	if err != nil {
		log.Printf("[Partition Maintenance] Failed to acquire connection from pool: %v", err)
		return err
	}
	defer conn.Release()

	return s.maintainPartitionsWithConn(ctx, conn)
}

func (s *chatService) maintainPartitionsWithConn(ctx context.Context, conn *pgxpool.Conn) error {
	// 2. Non-blocking advisory lock on this dedicated connection
	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", partitionMaintenanceLockID).Scan(&locked); err != nil {
		log.Printf("[Partition Maintenance] Failed to acquire advisory lock: %v", err)
		return err
	}
	if !locked {
		log.Println("[Partition Maintenance] Another maintenance worker is currently running, skipping.")
		return nil
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", partitionMaintenanceLockID)
	}()

	now := time.Now().UTC()

	// 3. Pre-create forward partitions
	if err := s.ensureForwardPartitions(ctx, conn, now); err != nil {
		return err
	}

	// 4. Drop expired partitions
	return s.dropExpiredPartitions(ctx, conn, now)
}
