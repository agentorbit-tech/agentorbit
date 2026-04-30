//go:build integration

package cron

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startPG spins up a fresh postgres container and returns a pool + cleanup.
// Kept local to this package to avoid a dependency on processing/internal/testutil
// (which imports the migrations package — not needed for advisory-lock tests).
func startPG(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("agentorbit_cron_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("conn string: %v", err)
	}
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("pgxpool: %v", err)
	}
	return pool, func() {
		pool.Close()
		_ = pgContainer.Terminate(ctx)
	}
}

// TestAdvisoryLock_OnlyOneReplicaRuns simulates two replicas trying to run the
// same cron concurrently: exactly one fn body must execute, the other returns
// nil (skip).
func TestAdvisoryLock_OnlyOneReplicaRuns(t *testing.T) {
	pool, cleanup := startPG(t)
	t.Cleanup(cleanup)

	const lockID int64 = 0xA0_00_99_01
	ctx := context.Background()

	var ran atomic.Int32
	hold := make(chan struct{})
	started := make(chan struct{})
	var wg sync.WaitGroup

	// Replica A: holds the lock until "hold" is closed.
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := WithAdvisoryLock(ctx, pool, lockID, func(_ context.Context) error {
			ran.Add(1)
			close(started)
			<-hold
			return nil
		})
		if err != nil {
			t.Errorf("replica A: WithAdvisoryLock: %v", err)
		}
	}()

	<-started

	// Replica B: tries to take the same lock; pg_try_advisory_xact_lock
	// returns false → fn must NOT execute, error must be nil.
	err := WithAdvisoryLock(ctx, pool, lockID, func(_ context.Context) error {
		ran.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("replica B: expected nil (skip), got %v", err)
	}
	if got := ran.Load(); got != 1 {
		t.Fatalf("expected exactly 1 fn execution, got %d", got)
	}

	close(hold)
	wg.Wait()
}

// TestAdvisoryLock_ReleasedOnError verifies that returning an error from fn
// rolls the tx back, releasing the lock so the next call succeeds.
func TestAdvisoryLock_ReleasedOnError(t *testing.T) {
	pool, cleanup := startPG(t)
	t.Cleanup(cleanup)

	const lockID int64 = 0xA0_00_99_02
	ctx := context.Background()
	myErr := errors.New("intentional")

	err := WithAdvisoryLock(ctx, pool, lockID, func(_ context.Context) error {
		return myErr
	})
	if !errors.Is(err, myErr) {
		t.Fatalf("expected returned error to wrap myErr, got %v", err)
	}

	// Lock must have been released — try again, this time succeed.
	var ran bool
	err = WithAdvisoryLock(ctx, pool, lockID, func(_ context.Context) error {
		ran = true
		return nil
	})
	if err != nil {
		t.Fatalf("second call after error: %v", err)
	}
	if !ran {
		t.Fatalf("second call did not execute fn — lock was leaked")
	}
}

// TestAdvisoryLock_ReleasedOnPanic verifies that when fn panics the deferred
// rollback releases the lock and the panic is re-raised.
func TestAdvisoryLock_ReleasedOnPanic(t *testing.T) {
	pool, cleanup := startPG(t)
	t.Cleanup(cleanup)

	const lockID int64 = 0xA0_00_99_03
	ctx := context.Background()

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("expected panic to propagate")
			}
		}()
		_ = WithAdvisoryLock(ctx, pool, lockID, func(_ context.Context) error {
			panic("boom")
		})
	}()

	// Lock must have been released by the deferred rollback.
	var ran bool
	err := WithAdvisoryLock(ctx, pool, lockID, func(_ context.Context) error {
		ran = true
		return nil
	})
	if err != nil {
		t.Fatalf("call after panic: %v", err)
	}
	if !ran {
		t.Fatalf("fn did not run after panic — lock was leaked")
	}
}
