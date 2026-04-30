//go:build integration

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startPG(t *testing.T) (string, func()) {
	t.Helper()
	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("agentorbit_check_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start pg: %v", err)
	}
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("conn string: %v", err)
	}
	return connStr, func() { _ = pgContainer.Terminate(ctx) }
}

// TestProcessingRefusesUnmigrated — verifyMigrationsApplied must reject a fresh
// database where schema_migrations does not exist.
func TestProcessingRefusesUnmigrated(t *testing.T) {
	connStr, cleanup := startPG(t)
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	err = verifyMigrationsApplied(ctx, pool)
	if err == nil {
		t.Fatalf("expected verifyMigrationsApplied to fail on fresh DB, got nil")
	}
	if !strings.Contains(err.Error(), "agentorbit-migrate") {
		t.Fatalf("expected error to mention agentorbit-migrate, got: %v", err)
	}
}

// TestProcessingRejectsDirtyMigration — even when the table exists with the
// correct version, dirty=true must block startup.
func TestProcessingRejectsDirtyMigration(t *testing.T) {
	connStr, cleanup := startPG(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	// Manufacture a dirty schema_migrations row at the maximum embedded version.
	if _, err := pool.Exec(ctx, "CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)"); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO schema_migrations (version, dirty) VALUES (9999, true)"); err != nil {
		t.Fatalf("insert dirty row: %v", err)
	}

	err = verifyMigrationsApplied(ctx, pool)
	if err == nil {
		t.Fatalf("expected verifyMigrationsApplied to fail on dirty migration, got nil")
	}
	if !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("expected error to mention dirty, got: %v", err)
	}
}
