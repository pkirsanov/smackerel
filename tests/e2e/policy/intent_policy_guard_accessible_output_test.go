//go:build e2e

// Spec 067 Scope 1 — SCN-067-A07 / SCN-067-A08 E2E report output.
//
// TestIntentPolicyGuardE2E_PrintsAccessibleFailureRows pins the
// canonical CI-output contract for the spec 067 guard suite using the
// shared report builder:
//
//   - JSON encoding carries rule_id, rule_name, path, line, owner,
//     policy_source, detail, resolution, and the exceptions delta;
//   - plain-text rendering carries those same labels with no ANSI
//     color escapes, so screen readers and log scrapers parse the
//     same row a human reads.
//
// A planted Violation + grew-delta MUST produce status=failed; the
// adversarial empty/unchanged baseline MUST produce status=passed.
// This is the contract spec 067 Scopes 2/3/4 guards will emit when
// they fail in CI.

package policy

import (
	"strings"
	"testing"

	policyfoundation "github.com/smackerel/smackerel/tests/integration/policy"
)

func TestIntentPolicyGuardE2E_PrintsAccessibleFailureRows(t *testing.T) {
	v := policyfoundation.Violation{
		RuleID:       "G067-A07",
		RuleName:     "policy-exception not in baseline",
		Path:         "config/prompt_contracts/example.yaml",
		Line:         12,
		Detail:       "accepted exception G067-A07-example not in baseline",
		PolicySource: "specs/067-intent-driven-policy-enforcement/spec.md",
		Owner:        "reviewer",
		Resolution:   "bump policy-exception-baseline.json with reviewer approval",
	}
	report := policyfoundation.BuildReport(8,
		[]policyfoundation.Violation{v},
		policyfoundation.ExceptionDelta{BaselineCount: 1, CurrentCount: 2, DeltaStatus: "grew"},
	)
	if report.Status != "failed" {
		t.Fatalf("status = %q, want failed", report.Status)
	}

	text := report.Text()
	for _, want := range []string{
		"rule_id=G067-A07",
		"path=config/prompt_contracts/example.yaml",
		"line=12",
		"owner=reviewer",
		"policy_source=specs/067-intent-driven-policy-enforcement/spec.md",
		"delta=grew",
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
		`"rule_id": "G067-A07"`,
		`"path": "config/prompt_contracts/example.yaml"`,
		`"policy_source": "specs/067-intent-driven-policy-enforcement/spec.md"`,
		`"delta_status": "grew"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("json missing %q\nfull:\n%s", want, string(raw))
		}
	}

	// Adversarial baseline: empty + unchanged MUST be status=passed.
	clean := policyfoundation.BuildReport(8, nil,
		policyfoundation.ExceptionDelta{BaselineCount: 0, CurrentCount: 0, DeltaStatus: "unchanged"})
	if clean.Status != "passed" {
		t.Fatalf("clean report status = %q, want passed", clean.Status)
	}
}
