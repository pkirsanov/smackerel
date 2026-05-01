//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/recommendation"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
	"github.com/smackerel/smackerel/internal/recommendation/watch"
)

// TestRecommendationWatchAudit_PerWatchCountsViaAuditJoin proves
// SCN-039-051 (R-034): the per-watch operator visibility surface
// computes watch run counts by JOINING the bounded
// `smackerel_recommendation_watch_runs_total{kind,outcome}` metric
// with the persisted `recommendation_watch_runs` table on `watch_id`.
// The store-level `GetWatchAuditCounts` accessor MUST return the
// per-watch breakdown grouped by the closed status enum, and the
// returned record MUST identify the watch by its persisted `kind` so
// dashboards can correlate a per-watch row against the bounded
// Prometheus metric without ever requiring a high-cardinality label.
//
// The test seeds three watch runs with mixed outcomes (delivered,
// withheld, no_match) for the SAME watch and asserts:
//
//  1. GetWatchAuditCounts returns exists=true with the persisted kind.
//  2. Total run count equals the sum of the per-status buckets.
//  3. Each per-status bucket reflects the seeded mix exactly.
//  4. An unrelated watch's runs do NOT bleed into the per-watch view.
//  5. Adversarial: an unknown watch_id returns exists=false WITHOUT an
//     error so callers can render a 404 directly.
func TestRecommendationWatchAudit_PerWatchCountsViaAuditJoin(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Date(2026, 5, 1, 14, 0, 0, 0, time.UTC)

	registry := recprovider.NewRegistry()
	registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))

	// Seed watch A — the one whose audit counts we will assert.
	consentA := newGrantedConsentRecord(t, []string{"location:dwell:5min"}, []string{}, now)
	watchA, err := store.CreateWatch(ctx, recstore.WatchInput{
		ActorUserID:        "audit-user-a",
		Name:               "audit watch A",
		Kind:               "topic_keyword",
		Enabled:            true,
		Scope:              map[string]any{"category": "place"},
		Filters:            map[string]any{"category": "place", "query": "coffee"},
		AllowedSources:     []string{"fixture_google_places"},
		Schedule:           map[string]any{"kind": "manual"},
		MaxAlertsPerWindow: 1,
		AlertWindowSeconds: 3600,
		CooldownSeconds:    0,
		QuietHours:         map[string]any{},
		LocationPrecision:  "neighborhood",
		DeliveryChannel:    "telegram",
		QueuePolicy:        "drop",
		FreshnessSeconds:   86400,
	}, consentA, now)
	if err != nil {
		t.Fatalf("create watch A: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteWatch(context.Background(), watchA.ID, now) })

	// Seed watch B — must NEVER bleed into watch A's audit counts.
	consentB := newGrantedConsentRecord(t, []string{"location:dwell:5min"}, []string{}, now)
	watchB, err := store.CreateWatch(ctx, recstore.WatchInput{
		ActorUserID:        "audit-user-b",
		Name:               "audit watch B",
		Kind:               "topic_keyword",
		Enabled:            true,
		Scope:              map[string]any{"category": "place"},
		Filters:            map[string]any{"category": "place", "query": "coffee"},
		AllowedSources:     []string{"fixture_google_places"},
		Schedule:           map[string]any{"kind": "manual"},
		MaxAlertsPerWindow: 1,
		AlertWindowSeconds: 3600,
		CooldownSeconds:    0,
		QuietHours:         map[string]any{},
		LocationPrecision:  "neighborhood",
		DeliveryChannel:    "telegram",
		QueuePolicy:        "drop",
		FreshnessSeconds:   86400,
	}, consentB, now)
	if err != nil {
		t.Fatalf("create watch B: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteWatch(context.Background(), watchB.ID, now) })

	// Seed run mix on watch A: 2 delivered, 1 withheld, 1 no_match.
	seedRun := func(target string, status, decision string, started time.Time) {
		t.Helper()
		_, err := store.PersistWatchRun(ctx, recstore.WatchRunInput{
			WatchID:           target,
			ScenarioID:        watch.ScenarioID,
			TriggerKind:       "dwell",
			TriggerContext:    map[string]any{},
			Status:            status,
			ProviderStatus:    []map[string]any{},
			RawCandidateCount: 0,
			DeliveredCount:    0,
			WithheldCount:     0,
			DeliveryDecision:  decision,
			StartedAt:         started,
			CompletedAt:       started.Add(2 * time.Second),
		})
		if err != nil {
			t.Fatalf("seed watch run %s status=%s: %v", target, status, err)
		}
	}
	seedRun(watchA.ID, "delivered", "sent", now.Add(-30*time.Minute))
	seedRun(watchA.ID, "delivered", "sent", now.Add(-25*time.Minute))
	seedRun(watchA.ID, "withheld", "drop", now.Add(-20*time.Minute))
	seedRun(watchA.ID, "no_match", "none", now.Add(-15*time.Minute))
	// One stray run on watch B that MUST NOT show up in watch A's audit.
	seedRun(watchB.ID, "delivered", "sent", now.Add(-10*time.Minute))

	// (1) audit join returns exists=true plus the persisted kind.
	countsA, exists, err := store.GetWatchAuditCounts(ctx, watchA.ID)
	if err != nil {
		t.Fatalf("audit counts watch A: %v", err)
	}
	if !exists {
		t.Fatal("audit counts watch A: exists=false")
	}
	if countsA.WatchKind != "topic_keyword" {
		t.Fatalf("audit counts kind = %q, want topic_keyword", countsA.WatchKind)
	}
	// (2) total equals sum of buckets.
	if countsA.TotalRuns != 4 {
		t.Fatalf("audit counts total = %d, want 4 (2 delivered + 1 withheld + 1 no_match)", countsA.TotalRuns)
	}
	// (3) per-status buckets match the seeded mix.
	if countsA.DeliveredRuns != 2 {
		t.Errorf("delivered runs = %d, want 2", countsA.DeliveredRuns)
	}
	if countsA.WithheldRuns != 1 {
		t.Errorf("withheld runs = %d, want 1", countsA.WithheldRuns)
	}
	if countsA.NoMatchRuns != 1 {
		t.Errorf("no_match runs = %d, want 1", countsA.NoMatchRuns)
	}
	if countsA.LastRunAt == nil {
		t.Fatal("audit counts last_run_at must be set")
	}
	if !countsA.LastRunAt.Equal(now.Add(-15 * time.Minute)) {
		t.Errorf("last_run_at = %v, want %v", countsA.LastRunAt, now.Add(-15*time.Minute))
	}

	// (4) watch B's stray run must NOT contaminate watch A's counts and
	// watch B's own audit counts must report exactly its own runs.
	countsB, existsB, err := store.GetWatchAuditCounts(ctx, watchB.ID)
	if err != nil {
		t.Fatalf("audit counts watch B: %v", err)
	}
	if !existsB {
		t.Fatal("audit counts watch B: exists=false")
	}
	if countsB.TotalRuns != 1 {
		t.Errorf("watch B total runs = %d, want 1", countsB.TotalRuns)
	}
	if countsB.DeliveredRuns != 1 {
		t.Errorf("watch B delivered runs = %d, want 1", countsB.DeliveredRuns)
	}

	// (5) adversarial: unknown watch_id returns exists=false, no error.
	_, existsX, err := store.GetWatchAuditCounts(ctx, "rec_watch_does_not_exist")
	if err != nil {
		t.Fatalf("audit counts unknown watch returned error: %v", err)
	}
	if existsX {
		t.Fatal("audit counts unknown watch returned exists=true")
	}
}
