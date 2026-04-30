//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRecommendationsConstraints_BS020_VegetarianHardConstraintExcludesIncompatible(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	parsed := postRecommendationForConstraintTest(t, cfg, "vegetarian ramen near mission")
	if parsed.Status != "delivered" {
		t.Fatalf("status = %q, want delivered", parsed.Status)
	}
	if len(parsed.Recommendations) == 0 {
		t.Fatal("expected vegetarian-compatible recommendations")
	}
	for _, rec := range parsed.Recommendations {
		if strings.Contains(strings.ToLower(rec.Title), "pork") || strings.Contains(strings.ToLower(rec.Title), "menkichi") {
			t.Fatalf("hard vegetarian constraint allowed incompatible candidate: %+v", rec)
		}
	}
}

func TestRecommendationsConstraints_BS029_NoSilentRelaxationWhenNoCandidateQualifies(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	parsed := postRecommendationForConstraintTest(t, cfg, "vegetarian ramen open now within 1 km")
	if parsed.Status != "no_eligible" {
		t.Fatalf("status = %q, want no_eligible", parsed.Status)
	}
	if len(parsed.Recommendations) != 0 {
		t.Fatalf("hard constraints were silently relaxed; got recommendations: %+v", parsed.Recommendations)
	}
}

type constraintRecommendationResponse struct {
	Status          string `json:"status"`
	Recommendations []struct {
		Title           string           `json:"title"`
		PolicyDecisions []map[string]any `json:"policy_decisions"`
	} `json:"recommendations"`
}

func postRecommendationForConstraintTest(t *testing.T, cfg e2eConfig, query string) constraintRecommendationResponse {
	t.Helper()
	resp, err := apiPostJSON(cfg, "/api/recommendations/requests", map[string]any{
		"query":            query,
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
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var parsed constraintRecommendationResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse recommendation response: %v; body=%s", err, string(body))
	}
	return parsed
}
