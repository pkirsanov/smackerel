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
		ConceptCount       *int            `json:"concept_count"`
		EntityCount        *int            `json:"entity_count"`
		EdgeCount          *int            `json:"edge_count"`
		SynthesisCompleted *int            `json:"synthesis_completed"`
		SynthesisPending   *int            `json:"synthesis_pending"`
		SynthesisFailed    *int            `json:"synthesis_failed"`
		LastSynthesisAt    json.RawMessage `json:"last_synthesis_at"`
		LintFindingsTotal  *int            `json:"lint_findings_total"`
		LintFindingsHigh   *int            `json:"lint_findings_high"`
		PromptContract     *string         `json:"prompt_contract_version"`
	}
	if err := json.Unmarshal(body, &stats); err != nil {
		t.Fatalf("parse stats: %v", err)
	}

	numericFields := map[string]*int{
		"concept_count":       stats.ConceptCount,
		"entity_count":        stats.EntityCount,
		"edge_count":          stats.EdgeCount,
		"synthesis_completed": stats.SynthesisCompleted,
		"synthesis_pending":   stats.SynthesisPending,
		"synthesis_failed":    stats.SynthesisFailed,
		"lint_findings_total": stats.LintFindingsTotal,
		"lint_findings_high":  stats.LintFindingsHigh,
	}
	for fieldName, value := range numericFields {
		if value == nil {
			t.Errorf("%s missing or null", fieldName)
			continue
		}
		if *value < 0 {
			t.Errorf("%s = %d, want non-negative", fieldName, *value)
		}
	}
	if len(stats.LastSynthesisAt) == 0 {
		t.Error("last_synthesis_at missing")
	} else if string(stats.LastSynthesisAt) != "null" {
		var synthesizedAt time.Time
		if err := json.Unmarshal(stats.LastSynthesisAt, &synthesizedAt); err != nil {
			t.Errorf("last_synthesis_at is not null or RFC3339 timestamp: %v", err)
		}
	}
	if stats.PromptContract == nil {
		t.Error("prompt_contract_version missing or null")
	}

	if stats.ConceptCount != nil && stats.EntityCount != nil && stats.EdgeCount != nil && stats.SynthesisCompleted != nil && stats.SynthesisPending != nil && stats.SynthesisFailed != nil && stats.PromptContract != nil {
		t.Logf("knowledge stats: concepts=%d entities=%d edges=%d completed=%d pending=%d failed=%d contract=%s",
			*stats.ConceptCount, *stats.EntityCount, *stats.EdgeCount,
			*stats.SynthesisCompleted, *stats.SynthesisPending, *stats.SynthesisFailed,
			*stats.PromptContract)
	}
}
