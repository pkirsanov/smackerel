//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"
	"time"
)

// T5-12 / BS-005: Lint report visible at GET /api/knowledge/lint.
func TestKnowledgeLint_ReportEndpoint(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiGet(cfg, "/api/knowledge/lint")
	if err != nil {
		t.Fatalf("lint request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	// 200 if lint has run, 404 if no report yet — both are valid
	if resp.StatusCode != 200 && resp.StatusCode != 404 {
		t.Fatalf("expected 200 or 404, got %d: %s", resp.StatusCode, string(body))
	}

	if resp.StatusCode == 200 {
		var report struct {
			ID       string `json:"id"`
			RunAt    string `json:"run_at"`
			Duration int    `json:"duration_ms"`
			Summary  struct {
				Total  int `json:"total"`
				High   int `json:"high"`
				Medium int `json:"medium"`
				Low    int `json:"low"`
			} `json:"summary"`
			Findings []json.RawMessage `json:"findings"`
		}
		if err := json.Unmarshal(body, &report); err != nil {
			t.Fatalf("parse lint report: %v", err)
		}
		t.Logf("lint report: run_at=%s findings=%d (high=%d medium=%d low=%d)",
			report.RunAt, report.Summary.Total, report.Summary.High, report.Summary.Medium, report.Summary.Low)
	} else {
		t.Log("no lint report yet (404) — lint cron has not run")
	}
}
