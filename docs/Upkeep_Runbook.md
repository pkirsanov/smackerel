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
- `backup.sh` — PostgreSQL dump + Compose volume backup via offsite-restic.sh
- `restore-test.sh` — restore latest T2 into isolated namespace, smoke
- `bcdr-drill.sh` — full DR exercise + RTO/RPO measurement
- `patch-cycle.sh`, `secret-rotation.sh`, `flag-cleanup-audit.sh`, `compliance-sweep.sh`

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
