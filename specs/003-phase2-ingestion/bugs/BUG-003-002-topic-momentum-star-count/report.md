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
