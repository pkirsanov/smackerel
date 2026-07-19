# Scopes: BUG-038-003 Drive E2E core health collapse

## Scope 1: Preserve core health through serialized Drive observability

**Status:** In Progress
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

- [ ] Root cause confirmed with current-session isolated and package-order RED evidence.
- [ ] Pre-fix regression fails at the first actual health/lifecycle defect.
- [ ] First confirmed defect fixed without blind timeout extension or arbitrary sleep.
- [ ] Observability reconciliation is isolated: live metric families, counters, and database rows reconcile.
- [ ] Package neighbors cannot poison core health before or after the observability scenario.
- [ ] Readiness failure reports the first terminal state when core is genuinely absent.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Focused unit and integration tests pass.
- [ ] Impacted Go and Python unit suites pass.
- [ ] Drive cascade-noise policy/retrieve/save/scan and later package symptoms recover.
- [ ] Regression tests contain no mock/interception, skip/only, bailout, or tautological patterns.
- [ ] Check, lint, and format pass with zero warnings.
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [ ] Packet artifact, traceability, implementation-reality, state-transition, and regression guards pass at `in_progress`.
- [ ] Source branch is committed and pushed through normal hooks; validate-owned certification remains `in_progress`.
