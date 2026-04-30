# Scopes: BUG-002-002 Postgres Startup Health Gate

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore postgres readiness and persistence lifecycle evidence

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-002-002 Postgres startup health gate

  Scenario: SCN-002-BUG-002-001 Health gate rejects a stopped postgres service
    Given the disposable test stack has a core health endpoint that is reachable or degraded
    And the postgres service is stopped, unhealthy, or unable to complete SELECT 1
    When the E2E readiness gate is invoked
    Then the gate fails with a postgres readiness error
    And no persistence test is allowed to continue as if the stack were healthy

  Scenario: SCN-002-BUG-002-002 Clean initdb does not produce a false ready signal
    Given the disposable test postgres volume has been removed
    When the test stack starts from a clean initdb state
    Then the stack startup and E2E readiness gate wait until PostgreSQL can complete a real psql SELECT 1 round trip
    And SCN-002-004 can insert its artifact without a fixed-sleep race

  Scenario: SCN-002-BUG-002-003 Persistence survives restart after the readiness gate
    Given SCN-002-004 inserts a uniquely identifiable artifact into PostgreSQL
    When the test stack is stopped without removing the postgres test volume and then started again
    Then the artifact is still present after restart
    And the scenario reports the persisted count as 1

  Scenario: SCN-002-BUG-002-004 Canonical E2E reaches Go block after lifecycle scenarios
    Given the canonical E2E suite is running through ./smackerel.sh test e2e
    When Phase 1 lifecycle scenarios complete
    Then the suite is not aborted by service "postgres" is not running
    And the Go E2E block containing tests/e2e/capture_process_search_test.go is eligible to run
```

### Implementation Plan

1. Capture pre-fix red evidence for `SCN-002-004` using repo-standard runtime commands.
2. Harden postgres Compose health so it proves runtime-path readiness and cannot pass during initdb transition.
3. Harden `./smackerel.sh up` so startup waits for Compose health with a bounded fail-loud timeout.
4. Harden `tests/e2e/lib/helpers.sh::e2e_wait_healthy` so it rejects degraded health and requires PostgreSQL `SELECT 1` success.
5. Make `tests/e2e/run_all.sh` use the shared readiness helper instead of an inline curl-only wait.
6. Make `tests/e2e/test_persistence.sh` use the hardened readiness helper around initial start and restart, preserve only the disposable test postgres volume during the restart step, and assert the persisted row count.
7. Add adversarial regression coverage for stopped/unhealthy postgres and clean-initdb transition behavior.
8. Run canonical lifecycle and broader E2E evidence through `./smackerel.sh` and record raw output.

### Shared Infrastructure Impact Sweep

| Surface | Contract Risk | Required Guard |
|---|---|---|
| `docker-compose.yml` postgres healthcheck | Can change when dependents unblock | Independent canary for postgres TCP/query readiness |
| `smackerel.sh up` | Affects every runtime stack start | Bounded wait, fail-loud output, no generated-env edits |
| `tests/e2e/lib/helpers.sh` | Shared by many shell E2E tests | Canary proving stopped postgres fails the gate |
| `tests/e2e/run_all.sh` | Shared-stack ordering and readiness | Run helper before shared tests and lifecycle tests |
| `tests/e2e/test_persistence.sh` | Direct owner of SCN-002-004 | Round-trip persistence proof across restart |
| Test storage lifecycle | Risk of deleting protected dev data | `--env test` disposable volume cleanup only |

### Change Boundary

Allowed file families:

- `docker-compose.yml`
- `smackerel.sh`
- `scripts/lib/runtime.sh` if needed for a narrow Compose-wrapper change
- `tests/e2e/lib/helpers.sh`
- `tests/e2e/run_all.sh`
- `tests/e2e/test_persistence.sh`
- `config/smackerel.yaml` and generator code only if introducing an SST-managed timeout or port value
- This bug folder under `specs/002-phase1-foundation/bugs/BUG-002-002-postgres-startup-health-gate/`

Excluded file families:

- `internal/connector/`
- `internal/recommendation/`
- `internal/intelligence/`
- `ml/app/` except for direct health contract evidence if explicitly routed
- `config/generated/`
- Product feature specs outside cross-reference evidence updates
- Host-wide Docker cleanup scripts or commands

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-BUG-002-002-01 | Regression E2E: pre-fix SCN-002-004 postgres-not-running red stage | Regression E2E | `./smackerel.sh test e2e`, `tests/e2e/test_persistence.sh` | Before the fix, the persistence scenario fails at the reported postgres readiness point; after the fix, that failure is absent | SCN-002-BUG-002-002, SCN-002-BUG-002-003, parent SCN-002-004 |
| T-BUG-002-002-02 | Adversarial Regression E2E: stopped postgres cannot pass readiness | Regression E2E | `tests/e2e/lib/helpers.sh` plus a focused shell E2E canary | With postgres stopped or unable to answer SELECT 1, the readiness gate exits non-zero and the persistence test cannot continue | SCN-002-BUG-002-001 |
| T-BUG-002-002-03 | Regression E2E: clean initdb transition waits for real DB readiness | Regression E2E | `tests/e2e/test_persistence.sh` or focused lifecycle canary | After disposable test volume removal, startup waits until PostgreSQL can answer `SELECT 1`; no initdb false-ready signal is accepted | SCN-002-BUG-002-002 |
| T-BUG-002-002-04 | Regression E2E: persistence survives restart | Regression E2E | `tests/e2e/test_persistence.sh` | Insert unique artifact, stop without deleting the test postgres volume, restart, and assert count remains 1 | SCN-002-BUG-002-003, parent SCN-002-004 |
| T-BUG-002-002-05 | Canary: shared helper rejects degraded health | e2e-api | `tests/e2e/lib/helpers.sh` | `/api/health` returning degraded or postgres query failure does not produce success | SCN-002-BUG-002-001 |
| T-BUG-002-002-06 | Broader E2E regression suite reaches Go block | e2e-api | `./smackerel.sh test e2e` | The suite does not abort at SCN-002-004 with `service "postgres" is not running` and can reach `tests/e2e/capture_process_search_test.go` when no separately-owned blocker appears | SCN-002-BUG-002-004 |
| T-BUG-002-002-07 | Static guard: no silent-pass or mock shortcuts | artifact | regression-quality guard and live-test interception scan | Required tests contain no bailout returns and no live-stack request interception | All bug scenarios |
| T-BUG-002-002-08 | Docker lifecycle guard: disposable test storage only | artifact/e2e-api | `./smackerel.sh --env test down --volumes`, `./smackerel.sh --env test up` | Test cleanup affects the test stack and preserves protected developer volumes by default | SCN-002-BUG-002-002, SCN-002-BUG-002-003 |

### Definition of Done

- [x] Root cause reproduced and confirmed with pre-fix output from the canonical or focused live E2E path. **Phase:** bug/devops. **Claim Source:** interpreted from workflow-supplied executed failure evidence. Evidence: [report.md](report.md) `### Finding Classification` records `./smackerel.sh test e2e` exit 1 at `SCN-002-004` / `tests/e2e/test_persistence.sh` with `service "postgres" is not running` before the Go E2E block.
- [x] Fix implemented in the narrow live-stack lifecycle/test-harness boundary. **Phase:** devops. **Claim Source:** interpreted from implementation evidence. Evidence: [report.md](report.md) `### Implementation Summary` lists changes limited to SST config emission, Compose postgres health, `smackerel.sh` startup wait, and E2E harness readiness/persistence scripts.
- [x] Pre-fix regression test FAILS with the postgres readiness failure before the fix. **Phase:** bug/devops. **Claim Source:** interpreted from workflow-supplied executed failure evidence. Evidence: [report.md](report.md) `### Finding Classification` records the red-stage canonical E2E failure at `Inserting test artifact...` with `service "postgres" is not running`.
- [x] Adversarial regression case exists and proves the readiness gate cannot pass while postgres is stopped, unhealthy, or unable to answer `SELECT 1`. **Phase:** devops. **Claim Source:** executed. Evidence: [report.md](report.md) `### Focused Adversarial Readiness Proof` records `timeout 360 bash tests/e2e/test_postgres_readiness_gate.sh` exit 0 with stopped postgres rejected and `PASS: SCN-002-BUG-002-001`.
- [x] Clean-initdb regression proves the postgres healthcheck cannot falsely pass during initdb transition. **Phase:** devops. **Claim Source:** executed/interpreted. Evidence: [report.md](report.md) `### Focused Persistence Evidence` records the disposable test stack starting from prepared storage, waiting on the hardened gate, then inserting and verifying count=1 without fixed-sleep readiness; [report.md](report.md) `### Implementation Summary` records the TCP `pg_isready` plus `psql SELECT 1` health contract.
- [x] Post-fix regression test PASSES for `SCN-002-004` persistence across restart. **Phase:** devops. **Claim Source:** executed. Evidence: [report.md](report.md) `### Focused Persistence Proof` records `timeout 360 bash tests/e2e/test_persistence.sh` exit 0 with `PASS: SCN-002-004 (data persisted, count=1)`.
- [x] Regression tests contain no silent-pass bailout patterns. **Phase:** devops. **Claim Source:** executed. Evidence: [report.md](report.md) `### Regression Quality Guard Evidence` records `regression-quality-guard.sh --bugfix` exit 0 with `0 violation(s), 0 warning(s)` across the changed E2E harness files.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior. **Phase:** devops. **Claim Source:** executed/interpreted. Evidence: [report.md](report.md) `### Focused Persistence Proof` covers restart persistence and clean startup, and `### Focused Adversarial Readiness Proof` covers stopped-postgres rejection.
- [x] Broader E2E regression suite passes or reaches a separately-owned blocker after `SCN-002-004` and after the Go E2E block becomes eligible. **Phase:** devops. **Claim Source:** executed. Evidence: [report.md](report.md) `### Canonical Broad E2E Evidence` records `timeout 3600 ./smackerel.sh test e2e` exit 1 after `PASS: SCN-002-004`, `PASS: SCN-002-BUG-002-001`, and Go E2E reachability; remaining failures are listed as outside this Postgres startup health-gate packet.
- [x] Shared Infrastructure Impact Sweep is satisfied with an independent canary suite before broad suite reruns. **Phase:** devops. **Claim Source:** executed/interpreted. Evidence: [report.md](report.md) records focused persistence and stopped-postgres canary evidence before the broad E2E follow-up; `### Shared Infrastructure Impact Sweep Closeout` maps each shared surface to the recorded guard.
- [x] Rollback or restore path for shared lifecycle/test-harness changes is documented and verified. **Phase:** validate. **Claim Source:** interpreted from existing executed cleanup evidence. Evidence: [report.md](report.md) `### Rollback And Restore Path` documents the source revert boundary and records `timeout 180 ./smackerel.sh --env test down --volumes` cleanup evidence for restoring disposable test-stack state.
- [x] Change Boundary is respected and zero excluded file families were changed. **Phase:** validate. **Claim Source:** interpreted from existing implementation evidence. Evidence: [report.md](report.md) `### Implementation Summary` lists only allowed live-stack lifecycle, config, Compose, and E2E harness files; [report.md](report.md) `### Validation Evidence` records this closeout touched only the bug packet.
- [x] SST governance is preserved: no generated config edits, no hidden defaults, no unmanaged config values. **Phase:** devops/validate. **Claim Source:** executed/interpreted. Evidence: [report.md](report.md) `### Repo Check Evidence` records `timeout 120 ./smackerel.sh check` exit 0, and `### Implementation Summary` states no `config/generated` files were edited by hand.
- [x] Docker lifecycle governance is preserved: test storage is disposable and protected developer volumes are not pruned. **Phase:** devops. **Claim Source:** executed/interpreted. Evidence: [report.md](report.md) `### Cleanup Evidence` records `timeout 180 ./smackerel.sh --env test down --volumes` exit 0 and confirms no `smackerel-test-*` containers remained; no broad Docker prune was run.
- [x] Bug marked as Fixed in `bug.md` only after validate-owned evidence confirms the fix. **Phase:** validate. **Claim Source:** executed/interpreted. Evidence: [report.md](report.md) `### Validation Evidence` records validation closeout from existing executed focused and broad evidence, and [bug.md](bug.md) now marks Reported, Confirmed, In Progress, Fixed, Verified, and Closed.
