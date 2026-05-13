# User Validation: Backup and Restore Automation

## Checklist

- [x] Planning packet covers 7 daily and 4 weekly retention.
- [x] Planning packet covers disposable restore-test automation.
- [x] Planning packet covers connector token and cursor preservation.
- [x] Planning packet separates product backup contract from target adapter scheduling.
- [x] Retention policy implemented and unit-tested. Evidence: `go test ./internal/backup -run TestSelectKept` passes 6 retention tests including adversarial `TestSelectKept_LongHistory_KeepsExactSlots` (exactly 11 retained) and `TestSelectKept_SameDayCollapsesToOneDailySlot`.
- [x] Backup script writes a redacted JSON status file and applies retention pruning. Evidence: `scripts/commands/backup.sh` sources required `BACKUP_*` SST keys, pipes `pg_dump | gzip`, prunes via the same algorithm the Go unit tests cover, and writes the status file via atomic `<file>.tmp → rename`. Status payload runs through `redact_secrets()` covering 14 closed-set secret env vars before any line is emitted.
- [x] Prometheus alert is backed by a real metric. Evidence: `config/prometheus/alerts.yml` `SmackerelBackupStale` uses `(time() - smackerel_backup_last_success_unixtime) > 90000`; the spec 049 alert-contract test (`TestMonitoringAlertsContract_LiveFile`) confirms the metric is emitted by `internal/metrics/backup.go`.
- [x] Restore drill is wired and reachable via the standard CLI. Evidence: `./smackerel.sh backup-restore-test --backup-file <path>` invokes `scripts/commands/restore-test.sh`, which spawns a tmpfs-only postgres with no published host port, restores the artifact through `psql`, and asserts `schema_migrations` non-empty, `sync_state` reachable, and `vector` extension present.
- [x] Operations and Deployment docs reflect the new contract. Evidence: `docs/Operations.md` "Backup & Restore" section documents SST keys, retention algorithm, status JSON, metrics, alert, and operator workflow; `docs/Deployment.md` "Spec 048 — Deploy Adapter Backup Contract" enumerates adapter responsibilities and forbids overriding retention slot counts.

## Operator Acceptance

To accept the spec 048 delivery, the operator should:

1. Run `./smackerel.sh up` to start the dev stack.
2. Run `./smackerel.sh backup` and confirm `backups/smackerel-*.sql.gz` and `backups/.backup-status.json` are produced.
3. Open `backups/.backup-status.json` and confirm the schema matches the documented payload (no secret values visible).
4. Run `./smackerel.sh backup-restore-test` and confirm the drill prints `Restore drill PASSED`.
5. Hit `http://127.0.0.1:40001/metrics` and confirm `smackerel_backup_last_success_unixtime` is non-zero, `smackerel_backup_size_bytes` matches the artifact, and `smackerel_backup_runs_total{status="success"}` is `1`.
6. Confirm `SmackerelBackupStale` evaluates to inactive in Prometheus immediately after the backup (alert window is 25h).
