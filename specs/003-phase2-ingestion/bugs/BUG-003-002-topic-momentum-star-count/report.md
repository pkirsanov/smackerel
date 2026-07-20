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

No audit-owned result is recorded.
