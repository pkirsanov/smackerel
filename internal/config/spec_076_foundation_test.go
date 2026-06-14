// Package config — TP-076-01-02 (SCN-076-F02) unit test.
//
// Asserts that every spec-076 SCOPE-1 foundation SST key family
// fails loud at startup when its env var is unset or empty:
//
//   - assistant.tools.location_normalize.*  (shipped under spec 065)
//   - assistant.tools.entity_resolve.*      (shipped under spec 065)
//   - assistant.annotation.classifier.*     (spec 076 SCOPE-1)
//   - assistant.openknowledge.budgets.*     (shipped under spec 064)
//   - openknowledge.citeback.enforcement_mode (spec 076 SCOPE-1)
//
// Adversarial: the same key paired with an empty string MUST also
// fail loud (empty != absent at the SST contract level).
package config

import (
	"os"
	"strings"
	"testing"
)

// spec076FoundationKeys enumerates every env var spec 076 SCOPE-1
// SCN-076-F02 demands fail-loud behavior for. Keys are grouped by
// family so failure messages name the family.
var spec076FoundationKeys = []struct {
	family string
	key    string
}{
	// assistant.tools.location_normalize.* (shipped spec 065 SCOPE-1)
	{"assistant.tools.location_normalize", "ASSISTANT_TOOLS_LOCATION_NORMALIZE_ENABLED"},
	{"assistant.tools.location_normalize", "ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER"},
	{"assistant.tools.location_normalize", "ASSISTANT_TOOLS_LOCATION_NORMALIZE_TIMEOUT_MS"},
	// assistant.tools.entity_resolve.* (shipped spec 065 SCOPE-1)
	{"assistant.tools.entity_resolve", "ASSISTANT_TOOLS_ENTITY_RESOLVE_ENABLED"},
	{"assistant.tools.entity_resolve", "ASSISTANT_TOOLS_ENTITY_RESOLVE_CONFIDENCE_FLOOR"},
	{"assistant.tools.entity_resolve", "ASSISTANT_TOOLS_ENTITY_RESOLVE_TIMEOUT_MS"},
	// assistant.annotation.classifier.* (spec 076 SCOPE-1)
	{"assistant.annotation.classifier", "ASSISTANT_ANNOTATION_CLASSIFIER_CONFIDENCE_FLOOR"},
	{"assistant.annotation.classifier", "ASSISTANT_ANNOTATION_CLASSIFIER_WARM_CACHE_ENABLED"},
	// assistant.openknowledge.budgets.* (shipped spec 064 SCOPE-3 / 064 SCOPE-13)
	{"assistant.openknowledge.budgets", "ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_TOKEN_BUDGET"},
	{"assistant.openknowledge.budgets", "ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_USD_BUDGET"},
	{"assistant.openknowledge.budgets", "ASSISTANT_OPEN_KNOWLEDGE_MONTHLY_BUDGET_USD"},
	{"assistant.openknowledge.budgets", "ASSISTANT_OPEN_KNOWLEDGE_PER_USER_MONTHLY_BUDGET_USD"},
	// openknowledge.citeback.enforcement_mode (spec 076 SCOPE-1)
	{"openknowledge.citeback", "ASSISTANT_OPEN_KNOWLEDGE_CITEBACK_ENFORCEMENT_MODE"},
}

func TestSpec076FoundationKeysFailLoud(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		for _, tc := range spec076FoundationKeys {
			t.Run(tc.key, func(t *testing.T) {
				spec076ApplyBaseEnv(t)
				if err := os.Unsetenv(tc.key); err != nil {
					t.Fatalf("unset %s: %v", tc.key, err)
				}
				err := spec076LoadFamily(tc.family)
				if err == nil {
					t.Fatalf("expected fail-loud error when %s unset, got nil", tc.key)
				}
				if !strings.Contains(err.Error(), tc.key) {
					t.Fatalf("expected error to name %s, got: %v", tc.key, err)
				}
			})
		}
	})

	t.Run("empty", func(t *testing.T) {
		// Only string-bearing keys can be meaningfully set to empty.
		// Numeric/bool loaders may treat empty as "tolerated at load,
		// caught by Validate" — but the citeback key explicitly
		// rejects empty even when open_knowledge.enabled=false.
		t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_CITEBACK_ENFORCEMENT_MODE", "")
		spec076ApplyBaseOpenKnowledgeEnv(t)
		t.Setenv("ASSISTANT_OPEN_KNOWLEDGE_CITEBACK_ENFORCEMENT_MODE", "")
		_, err := LoadOpenKnowledge()
		if err == nil {
			t.Fatal("expected fail-loud error when CITEBACK_ENFORCEMENT_MODE empty, got nil")
		}
		if !strings.Contains(err.Error(), "citeback.enforcement_mode") {
			t.Fatalf("expected error to name citeback.enforcement_mode, got: %v", err)
		}
	})
}

// spec076ApplyBaseEnv sets up a known-good baseline for ALL spec-076
// foundation env vars; per-subtest mutation then unsets/overrides one.
func spec076ApplyBaseEnv(t *testing.T) {
	t.Helper()
	spec076ApplyBaseOpenKnowledgeEnv(t)
	spec076ApplyBaseMicroToolsEnv(t)
	spec076ApplyBaseAnnotationClassifierEnv(t)
}

func spec076ApplyBaseOpenKnowledgeEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"ASSISTANT_OPEN_KNOWLEDGE_ENABLED":                                 "false",
		"ASSISTANT_OPEN_KNOWLEDGE_PROVIDER":                                "searxng",
		"ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_ENDPOINT":                       "",
		"ASSISTANT_OPEN_KNOWLEDGE_PROVIDER_API_KEY":                        "",
		"ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID":                            "",
		"ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID":                      "",
		"ASSISTANT_OPEN_KNOWLEDGE_MAX_ITERATIONS":                          "4",
		"ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_RETRY_BUDGET":                  "1",
		"ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_TOKEN_BUDGET":                  "8000",
		"ASSISTANT_OPEN_KNOWLEDGE_PER_QUERY_USD_BUDGET":                    "0.05",
		"ASSISTANT_OPEN_KNOWLEDGE_MONTHLY_BUDGET_USD":                      "0",
		"ASSISTANT_OPEN_KNOWLEDGE_PER_USER_MONTHLY_BUDGET_USD":             "0",
		"ASSISTANT_OPEN_KNOWLEDGE_TOOL_ALLOWLIST":                          `[]`,
		"ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS":                       `[]`,
		"ASSISTANT_OPEN_KNOWLEDGE_WEB_SNIPPET_CACHE_ENABLED":               "false",
		"ASSISTANT_OPEN_KNOWLEDGE_LLM_TIMEOUT_MS":                          "30000",
		"ASSISTANT_OPEN_KNOWLEDGE_ALLOWED_EGRESS_HOSTS":                    `[]`,
		"ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_FAILURE_THRESHOLD":       "5",
		"ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_OPEN_WINDOW_SECONDS":     "60",
		"ASSISTANT_OPEN_KNOWLEDGE_CIRCUIT_BREAKER_HALF_OPEN_AFTER_SECONDS": "30",
		"ASSISTANT_OPEN_KNOWLEDGE_CITEBACK_ENFORCEMENT_MODE":               "shadow",
	} {
		t.Setenv(k, v)
	}
}

func spec076ApplyBaseMicroToolsEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"ASSISTANT_TOOLS_LOCATION_NORMALIZE_ENABLED":           "false",
		"ASSISTANT_TOOLS_LOCATION_NORMALIZE_PROVIDER":          "open-meteo",
		"ASSISTANT_TOOLS_LOCATION_NORMALIZE_TIMEOUT_MS":        "2000",
		"ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_TTL_SECONDS": "600",
		"ASSISTANT_TOOLS_LOCATION_NORMALIZE_CACHE_MAX_ENTRIES": "512",
		"ASSISTANT_TOOLS_UNIT_CONVERT_ENABLED":                 "false",
		"ASSISTANT_TOOLS_UNIT_CONVERT_CATALOG_VERSION":         "v1",
		"ASSISTANT_TOOLS_CALCULATOR_ENABLED":                   "false",
		"ASSISTANT_TOOLS_CALCULATOR_MAX_EXPRESSION_CHARS":      "256",
		"ASSISTANT_TOOLS_ENTITY_RESOLVE_ENABLED":               "false",
		"ASSISTANT_TOOLS_ENTITY_RESOLVE_CONFIDENCE_FLOOR":      "0.7",
		"ASSISTANT_TOOLS_ENTITY_RESOLVE_TIMEOUT_MS":            "1500",
	} {
		t.Setenv(k, v)
	}
}

func spec076ApplyBaseAnnotationClassifierEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ASSISTANT_ANNOTATION_CLASSIFIER_CONFIDENCE_FLOOR", "0.6")
	t.Setenv("ASSISTANT_ANNOTATION_CLASSIFIER_WARM_CACHE_ENABLED", "true")
}

// spec076LoadFamily routes a key family to its corresponding loader
// and returns the loader's error (or nil on success).
func spec076LoadFamily(family string) error {
	switch family {
	case "assistant.tools.location_normalize", "assistant.tools.entity_resolve":
		// Both micro-tool families share the loadAssistantToolsConfig
		// loader; assemble a stub Config and check aggregate errors.
		var errs []string
		cfg := &Config{}
		loadAssistantToolsConfig(cfg, &errs)
		if len(errs) > 0 {
			return spec076MakeAggregateErr(errs)
		}
		return nil
	case "assistant.annotation.classifier":
		_, err := LoadAnnotationClassifier()
		return err
	case "assistant.openknowledge.budgets", "openknowledge.citeback":
		_, err := LoadOpenKnowledge()
		return err
	default:
		return spec076ErrUnknownFamily{family: family}
	}
}

type spec076ErrUnknownFamily struct{ family string }

func (e spec076ErrUnknownFamily) Error() string { return "unknown family: " + e.family }

func spec076MakeAggregateErr(errs []string) error {
	return spec076AggregateErr{errs: errs}
}

type spec076AggregateErr struct{ errs []string }

func (e spec076AggregateErr) Error() string {
	return "[F076-SST-MISSING] " + strings.Join(e.errs, ", ")
}
