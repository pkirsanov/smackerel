//go:build e2e

// Spec 067 Scope 2 — SCN-067-A01 / SCN-067-A02 e2e regression.
//
// TestIntentPolicyGuardE2E_ScenarioYamlFailuresAreActionable pins
// the CI-output contract for scenario-YAML guard failures: both the
// JSON and the plain-text rendering MUST name the offending
// scenario YAML path, the rule ID, the policy source, AND (for
// G067-A02) the configured cap and observed count. Without this
// e2e, the integration-layer tests would prove only that the
// scanner detects violations — not that the report rendering
// downstream consumers see actually includes the actionable
// information.

package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	policyfoundation "github.com/smackerel/smackerel/tests/integration/policy"
)

func TestIntentPolicyGuardE2E_ScenarioYamlFailuresAreActionable(t *testing.T) {
	dir := t.TempDir()
	contracts := filepath.Join(dir, "config", "prompt_contracts")
	if err := os.MkdirAll(contracts, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	missing := filepath.Join(contracts, "missing-align-v1.yaml")
	if err := os.WriteFile(missing, []byte(`id: missing_align
version: missing-align-v1
system_prompt: |
  short
`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	overCap := filepath.Join(contracts, "over-cap-v1.yaml")
	if err := os.WriteFile(overCap, []byte(`id: over_cap
version: over-cap-v1
principleAlignment:
  - Principle 1
system_prompt: |
  one
  two
  three
  four
`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	paths, err := policyfoundation.ListScenarioYAMLs(policyfoundation.Root(dir))
	if err != nil {
		t.Fatalf("ListScenarioYAMLs: %v", err)
	}
	var files []policyfoundation.ScenarioFile
	for _, p := range paths {
		sf, err := policyfoundation.ParseScenarioYAML(p)
		if err != nil {
			t.Fatalf("ParseScenarioYAML(%s): %v", p, err)
		}
		files = append(files, sf)
	}

	cfg := policyfoundation.PolicyConfig{ScenarioPromptMaxLines: 2}
	validIDs := map[string]struct{}{"Principle 1": {}}

	var vs []policyfoundation.Violation
	vs = append(vs, policyfoundation.PrincipleAlignmentGuard(files, validIDs)...)
	vs = append(vs, policyfoundation.ScenarioPromptCapGuard(files, cfg)...)

	report := policyfoundation.BuildReport(2, vs,
		policyfoundation.ExceptionDelta{BaselineCount: 0, CurrentCount: 0, DeltaStatus: "unchanged"})

	if report.Status != "failed" {
		t.Fatalf("status = %q, want failed", report.Status)
	}

	text := report.Text()
	for _, want := range []string{
		"rule_id=G067-A01",
		"rule_id=G067-A02",
		"config/prompt_contracts/missing-align-v1.yaml",
		"config/prompt_contracts/over-cap-v1.yaml",
		"docs/Product-Principles.md",
		"policy.scenario_prompt_max_lines",
		"cap is 2",
		"4 non-blank lines",
		"policy_source=specs/067-intent-driven-policy-enforcement/spec.md",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q\nfull:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b[") {
		t.Errorf("text contains ANSI color escape; expected color-free")
	}

	raw, err := report.JSON()
	if err != nil {
		t.Fatalf("Report.JSON: %v", err)
	}
	for _, want := range []string{
		`"rule_id": "G067-A01"`,
		`"rule_id": "G067-A02"`,
		`"path": "config/prompt_contracts/over-cap-v1.yaml"`,
		`"policy_source": "specs/067-intent-driven-policy-enforcement/spec.md"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("json missing %q\nfull:\n%s", want, string(raw))
		}
	}
}
