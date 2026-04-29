# Bug Fix Design: BUG-002-002 Postgres Startup Health Gate

## Root Cause Analysis

### Investigation Summary

This packet classifies a blocker found by test-owner verification of `BUG-031-003`. The reported failure is in the canonical live E2E suite, before the Go E2E block can execute. The observed scenario is `SCN-002-004: Data persistence across restarts`, owned by `specs/002-phase1-foundation/scopes.md`, with the executable shell test at `tests/e2e/test_persistence.sh`.

Workspace inspection found the current lifecycle surfaces are consistent with a readiness-contract regression:

```text
Claim Source: interpreted (workspace inspection)
docker-compose.yml postgres healthcheck: pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}
smackerel.sh up: smackerel_compose "$TARGET_ENV" up -d
tests/e2e/lib/helpers.sh e2e_wait_healthy: curl -sf --max-time 3 "$CORE_URL/api/health"
tests/e2e/run_all.sh Phase 1 wait: inline curl-only /api/health loop
tests/e2e/test_persistence.sh: fixed sleep before postgres psql insert and after restart
```

Prior evidence in `specs/038-cloud-drives-integration/report.md` describes the same class of cold-start postgres readiness flake and records a likely previous fix. A later state entry says that fix appeared un-applied during cross-cutting churn. This design treats that as a diagnostic lead, not as proof of the current root cause.

### Root Cause Hypothesis

The most likely root cause is a split readiness contract across Compose, the repo CLI, and the E2E harness:

1. The repo CLI returns from `up` after container start rather than after health readiness.
2. The postgres healthcheck can report ready without proving the real TCP/runtime path the test uses.
3. The E2E health helper and runner accept a shallow `/api/health` HTTP success instead of requiring semantic healthy status plus a database query.
4. The persistence test relies on fixed sleeps and then calls `docker compose exec postgres psql`, which fails if Compose considers the postgres service absent, stopped, or mid-transition.

The exact current failure string `service "postgres" is not running` can arise before `psql` even starts inside the container, so the implementation owner must reproduce and capture the red-state output before editing runtime files.

### Impact Analysis

- Affected owner scenario: `SCN-002-004 Data persistence across restarts`
- Affected owner test: `tests/e2e/test_persistence.sh`
- Affected shared surfaces: `docker-compose.yml`, `smackerel.sh`, `tests/e2e/lib/helpers.sh`, `tests/e2e/run_all.sh`, `tests/e2e/test_persistence.sh`
- Affected downstream evidence: `BUG-031-003` cannot receive post-fix live-stack evidence; `specs/039-recommendations-engine` full-delivery remains blocked
- Affected data: disposable test-stack PostgreSQL volume only; protected developer volumes must remain intact
- Affected users: developers and agents relying on canonical live-stack evidence

## Fix Design

### Solution Approach

The fix should be owned by `bubbles.devops` because it changes shared live-stack lifecycle and test harness contracts.

1. Reproduce the red state using `./smackerel.sh test e2e` or a focused repo-standard lifecycle command that exercises `tests/e2e/test_persistence.sh`.
2. Harden the postgres readiness contract so Compose health only succeeds when PostgreSQL is reachable through the intended runtime path. If the wait timeout or port is treated as configuration, route it through `config/smackerel.yaml` and generated config rather than introducing hidden defaults.
3. Harden `./smackerel.sh up` so it blocks on Compose health with a bounded timeout and fails loudly when services do not become healthy.
4. Replace shallow E2E health waits with the shared helper, and make that helper require both semantic service health and a real PostgreSQL `SELECT 1` round trip.
5. Update the persistence test to rely on the hardened readiness gate around both initial start and restart, while preserving the test postgres volume across the restart step.
6. Add adversarial regression coverage that forces postgres stopped or unhealthy and proves the readiness gate fails instead of silently passing.
7. Add clean-initdb regression coverage that proves the healthcheck cannot pass during the initdb transition before a real query succeeds.
8. Run canonical E2E evidence after the fix and verify the suite no longer aborts at `SCN-002-004` with postgres not running.

### Shared Infrastructure Impact Boundaries

Allowed file families for the fix:

- `docker-compose.yml` postgres healthcheck and health timing only
- `smackerel.sh` `up` lifecycle behavior only
- `scripts/lib/runtime.sh` only if the Compose wrapper needs a narrow helper adjustment
- `tests/e2e/lib/helpers.sh` readiness helper only
- `tests/e2e/run_all.sh` readiness orchestration only
- `tests/e2e/test_persistence.sh` persistence readiness and assertion flow only
- `config/smackerel.yaml` and generator code only if a newly configurable timeout or port value is introduced through the SST pipeline

Excluded surfaces:

- Product API behavior unrelated to `/api/health` semantics
- Domain connector code
- ML sidecar behavior unrelated to stack health
- Generated files under `config/generated/`
- Persistent developer volume cleanup behavior
- Broad Docker prune or host-wide cleanup commands
- Unrelated E2E scenario rewrites

### Regression Test Design

- **Pre-fix red evidence:** `./smackerel.sh test e2e` or focused lifecycle execution fails at `SCN-002-004` with postgres unavailable before the Go E2E block.
- **Adversarial readiness evidence:** With the disposable test stack in a state where postgres is stopped, unhealthy, or cannot answer `SELECT 1`, the shared readiness helper returns non-zero and the persistence scenario does not continue.
- **Clean-initdb evidence:** After removing the test postgres volume, stack startup waits through initdb and only returns after TCP/database query readiness.
- **Persistence evidence:** Insert a unique artifact, stop without removing the test postgres volume, restart, and assert the exact row count remains 1.
- **Broad evidence:** `./smackerel.sh test e2e` no longer fails at `SCN-002-004` for postgres readiness and reaches the Go E2E block unless a separately-owned later blocker appears.

### Alternative Approaches Considered

1. Increase fixed sleeps in `test_persistence.sh` - rejected because it preserves the race and does not prove readiness.
2. Treat this as part of `BUG-031-003` - rejected because ownership is Phase 1 live-stack lifecycle, not the capture-processing timeout bug.
3. Use broad Docker cleanup to clear the failure - rejected because it risks protected state and does not repair the lifecycle contract.
4. Mark downstream evidence as blocked without a bug packet - rejected because this is a separately-owned, reproducible canonical-suite blocker with its own regression requirements.
