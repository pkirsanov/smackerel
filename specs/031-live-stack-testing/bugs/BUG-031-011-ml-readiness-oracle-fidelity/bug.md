# Bug: [BUG-031-011] ML Readiness Oracle Fidelity

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Summary

`TestMLReadinessAlways200Regression` configures an ML health dependency that always returns HTTP 200. It then expects `WaitForMLReady` to return true. The test proves the healthy path and confirms that at least one probe occurred. It does not prove that a non-ready dependency cannot be reported ready.

The test comment also names `/ml/readyz`. The production probe calls `${MLSidecarURL}/health`. The registered core `/readyz` route is a separate database readiness contract.

## Severity

- [ ] Critical - System unusable or data loss
- [ ] High - Major behavior broken with no safe workaround
- [x] Medium - A required regression claim has a false-positive test oracle
- [ ] Low - Minor or cosmetic issue

## Status

- [x] Reported
- [ ] Confirmed by an executed failing or mutation-survival reproduction
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Read `TestMLReadinessAlways200Regression` in `tests/stress/ml_readiness_timeout_stress_test.go`.
2. Observe that its HTTP handler always writes `http.StatusOK`.
3. Observe that the test fails when `ready` is false and passes when `ready` is true after any probe.
4. Read `SearchEngine.probeMLHealth` in `internal/api/search.go`.
5. Observe that the production probe requests `${MLSidecarURL}/health` and treats only HTTP 200 as healthy.
6. Read the router registration and `ReadyzHandler` in `internal/api/router.go` and `internal/api/health.go`.
7. Observe that core `GET /readyz` checks database readiness and does not represent the ML dependency.

## Expected Behavior

The regression test must distinguish a ready ML dependency from a non-ready ML dependency. A non-200 ML `/health` response must cause `WaitForMLReady` to return false within the configured boundary. Test names, comments, and scenario contracts must identify the actual endpoints.

## Actual Behavior

The current test supplies only HTTP 200 and expects success. An implementation that probes the dependency but treats every response status as ready would survive this test. The scenario contract therefore claims more than its assertion proves.

## Environment

- Repository: `smackerel`
- Owning feature: `specs/031-live-stack-testing`
- Source revision inspected: `7ce32d703a17f3cb5a2481a1ebc0cc084a1fac61`
- Platform: Linux
- Finding date: 2026-09-02

## Error Output

No runtime error output was captured. This packet records a source-grounded test-oracle finding. The operator prohibited Docker execution during packet creation.

## Root Cause

The test setup encodes only the success condition that the assertion expects. Its `probes > 0` assertion detects a no-probe shortcut, but it cannot detect an always-ready interpretation after a real probe. Historical scenario text also conflates the ML `/health` dependency with core `/readyz`.

## Related

- Feature: `specs/031-live-stack-testing/`
- Follow-up to: `specs/031-live-stack-testing/bugs/BUG-031-006-strict-guard-gate-drift/`
- Historical scenario: `SCN-BUG-031-006-007`
- Existing negative companion: `tests/integration/ml_readiness_test.go::TestMLReadiness_TimeoutFallback`

## Current Invocation Boundary

This invocation creates artifacts only. It does not modify source or tests, run Docker, execute regression tests, or claim that the bug is fixed.