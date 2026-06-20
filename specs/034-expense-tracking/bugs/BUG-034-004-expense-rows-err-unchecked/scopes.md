# Scopes: BUG-034-004 Expense DB-iteration loops swallow mid-stream errors

## Scope 1: Harden all 11 expense row-iteration loops + adversarial regression

**Status:** [x] Done (in-process; consolidated commit + parent recert deferred to end-of-sweep bubbles.devops pass)
**Depends On:** none

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: BUG-034-004 — Expense reads fail loud on mid-stream DB errors

  Scenario: List summary surfaces a mid-iteration cursor error
    Given the expense currency-summary cursor drops mid-iteration
    When scanExpenseCurrencySummaries iterates it
    Then it returns a non-nil error wrapping the cursor error
    And it does not return a truncated summary slice

  Scenario: List data surfaces a per-row scan error
    Given the expense list cursor fails to scan row N
    When scanExpenseListItems iterates it
    Then it returns a non-nil error instead of silently dropping row N

  Scenario: Export decode surfaces a corrupt JSONB row
    Given an expense export row whose expense JSON is invalid
    When decodeExportExpenseRow decodes it
    Then it returns a non-nil unmarshal error instead of continue-skipping the row

  Scenario: Adversarial — removing the rows.Err() check fails the test
    Given the rows.Err() check is removed from scanExpenseCurrencySummaries
    When the regression suite runs
    Then TestScanExpenseCurrencySummaries_PropagatesRowsErr FAILS
```

### Implementation Plan

1. `internal/api/expenses.go`: add `rowScanner` interface; hoist `expenseCurrencySummary`
   + `expenseListItem` to package level; add `exportExpenseRow`; add
   `scanExpenseCurrencySummaries`, `scanExpenseListItems`, `decodeExportExpenseRow`
   helpers (each propagating Scan/Unmarshal/`rows.Err()`).
2. `internal/api/expenses.go` `List`: call the two collection helpers; on error →
   `writeExpenseError(500)`.
3. `internal/api/expenses.go` `Export`: distinct-currency loop → clean 500 on
   error; CSV-stream loop → `decodeExportExpenseRow` + `panic(http.ErrAbortHandler)`
   on decode/iteration error.
4. `internal/digest/expenses.go`: all 6 loops → `Scan`→`return nil, err` +
   `rows.Err()`→`return nil, err`.
5. `internal/intelligence/expenses.go`: candidate loop → `Scan`→`return 0, err` +
   `rows.Err()`→`return 0, err`.
6. `internal/api/expenses_rowserr_test.go` (new): `fakeExpenseRows` + 8 adversarial
   tests mirroring `internal/list/harden_test.go`.
7. Adversarial proof: reintroduce the defect, capture RED, restore, capture GREEN.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Unit | `unit` | `internal/api/expenses_rowserr_test.go` | `rows.Err()` + `Scan` + `Unmarshal` propagation for the 3 extracted helpers (adversarial) | `./smackerel.sh test unit --go --go-run 'ScanExpense\|DecodeExportExpenseRow'` | No |
| Unit | `unit` | `internal/api/expenses_test.go` | Existing expense handler tests still pass (no collateral regression) | `./smackerel.sh test unit --go --go-run 'Expense'` | No |
| Unit | `unit` | `internal/digest/expenses_test.go` | Existing digest tests still pass (Assemble change does not break them) | `./smackerel.sh test unit --go` (digest) | No |
| Build/Vet | `unit` | `./smackerel.sh check` | api + digest + intelligence compile and vet clean | `./smackerel.sh check` | No |
| Lint | `unit` | `./smackerel.sh lint` | golangci-lint clean on changed files | `./smackerel.sh lint` | No |

### Definition of Done — 3-Part Validation

- [x] Root cause confirmed and documented (missing `rows.Err()` + silent per-row
  `continue`, deviating from the ~20× sibling convention)
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      See report.md → "Before Fix (code-analysis reproduction)" — pre-fix source of all 11 loops captured verbatim.
      ```
- [x] Fix implemented across all 11 loops + 3 files
   - Raw output evidence:
      ```
      See report.md → "Files Modified This Session" + "After Fix — ./smackerel.sh check" (build+vet exit 0).
      ```
- [x] Adversarial regression case exists and FAILS if the bug were reintroduced
   - Raw output evidence:
      ```
      See report.md → "Adversarial regression proof (RED)" — rows.Err() check removed → TestScanExpenseCurrencySummaries_PropagatesRowsErr FAIL.
      ```
- [x] Post-fix regression test PASSES
   - Raw output evidence:
      ```
      See report.md → "After Fix — regression suite GREEN".
      ```
- [x] Regression tests contain no silent-pass bailout patterns
   - Raw output evidence:
      ```
      See report.md → "Bailout-pattern scan" (grep over expenses_rowserr_test.go shows no `return` early-exit on failure conditions).
      ```
- [x] All existing tests pass (no regressions) — api, digest, intelligence packages
   - Raw output evidence:
      ```
      See report.md → "After Fix — ./smackerel.sh test unit --go (api/digest/intelligence)".
      ```
- [x] `./smackerel.sh check` and `./smackerel.sh lint` green
   - Raw output evidence:
      ```
      See report.md → "After Fix — ./smackerel.sh check" and "After Fix — ./smackerel.sh lint".
      ```
- [x] bug.md status updated to "Fixed"
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      $ grep -n 'Status:' specs/034-expense-tracking/bugs/BUG-034-004-expense-rows-err-unchecked/bug.md
      3:**Status:** Fixed (in-process; consolidated commit + parent recert deferred to end-of-sweep bubbles.devops)
      ```
- [ ] Consolidated commit of the spec-034 code change + parent recertification —
  **Deferred to end-of-sweep `bubbles.devops` pass** (NOT forced here per the sweep
  contract; the certified parent spec's commit-state gate is expected to be dirty
  until then). Owner: `bubbles.devops`.

⚠️ No E2E/live-deploy item: this is a unit-tier code-correctness fix with no runtime
or deployment surface change. The adversarial unit regression is the binding proof.

### Honest Gap Summary

All engineering work is complete and verified in-process: 11 loops hardened to the
repo convention, 3 testable helpers extracted, 8 adversarial regression tests green,
and a real RED-on-reintroduction proof captured. The only open item is the
consolidated commit + parent-spec recertification, which the sweep contract
explicitly defers to a single end-of-sweep `bubbles.devops` pass — it is recorded as
a routed follow-up, not forced here.
