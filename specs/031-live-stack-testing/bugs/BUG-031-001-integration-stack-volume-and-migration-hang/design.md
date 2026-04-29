# Bug Fix Design: [BUG-031-001] Integration stack volume retention + non-idempotent migration crash-loops core

## Root Cause Analysis

### Investigation Summary
Two cooperating defects were identified by reading `smackerel.sh`, `tests/integration/test_runtime_health.sh`, and `internal/db/migrations/001_initial_schema.sql` against the observed symptom (`./smackerel.sh test integration` second run times out with `smackerel-core` crash-looping on a duplicate constraint).

### Root Cause

#### Defect A — `smackerel.sh` argv loop is pre-command only
The flag-consuming loop in `smackerel.sh` (lines 70–96) is:

```bash
while [[ $# -gt 0 ]]; do
  case "$1" in
    --env) TARGET_ENV="$2"; shift 2 ;;
    --env=*) TARGET_ENV="${1#*=}"; shift ;;
    --no-cache) NO_CACHE=true; shift ;;
    --check) FORMAT_CHECK=true; shift ;;
    --volumes) DOWN_VOLUMES=true; shift ;;
    --help|-h) usage; exit 0 ;;
    *) break ;;
  esac
done

COMMAND="${1:-help}"
shift || true
```

On encountering the first non-flag token (the command, e.g. `down`), the loop `break`s. Any flags positioned AFTER the command are NOT parsed by this loop. The `down` case body (lines 355–363) only consumes its options through the global `DOWN_VOLUMES` already-set state — it does NOT re-parse `"$@"`. So:

| Invocation | `DOWN_VOLUMES` | Compose flag |
|---|---|---|
| `./smackerel.sh --env test --volumes down` | `true` | `-v` |
| `./smackerel.sh --env test down --volumes` | `false` | (no `-v`) |
| `./smackerel.sh down --volumes --env test` | `false` | (no `-v`) |

Affected callers all use the post-command form:
- `tests/integration/test_runtime_health.sh:33` — `cleanup` trap.
- `tests/integration/test_runtime_health.sh:41` — pre-up baseline cleanup.
- `smackerel.sh` `integration_cleanup` trap (~line 240).

Result: every `down --volumes` invocation issued from the test surface silently degrades to a non-volume teardown, and the named `smackerel-test-postgres-data` volume persists across runs.

#### Defect B — `001_initial_schema.sql` has a non-idempotent ADD CONSTRAINT
`internal/db/migrations/001_initial_schema.sql:512`:

```sql
ALTER TABLE annotations ADD CONSTRAINT chk_rating_range
    CHECK (rating IS NULL OR (rating >= 1 AND rating <= 5));
```

PostgreSQL has no `ADD CONSTRAINT IF NOT EXISTS`. When the migration runner re-applies the consolidated initial migration against a database where `chk_rating_range` already exists (for example because the test PostgreSQL volume survived teardown but `schema_migrations` is empty/divergent, or because the migration runner re-applies based on a digest mismatch), the statement fails with:

```
ERROR: constraint "chk_rating_range" for relation "annotations" already exists (SQLSTATE 42710)
```

`smackerel-core` exits non-zero on migration failure, Docker restarts it, and the failure repeats indefinitely.

The audit MUST also cover migrations `001`, `018`, `019`, and `020` for sibling patterns:
- `ALTER TABLE … ADD CONSTRAINT` without an idempotency guard.
- `CREATE TYPE` without `IF NOT EXISTS` (PostgreSQL only supports this for some object kinds; types require a `DO $$ … EXCEPTION WHEN duplicate_object THEN NULL; END $$;` block).
- `CREATE FUNCTION` without `OR REPLACE`.

### Combined Failure Chain
1. Run #1: stack comes up; cleanup trap calls `down --volumes` (post-command) → Defect A drops `--volumes` → `smackerel-test-postgres-data` volume survives.
2. Run #2: `up` reuses the surviving volume; the migration runner re-applies the consolidated initial migration; Defect B fails on `chk_rating_range`; `smackerel-core` enters Docker restart loop.
3. `tests/integration/test_runtime_health.sh` polls `/api/health`; core never serves; `timeout 120` kills the probe with exit 143.

### Impact Analysis
- Affected components:
  - `smackerel.sh` (argv parser).
  - `tests/integration/test_runtime_health.sh` (depends on `down --volumes` actually dropping volumes).
  - `internal/db/migrations/001_initial_schema.sql` (and possibly 018/019/020).
  - `smackerel-core` runtime (crash-loops as downstream symptom — no code change required).
- Affected data: test environment only. No dev/prod data path is implicated; dev volumes are not exercised by the integration test surface.
- Affected users: every developer or CI run that executes `./smackerel.sh test integration` more than once on the same host.

## Fix Design

### Solution Approach

#### Fix A — Reorder/repeat-aware argv parsing in `smackerel.sh`
Replace the single pre-command flag loop with a two-pass strategy that accepts flags in any position:

1. Walk `"$@"` once, classifying each token as either a known flag (consumed into the corresponding global) or a positional. Build a positional array.
2. After the walk, treat positional[0] as `COMMAND` and pass the remaining positionals as the command's argument tail.
3. Preserve existing flag semantics: `--env <value>` and `--env=<value>` both supported; `--no-cache`, `--check`, `--volumes` are booleans; `--help/-h` exits with usage.

This matches common GNU-style argparse behavior (flags positional-agnostic, `--` could later be added to terminate flag scanning if desired, but is not required for this fix).

Pseudocode:

```bash
positional=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --env)        TARGET_ENV="$2"; shift 2 ;;
    --env=*)      TARGET_ENV="${1#*=}"; shift ;;
    --no-cache)   NO_CACHE=true; shift ;;
    --check)      FORMAT_CHECK=true; shift ;;
    --volumes)    DOWN_VOLUMES=true; shift ;;
    --help|-h)    usage; exit 0 ;;
    *)            positional+=("$1"); shift ;;
  esac
done
set -- "${positional[@]}"
COMMAND="${1:-help}"
shift || true
```

This change is local to `smackerel.sh` and preserves every existing call site.

#### Fix B — Wrap the offending `ADD CONSTRAINT` in an exception-swallowing block
Replace the bare statement with:

```sql
DO $$
BEGIN
    ALTER TABLE annotations ADD CONSTRAINT chk_rating_range
        CHECK (rating IS NULL OR (rating >= 1 AND rating <= 5));
EXCEPTION WHEN duplicate_object THEN
    NULL;
END
$$;
```

The same guard pattern is applied to any other `ALTER TABLE … ADD CONSTRAINT` discovered during the audit of migrations 001/018/019/020. For `CREATE TYPE` use the same `DO $$ … EXCEPTION WHEN duplicate_object THEN NULL; END $$;` shape; for `CREATE FUNCTION` use `CREATE OR REPLACE FUNCTION`.

The audit findings (which statements were patched, which were already safe) are recorded in `report.md` alongside the diff evidence.

### Alternative Approaches Considered

1. **Force `tests/integration/test_runtime_health.sh` and the `integration_cleanup` trap to use the pre-command form (`--volumes` BEFORE `down`).** Rejected: it papers over Defect A, leaves a footgun for every future caller, and does not match the documented `Usage:` text (`down [--volumes]`) which explicitly invites the post-command form.
2. **Rewrite the migration runner to detect and skip already-applied DDL.** Rejected: out of scope (production code change beyond the boundary), and the per-statement guard is the canonical PostgreSQL idiom for this exact problem.
3. **Drop the named volume in the integration test runner explicitly via `docker volume rm`.** Rejected: hides the defect behind out-of-band cleanup logic and bypasses the contract that `down --volumes` is supposed to honor.
4. **Replace `chk_rating_range` with a column-level `CHECK` baked into the `CREATE TABLE`.** Rejected: it would require restructuring the consolidated migration, and the underlying class of bug (other non-idempotent DDL in 018/019/020) would remain unaddressed.

### Affected Files (Change Boundary)
- `smackerel.sh` — argv loop only.
- `tests/integration/test_runtime_health.sh` — comment update only IF needed; no behavioral change required after Fix A lands (the existing post-command `--volumes` form will start working).
- `internal/db/migrations/001_initial_schema.sql` — wrap `chk_rating_range` `ADD CONSTRAINT`.
- `internal/db/migrations/018_*.sql`, `019_*.sql`, `020_*.sql` — same guard pattern WHERE the audit finds non-idempotent DDL.
- New regression test files (paths to be confirmed by `bubbles.implement`):
  - `tests/integration/test_smackerel_argv_volumes.sh` (or equivalent shell test) — SCN-031-BUG001-A1.
  - `tests/integration/test_integration_idempotent_restart.sh` (or extension to existing harness) — SCN-031-BUG001-B1.

Excluded (must NOT be touched):
- Any `internal/`, `cmd/`, `ml/`, or `web/` source outside `internal/db/migrations/`.
- `docker-compose.yml`, `docker-compose.prod.yml`.
- `config/smackerel.yaml` and `config/generated/`.
- Any other spec folder's artifacts.

## Regression Test Design

| Scenario ID | Test Type | Approach | Adversarial input |
|---|---|---|---|
| SCN-031-BUG001-A1 | Regression E2E (shell) | Invoke `./smackerel.sh --env test down --volumes` against a known-present test volume; assert volume is gone. Invoke with `--volumes` BEFORE `down` as a control case. | `--volumes` AFTER `down` (the broken position). On pre-fix HEAD this MUST fail; the volume MUST remain. |
| SCN-031-BUG001-B1 | Regression E2E (integration) | `./smackerel.sh --env test up` twice over a retained PostgreSQL volume; on the second run, poll `/api/health` and assert `services.postgres.status == "up"`. | Pre-existing `chk_rating_range` constraint (i.e. retained volume) — exactly the condition that triggers SQLSTATE 42710 on pre-fix HEAD. |
| SCN-031-BUG001-Pre-fix-fail | Evidence requirement | On pre-fix HEAD, run both regressions and capture failing output into `report.md` "Pre-Fix Regression Tests (MUST FAIL)" section. | N/A (this is the pre-fix evidence step itself). |

All three scenarios are listed in `scopes.md` Test Plan as `Regression E2E` rows (per Bubbles transition-guard requirement).

## Cross-Spec Coordination
- `specs/022-operational-resilience/` owns broader DB migration robustness expectations. This bug fix is consistent with that spec's posture but does NOT modify it; a follow-up note may be filed there if the audit reveals systemic non-idempotency beyond the four migrations in scope.
- `specs/027-user-annotations/` introduced the `chk_rating_range` constraint that is the proximate trigger for Defect B. No spec change is required there — the constraint's logical semantics are preserved; only its DDL idempotency is hardened.

## Observability
No new metrics or log events are required. The regression tests themselves are the observability for this defect class.

## Risks & Open Questions
- Risk: if `bubbles.implement`'s audit of migrations 018/019/020 surfaces multiple sibling defects, the `report.md` audit table may be larger than expected. This is a documentation expansion, not a scope expansion.
- Open question: whether `tests/integration/test_runtime_health.sh:33` and `:41` need any comment update once Fix A makes their post-command `--volumes` form work as written. Answer to be resolved during implementation; default is no behavioral change to that file.
