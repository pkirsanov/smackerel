// Spec 075 SCOPE-6.2 — TP-075-20 unit coverage for
// buildLegacyRetirementPolicy, the construction site that wires
// the spec 075 legacy-retirement dispatcher into
// FacadeConfig.Policy.
//
// These tests assert construction-time invariants only: the helper
// returns a non-nil Policy when a valid LegacyRetirement SST block
// + a *pgxpool.Pool are supplied, and fails loud (G028/G029,
// smackerel-no-defaults) when any required SST key or dependency is
// missing. They do NOT bring up the live stack — the pool is
// constructed lazily via pgxpool.New so no connection is opened.

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/config"
)

func newLegacyRetirementValidCfg() *config.Config {
	cfg := &config.Config{}
	cfg.LegacyRetirement = config.LegacyRetirementConfig{
		WindowID:                            "spec075-test-window",
		WindowState:                         config.LegacyRetirementWindowOpen,
		RollbackThresholdPercentActiveUsers: 1,
		RollbackThresholdDaysConsecutive:    3,
		PostWindowObservationDays:           7,
		ActiveUserWindowDays:                7,
		UserBucketHMACKey:                   "spec075-test-hmac-key",
		NoticeCopyPerCommand:                map[string]string{"/weather": "Use natural language for weather."},
		PostWindowUnknownResponseCopy:       map[string]string{"/weather": "That command is retired; ask in plain language."},
	}
	return cfg
}

func newFakePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	// pgxpool.New is lazy: it parses the connection string and
	// allocates the pool struct but does NOT dial Postgres until
	// the first Acquire. That makes it safe to use as a non-nil
	// pool stand-in for construction-time unit tests.
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/db")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// SCN-075-A12 happy path: a complete SST block + non-nil pool wires
// a non-nil Policy with no errors.
func TestBuildLegacyRetirementPolicy_ValidConfigWiresNonNilPolicy(t *testing.T) {
	cfg := newLegacyRetirementValidCfg()
	pool := newFakePool(t)
	p, err := buildLegacyRetirementPolicy(cfg, pool, time.Now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil Policy, got nil")
	}
}

func TestBuildLegacyRetirementPolicy_NilConfigErrors(t *testing.T) {
	pool := newFakePool(t)
	_, err := buildLegacyRetirementPolicy(nil, pool, time.Now)
	if err == nil || !strings.Contains(err.Error(), "nil config") {
		t.Fatalf("expected nil-config error, got: %v", err)
	}
}

func TestBuildLegacyRetirementPolicy_NilPoolErrors(t *testing.T) {
	cfg := newLegacyRetirementValidCfg()
	_, err := buildLegacyRetirementPolicy(cfg, nil, time.Now)
	if err == nil || !strings.Contains(err.Error(), "postgres pool") {
		t.Fatalf("expected nil-pool error, got: %v", err)
	}
}

func TestBuildLegacyRetirementPolicy_NilClockErrors(t *testing.T) {
	cfg := newLegacyRetirementValidCfg()
	pool := newFakePool(t)
	_, err := buildLegacyRetirementPolicy(cfg, pool, nil)
	if err == nil || !strings.Contains(err.Error(), "clock") {
		t.Fatalf("expected nil-clock error, got: %v", err)
	}
}

func TestBuildLegacyRetirementPolicy_EmptyWindowIDErrors(t *testing.T) {
	cfg := newLegacyRetirementValidCfg()
	cfg.LegacyRetirement.WindowID = ""
	pool := newFakePool(t)
	_, err := buildLegacyRetirementPolicy(cfg, pool, time.Now)
	if err == nil {
		t.Fatal("expected error for empty WindowID, got nil")
	}
}

func TestBuildLegacyRetirementPolicy_EmptyHMACKeyErrors(t *testing.T) {
	cfg := newLegacyRetirementValidCfg()
	cfg.LegacyRetirement.UserBucketHMACKey = ""
	pool := newFakePool(t)
	_, err := buildLegacyRetirementPolicy(cfg, pool, time.Now)
	if err == nil || !strings.Contains(err.Error(), "bucket hasher") {
		t.Fatalf("expected bucket-hasher error for empty HMAC key, got: %v", err)
	}
}

func TestBuildLegacyRetirementPolicy_EmptyNoticeCopyErrors(t *testing.T) {
	cfg := newLegacyRetirementValidCfg()
	cfg.LegacyRetirement.NoticeCopyPerCommand = nil
	pool := newFakePool(t)
	_, err := buildLegacyRetirementPolicy(cfg, pool, time.Now)
	if err == nil || !strings.Contains(err.Error(), "catalog") {
		t.Fatalf("expected catalog error for empty notice copy, got: %v", err)
	}
}

func TestBuildLegacyRetirementPolicy_InvalidWindowStateErrors(t *testing.T) {
	cfg := newLegacyRetirementValidCfg()
	cfg.LegacyRetirement.WindowState = "paused" // SST may only be open/closed
	pool := newFakePool(t)
	_, err := buildLegacyRetirementPolicy(cfg, pool, time.Now)
	if err == nil || !strings.Contains(err.Error(), "state resolver") {
		t.Fatalf("expected state-resolver error for invalid window state, got: %v", err)
	}
}
