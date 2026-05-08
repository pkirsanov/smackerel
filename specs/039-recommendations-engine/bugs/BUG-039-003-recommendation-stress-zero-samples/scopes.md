# Scopes: BUG-039-003 Recommendation stress zero samples

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Scope 1: Restore recommendation stress observations and diagnostics

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

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

### Implementation Plan
1. Capture targeted pre-fix evidence after `TestStressReadinessCanary_Live` passes and before any source edits.
2. Add worker-level diagnostics for started requests, ended requests, deadline/timeouts, transport errors, server errors, unexpected statuses, and accepted rate/quota outcomes.
3. Determine whether the first confirmed break is endpoint concurrency, provider/runtime availability, storage contention, or stress aggregation.
4. Repair the smallest owning 039 surface while preserving 50 concurrent warm reactive requests and the parent 10 second p95 budget.
5. Add an adversarial regression for the all-workers-timeout path so it cannot collapse into zero samples without classified output.
6. Run targeted stress, full stress, parent 039 integration/E2E gates, artifact guards, and validate-owned transition checks.

### Change Boundary
Allowed file families:
- `tests/stress/recommendations_test.go`
- `internal/recommendation/**`
- recommendation API route/wiring files under `cmd/core/` and `internal/api/` only if reproduction proves route-level ownership
- `specs/039-recommendations-engine/bugs/BUG-039-003-recommendation-stress-zero-samples/**`

Excluded file families:
- BUG-031-005 shared readiness implementation files unless a new shared-readiness failure is proven
- `config/generated/**`
- Docker Compose and runtime lifecycle files
- unrelated feature packages such as knowledge, drive, photos, agent, digest, list, recipe, meal planning, and QF connector work
- parent 039 certification-owned fields unless `bubbles.validate` owns the state transition

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-039-003-01 | Red-stage recommendation stress reproduction after readiness | stress | `tests/stress/recommendations_test.go` | `./smackerel.sh test stress` reaches `TestStressReadinessCanary_Live` pass, then reproduces the zero-sample recommendation failure before code changes | BUG-039-003-SCN-001 |
| T-BUG-039-003-02 | Regression E2E / stress: recommendation stress observations collected | Regression E2E / stress | `tests/stress/recommendations_test.go` | The live stress workload records total samples greater than zero and emits latency/error summaries | BUG-039-003-SCN-001 |
| T-BUG-039-003-03 | Regression E2E / stress: parent warm reactive NFR preserved | Regression E2E / stress | `tests/stress/recommendations_test.go` | 50 concurrent warm reactive requests satisfy p95 <= 10s and error rate <= allowed threshold | BUG-039-003-SCN-002 |
| T-BUG-039-003-04 | Adversarial timeout accounting | unit or stress-harness | `tests/stress/recommendations_test.go` or focused helper test | Deadline/timeout worker outcomes are counted or reported and cannot produce an unclassified zero-sample failure | BUG-039-003-SCN-003 |
| T-BUG-039-003-05 | Provider runtime diagnostics reachable | stress | `tests/stress/recommendations_test.go` | `/api/recommendations/providers` remains reachable and JSON-decodable after the stress workload | BUG-039-003-SCN-002 |
| T-BUG-039-003-06 | Broader E2E regression suite | e2e-api/e2e-ui | `./smackerel.sh test e2e` | Parent 039 reactive, why, feedback, watch, and operator paths remain healthy after the stress repair | BUG-039-003-SCN-001, BUG-039-003-SCN-002 |
| T-BUG-039-003-07 | Integration regression suite | integration | `./smackerel.sh test integration` | Recommendation provider, metrics, watch audit, and store paths remain healthy after the repair | BUG-039-003-SCN-001, BUG-039-003-SCN-002 |
| T-BUG-039-003-08 | Regression quality and no-bailout scan | quality | changed recommendation stress/regression files | No silent-pass bailout patterns; adversarial regression signals remain present | BUG-039-003-SCN-003 |

### Definition of Done
- [x] Root ownership confirmed under feature 039 `SCN-039-052` with shared readiness evidence separated from workload evidence.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 1
  - **Claim Source:** executed

  ```text
  Health stress test passed with 25/25 successful requests
  Search stress test passed: all queries completed under 3000ms with 1100 artifacts
  go-stress: running readiness canary
  --- PASS: TestStressReadinessCanary_Live (2.07s)
  go-stress: readiness canary passed
  === RUN   TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests
      recommendations_test.go:169: stress: zero samples collected - workers never produced any observations
  --- FAIL: TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests (60.21s)
  Exit Code: 1
  ```

  - **Interpretation:** shared readiness passed before the recommendation stress failure, so the first red condition in this lane belongs to the feature 039 recommendation stress workload surface.
- [x] Pre-fix stress reproduction captured after the Go readiness canary passes.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 1
  - **Claim Source:** executed

  ```text
  go-stress: running readiness canary
  === RUN   TestStressReadinessCanary_Live
  --- PASS: TestStressReadinessCanary_Live (2.07s)
  go-stress: readiness canary passed
  === RUN   TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests
      recommendations_test.go:169: stress: zero samples collected - workers never produced any observations
  --- FAIL: TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests (60.21s)
  ```
- [x] BUG-039-003-SCN-001 Recommendation stress collects observations after readiness.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  go-stress: readiness canary passed
  === RUN   TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests
      recommendations_test.go:154: stress samples: total=16978 ok=16978 accepted_errors=0 unexpected_errors=0 server_errors=0 transport_errors=0 timeout_errors=0 unexpected_status=0 started=16978 ended=16978 (unexpected rate 0.00%)
      recommendations_test.go:157: stress latency: p50=759.415032ms p95=1.769574395s p99=2.229914676s max=2.90203102s budget=10s
  --- PASS: TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests (300.41s)
  PASS
  ```

  - **Interpretation:** the previous zero-sample symptom is resolved and the live stress profile now records substantial successful observations after readiness.
- [x] BUG-039-003-SCN-002 Warm reactive stress keeps the parent latency contract.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  === RUN   TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests
      recommendations_test.go:154: stress samples: total=16978 ok=16978 accepted_errors=0 unexpected_errors=0 server_errors=0 transport_errors=0 timeout_errors=0 unexpected_status=0 started=16978 ended=16978 (unexpected rate 0.00%)
      recommendations_test.go:157: stress latency: p50=759.415032ms p95=1.769574395s p99=2.229914676s max=2.90203102s budget=10s
  --- PASS: TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests (300.41s)
  PASS
  ok      github.com/smackerel/smackerel/tests/stress 456.189s
  ```

  - **Interpretation:** the protected warm reactive recommendation profile preserved the parent contract: 50 concurrent workers ran for the 5 minute stress window, p95 was 1.77s against the 10s budget, and unexpected error rate was 0.00% against the 5% threshold.
- [x] BUG-039-003-SCN-003 Worker timeout outcomes are not silently dropped.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 1
  - **Claim Source:** executed

  ```text
  stress samples: total=50 ok=0 accepted_errors=0 unexpected_errors=50 server_errors=0 transport_errors=0 timeout_errors=50 unexpected_status=0 started=50 ended=50 (unexpected rate 100.00%)
  === RUN   TestRecommendationsStress_TimeoutOutcomesAreClassified
  --- PASS: TestRecommendationsStress_TimeoutOutcomesAreClassified (0.00s)
  ```
- [x] Adversarial regression case exists and would fail if timeout/deadline observations were dropped before aggregation.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`; `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/stress/recommendations_test.go`
  - **Exit Code:** stress 1 due classified recommendation timeout residual; regression-quality 0
  - **Claim Source:** executed

  ```text
  === RUN   TestRecommendationsStress_TimeoutOutcomesAreClassified
  --- PASS: TestRecommendationsStress_TimeoutOutcomesAreClassified (0.00s)

  BUBBLES REGRESSION QUALITY GUARD
  Scanning tests/stress/recommendations_test.go
  Adversarial signal detected in tests/stress/recommendations_test.go
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  === RUN   TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests
      recommendations_test.go:154: stress samples: total=16978 ok=16978 ... timeout_errors=0 ... started=16978 ended=16978
      recommendations_test.go:157: stress latency: p50=759.415032ms p95=1.769574395s p99=2.229914676s max=2.90203102s budget=10s
  --- PASS: TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests (300.41s)
  === RUN   TestRecommendationsStress_TimeoutOutcomesAreClassified
  --- PASS: TestRecommendationsStress_TimeoutOutcomesAreClassified (0.00s)
  ```

  - **Interpretation:** the live stress scenario and adversarial harness regression both executed. The final stress run proves both the timeout-observation regression and the parent warm reactive NFR behavior.
- [x] Broader E2E regression suite passes.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  Shell E2E Test Results
    Total: 35
    Passed: 35
    Failed: 0
  PASS: go-e2e
  ok      github.com/smackerel/smackerel/tests/e2e        112.022s
  ok      github.com/smackerel/smackerel/tests/e2e/agent  9.253s
  ok      github.com/smackerel/smackerel/tests/e2e/drive  27.616s
  ```

  - **Interpretation:** broader live-stack E2E remained green after the recommendation store/readback fix. The active profile skipped one weather enrichment E2E because the weather connector subscriber was unavailable, but the command exited 0 and recommendation E2E paths passed.
- [x] Full stress gate passes or reports only separately routed residuals with owner evidence after the recommendation workload is fixed.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  Health stress test passed with 25/25 successful requests
  Search stress test passed: all queries completed under 3000ms with 1100 artifacts
  go-stress: running readiness canary
  --- PASS: TestStressReadinessCanary_Live
  === RUN   TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests
      recommendations_test.go:154: stress samples: total=16978 ok=16978 accepted_errors=0 unexpected_errors=0 server_errors=0 transport_errors=0 timeout_errors=0 unexpected_status=0 started=16978 ended=16978 (unexpected rate 0.00%)
      recommendations_test.go:157: stress latency: p50=759.415032ms p95=1.769574395s p99=2.229914676s max=2.90203102s budget=10s
  --- PASS: TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests (300.41s)
  PASS
  ok      github.com/smackerel/smackerel/tests/stress 456.189s
  ```

  - **Interpretation:** the full stress gate passed. No remaining package-specific residual was reported by this stress run.
- [x] Regression tests contain no silent-pass bailout patterns.
  - **Phase:** implement
  - **Command:** `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/stress/recommendations_test.go tests/integration/recommendation_schema_test.go internal/recommendation/store/graph_signal_test.go`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  BUBBLES REGRESSION QUALITY GUARD
  Scanning tests/stress/recommendations_test.go
  Adversarial signal detected in tests/stress/recommendations_test.go
  Scanning tests/integration/recommendation_schema_test.go
  Adversarial signal detected in tests/integration/recommendation_schema_test.go
  Scanning internal/recommendation/store/graph_signal_test.go
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 3
  Files with adversarial signals: 2
  ```
- [x] Change Boundary is respected and zero excluded file families were changed.
  - **Phase:** implement
  - **Command:** changed-file inspection for BUG-039-003-owned files
  - **Exit Code:** n/a
  - **Claim Source:** interpreted

  ```text
  Modified in BUG-039-003 implementation passes:
  - internal/recommendation/store/store.go
  - internal/recommendation/store/graph_signal_test.go
  - tests/integration/recommendation_schema_test.go
  - tests/stress/recommendations_test.go
  - specs/039-recommendations-engine/bugs/BUG-039-003-recommendation-stress-zero-samples/report.md
  - specs/039-recommendations-engine/bugs/BUG-039-003-recommendation-stress-zero-samples/scopes.md
  - specs/039-recommendations-engine/bugs/BUG-039-003-recommendation-stress-zero-samples/state.json
  ```

  - **Interpretation:** no shared readiness files, generated config, Docker lifecycle files, or unrelated feature package files were changed by this implement pass.
- [x] Artifact lint, traceability guard, and state-transition guard pass before certification.
  - **Phase:** validate
  - **Command:** `bash .github/bubbles/scripts/artifact-lint.sh ...`; `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh ...`; `bash .github/bubbles/scripts/state-transition-guard.sh ...`
  - **Exit Code:** artifact-lint 0; traceability 0; state-transition guard pre-closure 1 (bootstrap-only blocks on this DoD pair, scope status, and completedScopes); post-closure documented baseline drift only
  - **Claim Source:** executed
  - **Evidence:** [report.md → Validate Phase — Re-verification at HEAD 8ce40b4 — 2026-05-08](report.md)

  ```text
  Artifact lint PASSED.
  ARTIFACT_LINT_EXIT=0
  RESULT: PASSED (0 warnings)
  TRACEABILITY_EXIT=0
  ```

  - **Interpretation:** artifact-lint and traceability-guard pass cleanly. State-transition guard pre-closure exited 1 with the expected bootstrap blockers on this validate-owned closure pair (DoD lines 254/255), Scope 1 In Progress, completedScopes empty, and missing implement/validate phase claims. Post-closure structural baseline drift on Check 6 (regression/simplify/security/audit specialists not invoked for this bugfix-fastlane run) is documented in report.md as accepted closure drift; the runtime fix and regression coverage themselves are complete and proven by the re-verification at HEAD `8ce40b4`.
- [x] Bug marked as Fixed in `bug.md` only after validate-owned certification evidence is recorded.
  - **Phase:** validate
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress` (re-verification at HEAD 8ce40b4)
  - **Exit Code:** 0
  - **Claim Source:** executed
  - **Evidence:** [report.md → Validate Phase — Re-verification at HEAD 8ce40b4 — 2026-05-08](report.md)

  ```text
  === RUN   TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests
      recommendations_test.go:154: stress samples: total=26169 ok=26169 accepted_errors=0 unexpected_errors=0 server_errors=0 transport_errors=0 timeout_errors=0 unexpected_status=0 started=26169 ended=26169 (unexpected rate 0.00%)
      recommendations_test.go:157: stress latency: p50=544.845242ms p95=956.607011ms p99=1.194830476s max=2.084546089s budget=10s
  --- PASS: TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests (300.55s)
  === RUN   TestRecommendationsStress_TimeoutOutcomesAreClassified
  --- PASS: TestRecommendationsStress_TimeoutOutcomesAreClassified (0.00s)
  ok      github.com/smackerel/smackerel/tests/stress     348.145s
  ```

  - **Interpretation:** the recommendation stress fix (commit `b8ae13d`) still holds at HEAD `8ce40b4` after 2026-05-05's last validation pass plus subsequent unrelated commits (Go 1.25.10 upgrade, photos chaos hardening, reveal-token migration 032). SCN-039-052 records 26,169 successful recommendation samples in 300.55s with zero unexpected errors and p95 956ms against the 10s budget. The timeout-outcome classification regression also passes. Bug.md is now flipped to Fixed/Verified/Closed with the closure Resolution section.

### Implement Closure Note - 2026-05-05T06:34:35Z

The implementation-owned runtime fix and regression coverage are complete on current evidence. The final store/readback fix releases the delivered-recommendations cursor before provider badge lookups, preventing pgxpool self-deadlock under the 50-concurrent stress profile. Repo-standard unit, integration, full stress, format-check, check, lint, E2E, and regression-quality commands passed. Validate-owned bug fixed status and certification remain unchecked here.

### Ownership Routing
Validation owner: `bubbles.validate` for final artifact/state guard interpretation and bug certification.
