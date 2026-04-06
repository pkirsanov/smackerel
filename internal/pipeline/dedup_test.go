package pipeline

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDedupChecker_NilPool(t *testing.T) {
	// Verify DedupChecker struct can be created with a nil pool
	// (actual database tests require a running PostgreSQL instance)
	checker := &DedupChecker{Pool: (*pgxpool.Pool)(nil)}
	if checker.Pool != nil {
		t.Error("expected nil pool")
	}
}

func TestDedupResult_Fields(t *testing.T) {
	result := &DedupResult{
		IsDuplicate: true,
		ExistingID:  "01HXYZ",
		Title:       "Test Article",
	}
	if !result.IsDuplicate {
		t.Error("expected duplicate")
	}
	if result.ExistingID != "01HXYZ" {
		t.Errorf("expected existing ID '01HXYZ', got %q", result.ExistingID)
	}

	// Non-duplicate
	nodup := &DedupResult{IsDuplicate: false}
	if nodup.IsDuplicate {
		t.Error("expected not duplicate")
	}
}

// TestDedupChecker_Check_Integration would test against a real DB.
// Skipped in unit tests; covered by integration tests.
func TestDedupChecker_Check_Integration(t *testing.T) {
	t.Skip("requires PostgreSQL; covered by integration tests")

	pool, err := pgxpool.New(context.Background(), "postgres://smackerel:smackerel@localhost:5432/smackerel")
	if err != nil {
		t.Fatalf("connect to test DB: %v", err)
	}
	defer pool.Close()

	checker := &DedupChecker{Pool: pool}
	result, err := checker.Check(context.Background(), "nonexistent_hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsDuplicate {
		t.Error("expected no duplicate for nonexistent hash")
	}
}
