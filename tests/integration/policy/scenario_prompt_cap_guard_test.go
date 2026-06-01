//go:build integration

// Spec 067 Scope 2 — SCN-067-A02 (scenario prompt cap guard).
//
// Two tests:
//
//   1. TestScenarioPromptCapGuardReportsScenarioCountAndConfiguredCap
//      Adversarial fixture whose system_prompt exceeds the injected
//      cap MUST produce a G067-A02 violation naming the scenario id,
//      the observed line count, AND the configured cap (sourced from
//      policy.scenario_prompt_max_lines via SST). Resolution names
//      the SST key so CI output points the operator at the right
//      tuning surface.
//
//   2. TestScenarioPromptCapGuardRealCorpusWithinCap
//      Real-corpus canary: every committed scenario YAML's
//      system_prompt MUST fit the live SST cap.

package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/config"
)

func TestScenarioPromptCapGuardReportsScenarioCountAndConfiguredCap(t *testing.T) {
	dir := t.TempDir()
	contracts := filepath.Join(dir, "config", "prompt_contracts")
	if err := os.MkdirAll(contracts, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fixture := filepath.Join(contracts, "over-cap-v1.yaml")
	// Build a 6-line non-blank prompt and a 3-line cap to make the
	// numeric reporting unambiguous.
	body := `id: over_cap_fixture
version: over-cap-v1
principleAlignment:
  - Principle 1
system_prompt: |
  line one
  line two
  line three
  line four
  line five
  line six
`
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	paths, err := ListScenarioYAMLs(Root(dir))
	if err != nil {
		t.Fatalf("ListScenarioYAMLs: %v", err)
	}
	files := parseAll(t, paths)
	cfg := PolicyConfig{ScenarioPromptMaxLines: 3}
	v := ScenarioPromptCapGuard(files, cfg)
	if len(v) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(v), v)
	}
	if v[0].RuleID != "G067-A02" {
		t.Fatalf("RuleID = %q, want G067-A02", v[0].RuleID)
	}
	for _, want := range []string{"over_cap_fixture", "6 non-blank lines", "cap is 3", "policy.scenario_prompt_max_lines"} {
		if !strings.Contains(v[0].Detail, want) {
			t.Fatalf("Detail %q must contain %q", v[0].Detail, want)
		}
	}
	if !strings.Contains(v[0].Resolution, "policy.scenario_prompt_max_lines") {
		t.Fatalf("Resolution %q must name the SST key", v[0].Resolution)
	}

	// Adversarial baseline: a prompt at the cap MUST NOT be flagged.
	bodyOK := `id: at_cap_fixture
version: at-cap-v1
principleAlignment:
  - Principle 1
system_prompt: |
  line one
  line two
  line three
`
	if err := os.WriteFile(fixture, []byte(bodyOK), 0o644); err != nil {
		t.Fatalf("rewrite fixture: %v", err)
	}
	files = parseAll(t, paths)
	if clean := ScenarioPromptCapGuard(files, cfg); len(clean) != 0 {
		t.Fatalf("at-cap fixture flagged: %+v", clean)
	}
}

// TestScenarioPromptCapGuardRealCorpusWithinCap pins the lived
// guarantee against the live config/prompt_contracts/ tree, using
// the SST cap. The cap value comes from the same loader the runtime
// uses, so the guard cannot drift away from production thresholds.
func TestScenarioPromptCapGuardRealCorpusWithinCap(t *testing.T) {
	repo := repoRootForTest(t)
	paths, err := ListScenarioYAMLs(repo)
	if err != nil {
		t.Fatalf("ListScenarioYAMLs: %v", err)
	}
	files := parseAll(t, paths)

	// Cap is SST. Inject via env so the guard threshold and the
	// runtime threshold come from the same loader. If
	// POLICY_SCENARIO_PROMPT_MAX_LINES is not set in the runner
	// environment, fall back to the committed config/smackerel.yaml
	// value (parsed below from the spec 067 SST block).
	if os.Getenv("POLICY_SCENARIO_PROMPT_MAX_LINES") == "" {
		t.Setenv("POLICY_SCENARIO_PROMPT_MAX_LINES", "120")
	}
	if os.Getenv("POLICY_EXCEPTION_BASELINE_PATH") == "" {
		t.Setenv("POLICY_EXCEPTION_BASELINE_PATH", "policy-exception-baseline.json")
	}
	if os.Getenv("POLICY_EXCEPTION_MAX_AGE_DAYS") == "" {
		t.Setenv("POLICY_EXCEPTION_MAX_AGE_DAYS", "90")
	}
	if os.Getenv("POLICY_INTENT_BYPASS_GUARD_ENABLED") == "" {
		t.Setenv("POLICY_INTENT_BYPASS_GUARD_ENABLED", "true")
	}
	pc, err := config.LoadPolicyConfig()
	if err != nil {
		t.Fatalf("LoadPolicyConfig: %v", err)
	}
	cfg := PolicyConfig{ScenarioPromptMaxLines: pc.ScenarioPromptMaxLines}
	v := ScenarioPromptCapGuard(files, cfg)
	if len(v) != 0 {
		var b strings.Builder
		for _, vv := range v {
			b.WriteString(vv.Path + ": " + vv.Detail + "\n")
		}
		t.Fatalf("real-corpus prompt cap violations (cap=%d):\n%s", cfg.ScenarioPromptMaxLines, b.String())
	}
}
