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

// ---------------------------------------------------------------------------
// R3.5 — "when go test itself fails AND the emission is absent or zero, the
// lane MUST still exit non-zero. Neither failure may mask the other."
//
// The lane already behaves this way. What was missing was a DETECTOR: every
// signal above is a presence check, and presence survives a reordering. R3.5
// is an ORDERING property — the gate diagnostic must reach stderr before the
// go-test path exits, and the gate's own exit must remain reachable after it.
// Commit fa61daa0 merged two consecutive `if [[ "$go_test_rc" -ne 0 ]]` blocks
// in this exact region; that edit was behaviour-preserving, and nothing would
// have turned red had it not been.
// ---------------------------------------------------------------------------

const (
	evalLaneGateFlagInit    = `gate_marker_check_failed=0`
	evalLaneGateFlagSet     = `gate_marker_check_failed=1`
	evalLaneGoTestExitGuard = `if [[ "$go_test_rc" -ne 0 ]]; then`
	evalLaneGoTestExit      = `exit "$go_test_rc"`
	evalLaneGoTestFailError = `ERROR: go-integration: go test failed`
	evalLaneGateExitGuard   = `if [[ "$gate_marker_check_failed" -ne 0 ]]; then`

	// Asserted verbatim by the adversarial cases so a reworded message
	// cannot silently retire the case it was written to catch.
	evalLaneMaskedGateDiagErr  = "reaches stderr only after the go-test exit guard"
	evalLaneSilentGateFlagErr  = "raises the gate-failure flag with no stderr diagnostic before it"
	evalLaneMaskedGoTestErr    = "the go-test failure would never be reported"
	evalLaneUnreportedExitErr  = "exits on the go-test failure without reporting it"
	evalLaneMissingGateExitErr = "has no gate-failure exit"
)

// evalLaneStderrDiagnostic reports whether a line is an error diagnostic
// written to stderr. Read as a shape rather than as specific wording: a
// reworded message must stay a REPORTED message, and that is the property
// R3.5 protects.
func evalLaneStderrDiagnostic(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "echo ") &&
		strings.Contains(trimmed, "ERROR:") &&
		strings.Contains(trimmed, ">&2")
}

// evalLaneIsGateDiagnostic distinguishes the gate-check diagnostics from the
// go-test one. Naming the gate test is the discriminator because R3.4 already
// requires every gate diagnostic to name it, and the go-test failure message
// does not.
func evalLaneIsGateDiagnostic(line string) bool {
	return evalLaneStderrDiagnostic(line) && strings.Contains(line, evalGateTestName)
}

// assertEvalLaneDualFailureReporting is the R3.5 invariant, expressed over the
// lane text as an ordering property so a future reordering turns red.
//
// Pure, like assertEvalLaneContract, so fixtures can be mutations of the real
// file with no filesystem dependency.
func assertEvalLaneDualFailureReporting(lane string) error {
	offset := func(literal string) int { return strings.Index(lane, literal) }

	flagInit := offset(evalLaneGateFlagInit)
	goTestGuard := offset(evalLaneGoTestExitGuard)
	goTestErr := offset(evalLaneGoTestFailError)
	goTestExit := offset(evalLaneGoTestExit)
	gateExitGuard := offset(evalLaneGateExitGuard)

	for _, required := range []struct {
		at      int
		literal string
	}{
		{flagInit, evalLaneGateFlagInit},
		{goTestGuard, evalLaneGoTestExitGuard},
		{goTestExit, evalLaneGoTestExit},
	} {
		if required.at < 0 {
			return fmt.Errorf("%s does not contain %q, so the two failure paths R3.5 governs cannot both be present",
				evalLaneRelpath, required.literal)
		}
	}
	if gateExitGuard < 0 {
		return fmt.Errorf("%s %s (%q absent); a gate-check failure would leave the lane exiting zero whenever go test passed",
			evalLaneRelpath, evalLaneMissingGateExitErr, evalLaneGateExitGuard)
	}
	if goTestErr < 0 || goTestErr > goTestExit {
		return fmt.Errorf("%s %s: the %q diagnostic must precede %q, otherwise a go-test failure exits silently",
			evalLaneRelpath, evalLaneUnreportedExitErr, evalLaneGoTestFailError, evalLaneGoTestExit)
	}

	// The gate-failure exit is a SIBLING of the go-test exit, reached after
	// it. Placed before, `exit 1` would fire first and the go-test failure
	// diagnostic at goTestErr would never print — the mirror-image mask.
	if gateExitGuard < goTestExit {
		return fmt.Errorf("%s places the gate-failure exit (offset %d) before %q (offset %d), so %s when both fail",
			evalLaneRelpath, gateExitGuard, evalLaneGoTestExit, goTestExit, evalLaneMaskedGoTestErr)
	}
	if flagInit > goTestGuard {
		return fmt.Errorf("%s initialises %q (offset %d) after the go-test exit guard (offset %d); the flag must exist before either exit is decided",
			evalLaneRelpath, evalLaneGateFlagInit, flagInit, goTestGuard)
	}

	// Every gate diagnostic must already be on stderr by the time the
	// go-test path can exit. This is the check that the "diagnostic moved
	// below the exit" mutation trips.
	lineStart := 0
	gateDiagnostics := 0
	for _, line := range strings.Split(lane, "\n") {
		start := lineStart
		lineStart += len(line) + 1
		if !evalLaneIsGateDiagnostic(line) {
			continue
		}
		gateDiagnostics++
		if start > goTestGuard {
			return fmt.Errorf("%s: a gate-check diagnostic at offset %d %s (offset %d), so a concurrent go-test failure exits before it prints and masks it: %s",
				evalLaneRelpath, start, evalLaneMaskedGateDiagErr, goTestGuard, strings.TrimSpace(line))
		}
	}
	if gateDiagnostics == 0 {
		return fmt.Errorf("%s emits no stderr diagnostic naming %s, so a gate-check failure is unreportable",
			evalLaneRelpath, evalGateTestName)
	}

	// Every raised flag must be paired with a diagnostic immediately above
	// it. A flag raised silently exits non-zero but tells the operator
	// nothing, which is R3.4's failure and R3.5's mask in combination.
	lineStart = 0
	flagSites := 0
	diagnosticPending := false
	for _, line := range strings.Split(lane, "\n") {
		start := lineStart
		lineStart += len(line) + 1

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if evalLaneStderrDiagnostic(line) {
			diagnosticPending = true
			continue
		}
		if !strings.Contains(line, evalLaneGateFlagSet) {
			diagnosticPending = false
			continue
		}
		flagSites++
		if !diagnosticPending {
			return fmt.Errorf("%s %s at offset %d: %s",
				evalLaneRelpath, evalLaneSilentGateFlagErr, start, trimmed)
		}
		if start > goTestGuard {
			return fmt.Errorf("%s raises the gate-failure flag at offset %d, after the go-test exit guard (offset %d), where a go-test failure has already exited past it",
				evalLaneRelpath, start, goTestGuard)
		}
		diagnosticPending = false
	}
	if flagSites == 0 {
		return fmt.Errorf("%s never sets %q, so no gate-check failure can reach the gate-failure exit",
			evalLaneRelpath, evalLaneGateFlagSet)
	}
	return nil
}

// requireEvalDualReportingBaselinePasses is the anti-tautology precondition for
// the R3.5 adversarial cases, mirroring requireEvalBaselinePasses.
func requireEvalDualReportingBaselinePasses(t *testing.T, lane string) {
	t.Helper()
	if err := assertEvalLaneDualFailureReporting(lane); err != nil {
		t.Fatalf("adversarial precondition failed: the unmutated lane must satisfy the R3.5 invariant, got: %v", err)
	}
}

// evalLaneFirstGateDiagnostic returns the first gate diagnostic verbatim, with
// its original indentation, so a fixture relocates the REAL line rather than a
// paraphrase that could drift out of sync with the script.
func evalLaneFirstGateDiagnostic(t *testing.T, lane string) string {
	t.Helper()
	for _, line := range strings.Split(lane, "\n") {
		if evalLaneIsGateDiagnostic(line) {
			return line
		}
	}
	t.Fatalf("adversarial fixture is stale: the lane contains no stderr diagnostic naming %s", evalGateTestName)
	return ""
}

// TestEvalLaneContract_DualFailureReportingNeitherMasksTheOther reads the real
// lane wrapper. This is the R3.5 assertion that was missing: report.md
// discharged R3.5 by code reading, and a reordering of the block this test
// covers would previously have turned nothing red.
func TestEvalLaneContract_DualFailureReportingNeitherMasksTheOther(t *testing.T) {
	lane, _ := evalLaneSources(t)
	if err := assertEvalLaneDualFailureReporting(lane); err != nil {
		t.Fatal(err)
	}
}

// TestEvalLaneContract_AdversarialRejectsMaskedFailureReporting is the R3.5
// adversarial set: A8 (gate diagnostic relocated below the go-test exit),
// A9 (the two exit blocks reordered so the go-test failure goes unreported),
// A10 (a gate-failure flag raised with no diagnostic at all).
func TestEvalLaneContract_AdversarialRejectsMaskedFailureReporting(t *testing.T) {
	lane, _ := evalLaneSources(t)
	requireEvalDualReportingBaselinePasses(t, lane)

	t.Run("A8_gate_diagnostic_moved_below_go_test_exit", func(t *testing.T) {
		diagnostic := evalLaneFirstGateDiagnostic(t, lane)
		stripped := mutateEvalFixture(t, lane, diagnostic+"\n", "")
		broken := mutateEvalFixture(t, stripped,
			evalLaneGateExitGuard,
			diagnostic+"\n"+evalLaneGateExitGuard)

		err := assertEvalLaneDualFailureReporting(broken)
		if err == nil {
			t.Fatal("contract accepted a lane whose gate-check diagnostic is emitted only after `exit \"$go_test_rc\"`, where a concurrent go-test failure masks it entirely")
		}
		if !strings.Contains(err.Error(), evalLaneMaskedGateDiagErr) {
			t.Fatalf("rejection did not name the masking it detected: want substring %q, got: %v",
				evalLaneMaskedGateDiagErr, err)
		}
		t.Logf("A8 REJECTED as required: %v", err)
	})

	t.Run("A9_exit_blocks_reordered_go_test_failure_unreported", func(t *testing.T) {
		gateExitBlock := evalLaneGateExitGuard + "\n\texit 1\nfi\n"
		stripped := mutateEvalFixture(t, lane, gateExitBlock, "")
		broken := mutateEvalFixture(t, stripped,
			evalLaneGoTestExitGuard,
			gateExitBlock+evalLaneGoTestExitGuard)

		err := assertEvalLaneDualFailureReporting(broken)
		if err == nil {
			t.Fatal("contract accepted a lane whose gate-failure exit fires before the go-test failure is reported, masking the go-test failure")
		}
		if !strings.Contains(err.Error(), evalLaneMaskedGoTestErr) {
			t.Fatalf("rejection did not name the masking it detected: want substring %q, got: %v",
				evalLaneMaskedGoTestErr, err)
		}
		t.Logf("A9 REJECTED as required: %v", err)
	})

	t.Run("A10_gate_flag_raised_with_no_diagnostic", func(t *testing.T) {
		diagnostic := evalLaneFirstGateDiagnostic(t, lane)
		broken := mutateEvalFixture(t, lane, diagnostic+"\n", "")

		err := assertEvalLaneDualFailureReporting(broken)
		if err == nil {
			t.Fatal("contract accepted a lane that raises the gate-failure flag without printing anything, so the operator sees a non-zero exit with no cause")
		}
		if !strings.Contains(err.Error(), evalLaneSilentGateFlagErr) {
			t.Fatalf("rejection did not name the silent flag it detected: want substring %q, got: %v",
				evalLaneSilentGateFlagErr, err)
		}
		t.Logf("A10 REJECTED as required: %v", err)
	})
}
