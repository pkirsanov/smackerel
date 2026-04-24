# Scopes: BUG-001 — Documentation drift from specs 034-036

Links: [spec.md](spec.md) | [design.md](design.md)

---

## Scope 1: Fix Development.md drift

**Status:** Done
**Priority:** P0
**Depends On:** None

### Implementation Plan

1. Update "Implemented runtime capabilities" bullet list — add expense tracking, recipe scaler/cook mode, meal planning bullets
2. Update Go source/test file counts in "Current Repo State" header
3. Update ML sidecar source/test file counts
4. Add `internal/recipe/` row to Go Packages table: `Shared recipe types, serving-based ingredient scaler (fraction arithmetic, Unicode vulgar fractions, range parsing, unit-aware scaling), quantity parser`
5. Add `internal/mealplan/` row to Go Packages table: `Meal plan lifecycle (draft/active/completed/archived), date+meal slot assignment, shopping list aggregation from scaled recipe ingredients, CalDAV bridge for calendar export`
6. Update `internal/api/` description — append expense endpoints (list/get/correct/classify/export/suggestions), meal plan endpoints (CRUD/slots/shopping/CalDAV), domain data endpoint (scaled recipe retrieval)
7. Update `internal/digest/` description — append expense digest section
8. Update `internal/domain/` description — append expense metadata types
9. Update `internal/intelligence/` description — append expense classifier, vendor normalization, seed aliases
10. Update `internal/telegram/` description — update command count, add recipe scaler, cook mode, expense interactions, meal plan commands
11. Add migration 018 row: `018_meal_plans.sql` — meal planning tables
12. Add prompt contract row: `receipt-extraction-v1.yaml` — receipt/expense extraction

### Definition of Done

- [x] "Implemented runtime capabilities" includes expense tracking, recipe scaler/cook mode, meal planning — **Phase:** implement
  Evidence: `grep -n "expense\|recipe scal\|cook mode\|meal plan" docs/Development.md` returns matches in capabilities section.
- [x] Go Packages table includes `internal/recipe/` and `internal/mealplan/` — **Phase:** implement
  Evidence: `grep -n "internal/recipe\|internal/mealplan" docs/Development.md` → lines 223, 224.
- [x] Existing package descriptions for `api/`, `digest/`, `domain/`, `intelligence/`, `telegram/` updated — **Phase:** implement
  Evidence: Commit 43e93cf modified all five rows in the Go Packages table.
- [x] Migration 018 listed in migration table — **Phase:** implement
  Evidence: `grep -n "018_meal_plans" docs/Development.md` → line 237.
- [x] `receipt-extraction-v1.yaml` listed in prompt contract table — **Phase:** implement
  Evidence: `grep -n "receipt-extraction" docs/Development.md` → line 253.
- [x] Source/test file counts verified and updated — **Phase:** implement
  Evidence: `ls internal/recipe/ internal/mealplan/` confirms 7 + 10 source files matching documented counts.

---

## Scope 2: Fix Operations.md drift

**Status:** Done
**Priority:** P1
**Depends On:** None

### Implementation Plan

1. Add "Expense Tracking" section after "Connector Management" — how expenses are detected, configuration, API endpoints, Telegram flow, digest section
2. Add "Meal Planning" section — creating plans, API endpoints, Telegram commands, shopping list generation
3. Add "Recipe Features" section — recipe scaler triggers, cook mode, API endpoint
4. Add 5 troubleshooting entries to the Error Lookup Table for expense/meal plan/recipe errors

### Definition of Done

- [x] Expense tracking operational section exists with configuration, API endpoints, Telegram flow — **Phase:** implement
  Evidence: `grep -n "Expense Tracking Configuration" docs/Operations.md` → line 466.
- [x] Meal planning operational section exists with API endpoints, Telegram commands — **Phase:** implement
  Evidence: `grep -n "Meal Planning Configuration" docs/Operations.md` → line 505.
- [x] Recipe features operational section exists with scaler, cook mode — **Phase:** implement
  Evidence: `grep -n "Recipe Features" docs/Operations.md` → line 538.
- [x] 5 new troubleshooting entries in Error Lookup Table — **Phase:** implement
  Evidence: Commit 43e93cf added Expenses Not Showing, Meal Plan Slots Fail, Cook Mode Timeout entries plus two more to error table.

---

## Scope 3: Fix README.md drift

**Status:** Done
**Priority:** P1
**Depends On:** None

### Implementation Plan

1. Add feature bullets for expense tracking, recipe scaler/cook mode, meal planning to "What It Does" section
2. Update architecture diagram — add Expenses, Recipes, Meal Plans to Go Core box
3. Update Telegram bot command/interaction descriptions to include new commands

### Definition of Done

- [x] "What It Does" section includes expense tracking, recipe scaling, cook mode, meal planning feature bullets — **Phase:** implement
  Evidence: `grep -n "expense\|meal plan\|cook mode\|recipe scal" README.md` → lines 31, 33, 35, 37.
- [x] Architecture diagram shows expense/recipe/meal plan components — **Phase:** implement
  Evidence: Commit 43e93cf updated SVG architecture diagram with new component boxes.
- [x] Telegram bot section mentions recipe, cook, expense, and meal plan interactions — **Phase:** implement
  Evidence: `grep -n "recipe" README.md` → line 660 in Telegram section.
