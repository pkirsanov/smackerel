# Scopes: Backup and Restore Automation

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope 1: Backup schedule and retention contract

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-048-B01 Scheduled backups retain daily and weekly history
  Given Smackerel is configured for deployment operation
  When backup automation runs over time
  Then the system keeps 7 daily backups and 4 weekly backups
  And backups older than the retention window are pruned without deleting retained points
```

### Implementation Plan

1. Define backup metadata and retention policy in product docs and config contract.
2. Implement retention decision logic in a product-owned script or command reachable through `./smackerel.sh`.
3. Add unit tests for retention edge cases.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-048-001 | unit | backup retention tests | SCN-048-B01 | Exactly 7 daily and 4 weekly restore points are retained. |
| T-048-002 | docs-static | deployment docs | SCN-048-B01 | Docs identify target adapter schedule installation separately from product backup contract. |

### Definition of Done

- [ ] T-048-001 passes and proves retention behavior.
- [ ] T-048-002 passes and docs preserve product-vs-adapter ownership.

## Scope 2: Restore-test automation and connector state validation

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
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

### Implementation Plan

1. Add restore-test command or workflow using disposable PostgreSQL storage.
2. Validate schema version, core health, connector token references, and cursor state.
3. Add redaction assertions for restore logs.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-048-003 | integration | restore-test workflow | SCN-048-B02 | Restored disposable database passes health checks. |
| T-048-004 | integration | restore-test workflow | SCN-048-B03 | Connector token references and cursors survive restore. |
| T-048-005 | security-static | restore logs | SCN-048-B03 | Secret values are not printed. |
| T-048-006 | artifact | spec folder | all | Artifact lint passes for this feature. |

### Definition of Done

- [ ] T-048-003 passes and proves restore health on disposable storage.
- [ ] T-048-004 passes and proves connector state survives restore.
- [ ] T-048-005 passes and proves restore logs do not expose secrets.
- [ ] T-048-006 passes and this planning packet remains lint-clean.
