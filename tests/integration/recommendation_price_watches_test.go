//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
	"github.com/smackerel/smackerel/internal/recommendation/watch"
)

// TestRecommendationPriceWatches_FiresOnlyOnThresholdCrossing proves
// SCN-039-034 (BS-007): a price_drop watch fires only when the current price
// crosses the threshold from above. A run with a current price still above the
// threshold persists no delivered candidate and emits a withheld:no-threshold-
// crossing reason.
func TestRecommendationPriceWatches_FiresOnlyOnThresholdCrossing(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	actor := "watches-price-user"
	clock := func() time.Time { return time.Date(2026, 4, 30, 15, 0, 0, 0, time.UTC) }
	consent := newGrantedConsentRecord(t, []string{"product:price"}, []string{}, clock())
	watchRecord, err := store.CreateWatch(ctx, recstore.WatchInput{
		ActorUserID:        actor,
		Name:               "Espresso machine price drop",
		Kind:               "price_drop",
		Enabled:            true,
		Scope:              map[string]any{"category": "product"},
		Filters:            map[string]any{"category": "product", "threshold_pct": 0.15},
		AllowedSources:     []string{},
		Schedule:           map[string]any{"kind": "price_check"},
		MaxAlertsPerWindow: 1,
		AlertWindowSeconds: 86400,
		CooldownSeconds:    0,
		QuietHours:         map[string]any{},
		LocationPrecision:  "city",
		DeliveryChannel:    "telegram",
		QueuePolicy:        "drop",
		FreshnessSeconds:   86400,
	}, consent, clock())
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteWatch(context.Background(), watchRecord.ID, clock()) })

	evaluator := watch.NewEvaluator(watch.Options{Store: store, Registry: recprovider.NewRegistry(), Clock: clock})

	// First run: current price still above threshold (10% drop on a 15%
	// threshold) — must NOT alert.
	noTrigger := watch.TriggerContext{Kind: "price_check", Context: map[string]any{
		"threshold_pct": 0.15,
		"products": []any{
			map[string]any{
				"canonical_key":  "product:espresso_machine",
				"title":          "Espresso Machine",
				"provider_id":    "fixture_price_provider",
				"baseline_price": 500.0,
				"current_price":  450.0, // 10% drop
				"currency":       "USD",
			},
		},
	}}
	noOutcome, err := evaluator.EvaluateWatch(ctx, watchRecord.ID, noTrigger)
	if err != nil {
		t.Fatalf("no-threshold evaluate: %v", err)
	}
	if noOutcome.Delivered != 0 {
		t.Fatalf("delivered = %d, want 0 below threshold crossing", noOutcome.Delivered)
	}
	if noOutcome.WithheldReasons["withheld:no-threshold-crossing"] < 1 {
		t.Fatalf("expected withheld:no-threshold-crossing reason, got %+v", noOutcome.WithheldReasons)
	}

	// Second run: current price now drops to 20% off (crosses 15% threshold) —
	// MUST deliver.
	yesTrigger := watch.TriggerContext{Kind: "price_check", Context: map[string]any{
		"threshold_pct": 0.15,
		"products": []any{
			map[string]any{
				"canonical_key":  "product:espresso_machine",
				"title":          "Espresso Machine",
				"provider_id":    "fixture_price_provider",
				"baseline_price": 500.0,
				"current_price":  400.0, // 20% drop
				"currency":       "USD",
			},
		},
	}}
	yesOutcome, err := evaluator.EvaluateWatch(ctx, watchRecord.ID, yesTrigger)
	if err != nil {
		t.Fatalf("threshold evaluate: %v", err)
	}
	if yesOutcome.Delivered < 1 {
		t.Fatalf("delivered = %d, want >= 1 after threshold crossing", yesOutcome.Delivered)
	}
}
