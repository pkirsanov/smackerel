// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"
)

// Spec 102 SCOPE-102-03 — KV-aware model-envelope validator tests.
//
// These pin the TRUTHFUL envelope math that replaces the prior static
// memory_mib ceiling:
//
//	resident = weights_mib + kv_mib_per_1k_ctx × (num_ctx/1000) × num_parallel
//
//	SCN-102-C3-02  the validator FAILS LOUD on a profile whose weights fit
//	               but whose weights+KV exceed the envelope; ADVERSARIAL:
//	               removing the KV term (kv 0) makes the SAME weights fit,
//	               proving the KV term — not a static ceiling — has bite.
//	SCN-102-C3-03  an uncapped-context config is refused before it can fail
//	               to load; the SST-capped num_ctx that fits is accepted.
//	SCN-102-C3-04  the co-resident SUM check keys off max_loaded_models:
//	               ==1 (on-demand swap) does NOT require the sum; >1
//	               (co-resident) requires the working set to fit.

// TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102 (ADVERSARIAL).
func TestValidateModelEnvelopes_KVMathFailsUnderstated_Spec102(t *testing.T) {
	// Envelope 8192 MiB. A model whose WEIGHTS (6144) fit alone, but whose
	// weights + KV (@ num_ctx 8192, num_parallel 4) exceed it:
	//   KV = 64 × 8.192 × 4 = 2097 MiB → resident = 6144 + 2097 = 8241 > 8192.
	mkCfg := func(kvPer1k int) *Config {
		return &Config{
			MLModelMemoryProfiles: map[string]ModelMemoryProfile{
				"kv-heavy": {WeightsMiB: 6144, KVMiBPer1kCtx: kvPer1k, NumCtx: 8192},
			},
			OllamaNumParallel:     4,
			OllamaMaxLoadedModels: 1,
			OllamaMemoryLimit:     "8G",
			OllamaMemoryLimitMiB:  8192,
			LLMModel:              "kv-heavy",
		}
	}

	// With the KV term: over-envelope → fail loud naming model + envelope + KV.
	err := mkCfg(64).validateModelEnvelopes()
	if err == nil {
		t.Fatal("SCN-102-C3-02: expected fail-loud — weights (6144) fit alone but weights+KV (8241) exceed the 8192 MiB envelope; the validator MUST use real KV math, not a static ceiling")
	}
	for _, want := range []string{"kv-heavy", "OLLAMA_MEMORY_LIMIT", "envelope exceeded", "KV", "num_ctx=8192"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("SCN-102-C3-02: error should contain %q, got: %v", want, err)
		}
	}

	// ADVERSARIAL: remove the KV term (kv 0) → resident = weights = 6144 < 8192
	// → accept. This proves the KV term is EXACTLY what triggers the rejection;
	// a regression to the static-ceiling (weights-only) validator would let the
	// over-envelope config through here, and the assertion above would then be
	// the ONLY thing standing between the operator and a silent OOM at load.
	if err := mkCfg(0).validateModelEnvelopes(); err != nil {
		t.Fatalf("SCN-102-C3-02 adversarial: with the KV term removed (kv_mib_per_1k_ctx=0) the same 6144 MiB weights MUST fit the 8192 MiB envelope — the fact that adding the KV term flips accept→reject is the proof the math has bite; got: %v", err)
	}
}

// TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102 (SCN-102-C3-03).
func TestValidateModelEnvelopes_RefusesUncappedAcceptsCapped_Spec102(t *testing.T) {
	// gemma4:26b — weights 16384, KV 256/1k, num_parallel 4, envelope 48G.
	mkCfg := func(numCtx int) *Config {
		return &Config{
			MLModelMemoryProfiles: map[string]ModelMemoryProfile{
				"gemma4:26b": {WeightsMiB: 16384, KVMiBPer1kCtx: 256, NumCtx: numCtx},
			},
			OllamaNumParallel:     4,
			OllamaMaxLoadedModels: 1,
			OllamaMemoryLimit:     "48G",
			OllamaMemoryLimitMiB:  49152,
			LLMModel:              "gemma4:26b",
		}
	}

	// Uncapped arch context (262144): KV = 256 × 262.144 × 4 ≈ 268 GiB →
	// resident explodes → refused BEFORE it can fail to load on the host.
	err := mkCfg(262144).validateModelEnvelopes()
	if err == nil {
		t.Fatal("SCN-102-C3-03: an uncapped gemma4:26b context (262144) MUST be refused fail-loud at config generation, not left to OOM at first inference on the host")
	}
	if !strings.Contains(err.Error(), "gemma4:26b") || !strings.Contains(err.Error(), "envelope exceeded") {
		t.Errorf("SCN-102-C3-03: error should name gemma4:26b + 'envelope exceeded', got: %v", err)
	}

	// SST-capped 8192: KV = 256 × 8.192 × 4 = 8388 MiB → resident = 16384 +
	// 8388 = 24772 MiB < 49152 → accepted (the exact host-fitting cap that the
	// retired `ollama create num_ctx 8192` host hack used to bake, now SST).
	if err := mkCfg(8192).validateModelEnvelopes(); err != nil {
		t.Fatalf("SCN-102-C3-03: the SST-capped num_ctx=8192 that fits the host envelope MUST be accepted, got: %v", err)
	}
}

// TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102 (SCN-102-C3-04).
func TestValidateModelEnvelopes_CoResidentSumGatedByMaxLoaded_Spec102(t *testing.T) {
	// gemma4:26b (18432) + deepseek-r1:32b (22528) each fit the 28G envelope
	// ALONE, but sum to 40960 MiB > 28672 co-resident.
	mkCfg := func(maxLoaded int) *Config {
		c := &Config{
			MLModelMemoryProfiles: staticMMPs(map[string]int{
				"gemma4:26b":      18432,
				"deepseek-r1:32b": 22528,
			}),
			OllamaNumParallel:     4,
			OllamaMaxLoadedModels: maxLoaded,
			OllamaMemoryLimit:     "28G",
			OllamaMemoryLimitMiB:  28672,
			OllamaKeepAlive:       "-1",
		}
		c.Assistant.OpenKnowledge.Enabled = true
		c.Assistant.OpenKnowledge.LLMModelID = "gemma4:26b"            // gather
		c.Assistant.OpenKnowledge.SynthesisModelID = "deepseek-r1:32b" // standing default
		return c
	}

	// max_loaded_models == 1 (on-demand SWAP): each model need only fit ALONE;
	// the co-resident sum is NOT required. Both fit 28672 individually → accept.
	if err := mkCfg(1).validateModelEnvelopes(); err != nil {
		t.Fatalf("SCN-102-C3-04: under on-demand swap (max_loaded_models=1) the co-resident SUM must NOT be required — each model fits alone; got: %v", err)
	}

	// max_loaded_models > 1 (CO-RESIDENT): the gather + standing-default
	// working set (40960) must fit 28672 → refused fail-loud.
	err := mkCfg(2).validateModelEnvelopes()
	if err == nil {
		t.Fatal("SCN-102-C3-04: under co-resident posture (max_loaded_models=2) the working-set SUM (40960 > 28672) MUST be refused")
	}
	if !strings.Contains(err.Error(), "synthesis_model_id") || !strings.Contains(err.Error(), "envelope exceeded") {
		t.Errorf("SCN-102-C3-04: co-resident rejection should name synthesis_model_id + 'envelope exceeded', got: %v", err)
	}
}

// TestModelMemoryProfile_ResidentMiB_Spec102 unit-pins the resident math and
// its num_parallel clamp (a mis-set num_parallel can never understate).
func TestModelMemoryProfile_ResidentMiB_Spec102(t *testing.T) {
	p := ModelMemoryProfile{WeightsMiB: 18432, KVMiBPer1kCtx: 102, NumCtx: 32768}
	// num_parallel 4: 18432 + 102×32768×4/1000 = 18432 + 13369 = 31801.
	if got := p.ResidentMiB(4); got != 31801 {
		t.Errorf("ResidentMiB(4) = %d, want 31801", got)
	}
	// num_parallel 1: 18432 + 102×32768×1/1000 = 18432 + 3342 = 21774.
	if got := p.ResidentMiB(1); got != 21774 {
		t.Errorf("ResidentMiB(1) = %d, want 21774", got)
	}
	// num_parallel 0 clamps to 1 (never understate).
	if got, want := p.ResidentMiB(0), p.ResidentMiB(1); got != want {
		t.Errorf("ResidentMiB(0) = %d, want clamp-to-1 %d", got, want)
	}
	// A pure embedding profile (kv 0) resolves to weights regardless of parallel.
	e := ModelMemoryProfile{WeightsMiB: 768, KVMiBPer1kCtx: 0, NumCtx: 2048}
	if got := e.ResidentMiB(8); got != 768 {
		t.Errorf("embedding ResidentMiB(8) = %d, want 768 (kv 0)", got)
	}
}
