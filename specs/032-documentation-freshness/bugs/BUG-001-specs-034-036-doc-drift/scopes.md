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

### DoD

- [x] "Implemented runtime capabilities" includes expense tracking, recipe scaler/cook mode, meal planning — **Phase:** implement
- [x] Go Packages table includes `internal/recipe/` and `internal/mealplan/` — **Phase:** implement
- [x] Existing package descriptions for `api/`, `digest/`, `domain/`, `intelligence/`, `telegram/` updated — **Phase:** implement
- [x] Migration 018 listed in migration table — **Phase:** implement
- [x] `receipt-extraction-v1.yaml` listed in prompt contract table — **Phase:** implement
- [x] Source/test file counts verified and updated — **Phase:** implement

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

### DoD

- [x] Expense tracking operational section exists with configuration, API endpoints, Telegram flow — **Phase:** implement
- [x] Meal planning operational section exists with API endpoints, Telegram commands — **Phase:** implement
- [x] Recipe features operational section exists with scaler, cook mode — **Phase:** implement
- [x] 5 new troubleshooting entries in Error Lookup Table — **Phase:** implement

---

## Scope 3: Fix README.md drift

**Status:** Done
**Priority:** P1
**Depends On:** None

### Implementation Plan

1. Add feature bullets for expense tracking, recipe scaler/cook mode, meal planning to "What It Does" section
2. Update architecture diagram — add Expenses, Recipes, Meal Plans to Go Core box
3. Update Telegram bot command/interaction descriptions to include new commands

### DoD

- [x] "What It Does" section includes expense tracking, recipe scaling, cook mode, meal planning feature bullets — **Phase:** implement
- [x] Architecture diagram shows expense/recipe/meal plan components — **Phase:** implement
- [x] Telegram bot section mentions recipe, cook, expense, and meal plan interactions — **Phase:** implement
