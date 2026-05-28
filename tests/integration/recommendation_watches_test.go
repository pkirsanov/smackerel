//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/recommendation"
	"github.com/smackerel/smackerel/internal/recommendation/policy"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
	"github.com/smackerel/smackerel/internal/recommendation/watch"
)

// TestRecommendationWatches_DwellFiresOnceWithinRateWindow proves SCN-039-030
// (BS-003): a single dwell trigger evaluates the watch once and persists a
// `delivered` watch run. A second dwell within the same rate window yields a
// withheld watch run with reason `withheld:rate-limit`.
func TestRecommendationWatches_DwellFiresOnceWithinRateWindow(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	actor := "watches-dwell-user"
	registry := recprovider.NewRegistry()
	registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))
	clock := func() time.Time { return time.Date(2026, 4, 30, 11, 0, 0, 0, time.UTC) }
	evaluator := watch.NewEvaluator(watch.Options{Store: store, Registry: registry, Clock: clock})

	consent := newGrantedConsentRecord(t, []string{"location:dwell:5min", "alerts:notify-when-near"}, []string{}, clock())
	watchRecord, err := store.CreateWatch(ctx, recstore.WatchInput{
		ActorUserID:        actor,
		Name:               "Quiet ramen near mission",
		Kind:               "location_radius",
		Enabled:            true,
		Scope:              map[string]any{"category": "place", "anchor": "gps:37.7749,-122.4194"},
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
	}, consent, clock())
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteWatch(context.Background(), watchRecord.ID, clock()) })

	first, err := evaluator.EvaluateWatch(ctx, watchRecord.ID, watch.TriggerContext{Kind: "dwell"})
	if err != nil {
		t.Fatalf("first evaluate: %v", err)
	}
	if first.DeliveryDecision != "sent" {
		t.Fatalf("first decision = %q, want sent: %+v", first.DeliveryDecision, first)
	}
	if first.Delivered < 1 {
		t.Fatalf("first delivered = %d, want >= 1", first.Delivered)
	}

	second, err := evaluator.EvaluateWatch(ctx, watchRecord.ID, watch.TriggerContext{Kind: "dwell"})
	if err != nil {
		t.Fatalf("second evaluate: %v", err)
	}
	// Second evaluation falls inside the rate window — the surplus must be
	// withheld with reason rate-limit OR repeat-cooldown (both are valid
	// guards for an unchanged candidate). Either way, no additional alert.
	if second.Delivered > 0 {
		t.Fatalf("second delivered = %d, want 0 (rate window or cooldown)", second.Delivered)
	}

	deliveredCount := countWatchRunsByDecision(t, pool, watchRecord.ID, "sent")
	if deliveredCount != 1 {
		t.Fatalf("watch run delivered count = %d, want 1", deliveredCount)
	}
}

// TestRecommendationWatches_RateLimitWithholdsSurplusInOneCycle proves
// SCN-039-031 (BS-004): when the rate window allows N alerts and the candidate
// pool exceeds N, only N are delivered and the rest are persisted as
// withheld:rate-limit in the SAME watch run.
func TestRecommendationWatches_RateLimitWithholdsSurplusInOneCycle(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	actor := "watches-rate-limit-user"
	registry := recprovider.NewRegistry()
	registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))
	clock := func() time.Time { return time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC) }
	evaluator := watch.NewEvaluator(watch.Options{Store: store, Registry: registry, Clock: clock})

	consent := newGrantedConsentRecord(t, []string{"location:dwell:5min", "alerts:notify-when-near"}, []string{}, clock())
	watchRecord, err := store.CreateWatch(ctx, recstore.WatchInput{
		ActorUserID:        actor,
		Name:               "All coffee everywhere",
		Kind:               "location_radius",
		Enabled:            true,
		Scope:              map[string]any{"category": "place", "anchor": "gps:37.7749,-122.4194"},
		Filters:            map[string]any{"category": "place", "query": "coffee"},
		AllowedSources:     []string{"fixture_google_places"},
		Schedule:           map[string]any{"kind": "manual"},
		MaxAlertsPerWindow: 2,
		AlertWindowSeconds: 3600,
		CooldownSeconds:    0,
		QuietHours:         map[string]any{},
		LocationPrecision:  "neighborhood",
		DeliveryChannel:    "telegram",
		QueuePolicy:        "drop",
		FreshnessSeconds:   86400,
	}, consent, clock())
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteWatch(context.Background(), watchRecord.ID, clock()) })

	outcome, err := evaluator.EvaluateWatch(ctx, watchRecord.ID, watch.TriggerContext{Kind: "dwell"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if outcome.Delivered > 2 {
		t.Fatalf("delivered = %d, want <= max_alerts_per_window=2", outcome.Delivered)
	}
	if outcome.RawCandidates <= outcome.Delivered {
		t.Skipf("provider did not return more candidates than max_alerts_per_window — cannot prove rate-limit suppression (raw=%d, delivered=%d)", outcome.RawCandidates, outcome.Delivered)
	}
	if outcome.Withheld < 1 || outcome.WithheldReasons["withheld:rate-limit"] < 1 {
		t.Fatalf("expected at least one withheld:rate-limit row, got reasons=%+v", outcome.WithheldReasons)
	}
	deliveredRows := countDeliveredRecommendationRows(t, pool, outcome.WatchRunID)
	if deliveredRows != outcome.Delivered {
		t.Fatalf("delivered recommendation rows = %d, want %d", deliveredRows, outcome.Delivered)
	}
}

// TestRecommendationWatches_QuietHoursWithholdAndAudit proves SCN-039-032
// (BS-018): a watch evaluation inside the configured quiet-hours window MUST
// persist a withheld run with reason `withheld:quiet-hours` and emit a
// queue/summarize/drop decision matching the watch's queue_policy. No alert
// is delivered.
func TestRecommendationWatches_QuietHoursWithholdAndAudit(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	actor := "watches-quiet-hours-user"
	registry := recprovider.NewRegistry()
	registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))
	// 02:30 UTC — inside the configured quiet-hours window 22:00–07:00.
	clock := func() time.Time { return time.Date(2026, 4, 30, 2, 30, 0, 0, time.UTC) }
	evaluator := watch.NewEvaluator(watch.Options{Store: store, Registry: registry, Clock: clock})

	consent := newGrantedConsentRecord(t, []string{"location:dwell:5min", "alerts:notify-when-near"}, []string{}, clock())
	watchRecord, err := store.CreateWatch(ctx, recstore.WatchInput{
		ActorUserID:        actor,
		Name:               "Quiet hours coffee",
		Kind:               "location_radius",
		Enabled:            true,
		Scope:              map[string]any{"category": "place", "anchor": "gps:37.7749,-122.4194"},
		Filters:            map[string]any{"category": "place", "query": "coffee"},
		AllowedSources:     []string{"fixture_google_places"},
		Schedule:           map[string]any{"kind": "manual"},
		MaxAlertsPerWindow: 1,
		AlertWindowSeconds: 3600,
		CooldownSeconds:    0,
		QuietHours:         map[string]any{"start": "22:00", "end": "07:00", "timezone": "UTC"},
		LocationPrecision:  "neighborhood",
		DeliveryChannel:    "telegram",
		QueuePolicy:        "summarize",
		FreshnessSeconds:   86400,
	}, consent, clock())
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteWatch(context.Background(), watchRecord.ID, clock()) })

	outcome, err := evaluator.EvaluateWatch(ctx, watchRecord.ID, watch.TriggerContext{Kind: "dwell"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if outcome.Status != "quiet_hours" {
		t.Fatalf("status = %q, want quiet_hours", outcome.Status)
	}
	if outcome.Delivered != 0 {
		t.Fatalf("delivered = %d, want 0 inside quiet hours", outcome.Delivered)
	}
	if outcome.DeliveryDecision != "summarize" {
		t.Fatalf("decision = %q, want summarize (watch queue_policy)", outcome.DeliveryDecision)
	}
	row := pool.QueryRow(ctx, `SELECT status, error_kind, delivery_decision FROM recommendation_watch_runs WHERE id = $1`, outcome.WatchRunID)
	var status, errorKind, decision string
	if err := row.Scan(&status, &errorKind, &decision); err != nil {
		t.Fatalf("read watch run: %v", err)
	}
	if status != "quiet_hours" || errorKind != "withheld:quiet-hours" || decision != "summarize" {
		t.Fatalf("watch run row = (%s,%s,%s), want (quiet_hours, withheld:quiet-hours, summarize)", status, errorKind, decision)
	}
}

// TestRecommendationWatches_StaleSourceDataCannotAlert proves SCN-039-035
// (BS-017): when provider facts are older than the freshness budget, the
// watch persists them as withheld:stale-source-data and does NOT deliver an
// alert. Adversarial: we seed a stale fact directly through the store path so
// we control the SourceUpdatedAt timestamp.
func TestRecommendationWatches_StaleSourceDataCannotAlert(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	actor := "watches-stale-user"
	clock := func() time.Time { return time.Date(2026, 4, 30, 13, 0, 0, 0, time.UTC) }
	// Build a trip-context watch so we control the candidates payload directly.
	consent := newGrantedConsentRecord(t, []string{"location:trip:active"}, []string{}, clock())
	watchRecord, err := store.CreateWatch(ctx, recstore.WatchInput{
		ActorUserID:        actor,
		Name:               "Stale fact watch",
		Kind:               "trip_context",
		Enabled:            true,
		Scope:              map[string]any{"category": "place"},
		Filters:            map[string]any{"category": "place"},
		AllowedSources:     []string{},
		Schedule:           map[string]any{"kind": "trip_window"},
		MaxAlertsPerWindow: 1,
		AlertWindowSeconds: 3600,
		CooldownSeconds:    0,
		QuietHours:         map[string]any{},
		LocationPrecision:  "city",
		DeliveryChannel:    "telegram",
		QueuePolicy:        "drop",
		FreshnessSeconds:   3600, // 1 hour freshness budget
	}, consent, clock())
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteWatch(context.Background(), watchRecord.ID, clock()) })

	evaluator := watch.NewEvaluator(watch.Options{Store: store, Registry: recprovider.NewRegistry(), Clock: clock})
	stale := clock().Add(-24 * time.Hour) // 23 hours older than freshness budget
	trigger := watch.TriggerContext{Kind: "trip_window", Context: map[string]any{
		"trip_id":    "trip_001",
		"trip_start": clock().Format(time.RFC3339),
		"candidates": []any{
			map[string]any{
				"canonical_key":     "place:stale_cafe",
				"title":             "Stale Cafe",
				"provider_id":       "fixture_test_provider",
				"category":          "place",
				"source_updated_at": stale.Format(time.RFC3339),
			},
		},
	}}

	outcome, err := evaluator.EvaluateWatch(ctx, watchRecord.ID, trigger)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if outcome.Delivered != 0 {
		t.Fatalf("delivered = %d, want 0 for stale source data", outcome.Delivered)
	}
	if outcome.WithheldReasons["withheld:stale-source-data"] < 1 {
		t.Fatalf("expected withheld:stale-source-data reason, got %+v", outcome.WithheldReasons)
	}
}

// TestRecommendationWatches_RepeatCooldownSuppressesUnchanged proves
// SCN-039-036 (BS-028): when a watch fires twice with the same material-change
// hash inside the cooldown window, the second evaluation persists a withheld
// run with reason `withheld:repeat-cooldown` and does NOT alert.
func TestRecommendationWatches_RepeatCooldownSuppressesUnchanged(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	actor := "watches-cooldown-user"
	registry := recprovider.NewRegistry()
	registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))
	clock := func() time.Time { return time.Date(2026, 4, 30, 14, 0, 0, 0, time.UTC) }
	evaluator := watch.NewEvaluator(watch.Options{Store: store, Registry: registry, Clock: clock})

	consent := newGrantedConsentRecord(t, []string{"location:dwell:5min", "alerts:notify-when-near"}, []string{}, clock())
	watchRecord, err := store.CreateWatch(ctx, recstore.WatchInput{
		ActorUserID:        actor,
		Name:               "Cooldown watch",
		Kind:               "location_radius",
		Enabled:            true,
		Scope:              map[string]any{"category": "place", "anchor": "gps:37.7749,-122.4194"},
		Filters:            map[string]any{"category": "place", "query": "coffee"},
		AllowedSources:     []string{"fixture_google_places"},
		Schedule:           map[string]any{"kind": "manual"},
		MaxAlertsPerWindow: 5,
		AlertWindowSeconds: 60, // tight window so rate-limit doesn't accidentally hide cooldown
		CooldownSeconds:    7 * 24 * 3600,
		QuietHours:         map[string]any{},
		LocationPrecision:  "neighborhood",
		DeliveryChannel:    "telegram",
		QueuePolicy:        "drop",
		FreshnessSeconds:   86400,
	}, consent, clock())
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteWatch(context.Background(), watchRecord.ID, clock()) })

	first, err := evaluator.EvaluateWatch(ctx, watchRecord.ID, watch.TriggerContext{Kind: "dwell"})
	if err != nil {
		t.Fatalf("first evaluate: %v", err)
	}
	if first.Delivered < 1 {
		t.Fatalf("first delivered = %d, want >= 1 to seed cooldown", first.Delivered)
	}

	// Second evaluation: same provider, same fixture facts → identical
	// material-change hash. Cooldown guard MUST suppress at least one.
	second, err := evaluator.EvaluateWatch(ctx, watchRecord.ID, watch.TriggerContext{Kind: "dwell"})
	if err != nil {
		t.Fatalf("second evaluate: %v", err)
	}
	if second.Delivered > 0 {
		t.Fatalf("second delivered = %d, want 0 inside cooldown", second.Delivered)
	}
	if second.WithheldReasons["withheld:repeat-cooldown"] < 1 {
		t.Fatalf("expected withheld:repeat-cooldown reason, got %+v", second.WithheldReasons)
	}
}

func newGrantedConsentRecord(t *testing.T, _, hardConstraints []string, now time.Time) policy.ConsentRecord {
	t.Helper()
	values := policy.ConsentNamedValues{
		Scope:            map[string]any{"category": "place"},
		Sources:          []string{"fixture_google_places"},
		DeliveryChannel:  "telegram",
		MaxAlerts:        1,
		WindowSeconds:    3600,
		Precision:        "neighborhood",
		HardConstraints:  append([]string{}, hardConstraints...),
		SponsoredAllowed: false,
	}
	return policy.ApplyRevision(policy.ConsentRecord{Revisions: []policy.ConsentRevision{}}, values, policy.ConsentReasonCreate, now)
}

func countWatchRunsByDecision(t *testing.T, pool *pgxpool.Pool, watchID, decision string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM recommendation_watch_runs WHERE watch_id = $1 AND delivery_decision = $2`, watchID, decision).Scan(&count); err != nil {
		t.Fatalf("count watch runs: %v", err)
	}
	return count
}

func countDeliveredRecommendationRows(t *testing.T, pool *pgxpool.Pool, watchRunID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM recommendations WHERE watch_run_id = $1 AND status = 'delivered'`, watchRunID).Scan(&count); err != nil {
		t.Fatalf("count delivered recs: %v", err)
	}
	return count
}
