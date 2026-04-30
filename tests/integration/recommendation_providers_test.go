//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/recommendation"
	"github.com/smackerel/smackerel/internal/recommendation/provider"
	"github.com/smackerel/smackerel/internal/recommendation/reactive"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

func TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	registry := provider.NewRegistry()
	h := api.NewRecommendationHandlers(recstore.New(pool), registry, config.RecommendationsConfig{
		Ranking: config.RecommendationRankingConfig{StandardStyle: "balanced", StandardResultCount: 3},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/recommendations/requests", bytes.NewBufferString(`{"query":"quiet ramen nearby","source":"api","precision_policy":"neighborhood"}`))
	h.CreateRequest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		RequestID       string `json:"request_id"`
		Status          string `json:"status"`
		TraceID         string `json:"trace_id"`
		Recommendations []any  `json:"recommendations"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "no_providers" {
		t.Fatalf("status = %q, want no_providers", resp.Status)
	}
	if resp.RequestID == "" || resp.TraceID == "" {
		t.Fatalf("response must include request_id and trace_id: %+v", resp)
	}
	if len(resp.Recommendations) != 0 {
		t.Fatalf("recommendations = %d, want 0", len(resp.Recommendations))
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM recommendation_requests WHERE id = $1", resp.RequestID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM agent_traces WHERE trace_id = $1", resp.TraceID)
	})

	var requestStatus string
	if err := pool.QueryRow(ctx, "SELECT status FROM recommendation_requests WHERE id = $1", resp.RequestID).Scan(&requestStatus); err != nil {
		t.Fatalf("load persisted recommendation request: %v", err)
	}
	if requestStatus != "no_providers" {
		t.Fatalf("persisted status = %q, want no_providers", requestStatus)
	}

	var traceOutcome string
	if err := pool.QueryRow(ctx, "SELECT outcome FROM agent_traces WHERE trace_id = $1", resp.TraceID).Scan(&traceOutcome); err != nil {
		t.Fatalf("load persisted agent trace: %v", err)
	}
	if traceOutcome != "no_providers" {
		t.Fatalf("trace outcome = %q, want no_providers", traceOutcome)
	}
}

func TestRecommendationProviders_OneProviderOutageDegradesWithoutBlocking(t *testing.T) {
	pool := testPool(t)
	registry := provider.NewRegistry()
	google := provider.NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace})
	yelp := provider.NewFixtureProvider("fixture_yelp", "Fixture Yelp", []recommendation.Category{recommendation.CategoryPlace})
	yelp.SetHealth(provider.StatusFailing, "fixture outage")
	registry.Register(google)
	registry.Register(yelp)

	engine := reactive.NewEngine(reactive.Options{
		Store:    recstore.New(pool),
		Registry: registry,
		Config:   recommendationTestConfig(),
		Clock:    func() time.Time { return time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC) },
	})

	outcome, err := engine.Run(context.Background(), reactive.Request{
		ActorUserID:     "integration-provider-outage",
		Source:          "api",
		Query:           "quiet ramen near mission",
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
		t.Fatal("expected recommendations from the healthy provider")
	}
	for _, rec := range outcome.Recommendations {
		for _, badge := range rec.ProviderBadges {
			if badge.ProviderID == "fixture_yelp" {
				t.Fatalf("failing provider appeared in delivered badge: %+v", rec.ProviderBadges)
			}
		}
	}
	if !traceToolResultHasProviderStatus(outcome.ToolCalls, "fixture_yelp", "degraded") {
		t.Fatalf("trace tool calls did not record Yelp degradation: %+v", outcome.ToolCalls)
	}
}

func traceToolResultHasProviderStatus(calls []recstore.ToolCallRecord, providerID, status string) bool {
	for _, call := range calls {
		if call.Name != "recommendation_fetch_candidates" {
			continue
		}
		statuses, _ := call.Result["provider_status"].([]map[string]any)
		for _, providerStatus := range statuses {
			if providerStatus["provider_id"] == providerID && providerStatus["status"] == status {
				return true
			}
		}
		genericStatuses, _ := call.Result["provider_status"].([]any)
		for _, raw := range genericStatuses {
			providerStatus, _ := raw.(map[string]any)
			if providerStatus["provider_id"] == providerID && providerStatus["status"] == status {
				return true
			}
		}
	}
	return false
}
