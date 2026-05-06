# Bug Fix Design: BUG-039-003 Recommendation stress zero samples

## Root Cause Analysis

### Investigation Summary
The residual finding comes from BUG-031-005 stress validation after shared readiness was repaired. The observed command reached the disposable test stack, passed shell health/search, passed `TestStressReadinessCanary_Live`, passed agent DB/NATS stress, and then failed inside `tests/stress/recommendations_test.go::TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests` with zero observations.

Source inspection confirms the failing test is the parent 039 `SCN-039-052` stress profile. It warms `/api/recommendations/requests`, starts 50 workers, posts reactive recommendation requests through a shared HTTP client, aggregates successes and non-success outcomes from `samplesCh`, and fails at `totalSamples == 0`.

### Root Ownership
Primary owner: `specs/039-recommendations-engine`, scope `scope-06-observability-stress-and-cutover`, scenario `SCN-039-052`.

Primary code surfaces for the next owner:

- `tests/stress/recommendations_test.go`
- `internal/recommendation/reactive/engine.go`
- `internal/recommendation/provider/runtime_registry_e2e.go`
- `internal/recommendation/provider/fixture_integration.go`
- `internal/recommendation/store/`
- recommendation API route wiring in `cmd/core/` and `internal/api/`/`internal/web/` as applicable

### Confirmed Boundary
This is not classified as shared stress readiness. BUG-031-005 evidence shows the readiness canary passed before the residual, and unrelated agent DB/NATS plus drive stress reached their workload assertions. The remaining failure appears only in the recommendation workload package after readiness.

### Open Technical Cause
The precise code defect is not confirmed in this classification pass. The implementation owner must determine whether the failure is caused by one or more of these 039-owned paths:

- concurrent POST requests hanging until the stress HTTP client timeout window elapses;
- the stress harness returning on deadline/timeout errors without recording observations;
- recommendation providers or fixture registry not being available under the stress build/runtime profile;
- persistence, transaction, pool, or lock contention in the reactive recommendation path;
- route/auth/request body behavior that succeeds for warmup but stalls under concurrency.

## Fix Design

### Solution Approach
Start with reproduction under the repaired stress readiness path, then add diagnostics before changing behavior. The red-stage run should show the warmup result, started/ended request counts, classified error-kind counts, and whether worker exits happen through deadline/timeout, transport, server status, or another path.

The likely fix must preserve the parent stress contract while making the workload observable:

1. Capture a targeted red run of `./smackerel.sh test stress` with the recommendation failure after `TestStressReadinessCanary_Live` passes.
2. Add or strengthen regression coverage for the aggregation edge case where every worker exits through timeout/deadline before any observation is sent.
3. Diagnose whether endpoint concurrency or stress aggregation is the first broken contract.
4. Apply the smallest change in the owning 039 surface:
   - If the endpoint hangs, repair the reactive engine/store/provider path and keep the stress harness strict.
   - If observations are dropped, count deadline/timeout outcomes as workload diagnostics and fail with counts instead of zero samples.
   - If provider/runtime availability differs in stress, align the test stack provider contract without fabricating production providers.
5. Re-run the targeted stress workload, full stress gate, and the parent 039 broad validation gates required by the workflow.

### Alternative Approaches Considered
1. Treat as BUG-031-005 readiness: rejected because readiness is green before the residual failure.
2. Reduce stress concurrency, profile duration, or p95 budget: rejected because that would weaken `SCN-039-052`/`R-032` instead of fixing the workload.
3. Ignore deadline/timeout outcomes to avoid noisy stress failures: rejected because the current zero-sample failure is already caused by insufficient workload diagnostics.
4. Replace live stress with unit-only coverage: rejected because parent 039 requires live stress proof.

## Change Boundary

Allowed implementation surfaces:

- `tests/stress/recommendations_test.go` and focused recommendation stress diagnostics/regressions.
- Recommendation reactive API and engine surfaces under `internal/recommendation/**`, `internal/api/**`, and route construction only when reproduction proves they own the hang/failure.
- Recommendation provider fixture/runtime registration only when reproduction proves the stress stack lacks the required fixture-backed behavior.
- This bug packet's evidence and validation artifacts.

Protected surfaces:

- Shared stress readiness/lifecycle files owned by BUG-031-005.
- Generated config under `config/generated/**`.
- Docker Compose lifecycle unless a new shared readiness bug is opened with evidence.
- Parent 039 certification fields until `bubbles.validate` owns promotion.

## Regression Test Design

- Red-stage stress reproduction: full repo stress command reaches `TestStressReadinessCanary_Live` pass and then fails in `TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests` with zero samples.
- Adversarial aggregation regression: simulate or force deadline/timeout worker outcomes and prove they are counted or reported instead of silently returning before enqueueing observations.
- Live stress green proof: `TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests` records total samples greater than zero and satisfies p95/error-budget requirements against the disposable test stack.
- Broad regression: `./smackerel.sh test e2e`, `./smackerel.sh test integration`, and parent 039 relevant gates remain green or honestly route unrelated residuals.
- No-bailout scan: regression tests must not contain conditional returns that silently pass when the failure condition appears.

## Validation Plan

Owning sequence:

1. `bubbles.implement`: reproduce, diagnose, implement the minimal fix and regression tests.
2. `bubbles.test`: run targeted stress, full stress, integration, E2E, unit, regression-quality, and skip/bailout scans using repo-standard commands.
3. `bubbles.validate`: certify only after artifact lint, traceability guard, state-transition guard, and parent 039 compatibility checks pass.
