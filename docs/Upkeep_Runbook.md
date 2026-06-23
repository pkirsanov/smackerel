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
  (restic). It composes the product pg_dump layer by calling `./smackerel.sh backup`
  for the database dump + retention (see **Backup: Two-Layer Split** below).
- `restore-test.sh` — restore latest T2 into isolated namespace, smoke
- `bcdr-drill.sh` — full DR exercise + RTO/RPO measurement
- `patch-cycle.sh`, `secret-rotation.sh`, `flag-cleanup-audit.sh`, `compliance-sweep.sh`

## Backup: Two-Layer Split

Backup responsibility is split across two layers; neither layer does the other's
job. The product script (`scripts/commands/backup.sh`) explicitly scopes
scheduling, volume backup, and offsite shipping **out** (see its header comment).

| Layer | Owner | Entrypoint | Responsibilities |
|-------|-------|------------|------------------|
| **Product** | this repo | `./smackerel.sh backup` (`scripts/commands/backup.sh`) | `pg_dump \| gzip` of smackerel core state, gzip-integrity validation, retention (7 daily + 4 weekly), status JSON for the `SmackerelBackupStale` alert, secret redaction |
| **knb / target adapter** | knb overlay | `<knb-repo>/smackerel/home-lab/backup.sh` | Scheduling (systemd/cron timer), Compose volume backup, off-host shipping (restic / `BACKUP_DESTINATION_URL`) |

The knb hook calls `./smackerel.sh backup` for the database dump + retention,
then performs the volume backup and offsite shipping it owns.

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
