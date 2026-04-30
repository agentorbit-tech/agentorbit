// Command agentorbit-migrate manages database schema migrations as a separate
// executable. It is intended to run as a one-shot init-container/job, decoupled
// from the long-running processing service so that bad migrations do not turn
// every replica into a crash-loop.
//
// Subcommands:
//
//	up                                 — apply all pending up migrations
//	status                             — print current version, dirty flag, and
//	                                     a count of pending migrations (exit 0)
//	down --steps=N --i-know-what-im-doing
//	                                   — destructive; gated by the
//	                                     AGENTORBIT_ALLOW_DESTRUCTIVE=1 env var
//	                                     AND the explicit flag
//	force <version>                    — destructive; same gate as `down`
//
// The binary reads DATABASE_URL from the environment and emits one JSON log
// line per migration containing version + duration via slog.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/agentorbit-tech/agentorbit/processing/migrations"
)

const destructiveEnvVar = "AGENTORBIT_ALLOW_DESTRUCTIVE"

func main() {
	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(logHandler))

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "up":
		if err := runUp(databaseURL); err != nil {
			slog.Error("migrate up failed", "error", err)
			os.Exit(1)
		}
	case "status":
		if err := runStatus(databaseURL); err != nil {
			slog.Error("migrate status failed", "error", err)
			os.Exit(1)
		}
	case "down":
		if err := runDown(databaseURL, args); err != nil {
			slog.Error("migrate down failed", "error", err)
			os.Exit(1)
		}
	case "force":
		if err := runForce(databaseURL, args); err != nil {
			slog.Error("migrate force failed", "error", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		usage()
	default:
		slog.Error("unknown subcommand", "command", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `agentorbit-migrate — schema migration tool for AgentOrbit processing

Usage:
  agentorbit-migrate up
  agentorbit-migrate status
  agentorbit-migrate down --steps=N --i-know-what-im-doing
  agentorbit-migrate force <version> --i-know-what-im-doing

Destructive operations (down, force) require the env var
AGENTORBIT_ALLOW_DESTRUCTIVE=1 AND the --i-know-what-im-doing flag.
DATABASE_URL must be set in the environment.`)
}

// migrateURLFromDB rewrites postgres:// to pgx5:// because the pgx/v5 driver
// for golang-migrate registers under the "pgx5" scheme.
func migrateURLFromDB(databaseURL string) string {
	return strings.Replace(databaseURL, "postgres://", "pgx5://", 1)
}

func newMigrator(databaseURL string) (*migrate.Migrate, error) {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("migrations source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, migrateURLFromDB(databaseURL))
	if err != nil {
		return nil, fmt.Errorf("migrate init: %w", err)
	}
	return m, nil
}

func runUp(databaseURL string) error {
	maxV, err := migrations.MaxEmbeddedVersion()
	if err != nil {
		return fmt.Errorf("scan embedded migrations: %w", err)
	}

	m, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	startVersion, _, err := currentVersion(m)
	if err != nil {
		return err
	}

	start := time.Now()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	dur := time.Since(start)

	endVersion, dirty, err := currentVersion(m)
	if err != nil {
		return err
	}
	slog.Info("migrate up complete",
		"start_version", startVersion,
		"end_version", endVersion,
		"max_embedded_version", maxV,
		"dirty", dirty,
		"duration_ms", dur.Milliseconds(),
	)
	return nil
}

func runStatus(databaseURL string) error {
	maxV, err := migrations.MaxEmbeddedVersion()
	if err != nil {
		return fmt.Errorf("scan embedded migrations: %w", err)
	}

	m, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	curVersion, dirty, err := currentVersion(m)
	if err != nil {
		return err
	}
	pending := uint(0)
	if curVersion < maxV {
		pending = maxV - curVersion
	}
	slog.Info("migrate status",
		"current_version", curVersion,
		"dirty", dirty,
		"max_embedded_version", maxV,
		"pending_count", pending,
	)
	return nil
}

func runDown(databaseURL string, args []string) error {
	fs := flag.NewFlagSet("down", flag.ContinueOnError)
	steps := fs.Int("steps", 0, "number of migrations to roll back (must be > 0)")
	confirm := fs.Bool("i-know-what-im-doing", false, "confirm destructive operation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := guardDestructive(*confirm); err != nil {
		return err
	}
	if *steps <= 0 {
		return fmt.Errorf("--steps must be > 0")
	}

	m, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	startVersion, _, err := currentVersion(m)
	if err != nil {
		return err
	}

	start := time.Now()
	if err := m.Steps(-*steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate down %d steps: %w", *steps, err)
	}
	dur := time.Since(start)

	endVersion, dirty, err := currentVersion(m)
	if err != nil {
		return err
	}
	slog.Info("migrate down complete",
		"steps", *steps,
		"start_version", startVersion,
		"end_version", endVersion,
		"dirty", dirty,
		"duration_ms", dur.Milliseconds(),
	)
	return nil
}

func runForce(databaseURL string, args []string) error {
	fs := flag.NewFlagSet("force", flag.ContinueOnError)
	confirm := fs.Bool("i-know-what-im-doing", false, "confirm destructive operation")
	// version is a positional arg
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("force requires exactly one positional argument: <version>")
	}
	version, err := strconv.Atoi(rest[0])
	if err != nil {
		return fmt.Errorf("invalid version %q: %w", rest[0], err)
	}
	if err := guardDestructive(*confirm); err != nil {
		return err
	}

	m, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	start := time.Now()
	if err := m.Force(version); err != nil {
		return fmt.Errorf("migrate force %d: %w", version, err)
	}
	dur := time.Since(start)

	curVersion, dirty, err := currentVersion(m)
	if err != nil {
		return err
	}
	slog.Info("migrate force complete",
		"forced_version", version,
		"current_version", curVersion,
		"dirty", dirty,
		"duration_ms", dur.Milliseconds(),
	)
	return nil
}

func guardDestructive(confirm bool) error {
	if os.Getenv(destructiveEnvVar) != "1" {
		return fmt.Errorf("destructive operation refused: env %s=1 is required", destructiveEnvVar)
	}
	if !confirm {
		return fmt.Errorf("destructive operation refused: --i-know-what-im-doing flag is required")
	}
	return nil
}

// currentVersion returns the current schema version + dirty flag, or 0/false
// when no migrations have ever run (migrate.ErrNilVersion).
func currentVersion(m *migrate.Migrate) (uint, bool, error) {
	v, dirty, err := m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read schema version: %w", err)
	}
	return v, dirty, nil
}
