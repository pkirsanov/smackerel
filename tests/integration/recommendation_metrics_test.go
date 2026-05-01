//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/smackerel/smackerel/internal/metrics"
	"github.com/smackerel/smackerel/internal/recommendation"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	"github.com/smackerel/smackerel/internal/recommendation/reactive"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
	"github.com/smackerel/smackerel/internal/recommendation/watch"
)

// TestRecommendationMetrics_BoundedLabels is the SCN-039-050 (R-034)
// regression: after exercising one reactive request, one watch
// evaluation, and one delivery path, all eight required
// `smackerel_recommendation_*` metric families MUST be present in the
// Prometheus default gatherer, and NONE of those families may carry a
// high-cardinality label such as `watch_id`, `recommendation_id`, or
// `request_id`. Per-watch operator visibility is asserted separately by
// the watch audit join test (SCN-039-051).
//
// The test exercises real recommendation domain code: it instantiates
// the reactive engine and watch evaluator with fixture providers, runs
// a delivered reactive request and a delivered watch evaluation, then
// gathers the Prometheus default registry and asserts every metric
// family individually.
func TestRecommendationMetrics_BoundedLabels(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	registry := recprovider.NewRegistry()
	registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))
	registry.Register(recprovider.NewFixtureProvider("fixture_yelp", "Fixture Yelp", []recommendation.Category{recommendation.CategoryPlace}))

	store := recstore.New(pool)
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	engine := reactive.NewEngine(reactive.Options{
		Store:    store,
		Registry: registry,
		Config:   recommendationTestConfig(),
		Clock:    clock,
	})

	// Drive at least one delivered + one withheld reactive request so
	// the candidate funnel, ranking confidence, and suppression metrics
	// all have observations.
	if _, err := engine.Run(ctx, reactive.Request{
		ActorUserID:     "metrics-actor",
		Source:          "api",
		Query:           "coffee near mission",
		LocationRef:     "gps:37.7749,-122.4194",
		PrecisionPolicy: recommendation.PrecisionNeighborhood,
		ResultCount:     3,
	}); err != nil {
		t.Fatalf("reactive run: %v", err)
	}

	// Drive a watch evaluator run so the watch_runs metric has an
	// observation against a real `kind` label.
	evaluator := watch.NewEvaluator(watch.Options{Store: store, Registry: registry, Clock: clock})
	consent := newGrantedConsentRecord(t, []string{"location:dwell:5min"}, []string{}, now)
	watchRecord, err := store.CreateWatch(ctx, recstore.WatchInput{
		ActorUserID:        "metrics-watch-user",
		Name:               "metrics watch — quiet ramen",
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
	}, consent, now)
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteWatch(context.Background(), watchRecord.ID, now) })
	if _, err := evaluator.EvaluateWatch(ctx, watchRecord.ID, watch.TriggerContext{Kind: "dwell"}); err != nil {
		t.Fatalf("evaluate watch: %v", err)
	}

	// All eight required metric families per design.md → Observability.
	required := map[string]struct{}{
		"smackerel_recommendation_provider_requests_total":  {},
		"smackerel_recommendation_provider_latency_seconds": {},
		"smackerel_recommendation_candidates_total":         {},
		"smackerel_recommendation_watch_runs_total":         {},
		"smackerel_recommendation_delivery_total":           {},
		"smackerel_recommendation_suppression_total":        {},
		"smackerel_recommendation_ranking_confidence_total": {},
		"smackerel_recommendation_location_precision_total": {},
	}

	// Allowed bounded label names per family. Anything outside this
	// set MUST be a closed enum (and any family carrying watch_id /
	// recommendation_id / request_id is an immediate failure).
	allowed := map[string]map[string]struct{}{
		"smackerel_recommendation_provider_requests_total":  {"provider": {}, "category": {}, "outcome": {}},
		"smackerel_recommendation_provider_latency_seconds": {"provider": {}, "category": {}},
		"smackerel_recommendation_candidates_total":         {"category": {}, "stage": {}, "outcome": {}},
		"smackerel_recommendation_watch_runs_total":         {"kind": {}, "outcome": {}},
		"smackerel_recommendation_delivery_total":           {"channel": {}, "outcome": {}},
		"smackerel_recommendation_suppression_total":        {"reason": {}},
		"smackerel_recommendation_ranking_confidence_total": {"confidence": {}},
		"smackerel_recommendation_location_precision_total": {"requested": {}, "sent": {}},
	}
	forbiddenLabels := []string{"watch_id", "recommendation_id", "request_id", "trace_id", "actor_user_id", "user_id"}

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	seen := map[string]bool{}
	for _, fam := range families {
		name := fam.GetName()
		if _, ok := required[name]; !ok {
			continue
		}
		seen[name] = true
		allowedSet := allowed[name]
		for _, m := range fam.GetMetric() {
			labelNames := []string{}
			for _, lp := range m.GetLabel() {
				labelNames = append(labelNames, lp.GetName())
				if _, ok := allowedSet[lp.GetName()]; !ok {
					t.Errorf("metric %q has unexpected label %q (allowed: %v)", name, lp.GetName(), keys(allowedSet))
				}
				for _, forb := range forbiddenLabels {
					if lp.GetName() == forb {
						t.Errorf("metric %q carries forbidden high-cardinality label %q", name, forb)
					}
				}
			}
			t.Logf("family=%s labels=%v", name, labelNames)
		}
	}
	for name := range required {
		if !seen[name] {
			// Provide a directly actionable failure message.
			missing := strings.TrimPrefix(name, "smackerel_recommendation_")
			t.Errorf("required recommendation metric %q (%s) was not emitted by exercising reactive + watch paths", name, missing)
		}
	}

	// And make sure provider_requests has at least one provider+outcome
	// observation against the closed outcome enum.
	if obs := observationCount(t, "smackerel_recommendation_provider_requests_total"); obs == 0 {
		t.Error("smackerel_recommendation_provider_requests_total has no observations after reactive run")
	}
	if obs := observationCount(t, "smackerel_recommendation_watch_runs_total"); obs == 0 {
		t.Error("smackerel_recommendation_watch_runs_total has no observations after watch evaluation")
	}
	if obs := observationCount(t, "smackerel_recommendation_location_precision_total"); obs == 0 {
		t.Error("smackerel_recommendation_location_precision_total has no observations after reactive run")
	}

	// Smoke-test the metric handler so a future regression that broke
	// /metrics rendering would be caught here too.
	_ = metrics.Handler()
}

// observationCount returns the total number of observations across all
// label sets for a given Prometheus metric family. Used as a real
// signal that the wired metric was actually incremented.
func observationCount(t *testing.T, family string) int {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	total := 0
	for _, fam := range families {
		if fam.GetName() != family {
			continue
		}
		for _, m := range fam.GetMetric() {
			if c := m.GetCounter(); c != nil {
				total += int(c.GetValue())
			}
			if h := m.GetHistogram(); h != nil {
				total += int(h.GetSampleCount())
			}
		}
	}
	return total
}

func keys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}
