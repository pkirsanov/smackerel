//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRecommendationsConfidence_BS032_LowConfidenceDisclosedWithoutOverstatingPersonalization(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiPostJSON(cfg, "/api/recommendations/requests", map[string]any{
		"query":            "low confidence coffee near mission",
		"source":           "api",
		"location_ref":     "gps:37.7749,-122.4194",
		"precision_policy": "neighborhood",
		"result_count":     1,
	})
	if err != nil {
		t.Fatalf("recommendation confidence request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read confidence body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Status          string `json:"status"`
		Recommendations []struct {
			Title                  string   `json:"title"`
			LowConfidence          bool     `json:"low_confidence"`
			PersonalSignalsApplied bool     `json:"personal_signals_applied"`
			GraphSignalRefs        []string `json:"graph_signal_refs"`
			Rationale              []string `json:"rationale"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse confidence response: %v; body=%s", err, string(body))
	}
	if parsed.Status != "delivered" || len(parsed.Recommendations) != 1 {
		t.Fatalf("unexpected confidence response: %+v body=%s", parsed, string(body))
	}
	rec := parsed.Recommendations[0]
	if !rec.LowConfidence {
		t.Fatalf("recommendation does not disclose low confidence: %+v", rec)
	}
	if rec.PersonalSignalsApplied || len(rec.GraphSignalRefs) != 0 {
		t.Fatalf("low-confidence generic result overstated personal signals: %+v", rec)
	}
	for _, rationale := range rec.Rationale {
		if strings.Contains(rationale, "ART-") || strings.Contains(strings.ToLower(rationale), "personal graph signal") {
			t.Fatalf("low-confidence rationale overstated graph personalization: %v", rec.Rationale)
		}
	}
}
