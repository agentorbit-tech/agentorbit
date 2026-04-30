//go:build integration

package main_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/agentorbit-tech/agentorbit/processing/migrations"
)

// buildBinary compiles cmd/migrate into a tempdir-relative path and returns it.
// Done once per test process via t.Helper + t.Cleanup.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "agentorbit-migrate")
	cmd := exec.Command("go", "build", "-o", binPath, "github.com/agentorbit-tech/agentorbit/processing/cmd/migrate")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build failed: %v", err)
	}
	return binPath
}

func startPG(t *testing.T) (string, func()) {
	t.Helper()
	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("agentorbit_migrate_test"),
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

// runBinary executes the migrate binary with given args, returns combined output.
// extraEnv is appended on top of a sanitized base env (DATABASE_URL only) so
// the destructive guard is not silently satisfied by the developer's shell.
func runBinary(t *testing.T, bin, dbURL string, extraEnv []string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	// Build a minimal env: PATH (for `go` resolution if needed), HOME, and
	// the explicit DATABASE_URL — but NOT the parent's
	// AGENTORBIT_ALLOW_DESTRUCTIVE. This keeps the destructive-refusal tests
	// honest when run on a developer machine that exports it.
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"DATABASE_URL=" + dbURL,
	}
	cmd.Env = append(cmd.Env, extraEnv...)
	return cmd.CombinedOutput()
}

func TestMigrateUp_AppliesPending(t *testing.T) {
	connStr, cleanup := startPG(t)
	t.Cleanup(cleanup)
	bin := buildBinary(t)

	out, err := runBinary(t, bin, connStr, nil, "up")
	if err != nil {
		t.Fatalf("migrate up: %v\noutput:\n%s", err, out)
	}

	// Confirm: applied version must equal MaxEmbeddedVersion.
	maxV, err := migrations.MaxEmbeddedVersion()
	if err != nil {
		t.Fatalf("MaxEmbeddedVersion: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()
	var version int64
	var dirty bool
	if err := pool.QueryRow(context.Background(), "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if dirty {
		t.Fatalf("schema_migrations is dirty after up")
	}
	if uint(version) != maxV {
		t.Fatalf("expected version=%d, got %d", maxV, version)
	}
}

func TestMigrateDown_RefusesWithoutFlag(t *testing.T) {
	connStr, cleanup := startPG(t)
	t.Cleanup(cleanup)
	bin := buildBinary(t)

	// Apply migrations first so there's something to roll back.
	if _, err := runBinary(t, bin, connStr, nil, "up"); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	// 1) Missing env var.
	out, err := runBinary(t, bin, connStr, nil, "down", "--steps=1", "--i-know-what-im-doing")
	if err == nil {
		t.Fatalf("expected non-zero exit when env var is missing, got success\noutput:\n%s", out)
	}
	if !strings.Contains(string(out), "AGENTORBIT_ALLOW_DESTRUCTIVE") {
		t.Fatalf("expected output to mention env var name, got:\n%s", out)
	}

	// 2) Env var set but flag missing.
	out, err = runBinary(t, bin, connStr, []string{"AGENTORBIT_ALLOW_DESTRUCTIVE=1"}, "down", "--steps=1")
	if err == nil {
		t.Fatalf("expected non-zero exit when flag is missing, got success\noutput:\n%s", out)
	}
	if !strings.Contains(string(out), "i-know-what-im-doing") {
		t.Fatalf("expected output to mention --i-know-what-im-doing, got:\n%s", out)
	}
}

func TestMigrateStatus_ReportsDirty(t *testing.T) {
	connStr, cleanup := startPG(t)
	t.Cleanup(cleanup)
	bin := buildBinary(t)

	// Apply migrations so the table exists.
	if _, err := runBinary(t, bin, connStr, nil, "up"); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	// Set dirty=true manually.
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), "UPDATE schema_migrations SET dirty = true"); err != nil {
		t.Fatalf("set dirty: %v", err)
	}

	out, err := runBinary(t, bin, connStr, nil, "status")
	if err != nil {
		t.Fatalf("status returned non-zero: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(string(out), `"dirty":true`) {
		t.Fatalf("expected status output to surface dirty flag, got:\n%s", out)
	}
}
