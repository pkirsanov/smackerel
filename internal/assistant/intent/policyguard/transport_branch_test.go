// Spec 069 SCOPE-5 — Unit-level coverage for ReportTransportBranchViolations.
// SCN-069-A08.
//
// This is the same proof shape as
// tests/integration/policy/transport_branch_guard_test.go, but
// scoped to a single fixture tree so it does NOT import any of the
// transitively-broken-during-active-spec-work packages (config,
// assistant, etc.). The integration counterpart re-runs the guard
// against the real repo subtree once those packages are healed.

package policyguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportTransportBranchViolations_FlagsSwitchOnTransport(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad_scenario.go"), []byte(`package badscenario

type msg struct{ Transport string }

func dispatch(m msg) string {
	switch m.Transport {
	case "telegram":
		return "tg"
	}
	return ""
}
`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := ReportTransportBranchViolations(dir)
	if err != nil {
		t.Fatalf("ReportTransportBranchViolations: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("findings=%d want 1; %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "bad_scenario.go") || !strings.Contains(got[0].Message, TransportBranchViolation) {
		t.Errorf("finding message=%q lacks file name or violation phrase", got[0].Message)
	}
}

func TestReportTransportBranchViolations_FlagsEqualityCheck(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "branch_eq.go"), []byte(`package x

type msg struct{ Transport string }

func pick(m msg) bool { return m.Transport == "web" }
`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReportTransportBranchViolations(dir)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("findings=%d want 1; %+v", len(got), got)
	}
}

func TestReportTransportBranchViolations_AssignmentIsNotFlagged(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok_adapter.go"), []byte(`package okadapter

type msg struct{ Transport string }

func translate() msg { m := msg{}; m.Transport = "web"; return m }
`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReportTransportBranchViolations(dir)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("clean assignment wrongly flagged: %+v", got)
	}
}

func TestReportTransportBranchViolations_TestFilesAreSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "scenario_test.go"), []byte(`package x

type msg struct{ Transport string }

func _() bool { return msg{}.Transport == "web" }
`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReportTransportBranchViolations(dir)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("test file wrongly flagged: %+v", got)
	}
}
