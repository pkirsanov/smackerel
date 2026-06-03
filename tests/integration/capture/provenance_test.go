//go:build integration

// Spec 076 SCOPE-5 — TP-076-05-01 / SCN-074-A02.
//
// Live-Postgres integration proof that an explicit capture
// (provenance="capture-explicit") and a fallback capture
// (provenance="capture-as-fallback") for the SAME (user, normalized
// text) remain provenance-distinct: both rows persist, both are
// individually retrievable, and CountByProvenance reports 1/1.
//
// Adversarial coverage: a regression that merged the two provenances
// (for example by dropping `provenance` from the dedup-unique index
// in migration 051) would collapse the second insert and one of the
// counts would drop to 0. The test asserts strict 1/1, not >=1, so
// the regression is caught.

package capture_integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/capturefallback"
)

const (
	scope5HashKey     = "tp-076-05-hmac-key-do-not-reuse"
	scope5DedupWindow = 5 * time.Minute
)

func openScope5Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live test stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// insertScope5Artifact creates an artifacts row so the
// artifact_capture_policy FK is satisfied. content_hash must be
// unique per call.
func insertScope5Artifact(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, 'idea', $2, $3, 'capture')
	`, id, "spec076-scope5-"+id, "h-"+id); err != nil {
		t.Fatalf("insert artifact %s: %v", id, err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		if _, derr := pool.Exec(cctx, `DELETE FROM artifacts WHERE id = $1`, id); derr != nil {
			t.Logf("cleanup artifact %s: %v", id, derr)
		}
	})
}

// TestCapture_ExplicitVsFallbackProvenance — TP-076-05-01 / SCN-074-A02.
func TestCapture_ExplicitVsFallbackProvenance(t *testing.T) {
	pool := openScope5Pool(t)
	store := capturefallback.NewPostgresStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	stamp := time.Now().UnixNano()
	userID := fmt.Sprintf("spec076-scope5-prov-user-%d", stamp)
	const text = "draft a thank-you note for the meetup hosts"
	normalized := capturefallback.NormalizeV1(text)
	now := time.Now().UTC()

	explicitID := fmt.Sprintf("spec076-scope5-prov-explicit-%d", stamp)
	insertScope5Artifact(t, pool, explicitID)
	explicit := capturefallback.BuildExplicitPayload(capturefallback.ExplicitCaptureInput{
		ArtifactID:     explicitID,
		UserID:         userID,
		NormalizedText: normalized,
		DedupHashKey:   scope5HashKey,
		SourceTurnID:   "api:" + explicitID,
		IntentTraceID:  "intent-explicit-" + explicitID,
		CreatedAt:      now,
	})
	if err := store.Record(ctx, explicit); err != nil {
		t.Fatalf("Record explicit: %v", err)
	}

	fallbackID := fmt.Sprintf("spec076-scope5-prov-fallback-%d", stamp+1)
	insertScope5Artifact(t, pool, fallbackID)
	fallback := capturefallback.CapturePayload{
		ArtifactID:         fallbackID,
		UserID:             userID,
		Provenance:         capturefallback.ProvenanceFallback,
		FallbackCause:      capturefallback.CauseUnrouted,
		NormalizedText:     normalized,
		NormalizedTextHash: capturefallback.HashNormalized(normalized, scope5HashKey),
		DedupBucketStart:   capturefallback.BucketStart(now, scope5DedupWindow),
		DedupWindowSeconds: int(scope5DedupWindow / time.Second),
		SourceTurnID:       "tg:" + fallbackID,
		IntentTraceID:      "intent-fallback-" + fallbackID,
		SchemaVersion:      capturefallback.SchemaVersion,
		CreatedAt:          now,
	}
	if err := store.Record(ctx, fallback); err != nil {
		t.Fatalf("Record fallback: %v", err)
	}

	explicitCount, err := store.CountByProvenance(ctx, userID, capturefallback.ProvenanceExplicit)
	if err != nil {
		t.Fatalf("CountByProvenance(explicit): %v", err)
	}
	if explicitCount != 1 {
		t.Errorf("explicit count = %d, want 1", explicitCount)
	}
	fallbackCount, err := store.CountByProvenance(ctx, userID, capturefallback.ProvenanceFallback)
	if err != nil {
		t.Fatalf("CountByProvenance(fallback): %v", err)
	}
	if fallbackCount != 1 {
		t.Errorf("fallback count = %d, want 1 (SCN-074-A02 regression: explicit and fallback collapsed)", fallbackCount)
	}

	gotExplicit, err := store.GetByArtifactID(ctx, explicitID)
	if err != nil {
		t.Fatalf("GetByArtifactID(explicit): %v", err)
	}
	if gotExplicit.Provenance != capturefallback.ProvenanceExplicit {
		t.Errorf("explicit provenance = %q, want %q", gotExplicit.Provenance, capturefallback.ProvenanceExplicit)
	}
	if !gotExplicit.DedupBucketStart.IsZero() {
		t.Errorf("explicit dedup_bucket_start = %s, want zero (explicit rows bypass fallback dedup)", gotExplicit.DedupBucketStart)
	}
	gotFallback, err := store.GetByArtifactID(ctx, fallbackID)
	if err != nil {
		t.Fatalf("GetByArtifactID(fallback): %v", err)
	}
	if gotFallback.Provenance != capturefallback.ProvenanceFallback {
		t.Errorf("fallback provenance = %q, want %q", gotFallback.Provenance, capturefallback.ProvenanceFallback)
	}
	if gotFallback.FallbackCause != capturefallback.CauseUnrouted {
		t.Errorf("fallback cause = %q, want %q", gotFallback.FallbackCause, capturefallback.CauseUnrouted)
	}
}
