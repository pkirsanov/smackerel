---
description: How to schedule, dispatch, and record recurring operational upkeep tasks (backup, restore-drill, BCDR-drill, patch-cycle, secret-rotation, flag-cleanup, compliance-sweep). Use when authoring or modifying `config/upkeep-calendar.yaml`, when investigating a missed task, when reading the upkeep ledger, or when adding a new task type.
---

# Upkeep Cadence

## Mental Model

**Upkeep = the recurring, scheduled, *preventive* work that keeps a deployed product from rotting.** Distinct from `stabilize` (diagnoses reliability problems after they appear) and `devops` (executes ops mechanics on demand).

Owned by `bubbles.upkeep` (Treena Lahey).

## The Seven Task Types

| Task | Default cadence | Mode | What it does |
|---|---|---|---|
| `backup` | daily | `upkeep-backup-verify` | T1 (ZFS snapshot) + T2 (host-local restic). Conditionally T3 (USB) + T4 (cloud) based on `OFFSITE_BACKEND` env. Writes ledger entry. |
| `restore-test` | weekly | `upkeep-restore-drill` | Restores latest T2 into isolated namespace, runs product smoke, tears down. Failure blocks next promote. |
| `bcdr-drill` | quarterly | `upkeep-bcdr-drill` | Full DR exercise. Restores entire stack into isolated namespace. Records RTO/RPO actuals. Warns when offsite=local-only. |
| `patch-cycle` | monthly | `upkeep-patch-cycle` | Base-image refresh + dependency audit. Produces candidate via `release-train-cut`. |
| `secret-rotation` | quarterly | `upkeep-secret-rotation` | Rotates managed secrets. Records old/new key-id HASHES (never values, G119). |
| `flag-cleanup-audit` | monthly | `upkeep-flag-cleanup` | Lists flags whose train graduated > 1 cycle. Packets cleanup to `bubbles.train`. |
| `compliance-sweep` | quarterly | `upkeep-compliance-sweep` | Generates `docs/Compliance_Report.md` (G117-G120 evidence). Packets to `bubbles.audit` for cert. |

## `config/upkeep-calendar.yaml` Schema

```yaml
# Per-repo upkeep calendar. Owned by bubbles.upkeep.
version: 1
tasks:
  - id: backup
    cadence: daily              # daily | weekly | monthly | quarterly
    at: "04:30"                 # optional time-of-day hint for scheduler
    retention: "14d"            # how long to keep ledger entries for this task
  - id: restore-test
    cadence: weekly
    at: "sun 05:00"
    blocks_on_failure: [release-train-promote]   # what this task blocks if it fails
  - id: bcdr-drill
    cadence: quarterly
    requires_offsite_backend: true                # WARN if OFFSITE_BACKEND=local-only
  - id: patch-cycle
    cadence: monthly
    at: "first-sun 06:00"
  - id: secret-rotation
    cadence: quarterly
  - id: flag-cleanup-audit
    cadence: monthly
  - id: compliance-sweep
    cadence: quarterly
```

## Ledger Format

`/srv/backups/upkeep-ledger.jsonl` is **append-only** (G117). One JSON object per line:

```json
{
  "task": "backup",
  "repo": "<product>",
  "train": "mvp",
  "sha": "abc12345",
  "tier": "T2",
  "started_at": "2026-06-02T04:30:00Z",
  "finished_at": "2026-06-02T04:32:14Z",
  "outcome": "success",
  "evidence_path": "/srv/backups/restic/<product>/snapshot-abc12345.json"
}
```

`outcome` ∈ `{success, failure, skipped, warning}`.

## Calendar Dispatch (`upkeep-calendar.sh`)

```bash
bash .github/bubbles/scripts/upkeep-calendar.sh /path/to/repo
```

Output: table of tasks with last-run, status (`DUE` or `ok`), next-due ISO timestamp.

The dispatcher reads `upkeep-calendar.yaml` for cadence + `upkeep-ledger.jsonl` for last successful run. Tasks past their cadence window appear as `DUE`. **Run the highest-priority DUE task first**: failure of `backup` blocks `restore-test`; failure of `restore-test` blocks next `release-train-promote`.

## Failure Routing (no in-line fixing)

| Failure type | Packet to | Why |
|---|---|---|
| Backup failed (restic check ≠ 0) | `bubbles.devops` | Operational mechanics |
| Restore-test failed | `bubbles.stabilize` (diagnose) + `bubbles.train` (block promote) | Root cause + safety lock |
| BCDR-drill failed | `bubbles.stabilize` + `bubbles.train` | Full RCA before next promote |
| Secret rotation failed | `bubbles.security` | Possible compromise risk |
| Flag-cleanup overdue | `bubbles.train` | Owner of flag lifecycle |
| Compliance evidence gap | `bubbles.audit` | Certifier owns sign-off |

## Cross-Domain Cooperative Reads (B2 boundary)

`bubbles.upkeep` **MAY** read `config/release-trains.yaml` and knb-side `<product>/<target>/manifest.yaml` to scope `restore-test` and `bcdr-drill` correctly — must restore the train+digest actually deployed, not trunk HEAD.

`bubbles.upkeep` **NEVER** writes to train config or manifest. Writes go through packet to `bubbles.train` or `bubbles.devops`.

## Anti-Patterns

| ❌ Wrong | ✅ Right |
|---|---|
| Running unscheduled "safety drills" | Calendar-driven only; unscheduled runs are ledger noise |
| Storing secret values in the ledger | Only key-id HASHES (G119) |
| Backup destination overlapping with test stack write paths | Strict separation (G115) |
| Restoring into the live prod namespace "to save time" | NEVER. Ephemeral `restore-test` namespace, torn down on exit |
| Treena editing knb manifest directly | Packet to `bubbles.devops` instead |

## See Also

- Skill: `bubbles-backup-bcdr-doctrine` (RTO/RPO, offsite tiers)
- Skill: `bubbles-env-pollution-isolation` (G115 enforcement)
- Instructions: `bubbles-upkeep-operations.instructions.md`
- Agent: `bubbles.upkeep` (Treena Lahey)
- Gates: G112, G113, G114, G115, G116, G117, G118, G119, G120
