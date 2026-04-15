## Checklist

### [Bug Fix] [BUG-003] Engine god-object file split
- [x] **What:** Split engine.go (1256 LOC, 17 methods) into 4 domain files + slim engine.go
  - **Steps:**
    1. Verify all intelligence package tests pass after split
    2. Verify engine.go ≤150 LOC
    3. Verify no consumer import changes required
  - **Expected:** Same behavior, reduced file churn, follows package convention
  - **Verify:** `./smackerel.sh test unit`
  - **Evidence:** report.md#scope-1
  - **Notes:** Bug fix for [BUG-003] — pure file move, no behavior change
