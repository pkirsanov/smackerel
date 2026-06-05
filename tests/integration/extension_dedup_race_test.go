//go:build integration

// Spec 058 BUG-058 BLOCKER-2 (closure subset) — live-Postgres regression
// proof for `PostgresDedupStore.ResolveOrPublish` race-loss path.
//
// The race scenario the production code guards against (dedup.go
// §2.3 INSERT ... ON CONFLICT block):
//
//  1. Goroutine A's UPDATE on raw_ingest_dedup returns ErrNoRows.
//  2. Goroutine A calls publish() and gets artifact A.
//  3. Goroutine B's UPDATE also returns ErrNoRows (A hasn't INSERTed yet).
//  4. Goroutine B calls publish() and gets artifact B.
//  5. Goroutine A wins the INSERT (xmax = 0, inserted=true).
//  6. Goroutine B's INSERT triggers ON CONFLICT DO UPDATE and the
//     RETURNING clause yields A's artifact_id with inserted=false.
//  7. Per contract, goroutine B returns (A, deduped=true, nil).
//
// Adversarial cover:
//   - Both goroutines MUST return the same artifact_id (the winner's).
//   - Exactly one goroutine MUST report deduped=false (the winner).
//   - Exactly one goroutine MUST report deduped=true (the loser).
//   - raw_ingest_dedup.visit_count MUST be 2 (one INSERT, one ON
//     CONFLICT increment) — NOT 1 (would mean the loser's path
//     short-circuited) and NOT 3 (would mean double counting).
//   - The dedup row's artifact_id MUST match what BOTH callers returned.
//   - A regression that swapped the !inserted branch's return value
//     to use the LOSER's locally published id would fail the
//     same-artifact-id assertion.
//
// Discharges BUG-058-EXTERNAL-INFRA-MISSING Blocker 2 (Scope 2
// `PostgresDedupStore.ResolveOrPublish` race-loss path) deferred row.

package integration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector/ingest"
)

// TestPostgresDedupStore_ResolveOrPublish_RaceLossReturnsWinnerArtifact
// pins the spec 058 §2.3 race-loss contract against the live stack.
func TestPostgresDedupStore_ResolveOrPublish_RaceLossReturnsWinnerArtifact(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tid := testID(t)
	dedupKey := sha256Of(fmt.Sprintf("bug058-race-%s", tid))

	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM raw_ingest_dedup WHERE dedup_key = $1`, dedupKey)
		_, _ = pool.Exec(cctx, `DELETE FROM artifacts WHERE id LIKE $1`, "bug058-race-"+tid+"-%")
	})

	row := ingest.DedupRow{
		Key:            dedupKey,
		OwnerUserID:    "owner-" + tid,
		SourceID:       "browser-extension",
		ContentType:    "browser_history_visit",
		SourceDeviceID: "device-" + tid,
		CapturedAt:     time.Now().UTC().Truncate(time.Second),
	}

	store := ingest.NewPostgresDedupStore(pool)

	// Two-phase synchronization:
	//
	//   Phase 1 (barrier): both goroutines arrive simultaneously,
	//                       then issue ResolveOrPublish.
	//
	//   Phase 2 (publishGate): each PublishFunc registers its arrival
	//                          inside publish() — this proves the
	//                          UPDATE→ErrNoRows step was reached by
	//                          both callers — and waits until BOTH
	//                          have registered before allowing
	//                          either INSERT to start. That makes
	//                          the race-loss path deterministic
	//                          (both callers race the INSERT and
	//                          one ON-CONFLICTs onto the other's row).
	const callers = 2
	var barrier sync.WaitGroup
	barrier.Add(callers)
	var publishGate sync.WaitGroup
	publishGate.Add(callers)

	type result struct {
		artifactID string
		deduped    bool
		err        error
		ownPubID   string // the id this caller's publish() created
	}
	results := make([]result, callers)

	var publishCount atomic.Int64

	// publishFor returns a PublishFunc that creates a unique artifact
	// row (so the FK is satisfied) and records the id in the slot.
	publishFor := func(slot int) ingest.PublishFunc {
		return func(ctx context.Context) (string, error) {
			seq := publishCount.Add(1)
			// Signal arrival INSIDE publish — proves the UPDATE
			// returned ErrNoRows so the store fell through to the
			// publish step.
			publishGate.Done()
			// Block until both callers are inside publish — only
			// then will they both attempt the INSERT race that
			// the !inserted branch of ON CONFLICT exercises.
			publishGate.Wait()

			id := fmt.Sprintf("bug058-race-%s-%d", tid, seq)
			results[slot].ownPubID = id
			if _, err := pool.Exec(ctx, `
				INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
				VALUES ($1, 'browser_history_visit', $2, $3, 'browser-extension')
			`, id, "race-"+id, "h-"+id); err != nil {
				return "", fmt.Errorf("seed artifact %s: %w", id, err)
			}
			return id, nil
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine arrives at the barrier, then issues
			// ResolveOrPublish simultaneously. The publishGate inside
			// the PublishFunc then forces both to be inside publish
			// before either INSERT can run — making the race-loss
			// path (where one INSERT collides with the other) the
			// deterministic outcome rather than a coincidence.
			barrier.Done()
			barrier.Wait()
			id, deduped, err := store.ResolveOrPublish(ctx, row, publishFor(i))
			results[i] = result{
				artifactID: id,
				deduped:    deduped,
				err:        err,
				ownPubID:   results[i].ownPubID,
			}
		}()
	}
	wg.Wait()

	for i, r := range results {
		if r.err != nil {
			t.Fatalf("caller %d: ResolveOrPublish err: %v", i, r.err)
		}
	}

	// Both callers must report the SAME bound artifact id. A
	// regression that returned the loser's locally published id from
	// the !inserted branch would fail HERE.
	if results[0].artifactID != results[1].artifactID {
		t.Fatalf("race-loss bound id mismatch: caller0=%q caller1=%q",
			results[0].artifactID, results[1].artifactID)
	}

	dedupedCount := 0
	freshCount := 0
	winnerID := results[0].artifactID
	winnerSlot := -1
	for i, r := range results {
		if r.deduped {
			dedupedCount++
		} else {
			freshCount++
			winnerSlot = i
		}
	}
	if freshCount != 1 {
		t.Fatalf("expected exactly 1 deduped=false (winner), got %d", freshCount)
	}
	if dedupedCount != 1 {
		t.Fatalf("expected exactly 1 deduped=true (race-loser), got %d", dedupedCount)
	}
	if winnerSlot < 0 || results[winnerSlot].ownPubID != winnerID {
		t.Fatalf("winner slot %d: bound id %q must match its own publish() id %q",
			winnerSlot, winnerID, results[winnerSlot].ownPubID)
	}

	// The dedup row exists and points to the winner's artifact, with
	// visit_count==2 (one INSERT + one ON CONFLICT increment). A
	// regression that skipped the increment on the !inserted branch
	// would yield visit_count==1; double-counting yields ==3.
	var boundID string
	var visitCount int
	if err := pool.QueryRow(ctx, `
		SELECT artifact_id, visit_count FROM raw_ingest_dedup WHERE dedup_key = $1
	`, dedupKey).Scan(&boundID, &visitCount); err != nil {
		t.Fatalf("post-race dedup row read: %v", err)
	}
	if boundID != winnerID {
		t.Fatalf("dedup row artifact_id %q != winner id %q", boundID, winnerID)
	}
	if visitCount != 2 {
		t.Fatalf("expected visit_count=2 after race (1 insert + 1 ON CONFLICT bump), got %d", visitCount)
	}

	// Both publish callbacks were invoked (the race is real). A
	// regression that prevented the second caller from reaching
	// publish() would leave publishCount==1.
	if got := publishCount.Load(); got != 2 {
		t.Fatalf("expected both publish callbacks to fire (publishCount=2), got %d", got)
	}
}

// TestPostgresDedupStore_ResolveOrPublish_FastPathHitIncrementsCount
// pins the non-race fast path: when raw_ingest_dedup already has the
// key, ResolveOrPublish returns immediately with deduped=true and
// bumps visit_count. The publish callback MUST NOT be invoked.
func TestPostgresDedupStore_ResolveOrPublish_FastPathHitIncrementsCount(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tid := testID(t)
	dedupKey := sha256Of(fmt.Sprintf("bug058-fastpath-%s", tid))
	seedArtifactID := "bug058-fastpath-seed-" + tid

	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM raw_ingest_dedup WHERE dedup_key = $1`, dedupKey)
		_, _ = pool.Exec(cctx, `DELETE FROM artifacts WHERE id = $1`, seedArtifactID)
	})

	// Seed an artifact + a dedup row that points at it.
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, 'browser_history_visit', $2, $3, 'browser-extension')
	`, seedArtifactID, "seed-"+seedArtifactID, "h-"+seedArtifactID); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO raw_ingest_dedup
			(dedup_key, owner_user_id, source_id, content_type,
			 source_device_id, artifact_id, first_seen_at, last_seen_at, visit_count)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW(), 1)
	`, dedupKey, "owner-"+tid, "browser-extension", "browser_history_visit",
		"device-"+tid, seedArtifactID); err != nil {
		t.Fatalf("seed dedup row: %v", err)
	}

	store := ingest.NewPostgresDedupStore(pool)
	row := ingest.DedupRow{
		Key:            dedupKey,
		OwnerUserID:    "owner-" + tid,
		SourceID:       "browser-extension",
		ContentType:    "browser_history_visit",
		SourceDeviceID: "device-" + tid,
		CapturedAt:     time.Now().UTC(),
	}

	publishCalled := atomic.Int64{}
	got, deduped, err := store.ResolveOrPublish(ctx, row, func(ctx context.Context) (string, error) {
		publishCalled.Add(1)
		return "publish-should-not-run", nil
	})
	if err != nil {
		t.Fatalf("fast-path ResolveOrPublish: %v", err)
	}
	if got != seedArtifactID {
		t.Fatalf("fast path artifact_id %q != seeded %q", got, seedArtifactID)
	}
	if !deduped {
		t.Fatal("fast path must report deduped=true")
	}
	if publishCalled.Load() != 0 {
		t.Fatal("fast path MUST NOT invoke publish() — would mean the UPDATE→RETURNING bypass was broken")
	}

	var visitCount int
	if err := pool.QueryRow(ctx, `
		SELECT visit_count FROM raw_ingest_dedup WHERE dedup_key = $1
	`, dedupKey).Scan(&visitCount); err != nil {
		t.Fatalf("post-call visit_count read: %v", err)
	}
	if visitCount != 2 {
		t.Fatalf("expected visit_count to bump from 1 to 2, got %d", visitCount)
	}
}

func sha256Of(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}
