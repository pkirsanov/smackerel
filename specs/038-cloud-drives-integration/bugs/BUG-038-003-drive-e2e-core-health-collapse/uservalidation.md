# User Validation: BUG-038-003 Drive E2E core health collapse

## Checklist

### Stable observability fixture

- [x] **What:** Drive observability reconciliation completes without stopping or exhausting the real disposable core.
  - **Steps:** Run the live observability regression on a fresh disposable stack.
  - **Expected:** Metric families, counter deltas, and database rows reconcile, and core remains healthy.
  - **Verify:** Use the repository E2E CLI and inspect the successor health assertion.
  - **Evidence:** `report.md#test-evidence`

### Serialized package continuity

- [x] **What:** Drive tests share the parent-owned stack safely in package order.
  - **Steps:** Run the predecessor, observability, successor, then the full serialized Drive package.
  - **Expected:** No core/network disappearance and no cascade failures.
  - **Verify:** Run the focused neighbor selector and Drive package E2E.
  - **Evidence:** `report.md#test-evidence`
