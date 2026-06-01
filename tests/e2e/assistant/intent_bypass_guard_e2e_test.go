//go:build e2e

// Spec 071 SCOPE-03 — Synthetic raw-route bypass is rejected
// in the live stack (SCN-071-A08).
//
// This e2e drives the LIVE bypass-guard runtime ancestor check
// (policyguard.CheckIntentTraceAncestor) plus the static-source
// guard (policyguard.ReportRawRouteBypasses) against the real
// repository tree, asserting two invariants:
//
//   1. The real internal/assistant tree has ZERO static raw-route
//      bypass findings (the spec 068 guard contract is preserved by
//      the spec 071 work).
//   2. A synthetic raw-route bypass observation — a tool call whose
//      span carries NO IntentTrace ancestor (Present=false) — is
//      rejected by the runtime ancestor check with the canonical
//      `MissingIntentTraceAncestor` finding that names both the
//      span and the observed tool.
//   3. Adversarial baseline: an observation WITH a valid
//      compiled-intent ancestor that lists the observed tool is
//      NOT flagged. Without this baseline, a regression where the
//      runtime check always-fires would still pass invariant (2).
//
// The "live stack" qualifier here is the running test-stack
// process source tree: invariant (1) is provably re-verified
// against the real internal/assistant directory at test time, so a
// future change that introduces a raw-route call site is caught
// here even when the spec 068 integration row is not in the
// selection set.

package assistant_e2e

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/intent/policyguard"
)

func bypassGuardRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("go.mod not found walking up from %s", file)
	return ""
}

// TestIntentBypassGuardE2E_SyntheticRawRouteBypassIsRejected —
// SCN-071-A08 (e2e-api row).
func TestIntentBypassGuardE2E_SyntheticRawRouteBypassIsRejected(t *testing.T) {
	root := bypassGuardRepoRoot(t)

	// Invariant 1 — live static guard finds zero raw-route bypasses
	// under internal/assistant. This re-verifies the spec 068
	// invariant from the spec 071 selection set.
	static, err := policyguard.ReportRawRouteBypasses(filepath.Join(root, "internal", "assistant"))
	if err != nil {
		t.Fatalf("ReportRawRouteBypasses(real): %v", err)
	}
	if len(static) != 0 {
		var msgs []string
		for _, f := range static {
			msgs = append(msgs, f.Message)
		}
		t.Fatalf("expected zero static raw-route bypasses under internal/assistant, got %d:\n%s",
			len(static), strings.Join(msgs, "\n"))
	}

	// Invariant 2 — synthetic raw-route observation is rejected by
	// the runtime ancestor check. This is the bypass shape the
	// spec 067 policy guard sees on the OTel span tree when a tool
	// is invoked without a compiled-intent ancestor.
	bypass := []policyguard.ToolCallObservation{
		{
			SpanName: "synthetic.bypass.weather.lookup",
			ToolName: "weather.lookup",
			Ancestor: policyguard.IntentTraceAncestor{Present: false},
		},
	}
	bypassFindings := policyguard.CheckIntentTraceAncestor(bypass)
	if len(bypassFindings) != 1 {
		t.Fatalf("synthetic raw-route bypass produced %d findings, want exactly 1", len(bypassFindings))
	}
	if !strings.Contains(bypassFindings[0].Message, policyguard.MissingIntentTraceAncestor) {
		t.Fatalf("bypass finding did not name MissingIntentTraceAncestor: %q", bypassFindings[0].Message)
	}
	if !strings.Contains(bypassFindings[0].Message, "weather.lookup") {
		t.Fatalf("bypass finding did not name observed tool: %q", bypassFindings[0].Message)
	}
	if !strings.Contains(bypassFindings[0].Message, "synthetic.bypass.weather.lookup") {
		t.Fatalf("bypass finding did not name observing span: %q", bypassFindings[0].Message)
	}

	// Invariant 3 (adversarial baseline) — observation WITH a valid
	// ancestor MUST NOT be flagged. Without this, a regression where
	// the runtime check always-fires would still pass invariant (2).
	ok := []policyguard.ToolCallObservation{
		{
			SpanName: "synthetic.ok.weather.lookup",
			ToolName: "weather.lookup",
			Ancestor: policyguard.IntentTraceAncestor{
				Present:         true,
				CompilerInvoked: true,
				RouteDecision:   "scenarios/weather",
				ToolCallNames:   []string{"weather.lookup"},
			},
		},
	}
	if clean := policyguard.CheckIntentTraceAncestor(ok); len(clean) != 0 {
		var msgs []string
		for _, f := range clean {
			msgs = append(msgs, f.Message)
		}
		t.Fatalf("adversarial baseline: valid-ancestor observation produced %d findings, want 0:\n%s",
			len(clean), strings.Join(msgs, "\n"))
	}
}
