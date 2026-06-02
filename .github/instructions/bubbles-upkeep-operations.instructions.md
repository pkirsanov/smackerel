---
applyTo: "**"
---

# Upkeep Operations Policy (NON-NEGOTIABLE)

> This instructions file is loaded into every agent context for repos that have
> adopted upkeep. It encodes the rules from
> [`bubbles-upkeep-cadence`](../skills/bubbles-upkeep-cadence/SKILL.md) and
> [`bubbles-backup-bcdr-doctrine`](../skills/bubbles-backup-bcdr-doctrine/SKILL.md).

## The Rule

**All recurring operational hygiene (backup, restore-drill, BCDR-drill, patch-cycle, secret-rotation, flag-cleanup, compliance-sweep) flows through `bubbles.upkeep` on a calendar-driven schedule. Ad-hoc unscheduled drills are forbidden — they are workload and ledger noise.**

## What This Means For Agents

### Calendar Discipline

- Tasks only run when the calendar declares them due. `bash .github/bubbles/scripts/upkeep-calendar.sh` lists due tasks.
- DO NOT invoke `upkeep-*` workflow modes unless the corresponding task is `DUE` in the calendar output.
- Exception: `force-drill <product>` action exists for legitimate ops events (post-incident, pre-launch). MUST be justified in ledger entry.

### Ledger Discipline (G117)

- `/srv/backups/upkeep-ledger.jsonl` is **append-only**. Never rewrite, never delete entries.
- Every upkeep task execution writes ONE ledger entry with: `{task, repo, train, sha, started_at, finished_at, outcome, evidence_path}`.
- Failed tasks STILL write a ledger entry (`outcome: failure`). Silent failures are forbidden.

### Restore Drill Discipline (G115)

- Restore drills MUST use an **ephemeral, isolated namespace**. Never the prod stack.
- The namespace MUST be torn down on exit, success OR failure.
- If teardown fails, packet to `bubbles.devops` for manual cleanup. NEVER abandon a half-restored namespace.

### Backup Tier Discipline

- T1 (ZFS) + T2 (host-local restic) MUST always run on schedule.
- T3 (USB) + T4 (cloud) run based on `OFFSITE_BACKEND` env in `<product>/<target>/params.yaml`. Engine handles backend swap transparently.
- When `OFFSITE_BACKEND=local-only`: T3/T4 SKIPPED with WARN ledger entry. G114/G116 emit warnings (NOT blocking) by default.
- When `offsite_required: true` in `release-trains.yaml`: G116 BLOCKS promote if `OFFSITE_BACKEND=local-only`. Operator flips this when USB/cloud arrives.

### Compliance Discipline (G117-G120)

- `compliance-sweep` quarterly task generates `docs/Compliance_Report.md` with evidence for G117 (audit-trail), G118 (retention policy), G119 (secret rotation), G120 (PII classification).
- `bubbles.upkeep` gathers evidence. `bubbles.audit` signs off. Separation is enforced — Treena cannot certify her own evidence.

### Secret Rotation Discipline (G119)

- Ledger entry MUST contain old/new key-id HASHES (e.g., `sha256:<first-12-chars>`). NEVER the secret values themselves.
- If a secret leaks into the ledger, the ledger is compromised and MUST be considered tainted. Rotate immediately and roll the ledger forward with a corruption notice (NEVER delete past entries).

## Forbidden Patterns

| ❌ Forbidden | ✅ Required |
|---|---|
| Running unscheduled "safety drills" | Calendar-driven only |
| Rewriting ledger entries (e.g., to fix typos) | Append a correction entry; never edit past |
| Storing secret values in ledger | Key-id hashes only |
| Restoring into prod namespace "to save time" | Ephemeral namespace, torn down on exit |
| Treena editing knb manifest directly | Packet to `bubbles.devops` |
| `upkeep-calendar.sh --force-due all` | No such flag; honor the calendar |
| Backup destinations under repo tree | Backups belong on `/srv/backups/` (knb-side) or `OFFSITE_BACKEND` only |

## Pre-Push Wiring

The repo's pre-push hook MUST call:

```bash
bash .github/bubbles/scripts/env-pollution-scan.sh "$(pwd)" || exit 1
```

`env-pollution-scan.sh` exit codes:
- `0` — clean
- `1` — test code writes to forbidden prod surface (commit blocked)

## See Also

- Skill: `bubbles-upkeep-cadence`
- Skill: `bubbles-backup-bcdr-doctrine`
- Skill: `bubbles-env-pollution-isolation`
- Agent: `bubbles.upkeep` (Treena Lahey)
- Gates: G112, G113, G114, G115, G116, G117, G118, G119, G120
