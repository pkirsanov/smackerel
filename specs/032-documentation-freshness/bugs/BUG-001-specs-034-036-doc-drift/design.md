# Design: BUG-001 — Documentation drift from specs 034-036

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [032 spec](../../spec.md) | [032 design](../../design.md)
> **Date:** April 19, 2026

---

## Analysis

### Phase 1 — Analyst

All three findings are classified as **bugs** (documentation that should have been updated when the implementation landed). They share the same root cause: specs 034-036 were implemented without updating the managed docs that spec 032 owns. This is a single bug with three affected files, not three independent bugs.

### Phase 2 — UX

The user experience impact is **onboarding and discoverability**:
- A new user reading README.md has no idea Smackerel can track expenses, scale recipes, provide cook mode, or plan meals
- A developer reading Development.md doesn't know `internal/recipe/` or `internal/mealplan/` exist, and thinks the API only has the endpoints listed (missing expenses, meal plans, domain data)
- An operator reading Operations.md has no guidance on configuring or troubleshooting expense tracking or meal planning
- The architecture diagram gives a false impression of the system's scope

### Phase 3 — Design

Exact changes required per file:

---

#### File 1: `docs/Development.md`

**Section: "Current Repo State" → "Implemented runtime capabilities" bullet list**

Add 3 new bullets after the existing list:
- `Expense tracking — receipt/invoice extraction via prompt contract, 7-level classification chain, vendor normalization with seed aliases, Telegram multi-turn corrections, daily digest expense section, CSV/JSON export`
- `Recipe scaler and cook mode — ingredient quantity parsing (fractions, Unicode vulgar fractions, ranges), serving-based scaling, step-by-step Telegram cook mode with timer extraction and navigation`
- `Meal planning — date-range plans with date+meal slots, recipe assignment, shopping list aggregation from scaled ingredients, CalDAV bridge for calendar export`

Update source file counts (currently "130 source files, 131 test files" for Go — will need recount).

**Section: "Go Packages (`internal/`)" table**

Add 2 new rows (alphabetically):
| Package | Purpose |
|---------|---------|
| `internal/mealplan/` | Meal plan lifecycle (draft/active/completed/archived), date+meal slot assignment, shopping list aggregation from scaled recipe ingredients, CalDAV bridge for calendar export |
| `internal/recipe/` | Shared recipe types, serving-based ingredient scaler (fraction arithmetic, Unicode vulgar fractions, range parsing, unit-aware scaling), quantity parser |

Update 5 existing rows to reflect new files:
- `internal/api/` → append: `, expenses (list/get/correct/classify/export/suggestions), meal plans (CRUD/slots/shopping list/CalDAV export), domain data (scaled recipe retrieval)`
- `internal/digest/` → append: `, expense digest section (spending summary, needs-review, suggestions, missing receipts, unusual charges)`
- `internal/domain/` → append: `, expense metadata types (vendor, amount, line items, classification, extraction status)`
- `internal/intelligence/` → append: `, expense classification (7-level rule chain, vendor normalization, seed aliases, reclassification batching, suggestion generation)`
- `internal/telegram/` → update command count from 9 to include recipe/cook/expense/meal-plan commands, add: `, recipe scaler (serving shortcuts), cook mode (step-by-step with timers and navigation), expense interactions (multi-turn corrections, amount prompts), meal plan commands (create/view/add/shop/cook-from-plan)`

**Section: "Database Migrations" table**

Add 1 new row:
| 018 | `018_meal_plans.sql` | Meal planning (spec 036): `meal_plans` and `meal_plan_slots` tables with status lifecycle and date range constraints |

**Section: "Prompt Contracts" table**

Add 1 new row:
| Receipt Extraction | `receipt-extraction-v1.yaml` | `domain-extraction` | Extract structured expense/receipt data (vendor, amount, line items, payment method, category) from receipt text, invoice text, or OCR output |

**Section: "Current Repo State" header**

Update ML sidecar file counts (currently "16 source files, 18 test files" — add `ml/app/receipt_detection.py`).

---

#### File 2: `docs/Operations.md`

**New section: "Expense Tracking" (after "Connector Management", before "Troubleshooting")**

Contents:
- How expenses are detected (receipt-extraction prompt contract triggers on email/bill/note/media content types)
- Configuration in `config/smackerel.yaml`: `expenses` block (categories, business_vendors, classification thresholds)
- Vendor normalization: system seeds + user corrections
- API endpoints: `GET /api/expenses`, `GET /api/expenses/{id}`, `PATCH /api/expenses/{id}` (corrections), `GET /api/expenses/export` (CSV/JSON), `POST /api/expenses/{id}/classify`, `POST /api/expenses/suggestions/{id}/accept|dismiss`
- Telegram flow: receipt photo → extraction → confirmation → correction flow
- Daily digest: expense section shows spending summary, needs-review items, suggestions

**New section: "Meal Planning" (after "Expense Tracking")**

Contents:
- Creating meal plans: date range, slot assignment
- API endpoints: `POST/GET /api/meal-plans`, `GET/DELETE /api/meal-plans/{id}`, slot management, `GET /api/meal-plans/{id}/shopping-list`, CalDAV export
- Telegram flow: `/plan` command for interactive plan creation
- Shopping list generation from scaled ingredients

**New section: "Recipe Features" (after "Meal Planning")**

Contents:
- Recipe scaler: natural language serving triggers ("4 servings", "for 6", "scale to 8")
- Cook mode: step-by-step Telegram walkthrough with timer extraction
- API endpoint: `GET /api/artifacts/{id}/domain?servings=N` for scaled recipe data
- Telegram commands for recipe interaction

**Section: "Troubleshooting" → "Error Lookup Table"**

Add entries:
| `expense extraction failed: content too short` | Content below 20 chars for receipt extraction | Verify the captured content contains receipt/invoice text |
| `expense amount_missing: true` | Receipt extraction couldn't find a monetary amount | Use Telegram or API to manually correct the expense amount |
| `meal plan dates_check violation` | End date is before start date | Provide end_date ≥ start_date when creating a plan |
| `recipe scaler: unparseable quantity` | Ingredient quantity couldn't be parsed for scaling | Quantity remains unscaled; original text preserved |
| `CalDAV export failed` | Calendar bridge couldn't generate iCalendar output | Check meal plan has valid date range and at least one slot |

---

#### File 3: `README.md`

**Section: "What It Does" feature icons list**

Add new feature bullets (or update existing ones to mention new capabilities):
- Add expense tracking bullet: tracks purchases, receipts, invoices with automatic categorization
- Add recipe scaler/cook mode bullet: scale recipes by servings, step-by-step cook mode via Telegram
- Add meal planning bullet: weekly meal plans, shopping lists, calendar export

**Section: "Architecture" diagram**

Update the Go Core box to include:
```
│                   │ • Expenses      │
│                   │ • Recipes       │
│                   │ • Meal Plans    │
```

**Section: Telegram Bot command list**

Update to include new commands beyond the current 9 (`/find`, `/concept`, `/person`, `/lint`, `/digest`, `/done`, `/status`, `/recent`, `/rate`). Add recipe, cook, expense, and meal plan interaction descriptions.

---

### Phase 4 — Plan

**Single bug packet** — all 3 findings share the same root cause (034-036 implementation batch landed without doc updates) and the same fix pattern (add missing content to each managed doc). A single scope with 3 sub-tasks (one per file) is appropriate.

#### Scope: Fix docs drift from specs 034-036

**Priority:** P0 (HIGH finding on Development.md, the developer reference)

**Implementation Order:**
1. `docs/Development.md` — largest delta, most technical, provides content for the other two
2. `docs/Operations.md` — operational guidance for the new features
3. `README.md` — user-facing feature discovery

**Depends On:** None (all source files already exist and are committed)

**DoD:**
- [ ] All acceptance criteria from spec.md pass
- [ ] `grep -rn 'recipe\|mealplan\|expense\|cook.mode' docs/Development.md` returns hits for all 4 features
- [ ] `grep -rn 'expense\|meal.plan\|recipe' docs/Operations.md` returns hits for all 3 features
- [ ] `grep -rn 'expense\|recipe\|cook\|meal.plan' README.md` returns feature-list hits (not just code example mentions)
- [ ] No documentation of planned-but-unimplemented features
- [ ] File counts in Development.md verified against actual `find` output
