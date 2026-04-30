//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRecommendationsClarification_BS015_NoProviderCallBeforeClarification(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiPostJSON(cfg, "/api/recommendations/requests", map[string]any{
		"query":            "something good",
		"source":           "api",
		"precision_policy": "neighborhood",
	})
	if err != nil {
		t.Fatalf("recommendation clarification request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read clarification body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		RequestID      string `json:"request_id"`
		Status         string `json:"status"`
		TraceID        string `json:"trace_id"`
		Clarification  *struct {
			Question string   `json:"question"`
			Choices  []string `json:"choices"`
		} `json:"clarification"`
		Recommendations []any `json:"recommendations"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse clarification response: %v; body=%s", err, string(body))
	}
	if parsed.Status != "ambiguous" {
		t.Fatalf("status = %q, want ambiguous; body=%s", parsed.Status, string(body))
	}
	if parsed.Clarification == nil || parsed.Clarification.Question == "" || len(parsed.Clarification.Choices) == 0 || len(parsed.Clarification.Choices) > 3 {
		t.Fatalf("clarification malformed: %+v", parsed.Clarification)
	}
	if len(parsed.Recommendations) != 0 {
		t.Fatalf("ambiguous request returned recommendations: %+v", parsed.Recommendations)
	}
	assertTraceHasNoProviderLookup(t, parsed.TraceID, parsed.RequestID)
}

func assertTraceHasNoProviderLookup(t *testing.T, traceID, requestID string) {
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
	defer pool.Close()

	var fetchCalls int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM agent_tool_calls WHERE trace_id = $1 AND tool_name = 'recommendation_fetch_candidates'`, traceID).Scan(&fetchCalls); err != nil {
		t.Fatalf("count fetch tool calls: %v", err)
	}
	if fetchCalls != 0 {
		t.Fatalf("ambiguous trace issued provider fetch calls: %d", fetchCalls)
	}

	var providerFacts int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM recommendation_provider_facts WHERE request_id = $1`, requestID).Scan(&providerFacts); err != nil {
		t.Fatalf("count provider facts: %v", err)
	}
	if providerFacts != 0 {
		t.Fatalf("ambiguous request persisted provider facts: %d", providerFacts)
	}
}