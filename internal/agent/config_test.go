package agent

import (
	"os"
	"strings"
	"testing"
)

// allAgentEnvKeys lists every required AGENT_* variable so adversarial tests
// can wipe the environment to a known clean state and toggle individual keys.
// Keep this list in lockstep with LoadConfig in config.go.
var allAgentEnvKeys = []string{
	"AGENT_SCENARIO_DIR",
	"AGENT_SCENARIO_GLOB",
	"AGENT_HOT_RELOAD",
	"AGENT_ROUTING_CONFIDENCE_FLOOR",
	"AGENT_ROUTING_CONSIDER_TOP_N",
	"AGENT_ROUTING_FALLBACK_SCENARIO_ID",
	"AGENT_ROUTING_EMBEDDING_MODEL",
	"AGENT_TRACE_RETENTION_DAYS",
	"AGENT_TRACE_RECORD_LLM_MESSAGES",
	"AGENT_TRACE_REDACT_MARKER",
	"AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING",
	"AGENT_DEFAULTS_TIMEOUT_MS_CEILING",
	"AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING",
	"AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING",
	"AGENT_PROVIDER_DEFAULT_PROVIDER",
	"AGENT_PROVIDER_DEFAULT_MODEL",
	"AGENT_PROVIDER_REASONING_PROVIDER",
	"AGENT_PROVIDER_REASONING_MODEL",
	"AGENT_PROVIDER_FAST_PROVIDER",
	"AGENT_PROVIDER_FAST_MODEL",
	"AGENT_PROVIDER_VISION_PROVIDER",
	"AGENT_PROVIDER_VISION_MODEL",
	"AGENT_PROVIDER_OCR_PROVIDER",
	"AGENT_PROVIDER_OCR_MODEL",
}

// validEnv returns a map representing a complete, valid AGENT_* environment.
// Tests mutate or delete entries to exercise adversarial paths.
func validEnv() map[string]string {
	return map[string]string{
		"AGENT_SCENARIO_DIR":                         "config/prompt_contracts",
		"AGENT_SCENARIO_GLOB":                        "*.yaml",
		"AGENT_HOT_RELOAD":                           "true",
		"AGENT_ROUTING_CONFIDENCE_FLOOR":             "0.65",
		"AGENT_ROUTING_CONSIDER_TOP_N":               "5",
		"AGENT_ROUTING_FALLBACK_SCENARIO_ID":         "",
		"AGENT_ROUTING_EMBEDDING_MODEL":              "",
		"AGENT_TRACE_RETENTION_DAYS":                 "30",
		"AGENT_TRACE_RECORD_LLM_MESSAGES":            "false",
		"AGENT_TRACE_REDACT_MARKER":                  "***",
		"AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING": "32",
		"AGENT_DEFAULTS_TIMEOUT_MS_CEILING":          "120000",
		"AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING": "5",
		"AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING": "30000",
		"AGENT_PROVIDER_DEFAULT_PROVIDER":            "ollama",
		"AGENT_PROVIDER_DEFAULT_MODEL":               "gemma4:26b",
		"AGENT_PROVIDER_REASONING_PROVIDER":          "ollama",
		"AGENT_PROVIDER_REASONING_MODEL":             "deepseek-r1:32b",
		"AGENT_PROVIDER_FAST_PROVIDER":               "ollama",
		"AGENT_PROVIDER_FAST_MODEL":                  "gpt-oss:20b",
		"AGENT_PROVIDER_VISION_PROVIDER":             "ollama",
		"AGENT_PROVIDER_VISION_MODEL":                "gemma4:26b",
		"AGENT_PROVIDER_OCR_PROVIDER":                "ollama",
		"AGENT_PROVIDER_OCR_MODEL":                   "deepseek-ocr:3b",
	}
}

// applyEnv installs env (clearing every other AGENT_* var first) for the
// duration of the test. It uses t.Setenv so cleanup is automatic.
func applyEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, k := range allAgentEnvKeys {
		t.Setenv(k, "")
		// t.Setenv also covers the unset case via the post-test reset; we then
		// explicitly delete to distinguish "absent" from "empty" below.
		os.Unsetenv(k)
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestLoadConfig_HappyPath(t *testing.T) {
	applyEnv(t, validEnv())

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error on full valid env: %v", err)
	}
	if cfg.ScenarioDir != "config/prompt_contracts" {
		t.Errorf("ScenarioDir = %q, want %q", cfg.ScenarioDir, "config/prompt_contracts")
	}
	if !cfg.HotReload {
		t.Errorf("HotReload = false, want true")
	}
	if cfg.Routing.ConfidenceFloor != 0.65 {
		t.Errorf("ConfidenceFloor = %g, want 0.65", cfg.Routing.ConfidenceFloor)
	}
	if cfg.Routing.FallbackScenarioID != "" {
		t.Errorf("FallbackScenarioID = %q, want empty", cfg.Routing.FallbackScenarioID)
	}
	if got := cfg.ProviderRouting["reasoning"]; got.Provider != "ollama" || got.Model != "deepseek-r1:32b" {
		t.Errorf("ProviderRouting[reasoning] = %+v, want {ollama deepseek-r1:32b}", got)
	}
	if cfg.Defaults.TimeoutMs != 120000 {
		t.Errorf("Defaults.TimeoutMs = %d, want 120000", cfg.Defaults.TimeoutMs)
	}
}

// Adversarial regression: missing-config → fail-loud. Each required var is
// removed in turn and LoadConfig MUST surface that variable in the error.
// If a future change silently substitutes a default, this test fails.
func TestLoadConfig_MissingRequiredEnv_FailsLoud(t *testing.T) {
	for _, key := range allAgentEnvKeys {
		// Empty-string is explicitly allowed for these two — the missing case
		// (env var entirely absent) is still fatal and exercised below.
		key := key
		t.Run("missing/"+key, func(t *testing.T) {
			env := validEnv()
			delete(env, key)
			applyEnv(t, env)

			_, err := LoadConfig()
			if err == nil {
				t.Fatalf("LoadConfig succeeded with %s removed; expected fail-loud", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("LoadConfig error %q does not name missing var %q", err.Error(), key)
			}
		})
	}
}

// Adversarial regression: partial-config → fail-loud. Wiping the entire
// environment must produce an error that enumerates every missing AGENT_*
// var rather than booting with a partial Config.
func TestLoadConfig_EmptyEnv_FailsLoudOnAllRequired(t *testing.T) {
	applyEnv(t, map[string]string{})

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig succeeded with empty env; expected fail-loud")
	}
	for _, key := range allAgentEnvKeys {
		// The two empty-allowed keys are reported as missing when the env var
		// is entirely absent (LookupEnv returns ok=false). All others are
		// reported as missing when empty too.
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error does not enumerate missing var %s\nerror: %s", key, err.Error())
		}
	}
}

// Adversarial regression: empty value (other than the two opt-outs) is
// indistinguishable from missing and MUST be fatal. This guards against a
// drift where ./smackerel.sh config generate emits AGENT_X= for a field
// where the YAML value was deleted.
func TestLoadConfig_EmptyValue_FailsLoud(t *testing.T) {
	requiredNonEmpty := []string{
		"AGENT_SCENARIO_DIR",
		"AGENT_SCENARIO_GLOB",
		"AGENT_HOT_RELOAD",
		"AGENT_ROUTING_CONFIDENCE_FLOOR",
		"AGENT_ROUTING_CONSIDER_TOP_N",
		"AGENT_TRACE_RETENTION_DAYS",
		"AGENT_TRACE_RECORD_LLM_MESSAGES",
		"AGENT_TRACE_REDACT_MARKER",
		"AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING",
		"AGENT_DEFAULTS_TIMEOUT_MS_CEILING",
		"AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING",
		"AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING",
		"AGENT_PROVIDER_DEFAULT_PROVIDER",
		"AGENT_PROVIDER_DEFAULT_MODEL",
		"AGENT_PROVIDER_REASONING_PROVIDER",
		"AGENT_PROVIDER_REASONING_MODEL",
		"AGENT_PROVIDER_FAST_PROVIDER",
		"AGENT_PROVIDER_FAST_MODEL",
		"AGENT_PROVIDER_VISION_PROVIDER",
		"AGENT_PROVIDER_VISION_MODEL",
		"AGENT_PROVIDER_OCR_PROVIDER",
		"AGENT_PROVIDER_OCR_MODEL",
	}
	for _, key := range requiredNonEmpty {
		key := key
		t.Run("empty/"+key, func(t *testing.T) {
			env := validEnv()
			env[key] = ""
			applyEnv(t, env)

			_, err := LoadConfig()
			if err == nil {
				t.Fatalf("LoadConfig succeeded with %s set to empty string; expected fail-loud", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error does not name empty var %s; error=%s", key, err.Error())
			}
		})
	}
}

// Adversarial regression: malformed numeric values must produce a structured
// error naming the field and the offending value rather than silently
// coercing to zero.
func TestLoadConfig_MalformedNumeric_FailsLoud(t *testing.T) {
	cases := []struct{ key, value, wantSubstr string }{
		{"AGENT_ROUTING_CONFIDENCE_FLOOR", "not-a-float", "AGENT_ROUTING_CONFIDENCE_FLOOR"},
		{"AGENT_ROUTING_CONFIDENCE_FLOOR", "1.5", "[0, 1]"},
		{"AGENT_ROUTING_CONSIDER_TOP_N", "0", ">= 1"},
		{"AGENT_DEFAULTS_TIMEOUT_MS_CEILING", "abc", "AGENT_DEFAULTS_TIMEOUT_MS_CEILING"},
		{"AGENT_HOT_RELOAD", "yes", "true or false"},
		{"AGENT_TRACE_RECORD_LLM_MESSAGES", "1", "true or false"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			env := validEnv()
			env[tc.key] = tc.value
			applyEnv(t, env)

			_, err := LoadConfig()
			if err == nil {
				t.Fatalf("LoadConfig accepted malformed %s=%q", tc.key, tc.value)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error %q missing substring %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

// Empty-string is explicitly allowed for these two opt-out fields. Removing
// them entirely (env var absent) MUST still be fatal — that case is covered
// above; here we prove the empty-but-present case is accepted.
func TestLoadConfig_OptionalEmptyOptOuts_Accepted(t *testing.T) {
	env := validEnv()
	env["AGENT_ROUTING_FALLBACK_SCENARIO_ID"] = ""
	env["AGENT_ROUTING_EMBEDDING_MODEL"] = ""
	applyEnv(t, env)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig rejected empty opt-out values: %v", err)
	}
	if cfg.Routing.FallbackScenarioID != "" || cfg.Routing.EmbeddingModel != "" {
		t.Errorf("opt-out fields should be empty, got fallback=%q embedding=%q",
			cfg.Routing.FallbackScenarioID, cfg.Routing.EmbeddingModel)
	}
}
