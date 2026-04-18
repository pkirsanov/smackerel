# Scopes: 036 Meal Planning Calendar

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

## Summary Table

| # | Scope | Priority | Depends On | Surfaces | Status |
|---|-------|----------|-----------|----------|--------|
| 01 | Config & Migration | P0 | — | Config, PostgreSQL | Not Started |
| 02 | Plan Store & Service | P0 | 01 | Go Core, PostgreSQL | Not Started |
| 03 | Plan API Endpoints | P1 | 02 | REST API | Not Started |
| 04 | Telegram Plan Commands | P1 | 02, 03 | Telegram | Not Started |
| 05 | Shopping List Bridge | P1 | 02 | Go Core, Spec 028 List Framework | Not Started |
| 06 | Plan Copy & Templates | P2 | 02, 03 | REST API, Telegram | Not Started |
| 07 | CalDAV Calendar Sync | P2 | 02 | Go Core, CalDAV Connector (Spec 003) | Not Started |
| 08 | Auto-Complete Lifecycle | P2 | 01, 02 | Scheduler | Not Started |

## Dependency Graph

```
01-config-migration ──┬──▶ 02-plan-store ──┬──▶ 03-plan-api ──┬──▶ 04-telegram
                      │                    │                   │
                      │                    │                   └──▶ 06-copy-templates
                      │                    │
                      │                    ├──▶ 05-shopping-bridge
                      │                    │
                      │                    └──▶ 07-caldav-sync
                      │                    │
                      └────────────────────┴──▶ 08-auto-complete
```

---

## Scope 01: Config & Migration

**Status:** Not Started
**Priority:** P0
**Depends On:** None
**Spec Refs:** BS-013, UC-001, UC-004, design §3, design §9

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-001 — Meal planning config section parsed from smackerel.yaml
  Given smackerel.yaml contains a meal_planning section with enabled, default_servings,
        meal_types, meal_times, calendar_sync, auto_complete_past_plans, and auto_complete_cron
  When the Go config loader parses the configuration
  Then MealPlanConfig is populated with all fields matching the YAML values

Scenario: SCN-036-002 — Config generate emits meal planning env vars
  Given the meal_planning section exists in smackerel.yaml
  When ./smackerel.sh config generate runs
  Then config/generated/dev.env and config/generated/test.env contain
       MEAL_PLANNING_ENABLED, MEAL_PLANNING_DEFAULT_SERVINGS,
       MEAL_PLANNING_MEAL_TYPES, MEAL_PLANNING_MEAL_TIME_BREAKFAST,
       MEAL_PLANNING_MEAL_TIME_LUNCH, MEAL_PLANNING_MEAL_TIME_DINNER,
       MEAL_PLANNING_MEAL_TIME_SNACK, MEAL_PLANNING_CALENDAR_SYNC,
       MEAL_PLANNING_AUTO_COMPLETE, and MEAL_PLANNING_AUTO_COMPLETE_CRON

Scenario: SCN-036-003 — Fail-loud on missing required config when enabled
  Given MEAL_PLANNING_ENABLED=true but MEAL_PLANNING_DEFAULT_SERVINGS is unset
  When the service starts
  Then it exits with a fatal error identifying the missing variable

Scenario: SCN-036-004 — Migration 018 creates meal_plans and meal_plan_slots tables
  Given the database has migrations up to 017
  When migration 018_meal_plans.sql is applied
  Then table meal_plans exists with columns id, title, start_date, end_date, status,
       created_at, updated_at and CHECK constraints on dates and status
  And table meal_plan_slots exists with columns id, plan_id, slot_date, meal_type,
      recipe_artifact_id, servings, batch_flag, notes, created_at
  And UNIQUE constraint on (plan_id, slot_date, meal_type) is enforced
  And FK from meal_plan_slots.plan_id to meal_plans.id with ON DELETE CASCADE exists
  And FK from meal_plan_slots.recipe_artifact_id to artifacts.id exists
  And indexes idx_meal_plans_status, idx_meal_plans_dates, idx_meal_plan_slots_plan,
      idx_meal_plan_slots_date, idx_meal_plan_slots_recipe exist

Scenario: SCN-036-005 — Configurable meal times stored in config (BS-013)
  Given smackerel.yaml has meal_planning.meal_times.dinner set to "19:00"
  When the config is loaded
  Then MealPlanConfig.MealTimes["dinner"] equals "19:00"
```

### Implementation Plan

**Files to create:**
- `internal/db/migrations/018_meal_plans.sql` — meal_plans and meal_plan_slots tables with indexes and constraints
- `internal/mealplan/types.go` — Plan, Slot, PlanWithSlots, PlanStatus types with JSON tags

**Files to modify:**
- `config/smackerel.yaml` — Add meal_planning section
- `scripts/commands/config.sh` — Emit MEAL_PLANNING_* env vars during config generate
- `internal/config/config.go` — Add MealPlanConfig struct and parsing with fail-loud validation

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-01-01 | Unit | `internal/config/config_test.go` | SCN-036-001 | Parse meal planning config struct |
| T-01-02 | Integration | `tests/integration/config_generate_test.go` | SCN-036-002 | Config generate emits MEAL_PLANNING_* env vars |
| T-01-03 | Unit | `internal/config/config_test.go` | SCN-036-003 | Fail-loud on missing required config |
| T-01-04 | Integration | `tests/integration/migration_test.go` | SCN-036-004 | Migration 018 creates tables with constraints and indexes |
| T-01-05 | Unit | `internal/config/config_test.go` | SCN-036-005 | Configurable meal times parsed correctly |
| T-01-06 | Regression E2E | `tests/e2e/mealplan_config_test.go` | SCN-036-001, SCN-036-004 | Live stack config load and migration verification |

### Definition of Done

- [ ] `config/smackerel.yaml` contains `meal_planning:` section with enabled, default_servings, meal_types, meal_times, calendar_sync, auto_complete_past_plans, auto_complete_cron
- [ ] `scripts/commands/config.sh` emits all `MEAL_PLANNING_*` env vars to `config/generated/dev.env` and `config/generated/test.env`
- [ ] `internal/config/config.go` defines `MealPlanConfig` struct with fail-loud validation for all required fields
- [ ] Migration `018_meal_plans.sql` creates `meal_plans` and `meal_plan_slots` tables with all indexes, constraints, and FKs
- [ ] Go types `Plan`, `Slot`, `PlanWithSlots`, `PlanStatus` defined in `internal/mealplan/types.go`
- [ ] All 5 Gherkin scenarios pass
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes

---

## Scope 02: Plan Store & Service

**Status:** Not Started
**Priority:** P0
**Depends On:** 01
**Spec Refs:** UC-001, UC-003, BS-001, BS-003, BS-005, BS-009, design §4

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-006 — Create meal plan with date range (BS-001)
  Given a plan title "Week of Apr 20" with start_date 2026-04-20 and end_date 2026-04-26
  When the plan is created via PlanService.CreatePlan
  Then a plan exists with status "draft", the provided title, and correct date boundaries
  And the plan ID is a valid ULID

Scenario: SCN-036-007 — Plan end_date must be >= start_date
  Given a plan creation request with end_date 2026-04-19 and start_date 2026-04-20
  When PlanService.CreatePlan is called
  Then a validation error is returned with message "end_date must be on or after start_date"

Scenario: SCN-036-008 — Assign recipe to date+meal slot (UC-001)
  Given a draft plan spanning Apr 20-26 and a recipe artifact "Pasta Carbonara"
  When PlanService.AddSlot is called for 2026-04-20, meal_type "dinner", 4 servings
  Then a slot exists with the correct plan_id, slot_date, meal_type, recipe_artifact_id, and servings 4

Scenario: SCN-036-009 — Unique slot constraint prevents double-booking
  Given a slot exists for plan P on 2026-04-20 with meal_type "dinner"
  When another recipe is assigned to plan P on 2026-04-20 with meal_type "dinner"
  Then a 409 conflict error is returned with existing slot details

Scenario: SCN-036-010 — Slot date must be within plan date range
  Given a plan spanning Apr 20-26
  When a slot is added for Apr 27
  Then a validation error is returned with code MEAL_PLAN_SLOT_OUT_OF_RANGE

Scenario: SCN-036-011 — Plan status lifecycle: draft → active → completed → archived
  Given a draft plan
  When the plan is activated, then completed, then archived
  Then each status transition succeeds and updated_at is refreshed

Scenario: SCN-036-012 — Forbidden status transitions rejected
  Given a completed plan
  When a transition to "draft" is attempted
  Then a 422 error is returned with message "cannot transition from completed to draft"
  And transitions from archived to active, archived to draft, and draft to completed are also rejected

Scenario: SCN-036-013 — Overlap detection on activation (BS-009)
  Given an active plan for Apr 20-26
  When a draft plan for Apr 23-29 is activated
  Then a 409 overlap warning is returned indicating 4 overlapping days
  And the conflicting plan ID and title are included in the response

Scenario: SCN-036-014 — Deleting a plan cascades to slots
  Given a plan with 5 assigned slots
  When the plan is deleted via PlanService.DeletePlan
  Then all 5 slots are also deleted via ON DELETE CASCADE
  And the plan no longer appears in plan listings

Scenario: SCN-036-015 — Query plan slots by date (UC-003, BS-003)
  Given an active plan with "Pasta Carbonara" assigned to 2026-04-20 dinner
  When PlanService.QueryByDate is called for 2026-04-20 with meal_type "dinner"
  Then the response contains the slot with recipe artifact ID and 4 servings

Scenario: SCN-036-016 — Query returns empty for unplanned date (BS-005)
  Given an active plan where 2026-04-23 has no dinner assigned
  When PlanService.QueryByDate is called for 2026-04-23 with meal_type "dinner"
  Then the response indicates no meal planned for that date and meal type

Scenario: SCN-036-017 — Batch slot creation for repeating recipes
  Given a draft plan spanning Apr 20-26
  When PlanService.AddBatchSlots is called for Apr 20-23, meal_type "breakfast",
       recipe "Overnight Oats", 2 servings
  Then 4 slots are created (one per day) with batch_flag=true on all
```

### Implementation Plan

**Files to create:**
- `internal/mealplan/store.go` — PostgreSQL store with Create/Get/List/Update/Delete for plans and AddSlot/UpdateSlot/DeleteSlot/GetSlotsByDate/BatchAddSlots
- `internal/mealplan/store_test.go` — Unit tests with mock DB
- `internal/mealplan/service.go` — Business logic: CreatePlan, AddSlot, lifecycle transitions, overlap detection, QueryByDate, BatchAddSlots
- `internal/mealplan/service_test.go` — Unit tests for service layer

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-02-01 | Unit | `internal/mealplan/store_test.go` | SCN-036-006 | Create plan with date range |
| T-02-02 | Unit | `internal/mealplan/store_test.go` | SCN-036-007 | Date validation rejects end < start |
| T-02-03 | Unit | `internal/mealplan/store_test.go` | SCN-036-008 | Assign recipe to slot |
| T-02-04 | Unit | `internal/mealplan/store_test.go` | SCN-036-009 | Unique slot constraint returns 409 |
| T-02-05 | Unit | `internal/mealplan/service_test.go` | SCN-036-010 | Slot date range validation |
| T-02-06 | Unit | `internal/mealplan/service_test.go` | SCN-036-011 | Valid lifecycle transitions |
| T-02-07 | Unit | `internal/mealplan/service_test.go` | SCN-036-012 | Forbidden transitions rejected |
| T-02-08 | Unit | `internal/mealplan/service_test.go` | SCN-036-013 | Overlap detection returns 409 |
| T-02-09 | Integration | `tests/integration/mealplan_store_test.go` | SCN-036-014 | CASCADE delete on live DB |
| T-02-10 | Unit | `internal/mealplan/service_test.go` | SCN-036-015 | Query slots by date returns match |
| T-02-11 | Unit | `internal/mealplan/service_test.go` | SCN-036-016 | Query returns empty for unplanned date |
| T-02-12 | Unit | `internal/mealplan/store_test.go` | SCN-036-017 | Batch slot creation |
| T-02-13 | Regression E2E | `tests/e2e/mealplan_model_test.go` | SCN-036-006, SCN-036-011 | Plan creation and lifecycle on live stack |

### Definition of Done

- [ ] `internal/mealplan/store.go` implements full CRUD for plans and slots against PostgreSQL
- [ ] `internal/mealplan/service.go` implements CreatePlan, AddSlot, UpdateSlot, DeleteSlot, BatchAddSlots, lifecycle transitions, overlap detection, QueryByDate
- [ ] Status lifecycle enforced: draft→active→completed→archived; all forbidden transitions rejected with 422
- [ ] Overlap detection returns 409 with conflicting plan details on draft→active
- [ ] Batch slot creation creates one slot per day with batch_flag=true
- [ ] Slot date range validated against plan boundaries
- [ ] All 12 Gherkin scenarios pass
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes

---

## Scope 03: Plan API Endpoints

**Status:** Not Started
**Priority:** P1
**Depends On:** 02
**Spec Refs:** UC-001, UC-002, UC-003, UC-004, UC-005, UX-7.1–UX-7.12, design §7

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-018 — POST /api/meal-plans creates a draft plan (UC-001)
  Given valid plan data with title "Week of Apr 20", start_date "2026-04-20", end_date "2026-04-26"
  When POST /api/meal-plans is called
  Then HTTP 201 is returned with a draft plan object including id, title, dates, status, and empty slots array

Scenario: SCN-036-019 — POST /api/meal-plans/{id}/slots assigns a recipe (UC-001)
  Given an existing plan and a valid recipe artifact ID
  When POST /api/meal-plans/{id}/slots is called with slot_date, meal_type, recipe_artifact_id, servings
  Then HTTP 201 is returned with the created slot object including resolved recipe title

Scenario: SCN-036-020 — GET /api/meal-plans/{id} returns plan with all slots
  Given a plan with 3 assigned slots
  When GET /api/meal-plans/{id} is called
  Then HTTP 200 is returned with the plan object and all 3 slots including recipe titles

Scenario: SCN-036-021 — PATCH /api/meal-plans/{id} activates plan with overlap check
  Given a draft plan and an existing active plan with overlapping dates
  When PATCH /api/meal-plans/{id} with status "active" is called
  Then HTTP 409 is returned with overlap details including conflicting plan ID, title, and overlapping day count

Scenario: SCN-036-022 — DELETE /api/meal-plans/{id} cascades to slots
  Given a plan with 5 slots
  When DELETE /api/meal-plans/{id} is called
  Then HTTP 204 is returned and the plan and all slots are removed

Scenario: SCN-036-023 — GET /api/meal-plans/query returns slot for date+meal (UC-003)
  Given an active plan with "Thai Green Curry" on Tuesday dinner
  When GET /api/meal-plans/query?date=2026-04-21&meal=dinner is called
  Then HTTP 200 is returned with the plan context and slot containing recipe title and servings

Scenario: SCN-036-024 — API validation: invalid meal_type returns 422
  Given a plan and a slot request with meal_type "brunch" (not in configured meal_types)
  When POST /api/meal-plans/{id}/slots is called
  Then HTTP 422 is returned with code MEAL_PLAN_INVALID_MEAL_TYPE

Scenario: SCN-036-025 — API validation: recipe artifact not found returns 422
  Given a slot request referencing a non-existent recipe artifact ID
  When POST /api/meal-plans/{id}/slots is called
  Then HTTP 422 is returned with code MEAL_PLAN_RECIPE_NOT_FOUND

Scenario: SCN-036-026 — All API endpoints require auth token
  Given no auth token in the request header
  When any /api/meal-plans endpoint is called
  Then HTTP 401 is returned
```

### Implementation Plan

**Files to create:**
- `internal/api/mealplan.go` — REST handlers for all 12 endpoints from design §7
- `internal/api/mealplan_test.go` — Unit tests for request validation, response shapes, error codes

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-03-01 | Unit | `internal/api/mealplan_test.go` | SCN-036-018 | POST creates draft plan |
| T-03-02 | Unit | `internal/api/mealplan_test.go` | SCN-036-019 | POST creates slot with recipe |
| T-03-03 | Unit | `internal/api/mealplan_test.go` | SCN-036-020 | GET returns plan with all slots |
| T-03-04 | Unit | `internal/api/mealplan_test.go` | SCN-036-021 | PATCH activation with overlap returns 409 |
| T-03-05 | Unit | `internal/api/mealplan_test.go` | SCN-036-022 | DELETE cascades and returns 204 |
| T-03-06 | Unit | `internal/api/mealplan_test.go` | SCN-036-023 | GET query by date+meal |
| T-03-07 | Unit | `internal/api/mealplan_test.go` | SCN-036-024 | Invalid meal_type returns 422 |
| T-03-08 | Unit | `internal/api/mealplan_test.go` | SCN-036-025 | Missing recipe returns 422 |
| T-03-09 | Unit | `internal/api/mealplan_test.go` | SCN-036-026 | Missing auth returns 401 |
| T-03-10 | Regression E2E | `tests/e2e/mealplan_api_test.go` | SCN-036-018, SCN-036-019, SCN-036-023 | Full API plan CRUD and query on live stack |

### Definition of Done

- [ ] All 12 REST endpoints from design §7 implemented in `internal/api/mealplan.go`
- [ ] Request validation for title, dates, meal_type, recipe_artifact_id, servings, status transitions
- [ ] Error response format matches existing Smackerel API pattern with error codes from design §7.3
- [ ] Auth middleware applied to all meal plan endpoints
- [ ] All 9 Gherkin scenarios pass
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes

---

## Scope 04: Telegram Plan Commands

**Status:** Not Started
**Priority:** P1
**Depends On:** 02, 03
**Spec Refs:** UC-001, UC-002, UC-003, BS-001, BS-003, BS-004, BS-005, BS-009, BS-010, UX-1 through UX-6, design §8

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-027 — "meal plan this week" creates a draft plan (UX-1.1)
  Given the user sends "meal plan this week" via Telegram
  When the bot processes the command
  Then a draft plan for the current week (Mon-Sun) is created
  And the bot responds with plan name, date range, status "draft", and instructions for adding recipes

Scenario: SCN-036-028 — Slot assignment "Monday dinner Pasta Carbonara for 4" (UX-1.2)
  Given an active draft plan exists for the current week
  When the user sends "Monday dinner Pasta Carbonara for 4"
  Then the system searches for "Pasta Carbonara" in recipe artifacts
  And assigns the matched recipe to Monday dinner with 4 servings
  And confirms the assignment with slot count summary

Scenario: SCN-036-029 — Batch assignment "Mon-Thu breakfast: Overnight Oats for 2" (UX-1.3)
  Given an active draft plan exists
  When the user sends "Mon-Thu breakfast: Overnight Oats for 2"
  Then 4 breakfast slots (Mon, Tue, Wed, Thu) are created with Overnight Oats at 2 servings each
  And the bot confirms "4 slots added"

Scenario: SCN-036-030 — "what's for dinner tomorrow?" resolves via plan (UX-2.2, BS-003)
  Given an active plan with "Thai Green Curry" (2 servings) assigned to tomorrow's dinner
  When the user asks "what's for dinner tomorrow?"
  Then the bot responds with "Tomorrow dinner: Thai Green Curry (2 servings)"

Scenario: SCN-036-031 — Weekly overview "meal plan" (UX-2.1, BS-004)
  Given an active plan for the current week with 7 meals assigned
  When the user sends "meal plan"
  Then the bot lists all assigned meals by day with title, date range, status, and summary line

Scenario: SCN-036-032 — "cook tonight's dinner" resolves via plan (BS-010, UX-4.2)
  Given an active plan with "Pasta Carbonara" (4 servings) for tonight's dinner
  When the user says "cook tonight's dinner"
  Then the system resolves to "Pasta Carbonara" via the plan
  And enters cook mode (spec 035) with 4 servings as the target serving count

Scenario: SCN-036-033 — Plan overlap warning via Telegram (BS-009, UX-1.4)
  Given an active plan for Apr 20-26
  When the user creates and activates a plan for Apr 23-29
  Then the bot warns about overlapping days and offers merge, replace, or keep-both options

Scenario: SCN-036-034 — No draft plan exists when assigning slots (UX-1.5)
  Given no draft plan exists
  When the user sends "Monday dinner Pasta Carbonara for 4"
  Then the bot responds "No draft plan. Create one first: meal plan this week"

Scenario: SCN-036-035 — Recipe disambiguation on slot assignment (UX-1.2)
  Given multiple recipe artifacts match "carbonara"
  When the user sends "Monday dinner carbonara for 4"
  Then the bot presents numbered disambiguation options
  And creates the slot when the user replies with a number

Scenario: SCN-036-036 — "shopping list for plan" via Telegram (UX-3.1, UC-002)
  Given an active plan with meals assigned
  When the user sends "shopping list for plan"
  Then the system generates a shopping list from the plan via the ShoppingBridge
  And responds with scaling summary, item count, and list name

Scenario: SCN-036-037 — "repeat last week" via Telegram (UX-5.1, BS-006)
  Given a completed plan "Week of Apr 13" with 5 meal assignments
  When the user sends "repeat last week's plan"
  Then a new draft plan "Week of Apr 20" is created with recipes shifted by 7 days
  And the bot confirms the copy with meal count and status
```

### Implementation Plan

**Files to create:**
- `internal/telegram/mealplan_commands.go` — Command handlers per design §8.1 command routing table
- `internal/telegram/mealplan_commands_test.go` — Unit tests for pattern matching, date parsing, response formatting

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-04-01 | Unit | `internal/telegram/mealplan_commands_test.go` | SCN-036-027 | "meal plan this week" creates draft |
| T-04-02 | Unit | `internal/telegram/mealplan_commands_test.go` | SCN-036-028 | Natural language slot assignment |
| T-04-03 | Unit | `internal/telegram/mealplan_commands_test.go` | SCN-036-029 | Batch slot assignment |
| T-04-04 | Unit | `internal/telegram/mealplan_commands_test.go` | SCN-036-030 | "what's for dinner tomorrow?" |
| T-04-05 | Unit | `internal/telegram/mealplan_commands_test.go` | SCN-036-031 | Weekly overview formatting |
| T-04-06 | Unit | `internal/telegram/mealplan_commands_test.go` | SCN-036-032 | "cook tonight's dinner" delegation |
| T-04-07 | Unit | `internal/telegram/mealplan_commands_test.go` | SCN-036-033 | Overlap warning Telegram flow |
| T-04-08 | Unit | `internal/telegram/mealplan_commands_test.go` | SCN-036-034 | No draft plan error message |
| T-04-09 | Unit | `internal/telegram/mealplan_commands_test.go` | SCN-036-035 | Recipe disambiguation |
| T-04-10 | Unit | `internal/telegram/mealplan_commands_test.go` | SCN-036-036 | Shopping list from plan via Telegram |
| T-04-11 | Unit | `internal/telegram/mealplan_commands_test.go` | SCN-036-037 | "repeat last week" via Telegram |
| T-04-12 | Regression E2E | `tests/e2e/mealplan_telegram_test.go` | SCN-036-027, SCN-036-030, SCN-036-032 | Telegram plan creation, query, and cook delegation on live stack |

### Definition of Done

- [ ] All Telegram command patterns from design §8.1 routing table registered and handled
- [ ] Plan creation from natural language: "meal plan this week", "meal plan next week", "meal plan {date} to {date}"
- [ ] Slot assignment with recipe search, disambiguation, servings parsing, and batch support
- [ ] Daily query ("what's for dinner?") and weekly overview ("meal plan") with formatted responses per UX-2.1/UX-2.2
- [ ] "Cook tonight's dinner" resolves recipe via plan and delegates to spec 035 cook mode
- [ ] Shopping list generation triggered via "shopping list for plan"
- [ ] Plan repeat via "repeat last week's plan"
- [ ] Draft plan context per chat ID with 24-hour TTL (in-process memory, not DB)
- [ ] All 11 Gherkin scenarios pass
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes

---

## Scope 05: Shopping List Bridge

**Status:** Not Started
**Priority:** P1
**Depends On:** 02
**Spec Refs:** UC-002, BS-002, BS-007, BS-012, design §5

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-038 — Generate shopping list from plan with merged ingredients (BS-002)
  Given an active plan with Pasta Carbonara (4 servings), Thai Green Curry (2 servings),
        and Caesar Salad (2 servings) assigned across the week
  When ShoppingBridge.GenerateFromPlan is called
  Then each recipe's domain_data is loaded via ArtifactResolver.ResolveByIDs
  And ingredients are scaled per slot via ScaleIngredients from spec 035
  And scaled AggregationSources are passed to RecipeAggregator.Aggregate
  And a single shopping list is produced with merged, normalized ingredients
  And the list has SourceQuery = "plan:{plan_id}"

Scenario: SCN-036-039 — Multi-day same recipe aggregates servings (BS-007)
  Given "Overnight Oats" planned for Mon-Thu breakfast at 2 servings each with batch_flag=true
  When the shopping list is generated
  Then the bridge emits a single AggregationSource with totalServings=8 (2×4)
  And oat ingredients in the final list reflect 8 total servings

Scenario: SCN-036-040 — Non-batch duplicate recipe slots aggregated individually
  Given "Overnight Oats" planned for Mon and Wed breakfast at 2 servings each without batch_flag
  When the shopping list is generated
  Then the bridge emits two separate AggregationSources (one per slot, each scaled to 2 servings)
  And RecipeAggregator merges duplicate ingredients across both sources

Scenario: SCN-036-041 — Recipe with missing domain_data is skipped (UC-002 A1)
  Given a plan slot references a recipe artifact with no domain_data
  When the shopping list is generated
  Then that recipe is skipped in the list with a note in the scaling summary
  And all other recipes are included normally

Scenario: SCN-036-042 — Regenerate after plan edit replaces old list (BS-012)
  Given a shopping list was generated from a plan (SourceQuery = "plan:{id}")
  And the plan was subsequently edited (plan.UpdatedAt > list.CreatedAt)
  When the user requests regeneration with force=true
  Then the old list is archived and a new list is generated reflecting the updated plan

Scenario: SCN-036-043 — Regeneration without force returns 409 when list exists
  Given a shopping list already exists for the plan
  When generation is requested without force=true
  Then HTTP 409 is returned with the existing list ID and plan_modified_since_list flag
```

### Implementation Plan

**Files to create:**
- `internal/mealplan/shopping.go` — ShoppingBridge: convert plan slots to AggregationSources with per-slot scaling, delegate to RecipeAggregator + Generator
- `internal/mealplan/shopping_test.go` — Unit tests with mocked ArtifactResolver, ScaleIngredients, RecipeAggregator

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-05-01 | Unit | `internal/mealplan/shopping_test.go` | SCN-036-038 | Plan → scaled AggregationSources → merged list |
| T-05-02 | Unit | `internal/mealplan/shopping_test.go` | SCN-036-039 | Batch recipe consolidation (totalServings) |
| T-05-03 | Unit | `internal/mealplan/shopping_test.go` | SCN-036-040 | Non-batch duplicate slots emit separate sources |
| T-05-04 | Unit | `internal/mealplan/shopping_test.go` | SCN-036-041 | Missing domain_data skipped with note |
| T-05-05 | Unit | `internal/mealplan/shopping_test.go` | SCN-036-042 | Regeneration archives old list, creates new |
| T-05-06 | Unit | `internal/mealplan/shopping_test.go` | SCN-036-043 | No-force returns 409 |
| T-05-07 | Integration | `tests/integration/mealplan_shopping_test.go` | SCN-036-038 | Plan shopping list with real RecipeAggregator and ScaleIngredients |
| T-05-08 | Regression E2E | `tests/e2e/mealplan_shopping_test.go` | SCN-036-038, SCN-036-039 | Live stack plan → list generation with scaling verification |
| T-05-09 | Regression Integration | `tests/integration/list_regression_test.go` | — | Existing spec 028 direct recipe→list path unchanged |

### Definition of Done

- [ ] `internal/mealplan/shopping.go` implements ShoppingBridge.GenerateFromPlan per design §5.1 algorithm
- [ ] Per-slot scaling via ScaleIngredients from spec 035 (no new scaling code)
- [ ] Batch-flagged slots consolidated into single AggregationSource with totalServings
- [ ] Delegation to existing RecipeAggregator.Aggregate and Generator.Generate (no new aggregation code)
- [ ] SourceQuery = "plan:{plan_id}" for traceability
- [ ] Missing domain_data recipes skipped with user-visible note in scaling summary
- [ ] Regeneration with force archives old list; without force returns 409
- [ ] Existing spec 028 direct-from-recipes shopping list generation unchanged (regression verified)
- [ ] All 6 Gherkin scenarios pass
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes

---

## Scope 06: Plan Copy & Templates

**Status:** Not Started
**Priority:** P2
**Depends On:** 02, 03
**Spec Refs:** UC-005, BS-006, BS-011, design §4.5

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-044 — Copy plan to new week with date shift (BS-006)
  Given a completed plan "Week of Apr 13" spanning Apr 13-19 with 5 slots
  When PlanService.CopyPlan is called with newStartDate=2026-04-20 and newTitle="Week of Apr 20"
  Then a new draft plan "Week of Apr 20" spanning Apr 20-26 is created
  And all 5 slots are copied with slot_date shifted by 7 days
  And original recipe artifact IDs and servings are preserved

Scenario: SCN-036-045 — Copy with deleted recipe omits slot (BS-011)
  Given a plan template references recipe artifact "01JDEL001" which no longer exists
  When the plan is copied
  Then the slot for the deleted recipe is omitted
  And the response includes slots_skipped with reason "recipe artifact not found"
  And all other slots are copied correctly

Scenario: SCN-036-046 — Copy with serving overrides
  Given a source plan with 2 dinner slots at 2 servings and 3 breakfast slots at 1 serving
  When the plan is copied with serving override "dinner: 6"
  Then dinner slots in the new plan have servings=6
  And breakfast slots retain the original servings=1

Scenario: SCN-036-047 — API POST /api/meal-plans/{id}/copy creates shifted plan
  Given a completed plan
  When POST /api/meal-plans/{id}/copy is called with new_start_date and new_title
  Then HTTP 201 is returned with the new plan, slots_copied count, and slots_skipped array
```

### Implementation Plan

**Files to modify:**
- `internal/mealplan/service.go` — Add CopyPlan with date shift, deleted recipe handling, optional serving overrides
- `internal/api/mealplan.go` — POST /api/meal-plans/{id}/copy handler
- `internal/telegram/mealplan_commands.go` — "repeat last week" and "copy plan" handlers

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-06-01 | Unit | `internal/mealplan/service_test.go` | SCN-036-044 | Copy plan with correct date shift |
| T-06-02 | Unit | `internal/mealplan/service_test.go` | SCN-036-045 | Copy skips deleted recipe slots |
| T-06-03 | Unit | `internal/mealplan/service_test.go` | SCN-036-046 | Copy with serving overrides |
| T-06-04 | Unit | `internal/api/mealplan_test.go` | SCN-036-047 | API copy endpoint |
| T-06-05 | Regression E2E | `tests/e2e/mealplan_copy_test.go` | SCN-036-044, SCN-036-045 | Live stack plan copy with date shift and deleted recipe handling |

### Definition of Done

- [ ] CopyPlan creates new draft plan with all slot dates shifted by dayOffset
- [ ] Deleted recipe slots omitted with explanation in slots_skipped response
- [ ] Optional serving overrides applied per meal type
- [ ] API POST /api/meal-plans/{id}/copy returns new plan with copy details
- [ ] Telegram "repeat last week" and "copy plan" commands work
- [ ] All 4 Gherkin scenarios pass
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes

---

## Scope 07: CalDAV Calendar Sync

**Status:** Not Started
**Priority:** P2
**Depends On:** 02
**Spec Refs:** UC-004, BS-008, BS-013, design §6

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-048 — Create CalDAV events from plan slots (BS-008)
  Given an active plan with "Thai Green Curry" on Tuesday dinner
  And CalDAV connector is configured and meal_planning.calendar_sync is true
  When POST /api/meal-plans/{id}/calendar-sync is called
  Then a CalDAV VEVENT is created with UID "smackerel-meal-{slotID}",
       SUMMARY "Thai Green Curry", DTSTART at Tuesday + configured dinner time,
       DESCRIPTION containing scaled ingredient list, and CATEGORIES "smackerel-meal"

Scenario: SCN-036-049 — Configurable meal times in CalDAV events (BS-013)
  Given meal_planning.meal_times.dinner is configured as "19:00" in smackerel.yaml
  When a dinner slot syncs to CalDAV
  Then the VEVENT DTSTART uses 19:00, not the default 18:00

Scenario: SCN-036-050 — CalDAV not configured returns 422
  Given CalDAV connector is not configured or meal_planning.calendar_sync is false
  When POST /api/meal-plans/{id}/calendar-sync is called
  Then HTTP 422 is returned with code MEAL_PLAN_CALDAV_NOT_CONFIGURED
  And the message instructs the user to configure CalDAV in smackerel.yaml

Scenario: SCN-036-051 — Plan deletion cleans up CalDAV events
  Given a plan has been synced to CalDAV (events exist with X-SMACKEREL-PLAN-ID)
  When the plan is deleted
  Then all CalDAV events with matching X-SMACKEREL-PLAN-ID are deleted
  And partial event deletion failures are logged but do not block plan deletion

Scenario: SCN-036-052 — Individual event sync failures do not abort batch
  Given a plan with 7 slots being synced to CalDAV
  When 1 event creation fails (transient CalDAV error) and 6 succeed
  Then the response shows events_created=6, events_failed=1
  And the failed event is logged with details
```

### Implementation Plan

**Files to create:**
- `internal/mealplan/calendar.go` — CalDAV bridge: create/update/delete VEVENTs from plan slots per design §6
- `internal/mealplan/calendar_test.go` — Unit tests with mock CalDAV client

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-07-01 | Unit | `internal/mealplan/calendar_test.go` | SCN-036-048 | VEVENT creation with correct fields |
| T-07-02 | Unit | `internal/mealplan/calendar_test.go` | SCN-036-049 | Configurable meal times in DTSTART |
| T-07-03 | Unit | `internal/mealplan/calendar_test.go` | SCN-036-050 | CalDAV not configured returns 422 |
| T-07-04 | Unit | `internal/mealplan/calendar_test.go` | SCN-036-051 | Cascade delete CalDAV events on plan delete |
| T-07-05 | Unit | `internal/mealplan/calendar_test.go` | SCN-036-052 | Partial sync failure: 6 succeed, 1 fail |
| T-07-06 | Integration | `tests/integration/mealplan_caldav_test.go` | SCN-036-048 | CalDAV event creation with real connector |
| T-07-07 | Regression E2E | `tests/e2e/mealplan_caldav_test.go` | SCN-036-048, SCN-036-049 | CalDAV sync on live stack with configurable times |

### Definition of Done

- [ ] CalDAV bridge maps plan slots to VEVENTs per design §6.2 field mapping
- [ ] UID format: `smackerel-meal-{slot.ID}`, CATEGORIES: `smackerel-meal`
- [ ] Meal times from `meal_planning.meal_times` config (SST, no hardcoded defaults)
- [ ] Sync is user-initiated via API/Telegram, not automatic
- [ ] Individual event failures logged but do not abort the batch
- [ ] Plan deletion triggers CalDAV event cleanup (best-effort, non-blocking)
- [ ] CalDAV-disabled returns 422 with actionable message
- [ ] All 5 Gherkin scenarios pass
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes

---

## Scope 08: Auto-Complete Lifecycle

**Status:** Not Started
**Priority:** P2
**Depends On:** 01, 02
**Spec Refs:** design §10

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-053 — Auto-complete transitions past active plans to completed
  Given an active plan with end_date 2026-04-19 (in the past) and auto_complete_past_plans=true
  When the daily auto-complete scheduler job runs
  Then the plan status transitions from "active" to "completed"
  And updated_at is refreshed

Scenario: SCN-036-054 — Auto-complete skips plans with future end_date
  Given an active plan with end_date 2026-04-25 (in the future)
  When the auto-complete scheduler job runs
  Then the plan remains "active" with no changes

Scenario: SCN-036-055 — Auto-complete disabled via config
  Given meal_planning.auto_complete_past_plans is false in smackerel.yaml
  When the scheduler initializes
  Then the auto-complete cron job is not registered

Scenario: SCN-036-056 — Auto-complete cron schedule from config
  Given meal_planning.auto_complete_cron is "0 2 * * *" in smackerel.yaml
  When the scheduler registers the auto-complete job
  Then the job is scheduled at 02:00 daily, not the default 01:00
```

### Implementation Plan

**Files to modify:**
- `internal/mealplan/service.go` — Add AutoCompletePastPlans method
- `internal/scheduler/scheduler.go` — Register meal plan auto-complete cron job

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-08-01 | Unit | `internal/mealplan/service_test.go` | SCN-036-053 | Auto-complete transitions past plans |
| T-08-02 | Unit | `internal/mealplan/service_test.go` | SCN-036-054 | Future plans not touched |
| T-08-03 | Unit | `internal/scheduler/scheduler_test.go` | SCN-036-055 | Auto-complete disabled skips registration |
| T-08-04 | Unit | `internal/scheduler/scheduler_test.go` | SCN-036-056 | Custom cron schedule from config |
| T-08-05 | Integration | `tests/integration/mealplan_lifecycle_test.go` | SCN-036-053 | Auto-complete on live DB with past plans |
| T-08-06 | Regression E2E | `tests/e2e/mealplan_lifecycle_test.go` | SCN-036-053 | Auto-complete verified on live stack |

### Definition of Done

- [ ] `Service.AutoCompletePastPlans` queries active plans with end_date < CURRENT_DATE and transitions to "completed"
- [ ] Scheduler registers cron job using `meal_planning.auto_complete_cron` from config
- [ ] Job only registered when `meal_planning.auto_complete_past_plans` is true
- [ ] Job executes with 60-second timeout context
- [ ] Future-dated active plans are not affected
- [ ] All 4 Gherkin scenarios pass
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
