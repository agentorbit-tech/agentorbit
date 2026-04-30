package cron

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrLockNotAcquired is the sentinel returned when another replica already
// holds the lock. Callers usually treat it as a no-op: WithAdvisoryLock itself
// handles the logging and swallows it before returning to fn-style callers.
var ErrLockNotAcquired = errors.New("advisory lock not acquired")

// WithAdvisoryLock attempts to take a session-level advisory lock for lockID
// using a dedicated short-lived transaction. The lock is released when the
// transaction commits or rolls back, so fn is free to issue further queries
// against the same pool without lock contention on the holding connection.
//
// Lock semantics:
//   - If another replica holds the lock, the function logs at debug level and
//     returns nil (no error) — callers treat it as "skip this tick".
//   - If fn returns an error, the tx is rolled back (releasing the lock) and
//     the error is returned to the caller.
//   - If fn panics, the tx is rolled back via the deferred Rollback (releasing
//     the lock) and the panic is re-raised so the supervising goroutine can
//     report it to errtrack.
//
// This is the "lock-only-tx" pattern documented in SP-3: the tx exists solely
// to scope the advisory lock; the actual cron work runs against the pool.
func WithAdvisoryLock(ctx context.Context, pool *pgxpool.Pool, lockID int64, fn func(context.Context) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("advisory lock: begin tx: %w", err)
	}
	// Rollback is safe to call after Commit (it becomes a no-op).
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	var got bool
	if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock($1)", lockID).Scan(&got); err != nil {
		return fmt.Errorf("advisory lock: try lock %d: %w", lockID, err)
	}
	if !got {
		slog.Debug("cron skipped, advisory lock held by another replica", "lock", lockID)
		return nil
	}

	if err := fn(ctx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("advisory lock: commit tx: %w", err)
	}
	committed = true
	return nil
}
