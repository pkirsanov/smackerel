//go:build integration

// Tests in this file cover Scope 5 SCN-039-043 and SCN-039-044, BS-027 and
// BS-031: diversity grouping caps near-duplicates from a single chain to one
// kept top-K row plus collapsed variants, and total-cost guard discloses
// unknown shipping/return facts and forbids unsupported `cheapest` claims.
package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/recommendation"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	"github.com/smackerel/smackerel/internal/recommendation/reactive"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

// TestRecommendationQuality_NearDuplicatesDiversifiedByDefault covers
// SCN-039-043 / BS-027: when three same-chain branches appear among five
// eligible candidates, the top-3 MUST contain at most one branch and
// omitted variants MUST be grouped under the parent card.
func TestRecommendationQuality_NearDuplicatesDiversifiedByDefault(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	now := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)

	makeChainFact := func(id string, title string, score float64) recprovider.Fact {
		return recprovider.Fact{
			ProviderID:          "scn043_chain",
			ProviderCandidateID: id,
			Category:            recommendation.CategoryPlace,
			Title:               title,
			RetrievedAt:         now,
			SourceUpdatedAt:     &now,
			NormalizedFact: map[string]any{
				"title":          title,
				"canonical_key":  id,
				"provider_score": score,
				"vegetarian":     true,
				"open_now":       true,
				"chain_id":       "starbucks",
				"chain_name":     "Starbucks",
			},
			Attribution:     map[string]any{"label": "Chain provider", "url": "https://example.test/" + id},
			SponsoredState:  "none",
			RestrictedFlags: map[string]any{},
		}
	}
	makeUniqueFact := func(id string, title string, score float64) recprovider.Fact {
		return recprovider.Fact{
			ProviderID:          "scn043_unique",
			ProviderCandidateID: id,
			Category:            recommendation.CategoryPlace,
			Title:               title,
			RetrievedAt:         now,
			SourceUpdatedAt:     &now,
			NormalizedFact: map[string]any{
				"title":          title,
				"canonical_key":  id,
				"provider_score": score,
				"vegetarian":     true,
				"open_now":       true,
			},
			Attribution:     map[string]any{"label": "Unique provider", "url": "https://example.test/" + id},
			SponsoredState:  "none",
			RestrictedFlags: map[string]any{},
		}
	}

	chainProvider := newStaticFactsProvider("scn043_chain", "Chain", []recommendation.Category{recommendation.CategoryPlace}, []recprovider.Fact{
		makeChainFact("scn-043-starbucks-mission", "Starbucks Mission", 0.92),
		makeChainFact("scn-043-starbucks-soma", "Starbucks SOMA", 0.88),
		makeChainFact("scn-043-starbucks-castro", "Starbucks Castro", 0.84),
	})
	uniqueProvider := newStaticFactsProvider("scn043_unique", "Unique", []recommendation.Category{recommendation.CategoryPlace}, []recprovider.Fact{
		makeUniqueFact("scn-043-fogline", "FoglineScn43 Coffee", 0.80),
		makeUniqueFact("scn-043-mission-bean", "BeanlordScn43 Mission", 0.78),
	})

	registry := recprovider.NewRegistry()
	registry.Register(chainProvider)
	registry.Register(uniqueProvider)

	cfg := recommendationTestConfig()
	cfg.Ranking.MaxFinalResults = 3
	cfg.Ranking.StandardResultCount = 3

	engine := reactive.NewEngine(reactive.Options{
		Store:    store,
		Registry: registry,
		Config:   cfg,
		Clock:    func() time.Time { return now },
	})

	outcome, err := engine.Run(context.Background(), reactive.Request{
		ActorUserID:     "scn-043-actor",
		Source:          "api",
		Query:           "scn043 coffee near mission",
		LocationRef:     "gps:37.7749,-122.4194",
		PrecisionPolicy: recommendation.PrecisionNeighborhood,
		ResultCount:     3,
	})
	if err != nil {
		t.Fatalf("engine.Run failed: %v", err)
	}
	if outcome.Status != "delivered" {
		t.Fatalf("status = %q, want delivered (recommendations: %d)", outcome.Status, len(outcome.Recommendations))
	}
	if len(outcome.Recommendations) != 3 {
		t.Fatalf("expected exactly 3 delivered recommendations after diversity grouping, got %d", len(outcome.Recommendations))
	}

	// BS-027: only ONE Starbucks branch may appear in the delivered top-3.
	starbucksCount := 0
	var starbucksDelivered recstore.RenderedRecommendation
	uniqueDeliveredTitles := map[string]bool{}
	for _, rec := range outcome.Recommendations {
		if strings.Contains(rec.Title, "Starbucks") {
			starbucksCount++
			starbucksDelivered = rec
		}
		if strings.HasPrefix(rec.Title, "FoglineScn43 ") || strings.HasPrefix(rec.Title, "BeanlordScn43 ") {
			uniqueDeliveredTitles[rec.Title] = true
		}
	}
	if starbucksCount != 1 {
		t.Fatalf("BS-027 violation: expected exactly 1 Starbucks branch in top-3, got %d (titles: %+v)", starbucksCount, deliveredTitles(outcome.Recommendations))
	}
	if !uniqueDeliveredTitles["FoglineScn43 Coffee"] || !uniqueDeliveredTitles["BeanlordScn43 Mission"] {
		t.Fatalf("BS-027 violation: expected both unique organic candidates in top-3, got %+v", uniqueDeliveredTitles)
	}

	// SCN-039-043: the kept Starbucks recommendation MUST carry a
	// quality decision with `kind: diversity`, `outcome: variants_grouped`,
	// `variant_count: 2`, and exactly two variant_keys.
	var diversityDecision map[string]any
	for _, decision := range starbucksDelivered.QualityDecisions {
		if kind, _ := decision["kind"].(string); kind == "diversity" {
			diversityDecision = decision
			break
		}
	}
	if diversityDecision == nil {
		t.Fatalf("SCN-039-043 violation: kept Starbucks recommendation missing diversity quality decision (got %+v)", starbucksDelivered.QualityDecisions)
	}
	if outcomeStr, _ := diversityDecision["outcome"].(string); outcomeStr != "variants_grouped" {
		t.Fatalf("SCN-039-043 violation: diversity decision outcome = %q, want variants_grouped", outcomeStr)
	}
	variantCount := numericField(t, diversityDecision, "variant_count")
	if int(variantCount) != 2 {
		t.Fatalf("SCN-039-043 violation: variant_count = %d, want 2 (decision: %+v)", int(variantCount), diversityDecision)
	}
	variantKeys, _ := diversityDecision["variant_keys"].([]any)
	if len(variantKeys) != 2 {
		t.Fatalf("SCN-039-043 violation: variant_keys length = %d, want 2 (decision: %+v)", len(variantKeys), diversityDecision)
	}
}

// TestRecommendationQuality_UnknownTotalCostFactsDisclosed covers
// SCN-039-044 / BS-031: a low headline price with unknown shipping/return
// facts MUST disclose unknown total-cost components and MUST NOT be
// labelled `cheapest` unless the candidate's total cost facts support it.
func TestRecommendationQuality_UnknownTotalCostFactsDisclosed(t *testing.T) {
	pool := testPool(t)
	store := recstore.New(pool)
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)

	// Cheap headline price but missing shipping/return/taxes facts. BS-031:
	// the total-cost guard must emit `disclose_unknown` for each missing
	// component AND must NOT label the candidate `cheapest` because the
	// `cheapest_supported` flag is false.
	cheapMissing := recprovider.Fact{
		ProviderID:          "scn044_deal",
		ProviderCandidateID: "scn-044-cheap",
		Category:            recommendation.CategoryDeal,
		Title:               "SCN044 Headline Cheap Deal",
		RetrievedAt:         now,
		SourceUpdatedAt:     &now,
		NormalizedFact: map[string]any{
			"title":            "SCN044 Headline Cheap Deal",
			"canonical_key":    "scn-044-cheap",
			"provider_score":   0.85,
			"vegetarian":       true,
			"open_now":         true,
			"headline_price":   42.0,
			"price_currency":   "USD",
			"cheapest_claimed": true,
			// shipping_cost / return_policy / taxes_included / total_cost
			// intentionally absent so the guard sees them all as unknown and
			// blocks the cheapest label.
		},
		Attribution:     map[string]any{"label": "Deal provider", "url": "https://example.test/cheap"},
		SponsoredState:  "none",
		RestrictedFlags: map[string]any{},
	}

	registry := recprovider.NewRegistry()
	registry.Register(newStaticFactsProvider("scn044_deal", "Deal provider", []recommendation.Category{recommendation.CategoryDeal}, []recprovider.Fact{cheapMissing}))

	cfg := recommendationTestConfig()
	cfg.Ranking.MaxFinalResults = 5

	engine := reactive.NewEngine(reactive.Options{
		Store:    store,
		Registry: registry,
		Config:   cfg,
		Clock:    func() time.Time { return now },
	})

	outcome, err := engine.Run(context.Background(), reactive.Request{
		ActorUserID:     "scn-044-actor",
		Source:          "api",
		Query:           "scn044 cheap deal vegetarian",
		LocationRef:     "gps:37.7749,-122.4194",
		PrecisionPolicy: recommendation.PrecisionNeighborhood,
		ResultCount:     3,
	})
	if err != nil {
		t.Fatalf("engine.Run failed: %v", err)
	}
	if len(outcome.Recommendations) != 1 {
		t.Fatalf("expected exactly 1 delivered recommendation, got %d", len(outcome.Recommendations))
	}
	rec := outcome.Recommendations[0]

	// BS-031 part 1: every unknown total-cost component must produce a
	// `total_cost_transparency`/`disclose_unknown` decision (one each for
	// shipping, return policy, taxes).
	wantUnknownReasons := map[string]bool{
		"shipping-cost-unknown": false,
		"return-policy-unknown": false,
		"taxes-not-included":    false,
	}
	for _, decision := range rec.QualityDecisions {
		kind, _ := decision["kind"].(string)
		outcome, _ := decision["outcome"].(string)
		reason, _ := decision["reason"].(string)
		if kind != "total_cost_transparency" || outcome != "disclose_unknown" {
			continue
		}
		if _, ok := wantUnknownReasons[reason]; ok {
			wantUnknownReasons[reason] = true
		}
	}
	for reason, found := range wantUnknownReasons {
		if !found {
			t.Fatalf("BS-031 violation: total-cost guard missing disclose_unknown decision for %s (decisions: %+v)", reason, rec.QualityDecisions)
		}
	}

	// BS-031 part 2: the candidate MUST carry a block_label_cheapest decision
	// because the headline `cheapest_claimed=true` cannot be supported when
	// total_cost / shipping / taxes are all unknown.
	var blockedLabel bool
	for _, decision := range rec.QualityDecisions {
		kind, _ := decision["kind"].(string)
		outcome, _ := decision["outcome"].(string)
		if kind == "total_cost_transparency" && outcome == "block_label_cheapest" {
			blockedLabel = true
		}
	}
	if !blockedLabel {
		t.Fatalf("BS-031 violation: expected total_cost_transparency.block_label_cheapest decision when cheapest is unsupported (decisions: %+v)", rec.QualityDecisions)
	}
}

func deliveredTitles(recs []recstore.RenderedRecommendation) []string {
	out := make([]string, 0, len(recs))
	for _, rec := range recs {
		out = append(out, rec.Title)
	}
	return out
}

func numericField(t *testing.T, decision map[string]any, field string) float64 {
	t.Helper()
	switch typed := decision[field].(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	}
	t.Fatalf("decision field %s is not numeric (got %T: %v)", field, decision[field], decision[field])
	return 0
}
