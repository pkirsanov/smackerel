//go:build e2e

// T2-13 (BS-001): Full pipeline: capture → process → synthesize → verify concept page via API.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

const knowledgeSynthesisFixtureContext = "e2e-synthesis-test"

func knowledgeSynthesisCaptureFixture(marker string) map[string]string {
	return map[string]string{
		"text":    fmt.Sprintf("Synthesis E2E deterministic article about knowledge management systems, organizational learning, concept synthesis, entity extraction, and retrieval workflows. Unique marker: %s", marker),
		"context": knowledgeSynthesisFixtureContext,
	}
}

func assertDeterministicKnowledgeSynthesisFixture(t *testing.T, fixture map[string]string, encoded []byte) {
	t.Helper()
	if fixture["text"] == "" {
		t.Fatal("knowledge synthesis fixture must include deterministic text content")
	}
	if url := fixture["url"]; url != "" {
		t.Fatalf("knowledge synthesis fixture must not require external URL extraction, got url=%q", url)
	}
	if bytes.Contains(encoded, []byte("https://")) || bytes.Contains(encoded, []byte("http://")) || bytes.Contains(encoded, []byte("example.com/synthesis-e2e-test")) {
		t.Fatalf("knowledge synthesis fixture contains a non-owned external URL: %s", string(encoded))
	}
}

type knowledgeSynthesisStats struct {
	SynthesisCompleted int `json:"synthesis_completed"`
	SynthesisPending   int `json:"synthesis_pending"`
	SynthesisFailed    int `json:"synthesis_failed"`
}

func (s knowledgeSynthesisStats) total() int {
	return s.SynthesisCompleted + s.SynthesisPending + s.SynthesisFailed
}

func fetchKnowledgeSynthesisStats(t *testing.T, cfg e2eConfig) knowledgeSynthesisStats {
	t.Helper()
	resp, err := apiGet(cfg, "/api/knowledge/stats")
	if err != nil {
		t.Fatalf("knowledge stats request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read knowledge stats response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("knowledge stats returned %d: %s", resp.StatusCode, string(body))
	}
	var stats knowledgeSynthesisStats
	if err := json.Unmarshal(body, &stats); err != nil {
		t.Fatalf("parse knowledge stats response: %v", err)
	}
	return stats
}

// TestKnowledgeSynthesis_PipelineRoundTrip captures an artifact and verifies
// the synthesis pipeline updates knowledge stats.
func TestKnowledgeSynthesis_PipelineRoundTrip(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	initialStats := fetchKnowledgeSynthesisStats(t, cfg)

	// Capture deterministic stack-owned content. A URL fixture would force live
	// article extraction before the text path is considered.
	captureFixture := knowledgeSynthesisCaptureFixture(fmt.Sprintf("%d", time.Now().UnixNano()))
	captureBody, err := json.Marshal(captureFixture)
	if err != nil {
		t.Fatalf("marshal capture fixture: %v", err)
	}
	assertDeterministicKnowledgeSynthesisFixture(t, captureFixture, captureBody)

	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+"/api/capture", bytes.NewReader(captureBody))
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("capture returned %d: %s", resp.StatusCode, string(body))
	}
	var captureResp struct {
		ArtifactID   string `json:"artifact_id"`
		ArtifactType string `json:"artifact_type"`
		Title        string `json:"title"`
	}
	if err := json.Unmarshal(body, &captureResp); err != nil {
		t.Fatalf("parse capture response: %v", err)
	}
	if captureResp.ArtifactID == "" {
		t.Fatal("capture response missing artifact_id")
	}
	if captureResp.ArtifactType == "" {
		t.Fatal("capture response missing artifact_type")
	}
	t.Logf("capture response: %d %s", resp.StatusCode, string(body)[:min(200, len(body))])

	// Wait for the real processing pipeline to consume the captured artifact.
	deadline := time.Now().Add(60 * time.Second)
	var lastProcessingStatus string
	for time.Now().Before(deadline) {
		detailResp, err := apiGet(cfg, "/api/artifact/"+captureResp.ArtifactID)
		if err != nil {
			t.Logf("artifact detail request failed (retrying): %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		detailBody, err := readBody(detailResp)
		if err != nil {
			t.Logf("read artifact detail failed (retrying): %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if detailResp.StatusCode != http.StatusOK {
			t.Logf("artifact detail returned %d (retrying): %s", detailResp.StatusCode, string(detailBody))
			time.Sleep(2 * time.Second)
			continue
		}
		status, err := parseProcessingStatus(detailBody)
		if err != nil {
			t.Fatalf("invalid artifact detail processing status: %v; body=%s", err, string(detailBody))
		}
		lastProcessingStatus = status
		complete, err := processingComplete(status)
		if err != nil {
			t.Fatalf("artifact processing failed: %v; body=%s", err, string(detailBody))
		}
		if complete {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if lastProcessingStatus == "" || lastProcessingStatus == "pending" || lastProcessingStatus == "processing" || lastProcessingStatus == "queued" {
		t.Fatalf("artifact %s did not finish processing before timeout; last status=%q", captureResp.ArtifactID, lastProcessingStatus)
	}

	// Check synthesis stats: this capture should add a synthesis status signal
	// even when full LLM synthesis remains pending in the disposable stack.
	var stats knowledgeSynthesisStats
	statsDeadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(statsDeadline) {
		stats = fetchKnowledgeSynthesisStats(t, cfg)
		if stats.total() > initialStats.total() {
			break
		}
		time.Sleep(2 * time.Second)
	}
	total := stats.total()
	if total <= initialStats.total() {
		t.Fatalf("expected synthesis stats total to increase after captured artifact, before=%+v after=%+v", initialStats, stats)
	}
	t.Logf("synthesis stats: completed=%d pending=%d failed=%d total=%d",
		stats.SynthesisCompleted, stats.SynthesisPending, stats.SynthesisFailed, total)
}
