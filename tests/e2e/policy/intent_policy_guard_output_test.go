//go:build e2e

// Spec 068 Scope 4 — Raw-route bypass policy guard output (SCN-068-A08).
//
// SCN-068-A08 is source-scanning policy behavior, NOT transport-bound.
// The e2e here proves the same guard library the integration test
// uses produces output that, on a planted bypass fixture, names BOTH
// the offending file AND the canonical missing-compiler-step phrase.
// Spec 067's policy-guard wiring will eventually invoke the same
// library from CI; this e2e pins the contract its output must hold.

package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/intent/policyguard"
)

// TestIntentPolicyGuardE2E_RawRouteBypassNamesCompilerStep
// SCN-068-A08.
//
// Plant a fixture file that calls router.Route() without any
// intent.Compiler reference. Run the guard. Assert the resulting
// Finding.Message names the file path AND contains the canonical
// "missing intent.Compiler step before Router.Route" phrase
// verbatim — this is the wording spec 067 (and any operator reading
// CI output) will rely on.
func TestIntentPolicyGuardE2E_RawRouteBypassNamesCompilerStep(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "raw_bypass_ingress.go")
	body := `package bad

import "context"

type Router interface {
	Route(ctx context.Context, env interface{}) (string, error)
}

func Handle(ctx context.Context, r Router, raw string) {
	_, _ = r.Route(ctx, raw)
}
`
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := policyguard.ReportRawRouteBypasses(dir)
	if err != nil {
		t.Fatalf("ReportRawRouteBypasses: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1; findings=%+v", len(got), got)
	}
	msg := got[0].Message
	if !strings.Contains(msg, "raw_bypass_ingress.go") {
		t.Fatalf("guard output does not name the offending file: %q", msg)
	}
	if !strings.Contains(msg, policyguard.MissingCompilerStep) {
		t.Fatalf("guard output does not contain the canonical missing-compiler-step phrase %q: got %q",
			policyguard.MissingCompilerStep, msg)
	}

	// Adversarial baseline: a fixture WITH an intent.Compiler
	// reference MUST NOT be flagged.
	good := strings.Replace(body,
		"_, _ = r.Route(ctx, raw)",
		"_ = \"intent.Compiler\"\n\t_, _ = r.Route(ctx, raw)",
		1)
	if err := os.WriteFile(fixture, []byte(good), 0o644); err != nil {
		t.Fatalf("rewrite fixture: %v", err)
	}
	clean, err := policyguard.ReportRawRouteBypasses(dir)
	if err != nil {
		t.Fatalf("ReportRawRouteBypasses (baseline): %v", err)
	}
	if len(clean) != 0 {
		t.Fatalf("adversarial baseline flagged %d findings, want 0: %+v", len(clean), clean)
	}
}
