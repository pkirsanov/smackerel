// Spec 068 SCOPE-1 — Intent compiler SST fail-loud test.

package config

import (
	"strings"
	"testing"
)

// TestIntentCompilerConfigRequiresEverySSTKey asserts the spec 068
// SCOPE-1 SST keys are all REQUIRED (no defaults; smackerel-no-defaults
// / Gate G028). When any key is missing or unparsable the loader
// records it in errs and the aggregate IntentCompilerMissingKeyError
// names every offender.
func TestIntentCompilerConfigRequiresEverySSTKey(t *testing.T) {
	requiredKeys := []string{
		"ASSISTANT_INTENT_COMPILER_ENABLED",
		"ASSISTANT_INTENT_COMPILER_MODEL_ROLE",
		"ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION",
		"ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION",
		"ASSISTANT_INTENT_COMPILER_TIMEOUT_MS",
		"ASSISTANT_INTENT_COMPILER_CONFIDENCE_FLOOR",
		"ASSISTANT_INTENT_COMPILER_MAX_CONTEXT_TURNS",
		"ASSISTANT_INTENT_COMPILER_MAX_OUTPUT_BYTES",
		"ASSISTANT_INTENT_COMPILER_RETRY_BUDGET",
	}
	// Sub-test 1: every key missing → loader names every required key
	// in the aggregate error.
	t.Run("all_missing_names_every_key", func(t *testing.T) {
		for _, k := range requiredKeys {
			t.Setenv(k, "")
		}
		var errs []string
		cfg := &Config{}
		loadIntentCompilerConfig(cfg, &errs)
		if len(errs) == 0 {
			t.Fatalf("expected loadIntentCompilerConfig to record missing keys, got none")
		}
		joined := strings.Join(errs, ",")
		for _, k := range requiredKeys {
			if !strings.Contains(joined, k) {
				t.Errorf("expected missing-key error to name %q, got %q", k, joined)
			}
		}
		// Aggregate error must be a fail-loud F068-SST-MISSING error.
		err := IntentCompilerMissingKeyError(errs)
		if err == nil {
			t.Fatalf("expected non-nil aggregate error")
		}
		if !strings.Contains(err.Error(), "[F068-SST-MISSING]") {
			t.Fatalf("expected aggregate error to carry [F068-SST-MISSING] tag, got %q", err.Error())
		}
	})

	// Sub-test 2: a fully-populated env produces no errors and
	// populates every field.
	t.Run("fully_populated_no_errors", func(t *testing.T) {
		t.Setenv("ASSISTANT_INTENT_COMPILER_ENABLED", "true")
		t.Setenv("ASSISTANT_INTENT_COMPILER_MODEL_ROLE", "assistant_intent_compiler")
		t.Setenv("ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION", "intent-compiler-v1")
		t.Setenv("ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION", "v1")
		t.Setenv("ASSISTANT_INTENT_COMPILER_TIMEOUT_MS", "5000")
		t.Setenv("ASSISTANT_INTENT_COMPILER_CONFIDENCE_FLOOR", "0.6")
		t.Setenv("ASSISTANT_INTENT_COMPILER_MAX_CONTEXT_TURNS", "8")
		t.Setenv("ASSISTANT_INTENT_COMPILER_MAX_OUTPUT_BYTES", "16384")
		t.Setenv("ASSISTANT_INTENT_COMPILER_RETRY_BUDGET", "1")
		var errs []string
		cfg := &Config{}
		loadIntentCompilerConfig(cfg, &errs)
		if len(errs) != 0 {
			t.Fatalf("expected zero errs, got %v", errs)
		}
		ic := cfg.Assistant.IntentCompiler
		if !ic.Enabled || ic.ModelRole != "assistant_intent_compiler" ||
			ic.PromptContractVersion != "intent-compiler-v1" || ic.SchemaVersion != "v1" ||
			ic.Timeout.Milliseconds() != 5000 || ic.ConfidenceFloor != 0.6 ||
			ic.MaxContextTurns != 8 || ic.MaxOutputBytes != 16384 || ic.RetryBudget != 1 {
			t.Fatalf("loaded IntentCompiler does not match env: %+v", ic)
		}
	})

	// Sub-test 3: each key independently required — clearing any
	// single key adds that key (and only that key) to errs.
	t.Run("each_key_independently_required", func(t *testing.T) {
		base := map[string]string{
			"ASSISTANT_INTENT_COMPILER_ENABLED":                 "true",
			"ASSISTANT_INTENT_COMPILER_MODEL_ROLE":              "x",
			"ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION": "v1",
			"ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION":          "v1",
			"ASSISTANT_INTENT_COMPILER_TIMEOUT_MS":              "1000",
			"ASSISTANT_INTENT_COMPILER_CONFIDENCE_FLOOR":        "0.5",
			"ASSISTANT_INTENT_COMPILER_MAX_CONTEXT_TURNS":       "4",
			"ASSISTANT_INTENT_COMPILER_MAX_OUTPUT_BYTES":        "1024",
			"ASSISTANT_INTENT_COMPILER_RETRY_BUDGET":            "0",
		}
		for _, target := range requiredKeys {
			t.Run("missing_"+target, func(t *testing.T) {
				for k, v := range base {
					if k == target {
						t.Setenv(k, "")
					} else {
						t.Setenv(k, v)
					}
				}
				var errs []string
				loadIntentCompilerConfig(&Config{}, &errs)
				if len(errs) == 0 {
					t.Fatalf("missing %s should produce an err entry; got none", target)
				}
				if !strings.Contains(strings.Join(errs, ","), target) {
					t.Fatalf("missing-key err for %s not present: %v", target, errs)
				}
			})
		}
	})
}
