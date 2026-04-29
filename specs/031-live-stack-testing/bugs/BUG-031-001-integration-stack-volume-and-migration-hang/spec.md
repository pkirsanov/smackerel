# Bug Spec: [BUG-031-001] Integration stack volume retention + non-idempotent migration crash-loops core

## Problem Statement
`./smackerel.sh test integration` is the contract surface that proves the live stack stays healthy across consecutive runs. Today it fails on the second run because (A) `down --volumes` silently drops the `--volumes` flag when it appears after the command token and (B) the consolidated initial schema migration is not idempotent. The combined effect is a crash-looping `smackerel-core` and a 120-second integration timeout, blocking spec 031's live-stack testing guarantee.

## Outcome Contract
- **Intent:** Restore the guarantee that `./smackerel.sh test integration` converges on a healthy stack on every invocation, including back-to-back runs that may inherit a prior test PostgreSQL volume.
- **Success Signal:** Two consecutive `./smackerel.sh test integration` runs against the same host both reach `services.{postgres,nats,ml_sidecar}.status == "up"` on `/api/health` within the existing 120-second probe budget, and any `down --volumes` invocation (regardless of flag position) leaves zero `smackerel-test-*-data` Docker volumes behind.
- **Hard Constraints:**
  - `smackerel.sh` MUST accept `--volumes`, `--env`, `--no-cache`, `--check`, and `--help/-h` flags in any position relative to the command token (pre- or post-command), matching common GNU-style argparse behavior.
  - All migrations under `internal/db/migrations/` MUST be safe to re-execute against a database where the relevant DDL objects already exist; re-execution MUST NOT crash `smackerel-core`.
  - The fix MUST NOT alter the visible CLI surface (commands, command names, exit codes, or printed `usage()`).
  - The fix MUST NOT touch production code outside `smackerel.sh`, `tests/integration/test_runtime_health.sh`, and `internal/db/migrations/*.sql`.
- **Failure Condition:** A second consecutive `./smackerel.sh test integration` run still produces a crash-looping `smackerel-core`, OR `down --volumes` (post-command position) leaves a `smackerel-test-postgres-data` volume behind, OR any other migration in `001/018/019/020` retains a non-idempotent DDL statement after the fix.

## Goals
- Repair `smackerel.sh` argv parsing so flags are positional-agnostic.
- Make `001_initial_schema.sql` (and any other affected migrations under `001/018/019/020`) idempotent for `ALTER TABLE … ADD CONSTRAINT`, `CREATE TYPE`, and `CREATE FUNCTION` statements.
- Add scenario-specific regression coverage that fails on pre-fix HEAD and passes after the fix.

## Non-Goals
- Refactoring the migration runner (`internal/db/migrate.go` or equivalent).
- Introducing a new migration framework, hashing scheme, or `IF NOT EXISTS` rewriter.
- Changing the consolidated migration's logical schema. Object definitions remain identical.
- Touching production Go or Python code beyond migration SQL.
- Renaming, restructuring, or relocating CLI commands.

## Requirements

### Functional
- FR1: `./smackerel.sh --env test down --volumes`, `./smackerel.sh down --volumes --env test`, and `./smackerel.sh down --env test --volumes` MUST all set `DOWN_VOLUMES=true` and `TARGET_ENV=test`, and MUST run `docker compose … down --timeout 30 -v --remove-orphans`.
- FR2: `./smackerel.sh test integration` invoked twice in succession MUST both produce a `/api/health` payload with `services.postgres.status == "up"`, `services.nats.status == "up"`, and `services.ml_sidecar.status == "up"`.
- FR3: Re-running the migration runner against a PostgreSQL database whose `annotations` table already carries `chk_rating_range` (and any other DDL objects covered by the audit) MUST succeed without crashing `smackerel-core`.
- FR4: The audit MUST cover migrations 001, 018, 019, and 020 for `ALTER TABLE … ADD CONSTRAINT`, `CREATE TYPE`, and `CREATE FUNCTION` statements lacking idempotency guards; any flaw found MUST be fixed using the same `DO $$ … EXCEPTION WHEN duplicate_object THEN NULL; END $$;` pattern (or equivalent guard for the specific object kind).

### Non-Functional
- NFR1: Fix scope is bounded to `smackerel.sh`, `tests/integration/test_runtime_health.sh`, and `internal/db/migrations/*.sql`. No other production code is modified.
- NFR2: Regression tests MUST run inside the project test surface (`./smackerel.sh test integration` and a shell-level test runnable from the repo root).
- NFR3: Adversarial regression test cases (per Bubbles bug-fix policy) MUST exercise input that would fail if the bug were reintroduced (post-command `--volumes` positioning; pre-existing constraint object).

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-031-BUG001-A1 — `--volumes` is honored when placed after the command token
  Given the smackerel.sh CLI on HEAD
  When the operator runs "./smackerel.sh --env test down --volumes"
  Then DOWN_VOLUMES MUST resolve to true
   And the docker compose invocation MUST include the "-v" flag
   And no "smackerel-test-*-data" named volume MUST remain after the command completes

Scenario: SCN-031-BUG001-B1 — Integration suite is healthy across consecutive runs
  Given a clean repository state
  When the operator runs "./smackerel.sh test integration" once and lets it complete
   And the operator immediately runs "./smackerel.sh test integration" a second time
  Then both runs MUST reach a payload where services.postgres.status, services.nats.status, and services.ml_sidecar.status all equal "up"
   And smackerel-core MUST NOT enter a Docker restart loop on the second run
   And the migration runner MUST NOT report SQLSTATE 42710 on chk_rating_range

Scenario: SCN-031-BUG001-Pre-fix-fail — Pre-fix HEAD demonstrably fails the regression
  Given pre-fix HEAD
  When the regression tests for SCN-031-BUG001-A1 and SCN-031-BUG001-B1 are executed
  Then SCN-031-BUG001-A1 MUST fail because "smackerel-test-postgres-data" remains after teardown
   And SCN-031-BUG001-B1 MUST fail because the second run times out at 120s with exit code 143
```

## Acceptance Criteria
- AC1: SCN-031-BUG001-A1 has at least one shell-level regression test that exercises the actual `smackerel.sh` argv loop and asserts on observable Docker volume state (or, equivalently, on the resolved `DOWN_VOLUMES` value via a test-harness hook). The test MUST include an adversarial case where `--volumes` appears AFTER `down`.
- AC2: SCN-031-BUG001-B1 has at least one integration regression test that runs `./smackerel.sh --env test up` twice over a retained PostgreSQL volume and asserts that core reaches `/api/health` healthy on the second run.
- AC3: SCN-031-BUG001-Pre-fix-fail is demonstrated by capturing the pre-fix HEAD failure output for both regressions in `report.md` BEFORE the fix is applied.
- AC4: All three scenarios pass post-fix; full `./smackerel.sh test integration` run is healthy.
- AC5: Audit notes for migrations 001/018/019/020 are recorded in `design.md` (which guards were added, which statements were already safe).
