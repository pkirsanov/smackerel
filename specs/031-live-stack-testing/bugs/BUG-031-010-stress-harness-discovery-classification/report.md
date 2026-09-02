# Report: BUG-031-010 Stress Harness Discovery and Classification Fail-Open

## Summary

The packet retains its completed root-cause and scenario-first planning. The implementation phase has now added the five planned adversarial tests and repaired the bounded stress-harness discovery, classification, and cleanup paths in `scripts/runtime/go-stress.sh`. No project test or Docker command was run in this implementation invocation.

## Completion Statement

The bounded source and test edits are present, and non-Docker static checks are clean. Runtime red-stage evidence was unavailable under the operator's static-only constraint, so green-stage unit tests, the live stress regression, validation, audit, and certification are not claimed. The bug and Scope 1 remain `in_progress`.

## Root Cause Evidence

**Executed:** NO

**Command:** Not applicable; source was inspected with repository read tools.

**Claim Source:** interpreted

Observed controlling paths:

- `go_stress_package_has_selected_tests` consumes `go test -list` through process substitution and has no producer-status branch.
- Its caller continues on every non-zero helper result, so no-match and discovery failure share one path.
- `go_stress_classify_output` runs several `grep -Eq` predicates but does not distinguish grep exit 1 from grep execution failure.
- Existing warning expressions cover logfmt and timestamp forms, not direct JSON `level` fields, and they are not anchored tightly enough to exclude quoted examples.
- Existing `canary_test.go` fixtures cover workload status, skip output, logfmt warning, timestamp warning, benign Go warning, and cleanup for those paths, but not the required list/JSON/quoted/grep/capture adversarial matrix.

### Uncertainty Declaration

The shell control-flow defects are grounded in the current source text. Their observable command output and exact exit codes remain unverified until the implementation owner executes the red-stage tests. This packet does not restate static reasoning as runtime evidence.

## Related Terminal Bug Evidence

**Executed:** NO

**Claim Source:** interpreted

- BUG-031-005 `state.json` records top-level and certification status `done`; it delivered readiness-before-workload, checked package output capture, warning/skip rejection, workload status propagation, and cleanup.
- BUG-031-006 `state.json` records top-level and certification status `done`; it delivered per-package `--run` selection and added the current harness fixture coverage.
- Neither terminal state file was modified.

## Red-Stage Reproduction

**Executed:** NO

**Command:** `./smackerel.sh test unit --go`

**Exit Code:** not observed

**Claim Source:** not-run

The red stage must be executed after the five adversarial tests are added and before `scripts/runtime/go-stress.sh` changes. No project test was run during planning.

## Code Diff Evidence

**Claim Source:** interpreted

The implementation delta is limited to:

- `scripts/runtime/go-stress.sh`
- `tests/stress/readiness/canary_test.go`

The shell harness now captures `go test -list` output and status explicitly, keeps selected/not-selected state outside the status channel, exits with the exact discovery status, uses anchored warning classes including direct JSON `level: WARN`, distinguishes grep no-match from grep execution failure, and cleans both workload and discovery captures through explicit returns plus the `EXIT` trap.

The Go canary test now contains the five planned test functions. Their fixtures cover list exit 42 after a prior package passes, compact and prefixed JSON WARN records, quoted logfmt/timestamp/escaped-JSON literals, grep exit 44, tee exit 45, and isolated capture cleanup. These are source-shape observations, not passing-test claims.

## Change Boundary Evidence

**Executed:** NO

**Claim Source:** not-run

The packet declares the allowed and excluded paths in `design.md`, `scopes.md`, and `state.json`. A post-implementation `git status --short` audit is required because both allowed non-packet files were already dirty before this packet was created.

## Test Evidence

No unit, integration, E2E, stress, or load test was run. Docker was not invoked because the user reported an active E2E lane and constrained this planning pass to artifact/static gates.

### TP-031-010-01 List Failure

**Claim Source:** not-run

### TP-031-010-02 Classifier Matrix

**Claim Source:** not-run

### TP-031-010-03 Classifier Failure

**Claim Source:** not-run

### TP-031-010-04 Capture Failure

**Claim Source:** not-run

### TP-031-010-05 Cleanup Matrix

**Claim Source:** not-run

### TP-031-010-06 Classifier Regression

**Claim Source:** not-run

### TP-031-010-07 Status Regression

**Claim Source:** not-run

### TP-031-010-08 Live Stress

**Claim Source:** not-run

## Remaining Executable Validation

- TP-031-010-01 through TP-031-010-07 remain unexecuted. Their registered command is `./smackerel.sh test unit --go`.
- TP-031-010-08 remains unexecuted. Its registered live-system command is `./smackerel.sh test stress` and it requires an idle E2E lane.
- The required pre-fix red-stage unit execution was not captured because this invocation was constrained to non-Docker static checks after the adversarial tests were authored.
- Project check, format check, lint, and broader regression execution remain unexecuted; no result is claimed for those surfaces.

## Validation Evidence

### Non-Docker Static Implementation Checks

**Executed:** YES (in current session)

**Command:**

```bash
set -euo pipefail
printf '%s\n' '[1/6] shell parse'
timeout 60 bash -n scripts/runtime/go-stress.sh
printf '%s\n' '[2/6] diff whitespace'
timeout 60 git diff --check -- scripts/runtime/go-stress.sh tests/stress/readiness/canary_test.go
printf '%s\n' '[3/6] legacy discovery path absent'
if timeout 60 grep -nF 'done < <(go test -tags stress -list' scripts/runtime/go-stress.sh; then exit 1; else grep_rc=$?; [[ "$grep_rc" -eq 1 ]] || exit "$grep_rc"; printf '%s\n' 'OK: process-substitution discovery absent'; fi
printf '%s\n' '[4/6] legacy failure-to-skip caller absent'
if timeout 60 grep -nF 'if ! go_stress_package_has_selected_tests' scripts/runtime/go-stress.sh; then exit 1; else grep_rc=$?; [[ "$grep_rc" -eq 1 ]] || exit "$grep_rc"; printf '%s\n' 'OK: failure-to-skip caller absent'; fi
printf '%s\n' '[5/6] fail-loud source markers present'
timeout 60 grep -nE 'output classification could not be trusted|runtime-warning-json|test discovery failed \(go test -list exit' scripts/runtime/go-stress.sh
printf '%s\n' '[6/6] adversarial tests present'
timeout 60 grep -nE '^func TestGoStressHarness_(ListFailurePropagates|JSONWarningAndQuotedLiteralClassification|ClassifierCommandFailurePropagates|CaptureFailurePropagates|FailurePathsCleanCaptureFiles)\(' tests/stress/readiness/canary_test.go
printf '%s\n' 'RESULT: non-Docker static checks passed'
```

**Exit Code:** 0

**Claim Source:** executed

**Output:**

```text
[1/6] shell parse
[2/6] diff whitespace
[3/6] legacy discovery path absent
OK: process-substitution discovery absent
[4/6] legacy failure-to-skip caller absent
OK: failure-to-skip caller absent
[5/6] fail-loud source markers present
16: echo "ERROR: go-stress: output classification could not be trusted (grep exit $grep_rc)." >&2
23: echo "ERROR: go-stress: output classification could not be trusted because the capture is unreadable." >&2
39: "runtime-warning-json"
148: runtime-warning-json)
149: echo "ERROR: go-stress: package $package_path emitted forbidden output class runtime-warning-json (direct JSON level WARN); fix the warning-producing runtime path before stress can pass." >&2
239: echo "ERROR: go-stress: package $package_path test discovery failed (go test -list exit $list_rc); selector result is unavailable." >&2
[6/6] adversarial tests present
217:func TestGoStressHarness_ListFailurePropagates(test *testing.T) {
298:func TestGoStressHarness_JSONWarningAndQuotedLiteralClassification(test *testing.T) {
355:func TestGoStressHarness_ClassifierCommandFailurePropagates(test *testing.T) {
397:func TestGoStressHarness_CaptureFailurePropagates(test *testing.T) {
429:func TestGoStressHarness_FailurePathsCleanCaptureFiles(test *testing.T) {
RESULT: non-Docker static checks passed
```

VS Code diagnostics reported no errors in either implementation file. This static evidence does not substitute for TP-031-010-01 through TP-031-010-08.

### Post-Edit Artifact Lint

**Executed:** YES (in current session)

**Command:** `timeout 660 bash .github/bubbles/scripts/evidence-capture.sh --label 'BUG-031-010 post-edit artifact lint' -- timeout 600 bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-010-stress-harness-discovery-classification`

**Exit Code:** 0

**Claim Source:** executed

**Bounded capture:** 41 lines, SHA-256 `d0fcc3d00860e2793d6f53a8434292f1a505ecf38ce4456d8d8db5bf25097ada`

```text
Required artifact exists: spec.md
Required artifact exists: design.md
Required artifact exists: uservalidation.md
Required artifact exists: state.json
Required artifact exists: scopes.md
Required artifact exists: report.md
Found DoD section in scopes.md
Found Checklist section in uservalidation.md
Detected state.json status: in_progress
Detected state.json workflowMode: bugfix-fastlane
Top-level status matches certification.status
All checked DoD items in scopes.md have evidence blocks
No unfilled evidence template placeholders in scopes.md
No unfilled evidence template placeholders in report.md
No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
```

### Post-Edit Traceability Guard

**Executed:** YES (in current session)

**Command:** `timeout 660 bash .github/bubbles/scripts/evidence-capture.sh --label 'BUG-031-010 post-edit traceability' -- timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing/bugs/BUG-031-010-stress-harness-discovery-classification`

**Exit Code:** 0

**Claim Source:** executed

**Bounded capture:** 69 lines, SHA-256 `3ca2ec4d4830b09c534db343f50e96bf5386d9eee990cd4dcbbc9c096a524ce8`

```text
scenario-manifest.json covers 6 scenario contract(s)
scenario-manifest.json records evidenceRefs for all 6 scenario contract(s)
All linked tests from scenario-manifest.json exist
Scope 1 scenario mapped to Test Plan row: SCN-031-010-001 list failure propagates
Scope 1 scenario mapped to Test Plan row: SCN-031-010-002 JSON warning is rejected
Scope 1 scenario mapped to Test Plan row: SCN-031-010-003 quoted warning literal is benign
Scope 1 scenario mapped to Test Plan row: SCN-031-010-004 grep failure fails loud
Scope 1 scenario mapped to Test Plan row: SCN-031-010-005 capture failure fails loud
Scope 1 scenario mapped to Test Plan row: SCN-031-010-006 failure paths clean captures
Scenarios checked: 6
Scenario-to-row mappings: 6
Concrete test file references: 6
Report evidence references: 6
DoD fidelity scenarios: 6 (mapped: 6, unmapped: 0)
RESULT: PASSED (0 warnings)
```

### Bugfix Regression Quality Guard

**Executed:** YES (in current session)

**Command:** `timeout 600 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/stress/readiness/canary_test.go`

**Exit Code:** 0

**Claim Source:** executed

```text
============================================================
	BUBBLES REGRESSION QUALITY GUARD
	Repo: ~/smackerel
	Timestamp: 2026-09-02T02:24:08Z
	Bugfix mode: true
============================================================

Scanning tests/stress/readiness/canary_test.go
Adversarial signal detected in tests/stress/readiness/canary_test.go

============================================================
	REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
	Files scanned: 1
	Files with adversarial signals: 1
============================================================
```

### Artifact Lint

**Executed:** YES

**Command:** `timeout 600 bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-010-stress-harness-discovery-classification`

**Exit Code:** 0

**Claim Source:** executed

**Bounded capture:** 41 lines, SHA-256 `d0fcc3d00860e2793d6f53a8434292f1a505ecf38ce4456d8d8db5bf25097ada`

```text
Required artifact exists: spec.md
Required artifact exists: design.md
Required artifact exists: uservalidation.md
Required artifact exists: state.json
Required artifact exists: scopes.md
Required artifact exists: report.md
Found DoD section in scopes.md
Found Checklist section in uservalidation.md
Detected state.json status: in_progress
Detected state.json workflowMode: bugfix-fastlane
Top-level status matches certification.status
No unfilled evidence template placeholders in scopes.md
No unfilled evidence template placeholders in report.md
Artifact lint PASSED.
```

### Traceability Guard

**Executed:** YES

**Command:** `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing/bugs/BUG-031-010-stress-harness-discovery-classification`

**Exit Code:** 0

**Claim Source:** executed

**Bounded capture:** 69 lines, SHA-256 `e21ebaaa6cc7437ae9b1da59c7dccc7bf144a71266a53bf1340fe6c78c0751e9`

```text
scenario-manifest.json covers 6 scenario contract(s)
scenario-manifest.json records evidenceRefs for all 6 scenario contract(s)
All linked tests from scenario-manifest.json exist
SCN-031-010-001 list failure propagates: mapped to Test Plan and DoD
SCN-031-010-002 JSON warning is rejected: mapped to Test Plan and DoD
SCN-031-010-003 quoted warning literal is benign: mapped to Test Plan and DoD
SCN-031-010-004 grep failure fails loud: mapped to Test Plan and DoD
SCN-031-010-005 capture failure fails loud: mapped to Test Plan and DoD
SCN-031-010-006 failure paths clean captures: mapped to Test Plan and DoD
Scenarios checked: 6
Scenario-to-row mappings: 6
DoD fidelity scenarios: 6 (mapped: 6, unmapped: 0)
RESULT: PASSED (0 warnings)
```

No validate-owned certification is claimed by these planning gates.

## NATS Disconnect Warning Classification Diagnostic

### Scope And Ownership Disposition

**Claim Source:** executed and interpreted

The packet's outcome contract covers truthful runtime-warning classification, and the current NATS change removes a warning only when the disconnect callback receives `err == nil`. The paired error case still requires exactly one warning. This is behaviorally related to the stress classifier because direct logfmt warning records remain a forbidden stress output class.

The current planning boundary does not admit the implementation, however. `spec.md` limits changes to the stress harness, its readiness canary, and this packet. `design.md` explicitly excludes all `internal/**` paths. The strict work-boundary resolver returned `route-same-repo` for both `internal/nats/client.go` and `internal/nats/client_test.go`, while returning `in-boundary` for this report. Planning expansion is therefore required before the NATS change can be attributed to BUG-031-010 or satisfy any existing DoD item.

No full stress command was run in this invocation. Previously supplied stress evidence is not copied or restated here as current-session execution evidence. No DoD checkbox, scope status, top-level status, execution status, or certification field is changed by this diagnostic.

### Test Mechanism And Negative Controls

**Claim Source:** interpreted

- `TestNATSDisconnectNilDoesNotWarn` installs a warning-level `slog` text handler, invokes `logNATSDisconnect(nil)`, and requires an empty capture. Its negative control is the pre-repair behavior, where every disconnect callback emitted a warning even when no error existed.
- `TestNATSDisconnectErrorWarnsOnce` invokes the same production helper with a non-nil transport error and requires one `level=WARN` record containing both the stable message and safe error text. Its negative control prevents the nil-noise repair from suppressing real disconnect warnings.
- The focused selector exercises both cases through the canonical repository unit runner with `-count=1`; it does not prove the full live stress path and does not replace TP-031-010-08.

### Linked-Test Resolution Red And Green

The mandatory linked-test resolver first found three ambiguous test IDs because the cleanup matrix directly reused exported test functions by name.

**Executed:** YES (in current session)

**Command:** `timeout 360 bash .github/bubbles/scripts/evidence-capture.sh --label 'BUG-031-010 linked test resolution before NATS diagnostic' -- timeout 300 bash .github/bubbles/scripts/scenario-test-resolve.sh specs/031-live-stack-testing/bugs/BUG-031-010-stress-harness-discovery-classification --repo-root ~/smackerel`

**Exit Code:** 1

**Claim Source:** executed

**Bounded capture:** 9 lines, SHA-256 `de7cbb101b1a0e9933b39ac97df336ecaa2df15f2c77d9e78d2a5a15aab5d6fe`

```text
scenario-test-resolve: FAIL — linked tests that do not resolve (Gate G057)
	AMBIGUOUS-TITLE: SCN-031-010-001 -> tests/stress/readiness/canary_test.go#TestGoStressHarness_ListFailurePropagates
		the title appears 2 times; a reference must resolve to exactly one
	AMBIGUOUS-TITLE: SCN-031-010-004 -> tests/stress/readiness/canary_test.go#TestGoStressHarness_ClassifierCommandFailurePropagates
		the title appears 2 times; a reference must resolve to exactly one
	AMBIGUOUS-TITLE: SCN-031-010-005 -> tests/stress/readiness/canary_test.go#TestGoStressHarness_CaptureFailurePropagates
		the title appears 2 times; a reference must resolve to exactly one
scenario-test-resolve: 3 unresolved reference(s) of 6 checked.
```

The in-boundary test repair leaves each exported test as the unique manifest target, moves its body to a private helper, and has the cleanup matrix call that helper. The same resolver then passed.

**Executed:** YES (in current session)

**Command:** `timeout 360 bash .github/bubbles/scripts/evidence-capture.sh --label 'BUG-031-010 linked test resolution after unique test helper repair' -- timeout 300 bash .github/bubbles/scripts/scenario-test-resolve.sh specs/031-live-stack-testing/bugs/BUG-031-010-stress-harness-discovery-classification --repo-root ~/smackerel`

**Exit Code:** 0

**Claim Source:** executed

**Bounded capture:** 1 line, SHA-256 `11c10d5a7034e0e16b3d9233f2caec9d63cb9c2978ad6f524053d658918a1b1a`

```text
[scenario-test-resolve] OK — 6 reference(s) resolved via literal-scan; 6 category comparison(s) not applicable (no test-discovery adapter declared)
```

### Focused Canary Refactor Unit Evidence

**Executed:** YES (in current session)

**Command:** `timeout 720 bash .github/bubbles/scripts/evidence-capture.sh --label 'BUG-031-010 unique linked test helper focused unit' -- timeout 600 ./smackerel.sh test unit --go --go-run '^(TestGoStressHarness_(ListFailurePropagates|ClassifierCommandFailurePropagates|CaptureFailurePropagates|FailurePathsCleanCaptureFiles))$'`

**Exit Code:** 0

**Claim Source:** executed

**Bounded capture:** 219 lines, SHA-256 `6d1189d7fbb13dbdcbe5ec6ea6f81e88cc2f990e16512843d39ebada0a58564b`

```text
--- first 20 ---
oom-preflight: OK — 39644 MB available (need 6000 MB; swap used 472 MB).
disk-preflight: OK — C: 40 GB free (need 40 GB), WSL / 483 GB free (need 25 GB).
++ dirname /workspace/scripts/runtime/go-unit.sh
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
+ echo '[go-unit] envsubst missing — installing gettext-base'
[go-unit] envsubst missing — installing gettext-base
+ apt-get update -qq
+ apt-get install -y --no-install-recommends gettext-base
Reading package lists...
Building dependency tree...
Reading state information...
The following NEW packages will be installed:
	gettext-base
0 upgraded, 1 newly installed, 0 to remove and 21 not upgraded.
Need to get 160 kB of archives.
After this operation, 660 kB of additional disk space will be used.
Get:1 http://deb.debian.org/debian bookworm/main amd64 gettext-base amd64 0.21-12 [160 kB]
--- omitted 179 line(s); sha256 above covers the full output ---
--- last 20 ---
ok      github.com/smackerel/smackerel/internal/testsupport/jssource    0.009s [no tests to run]
ok      github.com/smackerel/smackerel/internal/topics  0.014s [no tests to run]
ok      github.com/smackerel/smackerel/internal/web     0.157s [no tests to run]
ok      github.com/smackerel/smackerel/internal/web/admin       0.007s [no tests to run]
ok      github.com/smackerel/smackerel/internal/web/icons       0.017s [no tests to run]
ok      github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter     0.068s [no tests to run]
ok      github.com/smackerel/smackerel/tests/e2e/agent  0.008s [no tests to run]
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.016s [no tests to run]
ok      github.com/smackerel/smackerel/tests/eval/assistant     0.014s [no tests to run]
ok      github.com/smackerel/smackerel/tests/integration        0.016s [no tests to run]
?       github.com/smackerel/smackerel/tests/integration/agent/routerwarmup    [no test files]
?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
?       github.com/smackerel/smackerel/tests/integration/nslock [no test files]
ok      github.com/smackerel/smackerel/tests/observability      0.008s [no tests to run]
ok      github.com/smackerel/smackerel/tests/stress/readiness   0.537s
ok      github.com/smackerel/smackerel/tests/unit/clients       0.008s [no tests to run]
?       github.com/smackerel/smackerel/web/pwa  [no test files]
ok      github.com/smackerel/smackerel/web/pwa/tests    0.141s [no tests to run]
[go-unit] go test ./... finished OK
+ echo '[go-unit] go test ./... finished OK'
```

### Focused NATS Unit Evidence

**Executed:** YES (in current session)

**Command:** `timeout 720 bash .github/bubbles/scripts/evidence-capture.sh --label 'BUG-031-010 NATS disconnect warning classification focused unit' -- timeout 600 ./smackerel.sh test unit --go --go-run '^(TestNATSDisconnectNilDoesNotWarn|TestNATSDisconnectErrorWarnsOnce)$'`

**Exit Code:** 0

**Claim Source:** executed

**Bounded capture:** 219 lines, SHA-256 `bed8655cd834b3e2bacd8347757b927817ee15b4f009279f4753a22af0055ec3`

```text
--- first 20 ---
oom-preflight: OK — 39635 MB available (need 6000 MB; swap used 472 MB).
disk-preflight: OK — C: 40 GB free (need 40 GB), WSL / 483 GB free (need 25 GB).
++ dirname /workspace/scripts/runtime/go-unit.sh
[go-unit] envsubst missing — installing gettext-base
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
+ echo '[go-unit] envsubst missing — installing gettext-base'
+ apt-get update -qq
+ apt-get install -y --no-install-recommends gettext-base
Reading package lists...
Building dependency tree...
Reading state information...
The following NEW packages will be installed:
	gettext-base
0 upgraded, 1 newly installed, 0 to remove and 21 not upgraded.
Need to get 160 kB of archives.
After this operation, 660 kB of additional disk space will be used.
Get:1 http://deb.debian.org/debian bookworm/main amd64 gettext-base amd64 0.21-12 [160 kB]
--- omitted 179 line(s); sha256 above covers the full output ---
--- last 20 ---
ok      github.com/smackerel/smackerel/internal/testsupport/jssource    0.018s [no tests to run]
ok      github.com/smackerel/smackerel/internal/topics  0.011s [no tests to run]
ok      github.com/smackerel/smackerel/internal/web     0.217s [no tests to run]
ok      github.com/smackerel/smackerel/internal/web/admin       0.020s [no tests to run]
ok      github.com/smackerel/smackerel/internal/web/icons       0.006s [no tests to run]
ok      github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter     0.066s [no tests to run]
ok      github.com/smackerel/smackerel/tests/e2e/agent  0.018s [no tests to run]
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.008s [no tests to run]
ok      github.com/smackerel/smackerel/tests/eval/assistant     0.014s [no tests to run]
ok      github.com/smackerel/smackerel/tests/integration        0.031s [no tests to run]
?       github.com/smackerel/smackerel/tests/integration/agent/routerwarmup    [no test files]
?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
?       github.com/smackerel/smackerel/tests/integration/nslock [no test files]
ok      github.com/smackerel/smackerel/tests/observability      0.010s [no tests to run]
ok      github.com/smackerel/smackerel/tests/stress/readiness   0.006s [no tests to run]
ok      github.com/smackerel/smackerel/tests/unit/clients       0.009s [no tests to run]
?       github.com/smackerel/smackerel/web/pwa  [no test files]
ok      github.com/smackerel/smackerel/web/pwa/tests    0.155s [no tests to run]
[go-unit] go test ./... finished OK
+ echo '[go-unit] go test ./... finished OK'
```

### Current Regression Quality Evidence

**Executed:** YES (in current session)

**Command:** `timeout 180 bash .github/bubbles/scripts/evidence-capture.sh --label 'BUG-031-010 NATS and canary bugfix regression quality' -- timeout 120 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/nats/client_test.go tests/stress/readiness/canary_test.go`

**Exit Code:** 0

**Claim Source:** executed

**Bounded capture:** 17 lines, SHA-256 `a8a829561bd6d479c41ee79c3157c1d70a9d3e641209d0ad933f3938032fa5c5`

```text
============================================================
	BUBBLES REGRESSION QUALITY GUARD
	Repo: ~/smackerel
	Timestamp: 2026-09-02T04:10:14Z
	Bugfix mode: true
============================================================

ℹ️  Scanning internal/nats/client_test.go
✅ Adversarial signal detected in internal/nats/client_test.go
ℹ️  Scanning tests/stress/readiness/canary_test.go
✅ Adversarial signal detected in tests/stress/readiness/canary_test.go

============================================================
	REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
	Files scanned: 2
	Files with adversarial signals: 2
============================================================
```

Editor diagnostics reported no errors in `internal/nats/client.go`, `internal/nats/client_test.go`, or `tests/stress/readiness/canary_test.go`. A skip-marker scan over the two test files returned no matches. These checks do not override the `route-same-repo` boundary disposition.

## Audit Evidence

**Executed:** NO

**Command:** Not run

**Phase Agent:** `bubbles.audit`

**Claim Source:** not-run

## Chaos Evidence

**Executed:** NO

**Command:** Not run

**Phase Agent:** `bubbles.chaos`

**Claim Source:** not-run

## Build Quality Gate

The bounded non-Docker shell parse, diff whitespace check, source-shape assertions, editor diagnostics, post-edit artifact lint, post-edit traceability guard, and bugfix regression-quality guard are clean. Project check, format, lint, unit, integration, E2E, stress, and Docker-backed validation remain unexecuted and unchecked in `scopes.md`.

## Finding Accounting

| Finding | Disposition | Owner |
|---------|-------------|-------|
| BUG-031-010-F01: `go test -list` status can become selector no-match | Source repair and TP-031-010-01 fixture authored; executable red/green verification pending | `bubbles.test` |
| BUG-031-010-F02: classifier misses JSON WARN, can match quoted text, and ignores grep errors | Source repair and TP-031-010-02/03 fixtures authored; executable green verification pending | `bubbles.test` |
| BUG-031-010-F03: capture failure and cleanup adversarial coverage is incomplete | TP-031-010-04/05 fixtures authored and dual-capture cleanup wired; executable verification pending | `bubbles.test` |

## Invocation Audit

No subagent was invoked. The available runtime exposed no subagent-dispatch tool, so this top-level planning pass created the packet directly and routes the implementation phase through the result envelope.