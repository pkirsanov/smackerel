# Scopes: BUG-031-010 Stress Harness Discovery and Classification Fail-Open

## Scope 1: Make Stress Discovery and Output Classification Fail Loud

**Status:** [ ] Not Started | [x] In Progress | [ ] Done | [ ] Blocked

**Scope-Kind:** runtime-behavior

**Depends On:** none

### Gherkin Scenarios

```gherkin
Feature: BUG-031-010 truthful stress harness outcomes

  Scenario: SCN-031-010-001 list failure propagates
    Given the readiness canary has passed
    And go test -list emits a discovery diagnostic and exits non-zero
    When the stress harness evaluates a selected workload package
    Then the harness exits with the discovery status
    And it does not report a normal selector miss

  Scenario: SCN-031-010-002 JSON warning is rejected
    Given a workload exits zero with a direct JSON level WARN record
    When the harness classifies the captured output
    Then the harness exits non-zero with a runtime warning diagnostic

  Scenario: SCN-031-010-003 quoted warning literal is benign
    Given a workload exits zero with warning-shaped text only inside a quoted message
    When the harness classifies the captured output
    Then the clean workload result is preserved

  Scenario: SCN-031-010-004 grep failure fails loud
    Given the classifier grep command exits with an execution error
    When the harness classifies otherwise clean output
    Then the harness exits non-zero because classification is untrusted

  Scenario: SCN-031-010-005 capture failure fails loud
    Given the output capture writer fails
    When the harness runs a checked package
    Then the harness exits non-zero before classification

  Scenario: SCN-031-010-006 failure paths clean captures
    Given each adversarial fixture owns a separate temporary capture directory
    When discovery, classifier, capture, forbidden-output, and workload failures complete
    Then each capture directory is empty
```

### Implementation Plan

1. Add the five new adversarial cases to `tests/stress/readiness/canary_test.go` before changing the harness.
2. Run the repository Go unit test command and record red-stage failures for SCN-031-010-001 through SCN-031-010-005, plus cleanup assertions for SCN-031-010-006.
3. Replace process-substitution selector discovery with explicit output and status capture.
4. Separate selection state from command failure so the package loop continues only on a successful no-match.
5. Make every classifier match status-aware and add the direct JSON warning form.
6. Anchor warning formats so quoted warning-shaped text remains benign.
7. Preserve capture-write checks and cleanup on every return path.
8. Re-run the same adversarial tests unchanged for green-stage evidence.
9. When no E2E lane is active, run the live stress gate and broader repository checks through `./smackerel.sh`.

### Change Boundary

**Allowed files:**

- `scripts/runtime/go-stress.sh`
- `tests/stress/readiness/canary_test.go`
- `internal/nats/client.go`
- `internal/nats/client_test.go`
- `specs/031-live-stack-testing/bugs/BUG-031-010-stress-harness-discovery-classification/**`

**Excluded files:**

- BUG-031-005 and BUG-031-006 artifacts and state
- Every other source, test, script, config, generated, documentation, Compose, deployment, and framework-managed file

The implementation owner must inspect `git status --short` before and after the change and attribute only the four allowed non-packet files to this bug. Existing dirty changes are preserved and must not be reformatted or reverted.

### Test Plan

| ID | Test Type | Category | File/Location | Scenario | Description | Command | Live System |
|----|-----------|----------|---------------|----------|-------------|---------|-------------|
| TP-031-010-01 | Adversarial unit | `unit` | `tests/stress/readiness/canary_test.go::TestGoStressHarness_ListFailurePropagates` | SCN-031-010-001 | Fake `go test -list` exits distinctly; exact status and diagnostic propagate without selector-miss text | `./smackerel.sh test unit --go` | No |
| TP-031-010-02 | Classifier matrix unit | `unit` | `tests/stress/readiness/canary_test.go::TestGoStressHarness_JSONWarningAndQuotedLiteralClassification` | SCN-031-010-002, SCN-031-010-003 | Direct JSON WARN forms reject while quoted logfmt, timestamp, and escaped-JSON examples remain clean | `./smackerel.sh test unit --go` | No |
| TP-031-010-03 | Adversarial unit | `unit` | `tests/stress/readiness/canary_test.go::TestGoStressHarness_ClassifierCommandFailurePropagates` | SCN-031-010-004 | Fake grep execution error cannot become clean output | `./smackerel.sh test unit --go` | No |
| TP-031-010-04 | Adversarial unit | `unit` | `tests/stress/readiness/canary_test.go::TestGoStressHarness_CaptureFailurePropagates` | SCN-031-010-005 | Fake capture writer failure terminates before trusted classification | `./smackerel.sh test unit --go` | No |
| TP-031-010-05 | Cleanup matrix unit | `unit` | `tests/stress/readiness/canary_test.go::TestGoStressHarness_FailurePathsCleanCaptureFiles` | SCN-031-010-006 | Every injected failure uses an isolated TMPDIR and leaves it empty | `./smackerel.sh test unit --go` | No |
| TP-031-010-06 | Existing classifier regression | `unit` | `tests/stress/readiness/canary_test.go::TestGoStressHarness_ZeroSkipZeroRuntimeWarningContract` | SCN-031-010-002, SCN-031-010-003 | Existing clean, skip, logfmt warning, timestamp warning, benign Go warning, and canary-skip cases remain protected | `./smackerel.sh test unit --go` | No |
| TP-031-010-07 | Existing status regression | `unit` | `tests/stress/readiness/canary_test.go::TestGoStressHarness_WorkloadFailurePropagatesAfterCanary` | SCN-031-010-001, SCN-031-010-006 | Readiness ordering and workload exit-status propagation remain intact | `./smackerel.sh test unit --go` | No |
| TP-031-010-08 | Live stress regression | `stress` | `scripts/runtime/go-stress.sh` and `tests/stress/readiness/live_canary_test.go` | SCN-031-010-001 through SCN-031-010-006 | Full disposable-stack stress runner preserves readiness, selection, workload, classification, and cleanup behavior | `./smackerel.sh test stress` | Yes |

### Definition of Done

#### Core Outcomes

- [ ] Root cause is confirmed by red-stage execution and remains consistent with `design.md`.
  - Evidence target: `report.md#red-stage-reproduction`
- [ ] The selector contract preserves distinct match, no-match, and discovery-failure outcomes without sentinel collision.
  - Evidence target: `report.md#code-diff-evidence`
- [ ] The output classifier recognizes direct JSON WARN records, excludes quoted warning literals, and fails on matcher errors.
  - Evidence target: `report.md#code-diff-evidence`
- [ ] Capture failure ordering and cleanup hold on every changed return path.
  - Evidence target: `report.md#code-diff-evidence`
- [ ] Change Boundary is respected and zero excluded file families are changed by this bug.
  - Evidence target: `report.md#change-boundary-evidence`

#### Test Evidence Items

- [ ] TP-031-010-01 (SCN-031-010-001) fails before the fix and passes after the fix with the exact discovery status and diagnostic, without reporting a selector miss or zero matching stress packages.
  - Evidence target: `report.md#tp-031-010-01-list-failure`
- [ ] TP-031-010-02 fails before the fix and passes after the fix for both JSON warnings and quoted literals.
  - Evidence target: `report.md#tp-031-010-02-classifier-matrix`
- [ ] TP-031-010-03 fails before the fix and passes after the fix without converting grep failure to clean output.
  - Evidence target: `report.md#tp-031-010-03-classifier-failure`
- [ ] TP-031-010-04 fails before the fix and passes after the fix with capture failure visible.
  - Evidence target: `report.md#tp-031-010-04-capture-failure`
- [ ] TP-031-010-05 proves isolated cleanup after every adversarial failure path.
  - Evidence target: `report.md#tp-031-010-05-cleanup-matrix`
- [ ] TP-031-010-06 passes without weakening the existing output-contract cases.
  - Evidence target: `report.md#tp-031-010-06-classifier-regression`
- [ ] TP-031-010-07 passes and preserves readiness ordering plus workload status propagation.
  - Evidence target: `report.md#tp-031-010-07-status-regression`
- [ ] TP-031-010-08 passes against the disposable live stress stack after the active E2E lane is idle.
  - Evidence target: `report.md#tp-031-010-08-live-stress`

#### Build Quality Gate

- [ ] Repository check, format check, lint, artifact lint, traceability, regression-quality guard, and exact changed-path audit pass with zero warnings; required documentation remains accurate; no Docker command overlaps an active E2E lane.
  - Evidence target: `report.md#build-quality-gate`