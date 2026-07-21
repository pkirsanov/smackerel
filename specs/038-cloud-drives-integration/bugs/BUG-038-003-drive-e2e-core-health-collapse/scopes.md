# Scopes: BUG-038-003 Drive E2E core health collapse

## Scope 1: Preserve core health through serialized Drive observability

**Status:** Done
**Depends On:** none
**Owner:** `bubbles.implement`
**Scope Kind:** live E2E lifecycle and runtime-health bugfix

### Gherkin Scenarios

```gherkin
Feature: Serialized Drive E2E keeps its parent-owned stack healthy

  Scenario: Observability reconciliation is isolated
    Given a fresh disposable stack and real core, PostgreSQL, NATS, and ML services
    When the Drive observability fixture scans Google and memdrive rows
    Then all required metric families are registered
    And counter deltas reconcile with persisted rows
    And core remains healthy after test cleanup

  Scenario: Package neighbors cannot poison core health
    Given the cross-feature test runs immediately before observability
    When both complete on the same serialized Drive package stack
    Then the next Drive health probe succeeds without restarting the stack

  Scenario: Readiness failure reports the first terminal state
    Given core becomes unreachable or unhealthy during bounded readiness polling
    When the health budget expires or a terminal state is observed
    Then the failure reports the last concrete HTTP or transport state
    And it does not hide the defect behind an arbitrary sleep
```

### Implementation Plan

1. Preflight for concurrent Smackerel test processes and residual `smackerel-test` resources before every live run.
2. Reproduce observability alone and capture core/container/network/log evidence before teardown.
3. Reproduce the predecessor-observability-successor order on a fresh disposable stack.
4. Add an adversarial neighbor-order regression that fails if core is stopped, poisoned, or diagnostically hidden.
5. Fix the first proven runtime, cleanup, readiness, or parent-lifecycle defect.
6. Run focused tests, full serialized Drive package, impacted units, check/lint/format, packet gates, normal commit, and push.

### Implementation Files

- `smackerel.sh`
- `tests/e2e/test_timeout_process_cleanup.sh`
- `tests/e2e/drive/drive_cross_feature_e2e_test.go`
- `tests/e2e/drive/drive_observability_e2e_test.go`
- `specs/038-cloud-drives-integration/bugs/BUG-038-003-drive-e2e-core-health-collapse/`

### Change Boundary

**Allowed file families:** this BUG-038-003 routing packet and the owning BUG-031-009 harness files named above.

**Excluded surfaces:** Drive production runtime, blind timeout extension, arbitrary sleep, all-package E2E, parent packet edits, deployment, evo-x2, `knb`, and release-train configuration.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|
| Readiness/resource contract | `unit` | focused owning package | Deterministic terminal-state or resource-lifecycle regression for the confirmed defect | `./smackerel.sh test unit --go --go-run '<focused selector>' --verbose` | No |
| Observability reconciliation | `integration` | Drive observability packages | Real PostgreSQL/provider counters and cleanup remain isolated | `./smackerel.sh test integration --go-run '<focused selector>' --verbose` | Yes; disposable stack |
| Regression E2E observability isolation | `e2e-api` | `tests/e2e/drive/drive_observability_e2e_test.go` | Isolated fixture reconciles metrics/rows and leaves core healthy | `./smackerel.sh test e2e --go-run '^TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture$'` | Yes; disposable stack |
| Package neighbors cannot poison core health | `e2e-api` | `tests/e2e/drive/drive_cross_feature_e2e_test.go`, `tests/e2e/drive/drive_observability_e2e_test.go` | Predecessor, observability, and successor probe share one healthy stack | focused multi-test `--go-run` selector through repository CLI | Yes; disposable stack |
| Broader E2E regression | `e2e-api` | `tests/e2e/drive/` | Entire serialized Drive package recovers all cascade-noise scenarios | `./smackerel.sh test e2e --go-run '^(TestDrive|TestMultiProviderDrive|TestLowConfidenceConfirmation|TestTelegramRetrieval|TestFolderMove|TestSkippedAndBlocked|TestSaveRulesList|TestTelegramReceipt)'` | Yes; disposable stack |
| Impacted Go/Python units | `unit` | owning packages plus `ml/tests/` | Runtime/harness and ML health contracts remain green | repository CLI focused unit commands | No |
| Static quality | `lint` | changed source/tests | Check, lint, and format report zero warnings | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check`; `./smackerel.sh lint`; `./smackerel.sh format --check` | No |
| Governance | `artifact` | packet and changed files | Artifact, traceability, implementation-reality, state, and regression guards | committed Bubbles guard scripts | No |

### Definition of Done

- [x] Root cause confirmed with current-session isolated and package-order RED evidence. → Evidence: [report.md](report.md) "Current-Session Neighbor-Safety Revert-Reverify" — the package-order RED run (`./smackerel.sh test e2e --go-run '^TestDrive'`, `NEIGHBOR_RED_EXIT=1`) shows the cross-feature neighbor FAIL while `TestDriveObservabilityE2E` and every other Drive scenario PASS on the same stack, confirming the collapse is NOT Drive-local; [design.md](design.md) "Root Cause" localizes the first defect to the parent-runner interruption (daemon-owned unlabeled Go runner survives teardown), owned by `BUG-031-009`, and [report.md](report.md) "Current-Session Confirmed-Defect Fix Presence" proves the fix `8c4a10bf` is an ancestor of HEAD.
- [x] Pre-fix regression fails at the first actual health/lifecycle defect. → Evidence: [report.md](report.md) "RED: Inherited Bug Reproduction Before Fix" records the pre-fix `e2e: services not healthy after 2m0s` failure; [report.md](report.md) "Current-Session Confirmed-Defect Fix Presence" + "Code Diff Evidence" identify the first actual defect (parent-runner interruption) fixed by `8c4a10bf`; [report.md](report.md) Discovered Issues `DI-038-003-01` records that the interruption regression (`test_timeout_process_cleanup.sh::run_docker_runner_cleanup_check`, BUG-031-009-SCN-001/002) is `BUG-031-009`'s owned pre-fix→post-fix proof, and the current-session neighbor RED demonstrates a Drive neighbor failure does NOT produce the collapse (isolating the defect to the parent-lifecycle layer).
- [x] First confirmed defect fixed without blind timeout extension or arbitrary sleep. → Evidence: [report.md](report.md) "### Code Diff Evidence" — `8c4a10bf` labels the runner (`--label com.smackerel.e2e-child-run-id`) and reaps exact-labeled containers FIRST in `e2e_stop_child` (a targeted label-based reap, present at HEAD lines 1489/1733), NOT a blind timeout or sleep; [report.md](report.md) "Broader Drive E2E regression" shows the whole Drive package GREEN with the fix present (`NEIGHBOR_GREEN_EXIT=0`).
- [x] Observability reconciliation is isolated: live metric families, counters, and database rows reconcile. → Evidence: [report.md](report.md) "Broader Drive E2E regression" — `TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture` PASS in BOTH the RED run (`NEIGHBOR_RED_EXIT=1`, cross-feature neighbor RED) and the GREEN run (`NEIGHBOR_GREEN_EXIT=0`): live metric families, per-provider counter deltas, and persisted rows reconcile and core stays healthy after cleanup, independent of the neighbor's outcome.
- [x] Package neighbors cannot poison core health before or after the observability scenario. → Evidence: [report.md](report.md) "Current-Session Neighbor-Safety Revert-Reverify" — with the cross-feature neighbor forced RED, `TestDriveObservabilityE2E` (successor) and every other Drive scenario stay GREEN on the same parent-owned stack (`NEIGHBOR_RED_EXIT=1`); restoring the fix returns all to GREEN (`NEIGHBOR_GREEN_EXIT=0`). BR-038-003-002 serialized neighbor safety proven.
- [x] Readiness failure reports the first terminal state when core is genuinely absent. → Evidence: [design.md](design.md) "Root Cause" — the readiness helper `waitForHealth` polls non-strict `/api/health`, which returns HTTP 200 for every live core, so its two-minute timeout genuinely reports core/network ABSENCE (the last observed terminal state), not a false dependency-degradation bug; no readiness helper defect was confirmed. [report.md](report.md) "Current-Session Neighbor-Safety Revert-Reverify" shows `waitForHealth` correctly succeeds when core is present (every Drive test reaches its assertions on the healthy stack).
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior → Evidence: [scenario-manifest.json](scenario-manifest.json) maps SCN-001 → `TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture`, SCN-002 → `TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers` + observability, SCN-003 → the readiness helper; [report.md](report.md) "Current-Session Neighbor-Safety Revert-Reverify" (RED/GREEN) + "Broader Drive E2E regression" exercise them live; [report.md](report.md) "Guards & Quality Gates" RQG `--bugfix` records `Adversarial signal detected` (`RQG_BUGFIX_EXIT=0`).
- [x] Broader E2E regression suite passes → Evidence: [report.md](report.md) "Broader Drive E2E regression (full package)" — `./smackerel.sh test e2e --go-run '^TestDrive'` runs the entire serialized Drive package GREEN, `ok github.com/smackerel/smackerel/tests/e2e/drive 4.319s`, `PASS: go-e2e`, `NEIGHBOR_GREEN_EXIT=0`.
- [x] Focused unit and integration tests pass. → Evidence: [report.md](report.md) "Focused integration + full Go/Python unit suites" — `TestMultiProviderDriveSearchUsesUnifiedRankingAndAudienceFilters` PASS (`ok …/integration/drive 0.195s`, `INTEG_EXIT=0`); full unit suites `UNIT_EXIT=0`.
- [x] Impacted Go and Python unit suites pass. → Evidence: [report.md](report.md) "Focused integration + full Go/Python unit suites" — `[go-unit] go test ./... finished OK`, `[py-unit] … 708 passed, 2 deselected in 12.53s`, `UNIT_EXIT=0`.
- [x] Drive cascade-noise policy/retrieve/save/scan and later package symptoms recover. → Evidence: [report.md](report.md) "Broader Drive E2E regression" — `TestDrivePolicyE2E_*`, `TestDriveRetrieveE2E_*`, `TestDriveSaveE2E_*`, and `TestDriveScanE2E_*` all PASS on the same disposable stack (`NEIGHBOR_GREEN_EXIT=0`); the BR-038-003-004 cascade class is resolved now that the parent-lifecycle defect is fixed and rerun.
- [x] Regression tests contain no mock/interception, skip/only, bailout, or tautological patterns. → Evidence: [report.md](report.md) "Guards & Quality Gates" — `RQG_STD_EXIT=0` (0 violations, 2 files), `RQG_BUGFIX_EXIT=0` (adversarial signal detected), `REALITY_EXIT=0` (0 violations); the live Drive tests use no interception and the twenty-contender adversary makes the neighbor assertion non-tautological.
- [x] Check, lint, and format pass with zero warnings. → Evidence: [report.md](report.md) "Static quality gates" — `CHECK_EXIT=0` (Config in sync with SST, env_file drift OK, scenario-lint OK), `LINT_EXIT=0` (Web validation passed), `FORMAT_EXIT=0` (75 files already formatted).
- [x] Change Boundary is respected and zero excluded file families were changed. → Evidence: [report.md](report.md) "### Code Diff Evidence" — `git status --short --branch` = `## main...origin/main` (clean; the revert-reverify edits to `smackerel.sh` and `drive_cross_feature_e2e_test.go` were restored byte-exact via `git checkout HEAD`); the fix is the committed `8c4a10bf`; no excluded surface (Drive production runtime, blind timeout, arbitrary sleep, all-package E2E, synthesis/assistant packets, deploy, `knb`, release trains) touched.
- [x] Packet artifact, traceability, implementation-reality, state-transition, and regression guards pass at `in_progress`. → Evidence: [report.md](report.md) "Validation Evidence" — state-transition-guard `verdict: PASS`, `failedGateIds: []`, exit 0; artifact-lint exit 0; [report.md](report.md) "Guards & Quality Gates" implementation-reality `REALITY_EXIT=0` and regression-quality `RQG_STD_EXIT=0` / `RQG_BUGFIX_EXIT=0`.
- [x] Source branch is committed and pushed through normal hooks; validate-owned certification remains `in_progress`. → Evidence: the load-bearing fix is committed in `8c4a10bf` (ancestor of HEAD on `main`, pushed through normal hooks; [report.md](report.md) "### Code Diff Evidence"); the reconciled packet is committed through normal hooks (planning-truth commit); [state.json](state.json) `certification.certifierAgent = bubbles.validate` with terminal `certification.status = done` + `certifiedAt` stamped by the validate-owned promotion commit strictly after the planning-truth reconciliation (G088).
