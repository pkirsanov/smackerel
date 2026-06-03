//go:build integration

// Spec 076 SCOPE-5 — TP-076-05-02 + TP-076-05-03 / SCN-074-A03 + SCN-074-A04.
//
// Live-Postgres integration proof of per-user dedup window semantics
// driven through capturefallback.Policy + PostgresDedupStore:
//
//   - TP-076-05-02 (SCN-074-A03): same user, same normalized text,
//     second turn within the dedup window → second CaptureForUser
//     returns AlreadyCaptured=true and points to the first artifact;
//     no second artifact_capture_policy row is written.
//
//   - TP-076-05-03 (SCN-074-A04): same user, same normalized text,
//     second turn after the dedup window has elapsed → second
//     CaptureForUser returns AlreadyCaptured=false with a fresh
//     artifact id; the policy row count for the user goes from 1 to
//     2.
//
// Adversarial coverage: each test asserts the strict counter delta
// (1 vs 2) so a regression that leaked dedup across buckets (or
// failed to dedup inside the bucket) is caught — not merely a
// >= probe that would pass either way.

package capture_integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/capturefallback"
)

// scope5PGWriter inserts an artifacts row per WriteIdea call so the
// artifact_capture_policy FK is satisfied without pulling the facade
// IdeaWriter into a SCOPE-5 dedup test. Returns a fresh artifact id
// each call.
type scope5PGWriter struct {
	t      *testing.T
	pool   *pgxpool.Pool
	prefix string

	mu        sync.Mutex
	artifacts []string
}

func newScope5PGWriter(t *testing.T, pool *pgxpool.Pool, prefix string) *scope5PGWriter {
	w := &scope5PGWriter{t: t, pool: pool, prefix: prefix}
	t.Cleanup(func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		for _, id := range w.artifacts {
			cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
			if _, derr := pool.Exec(cctx, `DELETE FROM artifacts WHERE id = $1`, id); derr != nil {
				t.Logf("cleanup artifact %s: %v", id, derr)
			}
			ccancel()
		}
	})
	return w
}

func (w *scope5PGWriter) WriteIdea(ctx context.Context, _ string, normalized string, _ capturefallback.Decision) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	id := fmt.Sprintf("%s-%d-%d", w.prefix, time.Now().UnixNano(), len(w.artifacts))
	hashSuffix := "x"
	if len(normalized) > 0 {
		hashSuffix = normalized[:1]
	}
	if _, err := w.pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, 'idea', $2, $3, 'capture')
	`, id, "spec076-scope5-"+id, "h-"+id+"-"+hashSuffix); err != nil {
		return "", fmt.Errorf("scope5PGWriter insert %s: %w", id, err)
	}
	w.artifacts = append(w.artifacts, id)
	return id, nil
}

func newScope5DedupPolicy(t *testing.T, pool *pgxpool.Pool, prefix string) (capturefallback.Policy, *scope5PGWriter) {
	t.Helper()
	cfg := capturefallback.Config{
		DedupWindow:         scope5DedupWindow,
		NormalizationPolicy: capturefallback.NormalizationPolicyV1,
		DedupHashKey:        scope5HashKey,
	}
	writer := newScope5PGWriter(t, pool, prefix)
	policy, err := capturefallback.New(cfg, capturefallback.NewPostgresDedupStore(pool), writer)
	if err != nil {
		t.Fatalf("capturefallback.New: %v", err)
	}
	return policy, writer
}

// TestCaptureDedup_WithinWindowDedupes — TP-076-05-02 / SCN-074-A03.
func TestCaptureDedup_WithinWindowDedupes(t *testing.T) {
	pool := openScope5Pool(t)
	store := capturefallback.NewPostgresStore(pool)
	policy, _ := newScope5DedupPolicy(t, pool, "spec076-scope5-dedup-in-art")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userID := fmt.Sprintf("spec076-scope5-dedup-in-user-%d", time.Now().UnixNano())
	const text = "follow up with the architect about the kitchen plan"
	now := time.Now().UTC()

	mkReq := func(msg string) capturefallback.Request {
		return capturefallback.Request{
			UserID:             userID,
			Transport:          "telegram",
			TransportMessageID: msg,
			OriginalText:       text,
			Cause:              capturefallback.CauseUnrouted,
			IntentTraceID:      "intent-in-" + msg,
			OccurredAt:         now,
		}
	}

	dec1, err := policy.Decide(ctx, mkReq("m1"))
	if err != nil {
		t.Fatalf("Decide m1: %v", err)
	}
	res1, err := policy.CaptureForUser(ctx, userID, dec1)
	if err != nil {
		t.Fatalf("CaptureForUser m1: %v", err)
	}
	if res1.AlreadyCaptured {
		t.Fatalf("first capture wrongly reported AlreadyCaptured")
	}

	// Second turn — same bucket because OccurredAt is identical and
	// the Decide clock derives the bucket from OccurredAt.
	dec2, err := policy.Decide(ctx, mkReq("m2"))
	if err != nil {
		t.Fatalf("Decide m2: %v", err)
	}
	if !dec2.DedupBucketStart.Equal(dec1.DedupBucketStart) {
		t.Fatalf("test-setup error: buckets diverged within window — A=%s B=%s", dec1.DedupBucketStart, dec2.DedupBucketStart)
	}
	res2, err := policy.CaptureForUser(ctx, userID, dec2)
	if err != nil {
		t.Fatalf("CaptureForUser m2: %v", err)
	}
	if !res2.AlreadyCaptured {
		t.Fatalf("second in-window capture must dedup; got new artifact %q (SCN-074-A03 regression)", res2.IdeaArtifactID)
	}
	if res2.AlreadyCapturedSourceID != res1.IdeaArtifactID {
		t.Errorf("dedup source = %q, want %q", res2.AlreadyCapturedSourceID, res1.IdeaArtifactID)
	}

	fallbackCount, err := store.CountByProvenance(ctx, userID, capturefallback.ProvenanceFallback)
	if err != nil {
		t.Fatalf("CountByProvenance: %v", err)
	}
	if fallbackCount != 1 {
		t.Errorf("fallback row count = %d, want 1 (dedup MUST NOT write a second row)", fallbackCount)
	}
}

// TestCaptureDedup_OutsideWindowDoesNotDedup — TP-076-05-03 / SCN-074-A04.
func TestCaptureDedup_OutsideWindowDoesNotDedup(t *testing.T) {
	pool := openScope5Pool(t)
	store := capturefallback.NewPostgresStore(pool)
	policy, _ := newScope5DedupPolicy(t, pool, "spec076-scope5-dedup-out-art")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userID := fmt.Sprintf("spec076-scope5-dedup-out-user-%d", time.Now().UnixNano())
	const text = "check the boiler service interval before fall"
	t0 := time.Now().UTC()
	// Step past the dedup window so the second decision falls in a
	// new bucket. We add 2× the window to guarantee the boundary
	// crossing regardless of bucket alignment.
	t1 := t0.Add(scope5DedupWindow * 2)

	mkReq := func(msg string, when time.Time) capturefallback.Request {
		return capturefallback.Request{
			UserID:             userID,
			Transport:          "http",
			TransportMessageID: msg,
			OriginalText:       text,
			Cause:              capturefallback.CauseOpenKnowledgeNoGround,
			IntentTraceID:      "intent-out-" + msg,
			OccurredAt:         when,
		}
	}

	dec1, err := policy.Decide(ctx, mkReq("r1", t0))
	if err != nil {
		t.Fatalf("Decide r1: %v", err)
	}
	res1, err := policy.CaptureForUser(ctx, userID, dec1)
	if err != nil {
		t.Fatalf("CaptureForUser r1: %v", err)
	}
	if res1.AlreadyCaptured {
		t.Fatal("first capture wrongly reported AlreadyCaptured")
	}

	dec2, err := policy.Decide(ctx, mkReq("r2", t1))
	if err != nil {
		t.Fatalf("Decide r2: %v", err)
	}
	if dec2.DedupBucketStart.Equal(dec1.DedupBucketStart) {
		t.Fatalf("test-setup error: bucket did not advance after %s — A=%s B=%s", scope5DedupWindow*2, dec1.DedupBucketStart, dec2.DedupBucketStart)
	}
	res2, err := policy.CaptureForUser(ctx, userID, dec2)
	if err != nil {
		t.Fatalf("CaptureForUser r2: %v", err)
	}
	if res2.AlreadyCaptured {
		t.Fatalf("out-of-window capture wrongly reported AlreadyCaptured (source=%q) — SCN-074-A04 regression", res2.AlreadyCapturedSourceID)
	}
	if res2.IdeaArtifactID == res1.IdeaArtifactID {
		t.Errorf("out-of-window capture reused artifact %q (dedup leaked across buckets)", res1.IdeaArtifactID)
	}

	fallbackCount, err := store.CountByProvenance(ctx, userID, capturefallback.ProvenanceFallback)
	if err != nil {
		t.Fatalf("CountByProvenance: %v", err)
	}
	if fallbackCount != 2 {
		t.Errorf("fallback row count = %d, want 2 (out-of-window MUST create a new row)", fallbackCount)
	}
}
