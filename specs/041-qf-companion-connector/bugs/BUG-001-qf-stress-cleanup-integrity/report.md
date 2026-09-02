# Execution Reports: [BUG-001] QF Stress Cleanup Integrity

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope: Deterministic QF Stress Fixture Teardown - 2026-09-02 01:52 UTC

### Summary

- Created one focused artifact-only bug packet.
- Inspected the two current dirty stress-test paths and their shared cleanup helper.
- Defined four scenarios, eight exact test-plan rows, fail-loud set-based cleanup, current-run timestamp policy, and a parent/child zero-residue assertion.
- Did not modify source or test files and did not run Docker or product tests.

### Code Diff Evidence

**Executed:** YES
**Command:** `timeout 60 git status --short --untracked-files=all -- tests/stress/qf_decision_event_replay_test.go tests/stress/qf_decisions_sync_stress_test.go specs/041-qf-companion-connector && timeout 60 git --no-pager diff -- tests/stress/qf_decision_event_replay_test.go tests/stress/qf_decisions_sync_stress_test.go`
**Claim Source:** executed

```text
 M tests/stress/qf_decision_event_replay_test.go
 M tests/stress/qf_decisions_sync_stress_test.go
diff --git a/tests/stress/qf_decision_event_replay_test.go b/tests/stress/qf_decision_event_replay_test.go
index 1bb74f49..f936c5b1 100644
--- a/tests/stress/qf_decision_event_replay_test.go
+++ b/tests/stress/qf_decision_event_replay_test.go
@@ -183,7 +183,11 @@ func TestQFDecisionsFreshnessSLAP95IngestRender(t *testing.T) {
-                       _ = json.NewEncoder(w).Encode(stressEnvelope(packetID, "trace-"+packetID))
+                       _ = json.NewEncoder(w).Encode(stressEnvelope(
+                               packetID,
+                               "trace-"+packetID,
+                               time.Now().UTC().Format(time.RFC3339Nano),
+                       ))
diff --git a/tests/stress/qf_decisions_sync_stress_test.go b/tests/stress/qf_decisions_sync_stress_test.go
index bbd38586..1586232c 100644
--- a/tests/stress/qf_decisions_sync_stress_test.go
+++ b/tests/stress/qf_decisions_sync_stress_test.go
@@ -57,13 +57,13 @@ func TestQFDecisionsSyncStress_RepeatedCursorPagesDoNotDuplicatePacketIdentity(t
-       defer pool.Close()
+       t.Cleanup(pool.Close)
-       defer natsClient.Close()
+       t.Cleanup(natsClient.Close)
```

This evidence proves only the inspected dirty paths and diff shape. It does not prove the bug at runtime or prove a fix.

### Completion Statement

The requested bug documentation packet is initialized with substantive requirements, design, scope, scenario registry, test handoff, and in-progress control state. Runtime delivery is not complete: the RED test, implementation, GREEN test, broader suites, and validation have not run.

### Test Evidence

#### Bug Reproduction - Before Fix

**Executed:** NO
**Command:** Not run; Docker and product-test execution were explicitly excluded from this artifact-only request.
**Phase Agent:** `bubbles.test` not invoked
**Claim Source:** not-run

The future RED command is the repo-owned `./smackerel.sh test stress` flow targeting `TestQFDecisionsStressCleanupChildReturnsZeroOwnedRows`. No exit code or pass/fail result is recorded because the command did not execute.

#### Post-Fix Regression

**Executed:** NO
**Command:** Not run; no implementation exists in this packet.
**Phase Agent:** `bubbles.test` not invoked
**Claim Source:** not-run

#### Adversarial Cleanup Failure

**Executed:** NO
**Command:** Not run; `TestQFDecisionsStressCleanupReturnsErrorForClosedPool` is planned, not authored.
**Phase Agent:** `bubbles.test` not invoked
**Claim Source:** not-run

#### Existing And Broader Regressions

**Executed:** NO
**Command:** Neither `./smackerel.sh test e2e` nor `./smackerel.sh test stress` was run.
**Phase Agent:** `bubbles.test` not invoked
**Claim Source:** not-run

### Validation Evidence

#### Artifact Lint

**Executed:** YES
**Command:** `timeout 300 bash .github/bubbles/scripts/artifact-lint.sh specs/041-qf-companion-connector/bugs/BUG-001-qf-stress-cleanup-integrity`
**Exit Code:** 0
**Phase Agent:** `bubbles.validate` not invoked; this is packet-shape validation only
**Claim Source:** executed

```text
Required artifact exists: spec.md
Required artifact exists: design.md
Required artifact exists: uservalidation.md
Required artifact exists: state.json
Required artifact exists: scopes.md
Required artifact exists: report.md
No forbidden sidecar artifacts present
Found DoD section in scopes.md
scopes.md DoD contains checkbox items
All DoD bullet items use checkbox syntax in scopes.md
Found Checklist section in uservalidation.md
uservalidation checklist contains checkbox entries
All checklist bullet items use checkbox syntax
uservalidation separates automation readiness from human acceptance
Detected state.json status: in_progress
Detected state.json workflowMode: bugfix-fastlane
Top-level status matches certification.status
All checked DoD items in scopes.md have evidence blocks
No unfilled evidence template placeholders in scopes.md
No unfilled evidence template placeholders in report.md
No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

This check validates artifact shape only. Runtime reproduction, test execution, implementation, and validate-owned certification remain not run.

### Audit Evidence

**Executed:** NO
**Command:** Not run during initial packet creation.
**Phase Agent:** `bubbles.audit` not invoked
**Claim Source:** not-run

### Chaos Evidence

**Executed:** NO
**Command:** Not run during initial packet creation.
**Phase Agent:** `bubbles.chaos` not invoked
**Claim Source:** not-run

## Evidence Index

| Claim | Source | Status |
|---|---|---|
| Two exact stress files are dirty | current-session `git status` | Executed |
| Sync dirty diff changes resource close registration and timestamp injection | current-session `git diff` | Executed |
| Freshness tests still use function-level pool/NATS defers | current-session file inspection | Interpreted |
| Cleanup helper uses per-row deletes and `t.Logf` on errors | current-session file inspection | Interpreted |
| Runtime bug reproduced | none | Not run |
| Fix verified | none | Not run |

## Test Implementation - 2026-09-02 02:32 UTC

### Summary

- Replaced the two freshness tests' pool and NATS function defers with ordered `t.Cleanup` registrations while retaining their response-time UTC timestamps.
- Replaced the query-plus-loop/log-only cleanup helper with a bounded transaction that deletes child rows before parent rows using set-based SQL, verifies zero owned rows, and returns contextual begin, delete, verification, rollback, and commit errors to a fail-loud test wrapper.
- Added `TestQFDecisionsStressCleanupChildReturnsZeroOwnedRows`, which lets a child cleanup finish before the parent independently counts owned rows and verifies that an unowned artifact, annotation, edge, and sync-state row all survive.
- Added `TestQFDecisionsStressCleanupReturnsErrorForClosedPool` as the adversarial error-path regression.
- Preserved the pre-existing run-scoped timestamp correction in the sync stress fixture.
- Made missing live PostgreSQL or NATS configuration fail required stress tests instead of silently skipping them.

### Static Contract Check

**Executed:** YES
**Command:** bounded `git diff --check` plus required-pattern and legacy-pattern scans over the two exact stress files
**Exit Code:** 0
**Claim Source:** executed

```text
tests/stress/qf_decision_event_replay_test.go:80:       t.Cleanup(pool.Close)
tests/stress/qf_decision_event_replay_test.go:86:       t.Cleanup(natsClient.Close)
tests/stress/qf_decision_event_replay_test.go:327:      t.Cleanup(pool.Close)
tests/stress/qf_decision_event_replay_test.go:333:      t.Cleanup(natsClient.Close)
tests/stress/qf_decisions_sync_stress_test.go:61:       t.Cleanup(pool.Close)
tests/stress/qf_decisions_sync_stress_test.go:67:       t.Cleanup(natsClient.Close)
tests/stress/qf_decisions_sync_stress_test.go:81:       runTimestamp := time.Now().UTC().Format(time.RFC3339Nano)
tests/stress/qf_decisions_sync_stress_test.go:227:func TestQFDecisionsStressCleanupChildReturnsZeroOwnedRows(t *testing.T) {
tests/stress/qf_decisions_sync_stress_test.go:240:      t.Cleanup(pool.Close)
tests/stress/qf_decisions_sync_stress_test.go:312:func TestQFDecisionsStressCleanupReturnsErrorForClosedPool(t *testing.T) {
tests/stress/qf_decisions_sync_stress_test.go:432:                      cleanupErr = errors.Join(
tests/stress/qf_decisions_sync_stress_test.go:440:              DELETE FROM edges AS edge
tests/stress/qf_decisions_sync_stress_test.go:451:              DELETE FROM annotations AS annotation
STATIC_DIFF_CHECK_EXIT=0
REQUIRED_PATTERN_SCAN_EXIT=0
LEGACY_PATTERN_SCAN_EXIT=1 (1 means no matches)
SKIP_MARKER_SCAN_EXIT=1 (1 means no matches)
```

The negative scans covered function-level pool/NATS defers, cleanup `t.Logf`, the fixed `2026-05-06T00:00:00Z` fixture timestamp, the prior per-ID cleanup loop, and skip/only/todo/pending markers.

### Runtime Test Status

**Executed:** NO
**Command:** `./smackerel.sh test stress` was not run because this implementation request explicitly limited validation to editor and static checks.
**Claim Source:** not-run

The required RED/GREEN stress evidence, live parent/child database result, closed-pool test result, QF E2E canaries, broader stress suite, and validate-owned certification remain outstanding. No Definition of Done checkbox is changed by this static-only implementation pass.
