# Report: BUG-032-001 — Documentation drift from specs 034-036

Links: [spec.md](spec.md) | [scopes.md](scopes.md)

---

## Summary

Specs 034 (expense tracking), 035 (recipe scaler/cook mode), and 036 (meal planning) shipped runtime code without updating managed documentation (DC-001 HIGH, DC-002 MEDIUM, DC-003 MEDIUM). Three managed docs drifted: `docs/Development.md` was missing `internal/recipe/` and `internal/mealplan/` package rows, migration 018, and the receipt-extraction prompt contract; `docs/Operations.md` had no operational guidance for the new features; `README.md` feature bullets did not mention expenses, recipe scaling, cook mode, or meal planning.

Fix landed in commit 43e93cf updating all three documents.

## Completion Statement

All 13 DoD items across 3 scopes verified by grep against the live documentation files. Source artifacts (`internal/recipe/`, `internal/mealplan/`, migration 018, receipt-extraction prompt contract, `ml/app/receipt_detection.py`) confirmed present. No documentation of unimplemented features.

## Test Evidence

```
$ grep -n "internal/recipe\|internal/mealplan\|018_meal_plans\|receipt-extraction" docs/Development.md | head -10
223:| `internal/mealplan/` | Meal planning calendar — plan store, service (lifecycle, overlap, copy), shopping list bridge (reuses RecipeAggregator + ScaleIngredients), CalDAV calendar sync bridge |
224:| `internal/recipe/` | Shared recipe types, serving scaler, kitchen fraction formatter, quantity parsing (extracted from list aggregator for reuse by scaler and cook mode) |
237:| 018 | `018_meal_plans.sql` | Meal planning (spec 036): `meal_plans` + `meal_plan_slots` tables with date range, lifecycle status, slot constraints |
253:| Receipt Extraction | `receipt-extraction-v1.yaml` | `domain-extraction` | Extract structured receipt/invoice data (vendor, date, amount, currency, tax, line items, payment method) |
$ wc -l docs/Development.md
415 docs/Development.md
```

```
$ grep -n "Expense Tracking\|Meal Planning\|Recipe Features" docs/Operations.md
466:## Expense Tracking Configuration
505:## Meal Planning Configuration
538:## Recipe Features
$ wc -l docs/Operations.md
663 docs/Operations.md
```

### Validation Evidence

```
$ grep -n "expense\|meal plan\|cook mode\|recipe scal" README.md
31:<img src="assets/icons/feature-local.svg" width="20" height="20" alt="">&ensp;**Tracks expenses** — receipts from email, photo, or PDF with automatic classification and CSV export
33:<img src="assets/icons/feature-local.svg" width="20" height="20" alt="">&ensp;**Scales recipes** to any serving count with kitchen-friendly fractions
35:<img src="assets/icons/feature-local.svg" width="20" height="20" alt="">&ensp;**Cook mode** — step-by-step Telegram walkthrough for any recipe
37:<img src="assets/icons/feature-local.svg" width="20" height="20" alt="">&ensp;**Plans meals** — weekly meal plans with automatic shopping list generation
$ wc -l README.md docs/Operations.md
  868 README.md
  663 docs/Operations.md
```

```
$ ls -la internal/recipe/ | head -10
total 48
drwxr-xr-x  2 philipk philipk 4096 Apr 18 08:34 .
drwxr-xr-x 26 philipk philipk 4096 Apr 23 21:53 ..
-rw-r--r--  1 philipk philipk 1746 Apr 20 21:25 fractions.go
-rw-r--r--  1 philipk philipk 2247 Apr 18 18:45 fractions_test.go
-rw-r--r--  1 philipk philipk 5493 Apr 20 21:25 quantity.go
-rw-r--r--  1 philipk philipk 5805 Apr 18 18:45 quantity_test.go
-rw-r--r--  1 philipk philipk 1225 Apr 18 15:16 scaler.go
-rw-r--r--  1 philipk philipk 5723 Apr 18 18:45 scaler_test.go
-rw-r--r--  1 philipk philipk 1663 Apr 18 08:31 types.go
$ wc -l internal/recipe/scaler.go internal/recipe/fractions.go
  46 internal/recipe/scaler.go
  78 internal/recipe/fractions.go
 124 total
```

### Audit Evidence

```
$ ls -la ml/app/receipt_detection.py internal/db/migrations/018_meal_plans.sql config/prompt_contracts/receipt-extraction-v1.yaml
-rw-r--r-- 1 philipk philipk 3056 Apr 18 15:16 config/prompt_contracts/receipt-extraction-v1.yaml
-rw-r--r-- 1 philipk philipk 1574 Apr 18 15:16 internal/db/migrations/018_meal_plans.sql
-rw-r--r-- 1 philipk philipk 3531 Apr 18 18:45 ml/app/receipt_detection.py
```

```
$ grep -c "internal/recipe\|internal/mealplan\|expense" docs/Development.md
10
$ wc -l docs/Development.md docs/Operations.md README.md
  415 docs/Development.md
  663 docs/Operations.md
  868 README.md
 1946 total
```

Doc updates align with shipped code surface; no unimplemented features documented (per spec 032 constraint).
