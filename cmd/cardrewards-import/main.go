// Package main is the one-time CCManager JSON → PostgreSQL importer for the
// Card Rewards Companion feature (spec 083, Scope 03; design §11).
//
// It connects to the database named by DATABASE_URL, ensures the schema is
// migrated, and seeds the card-rewards tables from a directory of CCManager
// `data/*.json` files. The import is idempotent (safe to re-run) and
// partial-file tolerant (a missing file is logged, not fatal).
//
// The data directory is resolved fail-loud (no silent default): the --data-dir
// flag takes precedence, otherwise CARD_REWARDS_IMPORT_DIR (SST-generated from
// config/smackerel.yaml card_rewards.import_data_dir); if neither resolves to a
// non-empty path the importer exits non-zero. The data directory is never
// committed — the operator supplies their environment's CCManager location.
//
// This binary performs NO build/compile work (it consumes JSON + the existing
// schema only) and is invoked through ./smackerel.sh, never run directly on a
// host (Constitution C7 / terminal-discipline).
//
// Usage:
//
//	DATABASE_URL=postgres://... cardrewards-import --data-dir /path/to/CCManager/data
//
// Exit codes:
//
//	0 — import completed (status success or partial); JSON report on stdout
//	1 — DATABASE_URL unset, data dir unresolved/invalid, connect/migrate failed,
//	    or a hard import error
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/cardrewards"
	"github.com/smackerel/smackerel/internal/db"
)

func main() {
	var dataDirFlag string
	flag.StringVar(&dataDirFlag, "data-dir", "", "path to the CCManager data/ directory (overrides CARD_REWARDS_IMPORT_DIR)")
	flag.Parse()

	if err := run(dataDirFlag); err != nil {
		fmt.Fprintf(os.Stderr, "cardrewards-import: %v\n", err)
		os.Exit(1)
	}
}

func run(dataDirFlag string) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL must be set")
	}

	// Resolve the data directory fail-loud: explicit flag first, then the
	// SST-generated env var. No silent default.
	dataDir := strings.TrimSpace(dataDirFlag)
	if dataDir == "" {
		dataDir = strings.TrimSpace(os.Getenv("CARD_REWARDS_IMPORT_DIR"))
	}
	if dataDir == "" {
		return fmt.Errorf("data directory required: pass --data-dir or set card_rewards.import_data_dir (CARD_REWARDS_IMPORT_DIR)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pg, err := db.Connect(ctx, url, 4, 1)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pg.Close()

	// Ensure the card-rewards schema exists before importing (idempotent).
	if err := db.Migrate(ctx, pg.Pool); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	store := cardrewards.NewStore(pg.Pool)
	report, err := cardrewards.RunImport(ctx, store, dataDir)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	fmt.Println(string(out))
	return nil
}
