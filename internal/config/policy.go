// Spec 067 — Intent-Driven Policy Enforcement (CI guards).
//
// PolicyConfig holds the SST-sourced thresholds required by the spec 067
// policy guard suite. Every field is REQUIRED at startup/guard bootstrap;
// LoadPolicyConfig fails loud naming the missing key (G067-A08). There is
// no silent fallback for any field — this is the canonical fail-loud SST
// contract for the policy surface.

package config

import (
	"fmt"
	"os"
	"strconv"
)

// PolicyConfig carries the SST-required policy thresholds for spec 067
// guards. See config/smackerel.yaml under `policy:`.
type PolicyConfig struct {
	// ScenarioPromptMaxLines caps non-blank lines in scenario YAML
	// `system_prompt` blocks. Sourced from policy.scenario_prompt_max_lines.
	ScenarioPromptMaxLines int

	// ExceptionBaselinePath is the repo-relative path to the
	// committed policy-exception baseline file. Sourced from
	// policy.policy_exception_baseline_path.
	ExceptionBaselinePath string

	// ExceptionMaxAgeDays is the upper bound on `expires_on` distance
	// from now for any accepted policy exception. Sourced from
	// policy.policy_exception_max_age_days.
	ExceptionMaxAgeDays int

	// IntentBypassGuardEnabled wires the spec 068 compiler-bypass
	// guard through the spec 067 policy surface. Sourced from
	// policy.intent_bypass_guard_enabled.
	IntentBypassGuardEnabled bool
}

// PolicyConfigError is returned by LoadPolicyConfig when an SST key is
// missing or malformed. The Key field carries the canonical SST key path
// (e.g., "policy.scenario_prompt_max_lines") so test output and CI logs
// can name the missing key without scraping prose.
type PolicyConfigError struct {
	Key    string
	Reason string
}

func (e *PolicyConfigError) Error() string {
	return fmt.Sprintf("required policy configuration missing or invalid: %s (%s)", e.Key, e.Reason)
}

// LoadPolicyConfig reads the spec 067 policy thresholds from the
// process environment (the config-generated env file). It fails loud
// on any missing or malformed value with the canonical SST key path.
func LoadPolicyConfig() (*PolicyConfig, error) {
	maxLinesRaw := os.Getenv("POLICY_SCENARIO_PROMPT_MAX_LINES")
	if maxLinesRaw == "" {
		return nil, &PolicyConfigError{Key: "policy.scenario_prompt_max_lines", Reason: "env POLICY_SCENARIO_PROMPT_MAX_LINES is empty"}
	}
	maxLines, err := strconv.Atoi(maxLinesRaw)
	if err != nil || maxLines <= 0 {
		return nil, &PolicyConfigError{Key: "policy.scenario_prompt_max_lines", Reason: fmt.Sprintf("must be a positive integer, got %q", maxLinesRaw)}
	}

	baselinePath := os.Getenv("POLICY_EXCEPTION_BASELINE_PATH")
	if baselinePath == "" {
		return nil, &PolicyConfigError{Key: "policy.policy_exception_baseline_path", Reason: "env POLICY_EXCEPTION_BASELINE_PATH is empty"}
	}

	maxAgeRaw := os.Getenv("POLICY_EXCEPTION_MAX_AGE_DAYS")
	if maxAgeRaw == "" {
		return nil, &PolicyConfigError{Key: "policy.policy_exception_max_age_days", Reason: "env POLICY_EXCEPTION_MAX_AGE_DAYS is empty"}
	}
	maxAge, err := strconv.Atoi(maxAgeRaw)
	if err != nil || maxAge <= 0 {
		return nil, &PolicyConfigError{Key: "policy.policy_exception_max_age_days", Reason: fmt.Sprintf("must be a positive integer, got %q", maxAgeRaw)}
	}

	bypassRaw := os.Getenv("POLICY_INTENT_BYPASS_GUARD_ENABLED")
	if bypassRaw == "" {
		return nil, &PolicyConfigError{Key: "policy.intent_bypass_guard_enabled", Reason: "env POLICY_INTENT_BYPASS_GUARD_ENABLED is empty"}
	}
	var bypass bool
	switch bypassRaw {
	case "true":
		bypass = true
	case "false":
		bypass = false
	default:
		return nil, &PolicyConfigError{Key: "policy.intent_bypass_guard_enabled", Reason: fmt.Sprintf("must be \"true\" or \"false\", got %q", bypassRaw)}
	}

	return &PolicyConfig{
		ScenarioPromptMaxLines:   maxLines,
		ExceptionBaselinePath:    baselinePath,
		ExceptionMaxAgeDays:      maxAge,
		IntentBypassGuardEnabled: bypass,
	}, nil
}
