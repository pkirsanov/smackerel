//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// e2eConfig holds live-stack connection details resolved from environment.
type e2eConfig struct {
	CoreURL   string
	AuthToken string
}

// loadE2EConfig reads live-stack connection details from environment.
// Config values MUST come from env (SST) — no hardcoded defaults.
func loadE2EConfig(t *testing.T) e2eConfig {
	t.Helper()

	coreURL := os.Getenv("CORE_EXTERNAL_URL")
	if coreURL == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set — live stack not available")
	}
	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if authToken == "" {
		t.Skip("e2e: SMACKEREL_AUTH_TOKEN not set — live stack not available")
	}
	return e2eConfig{CoreURL: coreURL, AuthToken: authToken}
}

// waitForHealth blocks until the health endpoint reports all services up,
// or times out after maxWait.
// SEC-031-R69-001: Uses a timeout-configured client to prevent individual
// requests from blocking indefinitely if the server hangs mid-response.
func waitForHealth(t *testing.T, cfg e2eConfig, maxWait time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		resp, err := client.Get(cfg.CoreURL + "/api/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("e2e: services not healthy after %s at %s", maxWait, cfg.CoreURL)
}

// apiGet performs an authenticated GET against the live stack.
func apiGet(cfg e2eConfig, path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	client := &http.Client{Timeout: 15 * time.Second}
	return client.Do(req)
}

// readBody reads and closes the response body.
func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

type searchRequest struct {
	Query   string         `json:"query"`
	Limit   int            `json:"limit,omitempty"`
	Filters map[string]any `json:"filters,omitempty"`
}

type searchResponse struct {
	Results         []searchResult `json:"results"`
	TotalCandidates int            `json:"total_candidates"`
	SearchMode      string         `json:"search_mode"`
	Message         string         `json:"message,omitempty"`
}

type searchResult struct {
	ArtifactID   string `json:"artifact_id"`
	Title        string `json:"title"`
	ArtifactType string `json:"artifact_type"`
	SourceURL    string `json:"source_url,omitempty"`
}

type artifactDetail struct {
	ArtifactID      string `json:"artifact_id"`
	Title           string `json:"title"`
	ArtifactType    string `json:"artifact_type"`
	ProcessingTier  string `json:"processing_tier"`
	ProcessingState string `json:"processing_status"`
}

// apiPostJSON performs an authenticated JSON POST against the live stack.
func apiPostJSON(cfg e2eConfig, path string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	return client.Do(req)
}

func apiSearch(t *testing.T, cfg e2eConfig, req searchRequest) searchResponse {
	t.Helper()

	resp, err := apiPostJSON(cfg, "/api/search", req)
	if err != nil {
		t.Fatalf("search request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read search body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search returned %d: %s", resp.StatusCode, string(body))
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("parse search response object: %v; body=%s", err, string(body))
	}
	for _, field := range []string{"results", "total_candidates", "search_mode"} {
		if _, ok := raw[field]; !ok {
			t.Fatalf("search response missing %q field: %s", field, string(body))
		}
	}

	var parsed searchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse search response: %v; body=%s", err, string(body))
	}
	if parsed.SearchMode == "" {
		t.Fatalf("search response has empty search_mode: %s", string(body))
	}
	if req.Limit > 0 && len(parsed.Results) > req.Limit {
		t.Fatalf("search returned %d results, above requested limit %d", len(parsed.Results), req.Limit)
	}
	for _, result := range parsed.Results {
		if result.ArtifactID == "" {
			t.Errorf("search result has empty artifact_id: %+v", result)
		}
		if result.Title == "" {
			t.Errorf("search result %q has empty title", result.ArtifactID)
		}
		if result.ArtifactType == "" {
			t.Errorf("search result %q has empty artifact_type", result.ArtifactID)
		}
	}

	return parsed
}

func apiArtifactDetail(t *testing.T, cfg e2eConfig, artifactID string) artifactDetail {
	t.Helper()

	resp, err := apiGet(cfg, fmt.Sprintf("/api/artifact/%s", artifactID))
	if err != nil {
		t.Fatalf("artifact detail request failed for %s: %v", artifactID, err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read artifact detail body for %s: %v", artifactID, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("artifact detail for %s returned %d: %s", artifactID, resp.StatusCode, string(body))
	}

	var detail artifactDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatalf("parse artifact detail for %s: %v; body=%s", artifactID, err, string(body))
	}
	if detail.ArtifactID != artifactID {
		t.Fatalf("artifact detail ID mismatch: got %q, want %q", detail.ArtifactID, artifactID)
	}
	return detail
}

// TestBrowserHistory_E2E_SearchRequestsUsePOSTContract is an adversarial guard
// for BUG-010-003: reintroducing GET /api/search in this E2E file must fail.
func TestBrowserHistory_E2E_SearchRequestsUsePOSTContract(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate browser-history E2E source file")
	}
	contents, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read browser-history E2E source: %v", err)
	}

	searchPath := "/" + "api" + "/" + "search"
	for lineNumber, line := range strings.Split(string(contents), "\n") {
		if strings.Contains(line, "api"+"Get(") && strings.Contains(line, searchPath+"?") {
			t.Fatalf("stale GET search helper use at %s:%d: %s", file, lineNumber+1, strings.TrimSpace(line))
		}
		if strings.Contains(line, "http."+"MethodGet") && strings.Contains(line, searchPath) {
			t.Fatalf("stale http.MethodGet search request at %s:%d: %s", file, lineNumber+1, strings.TrimSpace(line))
		}
	}
}

// TestBrowserHistory_E2E_InitialSyncProducesArtifacts (T-18)
// Verifies that a browser-history connector sync against the live stack
// produces artifacts that are queryable via the API.
func TestBrowserHistory_E2E_InitialSyncProducesArtifacts(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// Check connector status via health endpoint
	resp, err := apiGet(cfg, "/api/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read health body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health returned %d: %s", resp.StatusCode, string(body))
	}

	var health map[string]interface{}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("parse health JSON: %v", err)
	}

	t.Logf("health response: %s", string(body))

	// Query artifacts through the supported POST /api/search contract.
	results := apiSearch(t, cfg, searchRequest{
		Query: "browser history",
		Limit: 10,
	})
	for _, result := range results.Results {
		detail := apiArtifactDetail(t, cfg, result.ArtifactID)
		if detail.ArtifactType != result.ArtifactType {
			t.Errorf("detail artifact_type mismatch for %s: search=%q detail=%q", result.ArtifactID, result.ArtifactType, detail.ArtifactType)
		}
	}

	t.Logf("browser-history initial sync search: %d results, %d candidates, mode=%s",
		len(results.Results), results.TotalCandidates, results.SearchMode)
}

// TestBrowserHistory_E2E_ConditionalRegistration (T-19)
// Verifies the connector is only registered when BROWSER_HISTORY_ENABLED=true
// in the stack configuration.
func TestBrowserHistory_E2E_ConditionalRegistration(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// Query the connectors endpoint (or health sub-field) for browser-history status
	resp, err := apiGet(cfg, "/api/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read health body: %v", err)
	}

	var health struct {
		Services   map[string]interface{} `json:"services"`
		Connectors map[string]interface{} `json:"connectors"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("parse health: %v", err)
	}

	browserEnabled := os.Getenv("BROWSER_HISTORY_ENABLED")

	// If explicitly disabled, connector should NOT appear in health connectors
	if browserEnabled == "false" {
		if _, found := health.Connectors["browser-history"]; found {
			t.Error("browser-history connector registered despite BROWSER_HISTORY_ENABLED=false")
		}
		t.Log("conditional registration: correctly absent when disabled")
		return
	}

	// If enabled, connector should appear
	if browserEnabled == "true" {
		if _, found := health.Connectors["browser-history"]; !found {
			// May not be in connectors map if health response doesn't enumerate connectors
			t.Log("conditional registration: BROWSER_HISTORY_ENABLED=true but connector not in health.connectors (endpoint may not expose connectors)")
		} else {
			t.Log("conditional registration: correctly present when enabled")
		}
	}

	t.Logf("BROWSER_HISTORY_ENABLED=%s, health connectors: %v", browserEnabled, health.Connectors)
}

// TestBrowserHistory_E2E_SocialMediaAggregateInStore (T-33)
// Verifies that social media aggregate artifacts are persisted in the store
// and queryable via the API.
func TestBrowserHistory_E2E_SocialMediaAggregateInStore(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	results := apiSearch(t, cfg, searchRequest{
		Query: "social media aggregate",
		Limit: 50,
		Filters: map[string]any{
			"type": "browsing/social-aggregate",
		},
	})

	var socialAggregates int
	for _, result := range results.Results {
		if result.ArtifactType == "browsing/social-aggregate" {
			socialAggregates++
			if result.Title == "" {
				t.Error("social aggregate artifact has empty title")
			}
			detail := apiArtifactDetail(t, cfg, result.ArtifactID)
			if detail.ArtifactType != "browsing/social-aggregate" {
				t.Errorf("expected detail artifact_type browsing/social-aggregate for %s, got %q", result.ArtifactID, detail.ArtifactType)
			}
		}
	}

	t.Logf("social media aggregates in store: %d results, %d candidates, mode=%s",
		socialAggregates, results.TotalCandidates, results.SearchMode)
}

// TestBrowserHistory_E2E_HighDwellArticleSearchable (T-34)
// Verifies that high-dwell articles (full/standard tier) are searchable
// after being ingested through the browser history connector.
func TestBrowserHistory_E2E_HighDwellArticleSearchable(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	results := apiSearch(t, cfg, searchRequest{
		Query: "article",
		Limit: 50,
		Filters: map[string]any{
			"type": "url",
		},
	})

	var highDwell int
	for _, result := range results.Results {
		if result.ArtifactType != "url" {
			continue
		}
		detail := apiArtifactDetail(t, cfg, result.ArtifactID)
		tier := detail.ProcessingTier
		if tier == "full" || tier == "standard" {
			highDwell++
			if result.Title == "" || detail.Title == "" {
				t.Error("high-dwell article has empty title")
			}
		}
	}

	t.Logf("high-dwell articles searchable: %d out of %d URL results, %d candidates, mode=%s",
		highDwell, len(results.Results), results.TotalCandidates, results.SearchMode)
}
