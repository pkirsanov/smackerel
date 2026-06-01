//go:build integration

// Spec 067 Scope 1 — guard report schema (SCN-067-A07 supporting).
//
// Pins the canonical Violation/Report shape: every documented field
// is present on a violation, the JSON encoding round-trips, the text
// rendering carries the same labelled fields without ANSI color, and
// the report status reflects violations and exception delta.

package policy

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPolicyGuardReportIncludesRulePathOwnerAndResolution proves the
// stable report shape carries the full documented field set so CI
// output and downstream tooling can rely on it.
func TestPolicyGuardReportIncludesRulePathOwnerAndResolution(t *testing.T) {
	v := Violation{
		RuleID:       "G067-A07",
		RuleName:     "policy-exception not in baseline",
		Path:         "config/prompt_contracts/example.yaml",
		Line:         12,
		Detail:       "accepted exception \"G067-A07-example\" is not in baseline",
		PolicySource: "specs/067-intent-driven-policy-enforcement/spec.md",
		Owner:        "reviewer",
		Resolution:   "bump policy-exception-baseline.json with reviewer approval",
	}
	report := BuildReport(8, []Violation{v}, ExceptionDelta{
		BaselineCount: 1, CurrentCount: 2, DeltaStatus: "grew",
	})

	if report.Status != "failed" {
		t.Fatalf("status = %q, want failed", report.Status)
	}
	if report.GuardsRun != 8 {
		t.Fatalf("guards_run = %d, want 8", report.GuardsRun)
	}

	// JSON shape: every documented field is present and round-trips.
	raw, err := report.JSON()
	if err != nil {
		t.Fatalf("Report.JSON(): %v", err)
	}
	for _, want := range []string{
		`"rule_id": "G067-A07"`,
		`"rule_name": "policy-exception not in baseline"`,
		`"path": "config/prompt_contracts/example.yaml"`,
		`"line": 12`,
		`"detail":`,
		`"policy_source": "specs/067-intent-driven-policy-enforcement/spec.md"`,
		`"owner": "reviewer"`,
		`"resolution":`,
		`"baseline_count": 1`,
		`"current_count": 2`,
		`"delta_status": "grew"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("report JSON missing %q\nfull:\n%s", want, string(raw))
		}
	}
	var roundTrip Report
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if len(roundTrip.Violations) != 1 || roundTrip.Violations[0].RuleID != "G067-A07" {
		t.Fatalf("round-trip lost violation fields: %+v", roundTrip.Violations)
	}

	// Text shape: labelled fields, no ANSI color codes.
	text := report.Text()
	for _, want := range []string{
		"rule_id=G067-A07",
		"path=config/prompt_contracts/example.yaml",
		"line=12",
		"owner=reviewer",
		"policy_source=specs/067-intent-driven-policy-enforcement/spec.md",
		"resolution=",
		"delta=grew",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report text missing %q\nfull:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b[") {
		t.Errorf("report text contains ANSI color escape; expected color-free output")
	}
}

// TestPolicyGuardReportStatusPassedWhenCleanAndUnchanged is the
// adversarial baseline: zero violations + delta=unchanged MUST yield
// status=passed, otherwise the report could not distinguish a real
// pass from a degraded one.
func TestPolicyGuardReportStatusPassedWhenCleanAndUnchanged(t *testing.T) {
	r := BuildReport(8, nil, ExceptionDelta{BaselineCount: 0, CurrentCount: 0, DeltaStatus: "unchanged"})
	if r.Status != "passed" {
		t.Fatalf("status = %q, want passed", r.Status)
	}
}
