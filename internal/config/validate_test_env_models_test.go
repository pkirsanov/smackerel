package config

import (
	"strings"
	"testing"
)

// TestValidate_TestEnv_MissingModelOverride_FailsLoud_LLMModel — adversarial
// case for spec 061 SCOPE-06a. Under SMACKEREL_ENV=test, LLM_MODEL falling
// back to the production literal MUST produce a named, non-empty error.
func TestValidate_TestEnv_MissingModelOverride_FailsLoud_LLMModel(t *testing.T) {
	assertTestEnvModelOverrideFailsLoud(t, "LLM_MODEL")
}

func TestValidate_TestEnv_MissingModelOverride_FailsLoud_OllamaModel(t *testing.T) {
	assertTestEnvModelOverrideFailsLoud(t, "OLLAMA_MODEL")
}

func TestValidate_TestEnv_MissingModelOverride_FailsLoud_OllamaVisionModel(t *testing.T) {
	assertTestEnvModelOverrideFailsLoud(t, "OLLAMA_VISION_MODEL")
}

func TestValidate_TestEnv_MissingModelOverride_FailsLoud_AgentProviderDefaultModel(t *testing.T) {
	assertTestEnvModelOverrideFailsLoud(t, "AGENT_PROVIDER_DEFAULT_MODEL")
}

func TestValidate_TestEnv_MissingModelOverride_FailsLoud_AgentProviderVisionModel(t *testing.T) {
	assertTestEnvModelOverrideFailsLoud(t, "AGENT_PROVIDER_VISION_MODEL")
}

// assertTestEnvModelOverrideFailsLoud — sets the entire required env
// envelope to known-good test values, then overrides the named model
// env var to the production literal (`gemma3:4b`). The validator MUST
// reject Load() with an error that names the offending key.
func assertTestEnvModelOverrideFailsLoud(t *testing.T, key string) {
	t.Helper()
	setRequiredEnv(t)
	// setRequiredEnv pins LLM_PROVIDER=openai which makes the Ollama
	// vars optional; force ollama so OLLAMA_MODEL participates in the
	// requiredVars() check too. LLM_API_KEY is then unused but harmless.
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv(key, productionLeakModelLiteral)

	_, err := Load()
	if err == nil {
		t.Fatalf("expected Load() to fail when SMACKEREL_ENV=test and %s=%q", key, productionLeakModelLiteral)
	}
	if !strings.Contains(err.Error(), key) {
		t.Errorf("error must name offending key %s; got: %v", key, err)
	}
	if !strings.Contains(err.Error(), "SCOPE-06a") {
		t.Errorf("error must reference spec 061 SCOPE-06a contract; got: %v", err)
	}
}

// TestValidate_TestEnv_NoProductionModelLeak — adversarial sweep.
// With SMACKEREL_ENV=test, none of the five tracked model env vars
// may equal the production literal. Sets every key to the production
// literal at once and asserts every offender appears in the single
// consolidated error message.
func TestValidate_TestEnv_NoProductionModelLeak(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("LLM_PROVIDER", "ollama")
	keys := []string{
		"LLM_MODEL",
		"OLLAMA_MODEL",
		"OLLAMA_VISION_MODEL",
		"AGENT_PROVIDER_DEFAULT_MODEL",
		"AGENT_PROVIDER_VISION_MODEL",
	}
	for _, k := range keys {
		t.Setenv(k, productionLeakModelLiteral)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected Load() to fail when every test-env model env var leaks the production literal")
	}
	for _, k := range keys {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error must name every leaked key; missing %s; got: %v", k, err)
		}
	}
}

// TestValidate_TestEnv_ModelOverrides_HappyPath — sanity: when every
// model env var is set to a non-production value, Load() succeeds.
// Pins LLM_PROVIDER=ollama (the stricter path) to keep the assertion
// honest.
func TestValidate_TestEnv_ModelOverrides_HappyPath(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("LLM_PROVIDER", "ollama")
	const testModel = "qwen2.5:0.5b-instruct"
	for _, k := range []string{
		"LLM_MODEL",
		"OLLAMA_MODEL",
		"OLLAMA_VISION_MODEL",
		"AGENT_PROVIDER_DEFAULT_MODEL",
		"AGENT_PROVIDER_VISION_MODEL",
	} {
		t.Setenv(k, testModel)
	}
	// Extend the memory profile catalog so the qwen test model AND
	// every default-fixture model from setRequiredEnv (gemma4:26b,
	// all-MiniLM-L6-v2, nomic-embed-text, deepseek-ocr:3b, llama3.2)
	// have an entry; otherwise validateModelEnvelopes rejects with a
	// "missing memory profile" error unrelated to this test.
	t.Setenv("ML_MODEL_MEMORY_PROFILES_JSON", `[{"model":"qwen2.5:0.5b-instruct","memory_mib":512},{"model":"llama3.2","memory_mib":2048},{"model":"all-MiniLM-L6-v2","memory_mib":256},{"model":"gpt-4o-mini","memory_mib":512},{"model":"gemma4:26b","memory_mib":3072},{"model":"nomic-embed-text","memory_mib":256},{"model":"deepseek-ocr:3b","memory_mib":2560}]`)

	if _, err := Load(); err != nil {
		t.Fatalf("expected Load() to succeed with full test-env model overrides; got: %v", err)
	}
}
