# Bug: [BUG-031-001] Integration stack volume retention + non-idempotent migration crash-loops core

## Summary
`./smackerel.sh test integration` reliably hangs and times out because two cooperating defects retain the test PostgreSQL volume across runs and then crash-loop `smackerel-core` on a re-applied, non-idempotent constraint migration.

## Severity
- [x] Critical - System unusable, data loss
- [ ] High
- [ ] Medium
- [ ] Low

Rationale: every `./smackerel.sh test integration` invocation fails the same way; the integration test surface is effectively unusable.

## Status
- [ ] Reported
- [x] Confirmed (reproduced)
- [ ] In Progress
- [x] Fixed
- [x] Verified
- [x] Closed

## Reproduction Steps
1. Start from a clean checkout (no `smackerel-test-postgres-data` volume).
2. Run `./smackerel.sh test integration`. First run brings the stack up and (eventually) succeeds or fails on test logic; the cleanup trap fires.
3. Inspect Docker volumes: `docker volume ls | grep smackerel-test-postgres-data` — the volume is still present despite the cleanup invoking `down --volumes`.
4. Run `./smackerel.sh test integration` a second time.
5. Observe `smackerel-core` repeatedly restarting and `tests/integration/test_runtime_health.sh` timing out at 120s with exit code 143 (SIGTERM from `timeout`). `docker logs smackerel-core` shows the migration runner failing with `SQLSTATE 42710` on `chk_rating_range`.

## Expected Behavior
- `./smackerel.sh --env test down --volumes` (and the equivalent invocation from `tests/integration/test_runtime_health.sh` and the `integration_cleanup` trap in `smackerel.sh`) MUST remove the named test volumes regardless of flag position relative to the command token.
- Migrations under `internal/db/migrations/` MUST be idempotent. Re-running the migration runner against a database where DDL objects already exist MUST NOT crash `smackerel-core`.
- `./smackerel.sh test integration` MUST converge on a healthy core on every run, including back-to-back runs.

## Actual Behavior
- DEFECT A (CLI argv parser): `smackerel.sh` (lines 70–96) only consumes flags BEFORE the command token. The `while` loop hits `*) break ;;` on the first non-flag argument, so `./smackerel.sh --env test down --volumes` parses `--env test`, breaks on `down`, and `--volumes` is never seen. `DOWN_VOLUMES` stays `false`, the `down)` case (lines 355–363) takes the no-`-v` branch, and the test PostgreSQL named volume survives teardown. Affected callers:
  - `tests/integration/test_runtime_health.sh` line 33 (cleanup trap) and line 41 (pre-up baseline cleanup).
  - `smackerel.sh` `integration_cleanup` trap (~line 240).
- DEFECT B (non-idempotent migration): `internal/db/migrations/001_initial_schema.sql` line 512 contains:
  ```sql
  ALTER TABLE annotations ADD CONSTRAINT chk_rating_range
      CHECK (rating IS NULL OR (rating >= 1 AND rating <= 5));
  ```
  PostgreSQL does not support `ADD CONSTRAINT IF NOT EXISTS`. When the test PostgreSQL volume survives teardown (because of Defect A) but the migration runner state (e.g. an empty `schema_migrations` table after a partial reset, or recomputed digest) decides to re-run the consolidated initial migration, the `ADD CONSTRAINT` fails with `SQLSTATE 42710 (duplicate_object)`. `smackerel-core` exits non-zero, Docker restarts it, and the failure repeats indefinitely.
- COMBINED SYMPTOM: integration runs reuse the prior test PostgreSQL volume (Defect A) → consolidated migration trips Defect B → `smackerel-core` crash-loops and never serves `/api/health` → `tests/integration/test_runtime_health.sh` times out at 120s with exit code 143.

## Environment
- Service: `smackerel-core` (Go), `smackerel-postgres` (test profile)
- Version: HEAD of `main` at 2026-04-26
- Platform: Linux + Docker Compose (test profile)
- Surface: `./smackerel.sh test integration`

## Error Output
Representative `docker logs smackerel-core` excerpt during the second run (illustrative — exact line will be captured during pre-fix reproduction in `report.md`):

```
migration 001_initial_schema.sql failed: ERROR: constraint "chk_rating_range" for relation "annotations" already exists (SQLSTATE 42710)
```

Representative test runner failure:

```
test_runtime_health.sh: timeout after 120s waiting for /api/health (exit 143)
smackerel-core   restarting (1) ...
```

## Root Cause (filled after analysis)
See `design.md` → "Root Cause Analysis". Two cooperating defects in `smackerel.sh` argv parsing and `internal/db/migrations/001_initial_schema.sql` idempotency.

## Related
- Feature: `specs/031-live-stack-testing/`
- Cross-spec dependencies (coordination only — primary packet stays under 031):
  - `specs/022-operational-resilience/` — owns DB migration robustness expectations.
  - `specs/027-user-annotations/` — originator of the `chk_rating_range` constraint added in `001_initial_schema.sql`.
- Related bugs: none currently open
- Related PRs: none yet

## Deferred Reason (if mode: document)
N/A — `mode: fix` packet. Defects block the integration test surface; fix should be scheduled immediately.
