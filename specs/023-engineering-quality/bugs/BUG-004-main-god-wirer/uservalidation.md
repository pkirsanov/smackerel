## Checklist

### [Bug Fix] [BUG-004] main.go god-wirer extraction
- [x] **What:** Extract connector wiring and service construction from main.go (724 LOC) into connectors.go and services.go
  - **Steps:**
    1. Verify application builds after extraction
    2. Verify main.go ≤200 LOC
    3. Verify all cmd/core tests pass
  - **Expected:** Same startup behavior, reduced file churn, cleaner separation
  - **Verify:** `./smackerel.sh build && ./smackerel.sh test unit`
  - **Evidence:** report.md#scope-1
  - **Notes:** Bug fix for [BUG-004] — pure file extraction, no behavior change
