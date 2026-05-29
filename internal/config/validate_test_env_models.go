package config

import (
	"fmt"
	"strings"
)

// productionLeakModelLiteral is the production-tier default model that
// MUST NOT appear in resolved test-environment config. Spec 061
// SCOPE-06a (finding BS-002-OPTION2-INCOMPLETE-MULTI-PATH-MODEL-LEAK):
// before SCOPE-06a, four of the five model env vars
// (LLM_MODEL, OLLAMA_MODEL, AGENT_PROVIDER_VISION_MODEL,
// OLLAMA_VISION_MODEL) silently fell back to this literal under
// SMACKEREL_ENV=test because environments.test in config/smackerel.yaml
// did not override them and `env_override_value` falls through to the
// base llm.model value. The retrieval-qa-v1 path then asked Ollama to
// run a model the test-tier Ollama daemon did not have, producing
// `OllamaException model not found` and blowing the 5s scenario budget.
//
// The validator below is defense-in-depth: even with the
// config/smackerel.yaml overrides in place (3A.1) and config.sh
// wiring complete (3A.2), if some future SST change drops a test-env
// override OR ships a fresh test bundle without one, the Go loader
// MUST refuse to start with a named, non-empty error pointing at the
// offending env var. This guarantees the multi-path leak cannot
// regress silently.
const productionLeakModelLiteral = "gemma3:4b"

// testEnvModelKeys lists the env vars that MUST be overridden under
// SMACKEREL_ENV=test (i.e. MUST NOT resolve to the production literal).
// Adding a new model env var that is also routed through the
// agent / synthesis / ollama paths should extend this slice.
func testEnvModelKeys() []struct {
	Name  string
	Value func(*Config) string
} {
	return []struct {
		Name  string
		Value func(*Config) string
	}{
		{"LLM_MODEL", func(c *Config) string { return c.LLMModel }},
		{"OLLAMA_MODEL", func(c *Config) string { return c.OllamaModel }},
		{"OLLAMA_VISION_MODEL", func(c *Config) string { return c.OllamaVisionModel }},
		{"AGENT_PROVIDER_DEFAULT_MODEL", func(c *Config) string { return c.AgentProviderDefaultModel }},
		{"AGENT_PROVIDER_VISION_MODEL", func(c *Config) string { return c.AgentProviderVisionModel }},
	}
}

// validateTestEnvModelOverrides enforces spec 061 SCOPE-06a — under
// SMACKEREL_ENV=test, every model env var in testEnvModelKeys() MUST
// have a non-empty value AND MUST NOT equal the production literal
// (productionLeakModelLiteral). Errors enumerate every offender in a
// single message so the operator sees the full picture in one boot.
func (c *Config) validateTestEnvModelOverrides() error {
	if c.Environment != "test" {
		return nil
	}
	var offenders []string
	for _, entry := range testEnvModelKeys() {
		value := strings.TrimSpace(entry.Value(c))
		if value == "" {
			offenders = append(offenders, fmt.Sprintf("%s (missing/empty)", entry.Name))
			continue
		}
		if value == productionLeakModelLiteral {
			offenders = append(offenders, fmt.Sprintf("%s resolves to production literal %q", entry.Name, productionLeakModelLiteral))
		}
	}
	if len(offenders) > 0 {
		return fmt.Errorf("spec 061 SCOPE-06a — SMACKEREL_ENV=test requires every model env var to be SST-overridden away from the production default; offenders: %s", strings.Join(offenders, ", "))
	}
	return nil
}
