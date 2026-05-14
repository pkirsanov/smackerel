package main
// Package main is a small standalone migration runner used by CI.
//
// Spec 047 R12: CI's integration job needs to apply database
// migrations BEFORE running tests, but it MUST do so via the
// idempotent internal/db.Migrate runner so that schema_migrations
// is properly populated. The previous CI step used raw `psql -f
// each migration` which did NOT populate schema_migrations. When
// integration test helpers (e.g. authTestPool) later called
// db.Migrate, it found schema_migrations empty and tried to
// re-apply 001 from scratch, crashing on chk_rating_range
// already-exists (SQLSTATE 42710). Running this binary in CI
// fixes that by being the single migration entry point.
//
// This binary is intentionally minimal: connect → migrate → exit.
// It MUST NOT bring up any other service (no NATS, no metrics,
// no HTTP, no scenario loader). Doing so would couple CI test
// setup to runtime startup and re-introduce the cross-service
// timing failures spec 047 has been chasing.
//
// Usage:
//
//	DATABASE_URL=postgres://... go run ./cmd/dbmigrate
//
// Exit codes:
//
//	0 — all migrations applied (or no-op if already up to date)
//	1 — DATABASE_URL unset, connect failed, or migration failed
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/smackerel/smackerel/internal/db"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "dbmigrate: DATABASE_URL must be set")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pg, err := db.Connect(ctx, url, 4, 1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbmigrate: connect: %v\n", err)
		os.Exit(1)
	}
	defer pg.Close()

	if err := db.Migrate(ctx, pg.Pool); err != nil {
		fmt.Fprintf(os.Stderr, "dbmigrate: migrate: %v\n", err)
		os.Exit(1)
	}

	slog.Info("dbmigrate: all migrations applied")
}
