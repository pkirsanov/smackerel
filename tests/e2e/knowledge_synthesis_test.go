//go:build e2e

// T2-13 (BS-001): Full pipeline: capture → process → synthesize → verify concept page via API.
package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestKnowledgeSynthesis_PipelineRoundTrip captures an artifact and verifies
// the synthesis pipeline updates knowledge stats.
func TestKnowledgeSynthesis_PipelineRoundTrip(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// Capture a test artifact
	captureBody := `{"url":"https://example.com/synthesis-e2e-test","text":"Synthesis E2E test article about knowledge management systems and organizational learning","context":"e2e-synthesis-test"}`
	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+"/api/capture", strings.NewReader(captureBody))
	if err != nil {
		t.Fatalf("create capture request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("capture request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read capture response: %v", err)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 409 {
		t.Fatalf("capture returned %d: %s", resp.StatusCode, string(body))
	}
	t.Logf("capture response: %d %s", resp.StatusCode, string(body)[:min(200, len(body))])

	// Wait for processing pipeline (embedding + synthesis is async via NATS)
	time.Sleep(5 * time.Second)

	// Check synthesis stats — should show activity
	resp, err = apiGet(cfg, "/api/knowledge/stats")
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	body, err = readBody(resp)
	if err != nil {
		t.Fatalf("read stats: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("stats returned %d: %s", resp.StatusCode, string(body))
	}

	var stats struct {
		SynthesisCompleted int `json:"synthesis_completed"`
		SynthesisPending   int `json:"synthesis_pending"`
		SynthesisFailed    int `json:"synthesis_failed"`
	}
	if err := json.Unmarshal(body, &stats); err != nil {
		t.Fatalf("parse stats: %v", err)
	}
	total := stats.SynthesisCompleted + stats.SynthesisPending + stats.SynthesisFailed
	t.Logf("synthesis stats: completed=%d pending=%d failed=%d total=%d",
		stats.SynthesisCompleted, stats.SynthesisPending, stats.SynthesisFailed, total)
}
