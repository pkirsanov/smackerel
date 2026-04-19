# Bug: BUG-001 — Documentation drift from specs 034-036 implementation

## Classification

- **Type:** Bug (documentation debt from implementation batch)
- **Severity:** HIGH (DC-001), MEDIUM (DC-002, DC-003)
- **Parent Spec:** 032 — Documentation Freshness
- **Source Specs:** 034 (Expense Tracking), 035 (Recipe Enhancements), 036 (Meal Planning)
- **Findings:** DC-001, DC-002, DC-003

## Problem Statement

Specs 034-036 (expense tracking, recipe scaler/cook mode, meal planning) were implemented and merged but the managed documentation was not updated. Three managed docs have drifted:

1. **`docs/Development.md` (DC-001, HIGH):** Missing 2 new packages (`internal/recipe/`, `internal/mealplan/`), missing new source files added to existing packages (`internal/domain/expense.go`, `internal/intelligence/expenses.go` + `vendor_seeds.go`, `internal/api/expenses.go` + `mealplan.go` + `domain.go`, `internal/telegram/recipe_commands.go` + `cook_session.go` + `cook_format.go` + `expenses.go` + `mealplan_commands.go`, `internal/digest/expenses.go`), missing ML sidecar file (`ml/app/receipt_detection.py`), missing prompt contract (`receipt-extraction-v1.yaml`), and missing migration 018. The "Implemented runtime capabilities" summary, Go package table, migration table, prompt contract table, Telegram command list, and API endpoint documentation are all stale.

2. **`docs/Operations.md` (DC-002, MEDIUM):** No operational guidance for expense tracking configuration, meal planning configuration, or recipe features. No troubleshooting entries for expense/meal plan/recipe errors.

3. **`README.md` (DC-003, MEDIUM):** Feature list doesn't mention expense tracking, recipe scaling, cook mode, or meal planning. Architecture diagram doesn't show the new components. Telegram bot command list is stale.

## Root Cause

Documentation updates were not included in the implementation change sets for specs 034-036. The 032 spec's original scopes covered docs up through spec 033 but didn't anticipate the 034-036 batch.

## Reproduction

1. Read `docs/Development.md` → no mention of `internal/recipe/` or `internal/mealplan/` packages
2. Read `docs/Operations.md` → no mention of expense tracking or meal planning
3. Read `README.md` → no feature bullets for expenses, recipe scaling, cook mode, or meal planning
4. Verify the source files exist: `internal/recipe/*.go`, `internal/mealplan/*.go`, `internal/domain/expense.go`, `internal/db/migrations/018_meal_plans.sql`, `config/prompt_contracts/receipt-extraction-v1.yaml`, `ml/app/receipt_detection.py`

## Acceptance Criteria

- [ ] `docs/Development.md` — "Implemented runtime capabilities" list includes expense tracking, recipe scaler/cook mode, and meal planning bullets
- [ ] `docs/Development.md` — Go Packages table includes `internal/recipe/` and `internal/mealplan/` with accurate descriptions
- [ ] `docs/Development.md` — Existing package descriptions updated: `internal/domain/`, `internal/intelligence/`, `internal/api/`, `internal/telegram/`, `internal/digest/` reflect new files
- [ ] `docs/Development.md` — Migration table includes 018 (meal planning tables)
- [ ] `docs/Development.md` — Prompt contract table includes `receipt-extraction-v1.yaml`
- [ ] `docs/Development.md` — ML sidecar file count updated
- [ ] `docs/Operations.md` — Expense tracking configuration and operational guidance section added
- [ ] `docs/Operations.md` — Meal planning configuration and operational guidance section added
- [ ] `docs/Operations.md` — Recipe features (scaler, cook mode) operational guidance section added
- [ ] `docs/Operations.md` — Troubleshooting entries added for expense/meal plan/recipe errors
- [ ] `README.md` — Feature list includes expense tracking, recipe scaling, cook mode, meal planning
- [ ] `README.md` — Architecture diagram updated to show expense/recipe/meal plan components
- [ ] `README.md` — Telegram bot command list updated with new commands
- [ ] No documentation of planned-but-unimplemented features (per spec 032 constraint)
