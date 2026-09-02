# Bug Specification: BUG-031-010 Stress Harness Discovery and Classification Fail-Open

## Problem

The stress harness has fail-open paths in selector discovery and output classification. A `go test -list` compile or discovery failure can be reported as a selector miss, a classifier command failure can be reported as clean output, JSON warning records are not recognized, and quoted warning-shaped text can be rejected even though it is not a runtime warning.

## Outcome Contract

**Intent:** Make stress-test selection and output classification truthful under success, no-match, warning, tool-failure, and cleanup conditions.

**Success Signal:** The adversarial tests in `tests/stress/readiness/canary_test.go` fail against the current harness, pass after the bounded repair, preserve exact discovery failure status, classify direct JSON warnings, ignore quoted benign literals, reject grep and capture failures, and prove capture cleanup. The repository unit test, live stress gate, and static artifact gates then pass when the E2E lane is available.

**Hard Constraints:**

- Only `scripts/runtime/go-stress.sh`, `tests/stress/readiness/canary_test.go`, and this bug packet may change.
- BUG-031-005 and BUG-031-006 remain terminal and unmodified.
- A selector miss must remain distinct from a `go test -list` failure.
- No classifier error may be interpreted as `clean`.
- The classifier must not add a runtime dependency or use a fallback result.
- Existing skip, logfmt warning, timestamp warning, workload-status, and cleanup behavior must remain protected.
- No Docker command runs while another E2E lane is active.

**Failure Condition:** The bug remains unresolved if discovery failure can still reach the zero-match path, if a direct JSON warning is accepted, if quoted benign text is rejected, if grep or capture failure is accepted, if temporary captures remain, or if any excluded file changes.

## Release Train

- Target train: `mvp`
- Flags introduced: none
- Other trains: no capability is enabled by this packet. This is an invariant repair on trunk, not an optional feature, and it introduces no default-on flag.

## Product Principle Alignment

- **Principle 8 - Trust Through Transparency:** A stress result must reflect the command that actually ran. Discovery failure, runtime warning, selector miss, and classifier failure remain distinguishable in exit status and diagnostics.
- This packet changes engineering validation behavior only. It does not claim a new shipped user capability or a roadmap example as delivered behavior.

## Functional Requirements

### FR-010-001 Discovery Status Integrity

The harness must capture the exit status of each `go test -list` invocation explicitly. A non-zero discovery status must terminate the harness and remain distinguishable from a successful list with zero matching tests.

### FR-010-002 Selector Outcome Integrity

The package-selection contract must represent match, no-match, and discovery failure as distinct states. The package loop must continue only for a successful no-match result.

### FR-010-003 JSON Warning Detection

The classifier must reject a direct JSON log record whose unescaped `level` field has the exact value `WARN`, including the compact form `{"level":"WARN"}` and a record where another top-level field precedes `level`.

### FR-010-004 Quoted Literal Exclusion

The classifier must accept benign lines that quote `level=WARN`, a timestamp followed by `WARN`, or escaped JSON warning text as message content rather than a direct log-level field.

### FR-010-005 Classifier Failure Integrity

A grep execution error must terminate classification and the harness with a non-zero result. Exit 1 remains the only grep no-match result.

### FR-010-006 Capture Failure Integrity

Failure to allocate or write the output capture must terminate the harness with an actionable diagnostic. Classification must not run against incomplete output.

### FR-010-007 Cleanup

Every allocated output capture must be removed on clean output, forbidden output, workload failure, classifier failure, capture failure, signal-driven exit, and normal function return.

### FR-010-008 Regression Preservation

The repair must preserve the contracts delivered by BUG-031-005 and BUG-031-006: readiness runs before workloads, workload exit status remains visible, selector misses skip only unmatched packages, skip output is forbidden, and existing logfmt and timestamp warning forms remain forbidden.

## Gherkin Scenarios

### SCN-031-010-001 List Failure Propagates

```gherkin
Given the readiness canary has passed
And go test -list for a workload package emits a discovery diagnostic and exits non-zero
When go-stress evaluates packages for a run selector
Then the harness exits with the discovery status
And it does not report that package as a selector miss
And it does not report zero matching stress packages as the root failure
```

### SCN-031-010-002 JSON Warning Is Rejected

```gherkin
Given a selected workload exits zero
And its output contains a direct JSON record with level WARN
When go-stress classifies the captured output
Then the harness exits non-zero
And the diagnostic identifies a runtime warning class
```

### SCN-031-010-003 Quoted Warning Literal Is Benign

```gherkin
Given a selected workload exits zero
And its output contains warning-shaped text only inside a quoted message value
When go-stress classifies the captured output
Then the quoted literal does not match a runtime warning class
And the clean workload result is preserved
```

### SCN-031-010-004 Grep Failure Fails Loud

```gherkin
Given a selected workload exits zero
And the classifier grep command exits with an execution error
When go-stress classifies the captured output
Then the harness exits non-zero
And it reports that output classification could not be trusted
```

### SCN-031-010-005 Capture Failure Fails Loud

```gherkin
Given the readiness canary or a selected workload emits output
And the capture writer fails
When go-stress runs the checked package
Then the harness exits non-zero
And it does not classify incomplete output as clean
```

### SCN-031-010-006 Failure Paths Clean Captures

```gherkin
Given a test-owned temporary directory is used for harness captures
When list failure, classifier failure, capture failure, forbidden output, and workload failure are exercised independently
Then every case leaves the capture directory empty
And no case relies on a later test to remove another case's file
```

## Acceptance Criteria

1. All six scenario contracts have exact adversarial tests in `tests/stress/readiness/canary_test.go`.
2. The red stage demonstrates the current failure for each changed behavior before the harness repair.
3. The green stage uses the same tests without weakening assertions.
4. Direct JSON warnings and quoted warning literals are both represented in the same classification matrix.
5. Fake `go`, `grep`, and capture-writer executables use distinctive exit codes so status substitution is observable.
6. Cleanup is asserted after every injected failure path.
7. The final diff contains no path outside the declared change boundary.

## Out of Scope

- Changing live-stack topology, readiness probes, package workloads, or stress duration limits
- Changing `go list` package enumeration
- Editing BUG-031-005 or BUG-031-006 state or evidence
- Changing Smackerel configuration, Compose files, product code, documentation outside this packet, or framework-managed files
- Running Docker or a live stress lane while another E2E lane is active