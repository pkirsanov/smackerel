// Spec 095 SCOPE-01 — Retrieval-Strategy Routing SST validation tests.
//
// LoadRetrieval is unconditional fail-loud (no Enabled short-circuit): every
// RETRIEVAL_* key MUST be present and valid or Load() aborts with the
// [F095-SST-MISSING] prefix (SCN-095-S01). vague_recall cannot be disabled
// (SCN-095-S01b). The closed query-shape and judgment-source vocabularies are
// enforced at load time.
//
// References:
//   - specs/095-retrieval-strategy-routing/design.md §10 (SST key set)
//   - specs/095-retrieval-strategy-routing/scopes.md SCOPE-01 (DoD + test plan)
//   - .github/instructions/smackerel-no-defaults.instructions.md
package config

import (
	"strings"
	"testing"
)

// setRetrievalEnv applies a known-good RETRIEVAL_* baseline; per-subtest
// mutation then unsets/overrides one key to exercise the fail-loud path.
func setRetrievalEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"RETRIEVAL_ROUTING_ENABLED":                                  "true",
		"RETRIEVAL_ROUTING_INTENT_CONFIDENCE_THRESHOLD":              "0.65",
		"RETRIEVAL_ROUTING_STRATEGY_WHOLE_DOCUMENT_ENABLED":          "true",
		"RETRIEVAL_ROUTING_STRATEGY_STRUCTURED_AGGREGATE_ENABLED":    "true",
		"RETRIEVAL_ROUTING_STRATEGY_VAGUE_RECALL_ENABLED":            "true",
		"RETRIEVAL_ROUTING_CONTRACTS":                                `{"transcript":["whole_document_summary","vague_recall"],"meeting":["whole_document_summary","vague_recall"],"subscription":["aggregate_spend","vague_recall"],"expense":["aggregate_spend","vague_recall"],"bill":["aggregate_spend","vague_recall"],"place":["dossier","vague_recall"],"trip":["dossier","vague_recall"]}`,
		"RETRIEVAL_EVERGREEN_ENABLED":                                "true",
		"RETRIEVAL_EVERGREEN_JUDGMENT_SOURCE":                        "scenario",
		"RETRIEVAL_EVERGREEN_CONFIDENCE_FLOOR":                       "0.60",
		"RETRIEVAL_EVERGREEN_PER_TICK_BUDGET":                        "50",
		"RETRIEVAL_EVERGREEN_DEDUP_WINDOW_DAYS":                      "7",
		"RETRIEVAL_EVERGREEN_POOLS_SYNTHESIS_EXCLUDES_LOW_EVERGREEN": "true",
		"RETRIEVAL_EVERGREEN_POOLS_DIGEST_EXCLUDES_LOW_EVERGREEN":    "true",
	} {
		t.Setenv(k, v)
	}
}

// TestLoadRetrieval_HappyPath proves every required key round-trips into the
// typed struct with the expected values.
func TestLoadRetrieval_HappyPath(t *testing.T) {
	setRetrievalEnv(t)
	cfg, err := LoadRetrieval()
	if err != nil {
		t.Fatalf("LoadRetrieval should succeed with full baseline env, got: %v", err)
	}
	if !cfg.Routing.Enabled {
		t.Error("routing.enabled should be true")
	}
	if cfg.Routing.IntentConfidenceThreshold != 0.65 {
		t.Errorf("intent_confidence_threshold = %g, want 0.65", cfg.Routing.IntentConfidenceThreshold)
	}
	if !cfg.Routing.WholeDocumentEnabled || !cfg.Routing.StructuredAggregateEnabled || !cfg.Routing.VagueRecallEnabled {
		t.Error("all three strategies should be enabled in the baseline")
	}
	if got := len(cfg.Routing.Contracts); got != 7 {
		t.Fatalf("contracts should declare 7 types, got %d", got)
	}
	transcript, ok := cfg.Routing.Contracts["transcript"]
	if !ok {
		t.Fatal("contracts should include transcript")
	}
	if len(transcript) != 2 || transcript[0] != QueryShapeWholeDocumentSummary || transcript[1] != QueryShapeVagueRecall {
		t.Errorf("transcript contract = %v, want [whole_document_summary vague_recall]", transcript)
	}
	if cfg.Evergreen.JudgmentSource != EvergreenJudgmentScenario {
		t.Errorf("judgment_source = %q, want scenario", cfg.Evergreen.JudgmentSource)
	}
	if cfg.Evergreen.ConfidenceFloor != 0.60 {
		t.Errorf("confidence_floor = %g, want 0.60", cfg.Evergreen.ConfidenceFloor)
	}
	if cfg.Evergreen.PerTickBudget != 50 || cfg.Evergreen.DedupWindowDays != 7 {
		t.Errorf("evergreen bounds = (%d, %d), want (50, 7)", cfg.Evergreen.PerTickBudget, cfg.Evergreen.DedupWindowDays)
	}
	if !cfg.Evergreen.SynthesisExcludesLowEvergreen || !cfg.Evergreen.DigestExcludesLowEvergreen {
		t.Error("both pool-exclusion switches should be true in the baseline")
	}
}

// TestLoadRetrieval_MissingKey_FailsLoud proves SCN-095-S01: each required key
// absent aborts with [F095-SST-MISSING] naming the field. No silent default.
func TestLoadRetrieval_MissingKey_FailsLoud(t *testing.T) {
	keys := []string{
		"RETRIEVAL_ROUTING_ENABLED",
		"RETRIEVAL_ROUTING_INTENT_CONFIDENCE_THRESHOLD",
		"RETRIEVAL_ROUTING_STRATEGY_WHOLE_DOCUMENT_ENABLED",
		"RETRIEVAL_ROUTING_STRATEGY_STRUCTURED_AGGREGATE_ENABLED",
		"RETRIEVAL_ROUTING_STRATEGY_VAGUE_RECALL_ENABLED",
		"RETRIEVAL_ROUTING_CONTRACTS",
		"RETRIEVAL_EVERGREEN_ENABLED",
		"RETRIEVAL_EVERGREEN_JUDGMENT_SOURCE",
		"RETRIEVAL_EVERGREEN_CONFIDENCE_FLOOR",
		"RETRIEVAL_EVERGREEN_PER_TICK_BUDGET",
		"RETRIEVAL_EVERGREEN_DEDUP_WINDOW_DAYS",
		"RETRIEVAL_EVERGREEN_POOLS_SYNTHESIS_EXCLUDES_LOW_EVERGREEN",
		"RETRIEVAL_EVERGREEN_POOLS_DIGEST_EXCLUDES_LOW_EVERGREEN",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			setRetrievalEnv(t)
			t.Setenv(key, "")
			_, err := LoadRetrieval()
			if err == nil {
				t.Fatalf("LoadRetrieval should fail when %s is empty", key)
			}
			if !strings.Contains(err.Error(), "[F095-SST-MISSING]") {
				t.Errorf("error should carry the [F095-SST-MISSING] prefix, got: %v", err)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error should name the offending key %s, got: %v", key, err)
			}
		})
	}
}

// TestLoadRetrieval_Adversarial proves each out-of-range / out-of-vocabulary /
// disabled-fallback case is rejected (no silent acceptance).
func TestLoadRetrieval_Adversarial(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		val     string
		wantSub string
	}{
		{"threshold-above-1", "RETRIEVAL_ROUTING_INTENT_CONFIDENCE_THRESHOLD", "1.5", "INTENT_CONFIDENCE_THRESHOLD"},
		{"threshold-zero-excluded", "RETRIEVAL_ROUTING_INTENT_CONFIDENCE_THRESHOLD", "0", "INTENT_CONFIDENCE_THRESHOLD"},
		{"threshold-not-float", "RETRIEVAL_ROUTING_INTENT_CONFIDENCE_THRESHOLD", "abc", "INTENT_CONFIDENCE_THRESHOLD"},
		{"vague-recall-disabled", "RETRIEVAL_ROUTING_STRATEGY_VAGUE_RECALL_ENABLED", "false", "safe fallback cannot be disabled"},
		{"judgment-bogus", "RETRIEVAL_EVERGREEN_JUDGMENT_SOURCE", "bogus", "JUDGMENT_SOURCE"},
		{"floor-above-1", "RETRIEVAL_EVERGREEN_CONFIDENCE_FLOOR", "1.5", "CONFIDENCE_FLOOR"},
		{"per-tick-zero", "RETRIEVAL_EVERGREEN_PER_TICK_BUDGET", "0", "PER_TICK_BUDGET"},
		{"dedup-negative", "RETRIEVAL_EVERGREEN_DEDUP_WINDOW_DAYS", "-1", "DEDUP_WINDOW_DAYS"},
		{"contracts-bad-json", "RETRIEVAL_ROUTING_CONTRACTS", `{not-json`, "CONTRACTS"},
		{"contracts-unknown-shape", "RETRIEVAL_ROUTING_CONTRACTS", `{"transcript":["bogus_shape"]}`, "unknown query shape"},
		{"contracts-empty-list", "RETRIEVAL_ROUTING_CONTRACTS", `{"transcript":[]}`, "at least one query shape"},
		{"contracts-empty-object", "RETRIEVAL_ROUTING_CONTRACTS", `{}`, "at least one artifact-type contract"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRetrievalEnv(t)
			t.Setenv(tc.key, tc.val)
			_, err := LoadRetrieval()
			if err == nil {
				t.Fatalf("LoadRetrieval should reject %s=%q", tc.key, tc.val)
			}
			if !strings.Contains(err.Error(), "[F095-SST-MISSING]") {
				t.Errorf("error should carry the [F095-SST-MISSING] prefix, got: %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error should mention %q, got: %v", tc.wantSub, err)
			}
		})
	}
}

// TestLoadRetrieval_ContractsNormalizeLowercase proves artifact-type keys are
// lowercased so the registry lookup is case-insensitive.
func TestLoadRetrieval_ContractsNormalizeLowercase(t *testing.T) {
	setRetrievalEnv(t)
	t.Setenv("RETRIEVAL_ROUTING_CONTRACTS", `{"Transcript":["whole_document_summary","vague_recall"]}`)
	cfg, err := LoadRetrieval()
	if err != nil {
		t.Fatalf("LoadRetrieval: %v", err)
	}
	if _, ok := cfg.Routing.Contracts["transcript"]; !ok {
		t.Errorf("contract key should be lowercased to 'transcript', got keys %v", cfg.Routing.Contracts)
	}
}
