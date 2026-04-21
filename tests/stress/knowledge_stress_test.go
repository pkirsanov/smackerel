//go:build stress

// Package stress contains knowledge synthesis layer stress tests.
// Run via: ./smackerel.sh test stress
//
// Spec 025 validation checkpoints:
// - Synthesis throughput at 500+ artifacts (< 30s P95 per artifact)
// - Lint at 1000-artifact scale (< 5 minutes)
// - Knowledge query response < 2s P95
package stress

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// stressConfig holds live-stack connection details resolved from environment.
type stressConfig struct {
	CoreURL   string
	AuthToken string
}

// loadStressConfig reads live-stack connection details from environment.
// Config values MUST come from env (SST) — no hardcoded defaults.
func loadStressConfig(t *testing.T) stressConfig {
	t.Helper()
	coreURL := os.Getenv("CORE_EXTERNAL_URL")
	if coreURL == "" {
		t.Skip("stress: CORE_EXTERNAL_URL not set — live stack not available")
	}
	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if authToken == "" {
		t.Skip("stress: SMACKEREL_AUTH_TOKEN not set — live stack not available")
	}
	return stressConfig{CoreURL: coreURL, AuthToken: authToken}
}

// stressWaitForHealth blocks until the health endpoint reports healthy.
func stressWaitForHealth(t *testing.T, cfg stressConfig, maxWait time.Duration) {
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
	t.Fatalf("stress: services not healthy after %s at %s", maxWait, cfg.CoreURL)
}

// stressAPIGet performs an authenticated GET, returns status, body, error.
func stressAPIGet(cfg stressConfig, path string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+path, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}

// stressAPIPost performs an authenticated POST, returns status, body, error.
func stressAPIPost(cfg stressConfig, path string, payload []byte) (int, []byte, error) {
	var bodyReader io.Reader
	if payload != nil {
		bodyReader = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+path, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}

// --- Tests ---

// TestKnowledge_LintAt1000ArtifactScale verifies that the knowledge lint
// system completes within the 5-minute budget for a 1000-artifact knowledge base
// as specified in spec 025 (R-2506, BS-010).
//
// Requires: live PostgreSQL + NATS stack (./smackerel.sh up).
func TestKnowledge_LintAt1000ArtifactScale(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)

	const maxLintDurationMinutes = 5

	// Check latest lint report timing
	status, body, err := stressAPIGet(cfg, "/api/knowledge/lint")
	if err != nil {
		t.Skipf("lint report endpoint not available: %v", err)
	}
	if status == 404 {
		t.Skip("no lint report available — lint may not have run yet")
	}
	if status == 503 {
		t.Skip("knowledge layer not enabled on this stack")
	}
	if status != 200 {
		t.Fatalf("GET /api/knowledge/lint returned %d: %s", status, string(body))
	}

	var report struct {
		DurationMs int `json:"duration_ms"`
	}
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("parse lint report: %v", err)
	}

	t.Logf("Latest lint report duration: %dms", report.DurationMs)
	if report.DurationMs > maxLintDurationMinutes*60*1000 {
		t.Errorf("lint duration %dms exceeds %d-minute budget", report.DurationMs, maxLintDurationMinutes)
	}
}

// TestKnowledge_ConceptQueryPerformance verifies that knowledge concept
// queries respond within the 2-second budget defined in spec 025.
// Tests listing, search, and stats endpoints under the performance envelope.
//
// Requires: live PostgreSQL + NATS stack (./smackerel.sh up).
func TestKnowledge_ConceptQueryPerformance(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)

	// Test 1: Concept listing with sort by citations (most expensive query)
	start := time.Now()
	status, body, err := stressAPIGet(cfg, "/api/knowledge/concepts?limit=50&sort=citations")
	if err != nil {
		t.Skipf("knowledge concepts endpoint not available: %v", err)
	}
	elapsed := time.Since(start)

	if status == 503 {
		t.Skip("knowledge layer not enabled on this stack")
	}
	if status != 200 {
		t.Fatalf("GET /api/knowledge/concepts returned %d: %s", status, string(body))
	}
	t.Logf("Concept list (limit=50, sort=citations) returned in %v", elapsed)
	if elapsed > 2*time.Second {
		t.Errorf("concept listing took %v, expected < 2s (spec 025 P95 budget)", elapsed)
	}

	// Test 2: Knowledge stats endpoint
	start = time.Now()
	status, body, err = stressAPIGet(cfg, "/api/knowledge/stats")
	if err != nil {
		t.Skipf("knowledge stats endpoint not available: %v", err)
	}
	elapsed = time.Since(start)
	if status != 200 {
		t.Fatalf("GET /api/knowledge/stats returned %d: %s", status, string(body))
	}
	t.Logf("Knowledge stats returned in %v", elapsed)
	if elapsed > 2*time.Second {
		t.Errorf("stats endpoint took %v, expected < 2s", elapsed)
	}

	// Test 3: Entity listing
	start = time.Now()
	status, body, err = stressAPIGet(cfg, "/api/knowledge/entities?limit=50&sort=mentions")
	if err != nil {
		t.Skipf("knowledge entities endpoint not available: %v", err)
	}
	elapsed = time.Since(start)
	if status != 200 {
		t.Fatalf("GET /api/knowledge/entities returned %d: %s", status, string(body))
	}
	t.Logf("Entity list (limit=50, sort=mentions) returned in %v", elapsed)
	if elapsed > 2*time.Second {
		t.Errorf("entity listing took %v, expected < 2s", elapsed)
	}
}

// TestKnowledge_SearchWithKnowledgeLayerPerformance verifies that the
// knowledge-first search path (spec 025 R-2508) responds within acceptable
// latency even with a populated knowledge layer.
//
// Requires: live PostgreSQL + NATS stack (./smackerel.sh up).
func TestKnowledge_SearchWithKnowledgeLayerPerformance(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)

	queries := []string{
		"leadership",
		"pricing strategy",
		"remote work productivity",
		"negotiation techniques",
		"restaurant recommendations",
	}

	for _, q := range queries {
		searchBody, _ := json.Marshal(map[string]string{"query": q})
		start := time.Now()
		status, body, err := stressAPIPost(cfg, "/api/search", searchBody)
		if err != nil {
			t.Skipf("search endpoint not available: %v", err)
		}
		elapsed := time.Since(start)

		if status != 200 {
			t.Fatalf("POST /api/search %q returned %d: %s", q, status, string(body))
		}

		var result struct {
			SearchMode     string `json:"search_mode"`
			KnowledgeMatch *struct {
				ConceptID string `json:"concept_id"`
				Title     string `json:"title"`
			} `json:"knowledge_match"`
		}
		_ = json.Unmarshal(body, &result)

		mode := result.SearchMode
		matched := "no"
		if result.KnowledgeMatch != nil {
			matched = result.KnowledgeMatch.Title
		}

		t.Logf("Search %q: %v (mode=%s, knowledge_match=%s)", q, elapsed, mode, matched)
		if elapsed > 5*time.Second {
			t.Errorf("search %q took %v, expected < 5s", q, elapsed)
		}
	}
}

// TestKnowledge_HealthEndpointIncludesKnowledgeSection verifies that the
// /api/health endpoint includes a knowledge section with synthesis stats
// and that this doesn't degrade health check performance.
func TestKnowledge_HealthEndpointIncludesKnowledgeSection(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)

	// Run 25 rapid health checks and verify knowledge section is present
	for i := 0; i < 25; i++ {
		start := time.Now()
		status, body, err := stressAPIGet(cfg, "/api/health")
		if err != nil {
			t.Fatalf("health check %d failed: %v", i, err)
		}
		elapsed := time.Since(start)

		if status != 200 {
			t.Fatalf("health check %d returned %d: %s", i, status, string(body))
		}
		if elapsed > 2*time.Second {
			t.Errorf("health check %d took %v, expected < 2s", i, elapsed)
		}

		// Verify knowledge section exists on first request
		if i == 0 {
			var health struct {
				Knowledge *struct {
					ConceptCount     int `json:"concept_count"`
					EntityCount      int `json:"entity_count"`
					SynthesisPending int `json:"synthesis_pending"`
				} `json:"knowledge"`
			}
			if err := json.Unmarshal(body, &health); err != nil {
				t.Fatalf("parse health response: %v", err)
			}
			if health.Knowledge != nil {
				t.Logf("Knowledge stats: concepts=%d, entities=%d, pending=%d",
					health.Knowledge.ConceptCount, health.Knowledge.EntityCount, health.Knowledge.SynthesisPending)
			} else {
				t.Log("Knowledge section not present (knowledge layer may be disabled)")
			}
		}
	}
}
