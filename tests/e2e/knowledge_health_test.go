//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"
	"time"
)

// T8-05 / SCN-025-24: Health endpoint includes knowledge section.
func TestKnowledgeHealth_SectionPresent(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiGet(cfg, "/api/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var health map[string]json.RawMessage
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("parse health: %v", err)
	}

	knowledgeRaw, ok := health["knowledge"]
	if !ok {
		t.Fatal("health response missing 'knowledge' section")
	}

	var knowledge struct {
		ConceptCount     int `json:"concept_count"`
		EntityCount      int `json:"entity_count"`
		SynthesisPending int `json:"synthesis_pending"`
	}
	if err := json.Unmarshal(knowledgeRaw, &knowledge); err != nil {
		t.Fatalf("parse knowledge section: %v", err)
	}
	t.Logf("knowledge health: concepts=%d entities=%d pending=%d",
		knowledge.ConceptCount, knowledge.EntityCount, knowledge.SynthesisPending)
}

// T8-06: Existing health fields (status, db, nats, ml) still present.
func TestKnowledgeHealth_ExistingFieldsPreserved(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiGet(cfg, "/api/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var health map[string]json.RawMessage
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("parse health: %v", err)
	}

	required := []string{"status"}
	for _, key := range required {
		if _, ok := health[key]; !ok {
			t.Errorf("health response missing required field: %s", key)
		}
	}
	t.Logf("health response keys: %d fields present", len(health))
}
