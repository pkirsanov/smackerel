// Spec 067 Scope 1 — SCN-067-A08 (policy threshold sourced from SST).
//
// These unit tests prove LoadPolicyConfig fails loud when any required
// policy SST key is missing or malformed, and names the canonical SST
// key path in the error.

package config

import (
	"errors"
	"strings"
	"testing"
)

// resetPolicyEnv clears and restores the policy env vars for a single
// test case. Each subtest owns its own env shape; using t.Setenv would
// keep the previous test's values for keys the new case did not touch.
func resetPolicyEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"POLICY_SCENARIO_PROMPT_MAX_LINES",
		"POLICY_EXCEPTION_BASELINE_PATH",
		"POLICY_EXCEPTION_MAX_AGE_DAYS",
		"POLICY_INTENT_BYPASS_GUARD_ENABLED",
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
}

// TestPolicyConfigRequiresScenarioPromptMaxLines — SCN-067-A08.
// When the SST key policy.scenario_prompt_max_lines is missing, the
// loader fails loud with the canonical key path; there is no silent
// numeric fallback.
func TestPolicyConfigRequiresScenarioPromptMaxLines(t *testing.T) {
	resetPolicyEnv(t)
	// Set every other required key so only scenario_prompt_max_lines
	// is missing — adversarial isolation: if a fallback existed the
	// load would succeed and this test would not catch the regression.
	t.Setenv("POLICY_EXCEPTION_BASELINE_PATH", "policy-exception-baseline.json")
	t.Setenv("POLICY_EXCEPTION_MAX_AGE_DAYS", "90")
	t.Setenv("POLICY_INTENT_BYPASS_GUARD_ENABLED", "true")

	_, err := LoadPolicyConfig()
	if err == nil {
		t.Fatalf("LoadPolicyConfig() succeeded with empty POLICY_SCENARIO_PROMPT_MAX_LINES; expected fail-loud")
	}
	var pErr *PolicyConfigError
	if !errors.As(err, &pErr) {
		t.Fatalf("expected *PolicyConfigError, got %T: %v", err, err)
	}
	if pErr.Key != "policy.scenario_prompt_max_lines" {
		t.Fatalf("error names wrong SST key: got %q, want policy.scenario_prompt_max_lines", pErr.Key)
	}
	if !strings.Contains(err.Error(), "policy.scenario_prompt_max_lines") {
		t.Fatalf("error message does not include the canonical key path: %q", err.Error())
	}
}

// TestPolicyConfigRequiresAllPolicyKeys — SCN-067-A08 (broader).
// Every documented SST key is required; missing any one fails loud
// with that key's canonical path. Adversarial: each subtest sets all
// keys except the one under test.
func TestPolicyConfigRequiresAllPolicyKeys(t *testing.T) {
	cases := []struct {
		envToClear string
		wantKey    string
	}{
		{"POLICY_SCENARIO_PROMPT_MAX_LINES", "policy.scenario_prompt_max_lines"},
		{"POLICY_EXCEPTION_BASELINE_PATH", "policy.policy_exception_baseline_path"},
		{"POLICY_EXCEPTION_MAX_AGE_DAYS", "policy.policy_exception_max_age_days"},
		{"POLICY_INTENT_BYPASS_GUARD_ENABLED", "policy.intent_bypass_guard_enabled"},
	}
	for _, tc := range cases {
		t.Run(tc.envToClear, func(t *testing.T) {
			resetPolicyEnv(t)
			t.Setenv("POLICY_SCENARIO_PROMPT_MAX_LINES", "120")
			t.Setenv("POLICY_EXCEPTION_BASELINE_PATH", "policy-exception-baseline.json")
			t.Setenv("POLICY_EXCEPTION_MAX_AGE_DAYS", "90")
			t.Setenv("POLICY_INTENT_BYPASS_GUARD_ENABLED", "true")
			t.Setenv(tc.envToClear, "")

			_, err := LoadPolicyConfig()
			if err == nil {
				t.Fatalf("LoadPolicyConfig() succeeded with empty %s; expected fail-loud", tc.envToClear)
			}
			var pErr *PolicyConfigError
			if !errors.As(err, &pErr) {
				t.Fatalf("expected *PolicyConfigError, got %T: %v", err, err)
			}
			if pErr.Key != tc.wantKey {
				t.Fatalf("error names wrong key: got %q, want %q", pErr.Key, tc.wantKey)
			}
		})
	}
}

// TestPolicyConfigRejectsMalformedScenarioPromptMaxLines proves the
// numeric SST contract: non-integer or non-positive values fail loud
// with the same canonical key path.
func TestPolicyConfigRejectsMalformedScenarioPromptMaxLines(t *testing.T) {
	cases := []string{"abc", "0", "-1", "1.5"}
	for _, v := range cases {
		t.Run(v, func(t *testing.T) {
			resetPolicyEnv(t)
			t.Setenv("POLICY_SCENARIO_PROMPT_MAX_LINES", v)
			t.Setenv("POLICY_EXCEPTION_BASELINE_PATH", "policy-exception-baseline.json")
			t.Setenv("POLICY_EXCEPTION_MAX_AGE_DAYS", "90")
			t.Setenv("POLICY_INTENT_BYPASS_GUARD_ENABLED", "true")

			_, err := LoadPolicyConfig()
			if err == nil {
				t.Fatalf("LoadPolicyConfig() accepted %q; expected fail-loud", v)
			}
			var pErr *PolicyConfigError
			if !errors.As(err, &pErr) || pErr.Key != "policy.scenario_prompt_max_lines" {
				t.Fatalf("expected PolicyConfigError on policy.scenario_prompt_max_lines, got %v", err)
			}
		})
	}
}

// TestPolicyConfigLoadsWithAllKeysPresent is the positive baseline
// pinning the happy path so a regression that broke the loader would
// be caught immediately.
func TestPolicyConfigLoadsWithAllKeysPresent(t *testing.T) {
	resetPolicyEnv(t)
	t.Setenv("POLICY_SCENARIO_PROMPT_MAX_LINES", "120")
	t.Setenv("POLICY_EXCEPTION_BASELINE_PATH", "policy-exception-baseline.json")
	t.Setenv("POLICY_EXCEPTION_MAX_AGE_DAYS", "90")
	t.Setenv("POLICY_INTENT_BYPASS_GUARD_ENABLED", "false")

	cfg, err := LoadPolicyConfig()
	if err != nil {
		t.Fatalf("LoadPolicyConfig() unexpected error: %v", err)
	}
	if cfg.ScenarioPromptMaxLines != 120 {
		t.Fatalf("ScenarioPromptMaxLines = %d, want 120", cfg.ScenarioPromptMaxLines)
	}
	if cfg.ExceptionBaselinePath != "policy-exception-baseline.json" {
		t.Fatalf("ExceptionBaselinePath = %q", cfg.ExceptionBaselinePath)
	}
	if cfg.ExceptionMaxAgeDays != 90 {
		t.Fatalf("ExceptionMaxAgeDays = %d, want 90", cfg.ExceptionMaxAgeDays)
	}
	if cfg.IntentBypassGuardEnabled != false {
		t.Fatalf("IntentBypassGuardEnabled = %v, want false", cfg.IntentBypassGuardEnabled)
	}
}
