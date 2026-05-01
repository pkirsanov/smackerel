//go:build e2e

// SCN-039-045 / Scope 5 DoD: the /admin/agent/traces page MUST accept a
// `scenario=recommendation-*` filter and exclude traces whose scenario_id
// (or scenario_version) does not match. The filter translates `*` to the
// SQL LIKE wildcard `%` and `?` to `_`; whitespace-only patterns are no-ops.
package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestAdminAgentTraces_FilterRecommendationScenarios covers SCN-039-045 /
// Scope 5 DoD: the operator trace view at /admin/agent/traces MUST accept a
// `scenario=recommendation-*` filter, list only recommendation traces, and
// exclude unrelated scenario traces.
func TestAdminAgentTraces_FilterRecommendationScenarios(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// Submit a recommendation request to seed a fresh trace with scenario_id
	// `recommendation_reactive` and scenario_version `recommendation-reactive-v1`.
	resp, err := apiPostJSON(cfg, "/api/recommendations/requests", map[string]any{
		"query":            "scn-045 quiet ramen near mission",
		"source":           "api",
		"location_ref":     "gps:37.7749,-122.4194",
		"precision_policy": "neighborhood",
		"result_count":     3,
	})
	if err != nil {
		t.Fatalf("seed recommendation request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read seed body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("seed status = %d, want 200; body=%s", resp.StatusCode, string(body))
	}
	var seeded struct {
		TraceID string `json:"trace_id"`
	}
	if err := json.Unmarshal(body, &seeded); err != nil {
		t.Fatalf("parse seed body: %v; body=%s", err, string(body))
	}
	if seeded.TraceID == "" {
		t.Fatalf("seeded recommendation request missing trace_id; body=%s", string(body))
	}

	// Positive case: the recommendation-* filter must include the seeded trace.
	matched, err := apiGet(cfg, "/admin/agent/traces?scenario=recommendation-*&page_size=200")
	if err != nil {
		t.Fatalf("admin traces GET (recommendation-*) failed: %v", err)
	}
	if matched.StatusCode != http.StatusOK {
		body, _ := readBody(matched)
		t.Fatalf("admin traces (recommendation-*) status = %d, want 200; body=%s", matched.StatusCode, string(body))
	}
	matchedBody, err := readBody(matched)
	if err != nil {
		t.Fatalf("read matched body: %v", err)
	}
	matchedHTML := string(matchedBody)
	if !strings.Contains(matchedHTML, seeded.TraceID) {
		t.Fatalf("recommendation-* filter missing seeded trace_id %q; body=%s", seeded.TraceID, matchedHTML)
	}
	// Adversarial: the only scenarios surfaced under `recommendation-*` must
	// look like recommendation scenarios. We pick a few well-known
	// non-recommendation scenario tokens and require none of them appear in
	// the rendered listing under this filter.
	forbidden := []string{
		"expense_question",
		"scope8_render",
		"no_such_scenario",
		"telegram_share",
	}
	for _, token := range forbidden {
		if strings.Contains(matchedHTML, token) {
			t.Fatalf("recommendation-* filter leaked non-recommendation scenario token %q", token)
		}
	}

	// Negative case: a filter that cannot match the seeded scenario MUST NOT
	// return the trace. This proves the LIKE pattern actually filters and
	// is not a no-op.
	excluded, err := apiGet(cfg, "/admin/agent/traces?scenario=expense-*&page_size=200")
	if err != nil {
		t.Fatalf("admin traces GET (expense-*) failed: %v", err)
	}
	if excluded.StatusCode != http.StatusOK {
		body, _ := readBody(excluded)
		t.Fatalf("admin traces (expense-*) status = %d, want 200; body=%s", excluded.StatusCode, string(body))
	}
	excludedBody, err := readBody(excluded)
	if err != nil {
		t.Fatalf("read excluded body: %v", err)
	}
	excludedHTML := string(excludedBody)
	if strings.Contains(excludedHTML, seeded.TraceID) {
		t.Fatalf("expense-* filter incorrectly included recommendation trace_id %q; body=%s", seeded.TraceID, excludedHTML)
	}
}
