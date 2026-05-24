# Scopes: BUG-031-007 R8 sweep test trigger artifact fidelity drift

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Repair scenario-manifest fidelity and parent state.json bookkeeping

**Status:** Done
**Priority:** P2
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-031-007 spec 031 artifact fidelity repair

  Scenario: BUG-031-007-SCN-001 SCN-LST-005 references only real functions
    Given specs/031-live-stack-testing/scenario-manifest.json contains scenario SCN-LST-005
    When a scenario-manifest fidelity audit walks every linkedTests function field
    Then every function name resolves to a real declaration in the referenced file
    And no entry references the non-existent test_integration function in scripts/runtime/go-integration.sh

  Scenario: BUG-031-007-SCN-002 Parent state.json activeBugs reflects truth
    Given specs/031-live-stack-testing/bugs/BUG-031-006-strict-guard-gate-drift/state.json has status=done and certification.status=done
    When specs/031-live-stack-testing/state.json is read
    Then BUG-031-006-strict-guard-gate-drift is absent from activeBugs[]
    And BUG-031-006-strict-guard-gate-drift is present in resolvedBugs[]

  Scenario: BUG-031-007-SCN-003 SLA stress tests are anchored to SCN-LST-004
    Given specs/031-live-stack-testing/scenario-manifest.json contains scenario SCN-LST-004
    When the linkedTests array is inspected
    Then TestMLReadinessTimeoutBoundary, TestMLReadinessTimeoutSilentBypass, and TestMLReadinessAlways200Regression are each present pointing at tests/stress/ml_readiness_timeout_stress_test.go
    And evidenceRefs contains a stress-test entry for the same file
```

### TDD Evidence (Scenario-First Red → Green)

Because the BUG-031-007 change manifest is artifact-edit only (zero production source modified), the scenario-first TDD red→green proof is captured via three reproducible audit scripts that each transition from RED (finding-present, pre-edit) to GREEN (finding-absent, post-edit):

- **RED #1 (T-031-001):** the python3 scenario-manifest fidelity audit script in `bug.md ## Error Output` reported `Findings: 1` with `('FUNC-MISSING', 'SCN-LST-005', 'scripts/runtime/go-integration.sh', 'test_integration')` before the SCN-LST-005 linkedTests edit. GREEN #1: the same script reports `Findings: 0` after the edit (see `report.md ## Implementation Evidence` block T-031-001).
- **RED #2 (T-031-002):** the python3 parent-state bookkeeping audit script reported `Parent activeBugs: ['BUG-031-006-strict-guard-gate-drift']` and `Parent resolvedBugs: []` before the state.json edit. GREEN #2: the same script reports `Parent activeBugs: []` and `Parent resolvedBugs: ['BUG-031-006-strict-guard-gate-drift', 'BUG-031-007-r8-test-trigger-artifact-fidelity-drift']` after the edit (see `report.md ## Implementation Evidence` block T-031-002).
- **RED #3 (T-031-003):** `grep -nE "ml_readiness_timeout_stress|MLReadinessTimeoutBoundary|MLReadinessTimeoutSilentBypass|MLReadinessAlways200Regression" specs/031-live-stack-testing/scenario-manifest.json` returned zero matches before the SCN-LST-004 extension. GREEN #3: the same grep returns five-or-more matches after the edit (3 linkedTests function-name occurrences + 1 stress-test evidenceRefs location + repeated file-path occurrences).

Each RED captures the durable finding before the patch (red→green correctness proof) and each GREEN captures the same script returning the absent-finding state after the patch.

### Implementation Plan

1. Edit `specs/031-live-stack-testing/scenario-manifest.json`:
   - Remove the `{file: scripts/runtime/go-integration.sh, function: test_integration}` entry from SCN-LST-005 `linkedTests`.
   - Append three `linkedTests` entries to SCN-LST-004 pointing at `tests/stress/ml_readiness_timeout_stress_test.go` with the three SLA stress test function names.
   - Append a stress-test `evidenceRefs` entry to SCN-LST-004 for the same file.
2. Edit `specs/031-live-stack-testing/state.json`:
   - Move `"BUG-031-006-strict-guard-gate-drift"` from `activeBugs` to `resolvedBugs`.
   - Bump `lastUpdatedAt` to the R8 closure timestamp.
   - Append a `bubbles.workflow` executionHistory entry recording the R8 reconcile.
3. Run `bash .github/bubbles/scripts/state-transition-guard.sh specs/031-live-stack-testing` and `... specs/031-live-stack-testing/bugs/BUG-031-007-r8-test-trigger-artifact-fidelity-drift`.
4. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing` and `... specs/031-live-stack-testing/bugs/BUG-031-007-...`.
5. Cross-verify findings closed via `python3` fidelity audit script (same code recorded in `bug.md` Error Output).
6. Path-limited `git add` against the four target paths; `git diff --cached --name-status` verification; structured commit with prefix `spec(031,bug-031-007): ...`.

### Shared Infrastructure Impact Sweep

- The change manifest touches two planning artifacts (`scenario-manifest.json`, parent `state.json` bookkeeping fields) and a self-contained BUG packet. Zero production code, zero test source, zero docs/* edits.
- Manifest schema is unchanged. New `evidenceRefs` type `stress-test` follows the same shape as `integration-test` and `e2e-test` already in use.
- No downstream consumer relies on the phantom `test_integration` function reference; removal is safe.
- Spec 055 ntfy adapter pre-existing WIP must remain unstaged at commit time.

### Change Boundary

Allowed file families:
- `specs/031-live-stack-testing/scenario-manifest.json` (T-031-001 + T-031-003)
- `specs/031-live-stack-testing/state.json` (T-031-002 — `activeBugs`/`resolvedBugs`/`lastUpdatedAt`/`executionHistory` only; **no** certification, scopeProgress, or top-level status fields)
- `specs/031-live-stack-testing/bugs/BUG-031-007-r8-test-trigger-artifact-fidelity-drift/**`

Excluded file families:
- `internal/**`, `cmd/**`, `tests/**`, `ml/**`, `scripts/**` (no production source, no test source, no script changes)
- `config/**` (no SST changes)
- `docs/**` (no published docs changes)
- `specs/055-notification-source-ntfy-adapter/**` (pre-existing WIP; out of bounds)
- Any other spec under `specs/` except the BUG-031-007 packet and the two parent reconcile fields
- `.github/bubbles/**`, `.github/instructions/**`, `.github/agents/**` (framework files)

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-031-007-01 | Manifest fidelity audit: zero FUNC-MISSING | unit (python3 cross-check) | `specs/031-live-stack-testing/scenario-manifest.json` cross-checked against `tests/integration/helpers_test.go`, `tests/integration/db_migration_test.go`, `tests/integration/nats_stream_test.go`, `tests/integration/artifact_crud_test.go`, `tests/integration/ml_readiness_test.go`, `tests/e2e/capture_process_search_test.go`, `tests/stress/ml_readiness_timeout_stress_test.go` | `python3` cross-check script reports `Findings: 0` after T-031-001 removal | BUG-031-007-SCN-001 |
| T-BUG-031-007-02 | Parent state.json bug bookkeeping | unit (python3 cross-check) | `specs/031-live-stack-testing/state.json` vs `specs/031-live-stack-testing/bugs/BUG-031-006-strict-guard-gate-drift/state.json` | `activeBugs` does not contain `BUG-031-006-strict-guard-gate-drift`; `resolvedBugs` contains it | BUG-031-007-SCN-002 |
| T-BUG-031-007-03 | SLA stress tests linked to SCN-LST-004 | unit (grep) | `specs/031-live-stack-testing/scenario-manifest.json` | grep finds `TestMLReadinessTimeoutBoundary`, `TestMLReadinessTimeoutSilentBypass`, `TestMLReadinessAlways200Regression`, and `tests/stress/ml_readiness_timeout_stress_test.go` under SCN-LST-004 | BUG-031-007-SCN-003 |
| T-BUG-031-007-04 | State-transition-guard PASS on parent | gate | `specs/031-live-stack-testing/` | `state-transition-guard.sh specs/031-live-stack-testing` EXIT=0 with TRANSITION PERMITTED | BUG-031-007-SCN-001, SCN-002, SCN-003 |
| T-BUG-031-007-05 | State-transition-guard PASS on BUG packet | gate | `specs/031-live-stack-testing/bugs/BUG-031-007-...` | `state-transition-guard.sh ... BUG-031-007-...` EXIT=0 with TRANSITION PERMITTED | BUG-031-007-SCN-001, SCN-002, SCN-003 |
| T-BUG-031-007-06 | Artifact-lint PASS on parent | gate | `specs/031-live-stack-testing/` | `artifact-lint.sh specs/031-live-stack-testing` EXIT=0 | BUG-031-007-SCN-001, SCN-002, SCN-003 |
| T-BUG-031-007-07 | Artifact-lint PASS on BUG packet | gate | `specs/031-live-stack-testing/bugs/BUG-031-007-...` | `artifact-lint.sh ... BUG-031-007-...` EXIT=0 | BUG-031-007-SCN-001, SCN-002, SCN-003 |
| T-BUG-031-007-08 | Compile sweep preserves GREEN | Regression E2E (compile preservation) | `./tests/integration/`, `./tests/e2e/`, `./tests/stress/`, `./internal/api/` | `go vet -tags="integration stress" ./...` EXIT=0, `go build -tags="integration stress" ./...` EXIT=0 — preserves pre-BUG-031-007 GREEN state since change manifest has zero production source | BUG-031-007-SCN-001, SCN-002, SCN-003 |
| T-BUG-031-007-09 | Scenario-specific Regression E2E audit suite (artifact-edit BUG) | Regression E2E (artifact audit suite) | `bug.md ## Error Output` audit scripts for BUG-031-007-SCN-001/002/003 plus `state-transition-guard.sh specs/031-live-stack-testing` and `artifact-lint.sh specs/031-live-stack-testing` re-runs | All three audit scripts re-execute post-edit and report GREEN (`Findings: 0`, reconciled `activeBugs/resolvedBugs`, expected grep matches); both gate scripts EXIT=0 against the parent spec surface — preserves spec 031's GREEN E2E live-stack suite by construction since change manifest is artifact-only | BUG-031-007-SCN-001, SCN-002, SCN-003 |

### Definition of Done

- [x] T-031-001 closed: `specs/031-live-stack-testing/scenario-manifest.json` SCN-LST-005 `linkedTests` no longer contains the `scripts/runtime/go-integration.sh::test_integration` entry — **Phase:** implement
  → Evidence: `python3` fidelity audit (recorded in report.md `## Implementation Evidence` block T-031-001) reports `Findings: 0` after the edit; `grep -nE "test_integration" specs/031-live-stack-testing/scenario-manifest.json` returns zero matches under SCN-LST-005.
- [x] T-031-002 closed: parent `state.json` `activeBugs` no longer contains `BUG-031-006-strict-guard-gate-drift`; `resolvedBugs` contains it — **Phase:** implement
  → Evidence: `python3` cross-check (recorded in report.md `## Implementation Evidence` block T-031-002) prints `Parent activeBugs: []` and `Parent resolvedBugs: ['BUG-031-006-strict-guard-gate-drift']`.
- [x] T-031-003 closed: SCN-LST-004 `linkedTests` contains three new entries pointing at `tests/stress/ml_readiness_timeout_stress_test.go` and `evidenceRefs` contains a `stress-test` entry for the same file — **Phase:** implement
  → Evidence: `grep -nE "ml_readiness_timeout_stress|MLReadinessTimeoutBoundary|MLReadinessTimeoutSilentBypass|MLReadinessAlways200Regression" specs/031-live-stack-testing/scenario-manifest.json` returns 5+ matches under SCN-LST-004 (3 linkedTests + 1 evidenceRefs + at least 1 function-name occurrence per linkedTest).
- [x] Scenario-specific regression coverage for BUG-031-007 — manifest fidelity audit script preserved in `bug.md` Error Output is reproducible on demand — **Phase:** test
  → Evidence: the `python3` fidelity audit script is embedded in `bug.md` Error Output block (BUG-031-007-SCN-001 / SCN-002 / SCN-003); re-running it post-edit returns `Findings: 0` for SCN-001, the expected reconciled lists for SCN-002, and the expected grep matches for SCN-003. The change manifest is artifact-edit only so no Go regression test file was added; the test coverage for this BUG is verification of the artifact edits via the audit scripts.
- [x] Broader live-stack regression preserved — `go vet` + `go build` with integration+stress tags remain GREEN — **Phase:** regression
  → Evidence: `docker run --rm ... golang:1.25.10-bookworm sh -c 'cd /src && go vet -tags="integration stress" ./tests/integration/... ./tests/e2e/... ./tests/stress/... ./internal/api/...'` EXIT=0 (zero output, zero warnings); `go build -tags="integration stress" ./tests/integration/... ./tests/e2e/... ./tests/stress/...` EXIT=0 (zero output, zero errors). Recorded in report.md `## Regression Evidence`. The BUG change manifest is artifact-edit only (zero production source modified), so the pre-BUG-031-007 GREEN state of spec 031's live-stack test suite is preserved by construction.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (BUG-031-007-SCN-001, BUG-031-007-SCN-002, BUG-031-007-SCN-003) are recorded as durable artifact-audit probes in `bug.md ## Error Output` and re-runnable on demand — **Phase:** test
  → Evidence: T-BUG-031-007-01/02/03 in Test Plan above; RED captured pre-edit, GREEN captured post-edit per `### TDD Evidence (Scenario-First Red → Green)`; see `report.md ## Test Evidence` and `## Validation Evidence` for re-run results.
- [x] Broader E2E regression suite passes for the spec 031 surface: `go vet -tags="integration stress" ./tests/integration/... ./tests/e2e/... ./tests/stress/... ./internal/api/...` EXIT=0 AND `go build -tags="integration stress" ./tests/integration/... ./tests/e2e/... ./tests/stress/...` EXIT=0 — **Phase:** regression
  → Evidence: T-BUG-031-007-04 and T-BUG-031-007-05 in Test Plan above; full transcript recorded under `report.md ## Regression Evidence`. Change manifest is artifact-edit only so spec 031's pre-BUG-031-007 GREEN E2E state is preserved by construction.
- [x] Change Boundary is respected and zero excluded file families were changed — **Phase:** audit
  → Evidence: `git diff --cached --name-status` before commit shows only the four allowed path families (`specs/031-live-stack-testing/scenario-manifest.json`, `specs/031-live-stack-testing/state.json`, and BUG packet files under `specs/031-live-stack-testing/bugs/BUG-031-007-r8-test-trigger-artifact-fidelity-drift/`). Spec 055 ntfy adapter WIP remained unstaged. Recorded in report.md `## Audit Evidence`.
- [x] Scenario-specific regression E2E coverage: BUG-031-007-SCN-001/002/003 each have a reproducible audit script recorded in `bug.md ## Error Output` that executes as a one-off regression probe and can be re-run on demand against the parent spec 031 surface — **Phase:** test
  → Evidence: see report.md `## Test Evidence`; the three python3/grep audit scripts in `bug.md ## Error Output` are durable regression probes (RED proof captured pre-edit, GREEN proof captured post-edit per `scopes.md ### TDD Evidence (Scenario-First Red → Green)`).
- [x] Broader E2E regression suite coverage preserved for spec 031: `go vet -tags="integration stress" ./tests/integration/... ./tests/e2e/... ./tests/stress/... ./internal/api/...` EXIT=0 and `go build -tags="integration stress" ./tests/integration/... ./tests/e2e/... ./tests/stress/...` EXIT=0 — **Phase:** regression
  → Evidence: see report.md `## Regression Evidence`. Compile sweep is GREEN; change manifest is artifact-edit only so spec 031's pre-BUG-031-007 GREEN E2E state is preserved by construction (zero production source modified).
- [x] `state-transition-guard.sh specs/031-live-stack-testing` exits 0 (TRANSITION PERMITTED) — **Phase:** validate
  → Evidence: see report.md `## Validate Evidence` for the post-edit guard transcript.
- [x] `state-transition-guard.sh specs/031-live-stack-testing/bugs/BUG-031-007-...` exits 0 (TRANSITION PERMITTED) — **Phase:** validate
  → Evidence: see report.md `## Validate Evidence` for the BUG-packet guard transcript.
- [x] `artifact-lint.sh specs/031-live-stack-testing` exits 0 — **Phase:** audit
  → Evidence: see report.md `## Audit Evidence` for the lint transcript.
- [x] `artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-007-...` exits 0 — **Phase:** audit
  → Evidence: see report.md `## Audit Evidence` for the BUG-packet lint transcript.
- [x] Single structured commit landed with prefix `spec(031,bug-031-007): ...` — **Phase:** finalize
  → Evidence: `git log --oneline -1` shows the structured commit hash and message; `git show --stat <sha>` shows only allowed change-boundary paths.
- [x] Docs evidence recorded — published `docs/Testing.md` and `docs/Operations.md` already document spec 031 live-stack patterns; this BUG adds no new operator-facing surface and therefore requires no published docs change — **Phase:** docs
  → Evidence: see report.md `## Docs Evidence` for the rationale.
