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

func TestRecommendationAttribution_BadgeAndLinkPersisted(t *testing.T) {
	pool := testPool(t)
	registry := recprovider.NewRegistry()
	registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))

	engine := reactive.NewEngine(reactive.Options{Store: recstore.New(pool), Registry: registry, Config: recommendationTestConfig()})
	outcome, err := engine.Run(context.Background(), reactive.Request{
		ActorUserID:     "integration-attribution",
		Source:          "api",
		Query:           "quiet ramen near mission",
		LocationRef:     "gps:37.7749,-122.4194",
		PrecisionPolicy: recommendation.PrecisionNeighborhood,
		ResultCount:     1,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(outcome.Recommendations) != 1 {
		t.Fatalf("recommendation count = %d, want 1", len(outcome.Recommendations))
	}
	rec := outcome.Recommendations[0]
	if len(rec.Attribution) == 0 {
		t.Fatalf("recommendation missing attribution badges: %+v", rec)
	}
	for _, badge := range rec.Attribution {
		if badge.Label == "" || badge.URL == "" {
			t.Fatalf("attribution badge missing label/link: %+v", badge)
		}
	}
}

var _ = time.UTC
