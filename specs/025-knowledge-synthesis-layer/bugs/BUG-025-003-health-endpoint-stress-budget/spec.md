# Bug Specification: BUG-025-003 Health endpoint stress budget

## Problem Statement
Feature 025 extends `/api/health` with a knowledge section that exposes concept count, entity count, pending synthesis count, and last synthesis timestamp. During the BUG-039 validation lane, the recommendation stress workload passed, but the full stress command still exited 1 because the knowledge health endpoint stress test reported rapid `/api/health` checks narrowly above the current 2 second budget.

This bug lane preserves the BUG-039 recommendation boundary and isolates the residual to the knowledge health endpoint. It does not reopen the closed empty-store stats or external URL extraction bugs.

## Outcome Contract
**Intent:** Authenticated `/api/health` remains a reliable live-stack health endpoint after the knowledge section is added, even under rapid stress checks.

**Success Signal:** `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress` no longer fails in `TestKnowledge_HealthEndpointIncludesKnowledgeSection`; the test observes HTTP 200 responses, preserves the knowledge section contract when enabled, and keeps the protected latency budget green while the already-fixed BUG-039 recommendation stress workload remains green.

**Hard Constraints:** Runtime validation stays repo-CLI-owned through `./smackerel.sh`; generated config under `config/generated/**` is not hand-edited; the fix must not remove the knowledge section, hide the knowledge section from authenticated callers, lower stress coverage by skipping rapid calls, or weaken the BUG-039 recommendation stress contract.

**Failure Condition:** The bug remains unresolved if full stress can still fail because rapid `/api/health` calls with the knowledge section exceed the current budget, if a fix hides the knowledge section instead of keeping it fast, if a test change silently accepts slow health calls, or if the recommendation stress workload regresses while this bug is fixed.

## Goals
- Classify the residual under `specs/025-knowledge-synthesis-layer` and Scope 8.
- Preserve the existing health knowledge-section contract from parent `SCN-025-23`.
- Capture red-stage evidence for the rapid `/api/health` timing failure after shared readiness passes and after the recommendation stress workload is green.
- Diagnose whether the timing breach is caused by endpoint implementation, knowledge health stats query behavior, cache behavior, live-stack cold state, or the stress contract itself.
- Add adversarial regression coverage so the issue cannot be hidden by a broad tolerance-only or bailout test change.

## Non-Goals
- Modifying BUG-039 recommendation runtime, tests, or certification artifacts.
- Reopening `BUG-025-001` or `BUG-025-002` unless fresh evidence proves the exact same failure recurred.
- Changing shared stress readiness, Docker lifecycle, or generated config.
- Removing the knowledge section from authenticated health responses.
- Weakening full-stress validation by skipping the health endpoint stress scenario.

## Requirements
- R-BUG-025-003-001: Rapid authenticated `/api/health` checks must stay within the protected stress latency budget while the knowledge layer is enabled.
- R-BUG-025-003-002: The health response must still include the knowledge section for authenticated callers when the knowledge layer is enabled.
- R-BUG-025-003-003: Existing core health fields and service topology must remain present for authenticated callers.
- R-BUG-025-003-004: If the knowledge health stats lookup is slow or unavailable, the endpoint must expose safe health behavior without turning health checks into multi-second serial work.
- R-BUG-025-003-005: Regression coverage must include an adversarial case that would fail if a slow knowledge stats path, cache miss, or serial health-check behavior reintroduces this budget breach.
- R-BUG-025-003-006: The final validation must keep BUG-039 recommendation stress green or clearly route any unrelated residual with owner evidence.

## User Scenarios (Gherkin)

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

## Acceptance Criteria
- [ ] `BUG-025-003-SCN-001` has red and green full-stress evidence after shared readiness passes.
- [ ] `BUG-025-003-SCN-002` proves the knowledge section remains present for authenticated health callers.
- [ ] `BUG-025-003-SCN-003` includes adversarial coverage for slow or cache-expired knowledge health behavior.
- [ ] `COMPOSE_PROGRESS=plain timeout 1800 ./smackerel.sh test stress` exits 0, or any remaining residual is outside BUG-025-003 and is routed with owner evidence.
- [ ] Artifact lint, traceability guard, and validate-owned certification gates pass before this bug is marked fixed or verified.