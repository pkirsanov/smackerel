# Execution Report: BUG-003-002 Topic momentum star aggregation

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore canonical topic star aggregation - 2026-07-20 20:17 UTC

### Summary

The worktree reproduced the production query/schema counterexample in Scope 6 Topic Lifecycle, replaced the invalid star source with canonical relationship aggregation, and verified the focused integration, R-208 unit/functional, scheduler-observability, and targeted full-stack paths. Delivery status remains `in_progress`; no terminal certification is claimed.

Changed implementation and test paths:

- `internal/topics/lifecycle.go`
- `tests/integration/topic_lifecycle_momentum_test.go`
- `internal/scheduler/jobs_test.go`
- `tests/e2e/test_topic_lifecycle.sh` (executed unchanged)

### Decision Record

- Canonical explicit-star source: `artifacts.user_starred`.
- Canonical topic membership: `artifact -> topic` `BELONGS_TO` edges.
- Repair boundary: replace the invalid lifecycle star expression without adding a migration or changing scheduler/health semantics.
- Required RED: actual `topics.Lifecycle.UpdateAllMomentum` against production migrations in the disposable `integration-light` PostgreSQL stack.

### Completion Statement

Bug delivery remains `in_progress`. Certification fields remain owned by `bubbles.validate`, and audit evidence remains owned by `bubbles.audit`.

### Code Diff Evidence

**Phase:** bug
**Command:** `git status --short && git diff --name-only && git diff --stat && git diff --check`
**Exit Code:** 0
**Claim Source:** executed

```text
 M internal/scheduler/jobs_test.go
 M internal/topics/lifecycle.go
?? specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/
?? tests/integration/topic_lifecycle_momentum_test.go
internal/scheduler/jobs_test.go
internal/topics/lifecycle.go
internal/scheduler/jobs_test.go | 34 ++++++++++++++++++++++++++++++++++
internal/topics/lifecycle.go    |  6 +++++-
2 files changed, 39 insertions(+), 1 deletion(-)
git diff --check produced no diagnostics and exited 0
```

Changed-path classification: one production query, one scheduler unit test, one PostgreSQL integration test, and one canonical bug packet. No config, schema, deployment, release-train, secret, manifest, or generated path is present.

### Bug Reproduction - Live Red-Team Observation

**Phase:** bug
**Command:** Not executed in this worktree; observation supplied from the live red-team run.
**Exit Code:** not-run
**Claim Source:** not-run

```text
Observed source: f5f05450848630fe84c0a215429bdfc701c4bcd2
Observed at: 2026-07-20T20:00:00Z
Observed service: smackerel-core
Observed job: topic momentum update
Observed database: PostgreSQL
Observed SQLSTATE: 42703
Observed error class: undefined column
Observed column: t.star_count
Observed scheduler result: failure log emitted
Observed public health result: remained green
Raw counterexample:
topic momentum update failed: query topics: ERROR: column t.star_count does not exist (SQLSTATE 42703)
```

The current-session RED below independently reproduced the same counterexample against production migrations.

### Test Evidence

#### RED - actual lifecycle query against canonical migrations

**Phase:** bug
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration-light --go-run '^TestTopicLifecycleMomentumFromPersistedStars$'`
**Exit Code:** 1 (expected RED)
**Claim Source:** executed

```text
[go-integration] gettext-base install OK
go-integration: applying -run selector: ^TestTopicLifecycleMomentumFromPersistedStars$
=== RUN   TestTopicLifecycleMomentumFromPersistedStars
	topic_lifecycle_momentum_test.go:36: UpdateAllMomentum against canonical migrations: query topics: ERROR: column t.star_count does not exist (SQLSTATE 42703)
--- FAIL: TestTopicLifecycleMomentumFromPersistedStars (0.06s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/integration        0.195s
FAIL: go-integration-light (exit=1)
Running project-scoped integration-light stack teardown (exit cleanup, timeout 120s)...
Container smackerel-test-nats-1 Removed
Container smackerel-test-postgres-1 Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-postgres-data Removed
Network smackerel-test_default Removed
```

#### Broader Go unit regression

**Phase:** bug
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go`
**Exit Code:** 0
**Claim Source:** executed

```text
ok      github.com/smackerel/smackerel/internal/recommendation/watch    (cached)
ok      github.com/smackerel/smackerel/internal/retrieval/evergreen     0.030s
ok      github.com/smackerel/smackerel/internal/retrieval/routing       0.020s
ok      github.com/smackerel/smackerel/internal/scheduler       5.071s
ok      github.com/smackerel/smackerel/internal/scopesdriftguard        0.126s
ok      github.com/smackerel/smackerel/internal/stringutil      (cached)
ok      github.com/smackerel/smackerel/internal/telegram        27.636s
ok      github.com/smackerel/smackerel/internal/topics  (cached)
ok      github.com/smackerel/smackerel/internal/web     0.233s
ok      github.com/smackerel/smackerel/tests/e2e/agent  0.013s
ok      github.com/smackerel/smackerel/tests/eval/assistant     0.046s
ok      github.com/smackerel/smackerel/tests/observability      0.007s
ok      github.com/smackerel/smackerel/tests/stress/readiness   0.036s
ok      github.com/smackerel/smackerel/web/pwa/tests    1.245s
[go-unit] go test ./... finished OK
```

#### GREEN - canonical persisted star aggregation

**Phase:** bug
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration-light --go-run '^TestTopicLifecycleMomentumFromPersistedStars$'`
**Exit Code:** 0
**Claim Source:** executed

```text
go-integration: applying -run selector: ^TestTopicLifecycleMomentumFromPersistedStars$
=== RUN   TestTopicLifecycleMomentumFromPersistedStars
INFO topic state transition topic=test-TestTopicLifecycleMomentumFromPersistedStars-1784579307715676916-zero from=emerging to=dormant momentum=0.5
INFO topic state transition topic=test-TestTopicLifecycleMomentumFromPersistedStars-1784579307715676916-multiple from=emerging to=active momentum=11.5
topic_lifecycle_momentum_test.go:41: zero-star persisted momentum=0.5000 state=dormant
topic_lifecycle_momentum_test.go:45: multiple-stars persisted momentum=11.5000 state=active
topic_lifecycle_momentum_test.go:47: PASS: canonical topics schema has no star_count column
topic_lifecycle_momentum_test.go:48: PASS: zero linked starred artifacts contribute 0.0 star momentum
topic_lifecycle_momentum_test.go:49: PASS: one linked unstarred artifact contributes only 0.5 connection momentum
topic_lifecycle_momentum_test.go:50: PASS: two linked starred artifacts contribute exactly 10.0 star momentum
topic_lifecycle_momentum_test.go:51: PASS: three linked artifacts contribute exactly 1.5 connection momentum
topic_lifecycle_momentum_test.go:52: PASS: an unrelated starred artifact contributes nothing to the tested topic
--- PASS: TestTopicLifecycleMomentumFromPersistedStars (0.06s)
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.167s
PASS: go-integration-light
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-nats-data Removed
Network smackerel-test_default Removed
```

#### Focused R-208 and scheduler regressions

**Phase:** bug
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run '^(TestCalculateMomentum.*|TestTransitionState.*|TestDefaultMomentumConfig|TestNewLifecycle|TestTopicMomentumJob_LogsLifecycleQueryFailure)$' --verbose`
**Exit Code:** 0
**Claim Source:** executed

```text
=== RUN   TestTopicMomentumJob_LogsLifecycleQueryFailure
--- PASS: TestTopicMomentumJob_LogsLifecycleQueryFailure (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/scheduler       0.050s
=== RUN   TestCalculateMomentum
--- PASS: TestCalculateMomentum (0.00s)
=== RUN   TestCalculateMomentum_Dormant
--- PASS: TestCalculateMomentum_Dormant (0.00s)
=== RUN   TestCalculateMomentum_Decay
--- PASS: TestCalculateMomentum_Decay (0.00s)
=== RUN   TestTransitionState
--- PASS: TestTransitionState (0.00s)
=== RUN   TestCalculateMomentum_StarsAndConnections
--- PASS: TestCalculateMomentum_StarsAndConnections (0.00s)
=== RUN   TestNewLifecycle
--- PASS: TestNewLifecycle (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/topics  0.006s
[go-unit] go test ./... finished OK
```

#### Targeted topic lifecycle E2E

**Phase:** bug
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --shell-run test_topic_lifecycle.sh`
**Exit Code:** 0
**Claim Source:** executed

```text
Container smackerel-test-postgres-1 Healthy
Container smackerel-test-smackerel-core-1 Healthy
Container smackerel-test-smackerel-ml-1 Healthy
=== Topic Lifecycle E2E Tests ===
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
Seeding topics...
Existing pricing topic owner: topic-lifecycle-existing-pricing
PASS: Adversarial pricing topic present without duplicate collision
Hot topic momentum: 20
Dormant topic momentum: 0.1
PASS: Topic lifecycle: states and momentum verified
=== Topic Lifecycle E2E tests passed ===
PASS: test_topic_lifecycle.sh
Total: 1
Passed: 1
Failed: 0
Volume smackerel-test-ollama-data Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-postgres-data Removed
Network smackerel-test_default Removed
```

### Uncertainty Declarations

Validate-owned certification and audit-owned final review were not executed in this invocation. The packet remains `in_progress` and all delivery DoD items remain unchecked rather than attributing evidence to unavailable specialists.

### Scenario Contract Evidence

Scenario contracts are registered in [scenario-manifest.json](scenario-manifest.json) and linked to these executed regressions:

- `tests/integration/topic_lifecycle_momentum_test.go`
- `internal/scheduler/jobs_test.go`
- `tests/e2e/test_topic_lifecycle.sh`

### Coverage Report

Focused behavior coverage includes canonical schema compatibility, zero linked stars, two linked stars, linked unstarred artifacts, unrelated starred artifacts, connection contribution, lifecycle state transitions, and scheduler failure/success log separation. No numeric repository-wide coverage percentage is claimed.

### Lint/Quality

Executed repository surfaces:

- `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check` - exit 0
- `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh lint` - exit 0, `All checks passed!`, `Web validation passed`
- `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh format --check` - exit 0, `75 files already formatted`
- Standard and `--bugfix` regression-quality guards - exit 0, zero violations and warnings

#### Traceability and implementation reality

**Phase:** bug
**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count` and `bash .github/bubbles/scripts/implementation-reality-scan.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count --verbose`
**Exit Code:** 0, 0
**Claim Source:** executed

```text
scenario-manifest.json covers 3 scenario contract(s)
scenario-manifest.json linked test exists: tests/integration/topic_lifecycle_momentum_test.go
scenario-manifest.json linked test exists: internal/scheduler/jobs_test.go
scenario-manifest.json linked test exists: tests/e2e/test_topic_lifecycle.sh
All linked tests from scenario-manifest.json exist
Scenarios checked: 3
Scenario-to-row mappings: 3
Report evidence references: 3
DoD fidelity scenarios: 3 (mapped: 3, unmapped: 0)
RESULT: PASSED (0 warnings)
INFO: Resolved 1 implementation file(s) to scan
Files scanned: 1
Violations: 0
Warnings: 0
PASSED: No source code reality violations detected
```

#### Adversarial regression guards

**Phase:** bug
**Command:** `bash .github/bubbles/scripts/regression-quality-guard.sh tests/integration/topic_lifecycle_momentum_test.go internal/scheduler/jobs_test.go tests/e2e/test_topic_lifecycle.sh` and the same command with `--bugfix`
**Exit Code:** 0, 0
**Claim Source:** executed

```text
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 3
Bugfix mode: true
Scanning tests/integration/topic_lifecycle_momentum_test.go
Adversarial signal detected in tests/integration/topic_lifecycle_momentum_test.go
Scanning internal/scheduler/jobs_test.go
Adversarial signal detected in internal/scheduler/jobs_test.go
Scanning tests/e2e/test_topic_lifecycle.sh
Adversarial signal detected in tests/e2e/test_topic_lifecycle.sh
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 3
Files with adversarial signals: 3
```

### Spot-Check Recommendations

- Verify the final query retains all five `BELONGS_TO` relationship predicates from BUG-R2.
- Verify the integration assertion includes connection contribution separately from explicit-star contribution.
- Verify no migration or health-contract path appears in the final diff.

### Validation Summary

No validate-owned certification result is recorded. Product tests and project-owned gates are recorded above; certification stays `in_progress`.

### Audit Verdict

The controlling audit-owned result is recorded in the bounded final delivery audit below.

## Bounded Final Delivery Audit - 2026-07-20

### Findings

1. **[BLOCKER] AUD-003-002-COMPLETION-001** - The assertion-only transition guard refused target `done`: all 16 DoD items remain unchecked, Scope 1 remains `In Progress`, and the required broader E2E DoD item has no executed evidence. The current `in_progress` state is truthful and must remain unchanged.
2. **[BLOCKER] AUD-003-002-PROVENANCE-001** - The registry-required `implement`, `test`, `regression`, `simplify`, `stabilize`, `security`, `validate`, and `audit` phases are absent from the execution/certification phase records. Existing `bubbles.bug` evidence is genuine but cannot impersonate those specialist claims.
3. **[BLOCKER] AUD-003-002-BOUNDARY-001** - Gate Check 8D accepts the presence of a Change Boundary section but rejects its inline `Allowed`/`Excluded` prose because the scope does not enumerate the allowed and excluded surfaces in the mechanically required form.
4. **[BLOCKER] AUD-003-002-G090-001** - Gate G090 cannot evaluate convergence because `.specify/memory/bubbles.session.json` is absent from the isolated exact-commit worktree.

### Verified Delivery Delta

- PASS: remote branch `origin/bug/topic-momentum-star-count-20260720`, isolated worktree HEAD, and requested commit all resolve to `7ff2d5441f8d90158873cff378c8b81d448900b8`.
- PASS: the branch is one commit ahead of parent and `origin/main` merge-base `f5f05450848630fe84c0a215429bdfc701c4bcd2`.
- PASS: production no longer references `t.star_count`; the correlated aggregate counts `DISTINCT a.id` through artifact-to-topic `BELONGS_TO` edges with `a.user_starred IS TRUE`.
- PASS: no migration, schema, deploy, config, release-train, secret, manifest, or framework path changed.
- PASS: the independent disposable PostgreSQL run applied canonical migrations and proved zero-star, multiple-star, unstarred, unrelated-star, momentum, and state outcomes.
- PASS: the independent focused unit run proved lifecycle formula/state behavior and scheduler failure logging without a success log.
- PASS: the recorded targeted lifecycle E2E is a real-stack shell flow; selected tests contain no skip, interception, or internal-fake markers.
- PASS: artifact lint, traceability, implementation reality, and standard plus bugfix regression-quality guards passed independently.
- PASS: post-test inspection found no residual `smackerel-test` containers, volumes, or networks.
- PASS: the exact changed-path boundary is limited to one production query, two focused test surfaces, and this packet.

### Independent Audit Evidence

**Phase:** audit
**Command:** `bash .github/bubbles/scripts/state-transition-guard.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count --target-status done --expect-workflow-mode bugfix-fastlane --expect-contract-digest sha256:aa91472c047d3d985d38c1d308feb1e6081955b2aa553816deb5987d9cdc449f`
**Exit Code:** 1
**Claim Source:** executed

```text
DoD items total: 16 (checked: 0, unchecked: 16)
BLOCK: Resolved scope artifacts have 16 UNCHECKED DoD items
Resolved scopes: total=1, Done=0, In Progress=1, Not Started=0, Blocked=0
BLOCK: Resolved scope artifacts have 1 scope(s) still marked 'In Progress'
BLOCK: Required phase 'implement' NOT in execution/certification phase records
BLOCK: Required phase 'test' NOT in execution/certification phase records
BLOCK: Required phase 'regression' NOT in execution/certification phase records
BLOCK: Required phase 'simplify' NOT in execution/certification phase records
BLOCK: Required phase 'stabilize' NOT in execution/certification phase records
BLOCK: Required phase 'security' NOT in execution/certification phase records
BLOCK: Required phase 'validate' NOT in execution/certification phase records
BLOCK: Required phase 'audit' NOT in execution/certification phase records
BLOCK: Scope is a refactor/repair but does not enumerate allowed and excluded surfaces
BLOCK: Retro convergence health failed - Gate G090
TRANSITION BLOCKED: 14 failure(s), 3 warning(s)
```

**Phase:** audit
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration-light --go-run '^TestTopicLifecycleMomentumFromPersistedStars$'`
**Exit Code:** 0
**Claim Source:** executed

```text
INFO applied migration version=001_initial_schema.sql
INFO applied migration version=062_model_usage_ledger.sql
INFO dbmigrate: all migrations applied
PASS: integration-light db migration (schema applied via cmd/dbmigrate)
=== RUN   TestTopicLifecycleMomentumFromPersistedStars
INFO topic state transition topic=<fixture>-zero from=emerging to=dormant momentum=0.5
INFO topic state transition topic=<fixture>-multiple from=emerging to=active momentum=11.5
zero-star persisted momentum=0.5000 state=dormant
multiple-stars persisted momentum=11.5000 state=active
PASS: canonical topics schema has no star_count column
PASS: zero linked starred artifacts contribute 0.0 star momentum
PASS: one linked unstarred artifact contributes only 0.5 connection momentum
PASS: two linked starred artifacts contribute exactly 10.0 star momentum
PASS: three linked artifacts contribute exactly 1.5 connection momentum
PASS: an unrelated starred artifact contributes nothing to the tested topic
--- PASS: TestTopicLifecycleMomentumFromPersistedStars (0.06s)
PASS: go-integration-light
Container smackerel-test-postgres-1 Removed
Volume smackerel-test-postgres-data Removed
Network smackerel-test_default Removed
```

### Observation Versus Blocker

- **Observation:** `artifact-lint.sh` reports the existing `scopeProgress` and `scopeLayout` fields as deprecated while still exiting 0. This is non-blocking for the current `in_progress` packet.
- **Observation:** focused `go test ./... -run ...` selectors emit `testing: warning: no tests to run` for unrelated packages; the selected scheduler, lifecycle, and integration tests all ran and passed. This does not substitute for the missing broader E2E evidence.
- **Blocker:** the clean code/test sub-audit cannot override the failed delivery-completion guard or promote packet state.

### Spot-Check Recommendations

1. **Code Diff Evidence block (lines 36-47)** - The raw block is exactly 10 lines, the minimum threshold. Verify its path inventory against `git show --name-status 7ff2d5441f8d90158873cff378c8b81d448900b8`.
2. **First packet-specific PostgreSQL integration regression** - Verify that the `0.5` and `11.5` expectations continue to separate connection contribution from explicit-star contribution.
3. **Not-run live red-team observation** - Treat the `not-run` block only as supplied context; the separate executed RED transcript and the parent-commit `t.star_count` search are the controlling counterexample proof.

### Audit Verdict

`REWORK_REQUIRED`. The red-team fix and its bounded behavior evidence pass, but final delivery certification is blocked by incomplete DoD/phase provenance, the mechanically incomplete Change Boundary, and missing G090 session state. The next repair owner is `bubbles.plan`, which must enumerate the allowed and excluded Change Boundary surfaces in the mechanically accepted scope form without changing the already-clean production fix; the authorized `bugfix-fastlane` chain can then resume at `implement` and record each later phase's own evidence before validate and re-audit.

BEGIN TRANSITION_GUARD_RESULT_V1
schemaVersion: transition-guard-result/v1
workflowMode: bugfix-fastlane
auditProfile: delivery-completion-v1
targetStatus: done
contractDigest: sha256:aa91472c047d3d985d38c1d308feb1e6081955b2aa553816deb5987d9cdc449f
targetRevision: sha256:3cdb2dd585aa85d66c045f909c7d8e899318128b88c29cb8451145e5d31820e8
applicableCheckClasses: [universal,mode-required,delivery-completion]
notApplicableChecks: []
passedGateIds: [G053,G040,G051,G068,G082,G083,G084,G128,G085,G086,G091,G087,G093,G088,G089,G092,G094,G095,G097,G098,G099,G100]
failedGateIds: [G022,G090]
failedChecks: [Check-4-completion,Check-5-all-done]
blockingCode: DELIVERY_COMPLETION_FAILED
failureCount: 14
exitStatus: 1
verdict: FAIL
END TRANSITION_GUARD_RESULT_V1

verdict: REWORK_REQUIRED
target: specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count
mode: bugfix-fastlane
audit class: delivery-completion
ceiling: done

BEGIN AUDIT_RESULT_V1
schemaVersion: audit-result/v1
runId: audit-003-002-20260720T205105Z
attemptId: audit-003-002-20260720T205105Z-a1
target: specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count
targetRevision: sha256:3cdb2dd585aa85d66c045f909c7d8e899318128b88c29cb8451145e5d31820e8
workflowMode: bugfix-fastlane
modeClass: none
auditClass: delivery-completion
statusCeiling: done
requestedStatus: done
auditVerdict: REWORK_REQUIRED
outcome: route_required
resultState: ACTIVE
certifiedStatus: none
planningEvaluation: NOT_EVALUATED
deliveryEvaluation: REFUSED
sourceEditLockout: PASS
applicableCheckClasses: [universal,mode-required,delivery-completion]
notApplicableChecks: []
passedGateIds: [G053,G040,G051,G068,G082,G083,G084,G128,G085,G086,G091,G087,G093,G088,G089,G092,G094,G095,G097,G098,G099,G100]
failedGateIds: [G022,G090]
failedChecks: [Check-4-completion,Check-5-all-done]
blockingCode: DELIVERY_COMPLETION_FAILED
unresolvedFields: []
contradictions: []
contractRef: bubbles/workflows/modes.yaml#bugfix-fastlane
contractDigest: sha256:aa91472c047d3d985d38c1d308feb1e6081955b2aa553816deb5987d9cdc449f
evidenceRefs: [.specify/runtime/audit-003-002-20260720T205105Z-a1.txt,report.md#bounded-final-delivery-audit---2026-07-20]
addressedFindings: []
unresolvedFindings: [AUD-003-002-COMPLETION-001,AUD-003-002-PROVENANCE-001,AUD-003-002-BOUNDARY-001,AUD-003-002-G090-001]
nextRequiredOwner: bubbles.plan
supersedesAttemptId: none
resumeFromPhase: none
END AUDIT_RESULT_V1

## Implement Phase Verification - 2026-07-20 21:22 UTC

### Implement Owner Statement

`bubbles.implement` inspected the authorized starting revision
`49be4c9735955e5a3ccefa612e54c7d265160aee` and found no concrete defect in the
scoped product delta. No product source or test file was modified in this phase.
The existing repair is the minimal design-prescribed query replacement: it
derives explicit stars from canonical persisted relationships, preserves the
existing momentum/state code, and preserves query-error propagation.

This phase does not claim the `test`, `regression`, `simplify`, `stabilize`,
`security`, `validate`, or `audit` phases. Scope and certification status remain
`in_progress`.

### Canonical Root Cause Reconciliation

**Phase:** implement
**Executed:** YES (current session)
**Command:** `printf '%s\n' 'IMPLEMENT INSPECTION: root cause and canonical contracts' 'base revision: f5f05450848630fe84c0a215429bdfc701c4bcd2' 'pre-fix lifecycle reference:' && git grep -n 't.star_count' f5f05450848630fe84c0a215429bdfc701c4bcd2 -- internal/topics/lifecycle.go && printf '%s\n' 'canonical star persistence:' && grep -n 'user_starred' internal/db/migrations/001_initial_schema.sql && printf '%s\n' 'canonical edge table:' && grep -n -A 8 'CREATE TABLE IF NOT EXISTS edges' internal/db/migrations/001_initial_schema.sql && printf '%s\n' 'canonical BELONGS_TO producers:' && grep -n 'BELONGS_TO' internal/graph/linker.go internal/connector/bookmarks/topics.go`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
IMPLEMENT INSPECTION: root cause and canonical contracts
base revision: f5f05450848630fe84c0a215429bdfc701c4bcd2
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
129-    weight      REAL DEFAULT 1.0,
130-    metadata    JSONB,
canonical BELONGS_TO producers:
internal/graph/linker.go:216:// linkByTopics assigns artifacts to topics and creates BELONGS_TO edges.
internal/graph/linker.go:266: if err := l.createEdge(ctx, "artifact", artifactID, "topic", topicID, "BELONGS_TO", 1.0); err != nil {
internal/connector/bookmarks/topics.go:152:// CreateTopicEdge creates a BELONGS_TO edge from an artifact to a topic.
internal/connector/bookmarks/topics.go:161: VALUES ($1, 'artifact', $2, 'topic', $3, 'BELONGS_TO', 1.0, '{}')
```

**Result:** PASS - the pre-fix query read an absent topic column while canonical
source persists stars on artifacts and membership as artifact-to-topic
`BELONGS_TO` edges.

### Production Query Reconciliation

**Phase:** implement
**Executed:** YES (current session)
**Command:** `printf '%s\n' 'IMPLEMENT INSPECTION: production aggregation query' 'required source: artifacts.user_starred' 'required relation: artifact -> topic BELONGS_TO' 'required cardinality: distinct artifact ids' && grep -n -A 17 -B 2 'COUNT(DISTINCT a.id)' internal/topics/lifecycle.go`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
IMPLEMENT INSPECTION: production aggregation query
required source: artifacts.user_starred
required relation: artifact -> topic BELONGS_TO
required cardinality: distinct artifact ids
121-    rows, err := l.Pool.Query(ctx, `
122-            SELECT t.id, t.name, t.state, t.capture_count_30d, t.capture_count_90d, t.search_hit_count_30d,
123:                   COALESCE((SELECT COUNT(DISTINCT a.id)
124-                             FROM edges e
125-                             JOIN artifacts a ON a.id = e.src_id AND e.src_type = 'artifact'
126-                             WHERE e.dst_type = 'topic' AND e.dst_id = t.id
127-                               AND e.edge_type = 'BELONGS_TO' AND a.user_starred IS TRUE), 0)::int,
128-                   COALESCE((SELECT COUNT(*) FROM edges WHERE src_type = 'topic' AND src_id = t.id
129-                              OR dst_type = 'topic' AND dst_id = t.id), 0)::int,
130-                   EXTRACT(DAY FROM NOW() - COALESCE(t.last_active, t.created_at))::int
131-            FROM topics t
132-    `)
133-    if err != nil {
134-            return fmt.Errorf("query topics: %w", err)
135-    }
136-    defer rows.Close()
137-
138-    for rows.Next() {
139-            var id, name, state string
140-            var cap30, cap90, search30, starCount, connCount, daysSince int
```

**Result:** PASS - all BUG-R2 predicates are present, duplicate relationships
cannot inflate the star signal because artifact IDs are distinct, no
denormalized column is introduced, and SQL query errors retain `query topics`
context.

### Persisted Case Contract Inspection

**Phase:** implement
**Executed:** YES (current session)
**Command:** `printf '%s\n' 'IMPLEMENT INSPECTION: persisted-star test contract' 'actual entrypoint: topics.NewLifecycle(pool).UpdateAllMomentum(ctx)' 'required cases: zero, multiple, unstarred, unrelated' && grep -n 'UpdateAllMomentum\|zero-star persisted\|multiple-stars persisted\|PASS: canonical\|PASS: zero\|PASS: one linked unstarred\|PASS: two linked starred\|PASS: three linked artifacts\|PASS: an unrelated' tests/integration/topic_lifecycle_momentum_test.go && printf '%s\n' 'fixture identities:' && grep -n 'artifact-zero\|artifact-star-one\|artifact-star-two\|artifact-unstarred\|artifact-unrelated-star' tests/integration/topic_lifecycle_momentum_test.go`
**Exit Code:** 0
**Claim Source:** interpreted
**Interpretation:** The inspected persistent regression invokes the production
`Lifecycle.UpdateAllMomentum` entrypoint and encodes separate zero-star,
multiple-star, unstarred, and unrelated-star fixtures and assertions. This is a
test-code existence/fidelity claim only; this implement phase did not re-run the
PostgreSQL integration test.
**Output:**

```text
IMPLEMENT INSPECTION: persisted-star test contract
actual entrypoint: topics.NewLifecycle(pool).UpdateAllMomentum(ctx)
required cases: zero, multiple, unstarred, unrelated
35:     if err := lifecycle.UpdateAllMomentum(ctx); err != nil {
36:             t.Fatalf("UpdateAllMomentum against canonical migrations: %v", err)
41:     t.Logf("zero-star persisted momentum=%.4f state=%s", zeroStar.momentum, zeroStar.state)
45:     t.Logf("multiple-stars persisted momentum=%.4f state=%s", multipleStars.momentum, multipleStars.state)
47:     t.Log("PASS: canonical topics schema has no star_count column")
48:     t.Log("PASS: zero linked starred artifacts contribute 0.0 star momentum")
49:     t.Log("PASS: one linked unstarred artifact contributes only 0.5 connection momentum")
50:     t.Log("PASS: two linked starred artifacts contribute exactly 10.0 star momentum")
51:     t.Log("PASS: three linked artifacts contribute exactly 1.5 connection momentum")
52:     t.Log("PASS: an unrelated starred artifact contributes nothing to the tested topic")
fixture identities:
106: prefix+"-artifact-zero", prefix+"-hash-zero",
107: prefix+"-artifact-star-one", prefix+"-hash-star-one",
108: prefix+"-artifact-star-two", prefix+"-hash-star-two",
109: prefix+"-artifact-unstarred", prefix+"-hash-unstarred",
110: prefix+"-artifact-unrelated-star", prefix+"-hash-unrelated-star",
126: prefix+"-edge-zero", prefix+"-artifact-zero", prefix+"-topic-zero",
127: prefix+"-edge-star-one", prefix+"-artifact-star-one", prefix+"-topic-multiple",
128: prefix+"-edge-star-two", prefix+"-artifact-star-two", prefix+"-topic-multiple",
129: prefix+"-edge-unstarred", prefix+"-artifact-unstarred", prefix+"-topic-multiple",
130: prefix+"-edge-unrelated-star", prefix+"-artifact-unrelated-star", prefix+"-topic-unrelated",
```

**Result:** PASS - the persistent regression contract directly represents every
required star-association case without a test-only production path.

### Focused Lifecycle And Scheduler Unit Check

**Phase:** implement
**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run '^(TestCalculateMomentum.*|TestTransitionState.*|TestDefaultMomentumConfig|TestNewLifecycle|TestTopicMomentumJob_LogsLifecycleQueryFailure)$' --verbose`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
=== RUN   TestTopicMomentumJob_LogsLifecycleQueryFailure
--- PASS: TestTopicMomentumJob_LogsLifecycleQueryFailure (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/scheduler       0.050s
=== RUN   TestCalculateMomentum
--- PASS: TestCalculateMomentum (0.00s)
=== RUN   TestCalculateMomentum_Dormant
--- PASS: TestCalculateMomentum_Dormant (0.00s)
=== RUN   TestCalculateMomentum_Decay
--- PASS: TestCalculateMomentum_Decay (0.00s)
=== RUN   TestTransitionState
--- PASS: TestTransitionState (0.00s)
=== RUN   TestDefaultMomentumConfig
--- PASS: TestDefaultMomentumConfig (0.00s)
=== RUN   TestCalculateMomentum_StarsAndConnections
--- PASS: TestCalculateMomentum_StarsAndConnections (0.00s)
=== RUN   TestNewLifecycle
--- PASS: TestNewLifecycle (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/topics  0.014s
[go-unit] go test ./... finished OK
```

**Result:** PASS - focused formula/state behavior remains green, and the real
closed-pool lifecycle path still returns enough query context for the scheduler
to log failure without logging success.

### Cheap Compile Check

**Phase:** implement
**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
IMPLEMENT COMPILE CHECK START
worktree: ~/smackerel-bug-topic-momentum-star-count-20260720
hardware tier: cpu
command: ./smackerel.sh check
config-validate: ~/smackerel-bug-topic-momentum-star-count-20260720/config/generated/dev.env.tmp.437564 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
compile check exit: 0
IMPLEMENT COMPILE CHECK END
```

**Result:** PASS - the scoped implementation compiles through the repository
CLI and the check did not mutate tracked generated configuration.

### Implement Change Boundary Proof

**Phase:** implement
**Executed:** YES (current session)
**Command:** `BASE=$(git merge-base HEAD origin/main) && printf 'IMPLEMENT BOUNDARY CHECK\nBASE=%s\nHEAD=%s\n' "$BASE" "$(git rev-parse HEAD)" && git diff --name-status "$BASE"..HEAD && printf '%s\n' 'diff check:' && git diff --check "$BASE"..HEAD && printf '%s\n' 'excluded families changed: none shown above'`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
IMPLEMENT BOUNDARY CHECK
BASE=f5f05450848630fe84c0a215429bdfc701c4bcd2
HEAD=49be4c9735955e5a3ccefa612e54c7d265160aee
M       internal/scheduler/jobs_test.go
M       internal/topics/lifecycle.go
A       specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/bug.md
A       specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/design.md
A       specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/report.md
A       specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scenario-manifest.json
A       specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scopes.md
A       specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/spec.md
A       specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/state.json
A       specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/uservalidation.md
A       tests/integration/topic_lifecycle_momentum_test.go
diff check:
excluded families changed: none shown above
```

**Result:** PASS - the complete branch delta contains only the authorized query,
focused test surfaces, and BUG-003-002 packet. No migration, deployment,
configuration, release-train, secret, manifest, or unrelated packet path appears.

### Implement Phase DoD Decision

The implementation owner can support only these five Scope 1 claims from the
current-session execution and inspection above:

1. root cause confirmation against canonical persistence and relationship source;
2. production query derivation from linked `artifacts.user_starred` rows;
3. persistent zero/multiple/unstarred/unrelated case assertions exist against the real lifecycle entrypoint;
4. query-error propagation and scheduler failure/success log separation remain intact;
5. the mechanically enumerated Change Boundary is respected.

All test-category, broad E2E, quality-chain, and documentation-phase claims
remain unchecked for their owning specialists. This phase neither changes Scope
1 to `Done` nor writes any `certification.*` field.

## Test Phase Execution - 2026-07-20 21:37-22:14 UTC

### Findings

1. **TEST-003-002-DUPLICATE-001 - addressed in the scoped regression test.** A
	 first attempt to persist the same artifact-to-topic `BELONGS_TO` relationship
	 twice failed with SQLSTATE `23505`, proving the canonical schema prevents the
	 duplicate row before lifecycle aggregation. The regression now asserts that
	 exact uniqueness constraint and separately proves two distinct linked starred
	 artifacts contribute exactly two star weights.
2. **TEST-003-002-BROAD-E2E-SKIPS-001 - independent, unresolved.** The canonical
	 complete E2E command returned success and all 12 executed Go E2E packages
	 passed, but 20 top-level Go tests emitted `SKIP`, and the harness omitted the
	 opt-in Ollama agent suite. This is unrelated to the topic-momentum delta and
	 was not fixed inline. Under the zero-skip test policy, the broader-E2E DoD and
	 test-phase completion claim remain unchecked.

### Test Owner Statement

`bubbles.test` began from branch `bug/topic-momentum-star-count-20260720` at the
authorized exact starting HEAD `4049cd7ea978dab1dd32e7651182b96b2e4ff658` with
no active `smackerel-test` container, network, volume, or matching process. Only
the packet's existing topic-momentum behavior was exercised. No production
source, migration, deployment, configuration, release-train, secret, or other
packet path was modified.

The test owner strengthened only
`tests/integration/topic_lifecycle_momentum_test.go` so duplicate safety is
proved against the canonical relationship uniqueness constraint rather than by
an impossible duplicate row. Scope and certification remain `in_progress`.

### Duplicate-Safety Probe

**Phase:** test
**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration-light --go-run '^TestTopicLifecycleMomentumFromPersistedStars$'`
**Exit Code:** 1
**Claim Source:** executed
**Output:**

```text
PASS: integration-light db migration (schema applied via cmd/dbmigrate)
go-integration: applying -run selector: ^TestTopicLifecycleMomentumFromPersistedStars$
=== RUN   TestTopicLifecycleMomentumFromPersistedStars
topic_lifecycle_momentum_test.go:32: insert edge momentum fixtures: ERROR: duplicate key value violates unique constraint "edges_src_type_src_id_dst_type_dst_id_edge_type_key" (SQLSTATE 23505)
--- FAIL: TestTopicLifecycleMomentumFromPersistedStars (0.05s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/integration        0.170s
FAIL: go-integration-light (exit=1)
Running project-scoped integration-light stack teardown (exit cleanup, timeout 120s)...
Container smackerel-test-postgres-1 Removed
Container smackerel-test-nats-1 Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-postgres-data Removed
Network smackerel-test_default Removed
```

**Result:** FAIL - the proposed duplicate fixture contradicted the canonical
relationship uniqueness contract. The test, not production source, was repaired.

### Canonical PostgreSQL Integration

**Phase:** test
**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration-light --go-run '^TestTopicLifecycleMomentumFromPersistedStars$'`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
INFO applied migration version=001_initial_schema.sql
INFO applied migration version=062_model_usage_ledger.sql
INFO dbmigrate: all migrations applied
PASS: integration-light db migration (schema applied via cmd/dbmigrate)
go-integration: applying -run selector: ^TestTopicLifecycleMomentumFromPersistedStars$
=== RUN   TestTopicLifecycleMomentumFromPersistedStars
topic_lifecycle_momentum_test.go:35: PASS: canonical relationship uniqueness rejects duplicate BELONGS_TO edges
INFO topic state transition topic=<fixture>-zero from=emerging to=dormant momentum=0.5
INFO topic state transition topic=<fixture>-multiple from=emerging to=active momentum=11.5
topic_lifecycle_momentum_test.go:44: zero-star persisted momentum=0.5000 state=dormant
topic_lifecycle_momentum_test.go:48: multiple-stars persisted momentum=11.5000 state=active
topic_lifecycle_momentum_test.go:50: PASS: canonical topics schema has no star_count column
topic_lifecycle_momentum_test.go:51: PASS: zero linked starred artifacts contribute 0.0 star momentum
topic_lifecycle_momentum_test.go:52: PASS: one linked unstarred artifact contributes only 0.5 connection momentum
topic_lifecycle_momentum_test.go:53: PASS: two distinct linked starred artifacts contribute exactly 10.0 star momentum
topic_lifecycle_momentum_test.go:54: PASS: three linked relationships contribute exactly 1.5 connection momentum
topic_lifecycle_momentum_test.go:55: PASS: an unrelated starred artifact contributes nothing to the tested topic
--- PASS: TestTopicLifecycleMomentumFromPersistedStars (0.07s)
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.184s
PASS: go-integration-light
Container smackerel-test-postgres-1 Removed
Container smackerel-test-nats-1 Removed
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-nats-data Removed
Network smackerel-test_default Removed
```

**Result:** PASS - one integration test passed with zero skips against disposable
PostgreSQL and the full canonical migration chain.

### Focused Lifecycle Formula And State Tests

**Phase:** test
**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run '^Test(CalculateMomentum|TransitionState)' --verbose`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
=== RUN   TestCalculateMomentum
--- PASS: TestCalculateMomentum (0.00s)
=== RUN   TestCalculateMomentum_Dormant
--- PASS: TestCalculateMomentum_Dormant (0.00s)
=== RUN   TestCalculateMomentum_Decay
--- PASS: TestCalculateMomentum_Decay (0.00s)
=== RUN   TestTransitionState
=== RUN   TestTransitionState/emerging_to_hot
=== RUN   TestTransitionState/emerging_to_active
=== RUN   TestTransitionState/hot_to_cooling
=== RUN   TestTransitionState/cooling_to_dormant
--- PASS: TestTransitionState (0.00s)
=== RUN   TestTransitionState_AllBoundaries
--- PASS: TestTransitionState_AllBoundaries (0.00s)
=== RUN   TestCalculateMomentum_StarsAndConnections
--- PASS: TestCalculateMomentum_StarsAndConnections (0.00s)
=== RUN   TestTransitionState_ArchivedResurfaces
--- PASS: TestTransitionState_ArchivedResurfaces (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/topics  0.009s
```

**Result:** PASS - 16 top-level tests and 10 subtests ran, with zero failures and
zero skips.

### Scheduler Failure-Log Regression

**Phase:** test
**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run '^TestTopicMomentumJob_LogsLifecycleQueryFailure$' --verbose`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
[go-unit] envsubst missing - installing gettext-base
[go-unit] gettext-base install OK
[go-unit] starting go test ./...
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/internal/retrieval/routing       0.014s [no tests to run]
=== RUN   TestTopicMomentumJob_LogsLifecycleQueryFailure
--- PASS: TestTopicMomentumJob_LogsLifecycleQueryFailure (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/scheduler       0.038s
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/internal/topics  0.013s [no tests to run]
[go-unit] go test ./... finished OK
```

**Result:** PASS - the one selected scheduler test ran and passed with zero
selected-test skips. Unrelated packages were selected out by the exact `-run`
filter.

### Targeted Topic Lifecycle E2E

**Phase:** test
**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --shell-run test_topic_lifecycle.sh`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
=== Topic Lifecycle E2E Tests ===
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
Seeding topics...
Existing pricing topic owner: topic-lifecycle-existing-pricing
PASS: Adversarial pricing topic present without duplicate collision
Hot topic momentum: 20
Dormant topic momentum: 0.1
PASS: Topic lifecycle: states and momentum verified
=== Topic Lifecycle E2E tests passed ===
PASS: test_topic_lifecycle.sh
Total:  1
Passed: 1
Failed: 0
Container smackerel-test-smackerel-core-1 Removed
Container smackerel-test-smackerel-ml-1 Removed
Container smackerel-test-postgres-1 Removed
Container smackerel-test-nats-1 Removed
Volume smackerel-test-postgres-data Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-ollama-data Removed
Network smackerel-test_default Removed
```

**Result:** PASS - one targeted full-stack shell E2E passed with zero skips and
the harness fully removed its disposable stack.

### Canonical Complete E2E

**Phase:** test
**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
--- SKIP: TestOpenKnowledgeE2E_A01_WebAnswerWithCitations (0.00s)
--- SKIP: TestOpenKnowledgeE2E_A02_UnitConvert (0.00s)
--- SKIP: TestOpenKnowledgeE2E_A03_HybridInternalPlusWeb (0.00s)
--- SKIP: TestOpenKnowledgeE2E_A04_PerTurnBudgetExhausted (0.00s)
--- SKIP: TestOpenKnowledgeE2E_A05_WebSearchDisabled (0.00s)
--- SKIP: TestOpenKnowledgeE2E_A06_PerUserMonthlyBudgetExceeded (0.00s)
--- SKIP: TestOpenKnowledgeE2E_A06_FabricatedSourceRejected (0.00s)
--- SKIP: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.08s)
--- SKIP: TestAssistantE2E_CaptureAcknowledgementIsCrossTransportIdentical_TP_074_17 (0.00s)
--- SKIP: TestAssistantHTTPE2E_CaptureFallbackDedupWithinWindow_TP_074_11 (0.00s)
--- SKIP: TestAssistantHTTPE2E_CaptureFallbackIsInviolable_TP_074_04 (0.00s)
--- SKIP: TestAssistantHTTPE2E_CaptureProvenanceIsDistinct_TP_074_07 (0.00s)
--- SKIP: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.15s)
--- SKIP: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.06s)
--- SKIP: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.06s)
--- SKIP: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.11s)
--- SKIP: TestLegacyRetirementE2E_AliasWindowRoutesPlainEnglishWithNotice (0.00s)
--- SKIP: TestLegacyRetirementE2E_ExpiredSlashCommandDoesNotInvokeScenario (0.00s)
--- SKIP: TestMicroToolsE2E_ConvertsThreeCupsFlourToGrams (0.05s)
--- SKIP: TestMicroToolsE2E_CalculatorRejectsUnsafeExpression (0.05s)
ok      github.com/smackerel/smackerel/tests/e2e/agent  7.615s
ok      github.com/smackerel/smackerel/tests/e2e/assistant      52.856s
ok      github.com/smackerel/smackerel/tests/e2e/auth   0.320s
ok      github.com/smackerel/smackerel/tests/e2e/capture        0.006s
ok      github.com/smackerel/smackerel/tests/e2e/drive  11.563s
ok      github.com/smackerel/smackerel/tests/e2e/foundation     1.866s
ok      github.com/smackerel/smackerel/tests/e2e/legacy_retirement      0.394s
ok      github.com/smackerel/smackerel/tests/e2e/microtools     0.032s
ok      github.com/smackerel/smackerel/tests/e2e/openknowledge  0.086s
ok      github.com/smackerel/smackerel/tests/e2e/policy 0.010s
ok      github.com/smackerel/smackerel/tests/e2e/transports     0.057s
ok      github.com/smackerel/smackerel/tests/e2e/wiki   0.082s
PASS: go-e2e
Skipping Ollama agent E2E (set SMACKEREL_TEST_OLLAMA=1 to enable tests/e2e/agent/happy_path_test.go)
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
Container smackerel-test-smackerel-core-1 Removed
Container smackerel-test-smackerel-ml-1 Removed
Container smackerel-test-postgres-1 Removed
Container smackerel-test-nats-1 Removed
Volume smackerel-test-ollama-data Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-postgres-data Removed
Network smackerel-test_default Removed
```

**Result:** FAIL (zero-skip policy) - the package runner passed 12 packages and recorded 126
top-level passes, but it also recorded 20 top-level Go skips plus one harness-
level opt-in Ollama-suite skip. No independent failure was fixed inline.

### Broader Go Unit Regression

**Phase:** test
**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/cmd/config-validate      0.036s
ok      github.com/smackerel/smackerel/cmd/core 1.373s
ok      github.com/smackerel/smackerel/internal/assistant/capturefallback      0.418s
ok      github.com/smackerel/smackerel/internal/config  30.452s
ok      github.com/smackerel/smackerel/internal/preflight       0.033s
ok      github.com/smackerel/smackerel/internal/scheduler       (cached)
ok      github.com/smackerel/smackerel/internal/topics  (cached)
ok      github.com/smackerel/smackerel/internal/web     (cached)
ok      github.com/smackerel/smackerel/tests/e2e/agent  (cached)
ok      github.com/smackerel/smackerel/tests/eval/assistant     (cached)
ok      github.com/smackerel/smackerel/tests/observability      (cached)
ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
ok      github.com/smackerel/smackerel/web/pwa/tests    (cached)
[go-unit] go test ./... finished OK
```

**Result:** PASS - 140 Go packages passed and 18 packages reported no test
files; no unit test emitted `SKIP`.

### Check, Lint, And Format

**Phase:** test
**Executed:** YES (current session)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check`; `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh lint`; `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh format --check`
**Exit Code:** 0, 0, 0
**Claim Source:** executed
**Output:**

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
All checks passed!
=== Validating web manifests ===
OK: PWA manifest has required fields
OK: Chrome extension manifest has required fields (MV3)
OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Validating JS syntax ===
OK: web/pwa/app.js
OK: web/pwa/sw.js
OK: web/extension/background.js
OK: web/extension/popup/popup.js
OK: Extension versions match (1.0.0)
Web validation passed
75 files already formatted
```

**Result:** PASS - all three canonical quality commands exited zero.

### Test Fidelity Audit

- The focused integration test uses the production migration entry point,
	disposable PostgreSQL, production `Lifecycle.UpdateAllMomentum`, and persisted
	round-trip assertions. It contains no internal mock or request interception.
- The duplicate-safety assertion is adversarial: removing canonical edge
	uniqueness would make the expected SQLSTATE/constraint assertion fail; changing
	star aggregation from two distinct artifact IDs would make persisted momentum
	differ from `11.5`.
- The scheduler regression uses a real closed PostgreSQL pool and asserts the
	code-produced structured log output contains failure and query context while
	excluding success.
- The targeted shell E2E exercises the live disposable stack and has direct
	state, momentum, HTTP, and rendered-content assertions.
- `traceContracts.observability.posture` is `wired`, but this packet's Test Plan
	declares no `observabilityWorkflow`; G080/G100 capture is therefore a clean
	no-op for this test phase.

### Uncertainty Declarations

- **Pre-fix PostgreSQL RED remains unchecked:** this test phase started at the
	already-repaired authorized HEAD. The prior append-only bug-phase RED remains
	historical evidence; it was not re-attributed as current-session test-owner
	execution.
- **Scenario-specific E2E for every fixed behavior remains unchecked:** the
	targeted full-stack flow proves the existing lifecycle surface, while the
	actual star aggregate and scheduler failure branch are directly proved by the
	integration and unit regressions prescribed by the packet.
- **Broader E2E remains unchecked:** the command exited zero, but 20 Go tests and
	one opt-in harness suite did not execute.
- **Test phase completion remains unclaimed:** a zero-skip verdict is not
	supportable, so no `completedPhaseClaims` entry, scope completion, or
	certification mutation is made.
- **Documentation alignment remains unchecked:** this test owner did not claim a
	docs, regression, security, validation, or audit phase.

### Packet-Local Guard Results

**Phase:** test
**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count`; `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count`; `bash .github/bubbles/scripts/implementation-reality-scan.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count --verbose`; `bash .github/bubbles/scripts/regression-quality-guard.sh tests/integration/topic_lifecycle_momentum_test.go internal/scheduler/jobs_test.go tests/e2e/test_topic_lifecycle.sh`; `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/integration/topic_lifecycle_momentum_test.go internal/scheduler/jobs_test.go tests/e2e/test_topic_lifecycle.sh`; scoped marker `grep`
**Exit Code:** 0 for every command
**Claim Source:** executed
**Output:**

```text
Artifact lint PASSED.
state.json uses deprecated field 'scopeProgress'
state.json uses deprecated field 'scopeLayout'
scenario-manifest.json covers 3 scenario contract(s)
All linked tests from scenario-manifest.json exist
Scenarios checked: 3
Scenario-to-row mappings: 3
DoD fidelity scenarios: 3 (mapped: 3, unmapped: 0)
RESULT: PASSED (0 warnings)
Files scanned: 1
Violations: 0
Warnings: 0
PASSED: No source code reality violations detected
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Adversarial signal detected in tests/integration/topic_lifecycle_momentum_test.go
Adversarial signal detected in internal/scheduler/jobs_test.go
Adversarial signal detected in tests/e2e/test_topic_lifecycle.sh
Files with adversarial signals: 3
SCOPED TEST MARKER CHECK: PASS
skip markers: 0
interception markers: 0
internal mock markers: 0
```

**Result:** PASS with two pre-existing, non-blocking artifact-schema deprecation
advisories. No deprecated state field was removed during this bounded test phase.

### Final Test Resource Cleanup

**Phase:** test
**Executed:** YES (current session)
**Command:** `docker ps -a --filter 'name=smackerel-test'`; `docker network ls --filter 'name=smackerel-test'`; `docker volume ls --filter 'name=smackerel-test'`; `pgrep -af 'smackerel\.sh test|smackerel-test|test_topic_lifecycle'`; `git status --short`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
FINAL SMACKEREL TEST RESOURCE CHECK
containers:
<none>
networks:
<none>
volumes:
<none>
processes:
<none>
tracked worktree delta:
M specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/report.md
M specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scopes.md
M specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/state.json
M tests/integration/topic_lifecycle_momentum_test.go
resource verdict:
PASS: zero residual smackerel-test resources or processes
```

**Result:** PASS - no disposable Smackerel test state or process remained.

## Discovered Issues

| Observed | Description | Disposition | Reference |
|---|---|---|---|
| 2026-07-20 | Canonical `./smackerel.sh test e2e` passed all 12 executed Go packages but emitted 20 top-level Go skips and omitted the opt-in Ollama agent suite; independent of BUG-003-002 | route-required: `bubbles.bug` to classify the repo-wide zero-skip test-harness debt; no inline fix in this packet | `specs/074-capture-as-fallback-policy/`; `tests/e2e/agent/openknowledge_e2e_test.go`; `tests/e2e/assistant/` |

### Assertion-Only Transition Guard

**Phase:** test
**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/state-transition-guard.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count --target-status done --expect-workflow-mode bugfix-fastlane --expect-contract-digest sha256:aa91472c047d3d985d38c1d308feb1e6081955b2aa553816deb5987d9cdc449f`
**Exit Code:** 1
**Claim Source:** executed
**Output:**

```text
Current state.json status: in_progress
Current workflowMode: bugfix-fastlane
DoD items total: 15 (checked: 11, unchecked: 4)
BLOCK: Resolved scope artifacts have 4 UNCHECKED DoD items
BLOCK: Resolved scope artifacts have 1 scope(s) still marked 'In Progress'
PASS: completedScopes count matches artifact Done scope count (0)
BLOCK: Required phase 'implement' NOT in execution/certification phase records
BLOCK: Required phase 'test' NOT in execution/certification phase records
BLOCK: Required phase 'regression' NOT in execution/certification phase records
BLOCK: Required phase 'simplify' NOT in execution/certification phase records
BLOCK: Required phase 'stabilize' NOT in execution/certification phase records
BLOCK: Required phase 'security' NOT in execution/certification phase records
BLOCK: Required phase 'validate' NOT in execution/certification phase records
BLOCK: Required phase 'audit' NOT in execution/certification phase records
PASS: All 11 checked DoD items across resolved scope files have evidence blocks
PASS: Artifact lint passes (exit 0)
PASS: Implementation reality scan passed
BLOCK: Retro convergence health failed - Gate G090
PASS: Discovered-issue disposition clean - no unfiled deferrals (Gate G095)
TRANSITION BLOCKED: 12 failure(s), 3 warning(s)
failedGateIds: [G022,G090]
failedChecks: [Check-4-completion,Check-5-all-done]
blockingCode: DELIVERY_COMPLETION_FAILED
failureCount: 12
exitStatus: 1
verdict: FAIL
```

**Result:** EXPECTED FAIL - this was a non-mutating assertion. No
`--revert-on-fail` flag was supplied, no status was promoted or reverted, and no
certification field was changed.

### Test Phase Guard Delta

- Dispatch baseline: 5 of 15 DoD items checked; 10 unchecked.
- Test-owner delta: 6 additional supported items checked; 11 of 15 now checked;
	4 remain unchecked.
- G095 moved from failed to passed after the independent E2E skip finding gained
	a dated `route-required` disposition.
- G027 remained clean because `execution.completedPhaseClaims` did not receive a
	`test` completion claim while Scope 1 remains `In Progress`.
- Final failed gate IDs are `G022` and `G090`; final transition failure count is
	12 with 3 warnings.
- The three warnings are non-terminal packet hygiene signals: no top-level
	`completedAt`, nine historical evidence blocks lacking terminal-signal
	heuristics, and one narrative-summary phrase. They were not reclassified as
	test success.

## Implement Evidence - 2026-08-29

### Implement Owner Statement

The implement phase began from clean source revision
`a6e927b2df7dfbe8e3e478be71a0c547fa3e67f1`. No product source or test change
was required. Current source inspection confirmed these bounded facts:

1. `Lifecycle.UpdateAllMomentum` derives the explicit-star input from distinct
	starred artifacts linked by canonical artifact-to-topic `BELONGS_TO` edges.
2. Initial query failures retain `query topics` context. The scheduler logs the
	failure branch and emits the success message only from the `else` branch.
3. `WebHandler.TopicsPage` reads ordered topic rows and passes the resulting
	topic names to `topics.html`, matching the narrowed SCN-003 rendering claim.
4. The scenario manifest still identifies R-020 as the open recalculation-path
	coverage obligation. This implement phase does not claim that obligation.

The packet remains `in_progress`. This phase does not claim test ownership or
certification, and it does not change any test-owned DoD item.

### Source And Narrowed Manifest Inspection

**Phase:** implement
**Executed:** YES (current session)
**Command:** `grep -nE "COUNT\(DISTINCT a.id\)|JOIN artifacts a|e.src_type = 'artifact'|e.dst_type = 'topic'|e.dst_id = t.id|e.edge_type = 'BELONGS_TO'|a.user_starred IS TRUE|return fmt.Errorf\(\"query topics|func \(s \*Scheduler\) doTopicMomentumJob|if err := s.lifecycle.UpdateAllMomentum|topic momentum update failed|topic momentum updated|func \(h \*Handler\) TopicsPage|SELECT id, name, state, capture_count_total, last_active|FROM topics ORDER BY momentum_score|Templates.ExecuteTemplate\(w, \"topics.html\"|\"Topics\":[[:space:]]+topics|\"title\": \"Existing topic lifecycle surface remains available\"|\"then\": \"the topics surface returns 200 and renders the seeded lifecycle topic by name\"|coverageNote.*R-020" internal/topics/lifecycle.go internal/scheduler/jobs.go internal/web/handler.go specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scenario-manifest.json`
**Exit Code:** 0
**Claim Source:** interpreted
**Interpretation:** The captured lines directly show the production query predicates,
error/success branch separation, topic-template data path, narrowed SCN-003 claim,
and the unchanged R-020 marker. Relating those source lines to the combined
packet claim requires source reconciliation rather than a runtime assertion.
**Output:**

```text
# BUG-003-002 implement source and manifest inspection at a6e927b2
$ grep -nE COUNT\(DISTINCT a.id\)|JOIN artifacts a|e.src_type = 'artifact'|e.dst_type = 'topic'|e.dst_id = t.id|e.edge_type = 'BELONGS_TO'|a.user_starred IS TRUE|return fmt.Errorf\("query topics|func \(s \*Scheduler\) doTopicMomentumJob|if err := s.lifecycle.UpdateAllMomentum|topic momentum update failed|topic momentum updated|func \(h \*Handler\) TopicsPage|SELECT id, name, state, capture_count_total, last_active|FROM topics ORDER BY momentum_score|Templates.ExecuteTemplate\(w, "topics.html"|"Topics":[[:space:]]+topics|"title": "Existing topic lifecycle surface remains available"|"then": "the topics surface returns 200 and renders the seeded lifecycle topic by name"|coverageNote.*R-020 internal/topics/lifecycle.go internal/scheduler/jobs.go internal/web/handler.go specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scenario-manifest.json
exit: 0
lines: 20
sha256: 4dde474eb5fcb0f5eb05d8203186921a467c9c459a361a71de9dd2fc21c43c6a
--- output ---
internal/topics/lifecycle.go:123:                   COALESCE((SELECT COUNT(DISTINCT a.id)
internal/topics/lifecycle.go:125:                             JOIN artifacts a ON a.id = e.src_id AND e.src_type = 'artifact'
internal/topics/lifecycle.go:126:                             WHERE e.dst_type = 'topic' AND e.dst_id = t.id
internal/topics/lifecycle.go:127:                               AND e.edge_type = 'BELONGS_TO' AND a.user_starred IS TRUE), 0)::int,
internal/topics/lifecycle.go:134:            return fmt.Errorf("query topics: %w", err)
internal/scheduler/jobs.go:202:func (s *Scheduler) doTopicMomentumJob() {
internal/scheduler/jobs.go:205: if err := s.lifecycle.UpdateAllMomentum(ctx); err != nil {
internal/scheduler/jobs.go:206:         slog.Error("topic momentum update failed", "error", err)
internal/scheduler/jobs.go:208:         slog.Info("topic momentum updated")
internal/web/handler.go:368:            "Topics":      topicsParsed,
internal/web/handler.go:449:func (h *Handler) TopicsPage(w http.ResponseWriter, r *http.Request) {
internal/web/handler.go:461:            SELECT id, name, state, capture_count_total, last_active
internal/web/handler.go:462:            FROM topics ORDER BY momentum_score DESC, capture_count_total DESC
internal/web/handler.go:467:            if err := h.Templates.ExecuteTemplate(w, "topics.html", map[string]interface{}{
internal/web/handler.go:504:    if err := h.Templates.ExecuteTemplate(w, "topics.html", map[string]interface{}{
internal/web/handler.go:506:            "Topics":   topics,
internal/web/handler.go:701:    h.Templates.ExecuteTemplate(w, "notifications-ntfy-source.html", map[string]interface{}{"Title": "ntfy Source", "Source": source, "Topics": topics, "DeadLetters": deadLetters, "LastEvent": lastEvent})
specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scenario-manifest.json:68:      "title": "Existing topic lifecycle surface remains available",
specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scenario-manifest.json:72:        "then": "the topics surface returns 200 and renders the seeded lifecycle topic by name"
specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scenario-manifest.json:80:      "coverageNote": "Narrowed 2026-08-29 from 'renders the lifecycle topics and momentum values'. The momentum half was not supported by the linked test: it seeds momentum_score as a literal and reads the same row back, so it cannot fail however badly recalculation breaks. Proven by injecting the pre-fix t.star_count fault and running the full e2e lane, which PASSED. The rendering half IS genuinely exercised via GET /topics against WebHandler.TopicsPage, so the scenario is anchored there. The uncovered recalculation path is tracked as R-020.",
```

**Result:** PASS for current source-to-manifest reconciliation. This is not a
replacement for test-owner runtime evidence.

### Repository Check

**Phase:** implement
**Executed:** YES (current session)
**Command:** `./smackerel.sh check`
**Exit Code:** 0
**Claim Source:** executed
**Path normalization:** The repository root is rendered as `~/smackerel` to
avoid recording a workstation home path. All other output is unchanged.
**Output:**

```text
# BUG-003-002 implement check at a6e927b2
$ ./smackerel.sh check
exit: 0
lines: 6
sha256: d45ad8ef5a1e8376323cd59e3b0435033e1341b586f8988f609cc6202fccec50
--- output ---
config-validate: ~/smackerel/config/generated/dev.env.tmp.2259220 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
```

**Result:** PASS. The repository-standard check completed successfully at the
unchanged product source revision.

### Focused Validation Initial Refusal

**Phase:** implement
**Executed:** YES (current session)
**Command:** `./smackerel.sh test unit --go --go-run '^(TestCalculateMomentum.*|TestTransitionState.*|TestDefaultMomentumConfig|TestNewLifecycle|TestTopicMomentumJob_LogsLifecycleQueryFailure)$' --verbose`
**Exit Code:** 1
**Claim Source:** executed
**Output:**

```text
# BUG-003-002 implement focused lifecycle and scheduler validation at a6e927b2
$ ./smackerel.sh test unit --go --go-run ^(TestCalculateMomentum.*|TestTransitionState.*|TestDefaultMomentumConfig|TestNewLifecycle|TestTopicMomentumJob_LogsLifecycleQueryFailure)$ --verbose
exit: 1
lines: 34
sha256: d317ce28abfa6ba4b5ef5e029c3edca6fa12f6551ad50809f19223244308ae66
--- output ---
oom-preflight: OK — 28922 MB available (need 6000 MB; swap used 807 MB).

  ┌─ disk-preflight: REFUSED — not enough free disk ──────────────────┐
  │  C: (backs the vhdx): 23     GB free   required: 40   GB
  │  WSL / (ext4)       : 490    GB free   required: 25   GB
  │
  │  A heavy build here can fill the disk and wedge the daemon.
  │  Reclaim space first, then re-run. Biggest levers, in order:
  │
  │   1) Safe Docker reclaim (keeps labeled-persistent volumes):
  │        docker-safe-prune.sh          # dry-run first
  │        docker-safe-prune.sh --apply
  │   2) Stop idle project stacks you are NOT working in:
  │        ./wanderaide.sh deploy stop complete
  │        ./guesthost.sh stop all
  │        ./quantitativefinance.sh --env=dev down
  │        ./smackerel.sh down
  │   3) Journald + apt (needs sudo password — run yourself):
  │        sudo journalctl --vacuum-size=200M && sudo apt-get clean
  │   4) If C: is low but WSL / looks fine, the vhdx is holding slack.
  │      Reclaim needs a full WSL shutdown + compact from Windows:
  │        wsl --shutdown            (kills ALL sessions — schedule it)
  │        Optimize-VHD -Path <ext4.vhdx> -Mode Full   (elevated)
  │
  │  Current Docker footprint:
  │      TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE
  │      Images          95        48        41.23GB   21.32GB (51%)
  │      Containers      74        74        32.97MB   0B (0%)
  │      Local Volumes   547       17        187.7GB   179.5GB (95%)
  │      Build Cache     226       0         25.6GB    19.36GB
  │
  │  Override (only if you KNOW it fits): DISK_PREFLIGHT_OVERRIDE=1
  └────────────────────────────────────────────────────────────────────┘
```

**Result:** REFUSED before test execution by the repository disk gate. The
successful bounded rerun below used the override printed by that gate.

### Focused Lifecycle And Scheduler Validation

**Phase:** implement
**Executed:** YES (current session)
**Command:** `env DISK_PREFLIGHT_OVERRIDE=1 ./smackerel.sh test unit --go --go-run '^(TestCalculateMomentum.*|TestTransitionState.*|TestDefaultMomentumConfig|TestNewLifecycle|TestTopicMomentumJob_LogsLifecycleQueryFailure)$' --verbose`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
# BUG-003-002 implement focused lifecycle and scheduler validation at a6e927b2 with disk preflight override
$ env DISK_PREFLIGHT_OVERRIDE=1 ./smackerel.sh test unit --go --go-run ^(TestCalculateMomentum.*|TestTransitionState.*|TestDefaultMomentumConfig|TestNewLifecycle|TestTopicMomentumJob_LogsLifecycleQueryFailure)$ --verbose
exit: 0
lines: 578
sha256: 19e585e7f3ca1ca57b992f326d1cd84de09a284cab94da8983c3bb07284cf94c
--- first 20 ---
oom-preflight: OK — 28940 MB available (need 6000 MB; swap used 807 MB).
disk-preflight: OVERRIDE set — skipping disk gate.
++ dirname /workspace/scripts/runtime/go-unit.sh
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
[go-unit] envsubst missing — installing gettext-base
+ echo '[go-unit] envsubst missing — installing gettext-base'
+ apt-get update -qq
+ apt-get install -y --no-install-recommends gettext-base
Reading package lists...
Building dependency tree...
Reading state information...
The following NEW packages will be installed:
  gettext-base
0 upgraded, 1 newly installed, 0 to remove and 20 not upgraded.
Need to get 160 kB of archives.
After this operation, 660 kB of additional disk space will be used.
Get:1 http://deb.debian.org/debian bookworm/main amd64 gettext-base amd64 0.21-12 [160 kB]
--- omitted 538 line(s); sha256 above covers the full output ---
--- last 20 ---
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.008s [no tests to run]
?       github.com/smackerel/smackerel/tests/integration/agent/routerwarmup    [no test files]
?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
?       github.com/smackerel/smackerel/tests/integration/nslock [no test files]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/observability      0.005s [no tests to run]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/stress/readiness   0.020s [no tests to run]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/unit/clients       0.022s [no tests to run]
?       github.com/smackerel/smackerel/web/pwa  [no test files]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/web/pwa/tests    0.123s [no tests to run]
+ echo '[go-unit] go test ./... finished OK'
[go-unit] go test ./... finished OK
```

**Result:** PASS. The selected momentum, state-transition, lifecycle-constructor,
and scheduler failure-path tests completed successfully. Unselected packages
reported their normal no-match messages because the repository runner applies
the regular expression across `./...`.

### Implement Phase Boundary

No product source, test, managed documentation, configuration, deployment, or
framework-managed file was modified by this implement closeout. The only owned
updates are implement evidence and execution-state provenance inside this bug
packet. Scope and certification status remain `in_progress`.

## Current-Session Test Closeout - 2026-08-29

### Test Owner Statement

This closeout reconciles executions already recorded in the current top-level
session. It does not rerun any test. The current tree preserves every product
and test input named by the scenario receipts from source revision
`a6e927b2df7dfbe8e3e478be71a0c547fa3e67f1`.

The test phase supports the packet's three active scenario contracts. The
physical regression categories remain proportionate: integration for SCN-001,
unit for SCN-002, and E2E API for SCN-003.

### Packet-Only Input Closure

**Phase:** test
**Executed:** YES (current session, evidence reconciliation only)
**Command:** `printf` framing, `git diff --name-only a6e927b2..HEAD`, `sha256sum` over the six receipt inputs, and `git diff --quiet a6e927b2..HEAD -- <six receipt inputs>`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
BUG-003-002 PACKET INPUT CLOSURE
receipt revision=a6e927b2df7dfbe8e3e478be71a0c547fa3e67f1
current revision=fc5f1d439c5543f8aea6a9b13be78dc9a9859eb2
changed paths a6e927b2..HEAD:
specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/report.md
specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/state.json
current product/test input hashes:
cb6ff3e629dec36fe4b49c3b4c7851d13d06237fd66bef209e772807cda108c7  internal/topics/lifecycle.go
07d35d2ebb6ed5010bb73e92413c034d856256943a39750369a856418685b1a4  internal/scheduler/jobs.go
5d4f11bad41e09d9558b2110a203a5431d2e835ea0c4caf8e100a5872b638b6a  internal/scheduler/jobs_test.go
67ca9b0c88a4c278ee98a3f5df11990d13ce7e98c9c16008e060bece579ad14a  internal/web/handler.go
87ab31988ef6325f02310d939c11241c0bf3ec2c9255e95d857ecca7893468eb  tests/integration/topic_lifecycle_momentum_test.go
df1bdd6f8c69580deab72debf21adbc315e59c4b2459c7dd67ec7978ba359998  tests/e2e/test_topic_lifecycle.sh
product/test input closure unchanged=yes
closure verdict=PASS
```

**Result:** PASS. The packet-only commit did not invalidate the recorded
product or test input closures.

### Current-Session Scenario Receipts

**Phase:** test
**Executed:** YES (current session receipts)
**Command:** `jq -s -r 'to_entries[] | select((.key + 1) >= 89 and (.key + 1) <= 102) | "line=\(.key + 1) scenario=\(.value.scenarioBinding.scenarioId) phase=\(.value.scenarioBinding.phase) exit=\(.value.exitCode) cmd=\(.value.cmd) sourceRevision=\(.value.scenarioBinding.sourceRevision[0:12]) stdoutSha256=\(.value.stdoutHash)"' .specify/runtime/tool-calls.jsonl`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
line=89 scenario=BUG-003-002-SCN-001 phase=red exit=1 cmd=./smackerel.sh test integration-light --go-run ^TestTopicLifecycleMomentumFromPersistedStars$ sourceRevision=a6e927b2df7d stdoutSha256=e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
line=90 scenario=BUG-003-002-SCN-001 phase=red exit=1 cmd=./smackerel.sh test integration-light --go-run ^TestTopicLifecycleMomentumFromPersistedStars$ sourceRevision=a6e927b2df7d stdoutSha256=bf593e054b8e56c085ed7d9a2ef5f4b39ebbb312405c8636bd3fb617771f730c
line=91 scenario=BUG-003-002-SCN-002 phase=red exit=1 cmd=./smackerel.sh test unit --go --go-run ^TestTopicMomentumJob_LogsLifecycleQueryFailure$ --verbose sourceRevision=a6e927b2df7d stdoutSha256=966e73d2a31b0c6f02776fcbc6196364891af162b9ba8b5cbb8be8ff85a8a0d0
line=92 scenario=BUG-003-002-SCN-003 phase=red exit=1 cmd=./smackerel.sh test e2e --shell-run test_topic_lifecycle.sh sourceRevision=a6e927b2df7d stdoutSha256=3bcb40e9a1c4d751ab5c0f15ccb854e3081dbe96a75427ef8be75bc0538211e8
line=93 scenario=BUG-003-002-SCN-001 phase=green exit=0 cmd=./smackerel.sh test integration-light --go-run ^TestTopicLifecycleMomentumFromPersistedStars$ sourceRevision=a6e927b2df7d stdoutSha256=fbc4941ba8e62e9403188ae206684b00e6117163ded6eee1937da75a4bdab3c4
line=94 scenario=BUG-003-002-SCN-001 phase=implement exit=0 cmd=git --no-pager grep -n -A 11 -B 1 COUNT(DISTINCT a.id) HEAD -- internal/topics/lifecycle.go sourceRevision=a6e927b2df7d stdoutSha256=5a295d6515810be919741310b5b123ab7e2e39b6aabf055b5cd402611051ce59
line=95 scenario=BUG-003-002-SCN-002 phase=implement exit=0 cmd=git --no-pager grep -n -A 7 -B 1 func (s \*Scheduler) doTopicMomentumJob HEAD -- internal/scheduler/jobs.go sourceRevision=a6e927b2df7d stdoutSha256=dfdb16333f7ac23c213b97f89479b915b4edb919fe719ee99a81fb5fdbe42fb3
line=96 scenario=BUG-003-002-SCN-003 phase=implement exit=0 cmd=git --no-pager grep -n -A 12 -B 2 "Topics":   topics, HEAD -- internal/web/handler.go sourceRevision=a6e927b2df7d stdoutSha256=cc11328964e84a8688dbe59681d10018d8d102ffc28eaa3d0871f21331f20005
line=97 scenario=BUG-003-002-SCN-002 phase=green exit=0 cmd=./smackerel.sh test unit --go --go-run ^TestTopicMomentumJob_LogsLifecycleQueryFailure$ --verbose sourceRevision=a6e927b2df7d stdoutSha256=ef1bbcffa3bec7e267594ab377e13d2a17c5249efa60f4b2cf400a587cb5c69a
line=98 scenario=BUG-003-002-SCN-003 phase=green exit=0 cmd=./smackerel.sh test e2e --shell-run test_topic_lifecycle.sh sourceRevision=a6e927b2df7d stdoutSha256=44b2d72cd3f3073aa48f3b3745f4fde524f4ea4886835a2e988379db566fc3f6
line=99 scenario=BUG-003-002-SCN-001 phase=regression exit=0 cmd=./smackerel.sh test integration --go-run TestTopicLifecycle sourceRevision=a6e927b2df7d stdoutSha256=5f117888282eb96ef81631bf3f752338d9ce7cc5cffae78edc87fd24d0f49472
line=100 scenario=BUG-003-002-SCN-002 phase=regression exit=0 cmd=./smackerel.sh test unit --go sourceRevision=a6e927b2df7d stdoutSha256=4b366167c9665b2d0a9ae7f85ef7aa1720745bcdae6ae0fcacbfa53d14015d27
line=101 scenario=BUG-003-002-SCN-003 phase=regression exit=1 cmd=./smackerel.sh test e2e --shell-run test_topic_lifecycle.sh sourceRevision=a6e927b2df7d stdoutSha256=e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
line=102 scenario=BUG-003-002-SCN-003 phase=regression exit=0 cmd=./smackerel.sh test e2e --shell-run test_topic_lifecycle.sh sourceRevision=a6e927b2df7d stdoutSha256=1db229d6922fad7307578c83a9dec4be5f20b13e446297a024f4f8064debeec5
```

The first SCN-001 RED receipt stopped before test output. The second RED receipt
contains the executed PostgreSQL counterexample and is the controlling RED.
The SCN-003 regression receipt at line 101 also stopped before test output. The
line 102 receipt is the controlling successful regression rerun.

### Scenario State Reconciliation

**Phase:** test
**Executed:** YES (current session, receipt resolver only)
**Command:** `bash .github/bubbles/scripts/scenario-state-resolve.sh --spec-dir specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count --source-revision a6e927b2df7dfbe8e3e478be71a0c547fa3e67f1 --require RED_VERIFIED --require IMPLEMENTED --require GREEN_TARGETED --require REGRESSION_GREEN --certifiable`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
scenario-state-resolve: specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count
	source revision: a6e927b2df7d
	BUG-003-002-SCN-001  state=REGRESSION_GREEN  derived=[PLANNED RED_VERIFIED IMPLEMENTED GREEN_TARGETED REGRESSION_GREEN]
	BUG-003-002-SCN-002  state=REGRESSION_GREEN  derived=[PLANNED RED_VERIFIED IMPLEMENTED GREEN_TARGETED REGRESSION_GREEN]
	BUG-003-002-SCN-003  state=REGRESSION_GREEN  derived=[PLANNED RED_VERIFIED IMPLEMENTED GREEN_TARGETED REGRESSION_GREEN]
	REFUSED SCS-REVISION-DRIFT [SCN-064-003-03]: receipt cites source revision ec31eddc6b1d but the resolved revision is a6e927b2df7d
	REFUSED SCS-REVISION-DRIFT [SCN-064-003-03]: receipt cites source revision ec31eddc6b1d but the resolved revision is a6e927b2df7d
	REFUSED SCS-REVISION-DRIFT [SCN-064-003-03]: receipt cites source revision ec31eddc6b1d but the resolved revision is a6e927b2df7d
	REFUSED SCS-REVISION-DRIFT [SCN-064-003-04]: receipt cites source revision ec31eddc6b1d but the resolved revision is a6e927b2df7d
	REFUSED SCS-REVISION-DRIFT [SCN-064-003-04]: receipt cites source revision ec31eddc6b1d but the resolved revision is a6e927b2df7d
	(all 85 refusals are SCS-REVISION-DRIFT: superseded receipts, excluded from derivation, not blocking)
	certifiable: yes
```

**Result:** PASS. All three active scenarios reached the required derived
states from receipts at the unchanged product and test input closure.

### Broader E2E And Unit Receipts

**Phase:** test
**Executed:** YES (current top-level session receipts)
**Command:** existing recorded commands `env DISK_PREFLIGHT_OVERRIDE=1 ./smackerel.sh test e2e` and `./smackerel.sh test unit --go`
**Exit Code:** 0 for both commands
**Claim Source:** interpreted
**Interpretation:** The full E2E receipt predates the packet-only a6e927b2
commit. The two intervening commit ranges contain no product or test input.
The receipt therefore remains valid for the unchanged execution closure. The
full E2E result is broad regression evidence only.
**Output:**

```text
line=87 session=auto-20260829T180049-4048534
cmd=env DISK_PREFLIGHT_OVERRIDE=1 ./smackerel.sh test e2e
exit=0
sourceRevision=6856d3160ef19a01b542069f711e225f4be3c8f5
stdoutSha256=24be09da4471777ce746a55afeb66c25317955be8045c1e1e666d4e77c39b191
git diff --name-status 6856d316..a6e927b2:
M .specify/memory/open-work.md
M specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scenario-manifest.json
git diff --name-only a6e927b2..fc5f1d43:
specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/report.md
specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/state.json
line=100 session=vscode-0cc9797b22a783b6a9be5d7946541ba3
cmd=./smackerel.sh test unit --go
exit=0
sourceRevision=a6e927b2df7dfbe8e3e478be71a0c547fa3e67f1
stdoutSha256=4b366167c9665b2d0a9ae7f85ef7aa1720745bcdae6ae0fcacbfa53d14015d27
product/test input closure unchanged=yes
```

**Result:** PASS for the recorded broader E2E and Go unit command outcomes at
the preserved input closure.

### R-020 Limitation

R-020 remains explicit. The full E2E lane returned exit zero while its earlier
fault restored the invalid `t.star_count` query. That run cannot prove lifecycle
recalculation. It is used only as broader regression evidence.

SCN-003 now proves that `GET /topics` renders the seeded topic by name. Its RED,
GREEN, and REGRESSION receipts use the `TopicsPage` render model as the negative
control. SCN-001 proves recalculation through the real PostgreSQL integration
path. This closeout makes no recalculation E2E claim.

### Packet Guard Closeout

**Phase:** test
**Executed:** YES (current session)
**Command:** packet `artifact-lint.sh`, `traceability-guard.sh`, `implementation-reality-scan.sh --verbose`, standard `regression-quality-guard.sh`, and `regression-quality-guard.sh --bugfix`
**Exit Code:** 0 for every command
**Claim Source:** executed
**Output:**

```text
Artifact lint PASSED.
✅ scenario-manifest.json covers 3 scenario contract(s)
✅ All linked tests from scenario-manifest.json exist
ℹ️  Scenarios checked: 3
ℹ️  Test rows checked: 6
ℹ️  Scenario-to-row mappings: 3
ℹ️  Concrete test file references: 3
ℹ️  Report evidence references: 3
ℹ️  DoD fidelity scenarios: 3 (mapped: 3, unmapped: 0)
RESULT: PASSED (0 warnings)
Files scanned:  1
Violations:     0
Warnings:       0
🟢 PASSED: No source code reality violations detected
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 3
✅ Adversarial signal detected in tests/integration/topic_lifecycle_momentum_test.go
✅ Adversarial signal detected in internal/scheduler/jobs_test.go
✅ Adversarial signal detected in tests/e2e/test_topic_lifecycle.sh
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files with adversarial signals: 3
artifact_lint=0
traceability=0
implementation_reality=0
regression_quality=0
regression_quality_bugfix=0
```

**Result:** PASS. The five requested packet and regression guards accepted the
reconciled packet before its final execution-state update.

## Independent Regression Review - 2026-08-29

### Review Boundary And Verdict

This review started from `cf005ed4d59cb1de3f025376f917dbc7ce6d4843`.
The tested closure was `a6e927b2df7dfbe8e3e478be71a0c547fa3e67f1`.

No post-closure change touched the momentum query, formula tests, scheduler
failure test, topic page, integration regression, targeted E2E, or scenario
manifest. The intervening product-wide input change updated the Go dependency
set and Docker builder security floor. This change can affect compilation and
container builds, so this review reran the focused unit, integration, and E2E
rows at current HEAD. It also ran the changed builder contract as a canary.

The focused current-HEAD closure is regression-free. This verdict does not
certify the packet and does not replace a full release-suite result.

### Tested-Closure To Current-HEAD Delta

**Phase:** regression
**Executed:** YES (current session)
**Command:** `git diff --name-status a6e927b2df7dfbe8e3e478be71a0c547fa3e67f1..HEAD`; per-file `git diff --quiet` checks; `git diff --numstat a6e927b2df7dfbe8e3e478be71a0c547fa3e67f1..HEAD -- Dockerfile go.mod go.sum internal/deploy/core_dockerfile_go_builder_contract_test.go`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
BUG-003-002 regression delta at HEAD
tested-closure=a6e927b2df7dfbe8e3e478be71a0c547fa3e67f1
current-head=cf005ed4d59cb1de3f025376f917dbc7ce6d4843
changed-paths:
M       Dockerfile
M       go.mod
M       go.sum
M       internal/deploy/core_dockerfile_go_builder_contract_test.go
M       specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/report.md
M       specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scopes.md
M       specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/state.json
focused product/test closure:
UNCHANGED internal/topics/lifecycle.go
UNCHANGED internal/topics/lifecycle_test.go
UNCHANGED internal/scheduler/jobs.go
UNCHANGED internal/scheduler/jobs_test.go
UNCHANGED internal/web/handler.go
UNCHANGED tests/integration/topic_lifecycle_momentum_test.go
UNCHANGED tests/e2e/test_topic_lifecycle.sh
UNCHANGED specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scenario-manifest.json
dependency-builder-delta:
6       1       Dockerfile
4       4       go.mod
8       8       go.sum
49      11      internal/deploy/core_dockerfile_go_builder_contract_test.go
```

**Result:** PASS. The bug's product, test, rendering, and scenario inputs are
unchanged. The dependency and builder changes required current-HEAD execution.

### Test Baseline Comparison

The existing test closeout records successful focused commands at the tested
closure. This review reran the same three behavior rows at current HEAD.

| Category | Tested closure | Current HEAD | Delta | Status |
|---|---|---|---|---|
| Focused unit and functional | selected command exit 0 | selected command exit 0 | stable | CLEAN |
| PostgreSQL integration | selected command exit 0 | selected command exit 0 | stable | CLEAN |
| Targeted E2E API | selected shell row exit 0 | selected shell row exit 0 | stable | CLEAN |
| Builder contract canary | not part of bug closure | selected command exit 0 | added impact check | CLEAN |

No numeric test-count delta is inferred from compact output. The command
registry exposes no separate coverage command, so no numeric coverage
percentage is claimed. The post-closure diff removed no tests or assertions
from the bug's source and test closure.

### Current-HEAD Focused Unit And Functional Regression

**Phase:** regression
**Executed:** YES (current session)
**Command:** `env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run '^(TestCalculateMomentum.*|TestTransitionState.*|TestDefaultMomentumConfig|TestNewLifecycle|TestTopicMomentumJob_LogsLifecycleQueryFailure)$' --verbose`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
# BUG-003-002 regression focused unit at cf005ed4
$ env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run ^(TestCalculateMomentum.*|TestTransitionState.*|TestDefaultMomentumConfig|TestNewLifecycle|TestTopicMomentumJob_LogsLifecycleQueryFailure)$ --verbose
exit: 0
lines: 578
sha256: ea306e7985bbf5f3d69c0a28461b80ce1c03a8211bfac3a9afdf4bbbdf25ca6a
--- first 20 ---
oom-preflight: OK — 28708 MB available (need 6000 MB; swap used 1181 MB).
disk-preflight: OVERRIDE set — skipping disk gate.
++ dirname /workspace/scripts/runtime/go-unit.sh
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
[go-unit] envsubst missing — installing gettext-base
+ echo '[go-unit] envsubst missing — installing gettext-base'
+ apt-get update -qq
+ apt-get install -y --no-install-recommends gettext-base
Reading package lists...
Building dependency tree...
Reading state information...
The following NEW packages will be installed:
	gettext-base
0 upgraded, 1 newly installed, 0 to remove and 20 not upgraded.
Need to get 160 kB of archives.
After this operation, 660 kB of additional disk space will be used.
Get:1 http://deb.debian.org/debian bookworm/main amd64 gettext-base amd64 0.21-12 [160 kB]
--- omitted 538 line(s); sha256 above covers the full output ---
--- last 20 ---
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.019s [no tests to run]
?       github.com/smackerel/smackerel/tests/integration/agent/routerwarmup    [no test files]
?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
?       github.com/smackerel/smackerel/tests/integration/nslock [no test files]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/observability      0.003s [no tests to run]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/stress/readiness   0.008s [no tests to run]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/unit/clients       0.007s [no tests to run]
?       github.com/smackerel/smackerel/web/pwa  [no test files]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/web/pwa/tests    0.136s [no tests to run]
+ echo '[go-unit] go test ./... finished OK'
[go-unit] go test ./... finished OK
```

**Result:** PASS. The focused selector completed through the repository CLI at
current HEAD with the updated dependency set.

### Current-HEAD PostgreSQL Integration Regression

**Phase:** regression
**Executed:** YES (current session)
**Command:** `env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration-light --go-run '^TestTopicLifecycleMomentumFromPersistedStars$'`
**Exit Code:** 0
**Claim Source:** executed
**Path normalization:** The repository root is rendered as `~/smackerel`.
**Output:**

```text
# BUG-003-002 regression PostgreSQL integration at cf005ed4
$ env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration-light --go-run ^TestTopicLifecycleMomentumFromPersistedStars$
exit: 0
lines: 309
sha256: 671631664de2212773f230dcbc672f478adc8a38dbf66143367b21437b96c8a9
--- first 20 ---
oom-preflight: OK — 28279 MB available (need 6000 MB; swap used 1181 MB).
disk-preflight: OVERRIDE set — skipping disk gate.
config-validate: ~/smackerel/config/generated/test.env.tmp.4051951 OK
Smackerel pre-flight resource check: OK
	RAM  available: 28145 MB (required >= 2000 MB)
	Disk available: 497119 MB / 485.5 GB (required >= 8 GB)
 Network smackerel-test_default  Creating
 Network smackerel-test_default  Created
 Volume "smackerel-test-nats-data"  Creating
 Volume "smackerel-test-nats-data"  Created
 Volume "smackerel-test-postgres-data"  Creating
 Volume "smackerel-test-postgres-data"  Created
 Container smackerel-test-postgres-1  Creating
 Container smackerel-test-nats-1  Creating
 Container smackerel-test-nats-1  Created
 Container smackerel-test-postgres-1  Created
 Container smackerel-test-postgres-1  Starting
 Container smackerel-test-nats-1  Starting
 Container smackerel-test-postgres-1  Started
 Container smackerel-test-nats-1  Started
--- omitted 269 line(s); sha256 above covers the full output ---
--- last 20 ---
PASS
ok      github.com/smackerel/smackerel/tests/eval/assistant     0.004s [no tests to run]
go-integration: NOTICE: acceptance-gate executed-assertion assertion NOT ENFORCED for this run — a focused --run selector (^TestTopicLifecycleMomentumFromPersistedStars$) is active. Only a full lane run with no --run selector enforces that TestAcceptanceGate_RoutingAccuracyAndCaptureFallback ran with a non-zero executed-assertion count.
PASS: go-integration-light
Running project-scoped integration-light stack teardown (exit cleanup, timeout 120s)...
config-validate: ~/smackerel/config/generated/test.env.tmp.4101620 OK
 Container smackerel-test-nats-1  Stopping
 Container smackerel-test-postgres-1  Stopping
 Container smackerel-test-nats-1  Stopped
 Container smackerel-test-nats-1  Removing
 Container smackerel-test-nats-1  Removed
 Container smackerel-test-postgres-1  Stopped
 Container smackerel-test-postgres-1  Removing
 Container smackerel-test-postgres-1  Removed
 Volume smackerel-test-nats-data  Removing
 Volume smackerel-test-postgres-data  Removing
 Network smackerel-test_default  Removing
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-nats-data  Removed
 Network smackerel-test_default  Removed
```

**Result:** PASS. The selected canonical PostgreSQL regression completed and
the disposable test resources were removed.

### Current-HEAD Targeted Topic Lifecycle E2E

**Phase:** regression
**Executed:** YES (current session)
**Command:** `env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --shell-run test_topic_lifecycle.sh`
**Exit Code:** 0
**Claim Source:** executed
**Path normalization:** The repository root is rendered as `~/smackerel`.
**Output:**

```text
# BUG-003-002 regression targeted topic E2E at cf005ed4
$ env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --shell-run test_topic_lifecycle.sh
exit: 0
lines: 334
sha256: ddcf656500722703c7af53b0a7f98f9bc9289929280fbe45410f0f09edf2eb52
--- first 20 ---
oom-preflight: OK — 28755 MB available (need 6000 MB; swap used 1181 MB).
disk-preflight: OVERRIDE set — skipping disk gate.
config-validate: ~/smackerel/config/generated/test.env.tmp.4115860 OK
Smackerel pre-flight resource check: OK
	RAM  available: 28662 MB (required >= 6000 MB)
	Disk available: 497106 MB / 485.5 GB (required >= 15 GB)
Running targeted shell E2E: test_topic_lifecycle.sh
Running project-scoped test stack teardown (before targeted shared-stack shell E2E, timeout 180s)...
config-validate: ~/smackerel/config/generated/test.env.tmp.4119024 OK
oom-preflight: OK — 28754 MB available (need 6000 MB; swap used 1181 MB).
disk-preflight: OVERRIDE set — skipping disk gate.
config-validate: ~/smackerel/config/generated/test.env.tmp.4122928 OK
Smackerel pre-flight resource check: OK
	RAM  available: 28726 MB (required >= 6000 MB)
	Disk available: 497105 MB / 485.5 GB (required >= 15 GB)
Preparing disposable test stack...
Building disposable test stack images before up (freshness convention)...
Compose can now delegate builds to bake for better performance.
 To do so, set COMPOSE_BAKE=true.
#0 building with "default" instance using docker driver
--- omitted 294 line(s); sha256 above covers the full output ---
--- last 20 ---
 Container smackerel-test-postgres-1  Removing
 Container smackerel-test-postgres-1  Removed
 Container smackerel-test-intent-compiler-provider-1  Stopped
 Container smackerel-test-intent-compiler-provider-1  Removing
 Container smackerel-test-intent-compiler-provider-1  Removed
 Container smackerel-test-smackerel-ml-1  Stopped
 Container smackerel-test-smackerel-ml-1  Removing
 Container smackerel-test-smackerel-ml-1  Removed
 Container smackerel-test-nats-1  Stopping
 Container smackerel-test-nats-1  Stopped
 Container smackerel-test-nats-1  Removing
 Container smackerel-test-nats-1  Removed
 Volume smackerel-test-nats-data  Removing
 Volume smackerel-test-ollama-data  Removing
 Volume smackerel-test-postgres-data  Removing
 Network smackerel-test_default  Removing
 Volume smackerel-test-ollama-data  Removed
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-nats-data  Removed
 Network smackerel-test_default  Removed
```

**Result:** PASS. The targeted shell E2E rebuilt the disposable stack from the
current Dockerfile, completed, and removed its volumes and network.

### Independent Builder-Delta Canary

**Phase:** regression
**Executed:** YES (current session)
**Command:** `env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run '^TestCoreDockerfileGoBuilderContract_.*$' --verbose`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
# BUG-003-002 independent builder delta canary at cf005ed4
$ env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run ^TestCoreDockerfileGoBuilderContract_.*$ --verbose
exit: 0
lines: 529
sha256: da99b90b5a06e2fbacabd170ac56623fa75876755afc9ea1d5b924a6cbee4792
--- first 20 ---
oom-preflight: OK — 28214 MB available (need 6000 MB; swap used 1185 MB).
disk-preflight: OVERRIDE set — skipping disk gate.
++ dirname /workspace/scripts/runtime/go-unit.sh
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
[go-unit] envsubst missing — installing gettext-base
+ echo '[go-unit] envsubst missing — installing gettext-base'
+ apt-get update -qq
+ apt-get install -y --no-install-recommends gettext-base
Reading package lists...
Building dependency tree...
Reading state information...
The following NEW packages will be installed:
	gettext-base
0 upgraded, 1 newly installed, 0 to remove and 20 not upgraded.
Need to get 160 kB of archives.
After this operation, 660 kB of additional disk space will be used.
Get:1 http://deb.debian.org/debian bookworm/main amd64 gettext-base amd64 0.21-12 [160 kB]
--- omitted 489 line(s); sha256 above covers the full output ---
--- last 20 ---
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.006s [no tests to run]
?       github.com/smackerel/smackerel/tests/integration/agent/routerwarmup    [no test files]
?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
?       github.com/smackerel/smackerel/tests/integration/nslock [no test files]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/observability      0.006s [no tests to run]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/stress/readiness   0.006s [no tests to run]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/unit/clients       0.003s [no tests to run]
?       github.com/smackerel/smackerel/web/pwa  [no test files]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/web/pwa/tests    0.114s [no tests to run]
[go-unit] go test ./... finished OK
+ echo '[go-unit] go test ./... finished OK'
```

**Result:** PASS. The changed builder contract test selection completed under
the current dependency graph.

### Parent And Neighbor Lifecycle Coherence

**Phase:** regression
**Executed:** YES (current session)
**Command:** `grep -nE 'explicit_star_count|Topic Lifecycle System|COUNT\(DISTINCT a.id\)|a.user_starred IS TRUE|UpdateAllMomentum\(ctx\)|two distinct linked starred|unrelated starred|ON CONFLICT \(name\) DO NOTHING|Existing pricing topic owner|HOT_MOMENTUM=|CORE_URL/topics|Topics page shows owned|coverageNote.*R-020' specs/003-phase2-ingestion/spec.md internal/topics/lifecycle.go tests/integration/topic_lifecycle_momentum_test.go tests/e2e/test_topic_lifecycle.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scenario-manifest.json`
**Exit Code:** 0
**Claim Source:** interpreted
**Interpretation:** The parent R-208 star signal, production aggregate,
integration assertions, neighboring duplicate-seed guard, page request, and
R-020 limitation remain mutually consistent. The focused executions above
provide the runtime result.
**Output:**

```text
BUG-003-002 parent and neighbor lifecycle contract inspection
specs/003-phase2-ingestion/spec.md:165:### R-208: Topic Lifecycle System
specs/003-phase2-ingestion/spec.md:172:    explicit_star_count × 5.0 +
internal/topics/lifecycle.go:123:                   COALESCE((SELECT COUNT(DISTINCT a.id)
internal/topics/lifecycle.go:127:                               AND e.edge_type = 'BELONGS_TO' AND a.user_starred IS TRUE), 0)::int,
tests/integration/topic_lifecycle_momentum_test.go:38:  if err := lifecycle.UpdateAllMomentum(ctx); err != nil {
tests/integration/topic_lifecycle_momentum_test.go:53:  t.Log("PASS: two distinct linked starred artifacts contribute exactly 10.0 star momentum")
tests/integration/topic_lifecycle_momentum_test.go:55:  t.Log("PASS: an unrelated starred artifact contributes nothing to the tested topic")
tests/e2e/test_topic_lifecycle.sh:20:ON CONFLICT (name) DO NOTHING;
tests/e2e/test_topic_lifecycle.sh:32:echo "  Existing pricing topic owner: $PRICING_OWNER"
tests/e2e/test_topic_lifecycle.sh:46:HOT_MOMENTUM=$(e2e_psql "SELECT momentum_score FROM topics WHERE id='topic-lifecycle-hot'")
tests/e2e/test_topic_lifecycle.sh:53:  -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/topics")
tests/e2e/test_topic_lifecycle.sh:57:  -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/topics" 2>/dev/null || true)
tests/e2e/test_topic_lifecycle.sh:58:e2e_assert_contains "$BODY" "topic-lifecycle-pricing" "Topics page shows owned lifecycle pricing topic"
specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scenario-manifest.json:80:      "coverageNote": "Narrowed 2026-08-29 from 'renders the lifecycle topics and momentum values'. The momentum half was not supported by the linked test: it seeds momentum_score as a literal and reads the same row back, so it cannot fail however badly recalculation breaks. Proven by injecting the pre-fix t.star_count fault and running the full e2e lane, which PASSED. The rendering half IS genuinely exercised via GET /topics against WebHandler.TopicsPage, so the scenario is anchored there. The uncovered recalculation path is tracked as R-020.",
```

The parent spec's R-208 formula still defines explicit stars as a weighted
signal. The integration test still exercises the real recalculation entrypoint.
The shared E2E still protects BUG-003-001's pre-existing `pricing` fixture and
the `GET /topics` rendering path. No route, table, or API contract changed in
the post-closure delta.

### R-020 Preservation

R-020 remains open and unchanged. The targeted E2E seeds literal
`momentum_score` values. It reads those values and renders the seeded topic by
name. It does not trigger `UpdateAllMomentum` through an E2E-visible entrypoint.

SCN-003 therefore proves only the narrowed `GET /topics` rendering contract.
SCN-001 remains the real PostgreSQL recalculation proof. This regression review
does not claim E2E recalculation coverage.

### Cross-Spec Impact Classification

| Surface | Relationship | Current evidence | Classification |
|---|---|---|---|
| Parent spec 003 Scope 6 and R-208 | Owns the momentum formula and lifecycle behavior | Focused unit and PostgreSQL integration commands exited 0 | no detected regression |
| BUG-003-001 duplicate-seed repair | Shares `test_topic_lifecycle.sh` and the `topics.name` uniqueness path | Targeted E2E exited 0 with the idempotent fixture code unchanged | no detected regression |
| Specs 002, 007, 009, 095, and 107 | Reference the topic lifecycle implementation | `internal/topics/lifecycle.go` is unchanged from the tested closure | no new post-closure impact |
| Independent builder-security change | Changes Docker builder and Go dependency inputs | Builder canary and image-rebuilding targeted E2E exited 0 | impact exercised, no detected regression |

### Coverage Regression Check

No bug source, test, assertion, or scenario-manifest file changed after the
tested closure. The three mapped scenario rows were rerun at current HEAD. No
numeric coverage result is available from the registered command surface, so
this review makes no percentage claim. The known missing E2E recalculation
trigger remains R-020 and is not misclassified as newly covered.

### Deployment Regression Applicability

The post-closure diff contains no adapter, build workflow, deployment manifest,
config SST, promotion script, rollback script, or image-pinning change. The
deployment regression scan does not apply. The changed Docker builder still
received focused coverage through the builder contract canary and a targeted
E2E run that rebuilt disposable stack images.

### Regression Phase Packet Guards

All five requested guards ran after the regression evidence was appended. Home
paths in the traceability and regression outputs are rendered as `~/smackerel`.

#### Artifact Lint

**Phase:** regression
**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
# BUG-003-002 regression artifact lint pre-state
$ bash .github/bubbles/scripts/artifact-lint.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count
exit: 0
lines: 41
sha256: d0fcc3d00860e2793d6f53a8434292f1a505ecf38ce4456d8d8db5bf25097ada
--- first 20 ---
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Found Checklist section in uservalidation.md
✅ uservalidation checklist contains checkbox entries
✅ All checklist bullet items use checkbox syntax
✅ uservalidation separates automation readiness from human acceptance
✅ Detected state.json status: in_progress
✅ Detected state.json workflowMode: bugfix-fastlane
✅ state.json v3 has required field: status
✅ state.json v3 has required field: execution
✅ state.json v3 has required field: certification
✅ state.json v3 has required field: policySnapshot
--- omitted 1 line(s); sha256 above covers the full output ---
--- last 20 ---
✅ state.json v3 has recommended field: reworkQueue
✅ state.json v3 has recommended field: executionHistory
✅ Top-level status matches certification.status
ℹ️  Workflow mode 'bugfix-fastlane' allows status 'done'; current status is 'in_progress'
✅ report.md contains section matching: ###[[:space:]]+Summary|^##[[:space:]]+Summary
✅ report.md contains section matching: ###[[:space:]]+Completion Statement|^##[[:space:]]+Completion Statement
✅ report.md contains section matching: ###[[:space:]]+Test Evidence|^##[[:space:]]+Test Evidence
✅ Mode-specific report gates skipped (status not in promotion set)
✅ Value-first selection rationale lint skipped (not a value-first report)
✅ Scenario path-placeholder lint skipped (no matching scenario sections found)

=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

**Result:** PASS.

#### Traceability Guard

**Phase:** regression
**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/traceability-guard.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count`
**Exit Code:** 0
**Claim Source:** executed
**Path normalization:** The repository root is rendered as `~/smackerel`.
**Output:**

```text
# BUG-003-002 regression traceability guard pre-state
$ bash .github/bubbles/scripts/traceability-guard.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count
exit: 0
lines: 48
sha256: 67ef63071196fe689f17ad2d1306b28b66898fb4f2afe048d41c37d816e7fc7e
--- first 20 ---
============================================================
	BUBBLES TRACEABILITY GUARD
	Feature: ~/smackerel/specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count
	Timestamp: 2026-08-29T21:15:25Z
============================================================

--- Scenario Manifest Cross-Check (G057/G059) ---
✅ scenario-manifest.json covers 3 scenario contract(s)
✅ scenario-manifest.json linked test exists: tests/integration/topic_lifecycle_momentum_test.go
✅ scenario-manifest.json linked test exists: internal/scheduler/jobs_test.go
✅ scenario-manifest.json linked test exists: tests/e2e/test_topic_lifecycle.sh
✅ scenario-manifest.json records evidenceRefs for all 3 scenario contract(s)
✅ All linked tests from scenario-manifest.json exist

ℹ️  Checking traceability for Scope 1: Restore canonical topic star aggregation
✅ Scope 1: Restore canonical topic star aggregation scenario mapped to Test Plan row: BUG-003-002-SCN-001 Canonical star aggregation updates momentum
ℹ️  Scope 1: Restore canonical topic star aggregation scenario→row match confidence: declared
✅ Scope 1: Restore canonical topic star aggregation scenario maps to concrete test file: tests/integration/topic_lifecycle_momentum_test.go
✅ Scope 1: Restore canonical topic star aggregation report references concrete test evidence: tests/integration/topic_lifecycle_momentum_test.go
✅ Scope 1: Restore canonical topic star aggregation scenario mapped to Test Plan row: BUG-003-002-SCN-002 Lifecycle query failures remain observable
--- omitted 8 line(s); sha256 above covers the full output ---
--- last 20 ---

--- Gherkin → DoD Content Fidelity (Gate G068) ---
✅ Scope 1: Restore canonical topic star aggregation scenario maps to DoD item: BUG-003-002-SCN-001 Canonical star aggregation updates momentum
ℹ️  Scope 1: Restore canonical topic star aggregation scenario→DoD match confidence: declared
✅ Scope 1: Restore canonical topic star aggregation scenario maps to DoD item: BUG-003-002-SCN-002 Lifecycle query failures remain observable
ℹ️  Scope 1: Restore canonical topic star aggregation scenario→DoD match confidence: declared
✅ Scope 1: Restore canonical topic star aggregation scenario maps to DoD item: BUG-003-002-SCN-003 Existing topic lifecycle surface remains available
ℹ️  Scope 1: Restore canonical topic star aggregation scenario→DoD match confidence: declared
ℹ️  DoD fidelity: 3 scenarios checked, 3 mapped to DoD, 0 unmapped

--- Traceability Summary ---
ℹ️  Scenarios checked: 3
ℹ️  Test rows checked: 6
ℹ️  Scenario-to-row mappings: 3
ℹ️  Concrete test file references: 3
ℹ️  Report evidence references: 3
ℹ️  DoD fidelity scenarios: 3 (mapped: 3, unmapped: 0)
ℹ️  Edge confidence (IMP-015 Scope B): declared=6 inferred=0 ambiguous=0

RESULT: PASSED (0 warnings)
```

**Result:** PASS.

#### Implementation Reality Scan

**Phase:** regression
**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/implementation-reality-scan.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count --verbose`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
# BUG-003-002 regression reality guard pre-state
$ bash .github/bubbles/scripts/implementation-reality-scan.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count --verbose
exit: 0
lines: 35
sha256: 7d7f9c395d2617b29bfde6d813eb7580efbcb7f29931f02d743f19512addf488
--- output ---
ℹ️  INFO: Resolved 1 implementation file(s) to scan

--- Scan 1: Gateway/Backend Stub Patterns ---

--- Scan 1B: Handler / Endpoint Execution Depth ---

--- Scan 1C: Endpoint Not-Implemented / Placeholder Responses ---

--- Scan 1D: External Integration Authenticity ---

--- Scan 2: Frontend Hardcoded Data Patterns ---

--- Scan 2B: Sensitive Client Storage ---

--- Scan 3: Frontend API Call Absence ---

--- Scan 4: Prohibited Simulation Helpers in Production ---

--- Scan 5: Default/Fallback Value Patterns ---

--- Scan 6: Live-System Test Interception ---

--- Scan 7: IDOR / Auth Bypass Detection (Gate G047) ---

--- Scan 8: Silent Decode Failure Detection (Gate G048) ---

============================================================
	IMPLEMENTATION REALITY SCAN RESULT
============================================================

	Files scanned:  1
	Violations:     0
	Warnings:       0

🟢 PASSED: No source code reality violations detected
```

**Result:** PASS.

#### Standard Regression Quality Guard

**Phase:** regression
**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/regression-quality-guard.sh tests/integration/topic_lifecycle_momentum_test.go internal/scheduler/jobs_test.go tests/e2e/test_topic_lifecycle.sh`
**Exit Code:** 0
**Claim Source:** executed
**Path normalization:** The repository root is rendered as `~/smackerel`.
**Output:**

```text
# BUG-003-002 regression quality guard pre-state
$ bash .github/bubbles/scripts/regression-quality-guard.sh tests/integration/topic_lifecycle_momentum_test.go internal/scheduler/jobs_test.go tests/e2e/test_topic_lifecycle.sh
exit: 0
lines: 15
sha256: 39a24635d6b95af15ebb0ddba208b2e250c8bf819664635080a58a5bc2e3c254
--- output ---
============================================================
	BUBBLES REGRESSION QUALITY GUARD
	Repo: ~/smackerel
	Timestamp: 2026-08-29T21:15:35Z
	Bugfix mode: false
============================================================

ℹ️  Scanning tests/integration/topic_lifecycle_momentum_test.go
ℹ️  Scanning internal/scheduler/jobs_test.go
ℹ️  Scanning tests/e2e/test_topic_lifecycle.sh

============================================================
	REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
	Files scanned: 3
============================================================
```

**Result:** PASS.

#### Adversarial Bugfix Regression Guard

**Phase:** regression
**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/integration/topic_lifecycle_momentum_test.go internal/scheduler/jobs_test.go tests/e2e/test_topic_lifecycle.sh`
**Exit Code:** 0
**Claim Source:** executed
**Path normalization:** The repository root is rendered as `~/smackerel`.
**Output:**

```text
# BUG-003-002 adversarial regression guard pre-state
$ bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/integration/topic_lifecycle_momentum_test.go internal/scheduler/jobs_test.go tests/e2e/test_topic_lifecycle.sh
exit: 0
lines: 19
sha256: 56558894c7d0e0e85e9ce21e8207fe8ab5b7e241f6aea303136b8fdefbf3b95f
--- output ---
============================================================
	BUBBLES REGRESSION QUALITY GUARD
	Repo: ~/smackerel
	Timestamp: 2026-08-29T21:15:40Z
	Bugfix mode: true
============================================================

ℹ️  Scanning tests/integration/topic_lifecycle_momentum_test.go
✅ Adversarial signal detected in tests/integration/topic_lifecycle_momentum_test.go
ℹ️  Scanning internal/scheduler/jobs_test.go
✅ Adversarial signal detected in internal/scheduler/jobs_test.go
ℹ️  Scanning tests/e2e/test_topic_lifecycle.sh
✅ Adversarial signal detected in tests/e2e/test_topic_lifecycle.sh

============================================================
	REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
	Files scanned: 3
	Files with adversarial signals: 3
============================================================
```

**Result:** PASS.

## Simplify Phase Review - 2026-08-29

### Simplify Owner Statement

`bubbles.simplify` reviewed the exact BUG-003-002 production delta and its
focused scheduler and PostgreSQL regressions at starting revision
`cd71d621cfd952c520805a4bf3cbec7f85291c06`. The review applied separate code
reuse, code quality, and efficiency passes.

No source or test simplification is warranted. The production change is one
correlated aggregate that directly expresses the persisted star source and all
relationship predicates. The starred predicate has no duplicate product
implementation to extract. The focused integration test already separates
schema inspection, fixture creation, uniqueness, persisted reads, assertions,
and cleanup through named helpers. The destination-side edge index supports the
correlation key, while the relationship uniqueness constraint and explicit
`COUNT(DISTINCT a.id)` preserve the distinct-artifact contract without another
abstraction.

No product source or test file changed. The existing R-020 boundary remains
unchanged and was not expanded into this packet.

### Consolidated Review

| Pass | Finding count | Decision |
|---|---:|---|
| Code reuse | 0 | Keep the single product aggregate and existing shared integration helpers. |
| Code quality | 0 | Keep the direct SQL predicates and focused named test helpers. |
| Efficiency | 0 | Keep the indexed correlated aggregate; no measured bottleneck justifies a wider query rewrite. |

The product and focused-test line counts remain unchanged:

- `internal/topics/lifecycle.go`: 170 to 170
- `internal/scheduler/jobs_test.go`: 583 to 583
- `tests/integration/topic_lifecycle_momentum_test.go`: 206 to 206
- Product-source net change in this phase: 0 lines added, 0 lines removed

### Current-Revision Simplification Evidence

**Phase:** simplify
**Executed:** YES (current session)
**Command:** `printf '%s\n' 'SIMPLIFY-REVIEW-V1' 'target=BUG-003-002' 'phase=simplify'; timeout 30 git rev-parse HEAD; printf '%s\n' 'worktree-status-begin'; timeout 30 git status --short; printf '%s\n' 'worktree-status-end' 'original-production-delta:'; timeout 60 git --no-pager diff --unified=0 f5f05450848630fe84c0a215429bdfc701c4bcd2 7ff2d5441f8d90158873cff378c8b81d448900b8 -- internal/topics/lifecycle.go; printf '%s\n' 'starred-predicate-product-occurrence:'; timeout 30 git grep -n "user_starred IS TRUE" -- internal; printf '%s\n' 'relationship-constraints-and-indexes:'; timeout 30 grep -nE 'UNIQUE\(src_type, src_id, dst_type, dst_id, edge_type\)|CREATE INDEX IF NOT EXISTS idx_edges_(src|dst|type)' internal/db/migrations/001_initial_schema.sql; printf '%s\n' 'focused-test-boundaries:'; timeout 30 grep -nE '^func (TestTopicLifecycleMomentumFromPersistedStars|assertTopicsStarCountColumnAbsent|seedTopicMomentumFixtures|assertDuplicateBelongsToRejected|readTopicMomentum|assertTopicMomentum|registerTopicMomentumCleanup)' tests/integration/topic_lifecycle_momentum_test.go; if timeout 30 git diff --quiet a6e927b2df7dfbe8e3e478be71a0c547fa3e67f1..HEAD -- internal/topics/lifecycle.go internal/topics/lifecycle_test.go internal/scheduler/jobs.go internal/scheduler/jobs_test.go tests/integration/topic_lifecycle_momentum_test.go tests/e2e/test_topic_lifecycle.sh; then printf '%s\n' 'scoped-closure=UNCHANGED-since-a6e927b2'; else printf '%s\n' 'scoped-closure=CHANGED-since-a6e927b2'; exit 1; fi; timeout 30 grep -n '"coverageNote"' specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/scenario-manifest.json; printf '%s\n' 'SIMPLIFY-REVIEW-COMPLETE'`
**Exit Code:** 0
**Claim Source:** interpreted
**Interpretation:** The output identifies the complete production delta, the
single product occurrence of the starred predicate, the canonical uniqueness
and index contracts, the focused helper boundaries, unchanged product and test
inputs since the tested closure, and the preserved R-020 marker. These facts
support the no-source-change simplify decision. They do not claim a new runtime
test execution.
**Output:**

```text
SIMPLIFY-REVIEW-V1
target=BUG-003-002
phase=simplify
cd71d621cfd952c520805a4bf3cbec7f85291c06
worktree-status-begin
worktree-status-end
original-production-delta:
diff --git a/internal/topics/lifecycle.go b/internal/topics/lifecycle.go
index 11bb4df6..fe5ded44 100644
--- a/internal/topics/lifecycle.go
+++ b/internal/topics/lifecycle.go
@@ -123 +123,5 @@ func (l *Lifecycle) UpdateAllMomentum(ctx context.Context) error {
-                      COALESCE(t.star_count, 0),
+                      COALESCE((SELECT COUNT(DISTINCT a.id)
+                                FROM edges e
+                                JOIN artifacts a ON a.id = e.src_id AND e.src_type = 'artifact'
+                                WHERE e.dst_type = 'topic' AND e.dst_id = t.id
+                                  AND e.edge_type = 'BELONGS_TO' AND a.user_starred IS TRUE), 0)::int,
starred-predicate-product-occurrence:
internal/topics/lifecycle.go:127:                                  AND e.edge_type = 'BELONGS_TO' AND a.user_starred IS TRUE), 0)::int,
relationship-constraints-and-indexes:
132:    UNIQUE(src_type, src_id, dst_type, dst_id, edge_type)
135:CREATE INDEX IF NOT EXISTS idx_edges_src ON edges(src_type, src_id);
136:CREATE INDEX IF NOT EXISTS idx_edges_dst ON edges(dst_type, dst_id);
137:CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(edge_type);
focused-test-boundaries:
25:func TestTopicLifecycleMomentumFromPersistedStars(t *testing.T) {
58:func assertTopicsStarCountColumnAbsent(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
79:func seedTopicMomentumFixtures(t *testing.T, ctx context.Context, pool *pgxpool.Pool, prefix string) {
140:func assertDuplicateBelongsToRejected(t *testing.T, ctx context.Context, pool *pgxpool.Pool, prefix string) {
163:func readTopicMomentum(t *testing.T, ctx context.Context, pool *pgxpool.Pool, topicID string) topicMomentumResult {
177:func assertTopicMomentum(t *testing.T, label string, result topicMomentumResult, expectedMomentum float64, expectedState string) {
188:func registerTopicMomentumCleanup(t *testing.T, pool *pgxpool.Pool, prefix string) {
scoped-closure=UNCHANGED-since-a6e927b2
80:      "coverageNote": "Narrowed 2026-08-29 from 'renders the lifecycle topics and momentum values'. The momentum half was not supported by the linked test: it seeds momentum_score as a literal and reads the same row back, so it cannot fail however badly recalculation breaks. Proven by injecting the pre-fix t.star_count fault and running the full e2e lane, which PASSED. The rendering half IS genuinely exercised via GET /topics against WebHandler.TopicsPage, so the scenario is anchored there. The uncovered recalculation path is tracked as R-020.",
SIMPLIFY-REVIEW-COMPLETE
```

**Result:** PASS. The bounded review found no behavior-preserving
simplification that improves the product delta enough to justify source or test
churn.

### Simplify Packet Artifact Lint

**Phase:** simplify
**Executed:** YES (current session)
**Command:** `timeout 300 bash .github/bubbles/scripts/evidence-capture.sh --label "BUG-003-002 simplify artifact lint final" -- bash .github/bubbles/scripts/artifact-lint.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
# BUG-003-002 simplify artifact lint final
$ bash .github/bubbles/scripts/artifact-lint.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count
exit: 0
lines: 41
sha256: d0fcc3d00860e2793d6f53a8434292f1a505ecf38ce4456d8d8db5bf25097ada
--- first 20 ---
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Found Checklist section in uservalidation.md
✅ uservalidation checklist contains checkbox entries
✅ All checklist bullet items use checkbox syntax
✅ uservalidation separates automation readiness from human acceptance
✅ Detected state.json status: in_progress
✅ Detected state.json workflowMode: bugfix-fastlane
✅ state.json v3 has required field: status
✅ state.json v3 has required field: execution
✅ state.json v3 has required field: certification
✅ state.json v3 has required field: policySnapshot
--- omitted 1 line(s); sha256 above covers the full output ---
--- last 20 ---
✅ state.json v3 has recommended field: reworkQueue
✅ state.json v3 has recommended field: executionHistory
✅ Top-level status matches certification.status
ℹ️  Workflow mode 'bugfix-fastlane' allows status 'done'; current status is 'in_progress'
✅ report.md contains section matching: ###[[:space:]]+Summary|^##[[:space:]]+Summary
✅ report.md contains section matching: ###[[:space:]]+Completion Statement|^##[[:space:]]+Completion Statement
✅ report.md contains section matching: ###[[:space:]]+Test Evidence|^##[[:space:]]+Test Evidence
✅ Mode-specific report gates skipped (status not in promotion set)
✅ Value-first selection rationale lint skipped (not a value-first report)
✅ Scenario path-placeholder lint skipped (no matching scenario sections found)

=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

**Result:** PASS.

### Simplify Phase Boundary

Only this report and simplify-owned `execution.*` provenance in `state.json`
change in this phase. Top-level status and certification remain `in_progress`.
No `certification.*` field, scenario receipt, source file, test file, or R-020
artifact changes. Routing advances to `bubbles.gaps`.

## Gaps Evidence - 2026-08-29

R-020 does not block this narrowed missing-column repair. SCN-001 exercises
`UpdateAllMomentum` against real disposable PostgreSQL state and proves the
recalculation contract. SCN-003 is explicitly limited to the independently
proved `GET /topics` rendering behavior. R-020 separately records the absent
test-reachable E2E recalculation trigger as an open item owned by
`bubbles.test`. It is a routed gap, not a hidden concern, and this packet makes
no E2E recalculation claim.

The bounded review covered every checked DoD entry present in `scopes.md` and
all three scenario-manifest contracts. This revision contains 15 checked DoD
entries, rather than the 16 stated in the request. Each of the 15 has an
evidence link, claim-source tag, and result marker. No additional packet gap
was found. Status remains `in_progress`, certification remains untouched, and
routing advances to `bubbles.harden`.

### Bounded DoD, Scenario, And Routing Audit

**Phase:** gaps
**Executed:** YES (current session)
**Command:** `P=specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count && printf '%s\n' 'GAPS-DISPOSITION-AUDIT' "head=$(git rev-parse HEAD)" && printf 'checked-dod=' && grep -c '^- \[x\]' "$P/scopes.md" && printf 'dod-evidence-links=' && grep -c '^  \*\*Evidence:\*\*' "$P/scopes.md" && printf 'dod-claim-sources=' && grep -c '^  \*\*Claim Source:\*\*' "$P/scopes.md" && printf 'dod-results=' && grep -c '^  \*\*Result:\*\*' "$P/scopes.md" && printf 'manifest-scenarios=' && grep -c '"id": "BUG-003-002-SCN-' "$P/scenario-manifest.json" && printf 'manifest-linked-tests=' && grep -c '"testId":' "$P/scenario-manifest.json" && grep -n '"requiredTestType": "integration"' "$P/scenario-manifest.json" && grep -n '"then": "the topics surface returns 200 and renders the seeded lifecycle topic by name"' "$P/scenario-manifest.json" && grep -n 'coverageNote.*R-020' "$P/scenario-manifest.json" && awk -F'|' '$2 ~ /R-020/ { gsub(/^[[:space:]]+|[[:space:]]+$/, "", $6); gsub(/^[[:space:]]+|[[:space:]]+$/, "", $7); print "r020-status=" $6; print "r020-owner=" $7 }' .specify/memory/open-work.md && printf '%s\n' 'GAPS-DISPOSITION-AUDIT-COMPLETE'`
**Exit Code:** 0
**Claim Source:** interpreted
**Interpretation:** The equal DoD marker counts cover every checked entry. The
manifest maps SCN-001 to integration coverage, narrows SCN-003 to rendering,
and names R-020. The open-work row independently keeps R-020 open and routes it
to `bubbles.test`.
**Output:**

```text
GAPS-DISPOSITION-AUDIT
head=1a4e8c687bed80d7b55f34061c060439e000a178
checked-dod=15
dod-evidence-links=15
dod-claim-sources=15
dod-results=15
manifest-scenarios=3
manifest-linked-tests=3
19:      "requiredTestType": "integration",
72:        "then": "the topics surface returns 200 and renders the seeded lifecycle topic by name"
80:      "coverageNote": "Narrowed 2026-08-29 from 'renders the lifecycle topics and momentum values'. The momentum half was not supported by the linked test: it seeds momentum_score as a literal and reads the same row back, so it cannot fail however badly recalculation breaks. Proven by injecting the pre-fix t.star_count fault and running the full e2e lane, which PASSED. The rendering half IS genuinely exercised via GET /topics against WebHandler.TopicsPage, so the scenario is anchored there. The uncovered recalculation path is tracked as R-020.",
r020-status=open
r020-owner=bubbles.test
GAPS-DISPOSITION-AUDIT-COMPLETE
```

**Result:** PASS. R-020 retains an explicit owner and does not invalidate this
bug packet's integration-proven recalculation repair.

## Hardening Evidence - 2026-08-29

The packet-only review started at revision `41d235f5`. It inspected every one
of the 15 checked DoD entries and opened each linked evidence section. Each
entry has phase, execution, exit-code, claim-source, evidence, and result
markers. The three interpreted entries retain explicit interpretations and do
not claim end-to-end recalculation coverage.

The three scenario-manifest contracts match Test Plan rows 01, 03, and 04 by
scenario ID, category, and test path. Rows 02 and 05 provide supporting
functional and broader unit coverage. The predecessor phase claims are ordered
through `gaps`, and `stabilize` follows `harden` in `bugfix-fastlane`.

R-020 remains open under `bubbles.test`. It is the packet's only known
hardening limitation, and this review makes no end-to-end recalculation claim.
No tests were rerun. No source, test, framework, scope, manifest, or
certification artifact changed during this review.

### Read-Only Packet Audit

**Phase:** harden
**Executed:** YES (current session)
**Command:** `cd ~/smackerel && P=specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count && printf '%s\n' 'HARDEN-EVIDENCE' && awk '/^- \[x\]/ { n++; item_line=NR; phase="missing"; claim="missing"; evidence="no"; result="no"; active=1; next } active && /^  \*\*Phase:\*\*/ { phase=$0; sub(/^  \*\*Phase:\*\*[[:space:]]*/, "", phase); next } active && /^  \*\*Claim Source:\*\*/ { claim=$0; sub(/^  \*\*Claim Source:\*\*[[:space:]]*/, "", claim); next } active && /^  \*\*Evidence:\*\*/ { evidence="yes"; next } active && /^  \*\*Result:\*\*/ { result="yes"; printf "dod=%02d line=%d phase=%s claim=%s evidence=%s result=%s\n", n, item_line, phase, claim, evidence, result; active=0 } END { printf "dod-total=%d\n", n }' "$P/scopes.md" && printf 'test-plan-rows=%s\n' "$(grep -c '^| T-BUG-003-002-' "$P/scopes.md")" && jq -r '.scenarios[] | "manifest=" + .id + " type=" + .requiredTestType + " category=" + .linkedTests[0].category + " test=" + .linkedTests[0].file' "$P/scenario-manifest.json" && jq -r '"phase-chain=" + ([.execution.completedPhaseClaims[].phase] | join(","))' "$P/state.json" && jq -r '"status=" + .status + " certification=" + .certification.status + " nextOwner=" + .routing.nextOwner' "$P/state.json" && awk -F'|' '$2 ~ /R-020/ { gsub(/^[[:space:]]+|[[:space:]]+$/, "", $6); gsub(/^[[:space:]]+|[[:space:]]+$/, "", $7); print "r020-status=" $6 " r020-owner=" $7 }' .specify/memory/open-work.md && printf '%s\n' 'HARDEN-EVIDENCE-COMPLETE'`
**Exit Code:** 0
**Claim Source:** interpreted
**Interpretation:** The output enumerates all checked DoD records, exposes the
three conservative claim-source labels, identifies all manifest mappings, and
shows the predecessor phase chain plus the separately routed R-020 limitation.
**Output:**

```text
HARDEN-EVIDENCE
dod=01 line=79 phase=implement claim=executed evidence=yes result=yes
dod=02 line=106 phase=test claim=executed evidence=yes result=yes
dod=03 line=134 phase=test claim=executed evidence=yes result=yes
dod=04 line=161 phase=test claim=executed evidence=yes result=yes
dod=05 line=191 phase=test claim=executed evidence=yes result=yes
dod=06 line=201 phase=implement claim=executed evidence=yes result=yes
dod=07 line=228 phase=implement claim=interpreted evidence=yes result=yes
dod=08 line=256 phase=implement claim=executed evidence=yes result=yes
dod=09 line=286 phase=test claim=interpreted evidence=yes result=yes
dod=10 line=297 phase=test claim=interpreted evidence=yes result=yes
dod=11 line=308 phase=test claim=executed evidence=yes result=yes
dod=12 line=341 phase=test claim=executed evidence=yes result=yes
dod=13 line=369 phase=test claim=executed evidence=yes result=yes
dod=14 line=398 phase=implement claim=executed evidence=yes result=yes
dod=15 line=426 phase=test claim=executed evidence=yes result=yes
dod-total=15
test-plan-rows=5
manifest=BUG-003-002-SCN-001 type=integration category=integration test=tests/integration/topic_lifecycle_momentum_test.go
manifest=BUG-003-002-SCN-002 type=unit category=unit test=internal/scheduler/jobs_test.go
manifest=BUG-003-002-SCN-003 type=e2e-api category=e2e-api test=tests/e2e/test_topic_lifecycle.sh
phase-chain=bug,implement,test,regression,simplify,gaps
status=in_progress certification=in_progress nextOwner=bubbles.harden
r020-status=open r020-owner=bubbles.test
HARDEN-EVIDENCE-COMPLETE
```

**Result:** PASS. No packet blocker was found beyond separately routed R-020.

## Stabilize Evidence - 2026-08-29

### Stability Verdict And Scope

The stabilize phase started from revision
`522f7e6651339e27db82111506c30321c3f30c57`. No in-scope operational defect
was observed in the canonical star-aggregation repair. This phase makes no
product source, test, configuration, build, deployment, or framework change.

The repair preserves one outer row per topic. Its scalar star subquery counts
distinct artifact IDs, joins artifacts by primary key, and constrains the edge
lookup by the indexed destination pair. The canonical relationship uniqueness
constraint prevents duplicate `artifact -> topic` `BELONGS_TO` edges. The
existing per-topic update loop is unchanged by the repair.

The scheduler retains an hourly overlap guard and a two-minute cancellation
budget. Initial query failures retain `query topics` context and reach the
failure log branch without emitting the success log. The focused scheduler
regression completed successfully at the starting revision.

The disposable PostgreSQL regression and targeted full-stack shell flow both
completed successfully. The full-stack flow proves only its existing
`GET /topics` rendering contract. It seeds and reads literal momentum values;
it does not invoke `UpdateAllMomentum`. R-020 therefore remains open under
`bubbles.test`, and this phase makes no end-to-end recalculation claim.

No production-cardinality sample or latency SLO is declared for this packet.
This phase therefore makes no arbitrary-scale latency claim. The no-source-change
decision rests on the observed indexed query shape, bounded scheduler execution,
focused PostgreSQL and scheduler regressions, and the accurately narrowed
full-stack contract.

### Stability Inventory

| Domain | Observed impact | Disposition |
|---|---|---|
| Query shape and cardinality | The scalar aggregate cannot multiply outer topic rows. `COUNT(DISTINCT a.id)` and relationship uniqueness preserve one contribution per linked starred artifact. | No repair required. |
| Database performance | The correlation key matches `idx_edges_dst(dst_type, dst_id)` and the artifact join uses the artifact primary key. The repair adds indexed edge and artifact work per topic rather than a denormalized-column read. | No measured bottleneck. No production-scale bound claimed. |
| Database reliability | The real migration-backed PostgreSQL regression completed and the disposable stores were removed. The configured pool permits ten connections with two minimum connections. | No repair required. |
| Scheduler reliability | `runGuarded` prevents overlap, the job context expires after two minutes, and the query-error regression preserves failure-versus-success log separation. | No repair required. |
| Infrastructure and resource use | The targeted test stack rebuilt, completed, and removed its containers, network, and volumes. | No residual test resource found. |
| Configuration and build | The repair introduces no config key, generated file, build input, or deployment surface. | No change required. |
| Targeted full-stack behavior | The shell E2E completed against the disposable stack and exercised the topics page. | Rendering regression is green; R-020 remains explicit. |

### Query, Index, Scheduler, And E2E Boundary Inspection

**Phase:** stabilize
**Executed:** YES (current session)
**Command:** `printf '%s\n' 'STABILIZE STATIC REVIEW' "head=$(timeout 30 git rev-parse HEAD)" 'query-shape:' && timeout 30 grep -nE "COUNT\(DISTINCT a.id\)|JOIN artifacts a|e.dst_type = 'topic'|e.dst_id = t.id|e.edge_type = 'BELONGS_TO'|a.user_starred IS TRUE|COUNT\(\*\) FROM edges|FROM topics t|WHERE id = \$1 AND" internal/topics/lifecycle.go && printf '%s\n' 'schema-cardinality:' && timeout 30 grep -nE 'UNIQUE\(src_type, src_id, dst_type, dst_id, edge_type\)|idx_edges_(src|dst|type)' internal/db/migrations/001_initial_schema.sql && printf '%s\n' 'pool-and-scheduler-bounds:' && timeout 30 grep -nE 'max_conns:|min_conns:' config/smackerel.yaml && timeout 30 grep -nE 'TryLock|skipping overlapping job' internal/scheduler/lifecycle.go && timeout 30 grep -nE 'context.WithTimeout\(s.baseCtx, 2\*time.Minute\)|UpdateAllMomentum|topic momentum update failed|topic momentum updated' internal/scheduler/jobs.go && printf '%s\n' 'targeted-e2e-boundary:' && timeout 30 grep -nE 'momentum_score.*capture_count|topic-lifecycle-hot|HOT_MOMENTUM=|DORMANT_MOMENTUM=|CORE_URL/topics|Topics page shows owned' tests/e2e/test_topic_lifecycle.sh && printf '%s\n' 'STABILIZE STATIC REVIEW COMPLETE'`
**Exit Code:** 0
**Claim Source:** interpreted
**Interpretation:** The output shows the scalar aggregate predicates, canonical
relationship uniqueness, eligible edge indexes, explicit pool size, overlap and
timeout bounds, scheduler log branches, and the literal-seed/read E2E boundary.
It does not claim an `EXPLAIN` plan or a production-scale latency measurement.
**Output:**

```text
STABILIZE STATIC REVIEW
head=522f7e6651339e27db82111506c30321c3f30c57
query-shape:
123:                   COALESCE((SELECT COUNT(DISTINCT a.id)
125:                             JOIN artifacts a ON a.id = e.src_id AND e.src_type = 'artifact'
126:                             WHERE e.dst_type = 'topic' AND e.dst_id = t.id
127:                               AND e.edge_type = 'BELONGS_TO' AND a.user_starred IS TRUE), 0)::int,
128:                   COALESCE((SELECT COUNT(*) FROM edges WHERE src_type = 'topic' AND src_id = t.id
131:            FROM topics t
schema-cardinality:
132:    UNIQUE(src_type, src_id, dst_type, dst_id, edge_type)
135:CREATE INDEX IF NOT EXISTS idx_edges_src ON edges(src_type, src_id);
136:CREATE INDEX IF NOT EXISTS idx_edges_dst ON edges(dst_type, dst_id);
137:CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(edge_type);
pool-and-scheduler-bounds:
2385:    max_conns: 10
2386:    min_conns: 2
35:// runGuarded runs fn under a TryLock guard. If the mutex is already held
40:     if !mu.TryLock() {
41:             slog.Warn("skipping overlapping job", "group", group, "job", job)
71:     ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
203:    ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
205:    if err := s.lifecycle.UpdateAllMomentum(ctx); err != nil {
206:            slog.Error("topic momentum update failed", "error", err)
208:            slog.Info("topic momentum updated")
257:    ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
382:    ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
399:    ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
485:            ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
targeted-e2e-boundary:
22:INSERT INTO topics (id, name, state, momentum_score, capture_count_total, capture_count_30d, capture_count_90d, search_hit_count_30d, last_active)
24:  ('topic-lifecycle-hot', 'topic-lifecycle-pricing', 'hot', 20.0, 15, 10, 15, 8, NOW()),
39:HOT_STATE=$(e2e_psql "SELECT state FROM topics WHERE id='topic-lifecycle-hot'")
46:HOT_MOMENTUM=$(e2e_psql "SELECT momentum_score FROM topics WHERE id='topic-lifecycle-hot'")
48:DORMANT_MOMENTUM=$(e2e_psql "SELECT momentum_score FROM topics WHERE id='topic-lifecycle-dormant'")
53:  -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/topics")
57:  -H "Authorization: Bearer $AUTH_TOKEN" "$CORE_URL/topics" 2>/dev/null || true)
58:e2e_assert_contains "$BODY" "topic-lifecycle-pricing" "Topics page shows owned lifecycle pricing topic"
STABILIZE STATIC REVIEW COMPLETE
```

**Result:** PASS for the bounded source and schema assessment.

### Current-HEAD PostgreSQL Aggregation Regression

**Phase:** stabilize
**Executed:** YES (current session)
**Command:** `env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration-light --go-run '^TestTopicLifecycleMomentumFromPersistedStars$'`
**Exit Code:** 0
**Claim Source:** interpreted
**Interpretation:** The repository runner completed its migration-backed
integration-light lane with the exact focused selector. The compact evidence
retains the full-output hash and final lane result. The focused-selector notice
means this run is not evidence for the unrelated acceptance-gate assertion.
**Path normalization:** The repository root is rendered as `~/smackerel`.
**Output:**

```text
# BUG-003-002 stabilize PostgreSQL aggregation
$ env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration-light --go-run ^TestTopicLifecycleMomentumFromPersistedStars$
exit: 0
lines: 309
sha256: e868cf18c11ae977b121ad5773402c4ba7844bba6d7c752cc6a5bbd5bdc19f03
--- first 20 ---
oom-preflight: OK — 28302 MB available (need 6000 MB; swap used 1855 MB).
disk-preflight: OVERRIDE set — skipping disk gate.
config-validate: ~/smackerel/config/generated/test.env.tmp.1688792 OK
Smackerel pre-flight resource check: OK
	RAM  available: 28316 MB (required >= 2000 MB)
	Disk available: 489550 MB / 478.1 GB (required >= 8 GB)
 Network smackerel-test_default  Creating
 Network smackerel-test_default  Created
 Volume "smackerel-test-postgres-data"  Creating
 Volume "smackerel-test-postgres-data"  Created
 Volume "smackerel-test-nats-data"  Creating
 Volume "smackerel-test-nats-data"  Created
 Container smackerel-test-postgres-1  Creating
 Container smackerel-test-nats-1  Creating
 Container smackerel-test-nats-1  Created
 Container smackerel-test-postgres-1  Created
 Container smackerel-test-nats-1  Starting
 Container smackerel-test-postgres-1  Starting
 Container smackerel-test-nats-1  Started
 Container smackerel-test-postgres-1  Started
--- omitted 269 line(s); sha256 above covers the full output ---
--- last 20 ---
PASS
ok      github.com/smackerel/smackerel/tests/eval/assistant     0.004s [no tests to run]
go-integration: NOTICE: acceptance-gate executed-assertion assertion NOT ENFORCED for this run — a focused --run selector (^TestTopicLifecycleMomentumFromPersistedStars$) is active. Only a full lane run with no --run selector enforces that TestAcceptanceGate_RoutingAccuracyAndCaptureFallback ran with a non-zero executed-assertion count.
PASS: go-integration-light
Running project-scoped integration-light stack teardown (exit cleanup, timeout 120s)...
config-validate: ~/smackerel/config/generated/test.env.tmp.1722077 OK
 Container smackerel-test-postgres-1  Stopping
 Container smackerel-test-nats-1  Stopping
 Container smackerel-test-nats-1  Stopped
 Container smackerel-test-nats-1  Removing
 Container smackerel-test-nats-1  Removed
 Container smackerel-test-postgres-1  Stopped
 Container smackerel-test-postgres-1  Removing
 Container smackerel-test-postgres-1  Removed
 Volume smackerel-test-postgres-data  Removing
 Volume smackerel-test-nats-data  Removing
 Network smackerel-test_default  Removing
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-nats-data  Removed
 Network smackerel-test_default  Removed
```

**Result:** PASS for the focused migration-backed PostgreSQL lane. No broader
acceptance-gate or production-scale performance claim is made.

### Current-HEAD Scheduler Failure Regression

**Phase:** stabilize
**Executed:** YES (current session)
**Command:** `env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run '^TestTopicMomentumJob_LogsLifecycleQueryFailure$' --verbose`
**Exit Code:** 0
**Claim Source:** interpreted
**Interpretation:** The full-output hash covers the selected scheduler test. Its
inspected assertions require the failure message and `query topics` context and
reject the success message. The compact block confirms the focused Go lane
completed; unrelated packages legitimately report no matching tests.
**Output:**

```text
# BUG-003-002 stabilize scheduler failure
$ env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run ^TestTopicMomentumJob_LogsLifecycleQueryFailure$ --verbose
exit: 0
lines: 523
sha256: ffb3e85ed947eea7016b5d3e1889f74cc1b3af09a9f479a1ae96756e65c53c7d
--- first 20 ---
oom-preflight: OK — 28261 MB available (need 6000 MB; swap used 1855 MB).
disk-preflight: OVERRIDE set — skipping disk gate.
++ dirname /workspace/scripts/runtime/go-unit.sh
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
+ echo '[go-unit] envsubst missing — installing gettext-base'
+ apt-get update -qq
[go-unit] envsubst missing — installing gettext-base
+ apt-get install -y --no-install-recommends gettext-base
Reading package lists...
Building dependency tree...
Reading state information...
The following NEW packages will be installed:
	gettext-base
0 upgraded, 1 newly installed, 0 to remove and 20 not upgraded.
Need to get 160 kB of archives.
After this operation, 660 kB of additional disk space will be used.
Get:1 http://deb.debian.org/debian bookworm/main amd64 gettext-base amd64 0.21-12 [160 kB]
--- omitted 483 line(s); sha256 above covers the full output ---
--- last 20 ---
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.006s [no tests to run]
?       github.com/smackerel/smackerel/tests/integration/agent/routerwarmup    [no test files]
?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
?       github.com/smackerel/smackerel/tests/integration/nslock [no test files]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/observability      0.003s [no tests to run]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/stress/readiness   0.006s [no tests to run]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/tests/unit/clients       0.003s [no tests to run]
?       github.com/smackerel/smackerel/web/pwa  [no test files]
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/web/pwa/tests    0.110s [no tests to run]
[go-unit] go test ./... finished OK
++ echo '[go-unit] go test ./... finished OK'
```

**Result:** PASS for the focused scheduler failure lane.

### Current-HEAD Targeted Full-Stack Regression

**Phase:** stabilize
**Executed:** YES (current session)
**Command:** `env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --shell-run test_topic_lifecycle.sh`
**Exit Code:** 0
**Claim Source:** interpreted
**Interpretation:** The targeted shell lane rebuilt and exercised the disposable
stack and exited zero. Source inspection above limits the claim to the topics
page assertion. The literal momentum seed/read does not cover recalculation and
does not close R-020.
**Path normalization:** The repository root is rendered as `~/smackerel`.
**Output:**

```text
# BUG-003-002 stabilize targeted full-stack
$ env DISK_PREFLIGHT_OVERRIDE=1 SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --shell-run test_topic_lifecycle.sh
exit: 0
lines: 396
sha256: 182c7e2e5a4f372399fe28bf11edffa5eaad0f2692df06680035045573ae8ce4
--- first 20 ---
oom-preflight: OK — 28277 MB available (need 6000 MB; swap used 1855 MB).
disk-preflight: OVERRIDE set — skipping disk gate.
config-validate: ~/smackerel/config/generated/test.env.tmp.1761762 OK
Smackerel pre-flight resource check: OK
	RAM  available: 28223 MB (required >= 6000 MB)
	Disk available: 489308 MB / 477.8 GB (required >= 15 GB)
Running targeted shell E2E: test_topic_lifecycle.sh
Running project-scoped test stack teardown (before targeted shared-stack shell E2E, timeout 180s)...
config-validate: ~/smackerel/config/generated/test.env.tmp.1770819 OK
oom-preflight: OK — 28341 MB available (need 6000 MB; swap used 1855 MB).
disk-preflight: OVERRIDE set — skipping disk gate.
config-validate: ~/smackerel/config/generated/test.env.tmp.1775856 OK
Smackerel pre-flight resource check: OK
	RAM  available: 27968 MB (required >= 6000 MB)
	Disk available: 489267 MB / 477.8 GB (required >= 15 GB)
Preparing disposable test stack...
Building disposable test stack images before up (freshness convention)...
Compose can now delegate builds to bake for better performance.
 To do so, set COMPOSE_BAKE=true.
#0 building with "default" instance using docker driver
--- omitted 356 line(s); sha256 above covers the full output ---
--- last 20 ---
 Container smackerel-test-postgres-1  Removing
 Container smackerel-test-postgres-1  Removed
 Container smackerel-test-intent-compiler-provider-1  Stopped
 Container smackerel-test-intent-compiler-provider-1  Removing
 Container smackerel-test-intent-compiler-provider-1  Removed
 Container smackerel-test-smackerel-ml-1  Stopped
 Container smackerel-test-smackerel-ml-1  Removing
 Container smackerel-test-smackerel-ml-1  Removed
 Container smackerel-test-nats-1  Stopping
 Container smackerel-test-nats-1  Stopped
 Container smackerel-test-nats-1  Removing
 Container smackerel-test-nats-1  Removed
 Volume smackerel-test-ollama-data  Removing
 Volume smackerel-test-nats-data  Removing
 Volume smackerel-test-postgres-data  Removing
 Network smackerel-test_default  Removing
 Volume smackerel-test-ollama-data  Removed
 Volume smackerel-test-nats-data  Removed
 Volume smackerel-test-postgres-data  Removed
 Network smackerel-test_default  Removed
```

**Result:** PASS for the narrowed targeted full-stack regression. R-020 remains
open and is not represented as covered.

### Post-Run Cleanup And Change Boundary

**Phase:** stabilize
**Executed:** YES (current session)
**Command:** `set -e && printf '%s\n' 'STABILIZE CLEANUP CONFIRMATION' 'tracked-delta:' && timeout 30 git status --short && printf '%s\n' 'test-containers:' && timeout 30 docker ps -a --filter 'name=smackerel-test' --format '{{.Names}} {{.Status}}' && printf '%s\n' 'test-networks:' && timeout 30 docker network ls --filter 'name=smackerel-test' --format '{{.Name}}' && printf '%s\n' 'test-volumes:' && timeout 30 docker volume ls --filter 'name=smackerel-test' --format '{{.Name}}' && printf '%s\n' 'test-processes:' && if timeout 30 pgrep -af '[s]mackerel\.sh test|[s]mackerel-test|[t]est_topic_lifecycle'; then printf '%s\n' 'cleanup-verdict=unexpected-process'; exit 1; else process_status=$?; if [[ "$process_status" -ne 1 ]]; then exit "$process_status"; fi; fi && printf '%s\n' 'cleanup-verdict=zero-listed-resources' 'STABILIZE CLEANUP CONFIRMATION COMPLETE'`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
STABILIZE CLEANUP CONFIRMATION
tracked-delta:
 M specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/report.md
 M specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count/state.json
test-containers:
test-networks:
test-volumes:
test-processes:
cleanup-verdict=zero-listed-resources
STABILIZE CLEANUP CONFIRMATION COMPLETE
```

**Result:** PASS. No disposable Smackerel test resource remained, and the only
tracked delta is the authorized report and state pair.

### Stabilize Packet Artifact Lint

**Phase:** stabilize
**Executed:** YES (current session)
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count`
**Exit Code:** 0
**Claim Source:** executed
**Output:**

```text
# BUG-003-002 stabilize packet artifact lint
$ bash .github/bubbles/scripts/artifact-lint.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count
exit: 0
lines: 41
sha256: d0fcc3d00860e2793d6f53a8434292f1a505ecf38ce4456d8d8db5bf25097ada
--- first 20 ---
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Found Checklist section in uservalidation.md
✅ uservalidation checklist contains checkbox entries
✅ All checklist bullet items use checkbox syntax
✅ uservalidation separates automation readiness from human acceptance
✅ Detected state.json status: in_progress
✅ Detected state.json workflowMode: bugfix-fastlane
✅ state.json v3 has required field: status
✅ state.json v3 has required field: execution
✅ state.json v3 has required field: certification
✅ state.json v3 has required field: policySnapshot
--- omitted 1 line(s); sha256 above covers the full output ---
--- last 20 ---
✅ state.json v3 has recommended field: reworkQueue
✅ state.json v3 has recommended field: executionHistory
✅ Top-level status matches certification.status
ℹ️  Workflow mode 'bugfix-fastlane' allows status 'done'; current status is 'in_progress'
✅ report.md contains section matching: ###[[:space:]]+Summary|^##[[:space:]]+Summary
✅ report.md contains section matching: ###[[:space:]]+Completion Statement|^##[[:space:]]+Completion Statement
✅ report.md contains section matching: ###[[:space:]]+Test Evidence|^##[[:space:]]+Test Evidence
✅ Mode-specific report gates skipped (status not in promotion set)
✅ Value-first selection rationale lint skipped (not a value-first report)
✅ Scenario path-placeholder lint skipped (no matching scenario sections found)

=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

**Result:** PASS.

### Stabilize Phase Boundary

Only this report and stabilize-owned `execution.*` provenance in `state.json`
change in this phase. Top-level status and certification remain `in_progress`.
No `certification.*` field, source, test, config, deployment, framework, scope,
scenario-manifest, or R-020 artifact changes. Routing advances to
`bubbles.devops`.

## DevOps Evidence — Retry 1 Packet-Only Review (2026-08-29)

**Phase:** devops

**Starting revision:** `0e1e850cf77ef3cb8b4b18bc5429cc77dad19874`

**Claim Source:** executed and interpreted

### Excluded Operational Surfaces

**Executed:** YES (current session)

**Command:** `BUG_COMMITS=(7a2d10d0 92b6893b 5e759d86 2298cc61 5247a0c3 a6e927b2 fc5f1d43 4f428517 cd71d621 1a4e8c68 41d235f5 522f7e66 0e1e850c); scan_bug_paths <deploy-contract|config|migration|secret|adapter>`

**Exit Code:** 0

**Output:**

```text
BUG-003-002 DEVOPS EXCLUDED-SURFACE SCAN
HEAD=0e1e850cf77ef3cb8b4b18bc5429cc77dad19874
bug_commit_count=13
deploy_contract=none
config=none
migration=none
secret=none
adapter=none
independent_security_commit=cf005ed4d59cb1de3f025376f917dbc7ce6d4843
independent_security_commit_in_bug_set=no
product_deploy_implementation_required=no
BUG-003-002 DEVOPS EXCLUDED-SURFACE SCAN COMPLETE
```

**Result:** PASS. The selected bug commits change only lifecycle source, focused
tests, packet artifacts, and the open-work record. They change no deploy
contract, config, migration, secret, or adapter path. No product deploy
implementation or deployment is required.

### Independent Security Commit

Current-session Git inspection confirms that independent commit `cf005ed4`
pins the builder to Go 1.25.13 and `golang.org/x/net` to v0.56.0. The operator
states that this commit already cleared Trivy and carries separate verification.
This phase did not rerun Trivy or adopt that prior result as current-session
execution evidence.

### Repository Check

**Executed:** YES (current session)

**Command:** `./smackerel.sh check`

**Exit Code:** 0

**Claim Source:** executed

**Path normalization:** The generated temporary path is rendered under
`~/smackerel`.
**Output:**

```text
# BUG-003-002 DevOps repository check at 0e1e850c
$ ./smackerel.sh check
exit: 0
lines: 6
sha256: 653750408fd7765a43b54a95fd36cecb82d013ed62a10c61009b6b373cb458ba
--- output ---
config-validate: ~/smackerel/config/generated/dev.env.tmp.2711454 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
```

**Result:** PASS.

### Artifact Lint

**Executed:** YES (current session)

**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count`

**Exit Code:** 0

**Claim Source:** executed

**Output:**

```text
# BUG-003-002 DevOps artifact lint
$ bash .github/bubbles/scripts/artifact-lint.sh specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count
exit: 0
lines: 41
sha256: d0fcc3d00860e2793d6f53a8434292f1a505ecf38ce4456d8d8db5bf25097ada
--- first 20 ---
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Found Checklist section in uservalidation.md
✅ uservalidation checklist contains checkbox entries
✅ All checklist bullet items use checkbox syntax
✅ uservalidation separates automation readiness from human acceptance
✅ Detected state.json status: in_progress
✅ Detected state.json workflowMode: bugfix-fastlane
✅ state.json v3 has required field: status
✅ state.json v3 has required field: execution
✅ state.json v3 has required field: certification
✅ state.json v3 has required field: policySnapshot
--- omitted 1 line(s); sha256 above covers the full output ---
--- last 20 ---
✅ state.json v3 has recommended field: reworkQueue
✅ state.json v3 has recommended field: executionHistory
✅ Top-level status matches certification.status
ℹ️  Workflow mode 'bugfix-fastlane' allows status 'done'; current status is 'in_progress'
✅ report.md contains section matching: ###[[:space:]]+Summary|^##[[:space:]]+Summary
✅ report.md contains section matching: ###[[:space:]]+Completion Statement|^##[[:space:]]+Completion Statement
✅ report.md contains section matching: ###[[:space:]]+Test Evidence|^##[[:space:]]+Test Evidence
✅ Mode-specific report gates skipped (status not in promotion set)
✅ Value-first selection rationale lint skipped (not a value-first report)
✅ Scenario path-placeholder lint skipped (no matching scenario sections found)

=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

**Result:** PASS.

### DevOps Phase Boundary

The BUG-003-002 delta changes only this report and DevOps-owned `execution.*`
provenance in `state.json`. BUG-003-002 has no deployment, configuration,
migration, or secret delta. The worktree contains a separate uncommitted
`config/smackerel.yaml` change. That change remains untouched and excluded from
this packet. Finding `BUG-102-001-ML-ENV-PREFIX` routes to `bubbles.implement`
under
`specs/102-target-deploy-hardening/bugs/BUG-102-001-product-journey-acceptance-gap`.

Top-level status and certification status remain `in_progress`. Every
`certification.*` field remains untouched. This phase did not build, deploy,
modify source or framework files, or mint receipts. Routing advances to
`bubbles.security`.
