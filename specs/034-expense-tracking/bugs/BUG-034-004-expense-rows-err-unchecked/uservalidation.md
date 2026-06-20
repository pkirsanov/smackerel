# User Validation: BUG-034-004 Expense row-iteration error handling

## Checklist

### [Bug Fix] BUG-034-004 Expense reads fail loud on mid-stream DB errors

- [x] **What:** Expense summary/list/export and digest reads no longer silently
  truncate on a mid-stream Postgres error; they return an error (API → HTTP 500 or
  aborted download; digest → section omitted) instead of a confidently-wrong total.
  - **Steps:**
    1. Run the adversarial regression suite: `./smackerel.sh test unit --go --go-run 'ScanExpense|DecodeExportExpenseRow' --verbose`
    2. Confirm all helper tests pass (rows.Err() + Scan + Unmarshal propagation).
    3. Confirm `./smackerel.sh check` and `./smackerel.sh lint` are green.
  - **Expected:** Regression suite green; on an injected mid-stream `rows.Err()` the
    extracted helpers return a non-nil error; success-path output unchanged.
  - **Verify:** `./smackerel.sh test unit --go --go-run 'ScanExpense|DecodeExportExpenseRow'`
  - **Evidence:** report.md#after-fix--regression-suite-green-expenses_rowserr_testgo
  - **Notes:** Bug fix for BUG-034-004. Unit-tier code-correctness fix; no live deploy.

- [x] **What:** Success-path expense output is byte-for-byte unchanged (no behavior
  change when there is no DB error).
  - **Steps:**
    1. Run existing expense + digest tests: `./smackerel.sh test unit --go --go-run 'Expense'`
    2. Confirm no collateral regression.
  - **Expected:** All pre-existing expense/digest tests still pass.
  - **Verify:** `./smackerel.sh test unit --go --go-run 'Expense'`
  - **Evidence:** report.md#after-fix--smackerelsh-test-unit---go-api--digest--intelligence--no-collateral-regression
  - **Notes:** Bug fix for BUG-034-004.
