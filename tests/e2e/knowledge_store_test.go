//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"
	"time"
)

// T1-10 / SCN-025-01: Knowledge layer tables created by migration — concept CRUD via live DB.
func TestKnowledgeStore_TablesExist(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// GET /api/knowledge/stats verifies tables exist and are queryable
	resp, err := apiGet(cfg, "/api/knowledge/stats")
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var stats struct {
		ConceptCount      int    `json:"concept_count"`
		EntityCount       int    `json:"entity_count"`
		SynthesisCompleted int   `json:"synthesis_completed"`
		SynthesisPending   int   `json:"synthesis_pending"`
		SynthesisFailed    int   `json:"synthesis_failed"`
		PromptContract    string `json:"prompt_contract_version"`
	}
	if err := json.Unmarshal(body, &stats); err != nil {
		t.Fatalf("parse stats: %v", err)
	}
	t.Logf("knowledge stats: concepts=%d entities=%d synthesized=%d pending=%d contract=%s",
		stats.ConceptCount, stats.EntityCount, stats.SynthesisCompleted, stats.SynthesisPending, stats.PromptContract)

	// Tables exist if stats query succeeds without error
	if stats.PromptContract == "" {
		t.Error("prompt_contract_version should not be empty")
	}
}
