//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/recommendation"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

// readinessProductionProvider is a production-class provider double for the live
// readiness proof. It does NOT implement the typed fixture marker, so the
// availability gate classifies it as production and lets one healthy instance
// satisfy first readiness.
type readinessProductionProvider struct {
	id     string
	cats   []recommendation.Category
	status recprovider.RuntimeStatus
}

func (p readinessProductionProvider) ID() string          { return p.id }
func (p readinessProductionProvider) DisplayName() string { return p.id }
func (p readinessProductionProvider) Categories() []recommendation.Category {
	return append([]recommendation.Category(nil), p.cats...)
}
func (p readinessProductionProvider) Fetch(context.Context, recprovider.ReducedQuery) (recprovider.FactsBundle, error) {
	return recprovider.FactsBundle{ProviderID: p.id}, nil
}
func (p readinessProductionProvider) Health(context.Context) recprovider.RuntimeHealth {
	return recprovider.RuntimeHealth{ProviderID: p.id, DisplayName: p.id, Status: p.status, ObservedAt: time.Now().UTC(), CategoryList: p.cats}
}

type availabilityEnvelope struct {
	Providers    []map[string]any `json:"providers"`
	Availability struct {
		Enabled   bool   `json:"enabled"`
		Ready     bool   `json:"ready"`
		State     string `json:"state"`
		Cause     string `json:"cause"`
		Category  string `json:"category"`
		Operation string `json:"operation"`
		Counts    struct {
			Declared          int `json:"declared"`
			Eligible          int `json:"eligible"`
			HealthyEligible   int `json:"healthy_eligible"`
			UnhealthyEligible int `json:"unhealthy_eligible"`
			Fixtures          int `json:"fixtures"`
		} `json:"counts"`
	} `json:"availability"`
}

func listProvidersAvailability(t *testing.T, h *api.RecommendationHandlers) availabilityEnvelope {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/recommendations/providers", nil)
	h.ListProviders(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var env availabilityEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode providers response: %v (body=%s)", err, rec.Body.String())
	}
	return env
}

// TestRecommendationAvailabilityReadiness_LiveStatusReflectsRealProviderState is
// the live BUG-039-005 proof: the recommendation status endpoint derives its
// ready/not-ready verdict from the availability gate over the REAL configured
// provider registry, on the disposable stack, with no interception.
//
//   - Enabled + zero configured providers  -> honest NOT-ready (the exact bug).
//   - Enabled + only a healthy fixture      -> still NOT-ready (fixtures excluded;
//     no false-ready from provider cardinality).
//   - Enabled + one healthy production       -> ready (one eligible healthy
//     provider suffices; no second required).
func TestRecommendationAvailabilityReadiness_LiveStatusReflectsRealProviderState(t *testing.T) {
	pool := testPool(t)
	cfg := config.RecommendationsConfig{
		Enabled: true,
		Ranking: config.RecommendationRankingConfig{StandardStyle: "balanced", StandardResultCount: 3},
	}

	t.Run("enabled zero providers is honest not ready", func(t *testing.T) {
		registry := recprovider.NewRegistry()
		h := api.NewRecommendationHandlers(recstore.New(pool), registry, cfg)
		env := listProvidersAvailability(t, h)

		if !env.Availability.Enabled {
			t.Fatalf("availability.enabled = false, want true (capability is enabled)")
		}
		if env.Availability.Ready {
			t.Fatalf("availability.ready = true with zero providers — FALSE READY regression; state=%s cause=%s", env.Availability.State, env.Availability.Cause)
		}
		if env.Availability.State != "unavailable" {
			t.Fatalf("availability.state = %q, want unavailable", env.Availability.State)
		}
		if env.Availability.Cause != "zero_configured_providers" {
			t.Fatalf("availability.cause = %q, want zero_configured_providers", env.Availability.Cause)
		}
		if env.Availability.Counts.Eligible != 0 || env.Availability.Counts.HealthyEligible != 0 {
			t.Fatalf("counts eligible=%d healthy=%d, want 0/0", env.Availability.Counts.Eligible, env.Availability.Counts.HealthyEligible)
		}
	})

	t.Run("enabled with only a healthy fixture is still not ready", func(t *testing.T) {
		registry := recprovider.NewRegistry()
		registry.Register(recprovider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))
		h := api.NewRecommendationHandlers(recstore.New(pool), registry, cfg)
		env := listProvidersAvailability(t, h)

		if env.Availability.Ready {
			t.Fatalf("availability.ready = true with only a fixture provider — fixtures MUST NOT dilute readiness; state=%s cause=%s", env.Availability.State, env.Availability.Cause)
		}
		if env.Availability.Cause != "zero_configured_providers" {
			t.Fatalf("availability.cause = %q, want zero_configured_providers (fixture excluded)", env.Availability.Cause)
		}
		if env.Availability.Counts.Fixtures != 1 {
			t.Fatalf("counts.fixtures = %d, want 1 (fixture observed but excluded)", env.Availability.Counts.Fixtures)
		}
		if env.Availability.Counts.Eligible != 0 {
			t.Fatalf("counts.eligible = %d, want 0 (fixture is not eligible)", env.Availability.Counts.Eligible)
		}
	})

	t.Run("enabled with one healthy production provider is ready", func(t *testing.T) {
		registry := recprovider.NewRegistry()
		registry.Register(readinessProductionProvider{id: "google_places", cats: []recommendation.Category{recommendation.CategoryPlace}, status: recprovider.StatusHealthy})
		h := api.NewRecommendationHandlers(recstore.New(pool), registry, cfg)
		env := listProvidersAvailability(t, h)

		if !env.Availability.Ready {
			t.Fatalf("availability.ready = false with one healthy production provider — want ready; state=%s cause=%s", env.Availability.State, env.Availability.Cause)
		}
		if env.Availability.State != "available" {
			t.Fatalf("availability.state = %q, want available", env.Availability.State)
		}
		if env.Availability.Counts.HealthyEligible != 1 {
			t.Fatalf("counts.healthy_eligible = %d, want 1", env.Availability.Counts.HealthyEligible)
		}
	})
}
