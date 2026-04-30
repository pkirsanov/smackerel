//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWhyRegression_BS010_NoProviderCall(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	seedRamenSignalArtifact(t)

	recommendationID, requestID := createWhySeedRecommendation(t, cfg)
	providerFactsBefore := countProviderFactsForRequest(t, requestID)

	resp, err := apiGet(cfg, "/api/recommendations/"+recommendationID+"/why")
	if err != nil {
		t.Fatalf("why request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read why body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		RecommendationID    string           `json:"recommendation_id"`
		TraceID             string           `json:"trace_id"`
		OriginTraceID       string           `json:"origin_trace_id"`
		ProviderCallsIssued bool             `json:"provider_calls_issued"`
		Explanation         []string         `json:"explanation"`
		ProviderFacts       []map[string]any `json:"provider_facts"`
		PersonalSignals     []string         `json:"personal_signals"`
		PolicyDecisions     []map[string]any `json:"policy_decisions"`
		QualityDecisions    []map[string]any `json:"quality_decisions"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse why response: %v; body=%s", err, string(body))
	}
	if parsed.RecommendationID != recommendationID || parsed.TraceID == "" || parsed.OriginTraceID == "" {
		t.Fatalf("why identity is incomplete: %+v", parsed)
	}
	if parsed.ProviderCallsIssued {
		t.Fatalf("why response claims provider calls were issued: %+v", parsed)
	}
	if len(parsed.ProviderFacts) == 0 || len(parsed.PersonalSignals) == 0 || len(parsed.PolicyDecisions) == 0 || len(parsed.QualityDecisions) == 0 {
		t.Fatalf("why response missing persisted provenance: %+v", parsed)
	}
	if !whyExplanationMentions(parsed.Explanation, "Provider facts") || !whyExplanationMentions(parsed.Explanation, "ART-123") {
		t.Fatalf("why explanation does not cite provider facts and ART-123: %+v", parsed.Explanation)
	}
	assertWhyTraceHasNoProviderCalls(t, parsed.TraceID)
	if after := countProviderFactsForRequest(t, requestID); after != providerFactsBefore {
		t.Fatalf("why call changed provider fact count: before=%d after=%d", providerFactsBefore, after)
	}
}

func createWhySeedRecommendation(t *testing.T, cfg e2eConfig) (string, string) {
	t.Helper()
	resp, err := apiPostJSON(cfg, "/api/recommendations/requests", map[string]any{
		"query":            "quiet ramen near mission",
		"source":           "api",
		"location_ref":     "gps:37.7749,-122.4194",
		"precision_policy": "neighborhood",
		"result_count":     3,
	})
	if err != nil {
		t.Fatalf("recommendation request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read recommendation body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		RequestID       string `json:"request_id"`
		Recommendations []struct {
			ID string `json:"id"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse recommendation response: %v; body=%s", err, string(body))
	}
	if parsed.RequestID == "" || len(parsed.Recommendations) == 0 || parsed.Recommendations[0].ID == "" {
		t.Fatalf("seed recommendation response incomplete: %+v", parsed)
	}
	return parsed.Recommendations[0].ID, parsed.RequestID
}

func assertWhyTraceHasNoProviderCalls(t *testing.T, traceID string) {
	t.Helper()
	pool := e2ePool(t)
	var providerCalls int
	if err := pool.QueryRow(context.Background(), `
SELECT COUNT(*)
FROM agent_tool_calls
WHERE trace_id = $1
  AND (tool_name = 'recommendation_fetch_candidates' OR side_effect_class = 'external')`, traceID).Scan(&providerCalls); err != nil {
		t.Fatalf("count why provider calls: %v", err)
	}
	if providerCalls != 0 {
		t.Fatalf("why trace issued provider/external calls: %d", providerCalls)
	}
}

func countProviderFactsForRequest(t *testing.T, requestID string) int {
	t.Helper()
	pool := e2ePool(t)
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM recommendation_provider_facts WHERE request_id = $1`, requestID).Scan(&count); err != nil {
		t.Fatalf("count provider facts for %s: %v", requestID, err)
	}
	return count
}

func e2ePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("e2e: DATABASE_URL not set - live stack not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func whyExplanationMentions(values []string, token string) bool {
	for _, value := range values {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}
