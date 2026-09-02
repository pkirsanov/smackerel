# Scopes: [BUG-031-011] ML Readiness Oracle Fidelity

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Planning Ownership

This route-ready scope draft records the bounded work discovered by `bubbles.bug`. `bubbles.plan` must approve or revise the scope and `test-plan.json` before implementation starts.

## Scope 1: Repair The ML Readiness Regression Oracle

**Status:** Not Started
**Priority:** P1
**Depends On:** None
**Goal Contribution:** Prevent a non-ready ML dependency from being certified as ready by a false-positive regression test.

### Gherkin Scenarios

```gherkin
Scenario: SCN-BUG-031-011-001 Non-ready ML dependency cannot be masked as ready
  Given the production ML readiness loop probes an HTTP dependency
  And the dependency returns HTTP 503 for every probe
  When the bounded readiness wait completes
  Then the result is not ready
  And at least one dependency probe occurred
```

```gherkin
Scenario: SCN-BUG-031-011-002 Healthy control distinguishes the harness
  Given the same production ML readiness loop and harness
  And the dependency returns HTTP 200
  When the readiness wait runs
  Then the result is ready
  And at least one dependency probe occurred
```

```gherkin
Scenario: SCN-BUG-031-011-003 Endpoint identity is accurate
  Given the ML loop requests `${MLSidecarURL}/health`
  And core `GET /readyz` is a separate database readiness route
  When current metadata describes the regression
  Then no current claim describes `/ml/readyz` as the tested route
```

### Implementation Plan

1. Capture a pre-repair mutation-survival result.
2. Replace the one-sided test with healthy and non-ready subtests.
3. Keep the timeout-silent-bypass test unchanged.
4. Correct endpoint names in current test and scenario metadata.
5. Supersede `SCN-BUG-031-006-007` without changing historical evidence.
6. Run focused and broader checks.

### Change Boundary

Allowed file families are the readiness stress test, this packet, and current scenario metadata for spec 031 and BUG-031-006. Production source, configuration, Docker, deployment, database, and UI files are excluded.

### Test Plan

| ID | Test Type | Category | File Or Location | Exact Behavior | Command | Live System | Scenario |
|---|---|---|---|---|---|---|---|
| TP-031-011-01 | Pre-fix mutation survival | functional | `tests/stress/ml_readiness_timeout_stress_test.go` | An always-ready status mutant survives the old one-sided oracle | `./smackerel.sh test stress --go-run '^TestMLReadinessAlways200Regression$'` | No | SCN-BUG-031-011-001 |
| TP-031-011-02 | Regression E2E readiness discriminator | stress | `tests/stress/ml_readiness_timeout_stress_test.go` | HTTP 200 returns ready and HTTP 503 cannot return ready through the production loop | `./smackerel.sh test stress --go-run '^TestMLReadinessNonReadyDependencyCannotBeMaskedAsReady$'` | No, external ML is simulated | SCN-BUG-031-011-001, SCN-BUG-031-011-002 |
| TP-031-011-03 | Integration companion regression | integration | `tests/integration/ml_readiness_test.go` | Existing healthy transition and timeout fallback remain intact | `./smackerel.sh test integration --go-run '^TestMLReadiness_(WaitForHealthy|TimeoutFallback)$'` | No, external ML is simulated | SCN-BUG-031-011-001, SCN-BUG-031-011-002 |
| TP-031-011-04 | Broader E2E regression | e2e-api | `tests/e2e/` | Existing live API journeys retain their behavior | `./smackerel.sh test e2e` | Yes | SCN-BUG-031-011-003 |
| TP-031-011-05 | Broader stress regression | stress | `tests/stress/` | Existing stress behavior remains intact | `./smackerel.sh test stress` | Yes | SCN-BUG-031-011-001 |
| TP-031-011-06 | Regression quality guard | functional | `tests/stress/ml_readiness_timeout_stress_test.go` | No bailout or tautological bugfix pattern remains | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/stress/ml_readiness_timeout_stress_test.go` | No | SCN-BUG-031-011-001 |

The `Regression E2E` row exercises the complete `WaitForMLReady` to HTTP dependency boundary. It does not claim a live Smackerel stack because the external ML dependency is represented by an in-process HTTP server.

### Definition of Done

- [ ] Root cause and route identity are approved by `bubbles.design`.
- [ ] `SCN-BUG-031-011-001`: a non-ready ML dependency that returns HTTP 503 cannot be masked as ready, and at least one dependency probe is observed.
- [ ] `SCN-BUG-031-011-002`: the healthy HTTP 200 control returns ready and observes at least one dependency probe, distinguishing the harness from a blanket failure.
- [ ] Pre-fix mutation survival is captured for TP-031-011-01.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Integration companion regression TP-031-011-03 passes.
- [ ] Broader E2E regression suite passes
- [ ] Broader stress regression TP-031-011-05 passes.
- [ ] Regression quality guard TP-031-011-06 passes.
- [ ] `SCN-BUG-031-006-007` is superseded without changing historical evidence.
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [ ] `SCN-BUG-031-011-003`: endpoint descriptions accurately name `${MLSidecarURL}/health` as the ML endpoint and core `GET /readyz` as the separate database readiness endpoint, with no current claim that `/ml/readyz` is exercised.
- [ ] Build Quality Gate passes with zero warnings, clean formatting, clean lint, clean artifact lint, and aligned documentation.

No item is checked. This invocation created planning artifacts only and executed no test or Docker command.