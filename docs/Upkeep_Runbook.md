# Smackerel Upkeep Runbook

Owned by `bubbles.upkeep` (Treena Lahey).

## RTO / RPO

| Metric | Target | Notes |
|--------|--------|-------|
| RTO | 12h | Single-user; longer RTO acceptable |
| RPO | 24h | Daily backup cadence |

## Scheduled Tasks

See [`config/upkeep-calendar.yaml`](../config/upkeep-calendar.yaml).

Per-task hooks live in `<knb-repo>/smackerel/home-lab/`:
- `backup.sh` — knb-side scheduling + Compose volume backup + offsite shipping
  (restic). On the artifact-only home-lab target (the product source/CLI is NOT
  present) it performs its OWN inline `pg_dump | gzip` with a `gzip -t` integrity
  gate + restic retention prune, rather than calling `./smackerel.sh backup` (see
  **Backup: Two-Layer Split** below).
- `restore-test.sh` — restore latest T2 into isolated namespace, smoke
- `bcdr-drill.sh` — full DR exercise + RTO/RPO measurement
- `patch-cycle.sh`, `secret-rotation.sh`, `flag-cleanup-audit.sh`, `compliance-sweep.sh`

## Backup: Two-Layer Split

Backup responsibility is split across two layers; neither layer does the other's
job. The product script (`scripts/commands/backup.sh`) explicitly scopes
scheduling, volume backup, and offsite shipping **out** (see its header comment).

| Layer | Owner | Entrypoint | Responsibilities |
|-------|-------|------------|------------------|
| **Product** | this repo | `./smackerel.sh backup` (`scripts/commands/backup.sh`), on a host where the product CLI is present (dev / operator host) | `pg_dump \| gzip` of smackerel core state, gzip-integrity validation, retention (7 daily + 4 weekly), status JSON for the `SmackerelBackupStale` alert, secret redaction |
| **knb / target adapter** | knb overlay | `<knb-repo>/smackerel/home-lab/backup.sh` | Scheduling (systemd/cron timer); on the artifact-only home-lab target, its OWN inline `pg_dump \| gzip` + `gzip -t` integrity gate + restic retention prune (the product CLI is not deployed there); Compose volume backup (NATS); off-host shipping (restic / `BACKUP_DESTINATION_URL`) |

On the artifact-only home-lab **target**, the knb hook does NOT call
`./smackerel.sh backup` — the product source/CLI is not deployed there (images +
config bundle only). Instead it performs its own inline `pg_dump | gzip` with a
`gzip -t` integrity gate and a restic retention prune, then the Compose volume
backup (NATS) and offsite shipping it owns. The dump lands under `/srv/backups`
(`0700`) and ships restic-encrypted, so it is encrypted at rest; the inline path
never writes a status JSON and never echoes dump contents, so the product layer's
status-file secret redaction has no leak surface to cover there. The product-layer
`./smackerel.sh backup` remains the backup path on the **dev / operator host**,
where the product CLI is present and emits the `SmackerelBackupStale` status JSON
and the secret-redacted dump.

## Gate Implementation Boundary — Restore-Test → Promote (G113)

The restore-test task declares `blocks_on_failure: [ release-train-promote ]` in
[`config/upkeep-calendar.yaml`](../config/upkeep-calendar.yaml), and Gate **G113**
(`restore_drill_evidence_gate`, registry `.github/bubbles/registry/gates.yaml`)
requires `release-train-promote` to verify a recent successful `restore-test`
ledger entry. Like the backup split above, this gate is implemented across two
layers — the product owns the drill, the knb engine plus `bubbles.train`'s promote
operation own the ledger read and the actual block.

| Layer | Owner | Provides |
|-------|-------|----------|
| **Product** | this repo | The restore drill itself: `scripts/commands/restore-test.sh` (run via `./smackerel.sh backup-restore-test`, spec 048) restores the latest backup into a disposable postgres, runs the schema/cursor/redaction assertions, and returns the pass/fail result that the `restore-test` ledger entry is built from. |
| **knb / `bubbles.train`** | knb overlay | The knb upkeep engine (`knb/shared/upkeep/upkeep-engine.sh`) schedules the drill and records its result into the upkeep ledger. During `release-train-promote` (a `bubbles.train` operation), the G113 check reads that ledger and refuses the promote when no recent successful `restore-test` entry exists. |

In short: this repo can prove a backup restores, but it does **not** read the
ledger or block promotes. The promote-blocking enforcement lives in the knb engine
and `bubbles.train`'s promote gate.

## What Gets Backed Up

- PostgreSQL dump (smackerel core state)
- NATS JetStream durable streams
- Per-train config bundles (already in registry; redundant copy)
- Connector state (bookmarks, browser history, twitter archive — already read-only, no backup needed)

## See Also

- [`Release_Trains.md`](Release_Trains.md)
- BCDR plan (knb-side): `<knb-repo>/docs/BCDR_Plan.md`
- Framework skill: `bubbles-upkeep-cadence`, `bubbles-backup-bcdr-doctrine`
- Bubbles agent: `bubbles.upkeep`
