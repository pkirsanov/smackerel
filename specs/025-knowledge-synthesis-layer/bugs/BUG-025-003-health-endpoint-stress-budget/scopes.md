# Scopes: BUG-025-003 Health endpoint stress budget

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Scope 1: Restore knowledge health stress budget

**Status:** In Progress
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-025-003 health endpoint stress budget

  Scenario: BUG-025-003-SCN-001 Rapid health checks stay within budget
    Given the disposable stress stack is healthy
    And the Go stress readiness canary has passed
    And the knowledge layer is enabled
    When the stress suite performs 25 rapid authenticated GET /api/health calls
    Then each call satisfies the protected stress latency budget
    And every call returns HTTP 200

  Scenario: BUG-025-003-SCN-002 Knowledge section contract is preserved
    Given the knowledge layer has health stats available
    When an authenticated caller requests GET /api/health
    Then the response includes the knowledge section with concept_count, entity_count, synthesis_pending, and last_synthesis_at when available
    And existing health fields remain present

  Scenario: BUG-025-003-SCN-003 Slow knowledge stats cannot serialize rapid health checks
    Given the knowledge health stats path is slow, cold, or cache-expired
    When multiple health checks arrive rapidly
    Then the health endpoint avoids serial multi-second work across requests
    And the regression test fails if the slow path can push rapid health checks beyond the stress budget
```

### Implementation Plan
1. Reproduce the full-stress red condition with `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress` before source edits.
2. Confirm the run passes shared readiness and BUG-039 recommendation stress before the knowledge health timing failure.
3. Measure whether the timing breach is first-call cold cache, repeated calls, knowledge stats query duration, cache lock behavior, or broader health probe fan-out.
4. Apply the smallest knowledge-owned fix while preserving the authenticated knowledge section and existing health fields.
5. Add an adversarial regression for slow or cache-expired knowledge health stats behavior.
6. Run full stress plus unit, integration, E2E, regression-quality, artifact lint, traceability guard, and validate-owned transition checks.

### Change Boundary
Allowed file families:
- `internal/api/health.go`
- `internal/api/health_test.go`
- `internal/knowledge/store.go`
- `tests/stress/knowledge_stress_test.go`
- `tests/e2e/knowledge_health_test.go`
- `specs/025-knowledge-synthesis-layer/bugs/BUG-025-003-health-endpoint-stress-budget/**`

Excluded file families:
- `internal/recommendation/**` and `tests/stress/recommendations_test.go`
- BUG-031 shared stress readiness lifecycle and Docker/runtime files
- `config/generated/**`
- Docker Compose files unless a separately owned shared readiness bug is opened
- parent 025 certification fields unless `bubbles.validate` owns the state transition

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-025-003-01 | Red-stage knowledge health stress reproduction after readiness | stress | `tests/stress/knowledge_stress_test.go` | Full stress reaches `TestStressReadinessCanary_Live` pass and BUG-039 recommendation stress green evidence, then reproduces the knowledge health timing failure before code changes | BUG-025-003-SCN-001 |
| T-BUG-025-003-02 | Regression E2E / stress: rapid health checks stay inside budget | Regression E2E / stress | `tests/stress/knowledge_stress_test.go` | `TestKnowledge_HealthEndpointIncludesKnowledgeSection` runs 25 rapid `/api/health` calls, returns HTTP 200, and satisfies the accepted latency budget | BUG-025-003-SCN-001 |
| T-BUG-025-003-03 | Regression E2E: knowledge section present | Regression E2E / e2e-api | `tests/e2e/knowledge_health_test.go` | `TestKnowledgeHealth_SectionPresent` proves live `/api/health` includes the knowledge section when enabled | BUG-025-003-SCN-002 |
| T-BUG-025-003-04 | Unit: health knowledge section and cache behavior | unit | `internal/api/health_test.go` | `TestSCN02524_HealthIncludesKnowledgeSection`, `TestHealthKnowledgeCache`, and related health tests preserve the knowledge section and cache behavior | BUG-025-003-SCN-002 |
| T-BUG-025-003-05 | Adversarial slow knowledge stats health checks | unit or integration | `internal/api/health_test.go` | Slow or cache-expired knowledge stats cannot serialize rapid health checks into multi-second latency | BUG-025-003-SCN-003 |
| T-BUG-025-003-06 | Broader E2E regression suite | e2e-api | `./smackerel.sh test e2e` | Knowledge health E2E and related live-stack flows remain green after the fix | BUG-025-003-SCN-002 |
| T-BUG-025-003-07 | Full stress gate with residual owner check | stress | `./smackerel.sh test stress` | Full stress exits 0 or routes only separately owned residuals; BUG-039 recommendation stress remains green | BUG-025-003-SCN-001, BUG-025-003-SCN-003 |
| T-BUG-025-003-08 | Regression quality and no-bailout scan | quality | Changed health/stress regression files | Regression-quality guard finds no silent-pass bailout patterns and confirms adversarial bugfix signals | BUG-025-003-SCN-003 |

### Definition of Done
- [x] Root cause confirmed and documented for the health endpoint stress budget breach.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`; `./smackerel.sh test unit --go`; source inspection of `internal/api/health.go`
  - **Exit Code:** stress red 1 before fix; unit red 1 before fix; post-fix validation commands listed below
  - **Claim Source:** interpreted

  ```text
  Pre-fix stress exceeded the strict <2s health budget at 2.026203474s, 2.017783385s, and 2.003494752s while BUG-039 recommendation stress stayed green.
  Pre-fix unit regressions failed: TestHealthKnowledgeStressBudgetWithSlowProbesAndColdStats at about 2.605s and TestHealthKnowledgeReturnsStaleCacheWhenRefreshTimesOut at about 3.000s.
  Source inspection showed /api/health used the same 2s-edge auxiliary probe budget and waited on health auxiliary work before writing the authenticated response.
  ```

  - **Interpretation:** the breach was a health-path budget issue: optional ML/Ollama probes and cold or expired knowledge stats could consume the full stress SLA. The fix bounds auxiliary health work below the 2s budget, overlaps knowledge stats with the existing health probe fan-out, and falls back to stale knowledge cache when refresh exceeds the bounded context.
- [x] Pre-fix full-stress reproduction captured after shared readiness passes and after BUG-039 recommendation stress is green.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 1
  - **Claim Source:** executed

  ```text
  TestStressReadinessCanary_Live passed before workloads.
  TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests passed with p95 1.156118085s and unexpected rate 0.00%.
  TestKnowledge_HealthEndpointIncludesKnowledgeSection failed: health calls took 2.026203474s, 2.017783385s, and 2.003494752s, expected < 2s.
  ```
- [x] BUG-025-003-SCN-001 Rapid health checks stay within budget.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  === RUN   TestKnowledge_HealthEndpointIncludesKnowledgeSection
  --- PASS: TestKnowledge_HealthEndpointIncludesKnowledgeSection (1.62s)
  === RUN   TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests
  --- PASS: TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests
  PASS
  Exit Code: 0
  ```

  - **Interpretation:** the protected 25-call authenticated `/api/health` stress regression is green, and the previously protected BUG-039 recommendation stress remains green in the same full stress run.
- [x] BUG-025-003-SCN-002 Knowledge section contract is preserved.
  - **Phase:** implement
  - **Command:** `./smackerel.sh test unit --go`; `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e`
  - **Exit Code:** 0 for both commands
  - **Claim Source:** executed

  ```text
  Go unit passed, including internal/api health tests.
  E2E passed, including TestKnowledgeHealth_SectionPresent and TestKnowledgeHealth_ExistingFieldsPreserved.
  ```

  - **Interpretation:** authenticated callers still receive the knowledge section, existing health fields are preserved, and unauthenticated hiding behavior remains covered by the existing unit suite.
- [x] BUG-025-003-SCN-003 Slow knowledge stats cannot serialize rapid health checks.
  - **Phase:** implement
  - **Command:** `./smackerel.sh test unit --go`
  - **Exit Code:** 1 before fix for the new regressions; 0 after fix
  - **Claim Source:** executed

  ```text
  RED: TestHealthKnowledgeStressBudgetWithSlowProbesAndColdStats failed at about 2.605s.
  RED: TestHealthKnowledgeReturnsStaleCacheWhenRefreshTimesOut failed at about 3.000s.
  GREEN: ./smackerel.sh test unit --go exited 0 after the bounded parallel health-path fix.
  ```

  - **Interpretation:** slow cold stats and slow expired-cache refreshes are no longer allowed to push authenticated health responses beyond the stress budget.
- [x] Adversarial regression case exists and would fail if slow knowledge health behavior or cache/serialization regression returns.
  - **Phase:** implement
  - **Command:** `./smackerel.sh test unit --go`; `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/api/health_test.go tests/stress/knowledge_stress_test.go tests/e2e/knowledge_health_test.go`
  - **Exit Code:** unit red 1 before fix, unit green 0 after fix, regression-quality 0
  - **Claim Source:** executed

  ```text
  TestHealthKnowledgeStressBudgetWithSlowProbesAndColdStats simulates slow optional probes plus cold knowledge stats and fails if the handler takes >= 2s.
  TestHealthKnowledgeReturnsStaleCacheWhenRefreshTimesOut simulates an expired stale cache plus slow refresh and fails if refresh blocks the response past the budget.
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s); adversarial signals detected in internal/api/health_test.go and tests/e2e/knowledge_health_test.go.
  ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`; `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e`; `./smackerel.sh test unit --go`
  - **Exit Code:** 0 for all three commands after fix
  - **Claim Source:** executed

  ```text
  Stress: TestKnowledge_HealthEndpointIncludesKnowledgeSection passed in 1.62s.
  E2E: TestKnowledgeHealth_SectionPresent and TestKnowledgeHealth_ExistingFieldsPreserved passed.
  Unit: slow-probe/cold-stats and stale-cache timeout regressions passed.
  ```

  - **Interpretation:** live stress covers the rapid-call latency contract, live E2E covers the authenticated knowledge-section contract, and unit regressions cover the slow-path timeout and stale-cache behavior introduced by the fix.
- [x] Broader E2E regression suite passes.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain ./smackerel.sh test e2e`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  Shell E2E passed 35/35.
  Go E2E packages passed, including tests/e2e, tests/e2e/agent, and tests/e2e/drive.
  PASS: go-e2e
  ```
- [x] Full stress gate passes or reports only separately routed residuals with owner evidence after the knowledge health stress fix.
  - **Phase:** implement
  - **Command:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  Full stress passed.
  TestKnowledge_HealthEndpointIncludesKnowledgeSection passed in 1.62s.
  TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests passed with p95 1.156118085s, budget 10s, unexpected rate 0.00%.
  ```
- [x] Regression tests contain no silent-pass bailout patterns.
  - **Phase:** implement
  - **Command:** `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/api/health_test.go tests/stress/knowledge_stress_test.go tests/e2e/knowledge_health_test.go`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  BUBBLES REGRESSION QUALITY GUARD
  Bugfix mode: true
  Files scanned: 3
  Files with adversarial signals: 2
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  ```
- [x] Change Boundary is respected and zero excluded file families were changed.
  - **Phase:** implement
  - **Command:** scoped changed-file inspection for BUG-025-003-owned files
  - **Exit Code:** n/a
  - **Claim Source:** interpreted

  ```text
  Implementation-owned code/test changes:
  - internal/api/health.go
  - internal/api/health_test.go
  - internal/api/knowledge_test.go

  Evidence/artifact changes are confined to:
  - specs/025-knowledge-synthesis-layer/bugs/BUG-025-003-health-endpoint-stress-budget/**
  ```

  - **Interpretation:** the stress budget, BUG-039 recommendation stress, BUG-031 shared readiness lifecycle, generated config, Docker Compose files, and unrelated feature packages were not changed for this bug fix.
- [ ] Artifact lint, traceability guard, and state-transition guard pass before certification.
- [ ] Bug marked as Fixed in `bug.md` only after validate-owned certification evidence is recorded.

### Test Phase Evidence Addendum - 2026-05-05T08:37:23Z

**Phase:** test  
**Claim Source:** executed

| Command | Exit Code | Evidence Summary |
|---|---:|---|
| `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/api/health_test.go tests/stress/knowledge_stress_test.go tests/e2e/knowledge_health_test.go` | 0 | 0 violations, 0 warnings; adversarial signals detected in 2 files. |
| `timeout 600 ./smackerel.sh test unit --go` | 0 | Go unit passed, including `internal/api` health regression coverage for slow probes/cold stats and stale-cache timeout behavior. |
| `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress` | 0 | Full stress passed. `TestKnowledge_HealthEndpointIncludesKnowledgeSection` logged `Knowledge stats: concepts=0, entities=0, pending=1100` and passed; BUG-039 recommendation stress stayed green. |
| `COMPOSE_PROGRESS=plain timeout 600 ./smackerel.sh test e2e --go-run '^TestKnowledgeHealth_(SectionPresent|ExistingFieldsPreserved)$'` | 0 after disposable-stack cleanup | Targeted health E2E passed with `TestKnowledgeHealth_SectionPresent` and `TestKnowledgeHealth_ExistingFieldsPreserved`. |

**Interpretation:** the test owner independently rechecked BUG-025-003-SCN-001 through BUG-025-003-SCN-003. The protected health stress contract remains intact: 25 rapid authenticated `/api/health` calls, strict `<2s` per-call budget, and parsed/logged knowledge stats on the first response when present. No validate-owned DoD item, scope status, bug fixed status, or certification field is claimed by this test addendum.

### Regression Phase Evidence Addendum - 2026-05-05T09:11:04Z

**Phase:** regression  
**Claim Source:** executed

| Command | Exit Code | Evidence Summary |
|---|---:|---|
| `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/api/health_test.go tests/stress/knowledge_stress_test.go tests/e2e/knowledge_health_test.go` | 0 | 0 violations, 0 warnings; adversarial signals detected in 2 files. |
| `timeout 600 ./smackerel.sh test unit --go` | 0 | Go unit passed, including `internal/api` health regression coverage for slow probes/cold stats and stale-cache timeout behavior. |
| `timeout 600 ./smackerel.sh test unit --python` | 0 | Python unit passed with 407 tests and 1 existing warning. |
| `COMPOSE_PROGRESS=plain timeout 900 ./smackerel.sh test integration` | 0 | Integration passed across live-stack packages; `/api/health` response included the `knowledge` object. |
| `COMPOSE_PROGRESS=plain timeout 1200 ./smackerel.sh test e2e` | 0 | Shell E2E 35/35 passed; health-specific Go E2E tests passed and preserved existing health fields. |
| `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress` | 0 | Full stress passed. Health stress reported 25/25 successful requests, `TestKnowledge_HealthEndpointIncludesKnowledgeSection` passed, and BUG-039 recommendation stress stayed green. |
| `timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh specs/025-knowledge-synthesis-layer/bugs/BUG-025-003-health-endpoint-stress-budget --verbose` | 0 | Cross-spec inventory completed; no route/endpoint collisions detected. |

**Interpretation:** `bubbles.regression` found no regression in BUG-025-003-SCN-001 through BUG-025-003-SCN-003 or in the broader integration/E2E/stress surface. The repo-standard command surface has no numeric line-coverage command, so this phase does not claim a numeric coverage delta; scenario coverage, adversarial regression coverage, and no-bailout checks were verified instead. No validate-owned DoD item, scope status, bug fixed status, or certification field is claimed by this regression addendum.

### Ownership Routing
Implementation owner: `bubbles.implement` for red-stage reproduction, root cause isolation, code/test fix, and regression evidence.

Test owner: `bubbles.test` for targeted and broad repo-standard validation.

Validation owner: `bubbles.validate` for final guard interpretation and bug certification.