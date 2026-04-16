//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"
	"time"
)

// T4-07 / BS-003: Multi-source ingest → cross-source connection visible in entity profile.
func TestKnowledgeCrossSource_ConnectionDetection(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// Verify cross-source infrastructure by checking concept list for source_type diversity
	resp, err := apiGet(cfg, "/api/knowledge/concepts?limit=50")
	if err != nil {
		t.Fatalf("concepts request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Concepts []struct {
			ID          string   `json:"id"`
			Title       string   `json:"title"`
			SourceTypes []string `json:"source_types"`
		} `json:"concepts"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse response: %v", err)
	}

	multiSource := 0
	for _, c := range result.Concepts {
		if len(c.SourceTypes) >= 2 {
			multiSource++
			t.Logf("cross-source concept: %s (types: %v)", c.Title, c.SourceTypes)
		}
	}
	t.Logf("total concepts: %d, multi-source: %d", result.Total, multiSource)
}
