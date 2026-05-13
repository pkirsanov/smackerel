# Feature: Backup and Restore Automation

## Status

Done - delivered via full-delivery workflow. Runtime owns the dump
contract, retention pruning, status JSON, metrics, and restore drill;
deploy adapter overlay owns scheduling and off-host shipping.

## Review Findings

- D-019: Backup scheduling and retention are not yet defined as a generic deployment contract. **Resolved** — see `docs/Deployment.md` "Spec 048 — Deploy Adapter Backup Contract" and `config/smackerel.yaml` `backup:` SST section.
- V-019: Restore-test automation and evidence are missing from the readiness plan. **Resolved** — `./smackerel.sh backup-restore-test` drives the drill against disposable storage and asserts schema, connector cursor reachability, and absence of secret leakage.

## Outcome Contract

**Intent:** Make Smackerel backup, retention, and restore verification repeatable enough for any deployment.

**Success Signal:** Backup artifacts, retention, restore testing, and connector cursor/token preservation are defined, automated, and verified with a disposable restore path before deployment readiness is claimed.

**Hard Constraints:**

- Backups must preserve PostgreSQL data and any connector state required to resume ingestion safely.
- Restore tests must run against disposable storage, not the persistent dev store.
- Secret material must not be printed in logs or committed to artifacts.
- Target adapter timer installation and storage paths remain outside Smackerel; Smackerel owns the product contract and verification behavior.

**Failure Condition:** Operators have no automated backup schedule, no retention contract, no restore-test command, or no proof that connector tokens and cursors survive restore.

## Requirements

- **FR-048-001:** Define backup artifacts, schedule, and retention policy: 7 daily and 4 weekly retained backups.
- **FR-048-002:** Define restore-test command or workflow that restores into disposable storage.
- **FR-048-003:** Verify connector tokens and cursors survive backup and restore without leaking secret values.
- **FR-048-004:** Document which parts are product-owned and which schedule installation details belong to target adapters.
- **FR-048-005:** Add tests that fail when backup contents omit required tables or restore validation does not run.

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-048-B01 Scheduled backups retain daily and weekly history
  Given Smackerel is configured for deployment operation
  When backup automation runs over time
  Then the system keeps 7 daily backups and 4 weekly backups
  And backups older than the retention window are pruned without deleting retained points

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

## Product Principle Alignment

This spec supports Principle 9, Design For Restart, Not Perfection, by making recovery a tested product behavior rather than an operator guess. It supports Principle 8 by requiring restore evidence.

## Non-Goals

- Implementing target-host systemd timers inside Smackerel source.
- Building a full disaster-recovery product UI.
- Changing connector schemas unless restore validation proves a schema gap.
