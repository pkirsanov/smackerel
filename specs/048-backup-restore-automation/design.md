# Design: Backup and Restore Automation

## Current Truth

The readiness review identified that backup presence is not enough. Smackerel needs a product-owned backup and restore verification contract, while target adapters own timer installation and target storage paths.

## Proposed Design

### Backup Contract

- Define the database and connector-state content required in each backup.
- Define artifact naming and metadata: source SHA, config bundle reference, timestamp, database schema version, and checksum.
- Define retention: 7 daily and 4 weekly backups.

### Restore Test

- Add a restore-test command or script reachable through the repo CLI.
- Restore into disposable PostgreSQL storage.
- Start the minimum runtime surface needed to verify health and data shape.
- Validate connector token references and cursor state without printing secret values.

### Target Adapter Boundary

- Smackerel documents product-owned backup commands and restore verification.
- Target adapters install timers and wire target-specific storage paths.

## Test Strategy

| Test ID | Type | Purpose |
|---------|------|---------|
| T-048-001 | unit | Retention policy keeps 7 daily and 4 weekly backups. |
| T-048-002 | integration | Restore into disposable PostgreSQL and pass health validation. |
| T-048-003 | integration | Connector cursor and token-reference rows survive restore. |
| T-048-004 | security-static | Restore logs redact secret values. |
| T-048-005 | artifact | Artifact lint passes for this feature. |

## Risk Controls

- Never run restore tests against persistent dev storage.
- Do not include raw secret values in backup metadata or logs.
- Keep scheduling and target-path details out of Smackerel implementation.
