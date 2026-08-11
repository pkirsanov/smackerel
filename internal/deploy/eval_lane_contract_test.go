package deploy

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// BUG-061-011 — the assistant acceptance gate lives in
// tests/eval/assistant/acceptance_test.go behind `//go:build integration`,
// and scripts/runtime/go-integration.sh selects packages by an explicit
// allow-list rather than `./...`. Those are two halves of one contract held
// in two files that never referenced each other, so the opt-out executed and
// the opt-in did not, and nothing turned red. This file is the missing
// invariant.
//
// It lives in internal/deploy (untagged, unit lane) deliberately: a guard
// hosted inside the integration lane would be disabled by the very edit that
// disables the gate.
const (
	evalLaneRelpath = "scripts/runtime/go-integration.sh"
	evalGateRelpath = "tests/eval/assistant/acceptance_test.go"

	evalGateMarkerPrefix = "ASSISTANT_ACCEPTANCE_GATE_V1"
	evalGateTestName     = "TestAcceptanceGate_RoutingAccuracyAndCaptureFallback"
	evalGatePackageArg   = "./tests/eval/..."
	evalGateSkipNotice   = "acceptance-gate executed-assertion assertion NOT ENFORCED"
	evalGateCountParse   = "executed_assertions"

	// Asserted as an assignment rather than a bare mention so a comment
	// naming the marker cannot satisfy the signal.
	evalGateMarkerAssignment = `gate_marker_prefix="ASSISTANT_ACCEPTANCE_GATE_V1"`

	// The bare `[[ -z "$go_run_selector" ]]` test also appears in the
	// wrapper's --run= argument parsing, so a contract keyed on it alone
	// would be satisfied by the wrong line. This literal identifies the
	// enforcement site itself and nothing else.
	evalGateSelectorGuard = `if [[ -z "$go_run_selector" ]]; then # full-lane run: acceptance-gate assertion is ENFORCED`

	evalGateFailedGuard    = "if !t.Failed()"
	evalGateThresholdError = "acceptance-gate FAIL: routing accuracy"
)

// evalLaneRejectsZeroComparisons are the comparison shapes that refuse an
// executed-assertion count of zero. At least one must be present, otherwise
// the lane would accept a gate that measured nothing.
var evalLaneRejectsZeroComparisons = []string{
	`"$gate_executed_assertions" -lt 1`,
	`"$gate_executed_assertions" -ge 1`,
	`"$gate_executed_assertions" -gt 0`,
}

// evalLaneAcceptsZeroComparisons are the shapes that would let a zero count
// through. None may be present.
var evalLaneAcceptsZeroComparisons = []string{
	`"$gate_executed_assertions" -lt 0`,
	`"$gate_executed_assertions" -ge 0`,
	`"$gate_executed_assertions" -gt -1`,
}

// evalLaneBypassTokens are refusal-shaped escape hatches. The focused-run
// path is the only non-enforcement path and it is visible in the output;
// anything else is a bypass.
var evalLaneBypassTokens = []string{
	"SKIP_",
	"--force",
	"--no-verify",
	"--insecure",
	"INSECURE_",
}

// evalLanePassesPackageToGoTest reports whether the lane actually appends the
// eval package tree to the go test argument vector. A plain substring search
// would also match a comment that merely mentions the package — and "a comment
// cannot fail" is the exact defect BUG-061-011 records, so the signal is read
// from the argument-building code only.
func evalLanePassesPackageToGoTest(lane string) bool {
	for _, line := range strings.Split(lane, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.Contains(line, "go_test_args+=(") && strings.Contains(line, evalGatePackageArg) {
			return true
		}
	}
	return false
}

// assertEvalLaneContract is the pure invariant: given the text of the
// integration lane wrapper and the text of the acceptance gate test, does the
// pairing still hold? Pure so fixtures can be string literals or mutations of
// the real files, with no filesystem dependency.
func assertEvalLaneContract(lane, gate string) error {
	if !evalLanePassesPackageToGoTest(lane) {
		return fmt.Errorf("%s does not pass %q to go test, so the acceptance gate's package is never compiled and its build tag can never match",
			evalLaneRelpath, evalGatePackageArg)
	}
	if !strings.Contains(lane, evalGateMarkerAssignment) {
		return fmt.Errorf("%s does not bind the %q marker prefix, so it cannot tell a gate that ran from a gate that was skipped",
			evalLaneRelpath, evalGateMarkerPrefix)
	}
	if !strings.Contains(lane, evalGateCountParse) {
		return fmt.Errorf("%s does not parse %q out of the marker line, so it asserts presence but not measurement",
			evalLaneRelpath, evalGateCountParse)
	}

	rejectsZero := false
	for _, cmp := range evalLaneRejectsZeroComparisons {
		if strings.Contains(lane, cmp) {
			rejectsZero = true
			break
		}
	}
	if !rejectsZero {
		return fmt.Errorf("%s has no comparison that refuses an executed-assertion count of zero (want one of %v)",
			evalLaneRelpath, evalLaneRejectsZeroComparisons)
	}
	for _, cmp := range evalLaneAcceptsZeroComparisons {
		if strings.Contains(lane, cmp) {
			return fmt.Errorf("%s contains the zero-accepting comparison %q, which would pass a gate that evaluated nothing",
				evalLaneRelpath, cmp)
		}
	}

	if !strings.Contains(lane, evalGateSelectorGuard) {
		return fmt.Errorf("%s does not key enforcement on the empty run-selector condition %q; enforcement must be skipped only for a focused run",
			evalLaneRelpath, evalGateSelectorGuard)
	}
	if !strings.Contains(lane, evalGateSkipNotice) {
		return fmt.Errorf("%s does not print the %q notice, so a focused run would skip the assertion silently",
			evalLaneRelpath, evalGateSkipNotice)
	}
	if !strings.Contains(lane, evalGateTestName) {
		return fmt.Errorf("%s does not name %s in its diagnostics, so an operator reading a failure cannot tell which gate did not run",
			evalLaneRelpath, evalGateTestName)
	}
	for _, token := range evalLaneBypassTokens {
		if strings.Contains(lane, token) {
			return fmt.Errorf("%s contains bypass token %q; the focused-run path is the only permitted non-enforcement path",
				evalLaneRelpath, token)
		}
	}

	markerCall := strings.Index(gate, "FormatGateMarker(")
	if markerCall < 0 {
		return fmt.Errorf("%s never calls FormatGateMarker, so no marker line is emitted for the lane to assert on",
			evalGateRelpath)
	}
	if failedGuard := strings.Index(gate, evalGateFailedGuard); failedGuard >= 0 && markerCall > failedGuard {
		return fmt.Errorf("%s emits the marker inside the %q block, so a failing gate is indistinguishable from a gate that never ran",
			evalGateRelpath, evalGateFailedGuard)
	}
	if threshold := strings.Index(gate, evalGateThresholdError); threshold >= 0 && markerCall > threshold {
		return fmt.Errorf("%s emits the marker after the threshold comparison; it must be emitted before, so it survives a threshold failure",
			evalGateRelpath)
	}
	return nil
}

func evalLaneSources(t *testing.T) (lane, gate string) {
	t.Helper()
	root := envsubstWrapperRepoRoot(t)
	return readDeployContractFile(t, filepath.Join(root, filepath.FromSlash(evalLaneRelpath))),
		readDeployContractFile(t, filepath.Join(root, filepath.FromSlash(evalGateRelpath)))
}

// mutateEvalFixture applies exactly one adversarial mutation and refuses to
// proceed if the replacement was a no-op. Without this guard a stale mutation
// literal would silently produce an unmutated fixture.
func mutateEvalFixture(t *testing.T, source, old, replacement string) string {
	t.Helper()
	if !strings.Contains(source, old) {
		t.Fatalf("adversarial fixture is stale: %q is not present in the source under mutation", old)
	}
	mutated := strings.Replace(source, old, replacement, 1)
	if mutated == source {
		t.Fatalf("adversarial mutation of %q was a no-op; the fixture would be identical to the baseline", old)
	}
	return mutated
}

// requireEvalBaselinePasses is the anti-tautology precondition every
// adversarial case runs first. Without it an adversarial test could pass
// because the baseline was already broken for an unrelated reason, which is
// exactly how a regression test becomes decorative.
func requireEvalBaselinePasses(t *testing.T, lane, gate string) {
	t.Helper()
	if err := assertEvalLaneContract(lane, gate); err != nil {
		t.Fatalf("adversarial precondition failed: the unmutated baseline must satisfy the contract, got: %v", err)
	}
}

// TestEvalLaneContract_LaneRunsGateAndAssertsExecutedAssertions reads the real
// files. This is the test that fails while the bug is present.
func TestEvalLaneContract_LaneRunsGateAndAssertsExecutedAssertions(t *testing.T) {
	lane, gate := evalLaneSources(t)
	if err := assertEvalLaneContract(lane, gate); err != nil {
		t.Fatal(err)
	}
}

// TestEvalLaneContract_AcceptsMinimalConformantFixtures is adversarial case A0
// from design.md: a minimal literal pair that satisfies every signal must be
// accepted. It proves the contract is satisfiable rather than unconditionally
// refusing, which would make every adversarial case below meaningless.
func TestEvalLaneContract_AcceptsMinimalConformantFixtures(t *testing.T) {
	const lane = `go_test_args+=(./tests/integration/... ./tests/eval/...)
gate_marker_prefix="ASSISTANT_ACCEPTANCE_GATE_V1"
if [[ -z "$go_run_selector" ]]; then # full-lane run: acceptance-gate assertion is ENFORCED
	gate_executed_assertions="${gate_marker_line##*executed_assertions=}"
	if [[ "$gate_executed_assertions" -lt 1 ]]; then
		echo "ERROR: TestAcceptanceGate_RoutingAccuracyAndCaptureFallback evaluated nothing" >&2
	fi
else
	echo "NOTICE: acceptance-gate executed-assertion assertion NOT ENFORCED"
fi`
	const gate = `r := Run(c)
fmt.Println(FormatGateMarker(r))
report := FormatReport(r)
t.Errorf("acceptance-gate FAIL: routing accuracy %.4f", r.RoutingAccuracy)
if !t.Failed() {
	fmt.Println(report)
}`
	if err := assertEvalLaneContract(lane, gate); err != nil {
		t.Fatalf("contract refused a conformant fixture pair: %v", err)
	}
}

// TestEvalLaneContract_AdversarialRejectsMissingEvalPackage is case A1 — the
// exact defect this bug records. The mutation reproduces the pre-fix content
// of the lane wrapper.
func TestEvalLaneContract_AdversarialRejectsMissingEvalPackage(t *testing.T) {
	lane, gate := evalLaneSources(t)
	requireEvalBaselinePasses(t, lane, gate)

	broken := mutateEvalFixture(t, lane, " "+evalGatePackageArg+")", ")")
	if evalLanePassesPackageToGoTest(broken) {
		t.Fatal("adversarial fixture is stale: the mutation did not remove the eval package from the go test argument vector")
	}
	if err := assertEvalLaneContract(broken, gate); err == nil {
		t.Fatal("contract accepted a lane that never passes ./tests/eval/... to go test — this is the original BUG-061-011 defect and it must be rejected")
	}
}

// TestEvalLaneContract_AdversarialRejectsMissingOrZeroAssertion covers cases
// A2 (assertion dropped) and A3 (assertion accepts zero). "The package is in
// the list" is not the property that matters: a package can be selected and
// still contribute zero executed assertions.
func TestEvalLaneContract_AdversarialRejectsMissingOrZeroAssertion(t *testing.T) {
	lane, gate := evalLaneSources(t)
	requireEvalBaselinePasses(t, lane, gate)

	t.Run("A2_assertion_removed", func(t *testing.T) {
		if !strings.Contains(lane, evalGateMarkerAssignment) {
			t.Fatalf("adversarial fixture is stale: the lane does not bind %q", evalGateMarkerAssignment)
		}
		broken := strings.ReplaceAll(lane, evalGateMarkerPrefix, "UNRELATED_MARKER")
		if err := assertEvalLaneContract(broken, gate); err == nil {
			t.Fatal("contract accepted a lane that runs the eval package but never looks for the gate marker")
		}
	})

	t.Run("A3_assertion_accepts_zero", func(t *testing.T) {
		broken := mutateEvalFixture(t, lane,
			`"$gate_executed_assertions" -lt 1`,
			`"$gate_executed_assertions" -lt 0`)
		if err := assertEvalLaneContract(broken, gate); err == nil {
			t.Fatal("contract accepted a lane whose count comparison passes an executed-assertion count of zero")
		}
	})
}

// TestEvalLaneContract_AdversarialRejectsConditionalOrAbsentMarker covers
// cases A4 (marker emitted only on pass) and A5 (marker never emitted).
func TestEvalLaneContract_AdversarialRejectsConditionalOrAbsentMarker(t *testing.T) {
	lane, gate := evalLaneSources(t)
	requireEvalBaselinePasses(t, lane, gate)

	t.Run("A4_marker_conditional_on_passing", func(t *testing.T) {
		stripped := mutateEvalFixture(t, gate, "fmt.Println(FormatGateMarker(r))", "")
		broken := mutateEvalFixture(t, stripped,
			evalGateFailedGuard+" {",
			evalGateFailedGuard+" {\n\t\tfmt.Println(FormatGateMarker(r))")
		if err := assertEvalLaneContract(lane, broken); err == nil {
			t.Fatal("contract accepted a gate that emits its marker only when the gate passes, which cannot distinguish a failing gate from a gate that never ran")
		}
	})

	t.Run("A5_marker_never_emitted", func(t *testing.T) {
		broken := strings.ReplaceAll(gate, "FormatGateMarker(", "formatGateMarkerRemoved(")
		if broken == gate {
			t.Fatal("adversarial fixture is stale: the gate does not call FormatGateMarker at all")
		}
		if err := assertEvalLaneContract(lane, broken); err == nil {
			t.Fatal("contract accepted a gate that never emits a marker line")
		}
	})
}

// TestEvalLaneContract_AdversarialRejectsBypassOrBroadenedSkip covers cases A6
// (a skip flag is introduced) and A7 (the skip condition is broadened beyond
// the focused-run case).
func TestEvalLaneContract_AdversarialRejectsBypassOrBroadenedSkip(t *testing.T) {
	lane, gate := evalLaneSources(t)
	requireEvalBaselinePasses(t, lane, gate)

	t.Run("A6_bypass_env_var_introduced", func(t *testing.T) {
		broken := mutateEvalFixture(t, lane,
			evalGateSelectorGuard,
			`[[ -z "$go_run_selector" && -z "${SKIP_EVAL_GATE:-}" ]]`)
		if err := assertEvalLaneContract(broken, gate); err == nil {
			t.Fatal("contract accepted a lane that can be told to skip the acceptance-gate assertion via an environment variable")
		}
	})

	t.Run("A7_skip_condition_broadened", func(t *testing.T) {
		broken := mutateEvalFixture(t, lane,
			evalGateSelectorGuard,
			`[[ -n "${ENFORCE_EVAL_GATE:-}" ]]`)
		if err := assertEvalLaneContract(broken, gate); err == nil {
			t.Fatal("contract accepted a lane that enforces only when an env var opts in, rather than whenever the run-selector is empty")
		}
	})
}
