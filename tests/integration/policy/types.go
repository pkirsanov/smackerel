//go:build integration || e2e

// Spec 067 Scope 1 — shared policy guard foundation.
//
// This file defines the reusable PolicyGuard interface, the Violation
// type, and the JSON/text report serializers that every spec 067 guard
// produces. Stable rule IDs (G067-A01..G067-A08) live alongside the
// guard each rule belongs to; this file only owns the foundation.
//
// The output contract is the one design.md pins:
//
//   {
//     "status": "failed",
//     "guards_run": 8,
//     "violations": [ {rule_id, rule_name, path, line, detail,
//                      policy_source, owner} ],
//     "exceptions": { baseline_count, current_count, delta_status }
//   }
//
// Plain text mirrors this with one labelled row per violation; no ANSI
// color codes, so CI consumers and accessibility tools see the same
// text the test harness prints.

package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// PolicyConfig is the SST-sourced subset of config.PolicyConfig that
// guards need at runtime. It is passed by value into Guard.Run so
// guard tests can inject deterministic thresholds without touching
// the process env.
type PolicyConfig struct {
	ScenarioPromptMaxLines   int
	ExceptionBaselinePath    string
	ExceptionMaxAgeDays      int
	IntentBypassGuardEnabled bool
}

// Root is a typed repo root path; guards walk only paths under this
// root so a guard can never accidentally scan an unrelated tree.
type Root string

// Violation is the canonical finding shape every spec 067 guard
// produces. Every field is part of the documented output contract
// (design.md → API And Output Contracts). Tests assert presence of
// each field by name.
type Violation struct {
	RuleID       string `json:"rule_id"`
	RuleName     string `json:"rule_name"`
	Path         string `json:"path"`
	Line         int    `json:"line"`
	Detail       string `json:"detail"`
	PolicySource string `json:"policy_source"`
	Owner        string `json:"owner,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
}

// Guard is the shared interface every spec 067 policy guard implements.
// ID() returns the stable rule id (e.g., "G067-A07"). Run scans the
// scoped repo subtree and returns a deterministic, ordered slice of
// violations.
type Guard interface {
	ID() string
	Run(ctx context.Context, repo Root, cfg PolicyConfig) ([]Violation, error)
}

// ExceptionDelta summarises baseline accounting for the report shape.
type ExceptionDelta struct {
	BaselineCount int    `json:"baseline_count"`
	CurrentCount  int    `json:"current_count"`
	DeltaStatus   string `json:"delta_status"` // "unchanged" | "grew" | "shrunk"
}

// Report is the stable JSON shape the guard suite emits. RulesRun
// counts only guards that completed (Run did not error); the status is
// "failed" when len(Violations) > 0 or when DeltaStatus == "grew", and
// "passed" otherwise.
type Report struct {
	Status     string         `json:"status"`
	GuardsRun  int            `json:"guards_run"`
	Violations []Violation    `json:"violations"`
	Exceptions ExceptionDelta `json:"exceptions"`
}

// BuildReport assembles a stable Report from the per-guard outputs.
// Violations are sorted by (RuleID, Path, Line) so the report is
// byte-stable across runs.
func BuildReport(guardsRun int, violations []Violation, delta ExceptionDelta) Report {
	sorted := append([]Violation(nil), violations...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].RuleID != sorted[j].RuleID {
			return sorted[i].RuleID < sorted[j].RuleID
		}
		if sorted[i].Path != sorted[j].Path {
			return sorted[i].Path < sorted[j].Path
		}
		return sorted[i].Line < sorted[j].Line
	})
	status := "passed"
	if len(sorted) > 0 || delta.DeltaStatus == "grew" {
		status = "failed"
	}
	return Report{
		Status:     status,
		GuardsRun:  guardsRun,
		Violations: sorted,
		Exceptions: delta,
	}
}

// JSON returns the canonical JSON encoding of the Report.
func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// Text returns the canonical color-free plain-text report. Each
// violation occupies one row with labelled fields so screen readers
// and CI log scrapers can parse it without ANSI handling.
func (r Report) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "intent-policy-guard: status=%s guards_run=%d violations=%d exceptions=%d/%d delta=%s\n",
		r.Status, r.GuardsRun, len(r.Violations),
		r.Exceptions.CurrentCount, r.Exceptions.BaselineCount, r.Exceptions.DeltaStatus)
	for _, v := range r.Violations {
		fmt.Fprintf(&b, "rule_id=%s rule_name=%q path=%s line=%d owner=%s policy_source=%s detail=%q resolution=%q\n",
			v.RuleID, v.RuleName, v.Path, v.Line, v.Owner, v.PolicySource, v.Detail, v.Resolution)
	}
	return b.String()
}
