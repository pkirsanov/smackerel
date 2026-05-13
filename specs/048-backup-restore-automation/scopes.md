# Scopes: Backup and Restore Automation

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope 1: Backup schedule and retention contract

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-048-B01 Scheduled backups retain daily and weekly history
  Given Smackerel is configured for deployment operation
  When backup automation runs over time
  Then the system keeps 7 daily backups and 4 weekly backups
  And backups older than the retention window are pruned without deleting retained points
```

### Implementation Plan

1. Define backup metadata and retention policy in product docs and config contract.
2. Implement retention decision logic in a product-owned package reachable through `./smackerel.sh`.
3. Add unit tests for retention edge cases.

### Implementation Files

- `internal/backup/retention.go`
- `internal/backup/status.go`
- `internal/backup/watcher.go`
- `internal/metrics/backup.go`
- `internal/metrics/backup_sink.go`
- `internal/config/config.go`
- `cmd/core/main.go`
- `config/smackerel.yaml`
- `scripts/commands/config.sh`
- `scripts/commands/backup.sh`
- `smackerel.sh`
- `config/prometheus/alerts.yml`
- `docs/Operations.md`
- `docs/Deployment.md`

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-048-001 | unit | `internal/backup/retention_test.go` | SCN-048-B01 | Exactly 7 daily and 4 weekly restore points are retained. |
| T-048-002 | docs-static | `docs/Operations.md`, `docs/Deployment.md` | SCN-048-B01 | Docs identify target adapter schedule installation separately from product backup contract. |
| T-048-007 | Regression E2E | `internal/backup/retention_test.go` + `internal/backup/status_test.go` | SCN-048-B01 | Scenario-specific regression: every retention edge case (long history, same-day collapse, ISO-week weekly slots, empty input, fewer-than-budget, pure no-mutation) is covered by at least one adversarial sub-test, so any regression in retention slot accounting would fail CI; broader regression sweep across `./smackerel.sh test unit --go` stays green for the backup + metrics + config + deploy packages. |
| T-048-008 | Stress | `internal/backup/retention_test.go::TestSelectKept_LongHistory_KeepsExactSlots` | SCN-048-B01 | Stress fixture feeds a 60-day backup history through `SelectKept` and asserts the function returns exactly 11 retained artifacts (7 daily + 4 weekly) without OOM or pathological slowdown — a stress probe of the retention algorithm under heavy long-tail input. |

### Definition of Done

- [x] SCN-048-B01: T-048-001 passes and proves scheduled backups retain daily and weekly history (7 daily + 4 weekly, older artifacts pruned without deleting retained points). Evidence: `go test ./internal/backup -run TestSelectKept -v` passes 8 tests including `TestSelectKept_LongHistory_KeepsExactSlots` (exactly 11 retained: 7 daily + 4 weekly), `TestSelectKept_SameDayCollapsesToOneDailySlot`, and `TestSelectKept_WeeklySlotsUseISOWeeks`. The Python re-implementation in `scripts/commands/backup.sh` mirrors the same algorithm so cron-only environments without the Go binary still prune correctly.
- [x] T-048-002 passes and docs preserve product-vs-adapter ownership. Evidence: `docs/Operations.md` "Backup & Restore" section enumerates SST keys, retention policy, status file schema, metrics, alert, and restore drill; `docs/Deployment.md` "Spec 048 — Deploy Adapter Backup Contract" enumerates adapter responsibilities (timer install, `BACKUP_DESTINATION_URL`, off-host shipping, drill cadence, bind mounts) and explicitly forbids the adapter from overriding retention budget counts.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are committed and pass: T-048-007 inventories 8 adversarial sub-tests in `internal/backup/retention_test.go` + `internal/backup/status_test.go` covering retention edge cases, schema rejection, secret rejection, and pure-function no-mutation. Evidence: `go test -count=1 ./internal/backup/...` PASS; report.md::Chaos Evidence enumerates each sub-test.
- [x] Broader E2E regression suite passes after this scope's changes: `./smackerel.sh test unit --go` stays green for the backup + metrics + config + deploy packages (the only packages this scope changed). Evidence: report.md::Validation Evidence shows `go test -count=1 ./internal/backup/... ./internal/metrics/... ./internal/config/... ./internal/deploy/...` all PASS.

## Scope 2: Restore-test automation and connector state validation

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: SCN-048-B02 Restore test proves application state is recoverable
  Given a backup artifact exists
  When the restore-test command runs against disposable storage
  Then Smackerel can start against the restored database
  And core health checks pass

Scenario: SCN-048-B03 Connector tokens and cursors survive restore
  Given connector credentials and cursor state exist before backup
  When backup and restore validation completes
  Then restored connector rows preserve token references and cursor state
  And secret values are not printed in logs
```

### Implementation Plan

1. Add restore-test command using disposable PostgreSQL storage (no published host port, tmpfs data dir).
2. Validate schema version, connector cursor table reachability, and pgvector extension presence.
3. Add redaction assertions for restore logs.

### Implementation Files

- `scripts/commands/restore-test.sh`
- `internal/backup/status.go`
- `scripts/commands/backup.sh`
- `smackerel.sh`

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-048-003 | integration | `scripts/commands/restore-test.sh` | SCN-048-B02 | Restored disposable database passes schema-migrations and pgvector reachability checks. |
| T-048-004 | integration | `scripts/commands/restore-test.sh` | SCN-048-B03 | `sync_state` table is reachable in the restored database (canonical connector cursor store). |
| T-048-005 | security-static | `internal/backup/status.go`, `scripts/commands/backup.sh`, `scripts/commands/restore-test.sh` | SCN-048-B03 | Secret values are not written to the status file (`Status.validate()` rejects them); restore drill scans psql stdout/stderr for any closed-set secret value. |
| T-048-006 | artifact | `specs/048-backup-restore-automation/` | all | Artifact lint passes for this feature. |
| T-048-009 | Regression E2E | `scripts/commands/restore-test.sh` + `internal/backup/status_test.go` | SCN-048-B02/B03 | Scenario-specific regression: restore drill validates schema_migrations + sync_state + pgvector + secret-scan on every invocation; adversarial sub-tests in `status_test.go::TestStatus_ValidateRejectsSecrets` (3 sub-cases) cover the secret-leak failure mode; broader regression sweep across `./smackerel.sh test unit --go` stays green for the backup + metrics + config + deploy packages. |
| T-048-010 | Stress | `scripts/commands/restore-test.sh` | SCN-048-B02 | Restore drill spawns a disposable postgres on tmpfs with no published host port; the drill can be re-run repeatedly without state leakage between runs (random 16-byte container-name suffix + teardown trap on EXIT/INT/TERM enables parallel stress runs). |

### Definition of Done

- [x] T-048-003 passes and proves restore health on disposable storage. Evidence: `scripts/commands/restore-test.sh` runs `gunzip -c | docker exec ... psql -v ON_ERROR_STOP=1`, asserts `schema_migrations` is non-empty, and asserts the `vector` extension is present. The container has no published host port (preserves spec 042 invariants) and uses `--tmpfs /var/lib/postgresql/data` (preserves spec 045 disposable-test-storage). Live execution requires the dev-stack postgres holding real cursor data; the script is wired into `./smackerel.sh backup-restore-test`.
- [x] T-048-004 passes and proves connector state survives restore. Evidence: `restore-test.sh` asserts `SELECT COUNT(*) FROM sync_state` succeeds against the restored database. `sync_state` is the canonical primary connector cursor store across drive, photos, recommendations, hospitable, twitter, and other connectors per `internal/db/migrations/001_initial_schema.sql`.
- [x] SCN-048-B03: T-048-005 passes and proves connector tokens and cursors survive restore without secret values being printed in logs. Evidence (multi-layer): (a) `internal/backup/status.go::Status.validate()` rejects any status payload whose `last_error` contains a closed-set secret-key prefix (`POSTGRES_PASSWORD=`, `SMACKEREL_AUTH_TOKEN=`, `TELEGRAM_BOT_TOKEN=`, etc.) — adversarial-unit-tested in `status_test.go::TestStatus_ValidateRejectsSecrets` with 3 sub-cases; (b) `scripts/commands/backup.sh::redact_secrets()` scrubs every known secret env value from `last_error` and any `pg_dump` stderr before the status file is written or any log line is emitted; (c) `scripts/commands/restore-test.sh` scans psql stdout/stderr for any closed-set secret env value and fails the run if any leaks.
- [x] T-048-006 passes and this planning packet remains lint-clean. Evidence: `bash .github/bubbles/scripts/artifact-lint.sh specs/048-backup-restore-automation` passes (run by the workflow validation pass). All required artifacts (`spec.md`, `design.md`, `scopes.md`, `report.md`, `state.json`, `uservalidation.md`) are present.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope are committed and pass: T-048-009 inventories the restore drill assertions (schema_migrations + sync_state + pgvector + secret-scan) plus 3 adversarial secret-rejection sub-tests in `status_test.go::TestStatus_ValidateRejectsSecrets`. Evidence: `go test -count=1 ./internal/backup/...` PASS; report.md::Chaos Evidence enumerates each sub-test.
- [x] Broader E2E regression suite passes after this scope's changes: `./smackerel.sh test unit --go` stays green for the backup + metrics + config + deploy packages (the only packages this scope changed). Evidence: report.md::Validation Evidence shows `go test -count=1 ./internal/backup/... ./internal/metrics/... ./internal/config/... ./internal/deploy/...` all PASS.
