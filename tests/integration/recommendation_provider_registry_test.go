//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/recommendation"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	"github.com/smackerel/smackerel/internal/recommendation/reactive"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

func TestRecommendationProviderRegistry_AdditionalProviderParticipatesWithoutScenarioChange(t *testing.T) {
	pool := testPool(t)
	registry := recprovider.NewRegistry()
	registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))
	registry.Register(recprovider.NewFixtureProvider("fixture_yelp", "Fixture Yelp", []recommendation.Category{recommendation.CategoryPlace}))
	registry.Register(recprovider.NewFixtureProvider("fixture_foursquare", "Fixture Foursquare", []recommendation.Category{recommendation.CategoryPlace}))

	engine := reactive.NewEngine(reactive.Options{
		Store:    recstore.New(pool),
		Registry: registry,
		Config:   recommendationTestConfig(),
		Clock:    func() time.Time { return time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC) },
	})
	outcome, err := engine.Run(context.Background(), reactive.Request{
		ActorUserID:     "integration-provider-registry",
		Source:          "api",
		Query:           "quiet ramen near mission",
		LocationRef:     "gps:37.7749,-122.4194",
		PrecisionPolicy: recommendation.PrecisionNeighborhood,
		ResultCount:     3,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if outcome.Status != "delivered" || len(outcome.Recommendations) == 0 {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if !recommendationHasProviderBadge(outcome.Recommendations[0], "fixture_foursquare") {
		t.Fatalf("new provider did not participate in delivered candidate badges: %+v", outcome.Recommendations[0].ProviderBadges)
	}
}

func recommendationHasProviderBadge(rec recstore.RenderedRecommendation, providerID string) bool {
	for _, badge := range rec.ProviderBadges {
		if badge.ProviderID == providerID {
			return true
		}
	}
	return false
}
