//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/recommendation"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	"github.com/smackerel/smackerel/internal/recommendation/reactive"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

func TestRecommendations_NoPersonalSignalsLabelOnEveryCandidate(t *testing.T) {
	pool := testPool(t)
	registry := recprovider.NewRegistry()
	registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))
	registry.Register(recprovider.NewFixtureProvider("fixture_yelp", "Fixture Yelp", []recommendation.Category{recommendation.CategoryPlace}))

	engine := reactive.NewEngine(reactive.Options{
		Store:    recstore.New(pool),
		Registry: registry,
		Config:   recommendationTestConfig(),
		Clock:    func() time.Time { return time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC) },
	})

	outcome, err := engine.Run(context.Background(), reactive.Request{
		ActorUserID:     "integration-coffee",
		Source:          "api",
		Query:           "coffee near mission",
		LocationRef:     "gps:37.7749,-122.4194",
		PrecisionPolicy: recommendation.PrecisionNeighborhood,
		ResultCount:     3,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if outcome.Status != "delivered" {
		t.Fatalf("status = %q, want delivered", outcome.Status)
	}
	if len(outcome.Recommendations) == 0 {
		t.Fatal("expected coffee recommendations")
	}
	for _, rec := range outcome.Recommendations {
		if !rec.NoPersonalSignal {
			t.Fatalf("recommendation %s was not labeled as no-personal-signal: %+v", rec.ID, rec)
		}
		if len(rec.GraphSignalRefs) != 0 {
			t.Fatalf("coffee recommendation %s has personal graph refs: %v", rec.ID, rec.GraphSignalRefs)
		}
	}
}

func recommendationTestConfig() config.RecommendationsConfig {
	return config.RecommendationsConfig{
		Enabled: true,
		LocationPrecision: config.RecommendationLocationPrecisionConfig{
			UserStandard:           "neighborhood",
			MobileStandard:         "neighborhood",
			WatchStandard:          "neighborhood",
			NeighborhoodCellSystem: "h3",
			NeighborhoodCellLevel:  9,
		},
		Ranking: config.RecommendationRankingConfig{
			MaxCandidatesPerProvider: 5,
			MaxFinalResults:          5,
			StandardResultCount:      3,
			StandardStyle:            "balanced",
			LowConfidenceThreshold:   0.4,
		},
		Policy: config.RecommendationPolicyConfig{
			SponsoredPromotionsEnabled: false,
			RestrictedCategories:       []string{"medical", "financial"},
			SafetySources:              []string{"fixture"},
		},
		Delivery: config.RecommendationDeliveryConfig{TelegramEnabled: true},
	}
}