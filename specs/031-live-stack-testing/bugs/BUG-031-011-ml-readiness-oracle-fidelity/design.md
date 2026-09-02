# Bug Fix Design: [BUG-031-011] ML Readiness Oracle Fidelity

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Ownership Routing

This is the initial root-cause packet created by `bubbles.bug`. `bubbles.design` owns design approval or revision before implementation. `bubbles.plan` owns final scope and test-plan approval.

## Root Cause Analysis

### Investigation Summary

- `TestMLReadinessAlways200Regression` always writes HTTP 200.
- The test requires `ready=true` and `probes>0`.
- `WaitForMLReady` calls `probeMLHealth` on each ticker interval.
- `probeMLHealth` requests `${MLSidecarURL}/health` and compares the status to HTTP 200.
- The router registers core `GET /readyz` to `Dependencies.ReadyzHandler`.
- `ReadyzHandler` checks database readiness by default. Its strict mode adds graph readiness, not ML readiness.

### Root Cause

The scenario claims a negative guarantee, but the test setup contains only the positive state. The probe-count assertion closes one bypass path. It does not close the status-classification path.

The endpoint text drift came from treating startup ML readiness and core service readiness as one route. They are separate contracts in the current implementation.

### Existing Coverage Relationship

`TestMLReadinessTimeoutSilentBypass` and `TestMLReadiness_TimeoutFallback` already use HTTP 503 and expect false. They protect timeout behavior. The repaired test must add a two-sided discriminator and mutation proof.

### Impact Analysis

- Affected component: ML readiness regression tests.
- Affected scenario contract: `SCN-BUG-031-006-007`.
- Affected production behavior: none expected.
- Affected data: none.
- User impact: false assurance can permit a future readiness regression to merge.

## Fix Design

### Solution Approach

1. Replace `TestMLReadinessAlways200Regression` with `TestMLReadinessNonReadyDependencyCannotBeMaskedAsReady`.
2. Use healthy and non-ready subtests that share the production call path.
3. Require a nonzero probe count in both cases.
4. Use a compressed timeout for the HTTP 503 case.
5. Correct comments to name ML `/health` and core `/readyz` separately.
6. Mark `SCN-BUG-031-006-007` superseded by `SCN-BUG-031-011-001` in current scenario registries.
7. Preserve historical report output without edits.

### Mutation Proof

Before the repair, temporarily change status interpretation so every completed HTTP response is healthy. The existing focused test should let that mutant survive.

After the repair, apply the same mutation. The HTTP 503 subtest must fail. Restore the production file and verify its original hash before normal execution.

### Route Reconciliation

The repaired test must not call or claim `/ml/readyz`. The ML dependency endpoint is `${MLSidecarURL}/health`. Core `GET /readyz` remains a separate readiness surface.

### Alternative Approaches Considered

1. Rename the current test as a healthy-path test. Rejected because the historical negative scenario would remain unprotected.
2. Add ML state to core `/readyz`. Rejected because this is a production contract change outside this test-quality scope.
3. Add another standalone HTTP 503 timeout test. Rejected because existing tests already cover timeout fallback.

## Testing Strategy

- Use the focused stress selector for the repaired oracle.
- Re-run existing integration readiness controls.
- Run broader E2E and stress suites.
- Run the bugfix regression-quality guard.
- Record pre-repair surviving mutation and post-repair killed mutation separately.

## Change Boundary

Allowed implementation paths:

- `tests/stress/ml_readiness_timeout_stress_test.go`
- Current scenario metadata under spec 031 and BUG-031-006
- This bug packet

Excluded paths:

- `internal/api/ml_readiness.go`
- `internal/api/search.go`
- `internal/api/router.go`
- `internal/api/health.go`
- Docker, deployment, generated config, database, and UI files

## Complexity Tracking

None. The smallest viable fix adds one negative case, keeps one positive control, and corrects scenario identity.

## Risks And Resolutions

- Timing drift could make the HTTP 503 case slow. Use the existing compressed boundary.
- Historical evidence could be corrupted by rewriting it. Use supersession metadata.
- A route change could hide inside a test repair. Enforce the Change Boundary with hashes.