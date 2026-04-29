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

func TestRecommendationPrivacy_PrecisionReducedBeforeProviderCall(t *testing.T) {
	pool := testPool(t)
	fixture := recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace})
	registry := recprovider.NewRegistry()
	registry.Register(fixture)

	engine := reactive.NewEngine(reactive.Options{
		Store:    recstore.New(pool),
		Registry: registry,
		Config:   recommendationTestConfig(),
		Clock:    func() time.Time { return time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC) },
	})

	outcome, err := engine.Run(context.Background(), reactive.Request{
		ActorUserID:     "integration-privacy",
		Source:          "api",
		Query:           "ramen near me",
		LocationRef:     "gps:37.7749,-122.4194",
		PrecisionPolicy: recommendation.PrecisionNeighborhood,
		ResultCount:     3,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	toolNames := outcome.ToolCallNames()
	reduceIndex := indexOf(toolNames, "recommendation_reduce_location")
	fetchIndex := indexOf(toolNames, "recommendation_fetch_candidates")
	if reduceIndex < 0 || fetchIndex < 0 || reduceIndex >= fetchIndex {
		t.Fatalf("tool order = %v, want reduce before fetch", toolNames)
	}

	queries := fixture.ObservedQueries()
	if len(queries) == 0 {
		t.Fatal("fixture provider recorded no provider-facing query")
	}
	query := queries[0]
	if query.PrecisionPolicy != recommendation.PrecisionNeighborhood {
		t.Fatalf("provider precision = %q, want neighborhood", query.PrecisionPolicy)
	}
	serialized := query.Query + " " + query.Geometry.CellID + " " + query.Geometry.Label
	for _, raw := range []string{"37.7749", "122.4194", "gps:"} {
		if strings.Contains(serialized, raw) {
			t.Fatalf("provider-facing query leaked raw location token %q in %+v", raw, query)
		}
	}
}

func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}