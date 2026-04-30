//go:build integration

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

func TestRecommendationConflicts_OpeningHoursConflictVisible(t *testing.T) {
	pool := testPool(t)
	registry := recprovider.NewRegistry()
	registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))
	registry.Register(recprovider.NewFixtureProvider("fixture_yelp", "Fixture Yelp", []recommendation.Category{recommendation.CategoryPlace}))

	engine := reactive.NewEngine(reactive.Options{Store: recstore.New(pool), Registry: registry, Config: recommendationTestConfig()})
	outcome, err := engine.Run(context.Background(), reactive.Request{
		ActorUserID:     "integration-conflict",
		Source:          "api",
		Query:           "ramen conflict near mission",
		LocationRef:     "gps:37.7749,-122.4194",
		PrecisionPolicy: recommendation.PrecisionNeighborhood,
		ResultCount:     1,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if outcome.Status != "delivered" || len(outcome.Recommendations) != 1 {
		t.Fatalf("unexpected conflict outcome: %+v", outcome)
	}
	rec := outcome.Recommendations[0]
	if !rec.SourceConflict {
		t.Fatalf("recommendation missing source_conflict=true: %+v", rec)
	}
	for _, rationale := range rec.Rationale {
		if strings.Contains(strings.ToLower(rationale), "open tonight") {
			t.Fatalf("conflicting source facts collapsed into settled open claim: %v", rec.Rationale)
		}
	}
	if len(rec.ProviderBadges) < 2 {
		t.Fatalf("conflict recommendation should preserve both source facts: %+v", rec.ProviderBadges)
	}
}

var _ = time.UTC