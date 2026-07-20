# Scopes: BUG-003-002 Topic momentum star aggregation

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore canonical topic star aggregation

**Status:** In Progress
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-003-002 derive topic stars from canonical relationships

  Scenario: BUG-003-002-SCN-001 Canonical star aggregation updates momentum
    Given canonical migrations created topics without a star_count column
    And one topic has no linked starred artifacts
    And another topic has two linked starred artifacts and one linked unstarred artifact
    When the actual lifecycle momentum update runs
    Then both topics are updated without a missing-column error
    And momentum reflects exactly the linked starred artifacts

  Scenario: BUG-003-002-SCN-002 Lifecycle query failures remain observable
    Given the lifecycle database pool cannot execute the topic query
    When the scheduler runs the topic momentum job
    Then the scheduler logs topic momentum update failed
    And does not log topic momentum updated for that run

  Scenario: BUG-003-002-SCN-003 Existing topic lifecycle surface remains available
    Given the disposable full test stack contains topic lifecycle fixtures
    When the targeted topic lifecycle shell flow runs
    Then the topics surface renders the lifecycle topics and momentum values
```

### Implementation Plan

1. Add the real PostgreSQL integration regression first and record the expected RED failure against the current query.
2. Replace `topics.star_count` with an aggregate over `artifacts.user_starred` joined through canonical `BELONGS_TO` edges.
3. Add scheduler regression coverage proving query failures remain logged as failures and never as successes.
4. Run focused topic lifecycle unit, integration, and targeted E2E regressions through `./smackerel.sh`.
5. Run check, lint, format, artifact, traceability, reality, and adversarial regression gates.

### Implementation Files

- `internal/topics/lifecycle.go`

### Change Boundary

Allowed:

- `internal/topics/lifecycle.go`
- `internal/scheduler/jobs_test.go`
- `tests/integration/topic_lifecycle_momentum_test.go`
- `tests/e2e/test_topic_lifecycle.sh` (execution only; file remains unchanged)
- `specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/**`

Excluded:

- all database migrations and schema definitions
- all deploy adapters, deployment manifests, and host state
- all release-train and feature-flag configuration
- all secrets and generated configuration
- scheduler cadence and health contracts
- all unrelated source, documentation, and tests

### Test Plan

| ID | Test Name | Category | File/Location | Command | Live System | Scenario |
|---|---|---|---|---|---|---|
| T-BUG-003-002-01 | BUG-003-002-SCN-001 Canonical star aggregation updates momentum | `integration` | `tests/integration/topic_lifecycle_momentum_test.go::TestTopicLifecycleMomentumFromPersistedStars` | `./smackerel.sh test integration-light --go-run '^TestTopicLifecycleMomentumFromPersistedStars$'` | Yes | BUG-003-002-SCN-001 |
| T-BUG-003-002-02 | Functional: R-208 momentum boundaries | `functional` | `internal/topics/lifecycle_test.go` | `./smackerel.sh test unit --go --go-run '^Test(CalculateMomentum|TransitionState)' --verbose` | No | BUG-003-002-SCN-001 |
| T-BUG-003-002-03 | BUG-003-002-SCN-002 Lifecycle query failures remain observable | `unit` | `internal/scheduler/jobs_test.go::TestTopicMomentumJob_LogsLifecycleQueryFailure` | `./smackerel.sh test unit --go --go-run '^TestTopicMomentumJob_LogsLifecycleQueryFailure$' --verbose` | No | BUG-003-002-SCN-002 |
| T-BUG-003-002-04 | Regression E2E: BUG-003-002-SCN-003 Existing topic lifecycle surface remains available | `e2e-api` | `tests/e2e/test_topic_lifecycle.sh` | `./smackerel.sh test e2e --shell-run test_topic_lifecycle.sh` | Yes | BUG-003-002-SCN-003 |
| T-BUG-003-002-05 | Broader unit regression | `unit` | Go unit suite | `./smackerel.sh test unit --go` | No | BUG-003-002-SCN-001, BUG-003-002-SCN-002 |

### Definition of Done

- [ ] Root cause is confirmed against canonical schema, star persistence, and topic relationship sources
- [ ] BUG-003-002-SCN-001 Canonical star aggregation updates momentum from zero and multiple persisted star relationships
- [ ] BUG-003-002-SCN-002 Lifecycle query failures remain observable as scheduler failures without a success log
- [ ] BUG-003-002-SCN-003 Existing topic lifecycle surface remains available through the targeted full-stack flow
- [ ] Pre-fix real PostgreSQL regression fails with the missing `topics.star_count` counterexample
- [ ] Lifecycle query derives explicit stars from linked `artifacts.user_starred` rows
- [ ] Zero-star, multiple-star, unstarred, and unrelated-star cases are asserted through the actual lifecycle query
- [ ] Query failures remain returned and scheduler failure logging remains honest
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Focused unit, functional, integration, and scheduler regressions pass
- [ ] Check, lint, and format gates pass with zero warnings
- [ ] Artifact lint, traceability, implementation-reality, and adversarial regression guards pass
- [ ] Change Boundary is respected and zero excluded file families were changed
- [ ] Documentation and bug packet match the implemented behavior

### Post-Scope Certification Gate (Not Scope DoD)

Route the packet now through the authorized `bugfix-fastlane` implementation/test chain so each remaining Scope 1 DoD item receives owner-recorded execution evidence. Only after every Scope 1 DoD item is complete and Scope 1 is marked Done may the packet proceed to `bubbles.validate` and `bubbles.audit` for independent certification. Validation and audit evidence remain owner-recorded phase-exit evidence; they do not gate Scope 1 completion and must not be self-certified by planning or implementation.
