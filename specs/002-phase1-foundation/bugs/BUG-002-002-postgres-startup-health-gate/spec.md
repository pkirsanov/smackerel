# Bug Spec: BUG-002-002 Postgres Startup Health Gate

## Problem Statement

The Phase 1 live-stack lifecycle contract owns `SCN-002-004: Data persistence across restarts`, but the canonical E2E runner can currently attempt persistence writes while the disposable PostgreSQL test service is not running. This blocks Phase 1 evidence and any downstream bug or feature that needs a clean full `./smackerel.sh test e2e` pass.

## Outcome Contract

**Intent:** The E2E harness must only enter persistence and downstream Go E2E work after the test-stack PostgreSQL service is truly ready, and it must prove artifact data survives a Compose restart that preserves the test volume.

**Success Signal:** `./smackerel.sh test e2e` no longer aborts at `SCN-002-004` with `service "postgres" is not running`, `tests/e2e/test_persistence.sh` records a successful insert and restart persistence check, and the suite proceeds far enough to execute the Go E2E block when no unrelated blocker intervenes.

**Hard Constraints:** All runtime work flows through `./smackerel.sh`; test storage is disposable; protected developer volumes are preserved by default; generated config is never edited; SST-managed config has no hidden defaults or fallbacks; live-stack tests use the real stack without request interception.

**Failure Condition:** The wait or health gate can return success while PostgreSQL is stopped, unhealthy, still in initdb transition, or unable to complete `psql SELECT 1`; or the persistence scenario passes without proving that data survived a restart.

## Requirements

- **R-BUG-002-002-01:** `./smackerel.sh --env test up` and the E2E lifecycle helper must not return success until PostgreSQL is available through the same Compose/test path used by the persistence scenario.
- **R-BUG-002-002-02:** The postgres container healthcheck must not falsely pass against an initdb-only unix-socket server or any other state that cannot serve the test harness over the intended runtime path.
- **R-BUG-002-002-03:** The E2E health gate must reject `/api/health` responses that indicate degraded service state and must also require a successful PostgreSQL round trip.
- **R-BUG-002-002-04:** `tests/e2e/test_persistence.sh` must prove write, restart without volume deletion, read, and value preservation through the disposable test stack.
- **R-BUG-002-002-05:** Regression coverage must include an adversarial case where PostgreSQL is stopped or unhealthy and the health gate fails loudly instead of silently passing.
- **R-BUG-002-002-06:** Regression coverage must include a clean initdb transition case that would fail if the postgres healthcheck could falsely pass before TCP readiness.
- **R-BUG-002-002-07:** Shared live-stack lifecycle changes must declare and respect a narrow boundary across Compose, CLI lifecycle, E2E helper, E2E runner, and persistence test surfaces.

## User Scenarios (Gherkin)

```gherkin
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
  Given the canonical E2E suite is running through `./smackerel.sh test e2e`
  When Phase 1 lifecycle scenarios complete
  Then the suite is not aborted by `service "postgres" is not running`
  And the Go E2E block containing `tests/e2e/capture_process_search_test.go` is eligible to run
```

## Acceptance Criteria

- `tests/e2e/test_persistence.sh` fails before the fix with the reported postgres-not-running failure or an equivalent pre-readiness failure.
- The adversarial readiness regression fails before the fix when postgres is stopped or unhealthy and passes after the fix by rejecting false readiness.
- The clean-initdb regression proves the health gate cannot pass during the initdb transition before PostgreSQL accepts a real query.
- The post-fix persistence scenario inserts a concrete artifact, preserves the test postgres volume across restart, and reads the same artifact afterward.
- `./smackerel.sh test e2e` no longer stops at `SCN-002-004` for postgres readiness.
- Static scans show no generated config edits, no hidden fallback config, and no silent-pass bailout patterns in the regression tests.
