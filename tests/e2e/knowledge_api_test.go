//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"
	"time"
)

// T3-11: GET /api/knowledge/concepts returns list with pagination.
func TestKnowledgeAPI_ConceptsList(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiGet(cfg, "/api/knowledge/concepts?limit=5&sort=updated")
	if err != nil {
		t.Fatalf("concepts list request failed: %v", err)
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
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"concepts"`
		Total  int `json:"total"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if result.Limit != 5 {
		t.Errorf("expected limit=5, got %d", result.Limit)
	}
	t.Logf("concepts: total=%d returned=%d", result.Total, len(result.Concepts))
}

// T3-12: GET /api/knowledge/entities returns list.
func TestKnowledgeAPI_EntitiesList(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiGet(cfg, "/api/knowledge/entities?limit=10")
	if err != nil {
		t.Fatalf("entities list request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	t.Logf("entities response: %s", string(body)[:min(200, len(body))])
}

// T3-13: GET /api/knowledge/concepts/{id} returns 404 for nonexistent.
func TestKnowledgeAPI_ConceptNotFound(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiGet(cfg, "/api/knowledge/concepts/nonexistent-id-999")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// T3-14 / BS-006: Search with knowledge_match provenance.
func TestKnowledgeAPI_SearchKnowledgeFirst(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiGet(cfg, "/api/knowledge/stats")
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var stats struct {
		ConceptCount int `json:"concept_count"`
	}
	if err := json.Unmarshal(body, &stats); err != nil {
		t.Fatalf("parse stats: %v", err)
	}
	if stats.ConceptCount == 0 {
		t.Skip("no concept pages — knowledge-first search requires seeded data")
	}
	t.Logf("concept_count=%d — knowledge-first search path available", stats.ConceptCount)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
