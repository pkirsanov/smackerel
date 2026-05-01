//go:build integration

// Tests in this file cover Scope 5 SCN-039-040, SCN-039-041, SCN-039-042
// and BS-023 / BS-025 / BS-026: sponsored does not buy rank, restricted
// candidates are withheld with a category-level reason, and recalled
// products never deliver as ordinary deal alerts.
package integration

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/recommendation"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	"github.com/smackerel/smackerel/internal/recommendation/reactive"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
	"github.com/smackerel/smackerel/internal/recommendation/watch"
)

// staticFactsProvider is a deterministic in-memory provider that returns a
// fixed list of facts for whatever query it receives. The integration tests
// use it to inject sponsored, restricted, and recalled fixtures without
// extending the shared FixtureProvider.
type staticFactsProvider struct {
	id          string
	display     string
	categories  []recommendation.Category
	facts       []recprovider.Fact
	healthMu    sync.Mutex
	healthState recprovider.RuntimeStatus
}

func newStaticFactsProvider(id, display string, cats []recommendation.Category, facts []recprovider.Fact) *staticFactsProvider {
	return &staticFactsProvider{
		id:          id,
		display:     display,
		categories:  cats,
		facts:       facts,
		healthState: recprovider.StatusHealthy,
	}
}

func (p *staticFactsProvider) ID() string                            { return p.id }
func (p *staticFactsProvider) DisplayName() string                   { return p.display }
func (p *staticFactsProvider) Categories() []recommendation.Category { return p.categories }
func (p *staticFactsProvider) Fetch(_ context.Context, _ recprovider.ReducedQuery) (recprovider.FactsBundle, error) {
	return recprovider.FactsBundle{ProviderID: p.id, Facts: p.facts}, nil
}
func (p *staticFactsProvider) Health(_ context.Context) recprovider.RuntimeHealth {
	p.healthMu.Lock()
	defer p.healthMu.Unlock()
	return recprovider.RuntimeHealth{
		ProviderID:   p.id,
		DisplayName:  p.display,
		Status:       p.healthState,
		ObservedAt:   time.Now().UTC(),
		CategoryList: p.categories,
	}
}

// TestRecommendationPolicy_SponsoredCannotBuyRank covers SCN-039-040 / BS-023:
// the engine's policy guard MUST label sponsored candidates and MUST NOT
// allow a sponsored candidate to outrank a stronger organic candidate when
// promotions are disabled.
func TestRecommendationPolicy_SponsoredCannotBuyRank(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)

	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	// Two organic candidates outscoring a sponsored one. SCN-039-040 / BS-023:
	// sponsored MUST never outrank an organic candidate just because it is
	// sponsored. Without a sponsored boost, the higher-scored organic facts
	// must rank ahead of the sponsored one.
	organic1 := recprovider.Fact{
		ProviderID:          "test_organic_a",
		ProviderCandidateID: "scn-040-organic-1",
		Category:            recommendation.CategoryPlace,
		Title:               "AlphaScn40 High Score Organic",
		RetrievedAt:         now,
		SourceUpdatedAt:     &now,
		NormalizedFact: map[string]any{
			"title":          "AlphaScn40 High Score Organic",
			"canonical_key":  "scn-040-organic-1",
			"provider_score": 0.91,
			"vegetarian":     true,
			"open_now":       true,
			"chain_id":       "scn40-organic-a",
		},
		Attribution:     map[string]any{"label": "Organic A", "url": "https://example.test/organic-1"},
		SponsoredState:  "none",
		RestrictedFlags: map[string]any{},
	}
	organic2 := recprovider.Fact{
		ProviderID:          "test_organic_b",
		ProviderCandidateID: "scn-040-organic-2",
		Category:            recommendation.CategoryPlace,
		Title:               "BravoScn40 Mid Score Organic",
		RetrievedAt:         now,
		SourceUpdatedAt:     &now,
		NormalizedFact: map[string]any{
			"title":          "BravoScn40 Mid Score Organic",
			"canonical_key":  "scn-040-organic-2",
			"provider_score": 0.87,
			"vegetarian":     true,
			"open_now":       true,
			"chain_id":       "scn40-organic-b",
		},
		Attribution:     map[string]any{"label": "Organic B", "url": "https://example.test/organic-2"},
		SponsoredState:  "none",
		RestrictedFlags: map[string]any{},
	}
	sponsored := recprovider.Fact{
		ProviderID:          "test_sponsored",
		ProviderCandidateID: "scn-040-sponsored",
		Category:            recommendation.CategoryPlace,
		Title:               "CharlieScn40 Sponsored Promo",
		RetrievedAt:         now,
		SourceUpdatedAt:     &now,
		NormalizedFact: map[string]any{
			"title":          "CharlieScn40 Sponsored Promo",
			"canonical_key":  "scn-040-sponsored",
			"provider_score": 0.78,
			"vegetarian":     true,
			"open_now":       true,
			"chain_id":       "scn40-sponsored",
		},
		Attribution:     map[string]any{"label": "Sponsored", "url": "https://example.test/sponsored"},
		SponsoredState:  "sponsored",
		RestrictedFlags: map[string]any{},
	}

	registry := recprovider.NewRegistry()
	registry.Register(newStaticFactsProvider("test_organic_a", "Organic A", []recommendation.Category{recommendation.CategoryPlace}, []recprovider.Fact{organic1}))
	registry.Register(newStaticFactsProvider("test_organic_b", "Organic B", []recommendation.Category{recommendation.CategoryPlace}, []recprovider.Fact{organic2}))
	registry.Register(newStaticFactsProvider("test_sponsored", "Sponsored Promo", []recommendation.Category{recommendation.CategoryPlace}, []recprovider.Fact{sponsored}))

	cfg := recommendationTestConfig()
	// Promotions disabled by default — sponsored MUST NOT receive a rank
	// boost regardless of promotions toggling.
	cfg.Policy.SponsoredPromotionsEnabled = false

	engine := reactive.NewEngine(reactive.Options{
		Store:    store,
		Registry: registry,
		Config:   cfg,
		Clock:    func() time.Time { return now },
	})

	outcome, err := engine.Run(context.Background(), reactive.Request{
		ActorUserID:     "scn-040-actor",
		Source:          "api",
		Query:           "scn040 lunch near mission",
		LocationRef:     "gps:37.7749,-122.4194",
		PrecisionPolicy: recommendation.PrecisionNeighborhood,
		ResultCount:     5,
	})
	if err != nil {
		t.Fatalf("engine.Run failed: %v", err)
	}
	if outcome.Status != "delivered" {
		t.Fatalf("status = %q, want delivered (recommendations: %d)", outcome.Status, len(outcome.Recommendations))
	}
	if len(outcome.Recommendations) < 3 {
		t.Fatalf("expected at least 3 delivered recommendations, got %d", len(outcome.Recommendations))
	}

	var sponsoredRank int
	var sponsoredFound bool
	organicRanks := map[string]int{}
	for _, rec := range outcome.Recommendations {
		if rec.Title == "CharlieScn40 Sponsored Promo" {
			sponsoredFound = true
			sponsoredRank = rec.Rank
		}
		if strings.HasPrefix(rec.Title, "AlphaScn40 ") || strings.HasPrefix(rec.Title, "BravoScn40 ") {
			organicRanks[rec.Title] = rec.Rank
		}
	}
	if !sponsoredFound {
		t.Fatalf("sponsored recommendation not delivered; expected it labeled but not boosted")
	}
	if rank, ok := organicRanks["AlphaScn40 High Score Organic"]; !ok || rank >= sponsoredRank {
		t.Fatalf("BS-023 violation: high-score organic rank=%d (found=%v) should be ranked AHEAD of sponsored rank=%d", rank, ok, sponsoredRank)
	}
	if rank, ok := organicRanks["BravoScn40 Mid Score Organic"]; !ok || rank >= sponsoredRank {
		t.Fatalf("BS-023 violation: mid-score organic rank=%d (found=%v) should be ranked AHEAD of sponsored rank=%d", rank, ok, sponsoredRank)
	}

	for _, rec := range outcome.Recommendations {
		if rec.Title != "CharlieScn40 Sponsored Promo" {
			continue
		}
		var sponsoredLabelled, hasUnauthorizedBoost bool
		for _, decision := range rec.PolicyDecisions {
			kind, _ := decision["kind"].(string)
			outcome, _ := decision["outcome"].(string)
			if kind == "sponsored" && (outcome == "label" || outcome == "withhold") {
				sponsoredLabelled = true
			}
			if kind == "sponsored" && outcome == "allow" {
				hasUnauthorizedBoost = true
			}
		}
		if !sponsoredLabelled {
			t.Fatalf("sponsored recommendation %s missing sponsored-label decision (got %+v)", rec.ID, rec.PolicyDecisions)
		}
		if hasUnauthorizedBoost {
			t.Fatalf("BS-023 violation: sponsored recommendation %s has sponsored:allow decision with promotions disabled (%+v)", rec.ID, rec.PolicyDecisions)
		}
	}
}

// TestRecommendationPolicy_RestrictedCategoryWithheldWithReason covers
// SCN-039-041 / BS-025: a candidate matching the user-blocked /
// restricted-category list MUST be withheld with a category-level reason
// even when its provider score outranks an unrestricted alternative.
func TestRecommendationPolicy_RestrictedCategoryWithheldWithReason(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)

	now := time.Date(2026, 5, 1, 12, 30, 0, 0, time.UTC)

	clean := recprovider.Fact{
		ProviderID:          "scn041_clean",
		ProviderCandidateID: "scn-041-clean",
		Category:            recommendation.CategoryPlace,
		Title:               "SCN041 Clean Cafe",
		RetrievedAt:         now,
		SourceUpdatedAt:     &now,
		NormalizedFact: map[string]any{
			"title":          "SCN041 Clean Cafe",
			"canonical_key":  "scn-041-clean",
			"provider_score": 0.85,
			"vegetarian":     true,
			"open_now":       true,
		},
		Attribution:     map[string]any{"label": "Clean", "url": "https://example.test/clean"},
		SponsoredState:  "none",
		RestrictedFlags: map[string]any{},
	}
	medical := recprovider.Fact{
		ProviderID:          "scn041_medical",
		ProviderCandidateID: "scn-041-medical",
		Category:            recommendation.CategoryPlace,
		Title:               "SCN041 Medical Clinic Cafe",
		RetrievedAt:         now,
		SourceUpdatedAt:     &now,
		NormalizedFact: map[string]any{
			"title":          "SCN041 Medical Clinic Cafe",
			"canonical_key":  "scn-041-medical",
			"provider_score": 0.95, // high score on purpose — must still be withheld
			"vegetarian":     true,
			"open_now":       true,
		},
		Attribution:     map[string]any{"label": "Medical", "url": "https://example.test/medical"},
		SponsoredState:  "none",
		RestrictedFlags: map[string]any{"restricted_category": "medical"},
	}

	registry := recprovider.NewRegistry()
	registry.Register(newStaticFactsProvider("scn041_clean", "Clean", []recommendation.Category{recommendation.CategoryPlace}, []recprovider.Fact{clean}))
	registry.Register(newStaticFactsProvider("scn041_medical", "Medical", []recommendation.Category{recommendation.CategoryPlace}, []recprovider.Fact{medical}))

	cfg := recommendationTestConfig()
	cfg.Policy.RestrictedCategories = []string{"medical"}

	engine := reactive.NewEngine(reactive.Options{
		Store:    store,
		Registry: registry,
		Config:   cfg,
		Clock:    func() time.Time { return now },
	})

	outcome, err := engine.Run(context.Background(), reactive.Request{
		ActorUserID:     "scn-041-actor",
		Source:          "api",
		Query:           "scn041 quiet cafe near mission",
		LocationRef:     "gps:37.7749,-122.4194",
		PrecisionPolicy: recommendation.PrecisionNeighborhood,
		ResultCount:     5,
	})
	if err != nil {
		t.Fatalf("engine.Run failed: %v", err)
	}

	for _, rec := range outcome.Recommendations {
		if rec.Title == "SCN041 Medical Clinic Cafe" {
			t.Fatalf("BS-025 violation: restricted (medical) candidate was DELIVERED at rank %d", rec.Rank)
		}
	}

	withheldReason := loadWithheldReasonByTitle(t, pool, outcome.ID, "SCN041 Medical Clinic Cafe")
	if !strings.Contains(withheldReason, "restricted") || !strings.Contains(withheldReason, "medical") {
		t.Fatalf("BS-025 reason violation: expected restricted:medical, got %q", withheldReason)
	}
}

// TestRecommendationPolicy_RecalledProductNotDeliveredAsDeal covers
// SCN-039-042 / BS-026: a recalled or unsafe product below a price
// threshold MUST be withheld with reason `withheld:safety-policy` and MUST
// NOT deliver as an ordinary deal alert.
func TestRecommendationPolicy_RecalledProductNotDeliveredAsDeal(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Date(2026, 5, 1, 13, 0, 0, 0, time.UTC)
	consent := newGrantedConsentRecord(t, []string{"product:price"}, []string{}, now)
	watchRecord, err := store.CreateWatch(ctx, recstore.WatchInput{
		ActorUserID:        "scn-042-actor",
		Name:               "SCN042 Recall Watch",
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
	}, consent, now)
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteWatch(context.Background(), watchRecord.ID, now) })

	evaluator := watch.NewEvaluator(watch.Options{
		Store:    store,
		Registry: recprovider.NewRegistry(),
		Clock:    func() time.Time { return now },
	})

	trigger := watch.TriggerContext{Kind: "price_check", Context: map[string]any{
		"threshold_pct": 0.15,
		"products": []any{
			map[string]any{
				"canonical_key":  "scn-042-recalled-stroller",
				"title":          "SCN042 Recalled Stroller",
				"provider_id":    "scn042_recalled",
				"baseline_price": 200.0,
				"current_price":  120.0, // 40% drop — would normally fire
				"currency":       "USD",
				"restricted_flags": map[string]any{
					"recall":        true,
					"recall_reason": "tip-over hazard",
				},
			},
		},
	}}

	outcome, err := evaluator.EvaluateWatch(ctx, watchRecord.ID, trigger)
	if err != nil {
		t.Fatalf("evaluator.EvaluateWatch failed: %v", err)
	}
	if outcome.Delivered != 0 {
		t.Fatalf("BS-026 violation: recalled product was delivered (delivered=%d)", outcome.Delivered)
	}
	if outcome.WithheldReasons["withheld:safety-policy"] == 0 {
		t.Fatalf("BS-026 violation: expected withheld:safety-policy reason, got %#v", outcome.WithheldReasons)
	}
}

// loadWithheldReasonByTitle returns the status_reason of the withheld
// recommendation row whose joined candidate title equals `title`. Fails the
// test if no such row exists.
func loadWithheldReasonByTitle(t *testing.T, pool *pgxpool.Pool, requestID, title string) string {
	t.Helper()
	var reason string
	err := pool.QueryRow(context.Background(), `
SELECT r.status_reason
FROM recommendations r
JOIN recommendation_candidates c ON c.id = r.candidate_id
WHERE r.request_id = $1 AND c.title = $2 AND r.status = 'withheld'
LIMIT 1`, requestID, title).Scan(&reason)
	if err != nil {
		t.Fatalf("loadWithheldReasonByTitle(request=%s,title=%q): %v", requestID, title, err)
	}
	return reason
}
