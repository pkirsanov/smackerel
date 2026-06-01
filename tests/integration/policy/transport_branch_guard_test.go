//go:build integration

// Spec 069 SCOPE-5 — Transport-branch policy guard test (SCN-069-A08).
//
// TestTransportBranchGuardRejectsScenarioTransportBranching asserts:
//
//   1. The real internal/assistant subtree is clean under the
//      transport-branch guard. Adapter + audit + context-store +
//      capturefallback files are on the closed AllowedTransportInspectors
//      allowlist; any scenario/facade/executor file that branches on
//      AssistantMessage.Transport would be flagged.
//   2. A planted fixture that switches on msg.Transport in a
//      scenario-style file IS reported. This is the adversarial
//      proof the guard is non-tautological: an actual policy
//      violation produces a finding with the canonical message
//      TransportBranchViolation naming the file.

package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/intent/policyguard"
)

func TestTransportBranchGuardRejectsScenarioTransportBranching(t *testing.T) {
	root := repoRoot(t)

	// 1. Real internal/assistant subtree MUST be clean.
	findings, err := policyguard.ReportTransportBranchViolations(filepath.Join(root, "internal", "assistant"))
	if err != nil {
		t.Fatalf("ReportTransportBranchViolations(real): %v", err)
	}
	if len(findings) != 0 {
		msgs := make([]string, 0, len(findings))
		for _, f := range findings {
			msgs = append(msgs, f.Message)
		}
		t.Fatalf("expected zero transport-branch findings under internal/assistant, got %d:\n%s",
			len(findings), strings.Join(msgs, "\n"))
	}

	// 2. Planted fixture with a scenario-style transport branch MUST
	// be flagged. Using a temp dir places the fixture outside the
	// allowlist (which is rooted at internal/assistant/...).
	dir := t.TempDir()
	fixture := filepath.Join(dir, "bad_scenario.go")
	body := `package badscenario

type msg struct{ Transport string }

func dispatch(m msg) string {
	switch m.Transport {
	case "telegram":
		return "tg"
	case "web":
		return "web"
	}
	return ""
}
`
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := policyguard.ReportTransportBranchViolations(dir)
	if err != nil {
		t.Fatalf("ReportTransportBranchViolations(fixture): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("fixture: got %d findings, want 1", len(got))
	}
	f := got[0]
	if !strings.Contains(f.Message, "bad_scenario.go") {
		t.Errorf("finding message does not name the planted file: %q", f.Message)
	}
	if !strings.Contains(f.Message, policyguard.TransportBranchViolation) {
		t.Errorf("finding message does not carry TransportBranchViolation phrase: %q", f.Message)
	}

	// Adversarial complement: a fixture that ONLY assigns transport
	// (the legitimate adapter Translate pattern) MUST NOT be flagged.
	cleanDir := t.TempDir()
	cleanFixture := filepath.Join(cleanDir, "ok_adapter.go")
	cleanBody := `package okadapter

type msg struct{ Transport string }

func translate() msg {
	m := msg{}
	m.Transport = "web"
	return m
}
`
	if err := os.WriteFile(cleanFixture, []byte(cleanBody), 0o644); err != nil {
		t.Fatalf("write clean fixture: %v", err)
	}
	cleanFindings, err := policyguard.ReportTransportBranchViolations(cleanDir)
	if err != nil {
		t.Fatalf("ReportTransportBranchViolations(clean): %v", err)
	}
	if len(cleanFindings) != 0 {
		t.Fatalf("clean fixture (assignment only) wrongly flagged: %+v", cleanFindings)
	}
}
