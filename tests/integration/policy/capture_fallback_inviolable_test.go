//go:build integration

// Spec 074 SCOPE-04A — TP-074-18 / SCN-074-A09 (live inviolability).
//
// Drives the live assistant.Facade with the capture-as-fallback
// policy wired against real Postgres state and proves that no
// runtime knob exposed by the Smackerel SST surface can suppress
// capture for an eligible BandLow turn. The static guard in SCOPE-1
// (capturefallback/inviolable_static_test.go) covers the code-level
// vocabulary; this test covers the deployed-binary level by
// exercising the facade hook end-to-end.
//
// Adversarial coverage: even with an empty manifest (the worst-case
// "no skills enabled" production stance) and a router that returns
// ok=false, the fallback Idea MUST still be written. If a regression
// adds any suppression branch — e.g. a "disable capture in test
// mode" env var, a manifest-disabled bypass, a transport-aware
// suppression — the fallback row count would drop to zero and this
// test would trip.

package policy

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/capturefallback"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

const (
	tp18HashKey     = "tp-074-18-hmac-key-do-not-reuse"
	tp18DedupWindow = 5 * time.Minute
)

func openTP18Pool(t *testing.T) *pgxpool.Pool {
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

// tp18IdeaWriter inserts a real artifacts row so the
// artifact_capture_policy FK is satisfied.
type tp18IdeaWriter struct {
	t         *testing.T
	pool      *pgxpool.Pool
	mu        sync.Mutex
	artifacts []string
}

func newTP18IdeaWriter(t *testing.T, pool *pgxpool.Pool) *tp18IdeaWriter {
	w := &tp18IdeaWriter{t: t, pool: pool}
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

func (w *tp18IdeaWriter) WriteIdea(ctx context.Context, _ string, normalizedText string, _ capturefallback.Decision) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	id := fmt.Sprintf("tp18-art-%d-%d", time.Now().UnixNano(), len(w.artifacts))
	_, err := w.pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, 'idea', $2, $3, 'capture')
	`, id, "spec074-tp18-"+id, "h-"+id+"-"+normalizedText[:1])
	if err != nil {
		return "", fmt.Errorf("tp18IdeaWriter insert %s: %w", id, err)
	}
	w.artifacts = append(w.artifacts, id)
	return id, nil
}

// TestCaptureFallbackInviolable_TP_074_18_FacadeHookCannotBeSuppressed
// — TP-074-18 / SCN-074-A09.
//
// Live proof that the SCOPE-04A facade hook persists a fallback Idea
// for every eligible BandLow turn regardless of SST-level posture.
// Two adversarial sub-runs sweep the policy surface this test can
// influence at runtime:
//
//   - empty-manifest worst-case: zero scenarios enabled, router
//     returns ok=false → BandLow MUST still capture.
//   - second turn from same user with different text (different
//     dedup hash) MUST also capture, proving that the first capture
//     did not flip a per-user "already captured, stop" suppression
//     latch.
//
// A regression that drops the policy hook (e.g. behind an env var
// or a "test mode" check) would surface here as zero rows.
func TestCaptureFallbackInviolable_TP_074_18_FacadeHookCannotBeSuppressed(t *testing.T) {
	pool := openTP18Pool(t)
	store := capturefallback.NewPostgresStore(pool)

	cfg := capturefallback.Config{
		DedupWindow:         tp18DedupWindow,
		NormalizationPolicy: capturefallback.NormalizationPolicyV1,
		DedupHashKey:        tp18HashKey,
	}
	writer := newTP18IdeaWriter(t, pool)
	policy, err := capturefallback.New(cfg, capturefallback.NewPostgresDedupStore(pool), writer)
	if err != nil {
		t.Fatalf("capturefallback.New: %v", err)
	}

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	router := assistant.NewStubRouter()
	router.OK = false
	router.Decision = agent.RoutingDecision{Reason: agent.ReasonUnknownIntent}
	manifest, err := assistant.NewManifestForTest(map[string]assistant.ManifestEntryForTest{})
	if err != nil {
		t.Fatalf("NewManifestForTest: %v", err)
	}
	facade, err := assistant.NewFacade(
		assistant.FacadeConfig{
			BorderlineFloor:      0.75,
			AgentConfidenceFloor: 0.50,
			SourcesMax:           5,
			BodyMaxChars:         1000,
			WindowTurns:          5,
			DisambigMaxChoices:   3,
			DisambigTimeout:      30 * time.Second,
			Now:                  func() time.Time { return now },
		},
		router,
		assistant.NewStubExecutor(),
		assistant.NewMapRegistry(map[string]*agent.Scenario{}),
		manifest,
		assistant.NewInMemoryContextStore(),
		assistant.NewRecordingAudit(),
	)
	if err != nil {
		t.Fatalf("NewFacade: %v", err)
	}
	facade = facade.WithCaptureFallbackPolicy(policy)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	userID := fmt.Sprintf("tp18-user-%d", time.Now().UnixNano())

	turns := []string{
		"first inviolable thought",
		"second inviolable thought with different normalized hash",
	}
	for i, text := range turns {
		resp, err := facade.Handle(ctx, contracts.AssistantMessage{
			UserID:    userID,
			Transport: "telegram",
			Text:      text,
			Kind:      contracts.KindText,
		})
		if err != nil {
			t.Fatalf("Handle(turn %d): %v", i+1, err)
		}
		if resp.Status != contracts.StatusSavedAsIdea {
			t.Errorf("turn %d: Status = %q, want %q (inviolability regression: BandLow surface did not produce saved_as_idea)", i+1, resp.Status, contracts.StatusSavedAsIdea)
		}
	}

	fallbackCount, err := store.CountByProvenance(ctx, userID, capturefallback.ProvenanceFallback)
	if err != nil {
		t.Fatalf("CountByProvenance(fallback): %v", err)
	}
	if fallbackCount != len(turns) {
		t.Errorf("fallback count = %d, want %d (inviolability regression: facade hook suppressed at least one eligible capture)", fallbackCount, len(turns))
	}
}
