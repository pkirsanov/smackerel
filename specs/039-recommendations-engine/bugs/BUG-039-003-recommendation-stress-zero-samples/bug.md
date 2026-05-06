# Bug: BUG-039-003 Recommendation stress zero samples

## Summary
`TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests` exits with `stress: zero samples collected - workers never produced any observations` after the shared stress readiness repair proves the disposable stack, shell health/search, Go readiness canary, agent DB/NATS wiring, and drive stress are no longer the first red condition.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Feature 039 stress/NFR certification is blocked for protected scenario `SCN-039-052`
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed by upstream BUG-031-005 stress evidence after shared readiness passed
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Start from the current BUG-031-005 repaired stress readiness path.
2. Run the repo stress gate through `./smackerel.sh test stress`.
3. Observe the disposable test stack become healthy, shell health/search stress pass, and `TestStressReadinessCanary_Live` pass before package workloads.
4. Observe `tests/stress/recommendations_test.go::TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests` fail after readiness with zero observations.

## Expected Behavior
After shared readiness passes, the recommendation stress workload should collect observations from 50 concurrent warm reactive POST requests against `/api/recommendations/requests`. The run should either meet the `SCN-039-052`/`R-032` p95 latency and error-budget contract or fail with workload diagnostics that name status/error classes and request counts.

## Actual Behavior
The workload exits after approximately one HTTP client timeout window with `stress: zero samples collected - workers never produced any observations`, so the stress output does not prove p95 latency, error rate, provider runtime state, or useful workload diagnostics for feature 039.

## Environment
- Service: Go core recommendation API and stress workload package
- Parent feature: `specs/039-recommendations-engine`
- Parent scope: `scope-06-observability-stress-and-cutover`
- Scenario: `SCN-039-052 Stress profile meets latency NFR`
- Test: `tests/stress/recommendations_test.go::TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests`
- Platform: Linux, Docker-backed disposable test stack managed by `./smackerel.sh test stress`
- Source context date: 2026-05-04

## Error Output
```text
$ timeout 1800 ./smackerel.sh test stress
Container smackerel-test-postgres-1 Healthy
Container smackerel-test-nats-1 Healthy
Container smackerel-test-smackerel-ml-1 Healthy
Container smackerel-test-smackerel-core-1 Healthy
Health stress test passed with 25/25 successful requests
Search stress test passed: all queries completed under 3000ms with 1100 artifacts
go-stress: running readiness canary
=== RUN   TestStressReadinessCanary_Live
--- PASS: TestStressReadinessCanary_Live (2.07s)
PASS
go-stress: readiness canary passed
=== RUN   TestConcurrentInvocationIsolation_BS018
--- PASS: TestConcurrentInvocationIsolation_BS018 (0.51s)
=== RUN   TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests
    recommendations_test.go:169: stress: zero samples collected - workers never produced any observations
--- FAIL: TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests (60.21s)
FAIL
Command exited with code 1
Exit Code: 1
```

## Root Ownership
The residual is classified under `specs/039-recommendations-engine` because shared readiness is green before the failure and the failing workload is the feature 039 stress contract for `SCN-039-052`. The initial code ownership surface is the recommendation stress harness plus the reactive recommendation API path it exercises:

- `tests/stress/recommendations_test.go`
- `internal/recommendation/reactive/engine.go`
- `internal/recommendation/provider/runtime_registry_e2e.go`
- `internal/recommendation/provider/fixture_integration.go`
- `internal/recommendation/store/`
- `cmd/core/` recommendation route wiring

Precise technical root cause remains open for the implementation owner. Source inspection shows the zero-sample terminal condition can occur when concurrent workers exit without enqueueing samples, especially through the current deadline/timeout branch that returns without recording an observation. The owner must reproduce the red state, determine whether the endpoint is hanging under concurrency, the stress harness is dropping timeout observations, the test-stack provider/runtime build is mismatched for stress, or another recommendation-owned path is responsible.

## Related
- Parent feature: `specs/039-recommendations-engine/`
- Routed from: `specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness/`
- Parent scenario: `SCN-039-052`
- Parent requirement: `R-032` reactive recommendation request p95 latency
- Parent acceptance criterion: `AC-17` 50 concurrent recommendation requests across 5 minutes
