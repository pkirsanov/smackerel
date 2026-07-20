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

- [x] Root cause is confirmed against canonical schema, star persistence, and topic relationship sources

  **Phase:** implement
  **Executed:** YES (current session)
  **Command:** `printf '%s\n' 'IMPLEMENT INSPECTION: root cause and canonical contracts' 'base revision: f5f05450848630fe84c0a215429bdfc701c4bcd2' 'pre-fix lifecycle reference:' && git grep -n 't.star_count' f5f05450848630fe84c0a215429bdfc701c4bcd2 -- internal/topics/lifecycle.go && printf '%s\n' 'canonical star persistence:' && grep -n 'user_starred' internal/db/migrations/001_initial_schema.sql && printf '%s\n' 'canonical edge table:' && grep -n -A 8 'CREATE TABLE IF NOT EXISTS edges' internal/db/migrations/001_initial_schema.sql && printf '%s\n' 'canonical BELONGS_TO producers:' && grep -n 'BELONGS_TO' internal/graph/linker.go internal/connector/bookmarks/topics.go`
  **Exit Code:** 0
  **Claim Source:** executed
  **Evidence:** [report.md#canonical-root-cause-reconciliation](report.md#canonical-root-cause-reconciliation)

  ```text
  pre-fix lifecycle reference:
  f5f05450848630fe84c0a215429bdfc701c4bcd2:internal/topics/lifecycle.go:123: COALESCE(t.star_count, 0),
  canonical star persistence:
  35:    user_starred            BOOLEAN DEFAULT FALSE,
  canonical edge table:
  122:CREATE TABLE IF NOT EXISTS edges (
  123-    id          TEXT PRIMARY KEY,
  124-    src_type    TEXT NOT NULL,
  125-    src_id      TEXT NOT NULL,
  126-    dst_type    TEXT NOT NULL,
  127-    dst_id      TEXT NOT NULL,
  128-    edge_type   TEXT NOT NULL,
  canonical BELONGS_TO producer:
  internal/graph/linker.go:266: l.createEdge(ctx, "artifact", artifactID, "topic", topicID, "BELONGS_TO", 1.0)
  ```

  **Result:** PASS
- [ ] BUG-003-002-SCN-001 Canonical star aggregation updates momentum from zero and multiple persisted star relationships
- [ ] BUG-003-002-SCN-002 Lifecycle query failures remain observable as scheduler failures without a success log
- [ ] BUG-003-002-SCN-003 Existing topic lifecycle surface remains available through the targeted full-stack flow
- [ ] Pre-fix real PostgreSQL regression fails with the missing `topics.star_count` counterexample
- [x] Lifecycle query derives explicit stars from linked `artifacts.user_starred` rows

  **Phase:** implement
  **Executed:** YES (current session)
  **Command:** `printf '%s\n' 'IMPLEMENT INSPECTION: production aggregation query' 'required source: artifacts.user_starred' 'required relation: artifact -> topic BELONGS_TO' 'required cardinality: distinct artifact ids' && grep -n -A 17 -B 2 'COUNT(DISTINCT a.id)' internal/topics/lifecycle.go`
  **Exit Code:** 0
  **Claim Source:** executed
  **Evidence:** [report.md#production-query-reconciliation](report.md#production-query-reconciliation)

  ```text
  required source: artifacts.user_starred
  required relation: artifact -> topic BELONGS_TO
  required cardinality: distinct artifact ids
  123: COALESCE((SELECT COUNT(DISTINCT a.id)
  124- FROM edges e
  125- JOIN artifacts a ON a.id = e.src_id AND e.src_type = 'artifact'
  126- WHERE e.dst_type = 'topic' AND e.dst_id = t.id
  127- AND e.edge_type = 'BELONGS_TO' AND a.user_starred IS TRUE), 0)::int,
  130- EXTRACT(DAY FROM NOW() - COALESCE(t.last_active, t.created_at))::int
  131- FROM topics t
  132- `)
  133- if err != nil {
  134- return fmt.Errorf("query topics: %w", err)
  135- }
  ```

  **Result:** PASS
- [x] Zero-star, multiple-star, unstarred, and unrelated-star cases are asserted through the actual lifecycle query

  **Phase:** implement
  **Executed:** YES (current-session inspection)
  **Command:** `printf '%s\n' 'IMPLEMENT INSPECTION: persisted-star test contract' 'actual entrypoint: topics.NewLifecycle(pool).UpdateAllMomentum(ctx)' 'required cases: zero, multiple, unstarred, unrelated' && grep -n 'UpdateAllMomentum\|zero-star persisted\|multiple-stars persisted\|PASS: canonical\|PASS: zero\|PASS: one linked unstarred\|PASS: two linked starred\|PASS: three linked artifacts\|PASS: an unrelated' tests/integration/topic_lifecycle_momentum_test.go && printf '%s\n' 'fixture identities:' && grep -n 'artifact-zero\|artifact-star-one\|artifact-star-two\|artifact-unstarred\|artifact-unrelated-star' tests/integration/topic_lifecycle_momentum_test.go`
  **Exit Code:** 0
  **Claim Source:** interpreted
  **Interpretation:** The persistent regression invokes the production lifecycle entrypoint and encodes each required fixture/assertion; this does not claim a current-session PostgreSQL test run.
  **Evidence:** [report.md#persisted-case-contract-inspection](report.md#persisted-case-contract-inspection)

  ```text
  actual entrypoint: topics.NewLifecycle(pool).UpdateAllMomentum(ctx)
  required cases: zero, multiple, unstarred, unrelated
  35: if err := lifecycle.UpdateAllMomentum(ctx); err != nil {
  41: t.Logf("zero-star persisted momentum=%.4f state=%s", zeroStar.momentum, zeroStar.state)
  45: t.Logf("multiple-stars persisted momentum=%.4f state=%s", multipleStars.momentum, multipleStars.state)
  48: PASS: zero linked starred artifacts contribute 0.0 star momentum
  49: PASS: one linked unstarred artifact contributes only 0.5 connection momentum
  50: PASS: two linked starred artifacts contribute exactly 10.0 star momentum
  52: PASS: an unrelated starred artifact contributes nothing to the tested topic
  106: prefix+"-artifact-zero"
  107: prefix+"-artifact-star-one"
  108: prefix+"-artifact-star-two"
  109: prefix+"-artifact-unstarred"
  110: prefix+"-artifact-unrelated-star"
  ```

  **Result:** PASS (test contract fidelity only)
- [x] Query failures remain returned and scheduler failure logging remains honest

  **Phase:** implement
  **Executed:** YES (current session)
  **Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run '^(TestCalculateMomentum.*|TestTransitionState.*|TestDefaultMomentumConfig|TestNewLifecycle|TestTopicMomentumJob_LogsLifecycleQueryFailure)$' --verbose`
  **Exit Code:** 0
  **Claim Source:** executed
  **Evidence:** [report.md#focused-lifecycle-and-scheduler-unit-check](report.md#focused-lifecycle-and-scheduler-unit-check)

  ```text
  === RUN   TestTopicMomentumJob_LogsLifecycleQueryFailure
  --- PASS: TestTopicMomentumJob_LogsLifecycleQueryFailure (0.00s)
  PASS
  ok github.com/smackerel/smackerel/internal/scheduler 0.050s
  === RUN   TestCalculateMomentum
  --- PASS: TestCalculateMomentum (0.00s)
  === RUN   TestTransitionState
  --- PASS: TestTransitionState (0.00s)
  === RUN   TestDefaultMomentumConfig
  --- PASS: TestDefaultMomentumConfig (0.00s)
  === RUN   TestCalculateMomentum_StarsAndConnections
  --- PASS: TestCalculateMomentum_StarsAndConnections (0.00s)
  === RUN   TestNewLifecycle
  --- PASS: TestNewLifecycle (0.00s)
  PASS
  ok github.com/smackerel/smackerel/internal/topics 0.014s
  [go-unit] go test ./... finished OK
  ```

  **Result:** PASS
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Focused unit, functional, integration, and scheduler regressions pass
- [ ] Check, lint, and format gates pass with zero warnings
- [ ] Artifact lint, traceability, implementation-reality, and adversarial regression guards pass
- [x] Change Boundary is respected and zero excluded file families were changed

  **Phase:** implement
  **Executed:** YES (current session)
  **Command:** `BASE=$(git merge-base HEAD origin/main) && printf 'IMPLEMENT BOUNDARY CHECK\nBASE=%s\nHEAD=%s\n' "$BASE" "$(git rev-parse HEAD)" && git diff --name-status "$BASE"..HEAD && printf '%s\n' 'diff check:' && git diff --check "$BASE"..HEAD && printf '%s\n' 'excluded families changed: none shown above'`
  **Exit Code:** 0
  **Claim Source:** executed
  **Evidence:** [report.md#implement-change-boundary-proof](report.md#implement-change-boundary-proof)

  ```text
  BASE=f5f05450848630fe84c0a215429bdfc701c4bcd2
  HEAD=49be4c9735955e5a3ccefa612e54c7d265160aee
  M internal/scheduler/jobs_test.go
  M internal/topics/lifecycle.go
  A specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/bug.md
  A specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/design.md
  A specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/report.md
  A specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scenario-manifest.json
  A specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scopes.md
  A specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/spec.md
  A specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/state.json
  A specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/uservalidation.md
  A tests/integration/topic_lifecycle_momentum_test.go
  diff check: no diagnostics
  excluded families changed: none
  ```

  **Result:** PASS
- [ ] Documentation and bug packet match the implemented behavior

### Post-Scope Certification Gate (Not Scope DoD)

Route the packet now through the authorized `bugfix-fastlane` implementation/test chain so each remaining Scope 1 DoD item receives owner-recorded execution evidence. Only after every Scope 1 DoD item is complete and Scope 1 is marked Done may the packet proceed to `bubbles.validate` and `bubbles.audit` for independent certification. Validation and audit evidence remain owner-recorded phase-exit evidence; they do not gate Scope 1 completion and must not be self-certified by planning or implementation.
