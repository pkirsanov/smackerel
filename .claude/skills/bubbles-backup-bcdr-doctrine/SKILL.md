---
description: Backup tier model, RTO/RPO definitions, restore-drill cadence requirements, and `OFFSITE_BACKEND` swap contract. Use when defining backup strategy for a new product, when choosing/changing offsite backend, when authoring a BCDR plan, or when interpreting backup ledger entries.
---

# Backup + BCDR Doctrine

## Four Tiers (Encapsulated via `OFFSITE_BACKEND` env)

| Tier | Protects against | Backend | Where engine writes | Frequency |
|---|---|---|---|---|
| **T1** ZFS snapshot | Accidental delete, app corruption | ZFS on `tank/<dataset>` | `tank/<dataset>@upkeep-<ts>` | Daily |
| **T2** Host-local restic | Single-volume corruption, restore-test isolation | restic to `/srv/backups/restic/<product>` | Daily |
| **T3** Near-line removable | RAID controller failure, ransomware, host wipe | restic to mounted USB drive (rotated weekly, physically separated) | Daily (when drive mounted) |
| **T4** True offsite cloud | Fire, theft, total host loss | restic to B2/R2/Wasabi/S3/rclone-any | Daily (when configured) |

## `OFFSITE_BACKEND` Env Contract

Single env var, set in per-product `<product>/<target>/params.yaml`:

```bash
# Today (RAID5 only, T1+T2):
OFFSITE_BACKEND=local-only

# When USB drives arrive:
OFFSITE_BACKEND=restic_usb:/mnt/usb-backup-1

# When cloud offsite added later:
OFFSITE_BACKEND=restic_b2:bucket=mybucket
OFFSITE_BACKEND=restic_r2:bucket=mybucket
OFFSITE_BACKEND=restic_wasabi:bucket=mybucket
OFFSITE_BACKEND=restic_s3:bucket=mybucket
OFFSITE_BACKEND=restic_rclone:remote=myremote
```

Engine (`knb/shared/upkeep/offsite-restic.sh`) parses the prefix (`restic_<type>:<config>`) and dispatches to the right restic backend. **Swapping tiers requires zero product changes.**

## RTO / RPO Definitions

| Term | Meaning | How measured |
|---|---|---|
| **RTO** Recovery Time Objective | Max time from declaring incident → product back online | `bcdr-drill` ledger entry: `finished_at − incident_declared_at` |
| **RPO** Recovery Point Objective | Max data loss tolerated (in time) | Backup cadence — daily = 24h RPO ceiling |

Per-product RTO/RPO declared in `docs/BCDR_Plan.md`:

```markdown
| Product | RTO | RPO | Notes |
|---|---|---|---|
| <product-a> | 4h | 24h | Daily backup + manual cutover |
| <product-b> | 12h | 24h | Single-user; longer RTO acceptable |
| <product-c> | 2h | 1h | Hot path; needs hourly backup eventually |
| <product-d> | 1h | 1h | Trading-adjacent; eventually needs sub-hourly |
```

## Drill Cadence

| Drill | Cadence | Mode | Pass criteria |
|---|---|---|---|
| Restore-test | Weekly | `upkeep-restore-drill` | Restore from T2 into isolated namespace; smoke check exit 0 |
| BCDR-drill | Quarterly | `upkeep-bcdr-drill` | Full stack restore from highest available tier; RTO ≤ declared; smoke + integration checks exit 0 |
| Compliance-sweep | Quarterly | `upkeep-compliance-sweep` | G117-G120 evidence collected; report generated; audit sign-off received |

## Gate Behavior

- **G112 backup-evidence-required** — Every train in `active` phase shipping to non-`none` slot MUST have a recent (≤ cadence) successful backup ledger entry. Blocks `release-train-promote`.
- **G113 restore-drill-evidence** — Every product MUST have a successful restore-test within the cadence. Failed restore-test blocks next `release-train-promote` on that product.
- **G114 bcdr-evidence** — Quarterly drill required for trains in `prod` slot.
- **G116 offsite-backup-required-for-prod-trains** — Behavior depends on `release-trains.yaml`:
  - `offsite_required: false` (default) → **warns** when `OFFSITE_BACKEND=local-only`
  - `offsite_required: true` → **blocks** promote when `OFFSITE_BACKEND=local-only`
  - Operator flips this switch when USB drives or cloud offsite arrive. **No code change needed.**

## Initial Reality Pattern (single-host RAID, no offsite yet)

- T1 + T2 run daily; protect against software corruption + single-disk failure (RAID5 absorbs 1 disk loss).
- T3 + T4 skipped with WARN entries in ledger.
- G114 BCDR drill: WARN only — true BCDR requires offsite.
- G116: WARN only (`offsite_required: false`).

When USB arrives: operator mounts drive, sets `OFFSITE_BACKEND=restic_usb:/mnt/usb-backup-1`, re-runs adapter `apply.sh`. T3 starts populating on next nightly backup.

When cloud arrives: operator sets `OFFSITE_BACKEND=restic_b2:...`, sets `offsite_required: true`, re-runs apply. G114/G116 flip from warn → block.

## Anti-Patterns

| ❌ Wrong | ✅ Right |
|---|---|
| Custom per-product backup scripts that bypass the engine | One engine; per-product `backup.sh` declares what to dump |
| Storing offsite credentials in the repo | Operator's password manager only; engine reads from runtime env |
| Skipping restore-test because "backup completed" | Backup completion ≠ restore success; G113 requires actual restore |
| Restoring into prod namespace to "save time" | Ephemeral `restore-test` namespace only (G115) |
| Treating `local-only` as production-ready | At minimum WARN; with `offsite_required: true` it blocks |

## See Also

- Skill: `bubbles-upkeep-cadence` (task scheduling)
- Skill: `bubbles-env-pollution-isolation` (G115)
- knb skill: `knb-offsite-backup-restic` (backend implementation)
- Agent: `bubbles.upkeep` (Treena Lahey)
- Gates: G112, G113, G114, G116
