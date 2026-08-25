//go:build stress

// BUG-004-004 SCOPE-03 — T004-03-STRESS. Concurrent triggers for one window.
//
// The integration coverage claims two holders cannot both win a window, but it
// proves it SEQUENTIALLY: holder A claims, then holder B is refused. That is a
// weaker statement than it appears. Sequential calls cannot exercise the
// advisory lock at all, because there is never a second transaction inside the
// critical section. A coordinator that dropped the lock entirely and relied on
// the read-then-write alone would pass the sequential test and lose windows to
// a real race.
//
// This runs N genuinely concurrent claims against one window and asserts
// exactly one wins.

package stress

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/intelligence"
)

const synthesisConcurrentHolders = 16

func synthesisStressPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		// Fail rather than skip: a silent skip would let this stop running and
		// nobody would notice the concurrency claim had gone unproven.
		t.Fatal("stress: DATABASE_URL is empty; T004-03-STRESS needs a real PostgreSQL to exercise the advisory lock")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("stress: connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestSynthesisConcurrentClaims_ExactlyOneHolderWins(t *testing.T) {
	pool := synthesisStressPool(t)
	ctx := context.Background()

	for _, table := range []string{
		"synthesis_citations", "synthesis_output_insights", "synthesis_output_source_classes",
		"synthesis_outputs", "synthesis_run_attempts", "synthesis_runs",
	} {
		if _, err := pool.Exec(ctx, "DELETE FROM "+table); err != nil {
			t.Fatalf("clear %s: %v", table, err)
		}
	}

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}

	key := intelligence.SynthesisRunKey{
		Cadence:       intelligence.CadenceDaily,
		Principal:     "stress-contended",
		WindowStart:   time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC),
		WindowEnd:     time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
		PolicyVersion: "synthesis/v1",
		SourceIDs:     []string{"art-a", "art-b"},
	}
	policy := intelligence.SynthesisRetryPolicy{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		LeaseTTL:     time.Minute,
	}

	// A shared start gate, so the holders arrive together rather than trickling
	// in. Without it the goroutines would serialise on scheduling and this would
	// degrade into the sequential test it exists to strengthen.
	var start sync.WaitGroup
	start.Add(1)

	var wg sync.WaitGroup
	results := make([]error, synthesisConcurrentHolders)
	now := time.Now().UTC()

	for i := 0; i < synthesisConcurrentHolders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			coord, err := intelligence.NewSynthesisCoordinator(
				persistence, policy, "stress-holder-"+string(rune('a'+idx)))
			if err != nil {
				results[idx] = err
				return
			}
			start.Wait()
			results[idx] = coord.ClaimWindow(ctx, key, now)
		}(i)
	}

	start.Done()
	wg.Wait()

	var winners, refused, unexpected int
	for i, err := range results {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, intelligence.ErrRunClaimedElsewhere):
			refused++
		default:
			unexpected++
			t.Errorf("holder %d failed unexpectedly: %v", i, err)
		}
	}

	if winners != 1 {
		t.Fatalf("%d holders won the same window, want exactly 1 (refused=%d, unexpected=%d); duplicate winners produce duplicate work",
			winners, refused, unexpected)
	}
	if refused != synthesisConcurrentHolders-1 {
		t.Fatalf("refused=%d, want %d; every loser must be told it lost rather than proceeding",
			refused, synthesisConcurrentHolders-1)
	}

	// One row, one holder. The database is the arbiter, not process memory.
	var runs int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM synthesis_runs WHERE logical_key = $1`, key.LogicalKey()).Scan(&runs); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runs != 1 {
		t.Fatalf("got %d run rows for one logical window, want 1", runs)
	}

	var attemptCount int
	var holder string
	if err := pool.QueryRow(ctx,
		`SELECT attempt_count, COALESCE(lease_holder, '') FROM synthesis_runs WHERE logical_key = $1`,
		key.LogicalKey()).Scan(&attemptCount, &holder); err != nil {
		t.Fatalf("read claim: %v", err)
	}
	if attemptCount != 1 {
		t.Fatalf("attempt_count is %d after one successful claim among %d racers, want 1; a losing holder incremented the budget",
			attemptCount, synthesisConcurrentHolders)
	}
	if holder == "" {
		t.Fatal("winning claim recorded no lease holder")
	}
}
