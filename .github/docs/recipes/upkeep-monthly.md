# Recipe: Upkeep — Monthly Operator Checklist

Operator-facing recipe for the recurring operational hygiene work owned by `bubbles.upkeep` (Treena Lahey).

Most of this runs automatically via systemd timer on knb-side `_shared/upkeep`. This checklist is for the human-in-the-loop review.

## What Runs Automatically (No Operator Action)

| Task | Cadence | What it does |
|---|---|---|
| `backup` | Daily 04:30 | T1 (ZFS) + T2 (host-local restic) for every product. T3/T4 if `OFFSITE_BACKEND` configured. |
| `restore-test` | Weekly Sunday 05:00 | Restores latest T2 into isolated namespace, smokes, tears down. |
| `patch-cycle` | Monthly first Sunday 06:00 | Base-image refresh + dependency audit per product. |
| `flag-cleanup-audit` | Monthly | Lists overdue flags. |

## Monthly Operator Review (15 minutes)

### 1. Check the upkeep ledger

```bash
ssh <home-lab-host>
cat /srv/backups/upkeep-ledger.jsonl | jq -r 'select(.outcome != "success") | "\(.finished_at) \(.task) \(.repo) \(.outcome)"' | tail -50
```

Any non-`success` entries from the past month → triage:
- `failure` → packet to right owner (devops/stabilize/security)
- `warning` → review whether to escalate
- `skipped` → check why (usually `OFFSITE_BACKEND=local-only` for T3/T4 — expected today)

### 2. Run calendar status per repo

```bash
cd /path/to/<product-a> && bash .github/bubbles/scripts/upkeep-calendar.sh
cd /path/to/<product-b> && bash .github/bubbles/scripts/upkeep-calendar.sh
cd /path/to/<product-c> && bash .github/bubbles/scripts/upkeep-calendar.sh
cd /path/to/<product-d> && bash .github/bubbles/scripts/upkeep-calendar.sh
```

Any tasks showing `DUE` that should have run → check timer status:

```bash
ssh <home-lab-host> systemctl --user status knb-upkeep.timer
```

### 3. Run flag-cleanup audits

```bash
cd /path/to/<each-repo> && bash .github/bubbles/scripts/release-train-flag-audit.sh
```

For each overdue flag listed:
- Confirm the train is genuinely graduated (in `frozen` or `retired` phase)
- Open a cleanup spec via `bubbles.train` packet:
  ```bash
  ./<repo>.sh release flag-cleanup-spec <flag-name>
  ```

### 4. Verify offsite status (preparing for USB drives)

Today (`OFFSITE_BACKEND=local-only`):
```bash
ssh <home-lab-host>
df -h /srv/backups        # check usage
zfs list -t snapshot | grep upkeep-  | tail -20    # confirm recent snapshots
```

When USB drives arrive:
```bash
ssh <home-lab-host>
mount /mnt/usb-backup-1                                          # confirm mount
# Edit knb/<product>/home-lab/params.yaml: OFFSITE_BACKEND=restic_usb:/mnt/usb-backup-1
# Re-run knb adapter apply for each product to pick up new env
```

## Quarterly (Every 3 Months)

In addition to the monthly checklist:

### BCDR drill

```bash
# Pick one product per quarter; rotate through all 4.
./<repo>.sh upkeep bcdr-drill
```

Records RTO/RPO actuals. With `OFFSITE_BACKEND=local-only` today, this emits a WARN — it's still useful (proves restore works) but it does NOT prove BCDR (true BCDR requires offsite). Document this caveat in `docs/BCDR_Plan.md`.

### Secret rotation

```bash
./<repo>.sh upkeep secret-rotation
```

Rotates managed secrets per the repo's secret inventory. Records old/new key-id HASHES in ledger.

### Compliance sweep

```bash
./<repo>.sh upkeep compliance-sweep
```

Generates `docs/Compliance_Report.md` with evidence for G117-G120. Packets to `bubbles.audit` for sign-off.

## When Things Go Wrong

| Symptom | First action |
|---|---|
| Daily backup failed | `ssh <home-lab-host> journalctl --user -u knb-upkeep.service --since today` |
| Restore-test failed | `bubbles.upkeep` packets to `bubbles.stabilize`; check `restore-test` namespace teardown |
| BCDR drill failed (RTO exceeded) | RCA spec; consider scaling backup tier or pre-staging restore image |
| Patch-cycle introduced regression | `bubbles.train` rollback on affected slot; open hotfix spec |
| Secret rotation failed | DO NOT proceed; packet to `bubbles.security` immediately — possible compromise indicator |

## See Also

- Skill: `bubbles-upkeep-cadence`
- Skill: `bubbles-backup-bcdr-doctrine`
- Recipe: `release-train-lifecycle.md`
- Agent: `bubbles.upkeep`
