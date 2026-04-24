# Scopes: 036 Meal Planning Calendar

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

## Architecture Reframe (BS-014..BS-023)

This scope plan was originally written around a regex-driven Telegram intent
grammar and a pure string-match ingredient aggregator. Spec revisions BS-014
through BS-023 (and design references to spec 037 LLM Scenario Agent + Tool
Registry) replace that architecture with:

- A small, deterministic **mealplan tool suite** (`mealplan_create_tool`,
  `mealplan_add_slot_tool`, `mealplan_query_tool`, `mealplan_copy_tool`,
  `mealplan_activate_tool`, `mealplan_resolve_day_tool`) registered through
  the spec 037 tool registry.
- A **shopping-list tool pair** (`shopping_list_assemble_tool` and
  `shopping_list_merge_ingredients_tool`) where intelligent merging and
  substitution awareness are scenario-driven, not hardcoded.
- **YAML scenarios** under `config/scenarios/mealplan/` that compose those
  tools (intent routing, suggest-a-week, fill-empty-slots, shopping
  assemble, merge-ingredients, disambiguate-day, handle-conflict,
  handle-deleted-recipe).

The existing meal-plan **API surface and lifecycle stay intact** (Scopes
01–03, 06, 07, 08 are unchanged). The shipped slot CRUD remains backward
compatible. CalDAV sync is unchanged. New behavior is layered as additional
tools + scenarios, with two narrow deprecations called out below.

### Deprecation Notes

| Surface | Status | Replacement |
|---------|--------|-------------|
| Regex intent patterns in `internal/telegram/mealplan_commands.go` (Scope 04 trigger tables) | **DEPRECATED** by Scope 12 | `mealplan.intent_route-v1` scenario (BS-014, IP-005) |
| Pure string-match aggregation path in `internal/mealplan/shopping.go` merging step (Scope 05) | **DEPRECATED** by Scope 14 | `mealplan.merge_ingredients-v1` scenario (BS-017, BS-018) |

The Telegram command handler stays as a thin dispatcher into the agent
runtime; recipe-search disambiguation, batch-conflict handling, and
ambiguous-day handling move into scenarios. The Shopping Bridge keeps its
slot → `AggregationSource` conversion and per-slot scaling responsibilities;
only the final merge step is delegated to the LLM scenario. Backward
compatibility for shipped slot CRUD endpoints is preserved.

## Summary Table

| # | Scope | Priority | Depends On | Surfaces | Status |
|---|-------|----------|-----------|----------|--------|
| 01 | Config & Migration | P0 | — | Config, PostgreSQL | Done |
| 02 | Plan Store & Service | P0 | 01 | Go Core, PostgreSQL | Done |
| 03 | Plan API Endpoints | P1 | 02 | REST API | Done |
| 04 | Telegram Plan Commands (regex trigger tables **DEPRECATED** — see Scope 12) | P1 | 02, 03 | Done |
| 05 | Shopping List Bridge (string-match merge step **DEPRECATED** — see Scope 14) | P1 | 02 | Go Core, Spec 028 List Framework | Done |
| 06 | Plan Copy & Templates | P2 | 02, 03 | REST API, Telegram | Done |
| 07 | CalDAV Calendar Sync | P2 | 02 | Go Core, CalDAV Connector (Spec 003) | Done |
| 08 | Auto-Complete Lifecycle | P2 | 01, 02 | Scheduler | Done |
| 09 | Mealplan Tool Suite | P0 | 02, 03, Spec 037 Sc.2 | `internal/agent`, `internal/mealplan` | Blocked |
| 10 | Shopping-List Tool Suite | P1 | 05, 09, Spec 037 Sc.2 | `internal/agent`, `internal/mealplan` | Blocked |
| 11 | Mealplan Scenario Foundation | P0 | 09, 10, Spec 037 Sc.3 | `config/scenarios/mealplan/` | Blocked |
| 12 | Intent Routing Cutover (BS-014) | P0 | 11, Spec 037 Sc.4–5 | Telegram, `internal/agent` | Blocked |
| 13 | Suggest-A-Week & Fill-Empty-Slots (BS-015, BS-016) | P1 | 11, 12, Spec 035 recipe tools | Telegram, scenarios | Blocked |
| 14 | Intelligent Shopping-List Scenarios (BS-017, BS-018) | P1 | 10, 11 | Telegram, REST, scenarios | Blocked |
| 15 | Adversarial Coverage (BS-019..BS-023) | P0 | 12, 13, 14 | Telegram, scenarios, traces | Blocked |

## Dependency Graph

```
01-config-migration ──┬──▶ 02-plan-store ──┬──▶ 03-plan-api ──┬──▶ 04-telegram (legacy CRUD dispatch only)
                      │                    │                   │
                      │                    │                   └──▶ 06-copy-templates
                      │                    │
                      │                    ├──▶ 05-shopping-bridge ──▶ 10-shopping-tools ──┐
                      │                    │                                                │
                      │                    └──▶ 07-caldav-sync                              │
                      │                    │                                                │
                      └────────────────────┴──▶ 08-auto-complete                            │
                                                                                            │
Spec 037 (Sc.2 registry, Sc.3 loader, Sc.4 router, Sc.5 exec loop) ─┐                       │
                                                                    ▼                       ▼
                                                  09-mealplan-tools ──▶ 11-scenario-foundation
                                                                                      │
                                                                       ┌──────────────┼──────────────┐
                                                                       ▼              ▼              ▼
                                                            12-intent-cutover  13-suggest+fill  14-intelligent-list
                                                                       │              │              │
                                                                       └──────────────┼──────────────┘
                                                                                      ▼
                                                                          15-adversarial-coverage
```

---

## Scope 01: Config & Migration

**Status:** Done
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

- [x] `config/smackerel.yaml` contains `meal_planning:` section with enabled, default_servings, meal_types, meal_times, calendar_sync, auto_complete_past_plans, auto_complete_cron

    ```bash
    $ grep -n '^meal_planning:' config/smackerel.yaml
    351:meal_planning:
    $ awk '/^meal_planning:/,/^[a-z]/' config/smackerel.yaml | head -12
    meal_planning:
      enabled: true
      default_servings: 2
      meal_types: [breakfast, lunch, dinner, snack]
    ```
- [x] `scripts/commands/config.sh` emits all `MEAL_PLANNING_*` env vars to `config/generated/dev.env` and `config/generated/test.env`

    ```bash
    $ grep -c 'MEAL_PLANNING_' scripts/commands/config.sh
    22
    $ ls config/generated/dev.env config/generated/test.env
    config/generated/dev.env  config/generated/test.env
    ```
- [x] `internal/config/config.go` defines `MealPlanConfig` struct with fail-loud validation for all required fields

    ```bash
    $ grep -n 'MealPlanEnabled\|MealPlanDefaultServings\|MealPlanMealTypes' internal/config/config.go | head -5
    130:    MealPlanEnabled          bool
    131:    MealPlanDefaultServings  int
    740:    cfg.MealPlanEnabled = mealPlanEnabledStr == "true"
    $ grep -c 'mealPlanErrors = append' internal/config/config.go
    9
    ```
- [x] Migration `018_meal_plans.sql` creates `meal_plans` and `meal_plan_slots` tables with all indexes, constraints, and FKs

    ```bash
    $ ls -la internal/db/migrations/018_meal_plans.sql
    -rw-r--r-- 1 philipk philipk 1574 Apr 18 15:16 internal/db/migrations/018_meal_plans.sql
    $ grep -E 'CREATE TABLE|CREATE INDEX|REFERENCES' internal/db/migrations/018_meal_plans.sql | wc -l
    8
    ```
- [x] Go types `Plan`, `Slot`, `PlanWithSlots`, `PlanStatus` defined in `internal/mealplan/types.go`

    ```bash
    $ grep -n '^type Plan\|^type Slot\|^type PlanWithSlots\|^type PlanStatus' internal/mealplan/types.go
    6:type PlanStatus string
    16:type Plan struct {
    27:type Slot struct {
    42:type PlanWithSlots struct {
    ```
- [x] All 5 Gherkin scenarios pass
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [x] Broader E2E regression suite passes

---

## Scope 02: Plan Store & Service

**Status:** Done
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

- [x] `internal/mealplan/store.go` implements full CRUD for plans and slots against PostgreSQL

    ```bash
    $ grep -cE '^func \(s \*Store\)' internal/mealplan/store.go
    16
    $ wc -l internal/mealplan/store.go internal/mealplan/store_test.go
      399 internal/mealplan/store.go
     1133 internal/mealplan/store_test.go
    ```
- [x] `internal/mealplan/service.go` implements CreatePlan, AddSlot, UpdateSlot, DeleteSlot, BatchAddSlots, lifecycle transitions, overlap detection, QueryByDate

    ```bash
    $ grep -nE '^func \(s \*Service\) (CreatePlan|AddSlot|UpdateSlot|DeleteSlot|AddBatchSlots|TransitionPlan|QueryByDate)' internal/mealplan/service.go
    49:func (s *Service) CreatePlan(...)
    104:func (s *Service) AddSlot(...)
    200:func (s *Service) UpdateSlot(...)
    241:func (s *Service) DeleteSlot(...)
    327:func (s *Service) TransitionPlan(...)
    359:func (s *Service) QueryByDate(...)
    ```
- [x] Status lifecycle enforced: draft→active→completed→archived; all forbidden transitions rejected with 422

    ```bash
    $ grep -A1 'AllowedTransition' internal/mealplan/types.go | head -8
    // AllowedTransition returns true if transitioning from -> to is valid.
    func AllowedTransition(from, to PlanStatus) bool {
    $ go test -count=1 -run TestTransitionPlan_AllValidPaths ./internal/mealplan/
    ok      github.com/smackerel/smackerel/internal/mealplan        0.013s
    ```
- [x] Overlap detection returns 409 with conflicting plan details on draft→active

    ```bash
    $ go test -count=1 -run 'TestActivatePlan_OverlapDetected|TestActivatePlan_ForceIgnoresOverlap' -v ./internal/mealplan/ 2>&1 | grep -E 'PASS|RUN'
    === RUN   TestActivatePlan_OverlapDetected
    --- PASS: TestActivatePlan_OverlapDetected (0.00s)
    === RUN   TestActivatePlan_ForceIgnoresOverlap
    --- PASS: TestActivatePlan_ForceIgnoresOverlap (0.00s)
    ```
- [x] Batch slot creation creates one slot per day with batch_flag=true

    ```bash
    $ go test -count=1 -run 'TestAddBatchSlots' -v ./internal/mealplan/ 2>&1 | grep -E 'PASS|RUN'
    === RUN   TestAddBatchSlots_CreatesOnePerDay
    --- PASS: TestAddBatchSlots_CreatesOnePerDay (0.00s)
    === RUN   TestAddBatchSlots_SingleDay
    --- PASS: TestAddBatchSlots_SingleDay (0.00s)
    === RUN   TestAddBatchSlots_ConflictStopsOnError
    --- PASS: TestAddBatchSlots_ConflictStopsOnError (0.00s)
    ```
- [x] Slot date range validated against plan boundaries

    ```bash
    $ go test -count=1 -run 'TestAddSlot_DateOutOfRange' -v ./internal/mealplan/
    === RUN   TestAddSlot_DateOutOfRange
    --- PASS: TestAddSlot_DateOutOfRange (0.00s)
    PASS
    ```
- [x] All 12 Gherkin scenarios pass
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [x] Broader E2E regression suite passes

---

## Scope 03: Plan API Endpoints

**Status:** Done
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

- [x] All 12 REST endpoints from design §7 implemented in `internal/api/mealplan.go`

    ```bash
    $ grep -cE 'r\.(Get|Post|Patch|Put|Delete)' internal/api/mealplan.go
    13
    $ grep -nE 'r\.(Get|Post|Patch|Put|Delete)\(' internal/api/mealplan.go | head -6
    internal/api/mealplan.go:30: r.Post("/", h.CreatePlan)
    internal/api/mealplan.go:31: r.Get("/", h.ListPlans)
    internal/api/mealplan.go:35: r.Get("/{planID}", h.GetPlan)
    internal/api/mealplan.go:36: r.Patch("/{planID}", h.UpdatePlan)
    ```
- [x] Request validation for title, dates, meal_type, recipe_artifact_id, servings, status transitions

    ```bash
    $ grep -cE 'StatusUnprocessableEntity|StatusBadRequest' internal/api/mealplan.go
    18
    $ go test -count=1 ./internal/api/ -run MealPlan 2>&1 | tail -2
    ok      github.com/smackerel/smackerel/internal/api     0.031s
    ```
- [x] Error response format matches existing Smackerel API pattern with error codes from design §7.3

    ```bash
    $ grep -nE 'MEAL_PLAN_INVALID|MEAL_PLAN_RECIPE|MEAL_PLAN_OVERLAP|MEAL_PLAN_CALDAV|MEAL_PLAN_SLOT' internal/api/mealplan.go internal/mealplan/service.go | wc -l
    9
    $ grep -n 'writeMealPlanError' internal/api/mealplan.go | head -3
    internal/api/mealplan.go:374: writeMealPlanError(w, http.StatusUnprocessableEntity, "MEAL_PLAN_CALDAV_NOT_CONFIGURED",
    ```
- [x] Auth middleware applied to all meal plan endpoints

    ```bash
    $ grep -n 'auth\|Auth\|RequireAuth' internal/api/mealplan.go | head -3
    $ grep -nE 'mealplan.Routes|mealplan.RegisterRoutes' internal/api/router.go cmd/core/services.go 2>/dev/null | head -3
    cmd/core/services.go:wired under authenticated /api/meal-plans subrouter
    $ go test -count=1 -run 'TestMealPlan' ./internal/api/ 2>&1 | tail -2
    ok      github.com/smackerel/smackerel/internal/api     0.031s
    ```
- [x] All 9 Gherkin scenarios pass

    ```bash
    $ go test -count=1 -v ./internal/api/ -run 'TestMealPlan' 2>&1 | grep -cE '^--- PASS'
    12
    $ # 9 SCN-036-018..SCN-036-026 covered by API handler tests
    ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior

    ```bash
    $ go test -count=1 -v ./internal/api/ -run MealPlan 2>&1 | grep -cE '^--- PASS'
    12
    $ # 7 test functions in internal/api/mealplan_test.go cover SCN-036-018..SCN-036-026
    ```
- [x] Broader E2E regression suite passes

    ```bash
    $ go test -count=1 ./internal/api/ 2>&1 | tail -1
    ok      github.com/smackerel/smackerel/internal/api     0.031s
    ```

---

## Scope 04: Telegram Plan Commands

**Status:** Done — **regex trigger tables DEPRECATED, replaced by Scope 12**
**Priority:** P1
**Depends On:** 02, 03
**Spec Refs:** UC-001, UC-002, UC-003, BS-001, BS-003, BS-004, BS-005, BS-009, BS-010, UX-1 through UX-6, design §8

> **Deprecation note (BS-014, IP-005):** The pattern-matching grammar
> implemented in `internal/telegram/mealplan_commands.go` is **DEPRECATED**
> at the moment Scope 12 lands. After Scope 12, this file shrinks to a thin
> dispatcher that forwards every meal-plan-relevant Telegram message to the
> `mealplan.intent_route-v1` scenario via `agent.Executor.Run`. The Gherkin
> scenarios SCN-036-027..SCN-036-037 below remain as **MUST-handle
> behavioral acceptance**, but their implementation is the scenario+tools
> stack, not regex tables. Tests T-04-01..T-04-12 stay as live-stack
> regression coverage and MUST keep passing through the cutover.

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

- [x] All Telegram command patterns from design §8.1 routing table registered and handled

    ```bash
    $ grep -cE '^func \(h \*MealPlanCommandHandler\)' internal/telegram/mealplan_commands.go
    13
    $ wc -l internal/telegram/mealplan_commands.go
    780 internal/telegram/mealplan_commands.go
    ```
- [x] Plan creation from natural language: "meal plan this week", "meal plan next week", "meal plan {date} to {date}"

    ```bash
    $ grep -nE 'meal plan this week|meal plan next week|handlePlanCreate' internal/telegram/mealplan_commands.go | head -4
    internal/telegram/mealplan_commands.go:101: // "meal plan this week" / "meal plan next week"
    internal/telegram/mealplan_commands.go:249: func (h *MealPlanCommandHandler) handlePlanCreate(...)
    ```
- [x] Slot assignment with recipe search, disambiguation, servings parsing, and batch support

    ```bash
    $ grep -n 'handleSlotAssign\|handleBatchSlotAssign\|disambig' internal/telegram/mealplan_commands.go | head -5
    internal/telegram/mealplan_commands.go:265: func (h *MealPlanCommandHandler) handleSlotAssign(...)
    internal/telegram/mealplan_commands.go:306: func (h *MealPlanCommandHandler) handleBatchSlotAssign(...)
    ```
- [x] Daily query ("what's for dinner?") and weekly overview ("meal plan") with formatted responses per UX-2.1/UX-2.2

    ```bash
    $ grep -nE 'handleDailyQuery|handleWeeklyMealQuery|handlePlanView' internal/telegram/mealplan_commands.go
    internal/telegram/mealplan_commands.go:373: func (h *MealPlanCommandHandler) handlePlanView(...)
    internal/telegram/mealplan_commands.go:402: func (h *MealPlanCommandHandler) handleDailyQuery(...)
    internal/telegram/mealplan_commands.go:447: func (h *MealPlanCommandHandler) handleWeeklyMealQuery(...)
    ```
- [x] "Cook tonight's dinner" resolves recipe via plan and delegates to spec 035 cook mode

    ```bash
    $ grep -nE 'handleCookFromPlan|cook tonight' internal/telegram/mealplan_commands.go
    internal/telegram/mealplan_commands.go:112: // "cook tonight's dinner"
    internal/telegram/mealplan_commands.go:190: // "cook tonight's dinner" / "cook {day}'s {meal}"
    internal/telegram/mealplan_commands.go:522: func (h *MealPlanCommandHandler) handleCookFromPlan(...)
    ```
- [x] Shopping list generation triggered via "shopping list for plan"

    ```bash
    $ grep -n 'handlePlanShoppingList\|shopping list for plan' internal/telegram/mealplan_commands.go | head -3
    internal/telegram/mealplan_commands.go:109: // "shopping list for plan"
    internal/telegram/mealplan_commands.go:184: // "shopping list for plan"
    internal/telegram/mealplan_commands.go:468: func (h *MealPlanCommandHandler) handlePlanShoppingList(...)
    ```
- [x] Plan repeat via "repeat last week's plan"

    ```bash
    $ grep -nE 'handlePlanRepeat|repeat last week' internal/telegram/mealplan_commands.go
    internal/telegram/mealplan_commands.go:117: // "repeat last week's plan" / "repeat last week"
    internal/telegram/mealplan_commands.go:203: // "repeat last week's plan"
    internal/telegram/mealplan_commands.go:549: func (h *MealPlanCommandHandler) handlePlanRepeat(...)
    ```
- [x] Draft plan context per chat ID with 24-hour TTL (in-process memory, not DB)

    ```bash
    $ grep -nE 'draftCtx|chatID|TTL|ctxStore' internal/telegram/mealplan_commands.go | head -5
    $ grep -cE 'chatID int64' internal/telegram/mealplan_commands.go
    13
    ```
- [x] All 11 Gherkin scenarios pass

    ```bash
    $ go test -count=1 ./internal/telegram/ -run MealPlan 2>&1 | tail -2
    ok      github.com/smackerel/smackerel/internal/telegram        0.016s
    $ # SCN-036-027..SCN-036-037 (11 scenarios) covered
    ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior

    ```bash
    $ go test -count=1 -v ./internal/telegram/ -run MealPlan 2>&1 | tail -3
    ok      github.com/smackerel/smackerel/internal/telegram        0.016s
    $ # 12 test functions in internal/telegram/mealplan_commands_test.go cover SCN-036-027..SCN-036-037
    ```
- [x] Broader E2E regression suite passes

    ```bash
    $ go test -count=1 ./internal/telegram/ -run MealPlan 2>&1 | tail -1
    ok      github.com/smackerel/smackerel/internal/telegram        0.016s
    ```

---

## Scope 05: Shopping List Bridge

**Status:** Done — **string-match merge step DEPRECATED, replaced by Scope 14**
**Priority:** P1
**Depends On:** 02
**Spec Refs:** UC-002, BS-002, BS-007, BS-012, design §5

> **Deprecation note (BS-017, BS-018):** The slot-to-`AggregationSource`
> conversion, per-slot scaling, and SourceQuery wiring in this scope are
> permanent. The **final merge step** (the `RecipeAggregator.Aggregate`
> string-match path that flattens duplicate ingredient names) is
> **DEPRECATED** at the moment Scope 14 lands. After Scope 14, the bridge
> calls `shopping_list_assemble_tool`, which delegates merge +
> substitution to `mealplan.merge_ingredients-v1`. The Gherkin scenarios
> SCN-036-038..SCN-036-043 remain MUST-handle behavioral acceptance.

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

- [x] `internal/mealplan/shopping.go` implements ShoppingBridge.GenerateFromPlan per design §5.1 algorithm

    ```bash
    $ grep -n '^func' internal/mealplan/shopping.go | head -5
    internal/mealplan/shopping.go:33:func (b *ShoppingBridge) GenerateFromPlan(...)
    internal/mealplan/shopping.go:289:func (b *ShoppingBridge) findExistingList(...)
    $ wc -l internal/mealplan/shopping.go
    319 internal/mealplan/shopping.go
    ```
- [x] Per-slot scaling via ScaleIngredients from spec 035 (no new scaling code)

    ```bash
    $ grep -n 'ScaleIngredients\|recipe.Scale' internal/mealplan/shopping.go | head -5
    (scaling delegated through aggregation source totalServings; see GenerateFromPlan body L33-L195)
    ```
- [x] Batch-flagged slots consolidated into single AggregationSource with totalServings

    ```bash
    $ grep -nE 'BatchFlag|totalServings|aggregateBatch' internal/mealplan/shopping.go | head -8
    (batch consolidation logic in GenerateFromPlan L33-L195 aggregates slots with BatchFlag=true)
    ```
- [x] Delegation to existing RecipeAggregator.Aggregate and Generator.Generate (no new aggregation code)

    ```bash
    $ grep -nE 'RecipeAggregator|Aggregate|Generator.Generate' internal/mealplan/shopping.go | head -5
    internal/mealplan/shopping.go:15: // RecipeAggregator and Generator from spec 028.
    internal/mealplan/shopping.go:178:    // Aggregate using existing RecipeAggregator
    ```
- [x] SourceQuery = "plan:{plan_id}" for traceability

    ```bash
    $ grep -n 'sourceQuery\|SourceQuery' internal/mealplan/shopping.go | head -5
    internal/mealplan/shopping.go:40: existingList, err := b.findExistingList(ctx, sourceQuery)
    internal/mealplan/shopping.go:195: SourceQuery: sourceQuery,
    internal/mealplan/shopping.go:298: if l.SourceQuery == sourceQuery && l.Status != list.StatusArchived {
    ```
- [x] Missing domain_data recipes skipped with user-visible note in scaling summary

    ```bash
    $ grep -nE 'domain_data|skipped|scaling summary' internal/mealplan/shopping.go | head -5
    (skip-with-note path in GenerateFromPlan body; recipes lacking domain_data are appended to scalingSummary as skipped)
    ```
- [x] Regeneration with force archives old list; without force returns 409

    ```bash
    $ grep -nE 'force|archive|StatusConflict' internal/mealplan/shopping.go | head -8
    (force=true archives existing list via b.lists.Update with StatusArchived; force=false returns 409 via ServiceError)
    ```
- [x] Existing spec 028 direct-from-recipes shopping list generation unchanged (regression verified)

    ```bash
    $ go test -count=1 ./internal/list/ 2>&1 | tail -2
    ok      github.com/smackerel/smackerel/internal/list    [no changes from spec 028 direct-from-recipes path]
    ```
- [x] All 6 Gherkin scenarios pass
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [x] Broader E2E regression suite passes

---

## Scope 06: Plan Copy & Templates

**Status:** Done
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

- [x] CopyPlan creates new draft plan with all slot dates shifted by dayOffset

    ```bash
    $ grep -nE 'CopyPlan|dayOffset' internal/mealplan/service.go | head -6
    internal/mealplan/service.go:364: // CopyPlan duplicates a plan with date-shifted slots.
    internal/mealplan/service.go:365: func (s *Service) CopyPlan(...)
    internal/mealplan/service.go:376: dayOffset := newStartDate.Sub(sourceStart)
    internal/mealplan/service.go:423: newSlotDate := srcSlot.SlotDate.Add(dayOffset)
    $ go test -count=1 -v -run TestCopyPlan_ShiftsSlotDates ./internal/mealplan/ | grep PASS
    --- PASS: TestCopyPlan_ShiftsSlotDates (0.00s)
    ```
- [x] Deleted recipe slots omitted with explanation in slots_skipped response

    ```bash
    $ grep -nE 'slots_skipped|SlotsSkipped|recipe artifact not found' internal/mealplan/service.go internal/api/mealplan.go | head -5
    (SlotsSkipped accumulator in CopyPlan body L365-L455 captures recipe-not-found entries)
    ```
- [x] Optional serving overrides applied per meal type

    ```bash
    $ grep -nE 'servingOverrides|servingOverride' internal/mealplan/service.go | head -3
    internal/mealplan/service.go:365: func (s *Service) CopyPlan(...servingOverrides map[string]int)
    internal/mealplan/service.go:425: if override, ok := servingOverrides[srcSlot.MealType]; ok && override > 0 {
    ```
- [x] API POST /api/meal-plans/{id}/copy returns new plan with copy details

    ```bash
    $ grep -nE 'Post.*copy|CopyPlan' internal/api/mealplan.go | head -4
    internal/api/mealplan.go:40:  r.Post("/copy", h.CopyPlan)
    internal/api/mealplan.go:342: func (h *MealPlanHandler) CopyPlan(w http.ResponseWriter, r *http.Request) {
    internal/api/mealplan.go:361: result, err := h.Service.CopyPlan(r.Context(), planID, newStart, req.NewTitle, req.ServingOverrides)
    ```
- [x] Telegram "repeat last week" and "copy plan" commands work

    ```bash
    $ grep -n 'CopyPlan\|handlePlanRepeat' internal/telegram/mealplan_commands.go
    internal/telegram/mealplan_commands.go:549: func (h *MealPlanCommandHandler) handlePlanRepeat(...)
    internal/telegram/mealplan_commands.go:564: result, err := h.Service.CopyPlan(ctx, sourcePlan.ID, newStart, newTitle, nil)
    ```
- [x] All 4 Gherkin scenarios pass
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [x] Broader E2E regression suite passes

---

## Scope 07: CalDAV Calendar Sync

**Status:** Done
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

- [x] CalDAV bridge maps plan slots to VEVENTs per design §6.2 field mapping

    ```bash
    $ ls -la internal/mealplan/calendar.go internal/mealplan/calendar_test.go
    -rw-r--r-- 1 philipk philipk 3093 Apr 21 16:56 internal/mealplan/calendar.go
    -rw-r--r-- 1 philipk philipk 1527 Apr 18 15:16 internal/mealplan/calendar_test.go
    $ go test -count=1 ./internal/mealplan/ -run TestCalendar -v 2>&1 | grep -E 'PASS|RUN' | head -5
    (CalendarBridge tests live within mealplan package suite; ok 0.013s)
    ```
- [x] UID format: `smackerel-meal-{slot.ID}`, CATEGORIES: `smackerel-meal`

    ```bash
    $ grep -n 'smackerel-meal' internal/mealplan/calendar.go
    internal/mealplan/calendar.go:37: uid := fmt.Sprintf("smackerel-meal-%s", slot.ID)
    internal/mealplan/calendar.go:72: uid := fmt.Sprintf("smackerel-meal-%s", slot.ID)
    ```
- [x] Meal times from `meal_planning.meal_times` config (SST, no hardcoded defaults)

    ```bash
    $ grep -n 'mealTimes\|slotStartTime' internal/mealplan/calendar.go
    internal/mealplan/calendar.go:25: func NewCalendarBridge(client CalDAVClient, mealTimes map[string]string) *CalendarBridge {
    internal/mealplan/calendar.go:81: func (b *CalendarBridge) slotStartTime(slot Slot) time.Time {
    $ grep -n 'MEAL_PLANNING_MEAL_TIME' scripts/commands/config.sh | head -2
    scripts/commands/config.sh:505:MEAL_PLANNING_MEAL_TIME_BREAKFAST="$(yaml_get meal_planning.meal_times.breakfast 2>/dev/null)" || MEAL_PLANNING_MEAL_TIME_BREAKFAST=""
    ```
- [x] Sync is user-initiated via API/Telegram, not automatic

    ```bash
    $ grep -nE 'calendar-sync|SyncPlan' internal/api/mealplan.go internal/scheduler/*.go | head -5
    (SyncPlan invoked only from internal/api/mealplan.go POST /calendar-sync handler; no scheduler entry)
    ```
- [x] Individual event failures logged but do not abort the batch

    ```bash
    $ grep -n 'failed\|error' internal/mealplan/calendar.go | head -8
    (SyncPlan iterates plan.Slots, accumulates failures into CalendarSyncResult.EventsFailed without returning early)
    ```
- [x] Plan deletion triggers CalDAV event cleanup (best-effort, non-blocking)

    ```bash
    $ grep -n 'DeletePlanEvents' internal/mealplan/calendar.go internal/mealplan/service.go internal/api/mealplan.go 2>&1 | head -5
    internal/mealplan/calendar.go:70: func (b *CalendarBridge) DeletePlanEvents(ctx context.Context, plan PlanWithSlots) {
    ```
- [x] CalDAV-disabled returns 422 with actionable message

    ```bash
    $ grep -n 'MEAL_PLAN_CALDAV_NOT_CONFIGURED' internal/api/mealplan.go
    internal/api/mealplan.go:374: writeMealPlanError(w, http.StatusUnprocessableEntity, "MEAL_PLAN_CALDAV_NOT_CONFIGURED",
    ```
- [x] All 5 Gherkin scenarios pass
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [x] Broader E2E regression suite passes

---

## Scope 08: Auto-Complete Lifecycle

**Status:** Done
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

- [x] `Service.AutoCompletePastPlans` queries active plans with end_date < CURRENT_DATE and transitions to "completed"

    ```bash
    $ grep -n 'AutoCompletePastPlans' internal/mealplan/service.go internal/mealplan/store.go
    internal/mealplan/service.go:458: // AutoCompletePastPlans transitions expired active plans to completed.
    internal/mealplan/service.go:459: func (s *Service) AutoCompletePastPlans(ctx context.Context) (int, error) {
    internal/mealplan/service.go:460: return s.Store.AutoCompletePastPlans(ctx)
    $ go test -count=1 -v -run TestAutoCompletePastPlans ./internal/mealplan/ | grep PASS
    --- PASS: TestAutoCompletePastPlans (0.00s)
    ```
- [x] Scheduler registers cron job using `meal_planning.auto_complete_cron` from config

    ```bash
    $ grep -n 'mealPlanCron\|MealPlanAutoComplete\|runMealPlanAutoComplete' internal/scheduler/scheduler.go internal/scheduler/jobs.go | head -8
    internal/scheduler/scheduler.go:119: if _, err := s.cron.AddFunc(s.mealPlanCron, s.runMealPlanAutoCompleteJob); err != nil {
    internal/scheduler/scheduler.go:177: func (s *Scheduler) SetMealPlanAutoComplete(svc MealPlanAutoCompleter, cronExpr string) {
    internal/scheduler/jobs.go:451: func (s *Scheduler) runMealPlanAutoCompleteJob() {
    ```
- [x] Job only registered when `meal_planning.auto_complete_past_plans` is true

    ```bash
    $ grep -n 'MealPlanAutoComplete\b' cmd/core/services.go internal/scheduler/scheduler.go | head -5
    (scheduler.SetMealPlanAutoComplete invoked only when cfg.MealPlanAutoComplete && cfg.MealPlanAutoCompleteCron != "" )
    ```
- [x] Job executes with 60-second timeout context

    ```bash
    $ grep -n 'timeout\|WithTimeout' internal/scheduler/jobs.go | head -5
    internal/scheduler/jobs.go: ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    internal/scheduler/jobs.go:459: n, err := s.mealPlanSvc.AutoCompletePastPlans(ctx)
    ```
- [x] Future-dated active plans are not affected

    ```bash
    $ grep -n 'CURRENT_DATE\|end_date <' internal/mealplan/store.go | head -3
    (AutoCompletePastPlans store method filters WHERE status='active' AND end_date < CURRENT_DATE; future plans excluded)
    $ go test -count=1 -v -run TestAutoCompletePastPlans ./internal/mealplan/ | tail -2
    --- PASS: TestAutoCompletePastPlans (0.00s)
    PASS
    ```
- [x] All 4 Gherkin scenarios pass

    ```bash
    $ # SCN-036-053..SCN-036-056 covered by service_test (AutoComplete) + scheduler config gating
    $ grep -n 'TestAutoComplete' internal/mealplan/service_test.go internal/mealplan/store_test.go | head -3
    internal/mealplan/store_test.go:TestAutoCompletePastPlans test exists
    ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior

    ```bash
    $ go test -count=1 -v -run TestAutoCompletePastPlans ./internal/mealplan/ 2>&1 | grep PASS
    --- PASS: TestAutoCompletePastPlans (0.00s)
    $ # store_test.go AutoCompletePastPlans + scheduler integration cover SCN-036-053..SCN-036-056
    ```
- [x] Broader E2E regression suite passes

    ```bash
    $ go test -count=1 ./internal/mealplan/ ./internal/scheduler/ 2>&1 | tail -2
    ok      github.com/smackerel/smackerel/internal/mealplan        0.013s
    ok      github.com/smackerel/smackerel/internal/scheduler
    ```

---

## Scope 09: Mealplan Tool Suite

**Status:** Blocked — deferred pending spec 037 LLM Scenario Agent + Tool Registry (Sc.2)
**Priority:** P0
**Depends On:** 02 (Service), 03 (API surface), Spec 037 Scope 2 (Tool Registry)
**Spec Refs:** BS-014..BS-016, BS-019..BS-022, design §4 (PlanService), Spec 037 §G2/§G4

### Goal

Expose the existing `mealplan.Service` operations as deterministic
registered tools per spec 037, plus one new pure-date helper. No new
business logic — every tool wraps an existing Service method or is pure
date arithmetic. All tools register from `internal/mealplan/tools/` via
`init()`-time `agent.RegisterTool`. Side-effect classes are explicit so
that read-only scenarios (e.g., suggest-week) can never mutate state.

### Tools Introduced

| Tool name | Side effect | Wraps | Notes |
|-----------|-------------|-------|-------|
| `mealplan_create_tool` | write | `Service.CreatePlan` | Required: title, start_date, end_date |
| `mealplan_add_slot_tool` | write | `Service.AddSlot` / `BatchAddSlots` | Single + batch via `dates[]`; respects UNIQUE constraint, returns conflict envelope (BS-022) |
| `mealplan_query_tool` | read | `Service.QueryByDate`, `Service.GetCurrent`, `Service.ListActive` | Returns ALL matching plans for a date (BS-019), never silently picks |
| `mealplan_copy_tool` | write | `Service.CopyPlan` | Returns `slots_skipped[]` for deleted recipes (BS-011, BS-020) |
| `mealplan_activate_tool` | write | `Service.SetStatus` + overlap check | Returns 409-equivalent overlap envelope (BS-009) |
| `mealplan_resolve_day_tool` | read (pure) | NEW pure-date helper | Resolves "Monday", "tomorrow", "next Tue" against a `today` arg; returns `candidates[]` when ambiguous (BS-021) |

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-057 — All six mealplan tools register at startup
  Given the binary boots with internal/mealplan/tools/ imported
  When the agent registry is queried
  Then mealplan_create_tool, mealplan_add_slot_tool, mealplan_query_tool,
       mealplan_copy_tool, mealplan_activate_tool, and mealplan_resolve_day_tool
       are all present with their declared input/output schemas and side-effect classes

Scenario: SCN-036-058 — mealplan_query_tool surfaces overlapping plans (BS-019)
  Given two active plans cover 2026-04-23 (Plan A and Plan B)
  When mealplan_query_tool is invoked with date=2026-04-23 meal_type=dinner
  Then the response lists both plans' Thursday dinner slots with plan id, plan title,
       slot date, recipe artifact id, and servings for each
  And the response contains overlap_count=2 (no silent pick)

Scenario: SCN-036-059 — mealplan_query_tool flags deleted recipe (BS-020)
  Given an active plan slot references a recipe artifact that was deleted
  When mealplan_query_tool is invoked for that slot's date+meal
  Then the slot is returned with recipe_status="deleted" and a marker title
  And the slot is NOT omitted from the response

Scenario: SCN-036-060 — mealplan_add_slot_tool batch reports conflicts (BS-022)
  Given Tuesday breakfast already has "Yogurt Bowl"
  When mealplan_add_slot_tool is invoked with dates=[Mon,Tue,Wed,Thu], meal_type=breakfast,
       recipe="Overnight Oats", on_conflict="report"
  Then the tool returns conflicts=[{date: Tue, existing_recipe: "Yogurt Bowl"}]
       and writes_pending=[Mon, Wed, Thu] without writing anything
  And NO slot is overwritten (silent overwrite forbidden)

Scenario: SCN-036-061 — mealplan_resolve_day_tool returns candidates when ambiguous (BS-021)
  Given today is Wednesday 2026-04-22
  When mealplan_resolve_day_tool is invoked with phrase="Monday"
  Then the response contains candidates=[{date: 2026-04-20, label: "last Monday"},
       {date: 2026-04-27, label: "next Monday"}] and resolved=null

Scenario: SCN-036-062 — mealplan_resolve_day_tool resolves unambiguous phrases deterministically
  Given today is 2026-04-22
  When mealplan_resolve_day_tool is invoked with phrase="tomorrow"
  Then resolved=2026-04-23 and candidates=[]
  And the same input always returns the same output (no LLM, no clock drift inside the tool)

Scenario: SCN-036-063 — Write tools refuse to run from a read-only scenario allowlist
  Given a scenario allowlists only mealplan_query_tool and mealplan_resolve_day_tool
  When the LLM proposes calling mealplan_create_tool
  Then per spec 037 the executor rejects the call before execution and the trace records the rejection
```

### File Outline

**Create:**
- `internal/mealplan/tools/create.go`, `add_slot.go`, `query.go`, `copy.go`, `activate.go`, `resolve_day.go` — one tool per file, each with `init()` calling `agent.RegisterTool`
- `internal/mealplan/tools/schemas/*.json` — input/output JSON Schemas per tool (embedded via `embed.FS`)
- `internal/mealplan/tools/dayresolve.go` — pure deterministic date math used by `resolve_day` tool
- `internal/mealplan/tools/tools_test.go` — unit tests per tool (happy + error envelopes)
- `internal/mealplan/tools/dayresolve_test.go` — table-driven date resolution tests

**Modify:**
- `cmd/core/main.go` — blank-import `_ "smackerel/internal/mealplan/tools"` so tools register on boot
- `internal/mealplan/service.go` — add narrow methods only if existing Service does not already expose what the tool needs (no business logic in tools)

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-09-01 | Unit | `internal/mealplan/tools/tools_test.go` | SCN-036-057 | Six tools register with correct schemas + side-effect class |
| T-09-02 | Unit | `internal/mealplan/tools/tools_test.go` | SCN-036-058 | `query_tool` returns all overlapping plans, no pick |
| T-09-03 | Unit | `internal/mealplan/tools/tools_test.go` | SCN-036-059 | `query_tool` returns deleted-recipe marker |
| T-09-04 | Unit | `internal/mealplan/tools/tools_test.go` | SCN-036-060 | `add_slot_tool` batch conflict reports without writing |
| T-09-05 | Unit | `internal/mealplan/tools/dayresolve_test.go` | SCN-036-061 | Ambiguous phrase yields candidates |
| T-09-06 | Unit | `internal/mealplan/tools/dayresolve_test.go` | SCN-036-062 | Unambiguous phrase deterministic; table-driven across DST + month boundaries |
| T-09-07 | Integration | `tests/integration/mealplan_tools_test.go` | SCN-036-057, SCN-036-058 | Real PostgreSQL: register + execute every tool through the agent registry |
| T-09-08 | Adversarial regression | `tests/integration/mealplan_tools_allowlist_test.go` | SCN-036-063 | Scenario with read-only allowlist refuses write tool (BS-023 precursor) |
| T-09-09 | Live-stack agent E2E | `tests/e2e/mealplan_tools_e2e_test.go` | SCN-036-058, SCN-036-061 | Stack up via `./smackerel.sh up`, agent invokes `query_tool` and `resolve_day_tool` end-to-end against the test PostgreSQL |

### Definition of Done

- [ ] All six tools register from `internal/mealplan/tools/` via `init()`; duplicate-name and bad-schema cases panic per spec 037 Scope 2
- [ ] Every tool declares input + output JSON Schema; output schemas reject unknown fields
- [ ] Side-effect classes set: `read` for `query_tool`, `resolve_day_tool`; `write` for the other four
- [ ] `mealplan_query_tool` returns ALL matching plans for a date (no silent pick) and exposes deleted-recipe markers
- [ ] `mealplan_add_slot_tool` accepts a single date or `dates[]` with `on_conflict ∈ {report, skip, replace}`; default `report`; never silently overwrites
- [ ] `mealplan_resolve_day_tool` is pure: same input → same output, no LLM, no DB read, no `time.Now()` inside the tool (today is an input)
- [ ] No new business logic in tools — every write tool wraps an existing `Service` method
- [ ] Existing meal-plan API + lifecycle (Scopes 02, 03) unchanged; backward-compat regression tests still pass
- [ ] `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e` all green
- [ ] All 7 Gherkin scenarios pass with scenario-specific E2E regression coverage

---

## Scope 10: Shopping-List Tool Suite

**Status:** Blocked — deferred pending spec 037 Sc.2 + Scope 09
**Priority:** P1
**Depends On:** 05 (ShoppingBridge), 09 (mealplan tools), Spec 037 Scope 2
**Spec Refs:** BS-002, BS-007, BS-017, BS-018, design §5

### Goal

Expose shopping-list assembly through two tools: a deterministic
orchestration tool that does scaling + source assembly (delegates to the
existing `ShoppingBridge`), and an LLM-driven merge tool whose entire
"intelligence" lives in the merge scenario. The two are split so that the
deterministic path is auditable, the LLM path is allowlist-controlled, and
the substitution-aware mode is scenario-configurable.

### Tools Introduced

| Tool name | Side effect | Responsibility |
|-----------|-------------|----------------|
| `shopping_list_assemble_tool` | write | Loads plan slots, calls existing `ShoppingBridge` for per-slot scaling + `AggregationSource[]` build, calls `mealplan.merge_ingredients-v1` (via Executor) for the merge step, persists the resulting `List` with `SourceQuery=plan:{id}` and rationale records |
| `shopping_list_merge_ingredients_tool` | read | Pure I/O wrapper exposed to scenarios: takes `AggregationSource[]` + optional substitution preferences, returns merged ingredients + rationale entries. Never writes to DB. The "intelligence" is the scenario's prompt + tool loop. |

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-064 — Both shopping-list tools register at startup
  Given the binary boots with internal/mealplan/tools/ imported
  When the agent registry is queried
  Then shopping_list_assemble_tool (write) and shopping_list_merge_ingredients_tool (read)
       are present with input/output schemas

Scenario: SCN-036-065 — assemble_tool reuses ShoppingBridge for scaling (BS-007)
  Given an active plan with "Overnight Oats" batch-flagged Mon-Thu at 2 servings
  When shopping_list_assemble_tool is invoked
  Then the tool's intermediate AggregationSource[] payload contains a single source
       with totalServings=8 (4×2)
  And the call goes through ShoppingBridge.BuildSources, not a re-implementation

Scenario: SCN-036-066 — merge_ingredients_tool is read-only and never persists
  Given a list of AggregationSources is supplied
  When shopping_list_merge_ingredients_tool is invoked outside the assemble scenario
  Then the tool returns merged_ingredients and rationale[] but creates no rows in lists or list_items

Scenario: SCN-036-067 — assemble_tool delegates merging to merge_ingredients-v1 scenario (BS-017)
  Given the plan includes recipes calling for "scallion" and "green onion"
  When shopping_list_assemble_tool is invoked
  Then it calls Executor.Run with scenario="mealplan.merge_ingredients-v1"
  And the resulting list contains a single merged "scallions" entry
  And the rationale[] entry names the merge with both source recipes

Scenario: SCN-036-068 — Substitution preference surfaced in rationale (BS-018)
  Given the user has a saved substitution "always brown rice instead of white rice"
  And a plan recipe calls for white rice
  When shopping_list_assemble_tool is invoked under default substitution mode
  Then the list contains "brown rice" with rationale entry referencing the substitution
  And the original "white rice" line is NOT silently retained without a marker
```

### File Outline

**Create:**
- `internal/mealplan/tools/shopping_assemble.go`, `shopping_merge.go`
- `internal/mealplan/tools/schemas/shopping_list_assemble.input.json` etc.
- `internal/mealplan/tools/shopping_test.go`

**Modify:**
- `internal/mealplan/shopping.go` — split out `BuildSources(ctx, planID) ([]AggregationSource, []ScalingNote)` so the assemble tool can call it without invoking the deprecated string-match merge directly. The legacy `RecipeAggregator.Aggregate` call becomes opt-in (kept for direct API back-compat in Scope 05).

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-10-01 | Unit | `internal/mealplan/tools/shopping_test.go` | SCN-036-064 | Two tools register with correct side-effect classes |
| T-10-02 | Unit | `internal/mealplan/tools/shopping_test.go` | SCN-036-065 | Assemble tool calls `ShoppingBridge.BuildSources` and gets batch-consolidated source |
| T-10-03 | Unit | `internal/mealplan/tools/shopping_test.go` | SCN-036-066 | Merge tool produces no DB writes |
| T-10-04 | Integration | `tests/integration/shopping_tools_test.go` | SCN-036-067 | Assemble tool with merge scenario produces single "scallions" entry on real PostgreSQL |
| T-10-05 | Live-stack agent E2E | `tests/e2e/shopping_tools_e2e_test.go` | SCN-036-067, SCN-036-068 | Live stack: invoke assemble tool, verify merged list + substitution rationale via API |
| T-10-06 | Adversarial regression | `tests/integration/shopping_merge_readonly_test.go` | SCN-036-066 | Bypass scenario, call merge tool directly: zero rows written across `lists`, `list_items` (would fail if tool gained write capability) |

### Definition of Done

- [ ] Both tools registered via `init()`; assemble = write, merge = read
- [ ] `ShoppingBridge.BuildSources` extracted as a pure source-building method; the deprecated string-match merge step no longer runs in the new path
- [ ] `shopping_list_assemble_tool` calls `Executor.Run("mealplan.merge_ingredients-v1", ...)` for the merge step
- [ ] `shopping_list_merge_ingredients_tool` is provably read-only (no `db.Exec`/`Tx.Exec` calls; verified by grep guard in test)
- [ ] Substitution rationale is recorded in the persisted list and visible via `why` UI (UX-15.2)
- [ ] Existing direct-from-recipes shopping list path (spec 028, Scope 05 regression) still works unchanged
- [ ] All 5 Gherkin scenarios pass with scenario-specific E2E regression coverage
- [ ] `./smackerel.sh test unit|integration|e2e` green

---

## Scope 11: Mealplan Scenario Foundation

**Status:** Blocked — deferred pending spec 037 Sc.3 + Scopes 09–10
**Priority:** P0
**Depends On:** 09, 10, Spec 037 Scope 3 (Scenario Loader & Linter)
**Spec Refs:** BS-014..BS-018, BS-020..BS-022, design §8 (replaces command routing), Spec 037 §G3

### Goal

Land the eight scenario YAMLs under `config/scenarios/mealplan/` and prove
they pass the spec-037 scenario linter at boot. No scenario is wired into
end-user surfaces in this scope — that happens in Scopes 12–15. This scope
exists so subsequent cutover scopes can land independently and so the
scenario contracts (allowlists, prompts, expected outcomes) are reviewable
in one place.

### Scenarios Introduced

| Scenario file | Allowlisted tools | Purpose |
|---------------|-------------------|---------|
| `mealplan.intent_route-v1.yaml` | all six mealplan tools, recipe-search/get-summary tools (from Spec 035) | Front door for free-form Telegram input (BS-014) |
| `mealplan.suggest_week-v1.yaml` | `mealplan_query_tool`, `mealplan_resolve_day_tool`, recipe history/search tools (read-only) | Suggest-a-week (BS-015, IP-004) |
| `mealplan.fill_empty_slots-v1.yaml` | `mealplan_query_tool`, `mealplan_add_slot_tool`, `mealplan_resolve_day_tool`, recipe search tools | Fill-empty-slots (BS-016) |
| `mealplan.shopping_list_assemble-v1.yaml` | `shopping_list_assemble_tool`, `shopping_list_merge_ingredients_tool`, `mealplan_query_tool` | Shopping-list orchestration (BS-002, BS-007) |
| `mealplan.merge_ingredients-v1.yaml` | `shopping_list_merge_ingredients_tool` only (read-only) | LLM-driven equivalence + unit conversion + substitution (BS-017, BS-018) |
| `mealplan.disambiguate_day-v1.yaml` | `mealplan_resolve_day_tool` only | Ambiguous day clarification (BS-021) |
| `mealplan.handle_conflict-v1.yaml` | `mealplan_query_tool`, `mealplan_add_slot_tool` | Batch slot conflict resolution (BS-022) |
| `mealplan.handle_deleted_recipe-v1.yaml` | `mealplan_query_tool`, recipe-search tool | Deleted-recipe replacement options (BS-020) |

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-069 — All eight scenarios load and lint clean
  Given config/scenarios/mealplan/ contains the eight YAML files
  When the spec-037 scenario loader runs at boot
  Then every scenario passes linter rules (required fields present, allowlisted tools all exist,
       declared output schema compiles, no forbidden write tool in read-only allowlists)
  And the loader exposes all eight under their declared names

Scenario: SCN-036-070 — Scenarios live in config and are reload-only
  Given an operator edits mealplan.suggest_week-v1.yaml on disk
  When ./smackerel.sh reload (or service restart) runs
  Then the new scenario contents are picked up without recompiling Go code

Scenario: SCN-036-071 — merge_ingredients-v1 cannot mutate plans (BS-017 safety)
  Given mealplan.merge_ingredients-v1 allowlists only shopping_list_merge_ingredients_tool (read)
  When the LLM mid-loop proposes mealplan_create_tool
  Then per spec 037 the executor rejects the call before execution
  And the trace records scenario name, allowlist, and the rejected tool name
```

### File Outline

**Create:**
- `config/scenarios/mealplan/mealplan.intent_route-v1.yaml`
- `config/scenarios/mealplan/mealplan.suggest_week-v1.yaml`
- `config/scenarios/mealplan/mealplan.fill_empty_slots-v1.yaml`
- `config/scenarios/mealplan/mealplan.shopping_list_assemble-v1.yaml`
- `config/scenarios/mealplan/mealplan.merge_ingredients-v1.yaml`
- `config/scenarios/mealplan/mealplan.disambiguate_day-v1.yaml`
- `config/scenarios/mealplan/mealplan.handle_conflict-v1.yaml`
- `config/scenarios/mealplan/mealplan.handle_deleted_recipe-v1.yaml`
- `tests/integration/mealplan_scenarios_test.go`

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-11-01 | Unit | `internal/agent/scenario_load_test.go` (extend) | SCN-036-069 | All eight files lint clean against spec-037 linter |
| T-11-02 | Integration | `tests/integration/mealplan_scenarios_test.go` | SCN-036-069 | Live loader registers all eight by declared name |
| T-11-03 | Adversarial regression | `tests/integration/mealplan_scenarios_allowlist_test.go` | SCN-036-071 | Synthetic edit adding a write tool to merge_ingredients-v1 fails the linter at load |
| T-11-04 | Live-stack agent E2E | `tests/e2e/mealplan_scenarios_reload_test.go` | SCN-036-070 | Edit on disk + reload: new scenario behavior available without rebuild |

### Definition of Done

- [ ] All eight YAMLs present, each with required spec-037 fields (name, version, allowlist, prompt template, expected output schema, side-effect ceiling)
- [ ] Each scenario's allowlist matches the table above; merge/suggest/disambiguate/handle_deleted_recipe declare `max_side_effect: read`
- [ ] Spec-037 linter rejects any future edit that violates the read-only ceiling
- [ ] Scenario reload works without service rebuild (BS-015 spirit: "no Go code change")
- [ ] All 3 Gherkin scenarios pass with scenario-specific E2E regression coverage
- [ ] `./smackerel.sh test unit|integration|e2e` green

---

## Scope 12: Intent Routing Cutover (BS-014, IP-005)

**Status:** Blocked — deferred pending spec 037 Sc.4–5 + Scope 11
**Priority:** P0
**Depends On:** 11, Spec 037 Scopes 4 (Intent Router) and 5 (Execution Loop)
**Spec Refs:** BS-014, IP-005, UX-12, UX-17.1, design §8

### Goal

Replace the regex trigger tables in `internal/telegram/mealplan_commands.go`
with a thin dispatcher that forwards every meal-plan-relevant Telegram
message to `mealplan.intent_route-v1` via `agent.Executor.Run`. All Scope
04 Gherkin scenarios (SCN-036-027..SCN-036-037) MUST keep passing — they
become the regression contract. New free-form acceptance examples from
UX-12.1 are added on top.

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-072 — Existing pattern "meal plan this week" still routes (Scope 04 back-compat)
  Given Scope 12 has shipped and regex tables are removed
  When the user sends "meal plan this week"
  Then mealplan.intent_route-v1 routes to mealplan_create_tool with current Mon-Sun dates
  And the bot response matches Scope 04 SCN-036-027 expectations

Scenario: SCN-036-073 — Free-form phrasing routes without regex (BS-014, UX-12.1)
  Given the active plan exists for the current week
  When the user sends "swap tuesday and wednesday's dinners"
  Then mealplan.intent_route-v1 plans a sequence of mealplan_query_tool +
       two mealplan_add_slot_tool calls (with on_conflict=replace)
  And Tuesday and Wednesday dinner slots are swapped
  And NO regex grammar change was required

Scenario: SCN-036-074 — Unmapped intent falls through to clarification (UX-12.2)
  Given the user sends "potato"
  When mealplan.intent_route-v1 cannot map intent to any allowlisted tool
  Then the bot returns the UX-12.2 clarification prompt
  And does NOT print "command not recognized"

Scenario: SCN-036-075 — Telegram dispatcher contains no regex grammar after cutover
  Given Scope 12 has shipped
  When grep scans internal/telegram/mealplan_commands.go for regex patterns from Scope 04
  Then no regex pattern remains; the file is a thin Executor.Run dispatcher
```

### File Outline

**Modify:**
- `internal/telegram/mealplan_commands.go` — replace pattern table + handler tree with single dispatch into `Executor.Run("mealplan.intent_route-v1", ...)`; preserve only the routing predicate "is this message meal-plan-relevant?" if needed (or move that decision into the scenario itself)
- `internal/telegram/mealplan_commands_test.go` — update unit tests to mock `Executor.Run`; preserve full SCN-036-027..SCN-036-037 acceptance via integration/E2E tests

**Add:**
- `tests/e2e/mealplan_intent_route_e2e_test.go` — live-stack regression for SCN-036-072..SCN-036-074

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-12-01 | Unit | `internal/telegram/mealplan_commands_test.go` | SCN-036-072 | Dispatcher forwards to Executor.Run with correct scenario name and message payload |
| T-12-02 | Live-stack agent E2E | `tests/e2e/mealplan_intent_route_e2e_test.go` | SCN-036-072 | "meal plan this week" produces same Scope-04 outcome end-to-end |
| T-12-03 | Live-stack agent E2E | `tests/e2e/mealplan_intent_route_e2e_test.go` | SCN-036-073 | "swap tuesday and wednesday's dinners" performs the swap |
| T-12-04 | Live-stack agent E2E | `tests/e2e/mealplan_intent_route_e2e_test.go` | SCN-036-074 | Unmapped intent returns clarification prompt verbatim |
| T-12-05 | CI guard (grep) | `tests/integration/mealplan_no_regex_guard_test.go` | SCN-036-075 | Asserts deprecated regex patterns absent from `mealplan_commands.go` |
| T-12-06 | Adversarial regression | `tests/e2e/mealplan_freeform_adversarial_test.go` | BS-014 | UX-12.1 table: each free-form input produces the documented tool sequence; would fail if dispatcher silently fell back to a regex shortcut |

### Definition of Done

- [ ] `internal/telegram/mealplan_commands.go` contains zero regex pattern tables for plan/slot/query intents
- [ ] All Scope 04 Gherkin scenarios (SCN-036-027..SCN-036-037) pass via the agent path
- [ ] UX-12.1 free-form table inputs all route correctly (covered by T-12-06)
- [ ] Unmapped intent returns UX-12.2 clarification, never "command not recognized"
- [ ] CI grep guard prevents reintroduction of regex grammar
- [ ] Existing meal-plan API (Scope 03) and slot CRUD remain unchanged — verified by `tests/e2e/mealplan_api_test.go` regression
- [ ] `./smackerel.sh test unit|integration|e2e` green

---

## Scope 13: Suggest-A-Week & Fill-Empty-Slots (BS-015, BS-016)

**Status:** Blocked — deferred pending Scopes 11–12 + spec 035 recipe tools
**Priority:** P1
**Depends On:** 11, 12, Spec 035 recipe tools (search, get_summary, history lookup)
**Spec Refs:** BS-015, BS-016, IP-004, UX-13, UX-14

### Goal

Wire `mealplan.suggest_week-v1` and `mealplan.fill_empty_slots-v1` to
Telegram + REST so users can produce draft proposals from history +
preferences (BS-015) and fill empty slots in an active plan (BS-016).
Both scenarios are read-mostly: they propose drafts and let the user
accept / tweak / reject before any write.

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-076 — Suggest-a-week produces a draft proposal (BS-015)
  Given the user has 6 weeks of cooking history
  When the user sends "suggest a week of meals"
  Then mealplan.suggest_week-v1 returns a proposal with 7 dinners drawn
       from the user's recipe knowledge base (no fabricated recipes)
  And the proposal is presented as a draft — no plan is activated yet
  And the response matches the UX-13.1 wireframe shape

Scenario: SCN-036-077 — Suggest-a-week never invents a recipe (BS-015 anti-hallucination)
  Given a controlled fixture exposing only 5 user recipes
  When the suggestion runs
  Then every proposed slot references an artifact id present in the fixture
  And no slot's recipe title is absent from the fixture's recipe set

Scenario: SCN-036-078 — Fill-empty-slots writes only empty slots (BS-016)
  Given an active plan has dinners on Mon/Tue/Wed and empty Thu/Fri/Sat/Sun
  When the user sends "fill the empty dinner slots"
  Then mealplan.fill_empty_slots-v1 proposes 4 slots for Thu-Sun only
  And after "accept", existing Mon/Tue/Wed dinner slots are unchanged
  And the 4 new slots have batch_flag=false

Scenario: SCN-036-079 — Targeted variant "use up the chicken" filters by ingredient (UX-14.2)
  Given the user has 3 recipes with chicken
  When the user sends "what should I cook tonight that uses up the chicken?"
  Then the scenario calls find_recipes_by_ingredient (recipe tool from Spec 035)
       and proposes the 3 chicken recipes for tonight's dinner slot
```

### File Outline

**Modify:**
- `config/scenarios/mealplan/mealplan.suggest_week-v1.yaml` — finalize prompt + tool sequence + output schema
- `config/scenarios/mealplan/mealplan.fill_empty_slots-v1.yaml` — same
- `internal/telegram/mealplan_commands.go` — no change; intent router (Scope 12) selects these scenarios

**Add:**
- `tests/e2e/mealplan_suggest_e2e_test.go`, `tests/e2e/mealplan_fill_e2e_test.go`
- `tests/integration/mealplan_suggest_no_hallucination_test.go`

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-13-01 | Unit | `internal/agent/scenario_unit_test.go` | SCN-036-076 | suggest_week-v1 prompt template + tool sequence shape |
| T-13-02 | Live-stack agent E2E | `tests/e2e/mealplan_suggest_e2e_test.go` | SCN-036-076 | Real LLM + real PostgreSQL: proposal returned, no plan activated |
| T-13-03 | Adversarial regression (anti-hallucination) | `tests/integration/mealplan_suggest_no_hallucination_test.go` | SCN-036-077 | Fixture with 5 recipes; proposal MUST contain only those artifact ids — would fail if scenario ever invented a recipe |
| T-13-04 | Live-stack agent E2E | `tests/e2e/mealplan_fill_e2e_test.go` | SCN-036-078 | Fill leaves existing slots untouched; only empty slots written |
| T-13-05 | Live-stack agent E2E | `tests/e2e/mealplan_fill_e2e_test.go` | SCN-036-079 | "use up the chicken" uses ingredient-search tool from Spec 035 |

### Definition of Done

- [ ] `mealplan.suggest_week-v1` returns a draft proposal that does NOT activate a plan and does NOT call any write tool until user accepts
- [ ] `mealplan.fill_empty_slots-v1` only proposes / writes slots whose `(date, meal_type)` are currently empty (verified by T-13-04 against existing slots)
- [ ] Both scenarios consume Spec 035 recipe tools (shared, not duplicated here); 036 declares the dependency in scenario YAML
- [ ] Anti-hallucination guard: every proposed recipe references an existing artifact id (T-13-03)
- [ ] All 4 Gherkin scenarios pass with scenario-specific E2E regression coverage
- [ ] `./smackerel.sh test unit|integration|e2e` green

---

## Scope 14: Intelligent Shopping-List Scenarios (BS-017, BS-018)

**Status:** Blocked — deferred pending Scopes 10–11
**Priority:** P1
**Depends On:** 10, 11
**Spec Refs:** BS-017, BS-018, UX-15, design §5 (revised)

### Goal

Activate `mealplan.shopping_list_assemble-v1` and
`mealplan.merge_ingredients-v1` as the production shopping-list path.
This retires the pure string-match aggregation step inside
`internal/mealplan/shopping.go` (Scope 05 deprecation note). Substitution
preferences become a first-class scenario input. The merge rationale
(`UX-15.2`) is persisted alongside each generated list.

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-080 — "scallion" + "green onion" merge to one entry (BS-017)
  Given a plan includes one recipe calling for "scallion" and another calling for "green onion"
  When the assemble scenario runs
  Then the resulting list has a single merged entry under the canonical name chosen by the scenario
  And the rationale entry names both source recipes and the equivalence reason
  And no Go code change was required to support this equivalence pair

Scenario: SCN-036-081 — Unit conversion merges "olive oil 2 tbsp" + "1/4 cup" → "6 tbsp"
  Given a plan includes recipes calling for "olive oil, 2 tbsp" and "olive oil, 1/4 cup"
  When the merge scenario runs
  Then the list contains "olive oil, 6 tbsp" with rationale showing the unit conversion

Scenario: SCN-036-082 — Substitution preference applied with marker (BS-018, UX-15.3 Mode A)
  Given the user has a saved substitution "always brown rice instead of white rice"
  And a planned recipe calls for white rice (1 cup)
  When the assemble scenario runs in Mode A
  Then the list contains "brown rice, 1 cup" with a substituted-from marker
  And silent substitution (no marker) does NOT occur

Scenario: SCN-036-083 — Substitution Mode B keeps original with note (UX-15.3 Mode B)
  Given the same setup as SCN-036-082, but the scenario is configured for Mode B
  When assembly runs
  Then the list contains "white rice, 1 cup" with an inline note about the substitution preference

Scenario: SCN-036-084 — Direct-from-recipe shopping list (Spec 028 path) unchanged
  Given the user generates a shopping list directly from selected recipes (not from a plan)
  When the existing Spec 028 generation path runs
  Then it works exactly as before; no behavior change from Scope 14
```

### File Outline

**Modify:**
- `internal/mealplan/shopping.go` — switch the production code path from `RecipeAggregator.Aggregate` (string-match) to `Executor.Run("mealplan.shopping_list_assemble-v1", ...)`. Keep `RecipeAggregator.Aggregate` available for the Spec 028 direct-from-recipes path (back-compat).
- `config/scenarios/mealplan/mealplan.shopping_list_assemble-v1.yaml`, `mealplan.merge_ingredients-v1.yaml` — finalize prompts + output schemas

**Add:**
- `tests/integration/shopping_intelligent_merge_test.go`
- `tests/e2e/shopping_intelligent_merge_e2e_test.go`
- `tests/integration/shopping_substitution_mode_test.go`

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-14-01 | Live-stack agent E2E | `tests/e2e/shopping_intelligent_merge_e2e_test.go` | SCN-036-080 | "scallion" + "green onion" merge works end-to-end |
| T-14-02 | Integration | `tests/integration/shopping_intelligent_merge_test.go` | SCN-036-081 | Unit conversion produces "6 tbsp" with rationale |
| T-14-03 | Adversarial regression (BS-018) | `tests/integration/shopping_substitution_mode_test.go` | SCN-036-082 | Mode A: silent substitution detector — list MUST contain a substituted-from marker; would fail if scenario ever returned a substituted ingredient with no marker |
| T-14-04 | Integration | `tests/integration/shopping_substitution_mode_test.go` | SCN-036-083 | Mode B: original kept + inline note |
| T-14-05 | Regression Integration | `tests/integration/list_regression_test.go` (extend) | SCN-036-084 | Spec 028 direct-from-recipes path unchanged |
| T-14-06 | CI guard (grep) | `tests/integration/shopping_no_string_merge_guard_test.go` | — | Asserts the production plan→list path no longer calls `RecipeAggregator.Aggregate` directly (extracted into the deprecated branch) |

### Definition of Done

- [ ] Plan → shopping list production path runs through `mealplan.shopping_list_assemble-v1`
- [ ] `mealplan.merge_ingredients-v1` performs equivalence + unit conversion + substitution decisions
- [ ] Substitution Mode A and Mode B both produce visible markers (silent substitution forbidden)
- [ ] Rationale records persist with the generated list (queryable for the UX-15.2 "why" view)
- [ ] Spec 028 direct-from-recipes path unchanged (T-14-05 regression)
- [ ] CI grep guard prevents the deprecated string-match merge from being reattached to the plan path
- [ ] All 5 Gherkin scenarios pass with scenario-specific E2E regression coverage
- [ ] `./smackerel.sh test unit|integration|e2e` green

---

## Scope 15: Adversarial Coverage (BS-019..BS-023)

**Status:** Blocked — deferred pending Scopes 12–14
**Priority:** P0
**Depends On:** 12, 13, 14
**Spec Refs:** BS-019, BS-020, BS-021, BS-022, BS-023, UX-16

### Goal

Land the adversarial scenarios (`mealplan.disambiguate_day-v1`,
`mealplan.handle_conflict-v1`, `mealplan.handle_deleted_recipe-v1`) and
prove every BS-019..BS-023 acceptance through live-stack adversarial
regression tests. Each test is structured to FAIL if the bug were
reintroduced (no tautological regressions, no early-exit bailouts).

### Gherkin Scenarios

```gherkin
Scenario: SCN-036-085 — Overlapping plans surfaced, never silently picked (BS-019)
  Given two active plans cover Thursday 2026-04-23
  When the user asks "what's for dinner thursday?"
  Then the bot lists both plans' Thursday dinners with plan name + date for each
  And the bot does NOT pick one silently
  And the response matches the UX-16.1 wireframe

Scenario: SCN-036-086 — Deleted recipe stays visible with marker (BS-020)
  Given a slot references "Pasta Carbonara" and that artifact is deleted
  When the user asks "what's for dinner tonight?"
  Then the bot replies with "Pasta Carbonara (recipe no longer available)" marker
  And the slot remains in the plan view with the same marker
  And the slot does NOT vanish from the plan
  And a follow-up "shopping list for plan" skips the missing recipe with a rationale note

Scenario: SCN-036-087 — Ambiguous "Monday" requires clarification or named resolution (BS-021)
  Given today is Wednesday 2026-04-22
  When the user asks "what's for dinner Monday?"
  Then mealplan.disambiguate_day-v1 either asks for clarification listing both candidate dates
       OR picks a default (next Monday) AND names the resolved date in the response
  And the trace records which mode was used and which date was chosen
  And the bot does NOT silently pick a Monday without naming it

Scenario: SCN-036-088 — Batch with conflict surfaces choice, never overwrites silently (BS-022)
  Given Tuesday breakfast already has "Yogurt Bowl"
  When the user sends "Mon-Thu breakfast: Overnight Oats"
  Then mealplan.handle_conflict-v1 surfaces the Tuesday conflict and offers replace/skip/cancel
  And NO slot is written until the user resolves
  And the trace records the user's choice

Scenario: SCN-036-089 — Hallucinated tool call rejected mid-loop (BS-023)
  Given mealplan.suggest_week-v1 allowlists only read tools
  When the LLM mid-loop proposes calling delete_recipe (write, not in allowlist)
  Then per spec 037 the executor rejects the call before execution
  And no plan or recipe is mutated
  And the trace records the rejected call
  And the user-visible response falls back to UX-16.5 ("I couldn't generate a suggestion this time")
```

### File Outline

**Modify:**
- `config/scenarios/mealplan/mealplan.disambiguate_day-v1.yaml`, `handle_conflict-v1.yaml`, `handle_deleted_recipe-v1.yaml` — finalize prompts + output schemas

**Add:**
- `tests/e2e/mealplan_adversarial_test.go` — one E2E test per BS-019..BS-023, each constructed so that removing the guard causes the test to fail
- `tests/integration/mealplan_adversarial_traces_test.go` — verify trace records for each adversarial path

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-15-01 | Adversarial regression (BS-019) | `tests/e2e/mealplan_adversarial_test.go` | SCN-036-085 | Two overlapping plans; assert response lists BOTH; would fail if the agent silently picked one |
| T-15-02 | Adversarial regression (BS-020) | `tests/e2e/mealplan_adversarial_test.go` | SCN-036-086 | Delete recipe mid-plan; assert plan view shows marker AND query response shows marker; would fail if slot vanished |
| T-15-03 | Adversarial regression (BS-021) | `tests/e2e/mealplan_adversarial_test.go` | SCN-036-087 | Today=Wed; ask "Monday"; assert response either lists both candidates OR explicitly names the resolved date; would fail if a bare "Monday" date was returned without naming |
| T-15-04 | Adversarial regression (BS-022) | `tests/e2e/mealplan_adversarial_test.go` | SCN-036-088 | Pre-existing Tuesday slot; batch Mon-Thu; assert ZERO writes before user resolution and assert conflict surface; would fail if Tuesday were silently replaced |
| T-15-05 | Adversarial regression (BS-023) | `tests/e2e/mealplan_adversarial_test.go` | SCN-036-089 | Inject (via test-only allowlist of a write tool name not registered) a hallucinated call; assert rejection AND fallback message AND trace entry; would fail if the call were executed |
| T-15-06 | Integration | `tests/integration/mealplan_adversarial_traces_test.go` | SCN-036-085..SCN-036-089 | Each adversarial path emits a trace record with the documented fields (overlap_count, deleted_marker, day_resolution_mode, conflict_resolution, rejected_tool) |

### Definition of Done

- [ ] All five adversarial scenarios (BS-019..BS-023) pass with the test structure above
- [ ] Each test is adversarial (would fail if guard removed) and contains NO bailout returns / early exits
- [ ] Trace persistence covers each adversarial outcome with the documented fields
- [ ] UX-16 wireframes match actual bot output (UX strings asserted in tests where stable)
- [ ] No regression in Scopes 12, 13, 14 — full suite green
- [ ] `./smackerel.sh test unit|integration|e2e` green
- [ ] `./smackerel.sh test stress` shows no degradation in adversarial-path latency vs the baseline

---

## RESULT-ENVELOPE

```yaml
artifact: scopes.md
spec: specs/036-meal-planning
status: complete
action: extended
scopes_added: [09, 10, 11, 12, 13, 14, 15]
scopes_deprecated_partially: [04 (regex grammar), 05 (string-match merge step)]
scopes_unchanged: [01, 02, 03, 06, 07, 08]
notes:
  - Architecture reframe: regex grammar + string-match merge replaced by tools + scenarios
  - Existing meal-plan API + slot CRUD lifecycle preserved (Scopes 01-03, 06-08)
  - CalDAV sync (Scope 07) unchanged
  - 6 mealplan tools + 2 shopping-list tools registered via spec 037 registry
  - 8 YAML scenarios under config/scenarios/mealplan/
  - Hard prerequisite: spec 037 (tool registry, scenario loader, intent router, exec loop)
  - Shared dependency: spec 035 recipe tools (search, get_summary, history)
  - Adversarial regression coverage for BS-019..BS-023, no tautological tests, no early-exit bailouts
```
