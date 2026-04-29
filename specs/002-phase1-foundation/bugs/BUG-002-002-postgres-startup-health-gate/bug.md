# Bug: BUG-002-002 Postgres Startup Health Gate

## Summary

The canonical live E2E suite aborts during `SCN-002-004: Data persistence across restarts` because `tests/e2e/test_persistence.sh` attempts to insert a test artifact while the Compose `postgres` service is not running.

## Severity

- [ ] Critical - System unusable, data loss
- [x] High - Canonical live-stack E2E is blocked for Phase 1 lifecycle evidence and downstream bug verification
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status

- [x] Reported
- [ ] Confirmed by this bug packet
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Run `./smackerel.sh test e2e` from the repository root.
2. Observe the suite enter `tests/e2e/test_persistence.sh` for `SCN-002-004: Data persistence across restarts`.
3. Observe the failure at `Inserting test artifact...`.
4. Observe the reported Docker Compose error: `service "postgres" is not running`.
5. Observe that the suite exits before the Go E2E block can execute `tests/e2e/capture_process_search_test.go`.

## Expected Behavior

The live E2E harness must not begin `SCN-002-004` database mutation work until the disposable test-stack PostgreSQL service is running, marked healthy by a TCP-ready healthcheck, and able to complete a real `psql SELECT 1` round trip. After a restart that preserves the test volume, the inserted artifact must still be present.

## Actual Behavior

The canonical E2E suite reaches the persistence scenario while `postgres` is not running from Compose's perspective. The insert step fails before the scenario can prove persistence across restart, and the remaining Go E2E block is never reached.

## Environment

- Command: `./smackerel.sh test e2e`
- Environment: generated test stack via `./smackerel.sh --env test`
- Compose project: `smackerel-test`
- Scenario: `SCN-002-004: Data persistence across restarts`
- Test file: `tests/e2e/test_persistence.sh`
- Platform: Linux

## Error Output

```text
Command: ./smackerel.sh test e2e
Exit: 1
Scenario: SCN-002-004: Data persistence across restarts
Test file: tests/e2e/test_persistence.sh
Failure point: Inserting test artifact...
Error: service "postgres" is not running
Consequence: BUG-031-003 cannot receive post-fix live-stack evidence; feature 039 full-delivery remains blocked.
```

## Root Cause Hypothesis

Workspace inspection shows the current live-stack readiness contract can falsely signal readiness before PostgreSQL is actually available to the test harness:

- `docker-compose.yml` currently uses `pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}` without an explicit TCP host/port in the postgres healthcheck.
- `smackerel.sh up` currently invokes Compose as `up -d` without `--wait` / `--wait-timeout`.
- `tests/e2e/lib/helpers.sh::e2e_wait_healthy` currently accepts any successful HTTP response from `/api/health` and does not require a database round trip.
- `tests/e2e/run_all.sh` currently contains its own inline `/api/health` wait instead of the shared helper.
- `tests/e2e/test_persistence.sh` currently relies on fixed sleeps around startup and restart before invoking `psql`.

Prior diagnostic evidence in `specs/038-cloud-drives-integration` describes the same class of cold-start readiness failure and records a likely prior fix. That prior evidence is treated here as a lead only; the implementation owner must reproduce the red state for this bug before applying a fix.

## Governance Constraints

- Runtime operations must use `./smackerel.sh`.
- Generated files under `config/generated/` must not be edited.
- Test and validation storage must remain disposable and must not write into protected developer volumes.
- Persistent developer volumes must be preserved by default.
- No hardcoded fallback config, hidden defaults, or generated-env edits are allowed.
- Live E2E tests must hit the real running stack with no request interception or mock shortcuts.

## Related

- Owner feature: `specs/002-phase1-foundation/`
- Owner scenario: `SCN-002-004: Data persistence across restarts`
- Owner test file: `tests/e2e/test_persistence.sh`
- Blocking consequence: `specs/031-live-stack-testing/bugs/BUG-031-003-capture-processing-timeout`
- Blocked continuation: `specs/039-recommendations-engine`
- Diagnostic lead: `specs/038-cloud-drives-integration/report.md` Round 9 cross-cutting stability finding
