//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func parseProcessingStatus(detailBody []byte) (string, error) {
	var detail struct {
		ProcessingStatus *string `json:"processing_status"`
	}
	if err := json.Unmarshal(detailBody, &detail); err != nil {
		return "", fmt.Errorf("parse artifact detail response: %w", err)
	}
	if detail.ProcessingStatus == nil {
		return "", fmt.Errorf("artifact detail response missing processing_status")
	}
	status := strings.TrimSpace(*detail.ProcessingStatus)
	if status == "" {
		return "", fmt.Errorf("artifact detail response has empty processing_status")
	}
	return status, nil
}

func processingComplete(status string) (bool, error) {
	switch status {
	case "processed", "completed":
		return true, nil
	case "pending", "processing", "queued":
		return false, nil
	case "failed":
		return false, fmt.Errorf("artifact processing failed")
	default:
		return false, fmt.Errorf("artifact detail response has unexpected processing_status %q", status)
	}
}

// Scenario: Full pipeline flow
// Given the full stack is running (core, ML, PostgreSQL, NATS)
// When POST /api/capture sends a text artifact
// And the test waits for processing (poll artifact status, max 30s)
// Then the artifact has processing_status = 'processed'
// And searching for content from that artifact returns it
func TestE2E_CaptureProcessSearch(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// Step 1: Capture a text artifact
	uniqueMarker := fmt.Sprintf("e2e-test-%d", time.Now().UnixNano())
	captureBody := map[string]string{
		"text":    fmt.Sprintf("This is a test artifact about Mediterranean cooking techniques. Unique marker: %s", uniqueMarker),
		"context": "e2e integration test",
	}
	bodyBytes, _ := json.Marshal(captureBody)

	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+"/api/capture", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("create capture request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("capture request failed: %v", err)
	}
	captureRespBody, err := readBody(resp)
	if err != nil {
		t.Fatalf("read capture response: %v", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 200/201 from capture, got %d: %s", resp.StatusCode, string(captureRespBody))
	}

	var captureResp struct {
		ArtifactID   string `json:"artifact_id"`
		Title        string `json:"title"`
		ArtifactType string `json:"artifact_type"`
	}
	if err := json.Unmarshal(captureRespBody, &captureResp); err != nil {
		t.Fatalf("parse capture response: %v", err)
	}
	if captureResp.ArtifactID == "" {
		t.Fatal("capture response missing artifact_id")
	}
	t.Logf("captured artifact: id=%s title=%q type=%s", captureResp.ArtifactID, captureResp.Title, captureResp.ArtifactType)

	// Step 2: Wait for processing to complete (poll artifact detail)
	deadline := time.Now().Add(60 * time.Second)
	processed := false
	for time.Now().Before(deadline) {
		detailResp, err := apiGet(cfg, "/api/artifact/"+captureResp.ArtifactID)
		if err != nil {
			t.Logf("artifact detail request failed (retrying): %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		detailBody, err := readBody(detailResp)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if detailResp.StatusCode != http.StatusOK {
			time.Sleep(2 * time.Second)
			continue
		}

		status, err := parseProcessingStatus(detailBody)
		if err != nil {
			t.Fatalf("invalid artifact detail processing status: %v; body=%s", err, string(detailBody))
		}
		complete, err := processingComplete(status)
		if err != nil {
			t.Fatalf("artifact did not reach a processable status: %v; body=%s", err, string(detailBody))
		}

		if complete {
			processed = true
			t.Logf("artifact processed: status=%s", status)
			break
		}
		t.Logf("waiting for processing... status=%s", status)
		time.Sleep(2 * time.Second)
	}

	if !processed {
		t.Fatal("artifact not processed within 60s timeout — pipeline may be broken")
	}

	// Step 3: Search for content from the captured artifact
	searchBody := map[string]interface{}{
		"query": "Mediterranean cooking techniques",
		"limit": 10,
	}
	searchBytes, _ := json.Marshal(searchBody)
	searchReq, err := http.NewRequest(http.MethodPost, cfg.CoreURL+"/api/search", bytes.NewReader(searchBytes))
	if err != nil {
		t.Fatalf("create search request: %v", err)
	}
	searchReq.Header.Set("Content-Type", "application/json")
	searchReq.Header.Set("Authorization", "Bearer "+cfg.AuthToken)

	searchResp, err := client.Do(searchReq)
	if err != nil {
		t.Fatalf("search request failed: %v", err)
	}
	searchRespBody, err := readBody(searchResp)
	if err != nil {
		t.Fatalf("read search response: %v", err)
	}

	if searchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from search, got %d: %s", searchResp.StatusCode, string(searchRespBody))
	}

	var searchResult struct {
		Results []struct {
			ArtifactID string `json:"artifact_id"`
			Title      string `json:"title"`
		} `json:"results"`
		TotalCandidates int    `json:"total_candidates"`
		SearchMode      string `json:"search_mode"`
	}
	if err := json.Unmarshal(searchRespBody, &searchResult); err != nil {
		t.Fatalf("parse search response: %v", err)
	}

	t.Logf("search returned %d results (mode=%s, candidates=%d)",
		len(searchResult.Results), searchResult.SearchMode, searchResult.TotalCandidates)

	// Verify the captured artifact appears in results (if processed)
	found := false
	for _, r := range searchResult.Results {
		if r.ArtifactID == captureResp.ArtifactID {
			found = true
			t.Logf("found captured artifact in search results: %s", r.Title)
			break
		}
	}

	if !found {
		t.Error("captured and processed artifact not found in search results")
	}

	// Cleanup note: E2E tests rely on the disposable test stack (smackerel-test
	// compose project with isolated volumes). Test data uses unique markers
	// (e2e-test-{UnixNano}) for idempotency. Stack teardown removes all data.
	t.Logf("e2e capture→process→search test completed, artifact_id=%s", captureResp.ArtifactID)
}

// TestE2E_CaptureProcessSearch_AdversarialEmptyStatus is a regression test for BUG-031-003.
// This test verifies that when an artifact detail response omits or provides an empty
// processing_status field, the E2E test fails loudly instead of silently passing.
// This prevents regression of the original bug where empty status was treated as "processed".
func TestE2E_CaptureProcessSearch_AdversarialEmptyStatus(t *testing.T) {
	parseRejects := []struct {
		name string
		body string
	}{
		{name: "missing status", body: `{}`},
		{name: "empty status", body: `{"processing_status":""}`},
		{name: "whitespace status", body: `{"processing_status":"   "}`},
	}
	for _, tc := range parseRejects {
		t.Run(tc.name, func(t *testing.T) {
			if status, err := parseProcessingStatus([]byte(tc.body)); err == nil {
				t.Fatalf("parseProcessingStatus(%s) = %q, nil; want failure", tc.body, status)
			}
		})
	}

	statusRejects := []string{"failed", "", "unknown"}
	for _, status := range statusRejects {
		t.Run("reject status "+status, func(t *testing.T) {
			if complete, err := processingComplete(status); err == nil || complete {
				t.Fatalf("processingComplete(%q) = complete=%v err=%v; want failure", status, complete, err)
			}
		})
	}

	statusWaits := []string{"pending", "processing", "queued"}
	for _, status := range statusWaits {
		t.Run("wait status "+status, func(t *testing.T) {
			complete, err := processingComplete(status)
			if err != nil || complete {
				t.Fatalf("processingComplete(%q) = complete=%v err=%v; want wait/no error", status, complete, err)
			}
		})
	}

	for _, status := range []string{"processed", "completed"} {
		t.Run("accept status "+status, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"processing_status":%q}`, status))
			parsed, err := parseProcessingStatus(body)
			if err != nil {
				t.Fatalf("parseProcessingStatus(%s): %v", string(body), err)
			}
			complete, err := processingComplete(parsed)
			if err != nil || !complete {
				t.Fatalf("processingComplete(%q) = complete=%v err=%v; want complete/no error", parsed, complete, err)
			}
		})
	}
}
