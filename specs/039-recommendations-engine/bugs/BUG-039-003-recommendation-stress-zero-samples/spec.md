# Bug Specification: BUG-039-003 Recommendation stress zero samples

## Problem Statement
Feature 039 is already a full-delivery feature with `SCN-039-052` protecting the recommendation engine's warm reactive stress NFR. After BUG-031-005 repaired shared stress readiness, the stress command now reaches recommendation workload execution and fails inside the 039-owned stress profile with zero collected samples.

This bug lane preserves the parent 039 full-delivery context and isolates the residual to the recommendation workload owner. It does not reopen shared stress readiness and does not alter parent certification state.

## Outcome Contract
**Intent:** The recommendation stress profile produces meaningful workload observations after shared readiness succeeds, so feature 039 can prove or fail its warm reactive latency/error budget with actionable diagnostics.

**Success Signal:** `./smackerel.sh test stress` reaches `TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests`; the test records total samples greater than zero, emits p50/p95/p99/max latency and error-rate logs, and either passes `R-032` or fails with classified workload diagnostics.

**Hard Constraints:** Stress validation remains repo-CLI-owned through `./smackerel.sh`; the disposable test stack and SST-generated env remain the source of runtime values; the fix must not weaken `SCN-039-052`, skip the workload, lower concurrency, shorten the protected profile, or accept zero observations.

**Failure Condition:** The bug remains unresolved if 50 concurrent warm reactive requests can exit with zero observations, if worker deadline/transport failures are silently dropped from the aggregation, if the workload hangs without useful diagnostics, or if a test change hides the failure instead of proving the recommendation API behavior.

## Goals
- Preserve `SCN-039-052` as the owning scenario for the residual stress failure.
- Capture pre-fix red evidence after the Go readiness canary passes.
- Diagnose whether the failure lives in the reactive recommendation endpoint, provider/runtime registration under the stress stack, persistence/contention path, or stress aggregation logic.
- Add adversarial regression coverage so deadline/transport failures cannot collapse into a zero-sample diagnostic.
- Keep shared readiness, drive stress, agent stress, and parent feature certification surfaces outside this bug's implementation boundary unless fresh evidence proves a direct 039 dependency.

## Non-Goals
- Changing BUG-031-005 readiness canary behavior.
- Modifying shared Docker lifecycle, generated config, or the stress command lifecycle.
- Weakening the parent 039 `R-032` p95 latency budget or reducing the protected 50-concurrent warm workload.
- Editing parent 039 `state.json` certification fields in this classification pass.

## Requirements
- R-BUG-039-003-001: After shared stress readiness passes, the recommendation stress test must collect at least one observation for the warm reactive workload.
- R-BUG-039-003-002: The stress result must classify successes, accepted rate/quota outcomes, server errors, unexpected statuses, transport errors, and deadline/timeouts without silently discarding all worker outcomes.
- R-BUG-039-003-003: If the recommendation endpoint hangs or all workers time out, the stress output must include started request count, ended request count, error kind counts, and enough route/provider context to classify ownership.
- R-BUG-039-003-004: A green run must satisfy the parent `SCN-039-052` contract: 50 concurrent warm reactive requests for the protected window, p95 within the 10 second warm budget, and error rate within the allowed threshold.
- R-BUG-039-003-005: Regression tests must include an adversarial case that would fail if worker deadline/timeout observations were dropped before aggregation.
- R-BUG-039-003-006: No implementation may replace the live stress proof with internal mocks or request interception.

## User Scenarios (Gherkin)

```gherkin
Feature: BUG-039-003 recommendation stress observations

  Scenario: BUG-039-003-SCN-001 Recommendation stress collects observations after readiness
    Given the disposable stress stack is healthy
    And the Go stress readiness canary has passed for core, database, NATS, and auth
    When the recommendation stress profile runs 50 concurrent warm reactive requests
    Then the workload records at least one observation
    And the output includes total sample counts and latency/error summaries

  Scenario: BUG-039-003-SCN-002 Warm reactive stress keeps the parent latency contract
    Given fixture-backed recommendation providers and the reactive recommendation API are available in the stress stack
    When the warm reactive stress profile completes
    Then p95 latency is within the 10 second warm budget
    And unexpected server or transport error rate remains within the allowed threshold
    And provider runtime state remains reachable for diagnostics

  Scenario: BUG-039-003-SCN-003 Worker timeout outcomes are not silently dropped
    Given concurrent recommendation workers hit deadline, timeout, transport, server, or unexpected-status paths
    When the stress harness aggregates worker outcomes
    Then each outcome is counted or reported as a workload diagnostic
    And the test cannot fail only with zero samples and no classified observations
```

## Acceptance Criteria
- [ ] `BUG-039-003-SCN-001` has red and green stress evidence after shared readiness passes.
- [ ] `BUG-039-003-SCN-002` preserves the parent 039 `R-032` p95/error-budget stress contract.
- [ ] `BUG-039-003-SCN-003` includes adversarial coverage for deadline/timeout observation accounting.
- [ ] `./smackerel.sh test stress` either passes or reports only separately routed residuals after the recommendation workload is fixed.
- [ ] Artifact lint, traceability guard, and validate-owned certification gates pass before this bug is marked fixed or verified.
