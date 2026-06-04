// Spec 080 SCOPE-080-01 — knowledge_graph_api.* SST regression tests.
package config

import (
	"strings"
	"testing"
)

// TestValidate_FailsWhenKnowledgeGraphAPIMissing — removing any
// required KNOWLEDGE_GRAPH_API_* env var causes Load() (and the
// underlying loadKnowledgeGraphAPIConfig) to fail loud and name the
// offending key. The setRequiredEnv canonical fixture populates every
// key; each sub-test unsets exactly one and asserts the error
// mentions it.
func TestValidate_FailsWhenKnowledgeGraphAPIMissing(t *testing.T) {
	keys := []string{
		"KNOWLEDGE_GRAPH_API_LIST_DEFAULT_LIMIT",
		"KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT",
		"KNOWLEDGE_GRAPH_API_TIME_WINDOW_MAX_DAYS",
		"KNOWLEDGE_GRAPH_API_EDGES_DEFAULT_LIMIT",
		"KNOWLEDGE_GRAPH_API_EDGES_MAX_LIMIT",
		"KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			setRequiredEnv(t)
			// Unset this single key via Setenv("") + the loader's
			// LookupEnv distinguishes empty-string from unset; the
			// SST contract treats both as missing.
			t.Setenv(key, "")
			_, err := loadKnowledgeGraphAPIConfig()
			if err == nil {
				t.Fatalf("loadKnowledgeGraphAPIConfig returned no error when %s is empty", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error message missing offender %q; got: %s", key, err.Error())
			}
			if !strings.Contains(err.Error(), "F080-SST-MISSING") {
				t.Errorf("error message missing [F080-SST-MISSING] tag; got: %s", err.Error())
			}
		})
	}
}

// TestValidate_KnowledgeGraphAPIHappy — canonical fixture loads
// cleanly. Guards against accidentally tightening Validate beyond
// what the canonical fixture honors.
func TestValidate_KnowledgeGraphAPIHappy(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := loadKnowledgeGraphAPIConfig()
	if err != nil {
		t.Fatalf("loadKnowledgeGraphAPIConfig: %v", err)
	}
	if cfg.ListMaxLimit != 200 {
		t.Errorf("ListMaxLimit = %d; want 200", cfg.ListMaxLimit)
	}
	if cfg.CursorSecretEnv != "KNOWLEDGE_GRAPH_API_CURSOR_SECRET" {
		t.Errorf("CursorSecretEnv = %q", cfg.CursorSecretEnv)
	}
}
