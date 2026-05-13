// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"
)

// Spec 045 FR-045-002 — ML model memory envelope validation.
// These tests pin the contract that Validate() rejects:
//   1. Missing ML_MEMORY_LIMIT (named by requiredVars).
//   2. Configured Ollama models with no entry in the model_memory_profiles
//      map (named by validateMLModelEnvelope).
//   3. Configured Ollama models whose profile memory_mib exceeds the
//      ML deploy envelope (named by validateMLModelEnvelope).
//   4. Accepts the happy path where every used model fits.
//
// These tests are referenced by SCN-045-A02 (ML model envelope) in
// specs/045-deploy-resource-filesystem-hardening/scenario-manifest.json.

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

// TestValidate_RejectsOversizedMLModel asserts validateMLModelEnvelope
// rejects a configured model whose profile exceeds ML_MEMORY_LIMIT.
// SCN-045-A02 — fact-of-life: Sizing the envelope smaller than the
// largest configured model MUST fail-loud at startup, not at first
// inference.
func TestValidate_RejectsOversizedMLModel(t *testing.T) {
	setRequiredEnv(t)
	// Set ML envelope to 1G (1024 MiB) but configure a model whose
	// profile is 2048 MiB — must reject loudly.
	t.Setenv("ML_MEMORY_LIMIT", "1G")
	t.Setenv("OLLAMA_MODEL", "big-model")
	t.Setenv("LLM_MODEL", "big-model")
	t.Setenv("EMBEDDING_MODEL", "small-model")
	t.Setenv("ML_MODEL_MEMORY_PROFILES_JSON",
		`[{"model":"big-model","memory_mib":2048},{"model":"small-model","memory_mib":256}]`)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when configured model exceeds ML_MEMORY_LIMIT envelope")
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
}

// TestValidate_RejectsMissingModelProfileEntry asserts
// validateMLModelEnvelope rejects a configured model that has no profile
// entry. SCN-045-A02 — fact-of-life: Adding a new model to the runtime
// config without a memory profile is a setup defect that MUST fail-loud
// before the model is loaded into Ollama at runtime.
func TestValidate_RejectsMissingModelProfileEntry(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OLLAMA_MODEL", "unprofiled-model")
	t.Setenv("LLM_MODEL", "profiled-model")
	t.Setenv("EMBEDDING_MODEL", "profiled-embed")
	t.Setenv("ML_MODEL_MEMORY_PROFILES_JSON",
		`[{"model":"profiled-model","memory_mib":1024},{"model":"profiled-embed","memory_mib":256}]`)

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
		`[{"model":"fits-model","memory_mib":2048},{"model":"fits-embed","memory_mib":256}]`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should succeed when every model fits within envelope, got: %v", err)
	}
	if cfg.MLMemoryLimitMiB != 3072 {
		t.Errorf("MLMemoryLimitMiB: expected 3072 (3G), got %d", cfg.MLMemoryLimitMiB)
	}
	if cfg.MLModelMemoryProfiles["fits-model"] != 2048 {
		t.Errorf("MLModelMemoryProfiles[fits-model]: expected 2048, got %d",
			cfg.MLModelMemoryProfiles["fits-model"])
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
