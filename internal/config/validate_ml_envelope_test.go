// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"
)

// staticMMPs builds a map[string]ModelMemoryProfile from a legacy
// model→MiB map, encoding each value as WEIGHTS-only (kv_mib_per_1k_ctx: 0),
// so ResidentMiB(anyParallel) == the given MiB. This preserves the exact
// arithmetic the pre-spec-102 tests assert (profile value == resident) while
// exercising the new KV-aware type. Spec 102 SCOPE-102-03.
func staticMMPs(m map[string]int) map[string]ModelMemoryProfile {
	out := make(map[string]ModelMemoryProfile, len(m))
	for k, v := range m {
		out[k] = ModelMemoryProfile{WeightsMiB: v, KVMiBPer1kCtx: 0, NumCtx: 4096}
	}
	return out
}

// mmpJSON renders a legacy model→MiB map as a weights-only
// ML_MODEL_MEMORY_PROFILES_JSON payload (kv 0, num_ctx 4096) so resident ==
// weights, preserving legacy envelope arithmetic under the KV-aware parser.
func mmpJSON(entries ...[2]string) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, e := range entries {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"model":"` + e[0] + `","weights_mib":` + e[1] + `,"kv_mib_per_1k_ctx":0,"num_ctx":4096}`)
	}
	b.WriteByte(']')
	return b.String()
}

// Spec 045 FR-045-002 — Per-service model memory envelope validation
// (BUG-045-001: validator now routes each model env var to its correct
// deploy envelope; ollama-routed against OLLAMA_MEMORY_LIMIT and
// ml-sidecar-routed against ML_MEMORY_LIMIT).
// These tests pin the contract that Validate() rejects:
//   1. Missing ML_MEMORY_LIMIT (named by requiredVars).
//   2. Configured models with no entry in the model_memory_profiles
//      map (named by validateModelEnvelopes).
//   3. Configured models whose profile memory_mib exceeds the deploy
//      envelope of the service that loads them (named by
//      validateModelEnvelopes, with the per-service envelope key
//      reported in the offender's segment).
//   4. Accepts the happy path where every used model fits its bucket.
//
// These tests are referenced by SCN-045-A02 (model envelope routing)
// in specs/045-deploy-resource-filesystem-hardening/scenario-manifest.json
// and SCN-045-001-A / SCN-045-001-B in BUG-045-001's scenario manifest.

// TestValidate_RejectsMissingMLMemoryLimit asserts the requiredVars
// fail-loud path names ML_MEMORY_LIMIT when the env var is empty.
// SCN-045-A02 — fact-of-life: Operators MUST be told that the envelope
// itself is missing before model-fit checks can run.
func TestValidate_RejectsMissingMLMemoryLimit(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ML_MEMORY_LIMIT", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when ML_MEMORY_LIMIT is empty")
	}
	if !strings.Contains(err.Error(), "ML_MEMORY_LIMIT") {
		t.Errorf("error should name ML_MEMORY_LIMIT, got: %v", err)
	}
}

// TestValidate_RejectsOversizedMLModel asserts validateModelEnvelopes
// rejects a configured model whose profile exceeds the deploy
// envelope of the service that loads it. BUG-045-001 — under the
// per-service routing contract, LLM_MODEL and OLLAMA_MODEL route
// against OLLAMA_MEMORY_LIMIT, not ML_MEMORY_LIMIT, so this test
// also tightens OLLAMA_MEMORY_LIMIT to surface the fail-loud path.
// SCN-045-A02 — fact-of-life: Sizing an envelope smaller than the
// largest configured model MUST fail-loud at startup, not at first
// inference.
func TestValidate_RejectsOversizedMLModel(t *testing.T) {
	setRequiredEnv(t)
	// Set BOTH envelopes to 1G (1024 MiB) but configure a model whose
	// profile is 2048 MiB on the ollama bucket — must reject loudly
	// naming OLLAMA_MEMORY_LIMIT (the envelope of the bucket the
	// offender routes into).
	t.Setenv("ML_MEMORY_LIMIT", "1G")
	t.Setenv("OLLAMA_MEMORY_LIMIT", "1G")
	t.Setenv("OLLAMA_MODEL", "big-model")
	t.Setenv("LLM_MODEL", "big-model")
	t.Setenv("EMBEDDING_MODEL", "small-model")
	t.Setenv("ML_MODEL_MEMORY_PROFILES_JSON",
		mmpJSON([2]string{"big-model", "2048"}, [2]string{"small-model", "256"}, [2]string{"gemma4:26b", "256"}, [2]string{"nomic-embed-text", "256"}, [2]string{"deepseek-ocr:3b", "256"}))

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when configured model exceeds its bucket envelope")
	}
	if !strings.Contains(err.Error(), "FR-045-002") {
		t.Errorf("error should reference spec 045 FR-045-002, got: %v", err)
	}
	if !strings.Contains(err.Error(), "big-model") {
		t.Errorf("error should name the offending model 'big-model', got: %v", err)
	}
	if !strings.Contains(err.Error(), "envelope exceeded") {
		t.Errorf("error should say 'envelope exceeded', got: %v", err)
	}
	// BUG-045-001 — error MUST name the OLLAMA_MEMORY_LIMIT envelope
	// for the ollama-routed offender, NOT the ML_MEMORY_LIMIT envelope.
	if !strings.Contains(err.Error(), "OLLAMA_MEMORY_LIMIT") {
		t.Errorf("error should name OLLAMA_MEMORY_LIMIT for ollama-routed offender, got: %v", err)
	}
}

// TestValidate_RejectsMissingModelProfileEntry asserts
// validateModelEnvelopes rejects a configured model that has no profile
// entry. SCN-045-A02 — fact-of-life: Adding a new model to the runtime
// config without a memory profile is a setup defect that MUST fail-loud
// before the model is loaded into Ollama at runtime.
func TestValidate_RejectsMissingModelProfileEntry(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OLLAMA_MODEL", "unprofiled-model")
	t.Setenv("LLM_MODEL", "profiled-model")
	t.Setenv("EMBEDDING_MODEL", "profiled-embed")
	t.Setenv("ML_MODEL_MEMORY_PROFILES_JSON",
		mmpJSON([2]string{"profiled-model", "1024"}, [2]string{"profiled-embed", "256"}, [2]string{"gemma4:26b", "256"}, [2]string{"nomic-embed-text", "256"}, [2]string{"deepseek-ocr:3b", "256"}))

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when configured model has no profile entry")
	}
	if !strings.Contains(err.Error(), "FR-045-002") {
		t.Errorf("error should reference spec 045 FR-045-002, got: %v", err)
	}
	if !strings.Contains(err.Error(), "unprofiled-model") {
		t.Errorf("error should name the unprofiled model, got: %v", err)
	}
	if !strings.Contains(err.Error(), "missing model memory profile") {
		t.Errorf("error should say 'missing model memory profile', got: %v", err)
	}
}

// TestValidate_AcceptsModelWithinEnvelope asserts the happy path:
// when every configured model has a profile and every profile fits
// within ML_MEMORY_LIMIT, Load() succeeds. SCN-045-A02 — fact-of-life:
// The envelope check MUST NOT false-positive on correctly-sized
// configurations or it would block legitimate deployments.
func TestValidate_AcceptsModelWithinEnvelope(t *testing.T) {
	setRequiredEnv(t)
	// 3G envelope (3072 MiB), all models comfortably under that.
	t.Setenv("ML_MEMORY_LIMIT", "3G")
	t.Setenv("OLLAMA_MODEL", "fits-model")
	t.Setenv("LLM_MODEL", "fits-model")
	t.Setenv("EMBEDDING_MODEL", "fits-embed")
	t.Setenv("ML_MODEL_MEMORY_PROFILES_JSON",
		mmpJSON([2]string{"fits-model", "2048"}, [2]string{"fits-embed", "256"}, [2]string{"gemma4:26b", "256"}, [2]string{"nomic-embed-text", "256"}, [2]string{"deepseek-ocr:3b", "256"}))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should succeed when every model fits within envelope, got: %v", err)
	}
	if cfg.MLMemoryLimitMiB != 3072 {
		t.Errorf("MLMemoryLimitMiB: expected 3072 (3G), got %d", cfg.MLMemoryLimitMiB)
	}
	if cfg.MLModelMemoryProfiles["fits-model"].WeightsMiB != 2048 {
		t.Errorf("MLModelMemoryProfiles[fits-model].WeightsMiB: expected 2048, got %d",
			cfg.MLModelMemoryProfiles["fits-model"].WeightsMiB)
	}
}

// TestValidateModelEnvelopes_SwitchableOverEnvelopeRejected_Spec088
// (ADVERSARIAL) pins the spec-088 SCOPE-01 switchable_models co-residence
// envelope guard. A switchable entry that busts the ollama envelope
// co-resident with the gather model is rejected as oversized; an
// un-profiled switchable entry is rejected as a missing profile; an
// envelope-consistent set passes; and a dev env with an unknown ollama
// envelope (OllamaMemoryLimitMiB == 0) skips the check (matching the
// runtime modelswitch.Allowlist semantics). Calls validateModelEnvelopes
// directly on a hand-built Config so the assertion is not diluted by the
// whole-config env dance — the ollama/ml bucket model fields are left
// empty and skipped. SCN-088-A07 / FR-10.
func TestValidateModelEnvelopes_SwitchableOverEnvelopeRejected_Spec088(t *testing.T) {
	profiles := staticMMPs(map[string]int{
		"gemma4:26b":      18432,
		"deepseek-r1:7b":  4864,
		"deepseek-r1:32b": 22528,
	})
	base := func() *Config {
		c := &Config{
			MLModelMemoryProfiles: profiles,
			OllamaNumParallel:     4,
			OllamaMaxLoadedModels: 2, // co-resident posture: activate the sum checks (spec 102)
			MLMemoryLimit:         "4G",
			MLMemoryLimitMiB:      4096,
			OllamaMemoryLimit:     "28G",
			OllamaMemoryLimitMiB:  28672,
		}
		c.Assistant.OpenKnowledge.Enabled = true
		c.Assistant.OpenKnowledge.LLMModelID = "gemma4:26b" // gather (co-resident during synthesis)
		return c
	}

	t.Run("over-envelope switchable entry rejected", func(t *testing.T) {
		c := base()
		// deepseek-r1:32b co-resident with the gemma4:26b gather model =
		// 18432 + 22528 = 40960 MiB > 28672 MiB envelope.
		c.Assistant.OpenKnowledge.SwitchableModels = []string{"deepseek-r1:7b", "deepseek-r1:32b"}
		err := c.validateModelEnvelopes()
		if err == nil {
			t.Fatalf("expected fail-loud envelope error for over-envelope switchable entry")
		}
		if !strings.Contains(err.Error(), "deepseek-r1:32b") {
			t.Fatalf("error should name the offending switchable model, got: %v", err)
		}
		if !strings.Contains(err.Error(), "switchable_models") {
			t.Fatalf("error should name switchable_models, got: %v", err)
		}
		if !strings.Contains(err.Error(), "envelope exceeded") {
			t.Fatalf("error should say 'envelope exceeded', got: %v", err)
		}
		// The fitting entry (deepseek-r1:7b co-resident = 23296 <= 28672)
		// MUST NOT be flagged — proving the guard is selective.
		if strings.Contains(err.Error(), "\"deepseek-r1:7b\"") {
			t.Fatalf("fitting switchable entry must not be flagged: %v", err)
		}
	})

	t.Run("unprofiled switchable entry rejected as missing profile", func(t *testing.T) {
		c := base()
		c.Assistant.OpenKnowledge.SwitchableModels = []string{"totally-made-up"}
		err := c.validateModelEnvelopes()
		if err == nil {
			t.Fatalf("expected fail-loud missing-profile error for un-profiled switchable entry")
		}
		if !strings.Contains(err.Error(), "totally-made-up") {
			t.Fatalf("error should name the un-profiled model, got: %v", err)
		}
		if !strings.Contains(err.Error(), "missing model memory profile") {
			t.Fatalf("error should say 'missing model memory profile', got: %v", err)
		}
	})

	t.Run("envelope-consistent switchable set accepted", func(t *testing.T) {
		c := base()
		// gemma4:26b == gather (single load 18432); deepseek-r1:7b
		// co-resident = 23296 — both <= 28672.
		c.Assistant.OpenKnowledge.SwitchableModels = []string{"gemma4:26b", "deepseek-r1:7b"}
		if err := c.validateModelEnvelopes(); err != nil {
			t.Fatalf("envelope-consistent switchable set must pass, got: %v", err)
		}
	})

	t.Run("dev envelope 0 skips switchable check", func(t *testing.T) {
		c := base()
		c.OllamaMemoryLimit = ""
		c.OllamaMemoryLimitMiB = 0
		// MLMemoryLimitMiB stays non-zero so the function proceeds past the
		// both-envelopes-missing early return; the switchable pass itself is
		// gated on OllamaMemoryLimitMiB != 0, so this over-envelope entry is
		// NOT checked (matches dev: no ollama daemon, no known envelope).
		c.Assistant.OpenKnowledge.SwitchableModels = []string{"deepseek-r1:32b"}
		if err := c.validateModelEnvelopes(); err != nil {
			t.Fatalf("dev (ollama envelope 0) must skip the switchable envelope check, got: %v", err)
		}
	})
}

// TestValidateModelEnvelopes_StandingDefaultOverEnvelopeRejected_Spec089
// (ADVERSARIAL) pins the spec-089 SCOPE-01 standing-default co-residence
// envelope guard — the CT-6 gap close. The STANDING DEFAULT synthesis
// model (synthesis_model_id) runs on EVERY /ask with no override; before
// spec 089 it was the ONE large selection NOT envelope-checked (only the
// switchable entries were). This guard rejects a standing default whose
// co-resident profile (default + gather) busts OllamaMemoryLimitMiB,
// naming the offending model + the envelope. Calls validateModelEnvelopes
// directly on a hand-built Config (the ollama/ml bucket model fields are
// left empty and skipped). SCN-089-A06 / FR-2.
func TestValidateModelEnvelopes_StandingDefaultOverEnvelopeRejected_Spec089(t *testing.T) {
	profiles := staticMMPs(map[string]int{
		"gemma4:26b":      18432,
		"deepseek-r1:7b":  4864,
		"deepseek-r1:32b": 22528,
		"llama3.1:8b":     6144,
	})
	base := func() *Config {
		c := &Config{
			MLModelMemoryProfiles: profiles,
			OllamaNumParallel:     4,
			OllamaMaxLoadedModels: 2, // co-resident posture: activate the sum checks (spec 102)
			MLMemoryLimit:         "4G",
			MLMemoryLimitMiB:      4096,
			OllamaMemoryLimit:     "28G",
			OllamaMemoryLimitMiB:  28672,
		}
		c.Assistant.OpenKnowledge.Enabled = true
		c.Assistant.OpenKnowledge.LLMModelID = "gemma4:26b" // gather (co-resident during synthesis)
		// A profiled, fitting switchable + tool-capable set so ONLY the
		// standing-default guard can be the source of a rejection here.
		c.Assistant.OpenKnowledge.SwitchableModels = []string{"gemma4:26b"}
		c.Assistant.OpenKnowledge.ToolCapableGatherModels = []string{"gemma4:26b"}
		return c
	}

	t.Run("over-envelope standing default rejected at 28G", func(t *testing.T) {
		c := base()
		// deepseek-r1:32b standing default co-resident with the gemma4:26b
		// gather model = 22528 + 18432 = 40960 MiB > 28672 MiB envelope.
		// This is the every-query default — the exact CT-6 gap.
		c.Assistant.OpenKnowledge.SynthesisModelID = "deepseek-r1:32b"
		err := c.validateModelEnvelopes()
		if err == nil {
			t.Fatalf("expected fail-loud envelope error for over-envelope STANDING DEFAULT (the CT-6 gap)")
		}
		if !strings.Contains(err.Error(), "deepseek-r1:32b") {
			t.Fatalf("error should name the offending standing-default model, got: %v", err)
		}
		if !strings.Contains(err.Error(), "synthesis_model_id") {
			t.Fatalf("error should name synthesis_model_id (standing default), got: %v", err)
		}
		if !strings.Contains(err.Error(), "OLLAMA_MEMORY_LIMIT") {
			t.Fatalf("error should name the OLLAMA_MEMORY_LIMIT envelope, got: %v", err)
		}
		if !strings.Contains(err.Error(), "envelope exceeded") {
			t.Fatalf("error should say 'envelope exceeded', got: %v", err)
		}
	})

	t.Run("unprofiled standing default rejected as missing profile", func(t *testing.T) {
		c := base()
		c.Assistant.OpenKnowledge.SynthesisModelID = "totally-made-up-synth"
		err := c.validateModelEnvelopes()
		if err == nil {
			t.Fatalf("expected fail-loud missing-profile error for an un-profiled standing default")
		}
		if !strings.Contains(err.Error(), "totally-made-up-synth") {
			t.Fatalf("error should name the un-profiled standing default, got: %v", err)
		}
		if !strings.Contains(err.Error(), "missing model memory profile") {
			t.Fatalf("error should say 'missing model memory profile', got: %v", err)
		}
	})
}

// TestValidateModelEnvelopes_StandingDefaultCoResidenceFits_Spec089 pins
// the not-over-tight half of the guard: at the raised 48G envelope the
// same deepseek-r1:32b standing default co-resident with the gemma4:26b
// gather (40960 <= 49152) PASSES, AND the spec-089 self-hosted switchable
// set [deepseek-r1:32b, deepseek-r1:7b, gemma4:26b] all fit. The guard
// must not false-positive on the real shipped self-hosted configuration.
// SCN-089-A06 / SCN-089-A01.
func TestValidateModelEnvelopes_StandingDefaultCoResidenceFits_Spec089(t *testing.T) {
	profiles := staticMMPs(map[string]int{
		"gemma4:26b":      18432,
		"deepseek-r1:7b":  4864,
		"deepseek-r1:32b": 22528,
		"llama3.1:8b":     6144,
	})
	c := &Config{
		MLModelMemoryProfiles: profiles,
		OllamaNumParallel:     4,
		OllamaMaxLoadedModels: 2, // co-resident posture: require the sum fits (spec 102)
		MLMemoryLimit:         "4G",
		MLMemoryLimitMiB:      4096,
		OllamaMemoryLimit:     "48G",
		OllamaMemoryLimitMiB:  49152,
	}
	c.Assistant.OpenKnowledge.Enabled = true
	c.Assistant.OpenKnowledge.LLMModelID = "gemma4:26b"
	c.Assistant.OpenKnowledge.SynthesisModelID = "deepseek-r1:32b" // 22528 + 18432 = 40960 <= 49152
	c.Assistant.OpenKnowledge.SwitchableModels = []string{"deepseek-r1:32b", "deepseek-r1:7b", "gemma4:26b"}
	c.Assistant.OpenKnowledge.ToolCapableGatherModels = []string{"gemma4:26b", "llama3.1:8b"}
	if err := c.validateModelEnvelopes(); err != nil {
		t.Fatalf("the shipped 48G self-hosted standing-default + switchable + tool-capable set MUST pass, got: %v", err)
	}
}

// TestValidateModelEnvelopes_ToolCapableGatherEntryUnprofiledRejected_Spec089
// (ADVERSARIAL) pins the per-entry profile sanity for the new
// tool_capable_gather_models set: a gather-override model with no
// model_memory_profiles entry cannot be loaded and is rejected fail-loud.
// SCN-089-A07 (supplementary) / FR-8.
func TestValidateModelEnvelopes_ToolCapableGatherEntryUnprofiledRejected_Spec089(t *testing.T) {
	profiles := staticMMPs(map[string]int{
		"gemma4:26b":      18432,
		"deepseek-r1:32b": 22528,
	})
	c := &Config{
		MLModelMemoryProfiles: profiles,
		OllamaNumParallel:     4,
		OllamaMaxLoadedModels: 2,
		MLMemoryLimit:         "4G",
		MLMemoryLimitMiB:      4096,
		OllamaMemoryLimit:     "48G",
		OllamaMemoryLimitMiB:  49152,
	}
	c.Assistant.OpenKnowledge.Enabled = true
	c.Assistant.OpenKnowledge.LLMModelID = "gemma4:26b"
	c.Assistant.OpenKnowledge.SynthesisModelID = "deepseek-r1:32b"
	c.Assistant.OpenKnowledge.SwitchableModels = []string{"gemma4:26b"}
	// An un-profiled tool-capable gather entry — the gather override could
	// never be loaded; must fail loud naming the model + the set key.
	c.Assistant.OpenKnowledge.ToolCapableGatherModels = []string{"gemma4:26b", "phantom-gather-model"}
	err := c.validateModelEnvelopes()
	if err == nil {
		t.Fatalf("expected fail-loud missing-profile error for an un-profiled tool_capable_gather_models entry")
	}
	if !strings.Contains(err.Error(), "phantom-gather-model") {
		t.Fatalf("error should name the un-profiled gather model, got: %v", err)
	}
	if !strings.Contains(err.Error(), "tool_capable_gather_models") {
		t.Fatalf("error should name tool_capable_gather_models, got: %v", err)
	}
	if !strings.Contains(err.Error(), "missing model memory profile") {
		t.Fatalf("error should say 'missing model memory profile', got: %v", err)
	}
}

// TestParseComposeMemoryToMiB asserts the unit-suffix parser handles
// every form the docker-compose memory contract accepts. Exercises the
// helper directly so spec 045 envelope sizing is provably correct
// independent of Validate() wiring.
func TestParseComposeMemoryToMiB(t *testing.T) {
	cases := []struct {
		raw      string
		expected int
		wantErr  bool
	}{
		{"512M", 512, false},
		{"512MB", 512, false},
		{"512MiB", 512, false},
		{"1G", 1024, false},
		{"1GB", 1024, false},
		{"1GiB", 1024, false},
		{"3G", 3072, false},
		{"8G", 8192, false},
		{"1024K", 1, false},
		{"1024KiB", 1, false},
		{"", 0, true},
		{"abc", 0, true},
		{"3X", 0, true},
		{"-1G", 0, true},
		{"0G", 0, true},
	}
	for _, tc := range cases {
		got, err := parseComposeMemoryToMiB(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseComposeMemoryToMiB(%q): expected error, got %d", tc.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseComposeMemoryToMiB(%q): unexpected error: %v", tc.raw, err)
			continue
		}
		if got != tc.expected {
			t.Errorf("parseComposeMemoryToMiB(%q): expected %d MiB, got %d", tc.raw, tc.expected, got)
		}
	}
}

// --- BUG-045-001 — Per-service envelope routing adversarial tests ---
//
// Synthetic fixture names per DD-6 decouple test correctness from live
// model availability:
//   - bug-045-fixture-llm-6gib   → 6144 MiB (fits 8 GiB ollama, fails 3 GiB ml-sidecar)
//   - bug-045-fixture-llm-20gib  → 20480 MiB (exceeds even 10 GiB ollama envelope)
//   - bug-045-fixture-embed-512mib → 512 MiB (fits any sane ml-sidecar envelope)
//
// setBug045RoutingFixture populates a COMPLETE Config env fixture so
// the per-bucket loop in validateModelEnvelopes sees every ollama-
// routed and ml-sidecar-routed field as a non-empty value (otherwise
// the per-bucket validator's empty-skip branch would hide an
// un-populated field and the test would be a false-positive on the
// post-fix code path — DD-6 rationale). All ollama-routed env vars
// EXCEPT LLM_MODEL and OLLAMA_MODEL are set to the 6 GiB fixture so
// they all fit the 8 GiB envelope; LLM_MODEL and OLLAMA_MODEL are
// per-test overrides because each scenario exercises a different
// offender shape.
func setBug045RoutingFixture(t *testing.T) {
	t.Helper()
	setRequiredEnv(t)
	t.Setenv("ML_MODEL_MEMORY_PROFILES_JSON",
		mmpJSON(
			[2]string{"bug-045-fixture-llm-6gib", "6144"},
			[2]string{"bug-045-fixture-llm-20gib", "20480"},
			[2]string{"bug-045-fixture-embed-512mib", "512"},
		))
	// Ml-sidecar bucket — both fields use the 512 MiB synthetic embed.
	t.Setenv("EMBEDDING_MODEL", "bug-045-fixture-embed-512mib")
	t.Setenv("PHOTOS_INTELLIGENCE_EMBED_MODEL", "bug-045-fixture-embed-512mib")
	// Ollama bucket — default every ollama-routed env var to the 6 GiB
	// fixture so every bucket member is non-empty. Per-test overrides
	// then point LLM_MODEL and OLLAMA_MODEL at the offender shape.
	t.Setenv("LLM_MODEL", "bug-045-fixture-llm-6gib")
	t.Setenv("OLLAMA_MODEL", "bug-045-fixture-llm-6gib")
	t.Setenv("OLLAMA_VISION_MODEL", "bug-045-fixture-llm-6gib")
	t.Setenv("PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", "bug-045-fixture-llm-6gib")
	t.Setenv("PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL", "bug-045-fixture-llm-6gib")
	t.Setenv("PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", "bug-045-fixture-llm-6gib")
	t.Setenv("PHOTOS_INTELLIGENCE_OCR_MODEL", "bug-045-fixture-llm-6gib")
	t.Setenv("AGENT_PROVIDER_DEFAULT_MODEL", "bug-045-fixture-llm-6gib")
	t.Setenv("AGENT_PROVIDER_REASONING_MODEL", "bug-045-fixture-llm-6gib")
	t.Setenv("AGENT_PROVIDER_FAST_MODEL", "bug-045-fixture-llm-6gib")
	t.Setenv("AGENT_PROVIDER_VISION_MODEL", "bug-045-fixture-llm-6gib")
	t.Setenv("AGENT_PROVIDER_OCR_MODEL", "bug-045-fixture-llm-6gib")
}

// TestValidateModelEnvelopes_AC5a_OllamaRoutedFitsOllamaEnvelopeAccepted
// is the REGRESSION DETECTOR for BUG-045-001 (SCN-045-001-A). The
// fixture configures every ollama-routed env var to a 6144 MiB model
// and the ml-sidecar bucket to a 512 MiB model. Envelopes are
// OLLAMA_MEMORY_LIMIT=8G (8192 MiB) and ML_MEMORY_LIMIT=3G (3072 MiB).
//
// Under the post-fix per-service routing contract: 6144 <= 8192 (ollama)
// and 512 <= 3072 (ml-sidecar) — Load() returns no error.
//
// Under the pre-fix single-bucket validator (HEAD de49b2f9): the
// 6144 MiB ollama-routed model is checked against the ML_MEMORY_LIMIT
// envelope (3072 MiB) and incorrectly REJECTED with
// "LLM_MODEL=\"bug-045-fixture-llm-6gib\" requires 6144 MiB but
// ML_MEMORY_LIMIT=\"3G\" resolves to 3072 MiB" — this test would FAIL.
//
// AC-5(a) — fact-of-life: A correctly-sized ollama-routed model MUST
// NOT be rejected because the ml-sidecar envelope is smaller than
// the ollama envelope.
func TestValidateModelEnvelopes_AC5a_OllamaRoutedFitsOllamaEnvelopeAccepted(t *testing.T) {
	setBug045RoutingFixture(t)
	// OLLAMA_MEMORY_LIMIT=8G and ML_MEMORY_LIMIT=3G are inherited from
	// setRequiredEnv defaults; no override needed.

	cfg, err := Load()
	if err != nil {
		t.Fatalf("AC-5(a) — post-fix per-service routing: Load() should accept ollama-routed 6 GiB model against 8 GiB ollama envelope. The pre-fix single-bucket validator would have rejected this. Got error: %v", err)
	}
	if cfg.OllamaMemoryLimitMiB != 8192 {
		t.Errorf("expected OllamaMemoryLimitMiB=8192 (8G), got %d", cfg.OllamaMemoryLimitMiB)
	}
	if cfg.MLMemoryLimitMiB != 3072 {
		t.Errorf("expected MLMemoryLimitMiB=3072 (3G), got %d", cfg.MLMemoryLimitMiB)
	}
	if cfg.MLModelMemoryProfiles["bug-045-fixture-llm-6gib"].WeightsMiB != 6144 {
		t.Errorf("expected fixture profile 6144 MiB, got %d", cfg.MLModelMemoryProfiles["bug-045-fixture-llm-6gib"].WeightsMiB)
	}
}

// TestValidateModelEnvelopes_AC5b_OllamaRoutedExceedsOllamaEnvelopeRejectedWithCorrectEnvelopeName
// (SCN-045-001-B) asserts the error-message contract: when an
// ollama-routed model exceeds its bucket envelope, the offender's
// segment of the error MUST name OLLAMA_MEMORY_LIMIT (NOT
// ML_MEMORY_LIMIT). LLM_MODEL is set to the 20480 MiB synthetic
// fixture against an OLLAMA_MEMORY_LIMIT of 10G (10240 MiB) and an
// ML_MEMORY_LIMIT of 3G (3072 MiB). All other ollama-routed fields
// stay on the 6144 MiB fits fixture so they do NOT appear in the
// error.
//
// AC-5(b) — fact-of-life: An oversized model's error message MUST
// name the envelope of the deploy service that actually loads the
// model, otherwise the operator's mental model of which envelope
// to raise is broken.
func TestValidateModelEnvelopes_AC5b_OllamaRoutedExceedsOllamaEnvelopeRejectedWithCorrectEnvelopeName(t *testing.T) {
	setBug045RoutingFixture(t)
	// Override OLLAMA_MEMORY_LIMIT to 10G to make the assertion
	// envelope-specific (8G default would also reject, but a distinct
	// 10G value lets us assert it appears verbatim in the error).
	t.Setenv("OLLAMA_MEMORY_LIMIT", "10G")
	// LLM_MODEL is the offender; OLLAMA_MODEL stays on the 6 GiB fits
	// fixture so it should NOT appear in the oversized segment.
	t.Setenv("LLM_MODEL", "bug-045-fixture-llm-20gib")

	_, err := Load()
	if err == nil {
		t.Fatal("AC-5(b) — expected Load() to reject 20 GiB ollama-routed model against 10 GiB ollama envelope")
	}
	msg := err.Error()
	if !strings.Contains(msg, "FR-045-002") {
		t.Errorf("AC-5(b) — error should reference FR-045-002, got: %v", err)
	}
	if !strings.Contains(msg, "bug-045-fixture-llm-20gib") {
		t.Errorf("AC-5(b) — error should name the offending model bug-045-fixture-llm-20gib, got: %v", err)
	}
	if !strings.Contains(msg, "20480") {
		t.Errorf("AC-5(b) — error should report the offender's required MiB (20480), got: %v", err)
	}
	if !strings.Contains(msg, "OLLAMA_MEMORY_LIMIT") {
		t.Errorf("AC-5(b) — error MUST name OLLAMA_MEMORY_LIMIT (the bucket envelope of the offender), got: %v", err)
	}
	if !strings.Contains(msg, "10G") {
		t.Errorf("AC-5(b) — error MUST contain the raw envelope value 10G, got: %v", err)
	}
	// AC-5(b) negative assertion: the SEGMENT naming the offender MUST
	// NOT name ML_MEMORY_LIMIT as its envelope. We slice the error at
	// the offender's model name and inspect the surrounding context to
	// the next "; " separator (next offender segment) or the end of
	// the parts join. The expected format is
	//   "<envVar>=\"<model>\" requires N MiB but OLLAMA_MEMORY_LIMIT=\"10G\" resolves to 10240 MiB"
	// — so the substring immediately following the offender's model
	// must contain OLLAMA_MEMORY_LIMIT and must NOT contain
	// ML_MEMORY_LIMIT.
	idx := strings.Index(msg, "bug-045-fixture-llm-20gib")
	if idx == -1 {
		t.Fatalf("AC-5(b) — could not locate offender model in error message: %v", err)
	}
	// Inspect a 200-character window after the offender — large
	// enough to cover the templated suffix even if other formatting
	// changes (e.g. a trailing parenthetical).
	end := idx + 200
	if end > len(msg) {
		end = len(msg)
	}
	segment := msg[idx:end]
	if !strings.Contains(segment, "OLLAMA_MEMORY_LIMIT") {
		t.Errorf("AC-5(b) — offender's segment MUST name OLLAMA_MEMORY_LIMIT, got segment: %q (full error: %v)", segment, err)
	}
	// Truncate the segment at the next "; " (segment separator) so a
	// later offender on the ML side does not poison the assertion.
	if sepIdx := strings.Index(segment, "; "); sepIdx != -1 {
		segment = segment[:sepIdx]
	}
	if strings.Contains(segment, "ML_MEMORY_LIMIT") {
		t.Errorf("AC-5(b) — offender's segment MUST NOT name ML_MEMORY_LIMIT (the wrong envelope); got segment: %q (full error: %v)", segment, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Spec 082 SCOPE-082-02 — concurrent interactive-set ollama envelope guard.
//
// The per-model checks above ensure each model fits OLLAMA_MEMORY_LIMIT
// ALONE. But under a resident keep-alive, ollama retains every model it
// loads, so the distinct interactive hot-path models are co-resident and
// their SUM must ALSO fit the envelope — otherwise Docker OOM-kills the
// ollama container into a restart crash-loop (the live self-hosted failure
// these tests prevent). The guard sums the distinct interactive slots
// (LLM_MODEL + OLLAMA_MODEL + OLLAMA_VISION_MODEL + AGENT_PROVIDER_DEFAULT
// + AGENT_PROVIDER_FAST + AGENT_PROVIDER_VISION) and rejects when resident
// AND sum > OLLAMA_MEMORY_LIMIT.
//
// Covers SCN-082-B01 (reject) and SCN-082-B02 (accept, no false positive).
// ─────────────────────────────────────────────────────────────────────────

// spec082InteractiveProfiles is a complete model-profile fixture covering
// every model referenced by setRequiredEnv() after the SCOPE-082-02
// interactive overrides below. big-a (18432) and big-b (6144) each fit a
// 20 GiB ollama envelope ALONE but sum to 24576 MiB (> 20480), exactly the
// self-hosted gemma4:26b + llama3.1:8b over-subscription this guard catches.
const spec082InteractiveProfiles = `[` +
	`{"model":"spec082-big-a-18g","weights_mib":18432,"kv_mib_per_1k_ctx":0,"num_ctx":4096},` +
	`{"model":"spec082-big-b-6g","weights_mib":6144,"kv_mib_per_1k_ctx":0,"num_ctx":4096},` +
	`{"model":"spec082-small-x-1g","weights_mib":1024,"kv_mib_per_1k_ctx":0,"num_ctx":4096},` +
	`{"model":"spec082-small-y-4g","weights_mib":4096,"kv_mib_per_1k_ctx":0,"num_ctx":4096},` +
	`{"model":"all-MiniLM-L6-v2","weights_mib":256,"kv_mib_per_1k_ctx":0,"num_ctx":4096},` +
	`{"model":"gemma4:26b","weights_mib":3072,"kv_mib_per_1k_ctx":0,"num_ctx":4096},` +
	`{"model":"nomic-embed-text","weights_mib":256,"kv_mib_per_1k_ctx":0,"num_ctx":4096},` +
	`{"model":"deepseek-ocr:3b","weights_mib":2560,"kv_mib_per_1k_ctx":0,"num_ctx":4096}]`

// TestValidate_RejectsOversubscribedInteractiveOllamaSet — SCN-082-B01.
// Two interactive models each fit a 20 GiB envelope alone but sum to
// 24576 MiB under a resident keep-alive → fail loud.
//
// ADVERSARIAL: this test FAILS if the concurrent-sum branch in
// validateModelEnvelopes is removed (Load() would then succeed because
// every model fits the envelope individually).
func TestValidate_RejectsOversubscribedInteractiveOllamaSet(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OLLAMA_MEMORY_LIMIT", "20G")    // 20480 MiB
	t.Setenv("OLLAMA_KEEP_ALIVE", "-1")       // resident → sum guard active
	t.Setenv("OLLAMA_MAX_LOADED_MODELS", "2") // co-resident posture → sum guard active (spec 102)
	// Distinct interactive set = {big-a 18432, big-b 6144} = 24576 MiB.
	t.Setenv("LLM_MODEL", "spec082-big-a-18g")
	t.Setenv("OLLAMA_MODEL", "spec082-big-a-18g")
	t.Setenv("OLLAMA_VISION_MODEL", "spec082-big-a-18g")
	t.Setenv("AGENT_PROVIDER_DEFAULT_MODEL", "spec082-big-b-6g")
	t.Setenv("AGENT_PROVIDER_FAST_MODEL", "spec082-big-b-6g")
	t.Setenv("AGENT_PROVIDER_VISION_MODEL", "spec082-big-b-6g")
	// Remaining gemma4:26b / deepseek-ocr:3b / nomic / all-MiniLM refs in
	// setRequiredEnv stay profiled and individually fit 20480.
	t.Setenv("ML_MODEL_MEMORY_PROFILES_JSON", spec082InteractiveProfiles)

	_, err := Load()
	if err == nil {
		t.Fatal("expected Load() to reject the over-subscribed interactive ollama set (24576 MiB > 20480 MiB resident) — if this passes, the SCOPE-082-02 concurrent-sum branch is missing and the contract is tautological")
	}
	msg := err.Error()
	for _, want := range []string{
		"FR-045-002",
		"concurrent ollama envelope exceeded",
		"24576",
		"OLLAMA_MEMORY_LIMIT",
		"spec082-big-a-18g",
		"spec082-big-b-6g",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should contain %q, got: %v", want, err)
		}
	}
}

// TestValidate_AcceptsFittingInteractiveOllamaSum — SCN-082-B02.
// A fitting interactive sum under a resident keep-alive MUST NOT
// false-positive (5120 MiB ≤ 8192 MiB).
func TestValidate_AcceptsFittingInteractiveOllamaSum(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OLLAMA_MEMORY_LIMIT", "8G")     // 8192 MiB
	t.Setenv("OLLAMA_KEEP_ALIVE", "-1")       // resident → sum guard active
	t.Setenv("OLLAMA_MAX_LOADED_MODELS", "2") // co-resident posture → sum guard active (spec 102)
	// Distinct interactive set = {small-x 1024, small-y 4096} = 5120 MiB.
	t.Setenv("LLM_MODEL", "spec082-small-x-1g")
	t.Setenv("OLLAMA_MODEL", "spec082-small-x-1g")
	t.Setenv("OLLAMA_VISION_MODEL", "spec082-small-x-1g")
	t.Setenv("AGENT_PROVIDER_DEFAULT_MODEL", "spec082-small-y-4g")
	t.Setenv("AGENT_PROVIDER_FAST_MODEL", "spec082-small-y-4g")
	t.Setenv("AGENT_PROVIDER_VISION_MODEL", "spec082-small-y-4g")
	t.Setenv("ML_MODEL_MEMORY_PROFILES_JSON", spec082InteractiveProfiles)

	if _, err := Load(); err != nil {
		t.Fatalf("expected Load() to ACCEPT a fitting interactive sum (5120 MiB ≤ 8192 MiB) — false positive: %v", err)
	}
}

// TestValidate_SumGuardRelaxedForNonResidentKeepAlive — proves the
// resident gate: the SAME over-subscribed set that fails under keep_alive
// "-1" is ACCEPTED under a short keep_alive ("5m") because ollama evicts
// between sporadic uses, so the models are not guaranteed co-resident.
func TestValidate_SumGuardRelaxedForNonResidentKeepAlive(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OLLAMA_MEMORY_LIMIT", "20G")
	t.Setenv("OLLAMA_KEEP_ALIVE", "5m")       // NON-resident → sum guard relaxed
	t.Setenv("OLLAMA_MAX_LOADED_MODELS", "2") // co-resident posture: isolates the keep-alive gate (spec 102)
	t.Setenv("LLM_MODEL", "spec082-big-a-18g")
	t.Setenv("OLLAMA_MODEL", "spec082-big-a-18g")
	t.Setenv("OLLAMA_VISION_MODEL", "spec082-big-a-18g")
	t.Setenv("AGENT_PROVIDER_DEFAULT_MODEL", "spec082-big-b-6g")
	t.Setenv("AGENT_PROVIDER_FAST_MODEL", "spec082-big-b-6g")
	t.Setenv("AGENT_PROVIDER_VISION_MODEL", "spec082-big-b-6g")
	t.Setenv("ML_MODEL_MEMORY_PROFILES_JSON", spec082InteractiveProfiles)

	if _, err := Load(); err != nil {
		t.Fatalf("expected Load() to ACCEPT the over-subscribed set under non-resident keep_alive 5m (per-model checks still pass, sum guard relaxed): %v", err)
	}
}
