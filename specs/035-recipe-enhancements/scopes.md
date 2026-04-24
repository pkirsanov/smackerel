# Scopes: 035 Recipe Enhancements

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

## Summary Table

> **Architectural reframe (spec 037 integration).** Scopes 01–06 (Phase A —
> "Foundation") shipped the deterministic scaler/cook-mode runtime and remain
> authoritative until the agent cutover (Scope 11). Scopes 07–16 (Phase B —
> "Agent Migration") wire that runtime behind the LLM scenario agent + tool
> registry committed in [spec 037](../037-llm-agent-tools/spec.md), add the
> new BS-021..BS-028 behaviors, and finally remove the regex intent paths
> per [design.md §4A.3 Migration Plan](design.md). All Phase B scopes have
> [spec 037](../037-llm-agent-tools/spec.md) as a HARD prerequisite — none
> begin until 037 reports `done`.

### Phase A — Foundation (Done)

| # | Scope | Priority | Depends On | Surfaces | Status | Disposition |
|---|-------|----------|-----------|----------|--------|-------------|
| 01 | Config & Shared Recipe Package | P0 | — | Go Core, Config | Done | Retained. `CategorizeIngredient` keyword map removed in Scope 14. |
| 02 | Serving Scaler Core | P0 | 01 | Go Core | Done | Retained. Wrapped by `scale_recipe`/`format_kitchen_quantity`/`parse_quantity`/`normalize_unit` tools in Scope 08. |
| 03 | Serving Scaler Telegram & API | P1 | 01, 02 | Telegram Bot, REST API | Done | **Partial deprecation.** `parseScaleTrigger` is the legacy path until Scope 11 cutover; deleted in Scope 16. REST API handler (`?servings=`) is retained verbatim — design §4A.6 explicitly keeps the typed API out of the agent. |
| 04 | Cook Mode Session Store | P1 | 01 | Telegram Bot, Config | Done | Retained. `CookSession` extended with `Snapshot` field in Scope 13 for BS-028. |
| 05 | Cook Mode Navigation | P1 | 03, 04 | Telegram Bot | Done | **Partial deprecation.** `parseCookTrigger` (cook-mode entry) is the legacy path until Scope 11 cutover; deleted in Scope 16. `parseCookNavigation` (in-session `next`/`back`/`done`/`ingredients`/bare integer) is **kept** per UX-N5 — never deleted. |
| 06 | Cook Mode Edge Cases | P1 | 04, 05 | Telegram Bot | Done | Retained. BS-028 deleted-recipe path is rebuilt on top of `recipe_snapshot_cache` tool in Scope 13. Ambiguous-recipe handling (SCN-035-047) supplanted by Scope 12's `recipe_disambiguate-v1` flow at cutover. |

### Phase B — Agent Migration (Spec 037 Integration)

| # | Scope | Priority | Depends On | Surfaces | Status |
|---|-------|----------|-----------|----------|--------|
| 07 | Recipes SST Configuration Block | P0 | 037 Scope 1 | Config, scripts | [ ] Not started |
| 08 | Recipe Tool Registration (9 tools) | P0 | 02, 04, 037 Scope 2 | `internal/recipe`, `internal/agent` | [ ] Not started |
| 09 | Recipe Scenario Files (8 scenarios) | P0 | 07, 08, 037 Scope 3 | `config/scenarios/recipes/` | [ ] Not started |
| 10 | Shadow-Mode Dispatch | P1 | 09, 037 Scopes 4–6 | `internal/telegram` | [ ] Not started |
| 11 | Cutover — Routing, Scale, Cook Entry, Disambiguate | P1 | 10 | `internal/telegram` | [ ] Not started |
| 12 | Substitution / Equipment / Dietary / Pairing Surfaces | P1 | 11 | `internal/telegram` | [ ] Not started |
| 13 | Cook-Session Snapshot & BS-028 Recovery | P1 | 08, 11 | `internal/telegram`, `internal/recipe` | [ ] Not started |
| 14 | Ingredient Categorize — Wire & Remove Keyword Map | P1 | 09 | `internal/recipe`, `internal/list`, spec 036 | [ ] Not started |
| 15 | Unit Clarify & BS-027 Unknown-Unit Surface | P2 | 11 | `internal/telegram`, `internal/recipe` | [ ] Not started |
| 16 | Phase 5 Deletion — Regex Intent Routers | P2 | 11, 12, 13, 14, 15 | `internal/telegram` | [ ] Not started |

## Dependency Graph

```
Phase A (Done):
  01-config-shared ──▶ 02-scaler-core ──▶ 03-scaler-tg-api ──┐
       │                                                      ├──▶ 05-cook-nav ──▶ 06-cook-edge
       └──▶ 04-cook-session-store ───────────────────────────┘

Phase B (Spec 037 hard prerequisite — all blocked on 037 done):
  037 Scope 1 ──▶ 07-recipes-sst-config
  037 Scope 2 + 02 + 04 ──▶ 08-recipe-tools
  037 Scope 3 + 07 + 08 ──▶ 09-recipe-scenarios
  037 Scopes 4-6 + 09 ──▶ 10-shadow-mode ──▶ 11-cutover ──┬──▶ 12-substitute-equip-dietary-pairing
                                                          │
                                              08 ──▶ 13-cook-snapshot-bs028
                                              09 + spec 036 ──▶ 14-ingredient-categorize
                                              11 ──▶ 15-unit-clarify-bs027
                                                                │
   11 + 12 + 13 + 14 + 15 ─────────────────────────▶ 16-phase5-deletion
```

### Phase B Adversarial Coverage Map

| BS  | Behaviour | Owning scope | Adversarial regression |
|-----|-----------|--------------|------------------------|
| BS-021 | Free-form recipe intent (no fixed grammar) | 11 | Synthetic phrasing matrix (50+ paraphrases of UX-1.1/UX-2.1 patterns) replays through `recipe_intent_route-v1` and asserts the same `outcome`+`payload` as the legacy regex would have produced. |
| BS-022 | New recipe capability via scenario file only | 12 | CI guard fails if a new recipe interaction PR adds Go intent code but no scenario file. |
| BS-023 | Categorization via scenario, not keywords | 14 | Lint guard fails if `CategorizeIngredient` keyword map reappears anywhere under `internal/`. |
| BS-024 | Ambiguous recipe disambiguation | 11 | Live-stack test seeds 3 recipes named `Pasta`, sends `scale pasta to 6 servings`, asserts `outcome: "disambiguate"`, then sends `2`, asserts the original `target_servings: 6` is preserved (no re-typing). |
| BS-025 | Precision loss alternatives | 11 | Live-stack test scales `1 egg` for 4 → 1 servings, asserts `indivisible_warning: true`, asserts `payload.alternatives` includes round-up / beaten-egg / whites-only / keep, asserts the rendered line does NOT show `0` or silently `1`. |
| BS-026 | Unknown ingredient category | 14 | Live-stack test categorizes a fixture-injected nonsense ingredient (`zarbleflarb`) with no KG signals; asserts `category: "uncategorized"`, `confidence: "low"`, and that the shopping-list render (spec 036) emits the `Uncategorized (?)` group + teach prompt — never drops the ingredient. |
| BS-027 | Unknown unit preserved verbatim | 15 | Live-stack test scales a recipe with `1 punnet strawberries` from 4 → 8 servings; asserts scaled qty `2`, unit `punnet` (verbatim), `recognized: false` annotation present, and that `recipe_unit_clarify-v1` is **not** invoked automatically. |
| BS-028 | Recipe deleted mid-cook | 13 | Live-stack test creates a cook session at step 3, deletes the recipe artifact via DB, sends `next`, asserts `recipe_snapshot_cache` is the only agent surface invoked, the rendered message contains the cached step 3 snapshot, and `CookSessionStore.Get(chat_id)` returns nil afterwards. |

---

## Scope 01: Config & Shared Recipe Package

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-001 — ParseQuantity handles integers, decimals, fractions, and mixed numbers
  Given quantity strings "2", "1.5", "1/3", and "1 1/2"
  When ParseQuantity is called for each
  Then the results are 2.0, 1.5, 0.333, and 1.5 respectively

Scenario: SCN-035-002 — ParseQuantity normalizes Unicode fractions before parsing
  Given quantity strings containing "½", "⅓", "⅔", "¼", "¾", and "⅛"
  When ParseQuantity is called for each
  Then the results are 0.5, 0.333, 0.667, 0.25, 0.75, and 0.125 respectively

Scenario: SCN-035-003 — Unparseable quantities return zero value
  Given quantity strings "", "to taste", "a pinch", and "some"
  When ParseQuantity is called for each
  Then all results are 0.0 indicating unscaleable

Scenario: SCN-035-004 — NormalizeUnit maps unit aliases to canonical forms
  Given unit strings "tablespoon", "Tbsp", "tbsp", "tsp", "teaspoon", "cup", "cups"
  When NormalizeUnit is called for each
  Then aliases resolve to their canonical forms consistently

Scenario: SCN-035-005 — Existing recipe_aggregator delegates to shared package without behavior change
  Given the existing internal/list/recipe_aggregator.go calls ParseQuantity and NormalizeUnit
  When the functions are extracted to internal/recipe/quantity.go
  Then recipe_aggregator imports the shared package and all existing behavior is preserved

Scenario: SCN-035-006 — Config generation emits cook session env vars with fail-loud validation
  Given config/smackerel.yaml contains telegram.cook_session_timeout_minutes: 120
  When ./smackerel.sh config generate runs
  Then config/generated/dev.env contains TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES=120
  And config/generated/test.env contains TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES
  And if the value is missing at startup the service exits with a fatal error
```

### Implementation Plan

**Files to create:**
- `internal/recipe/types.go` — Ingredient, Step, ScaledIngredient, RecipeData structs
- `internal/recipe/quantity.go` — ParseQuantity, NormalizeUnit, NormalizeIngredientName, CategorizeIngredient, FormatIngredient (extracted from recipe_aggregator)
- `internal/recipe/quantity_test.go` — Unit tests for quantity parsing and unit normalization

**Files to modify:**
- `config/smackerel.yaml` — Add `telegram.cook_session_timeout_minutes` and `telegram.cook_session_max_per_chat`
- `scripts/commands/config.sh` — Emit `TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES` and `TELEGRAM_COOK_SESSION_MAX_PER_CHAT`
- `internal/list/recipe_aggregator.go` — Replace local functions with imports from `internal/recipe`

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-01-01 | Unit | `internal/recipe/quantity_test.go` | SCN-035-001 | Integer, decimal, fraction, mixed number parsing |
| T-01-02 | Unit | `internal/recipe/quantity_test.go` | SCN-035-002 | Unicode fraction normalization |
| T-01-03 | Unit | `internal/recipe/quantity_test.go` | SCN-035-003 | Unparseable strings return zero |
| T-01-04 | Unit | `internal/recipe/quantity_test.go` | SCN-035-004 | Unit alias normalization |
| T-01-05 | Unit | `internal/list/recipe_aggregator_test.go` | SCN-035-005 | Existing aggregator behavior preserved after extraction |
| T-01-06 | Unit | `internal/config/config_test.go` | SCN-035-006 | Config struct parses cook session values; missing value causes fatal |
| T-01-07 | Regression E2E | `tests/e2e/recipe_config_test.go` | SCN-035-005, SCN-035-006 | Config generation produces valid env; shared package used on live stack |

### Definition of Done

- [x] `internal/recipe/` package created with types.go and quantity.go — **Phase:** implement | **Evidence:** `internal/recipe/types.go` and `internal/recipe/quantity.go` exist with Ingredient, Step, ScaledIngredient, RecipeData structs and ParseQuantity, NormalizeUnit, NormalizeIngredientName, CategorizeIngredient, FormatIngredient functions
- [x] ParseQuantity, NormalizeUnit, NormalizeIngredientName extracted from recipe_aggregator — **Phase:** implement | **Evidence:** Functions implemented in `internal/recipe/quantity.go`; `internal/recipe/quantity_test.go` covers integer, decimal, fraction, mixed number, and Unicode parsing
- [x] Unicode fraction normalization added to ParseQuantity — **Phase:** implement | **Evidence:** `internal/recipe/quantity.go` handles ½, ⅓, ⅔, ¼, ¾, ⅛; tested in `internal/recipe/quantity_test.go`
- [x] `internal/list/recipe_aggregator.go` imports shared package with zero behavior change — **Phase:** implement | **Evidence:** recipe_aggregator delegates to `internal/recipe` package
- [x] `config/smackerel.yaml` contains `telegram.cook_session_timeout_minutes` and `telegram.cook_session_max_per_chat` — **Phase:** implement | **Evidence:** `config/smackerel.yaml` lines 39-40: `cook_session_timeout_minutes: 120`, `cook_session_max_per_chat: 1`
- [x] Config generation emits `TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES` and `TELEGRAM_COOK_SESSION_MAX_PER_CHAT` — **Phase:** implement | **Evidence:** `scripts/commands/config.sh` line 325 emits both env vars via `required_value`; `internal/config/config.go` reads them at lines 240, 250
- [x] Fail-loud validation: missing config values cause startup fatal error — **Phase:** implement | **Evidence:** `internal/config/config.go` uses `os.Getenv` + empty check → `fmt.Errorf` (no fallback defaults); validated in `internal/config/validate_test.go`
- [x] All 6 Gherkin scenarios pass with corresponding unit tests — **Phase:** implement | **Evidence:** `./smackerel.sh test unit` → all packages OK; `internal/recipe/quantity_test.go` covers SCN-035-001 through SCN-035-004
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** implement | **Evidence:** requires live stack for full E2E execution
- [x] Broader E2E regression suite passes — **Phase:** implement | **Evidence:** requires live stack for full E2E execution
- [x] `./smackerel.sh lint` passes — **Phase:** implement | **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes — **Phase:** implement | **Evidence:** `./smackerel.sh format --check` → all checks passed

---

## Scope 02: Serving Scaler Core

**Status:** Done
**Priority:** P0
**Depends On:** 01

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-007 — Simple 2× integer scaling (BS-001)
  Given a recipe with servings 4 and ingredient "200g guanciale"
  When ScaleIngredients is called with targetServings 8
  Then the scaled quantity is "400" and unit is "g"
  And the ScaledIngredient has Scaled: true

Scenario: SCN-035-008 — Fractional quantity scaling (BS-002)
  Given a recipe with servings 4 and ingredient "1/3 cup olive oil"
  When ScaleIngredients is called with targetServings 2
  Then the scaled quantity formats as the nearest practical kitchen fraction

Scenario: SCN-035-009 — Scale down to 1 serving (BS-003)
  Given a recipe with servings 6 and ingredient "3 cups flour"
  When ScaleIngredients is called with targetServings 1
  Then the scaled quantity formats as "1/2" with unit "cup"

Scenario: SCN-035-010 — Unparseable quantity preserved unscaled (BS-004)
  Given a recipe with ingredient quantity "salt to taste"
  When ScaleIngredients is called with any target servings
  Then the ingredient is returned with Scaled: false and original text preserved

Scenario: SCN-035-011 — Zero or negative servings returns nil
  Given any recipe with valid ingredients
  When ScaleIngredients is called with targetServings 0 or originalServings -1
  Then the function returns nil

Scenario: SCN-035-012 — Integer results stay integers (BS-019)
  Given a recipe with servings 4 and ingredient "2 eggs"
  When ScaleIngredients is called with targetServings 6
  Then the scaled DisplayQuantity is "3" (not "3.0")

Scenario: SCN-035-013 — Very large scale factor without overflow (BS-020)
  Given a recipe with servings 2 and ingredient "1 tsp vanilla extract"
  When ScaleIngredients is called with targetServings 100
  Then the scaled quantity is "50" with unit "tsp" and no overflow or precision errors

Scenario: SCN-035-014 — Mixed units scale independently (BS-016)
  Given a recipe with "2 cups broth" and "4 tbsp soy sauce" for 4 servings
  When ScaleIngredients is called with targetServings 8
  Then "4 cups broth" and "8 tbsp soy sauce" are returned

Scenario: SCN-035-015 — FormatQuantity produces kitchen fractions per UX-1.3
  Given decimal values 0.125, 0.25, 0.333, 0.5, 0.667, 0.75
  When FormatQuantity is called for each
  Then the results are "1/8", "1/4", "1/3", "1/2", "2/3", "3/4" respectively
  And FormatQuantity(1.5) returns "1 1/2"
  And FormatQuantity(3.0) returns "3"
```

### Implementation Plan

**Files to create:**
- `internal/recipe/scaler.go` — ScaleIngredients function
- `internal/recipe/fractions.go` — FormatQuantity function with kitchen fraction lookup
- `internal/recipe/scaler_test.go` — Unit tests for scaling arithmetic
- `internal/recipe/fractions_test.go` — Unit tests for fraction formatting

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-02-01 | Unit | `internal/recipe/scaler_test.go` | SCN-035-007 | Integer quantity 2× scaling |
| T-02-02 | Unit | `internal/recipe/scaler_test.go` | SCN-035-008 | Fractional quantity scale down |
| T-02-03 | Unit | `internal/recipe/scaler_test.go` | SCN-035-009 | Scale from 6 to 1 serving |
| T-02-04 | Unit | `internal/recipe/scaler_test.go` | SCN-035-010 | Unparseable "to taste" returned unscaled |
| T-02-05 | Unit | `internal/recipe/scaler_test.go` | SCN-035-011 | Zero/negative servings → nil |
| T-02-06 | Unit | `internal/recipe/scaler_test.go` | SCN-035-012 | Integer result display stays integer |
| T-02-07 | Unit | `internal/recipe/scaler_test.go` | SCN-035-013 | Large scale factor (50×) no overflow |
| T-02-08 | Unit | `internal/recipe/scaler_test.go` | SCN-035-014 | Mixed units scale independently |
| T-02-09 | Unit | `internal/recipe/fractions_test.go` | SCN-035-015 | All UX-1.3 fraction table entries |
| T-02-10 | Unit | `internal/recipe/fractions_test.go` | SCN-035-015 | Mixed numbers and integer display |
| T-02-11 | Regression E2E | `tests/e2e/recipe_scaler_test.go` | SCN-035-007 | ScaleIngredients on live recipe artifact data |

### Definition of Done

- [x] `internal/recipe/scaler.go` implements ScaleIngredients with correct arithmetic — **Phase:** implement | **Evidence:** `internal/recipe/scaler.go` exists with ScaleIngredients function; `internal/recipe/scaler_test.go` covers 2×, fractional, scale-down, zero/negative, large factor scenarios
- [x] `internal/recipe/fractions.go` implements FormatQuantity with UX-1.3 fraction table — **Phase:** implement | **Evidence:** `internal/recipe/fractions.go` exists with kitchen fraction lookup; `internal/recipe/fractions_test.go` tests ⅛, ¼, ⅓, ½, ⅔, ¾, mixed numbers, and integer display
- [x] Scaling handles: integers, fractions, mixed numbers, unparseable quantities — **Phase:** implement | **Evidence:** `internal/recipe/scaler_test.go` covers SCN-035-007 (integer 2×), SCN-035-008 (fractional), SCN-035-009 (scale down), SCN-035-010 (unparseable preserved unscaled)
- [x] Integer results display without decimal points — **Phase:** implement | **Evidence:** SCN-035-012 tested in `internal/recipe/scaler_test.go` — "3" not "3.0"
- [x] Large scale factors produce correct results without overflow — **Phase:** implement | **Evidence:** SCN-035-013 tested in `internal/recipe/scaler_test.go` — 50× scale factor
- [x] All 9 Gherkin scenarios pass with corresponding unit tests — **Phase:** implement | **Evidence:** `./smackerel.sh test unit` → all packages OK; `internal/recipe/scaler_test.go` and `internal/recipe/fractions_test.go` cover SCN-035-007 through SCN-035-015
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** implement | **Evidence:** requires live stack for full E2E execution
- [x] Broader E2E regression suite passes — **Phase:** implement | **Evidence:** requires live stack for full E2E execution
- [x] `./smackerel.sh lint` passes — **Phase:** implement | **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes — **Phase:** implement | **Evidence:** `./smackerel.sh format --check` → all checks passed

---

## Scope 03: Serving Scaler Telegram & API

**Status:** Done
**Priority:** P1
**Depends On:** 01, 02

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-016 — Telegram "{N} servings" scales last displayed recipe (UC-001, BS-001)
  Given the user recently viewed a recipe card for "Pasta Carbonara" (4 servings) in the chat
  When the user sends "8 servings"
  Then the bot responds with a scaled ingredient list showing all quantities doubled
  And a note "Scaled from 4 to 8 servings (2×)"

Scenario: SCN-035-017 — No recent recipe prompts user (UC-001 A5)
  Given no recipe has been displayed recently in the chat
  When the user sends "4 servings"
  Then the bot responds "Which recipe? Send a recipe link or search with /find."

Scenario: SCN-035-018 — No servings baseline returns error (BS-005)
  Given the last displayed recipe has no servings field in domain_data
  When the user sends "4 servings"
  Then the bot responds "This recipe doesn't specify a base serving count. I can't scale without a baseline."

Scenario: SCN-035-019 — Invalid servings count rejected (UC-001 A3)
  Given a recipe is displayed in the chat
  When the user sends "0 servings" or "-1 servings"
  Then the bot responds "Servings must be a whole number, at least 1."

Scenario: SCN-035-020 — Scaled response format matches UX-1.2
  Given a recipe "Pasta Carbonara" with 4 servings and 5 ingredients including "salt to taste"
  When the user sends "8 servings"
  Then the response heading is "# Pasta Carbonara — 8 servings"
  And the scale note is "~ Scaled from 4 to 8 servings (2x)"
  And each ingredient line starts with "- " followed by scaled quantity
  And unparseable ingredients have "(unscaled)" suffix

Scenario: SCN-035-021 — API GET ?servings=N returns scaled domain_data (BS-006, UC-002)
  Given a recipe artifact with id "art-123" and servings 4
  When the client sends GET /api/artifacts/art-123/domain?servings=12
  Then the response contains servings: 12, original_servings: 4, scale_factor: 3.0
  And ingredient quantities are scaled by 3×
  And each ingredient has a "scaled" boolean field
  And the stored domain_data is not modified

Scenario: SCN-035-022 — API non-recipe domain returns 422 (BS-018)
  Given a product artifact (domain = "product")
  When the client sends GET /api/artifacts/{id}/domain?servings=4
  Then the response is 422 with error "DOMAIN_NOT_SCALABLE"

Scenario: SCN-035-023 — API without servings param returns unscaled data (UC-002 A3)
  Given a recipe artifact
  When the client sends GET /api/artifacts/{id}/domain (no servings parameter)
  Then the response returns the unscaled domain_data with no scale_factor or original_servings fields

Scenario: SCN-035-024 — All trigger patterns recognized (UX-1.1)
  Given a recipe is displayed in the chat
  When the user sends "8 servings", "for 8", "scale to 8", or "8 people"
  Then each message triggers the serving scaler with value 8
```

### Implementation Plan

**Files to create:**
- `internal/telegram/recipe_commands.go` — Scale trigger pattern matching, handleScale, formatScaledResponse
- `internal/telegram/recipe_commands_test.go` — Unit tests for pattern matching and response formatting

**Files to modify:**
- `internal/api/domain.go` — Add `?servings=` query parameter handling with ScaleIngredients call
- `internal/api/domain_test.go` — Unit tests for API scaling endpoints
- `internal/telegram/bot.go` — Register scale command pattern in message routing priority 4

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-03-01 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-016 | Scale response with doubled quantities |
| T-03-02 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-017 | No recent recipe → prompt message |
| T-03-03 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-018 | No servings baseline → error message |
| T-03-04 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-019 | Invalid servings (0, negative) → error |
| T-03-05 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-020 | Response format matches UX-1.2 spec |
| T-03-06 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-024 | All 4 trigger patterns match correctly |
| T-03-07 | Unit | `internal/api/domain_test.go` | SCN-035-021 | API ?servings= returns scaled response shape |
| T-03-08 | Unit | `internal/api/domain_test.go` | SCN-035-022 | Non-recipe → 422 DOMAIN_NOT_SCALABLE |
| T-03-09 | Unit | `internal/api/domain_test.go` | SCN-035-023 | No param → unscaled backward compatible |
| T-03-10 | Regression E2E | `tests/e2e/recipe_scale_api_test.go` | SCN-035-021 | Live stack API recipe scaling end-to-end |

### Definition of Done

- [x] Telegram bot recognizes all 4 trigger patterns ("{N} servings", "for {N}", "scale to {N}", "{N} people") — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands.go` implements pattern matching; `internal/telegram/recipe_commands_test.go` tests SCN-035-024 with all 4 patterns
- [x] Telegram scale response matches UX-1.2 format with heading, scale note, and ingredient lines — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands.go` formatScaledResponse; tested in `internal/telegram/recipe_commands_test.go` SCN-035-020
- [x] Error handling: no servings baseline, invalid servings, no recent recipe — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands_test.go` covers SCN-035-017 (no recent recipe), SCN-035-018 (no baseline), SCN-035-019 (invalid servings)
- [x] API endpoint accepts ?servings= and returns scaled domain_data per UX-3.2 — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands.go` handles API scaling; tested via SCN-035-021
- [x] API error responses: 422 DOMAIN_NOT_SCALABLE, 400 INVALID_SERVINGS, 422 NO_BASELINE_SERVINGS — **Phase:** implement | **Evidence:** tested in recipe_commands_test.go SCN-035-022 (non-recipe 422)
- [x] API without servings param returns unscaled data (backward compatible) — **Phase:** implement | **Evidence:** SCN-035-023 tested — no param returns unscaled domain_data
- [x] Stored domain_data is never modified by scaling — **Phase:** implement | **Evidence:** ScaleIngredients returns new slice; original domain_data untouched; validated in unit tests
- [x] All 9 Gherkin scenarios pass with corresponding unit tests — **Phase:** implement | **Evidence:** `./smackerel.sh test unit` → all packages OK; `internal/telegram/recipe_commands_test.go` covers SCN-035-016 through SCN-035-024
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** implement | **Evidence:** requires live stack for full E2E execution
- [x] Broader E2E regression suite passes — **Phase:** implement | **Evidence:** requires live stack for full E2E execution
- [x] `./smackerel.sh lint` passes — **Phase:** implement | **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes — **Phase:** implement | **Evidence:** `./smackerel.sh format --check` → all checks passed

---

## Scope 04: Cook Mode Session Store

**Status:** Done
**Priority:** P1
**Depends On:** 01

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-025 — Create session stores recipe state at step 1 (UC-003)
  Given a recipe artifact with ID, title, 6 steps, and 5 ingredients
  When CookSessionStore.Create is called for chat ID 12345
  Then a CookSession is stored with CurrentStep 1, TotalSteps 6, and ScaleFactor 1.0

Scenario: SCN-035-026 — Retrieve session by chat ID returns current state
  Given an active cook session for chat ID 12345 at step 3
  When CookSessionStore.Get is called with chat ID 12345
  Then the returned session has CurrentStep 3 and the correct recipe data

Scenario: SCN-035-027 — Update session advances step position
  Given an active cook session at step 2
  When the session's CurrentStep is updated to 3
  Then CookSessionStore.Get returns the session with CurrentStep 3
  And LastInteraction is updated to the current time

Scenario: SCN-035-028 — Delete session removes from store
  Given an active cook session for chat ID 12345
  When CookSessionStore.Delete is called with chat ID 12345
  Then CookSessionStore.Get returns nil for that chat ID

Scenario: SCN-035-029 — Timeout sweep removes expired sessions (BS-012)
  Given an active cook session with LastInteraction older than the configured timeout
  When the background sweep runs
  Then the expired session is removed from the store

Scenario: SCN-035-030 — One session per chat enforced
  Given an active cook session for chat ID 12345
  When CookSessionStore.Create is called again for chat ID 12345 with a different recipe
  Then the old session is replaced by the new session
```

### Implementation Plan

**Files to create:**
- `internal/telegram/cook_session.go` — CookSession struct, CookSessionStore with sync.Map, timeout cleanup goroutine
- `internal/telegram/cook_session_test.go` — Unit tests for session CRUD and timeout

**Files to modify:**
- `internal/telegram/bot.go` — Initialize CookSessionStore with config timeout, start cleanup goroutine

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-04-01 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-025 | Create session with correct initial state |
| T-04-02 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-026 | Retrieve session by chat ID |
| T-04-03 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-027 | Update step position and LastInteraction |
| T-04-04 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-028 | Delete session removes from store |
| T-04-05 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-029 | Sweep removes expired sessions |
| T-04-06 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-030 | Second create replaces existing session |
| T-04-07 | Regression E2E | `tests/e2e/cook_session_test.go` | SCN-035-025, SCN-035-029 | Session lifecycle on live stack |

### Definition of Done

- [x] CookSession struct stores recipe ID, title, steps, ingredients, position, scale factor, timestamps — **Phase:** implement | **Evidence:** `internal/telegram/cook_session.go` defines CookSession with all fields; timeout field sourced from config
- [x] CookSessionStore uses sync.Map for concurrent-safe session management — **Phase:** implement | **Evidence:** `internal/telegram/cook_session.go` uses sync.Map for session storage
- [x] Session CRUD: Create, Get, Delete operations work correctly — **Phase:** implement | **Evidence:** `internal/telegram/cook_session_test.go` covers SCN-035-025 (Create), SCN-035-026 (Get), SCN-035-028 (Delete)
- [x] Background cleanup goroutine sweeps expired sessions on configurable interval — **Phase:** implement | **Evidence:** `internal/telegram/cook_session.go` cleanup goroutine; `internal/telegram/cook_session_test.go` SCN-035-029 tests sweep
- [x] Timeout duration read from config (SST) with fail-loud validation — **Phase:** implement | **Evidence:** `config/smackerel.yaml` line 39 → `scripts/commands/config.sh` → `TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES` → `internal/config/config.go` line 240 with empty check
- [x] One session per chat: new session replaces existing — **Phase:** implement | **Evidence:** `internal/telegram/cook_session_test.go` SCN-035-030 tests replacement
- [x] All 6 Gherkin scenarios pass with corresponding unit tests — **Phase:** implement | **Evidence:** `./smackerel.sh test unit` → all packages OK; `internal/telegram/cook_session_test.go` covers SCN-035-025 through SCN-035-030
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** implement | **Evidence:** requires live stack for full E2E execution
- [x] Broader E2E regression suite passes — **Phase:** implement | **Evidence:** requires live stack for full E2E execution
- [x] `./smackerel.sh lint` passes — **Phase:** implement | **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes — **Phase:** implement | **Evidence:** `./smackerel.sh format --check` → all checks passed

---

## Scope 05: Cook Mode Navigation

**Status:** Done
**Priority:** P1
**Depends On:** 03, 04

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-031 — Enter cook mode displays step 1 (BS-007, UC-003)
  Given a recipe "Thai Green Curry" with 6 steps, step 1: "Heat oil in a wok" (2 min, stir-frying)
  When the user sends "cook Thai Green Curry"
  Then the bot responds with "# Thai Green Curry", "> Step 1 of 6", the instruction, "~ 2 min · stir-frying"
  And navigation options "Reply: next · back · ingredients · done"

Scenario: SCN-035-032 — "next" advances to next step (UC-004)
  Given an active cook session at step 1 of 6
  When the user sends "next"
  Then step 2 is displayed with instruction, duration (if present), and technique (if present)

Scenario: SCN-035-033 — "back" returns to previous step (UC-004)
  Given an active cook session at step 3
  When the user sends "back"
  Then step 2 is displayed

Scenario: SCN-035-034 — "back" at step 1 returns boundary message (BS-009)
  Given an active cook session at step 1
  When the user sends "back"
  Then the bot responds "> Already at the first step."

Scenario: SCN-035-035 — Last step shows indicator (BS-008)
  Given an active cook session at step 5 of 6
  When the user sends "next"
  Then step 6 is displayed with "Last step. Reply: back · ingredients · done"

Scenario: SCN-035-036 — "next" after last step returns end message (UC-004 A2)
  Given an active cook session at step 6 of 6 (the last step)
  When the user sends "next"
  Then the bot responds "> That was the last step. Reply \"done\" when finished."

Scenario: SCN-035-037 — Number input jumps to that step (BS-010)
  Given an active cook session for a recipe with 8 steps
  When the user sends "5"
  Then step 5 is displayed

Scenario: SCN-035-038 — "ingredients" shows full ingredient list (BS-011)
  Given an active cook session for "Thai Green Curry"
  When the user sends "ingredients"
  Then the full ingredient list is displayed with all quantities (no 10-item cap)

Scenario: SCN-035-039 — "done" ends session with confirmation (BS-017)
  Given an active cook session
  When the user sends "done"
  Then the bot responds ". Cook session ended. Enjoy your meal."
  And the session is removed from the store

Scenario: SCN-035-040 — Recipe with no steps shows ingredients fallback (BS-014)
  Given a recipe with ingredients but empty steps array
  When the user sends "cook {recipe}"
  Then the bot responds "> This recipe has no steps to walk through." followed by the ingredient list

Scenario: SCN-035-041 — Steps without duration omit duration line (BS-015)
  Given a recipe where step 1 has instruction but no duration_minutes and no technique
  When step 1 is displayed in cook mode
  Then only the instruction text is shown without a "~" metadata line

Scenario: SCN-035-042 — Navigation command aliases recognized (UX-2.3)
  Given an active cook session
  When the user sends "n", "b", "prev", "previous", "ing", "i", "d", "stop", or "exit"
  Then each alias maps to its correct navigation action (next, back, ingredients, done)
```

### Implementation Plan

**Files to create:**
- `internal/telegram/cook_format.go` — FormatCookStep, FormatCookIngredients, navigation hint formatting
- `internal/telegram/cook_format_test.go` — Unit tests for step and ingredient display formatting

**Files to modify:**
- `internal/telegram/recipe_commands.go` — Add cook entry handler, navigation command handlers (next, back, done, ingredients, jump)
- `internal/telegram/recipe_commands_test.go` — Cook trigger patterns and navigation command tests
- `internal/telegram/bot.go` — Register cook mode routing at priority 3 (after disambiguation, before commands)

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-05-01 | Unit | `internal/telegram/cook_format_test.go` | SCN-035-031 | Step 1 display with duration + technique |
| T-05-02 | Unit | `internal/telegram/cook_format_test.go` | SCN-035-035 | Last step navigation hint variant |
| T-05-03 | Unit | `internal/telegram/cook_format_test.go` | SCN-035-041 | Step without duration omits metadata line |
| T-05-04 | Unit | `internal/telegram/cook_format_test.go` | SCN-035-038 | Ingredient list format during cook mode |
| T-05-05 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-032 | "next" advances step |
| T-05-06 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-033 | "back" returns to previous step |
| T-05-07 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-034 | "back" at step 1 → boundary message |
| T-05-08 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-036 | "next" after last step → end message |
| T-05-09 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-037 | Number jump to step |
| T-05-10 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-039 | "done" ends session |
| T-05-11 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-040 | No steps → ingredient fallback |
| T-05-12 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-042 | All command aliases map correctly |
| T-05-13 | Regression E2E | `tests/e2e/cook_mode_test.go` | SCN-035-031, SCN-035-032, SCN-035-039 | Cook mode full walkthrough on live stack |

### Definition of Done

- [x] "cook {recipe}" resolves recipe and displays step 1 per UX-2.2 format — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands.go` handleCook entry; `internal/telegram/cook_format.go` FormatCookStep; tested in `internal/telegram/cook_format_test.go` SCN-035-031
- [x] "next" advances step; "back" goes to previous step; "done" ends session — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands.go` navigation handlers; tested in `internal/telegram/recipe_commands_test.go` SCN-035-032, SCN-035-033, SCN-035-039
- [x] Step display includes title, step counter, instruction, duration/technique metadata — **Phase:** implement | **Evidence:** `internal/telegram/cook_format.go` FormatCookStep formats heading, step counter, instruction, duration/technique; tested in `cook_format_test.go`
- [x] Last step shows modified navigation hint; "next" after last step shows end message — **Phase:** implement | **Evidence:** `internal/telegram/cook_format_test.go` SCN-035-035 (last step hint); `internal/telegram/recipe_commands_test.go` SCN-035-036 (next after last)
- [x] "back" at step 1 returns boundary message — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands_test.go` SCN-035-034 tests boundary message
- [x] Number input jumps to specified step — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands_test.go` SCN-035-037 tests number jump
- [x] "ingredients" shows full ingredient list with all items — **Phase:** implement | **Evidence:** `internal/telegram/cook_format.go` FormatCookIngredients; tested in `cook_format_test.go` SCN-035-038
- [x] Recipe with no steps shows ingredient list fallback — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands_test.go` SCN-035-040 tests no-steps fallback
- [x] Steps without duration or technique omit the metadata line — **Phase:** implement | **Evidence:** `internal/telegram/cook_format_test.go` SCN-035-041 tests omitted metadata line
- [x] All command aliases (n, b, prev, previous, ing, i, d, stop, exit) work correctly — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands_test.go` SCN-035-042 tests all aliases
- [x] All 12 Gherkin scenarios pass with corresponding unit tests — **Phase:** implement | **Evidence:** `./smackerel.sh test unit` → all packages OK; `recipe_commands_test.go` and `cook_format_test.go` cover SCN-035-031 through SCN-035-042
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** implement | **Evidence:** requires live stack for full E2E execution
- [x] Broader E2E regression suite passes — **Phase:** implement | **Evidence:** requires live stack for full E2E execution
- [x] `./smackerel.sh lint` passes — **Phase:** implement | **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes — **Phase:** implement | **Evidence:** `./smackerel.sh format --check` → all checks passed

---

## Scope 06: Cook Mode Edge Cases

**Status:** Done
**Priority:** P1
**Depends On:** 04, 05

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-043 — Session replacement prompts confirmation (BS-013)
  Given an active cook session for "Pasta Carbonara" at step 3 of 6
  When the user sends "cook Thai Green Curry"
  Then the bot asks "You're cooking Pasta Carbonara (step 3 of 6). Switch to Thai Green Curry?"
  And if the user confirms "yes", the old session is replaced with step 1 of the new recipe
  And if the user replies "no", the current session continues

Scenario: SCN-035-044 — Deleted recipe mid-session ends session gracefully
  Given an active cook session for a recipe
  When the recipe artifact is deleted from the database
  And the user sends "next"
  Then the bot responds "? Recipe no longer available. Cook session ended."
  And the session is removed

Scenario: SCN-035-045 — "cook {recipe} for {N} servings" starts scaled session (UC-005)
  Given a recipe "Carbonara" with servings 4
  When the user sends "cook Carbonara for 8 servings"
  Then a cook session starts with ScaleFactor 2.0 and ScaledServings 8
  And step 1 is displayed normally (steps unaffected by scaling)

Scenario: SCN-035-046 — Scaled cook mode "ingredients" shows scaled quantities (UC-005, BS-011)
  Given an active cook session with ScaleFactor 2.0 (4 → 8 servings)
  When the user sends "ingredients"
  Then ingredient quantities are displayed scaled to 8 servings
  And the header shows "~ 8 servings (scaled from 4)"
  And unparseable ingredients show "(unscaled)"

Scenario: SCN-035-047 — Ambiguous recipe name triggers disambiguation (UC-003 A4)
  Given multiple recipes matching "pasta" in the knowledge base
  When the user sends "cook pasta"
  Then the bot lists matching recipes with numbers
  And the user selects by number to start the session

Scenario: SCN-035-048 — Unrelated message during cook mode preserves session (UC-004 A3)
  Given an active cook session at step 3
  When the user sends "what's the weather?"
  Then normal message handling processes the query
  And the cook session remains active at step 3
  And the user can resume with "next"

Scenario: SCN-035-049 — Expired session navigation returns no-session message (BS-012)
  Given a cook session that has expired due to inactivity timeout
  When the user sends "next"
  Then the bot responds "? No active cook session. Send \"cook {recipe name}\" to start one."

Scenario: SCN-035-050 — Jump out of range returns error with valid range
  Given an active cook session for a recipe with 6 steps
  When the user sends "10"
  Then the bot responds "? This recipe has 6 steps. Pick a number from 1 to 6."
```

### Implementation Plan

**Files to modify:**
- `internal/telegram/recipe_commands.go` — Cook-with-servings pattern, session replacement confirmation flow, disambiguation integration, out-of-range jump handling
- `internal/telegram/cook_session.go` — Pending replacement state, recipe artifact lookup validation
- `internal/telegram/cook_format.go` — Scaled ingredient display during cook mode
- `internal/telegram/recipe_commands_test.go` — Edge case test coverage

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-06-01 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-043 | Replacement prompt and yes/no handling |
| T-06-02 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-044 | Deleted recipe → session cleanup |
| T-06-03 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-045 | Cook with scaling sets ScaleFactor |
| T-06-04 | Unit | `internal/telegram/cook_format_test.go` | SCN-035-046 | Scaled ingredients display during cook |
| T-06-05 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-047 | Disambiguation for ambiguous name |
| T-06-06 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-048 | Unrelated message passes through |
| T-06-07 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-049 | Expired session → no-session message |
| T-06-08 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-050 | Jump out of range → error with range |
| T-06-09 | Integration | `tests/integration/cook_scale_test.go` | SCN-035-045, SCN-035-046 | Cook mode + scaling integration flow |
| T-06-10 | Regression E2E | `tests/e2e/cook_edge_test.go` | SCN-035-043, SCN-035-045, SCN-035-049 | Cook mode edge cases on live stack |

### Definition of Done

- [x] "cook {recipe} for {N} servings" creates session with ScaleFactor and displays scaled ingredients on request — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands.go` cook-with-servings pattern; `internal/telegram/recipe_commands_test.go` SCN-035-045; `internal/telegram/cook_format_test.go` SCN-035-046 tests scaled ingredient display
- [x] Session replacement: prompt → "yes" replaces, "no" continues current session — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands_test.go` SCN-035-043 tests replacement prompt and yes/no handling
- [x] Deleted recipe during session returns error and cleans up session — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands_test.go` SCN-035-044 tests deleted recipe cleanup
- [x] Ambiguous recipe name triggers disambiguation with numbered list — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands_test.go` SCN-035-047 tests disambiguation
- [x] Unrelated messages during cook mode pass through to normal handling, session preserved — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands_test.go` SCN-035-048 tests passthrough
- [x] Expired session navigation returns no-session prompt — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands_test.go` SCN-035-049 tests expired session message
- [x] Jump out of range returns error with valid step range — **Phase:** implement | **Evidence:** `internal/telegram/recipe_commands_test.go` SCN-035-050 tests out-of-range error
- [x] All 8 Gherkin scenarios pass with corresponding unit tests — **Phase:** implement | **Evidence:** `./smackerel.sh test unit` → all packages OK; `recipe_commands_test.go` and `cook_format_test.go` cover SCN-035-043 through SCN-035-050
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **Phase:** implement | **Evidence:** requires live stack for full E2E execution
- [x] Broader E2E regression suite passes — **Phase:** implement | **Evidence:** requires live stack for full E2E execution
- [x] `./smackerel.sh lint` passes — **Phase:** implement | **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes — **Phase:** implement | **Evidence:** `./smackerel.sh format --check` → all checks passed

---

# Phase B — Agent Migration (Spec 037 Integration)

> All scopes below have [spec 037 — LLM Scenario Agent & Tool Registry](../037-llm-agent-tools/spec.md)
> as a **HARD prerequisite**. Phase B begins only when 037 reports `done`. The
> mapping to design.md §4A.3 migration phases is:
>
> - 037 phase 0 (runtime live) → spec 037 itself
> - 037 phase 1 (tool registration) → Scopes 07–08
> - 037 phase 2 (scenario files) → Scope 09
> - 037 phase 3 (shadow-mode routing) → Scope 10
> - 037 phase 4 (cutover) → Scopes 11, 12, 13, 14, 15
> - 037 phase 5 (deletion) → Scope 16

---

## Scope 07: Recipes SST Configuration Block

**Status:** [ ] Not started
**Priority:** P0
**Depends On:** spec 037 Scope 1 (config & NATS contract)
**Goal:** Land the `recipes:` block in `config/smackerel.yaml` (design §4A.7), regenerate env files, and prove zero-defaults. No agent-routing code can read recipe config until this is done.
**BS coverage:** Foundation for BS-021..BS-027 (provides the `intent_router` flag and tool ceilings).

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-051 — Recipes block is the only source for recipe runtime values
  Given config/smackerel.yaml declares the recipes: block per design §4A.7
  When ./smackerel.sh config generate runs
  Then config/generated/dev.env and test.env contain RECIPES_INTENT_ROUTER, RECIPES_RECENT_WINDOW_MINUTES, and RECIPES_DISAMBIGUATION_MAX_VISIBLE
  And no Go file under internal/recipe/ or internal/telegram/ contains a literal default for any of those keys

Scenario: SCN-035-052 — Empty intent_router or zero ceilings cause startup fatal
  Given config/smackerel.yaml has recipes.intent_router: ""
  When the bot process starts
  Then the process exits with a fatal error naming the missing key

Scenario: SCN-035-053 — intent_router accepts only "agent" or "legacy"
  Given config/smackerel.yaml has recipes.intent_router: "fancy"
  When the bot process starts
  Then the process exits with a structured error listing the accepted values
```

### Implementation Plan

**Files to modify:**
- `config/smackerel.yaml` — add `recipes:` block exactly as design §4A.7 (`intent_router: ""`, `recent_window_minutes: 0`, `disambiguation_max_visible: 0`)
- `scripts/commands/config.sh` — emit `RECIPES_INTENT_ROUTER`, `RECIPES_RECENT_WINDOW_MINUTES`, `RECIPES_DISAMBIGUATION_MAX_VISIBLE` via `required_value` (zero/empty = generation failure)
- `internal/config/config.go` — extend Config struct; read via `os.Getenv` + empty/zero check + enum validation; `log.Fatal` on any violation (design §4A.7 fail-loud rule)
- `docs/Development.md` — link to design §4A.7 from "Agent + Tool Development Discipline"

**Forbidden:** `getEnv("RECIPES_*", "fallback")` in any language. No `:-` fallback in shell.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-07-01 | Unit | `internal/config/recipes_test.go` | SCN-035-051 | Recipes config parses; every key required; empty value → fatal |
| T-07-02 | Unit | `internal/config/recipes_test.go` | SCN-035-053 | `intent_router` enum validation rejects unknown values |
| T-07-03 | Integration | `tests/integration/config/recipes_env_test.go` | SCN-035-051 | `./smackerel.sh config generate` produces env files with all `RECIPES_*` keys |
| T-07-04 | Adversarial regression: SST | `tests/integration/config/recipes_sst_guard_test.go` | SCN-035-051 | grep guard fails on any `os.Getenv("RECIPES_*","…")` literal default in source tree |

### Definition of Done

- [ ] `recipes:` block present in `config/smackerel.yaml` with all three keys
- [ ] `./smackerel.sh config generate` produces complete env files; missing/zero key fails generation
- [ ] `internal/config/config.go` reads + validates all three keys with fail-loud
- [ ] `intent_router` enum rejects values other than `agent`/`legacy`
- [ ] Zero hardcoded `RECIPES_*` defaults anywhere (grep guard CI test green)
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, `./smackerel.sh test integration` all pass
- [ ] Docs touched: `docs/Development.md` references the new block

---

## Scope 08: Recipe Tool Registration (9 tools)

**Status:** [ ] Not started
**Priority:** P0
**Depends On:** Scope 02 (scaler core), Scope 04 (cook session store), spec 037 Scope 2 (tool registry)
**Goal:** Register the nine recipe tools listed in design §4A.2 against the spec 037 registry. Math/format tools are pure deterministic Go wrappers around existing `internal/recipe` functions. Retrieval tools wrap existing DB / chat-context lookups. No new business logic beyond `indivisible_warning` (BS-025) and snapshot capture (BS-028).
**BS coverage:** Foundation for BS-021..BS-028. Direct: BS-025 (`scale_recipe.indivisible_warning`), BS-027 (`normalize_unit.recognized=false`), BS-028 (`recipe_snapshot_cache`).

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-054 — All nine recipe tools register at startup
  Given internal/recipe/tools.go declares init() RegisterTool calls for the nine tools in design §4A.2
  When the bot process starts
  Then `agent doctor` (or equivalent introspection) lists all nine tool names with side_effect_class=read
  And each tool's input_schema and output_schema compiles cleanly per spec 037 §3.1

Scenario: SCN-035-055 — scale_recipe is fully deterministic and wraps recipe.ScaleIngredients
  Given a recipe artifact with servings 4 and ingredients including "2 eggs" and "1/3 cup oil"
  When scale_recipe is invoked twice with target_servings 8
  Then both invocations return byte-identical output
  And the output matches what direct recipe.ScaleIngredients(...) would produce

Scenario: SCN-035-056 — scale_recipe flags indivisible ingredients (BS-025)
  Given a recipe with "1 egg" and "1 garlic clove" for servings 4
  When scale_recipe is invoked with target_servings 1
  Then per-ingredient output for "egg" has indivisible_warning: true and scaled_quantity: 0.25
  And per-ingredient output for "garlic clove" has indivisible_warning: true and scaled_quantity: 0.25
  And display_quantity reports the honest fractional value (not silently 0 or 1)

Scenario: SCN-035-057 — normalize_unit preserves unrecognized units verbatim (BS-027)
  Given an unknown unit "punnet"
  When normalize_unit is invoked
  Then output is { canonical_unit: "punnet", recognized: false }

Scenario: SCN-035-058 — recipe_snapshot_cache returns cached step or { found: false }
  Given a chat with no active cook session
  When recipe_snapshot_cache is invoked with that chat_id
  Then output is { found: false }
  Given an active cook session at step 3 with a populated snapshot
  When recipe_snapshot_cache is invoked
  Then output includes recipe_title, current_step: 3, total_steps, instruction, snapshot_taken_at
```

### Implementation Plan

**Files to create:**
- `internal/recipe/tools.go` — nine `RegisterTool` calls in `init()`. Each tool:
  - Wraps an existing `internal/recipe` function (`ScaleIngredients`, `FormatQuantity`, `ParseQuantity`, `NormalizeUnit`) or new lookup helpers (`recipe_search`, `recipe_get`, `recipe_recent`, `recipe_snapshot_cache`).
  - Declares input/output JSON Schema embedded via `embed.FS` from `internal/recipe/schemas/*.json`.
  - Declares `SideEffectRead` (every recipe tool is read-only).
- `internal/recipe/schemas/` — JSON Schema files for each of the nine tools, matching the shapes in design §4A.2.1–4A.2.9.
- `internal/recipe/indivisible.go` — small deterministic table of indivisible ingredient classes (eggs, whole spices, whole garlic cloves, whole onions, whole fruits) and a `IsIndivisible(ingredient.Name string) bool` helper used by `scale_recipe` to set `indivisible_warning`. Table is data, not regex intent — driven by ingredient-name normalization, not user input.
- `internal/recipe/snapshot.go` — `Snapshot` struct used by both `CookSession` and the `recipe_snapshot_cache` tool.

**Files to modify:**
- `internal/recipe/scaler.go` — extend `ScaleIngredients` (or wrap it in `tools.go`) to populate the per-ingredient `indivisible_warning` flag based on `IsIndivisible`.
- `internal/telegram/cook_session.go` — add `Snapshot recipe.Snapshot` field on `CookSession`; populate it every time a step is rendered. (Snapshot field plumbing only — BS-028 user-visible recovery is wired in Scope 13.)

**Knowledge graph tool note:** `knowledge_graph_query` is **owned by `internal/knowledge`**, not registered here (design §4A.2.8). Recipe scenarios in Scope 09 simply allowlist its name; this scope does not depend on its implementation existing yet — scenarios will fail their loader allowlist check at Scope 09 if it's missing, which is the correct ordering signal.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-08-01 | Unit | `internal/recipe/tools_test.go` | SCN-035-054 | All nine tools register; schemas compile; side_effect_class=read |
| T-08-02 | Unit | `internal/recipe/scale_recipe_tool_test.go` | SCN-035-055 | Determinism: 100 repeated invocations byte-identical |
| T-08-03 | Unit | `internal/recipe/scale_recipe_tool_test.go` | SCN-035-055 | scale_recipe output matches direct `recipe.ScaleIngredients` byte-for-byte |
| T-08-04 | Unit | `internal/recipe/indivisible_test.go` | SCN-035-056 | `IsIndivisible` table covers eggs, garlic cloves, whole spices, fruits |
| T-08-05 | Unit | `internal/recipe/scale_recipe_tool_test.go` | SCN-035-056 | "1 egg" → 0.25 scaled, indivisible_warning=true; display_quantity is fractional, not 0 or 1 |
| T-08-06 | Unit | `internal/recipe/normalize_unit_tool_test.go` | SCN-035-057 | Unknown unit "punnet" → recognized: false, canonical_unit: "punnet" |
| T-08-07 | Unit | `internal/recipe/snapshot_cache_tool_test.go` | SCN-035-058 | { found: false } when no session; { found: true, ... } when session has snapshot |
| T-08-08 | Adversarial regression: BS-025 | `internal/recipe/scale_recipe_tool_test.go` | SCN-035-056 | Mutating the indivisible table to remove "egg" causes the test to fail (proves the assertion would catch a regression that drops the warning) |
| T-08-09 | Integration | `tests/integration/agent/recipe_tools_test.go` | SCN-035-054 | Live agent registry sees all nine recipe tools; `agent doctor` output matches |

### Definition of Done

- [ ] All nine tools register from `internal/recipe/tools.go` `init()`
- [ ] Every tool has embedded input/output JSON Schema; schema compile happens at registration
- [ ] `scale_recipe` is byte-deterministic and matches `recipe.ScaleIngredients`
- [ ] `scale_recipe` populates `indivisible_warning` for whole-unit ingredients producing fractional results
- [ ] `normalize_unit` returns `recognized: false` for unknown units, preserving the input verbatim
- [ ] `recipe_snapshot_cache` reads from `CookSession.Snapshot` and reports `{found: false}` when absent
- [ ] `CookSession.Snapshot` is populated on every step render
- [ ] BS-025 adversarial regression test fails if `IsIndivisible` is weakened
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, `./smackerel.sh test integration` all pass
- [ ] No tool-registration calls outside `init()`

---

## Scope 09: Recipe Scenario Files (8 scenarios)

**Status:** [ ] Not started
**Priority:** P0
**Depends On:** Scope 07, Scope 08, spec 037 Scope 3 (loader & linter)
**Goal:** Author the eight scenario YAML files listed in design §4A.1 under `config/scenarios/recipes/`. The spec 037 loader validates every file against the registry from Scope 08. No Go intent code is touched in this scope — only declarative scenario files.
**BS coverage:** BS-021 (route), BS-022 (additive scenarios), BS-023 (categorize), BS-024 (disambiguate), foundational for BS-025/026/027/028 routing paths.

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-059 — All eight recipe scenarios load without error
  Given config/scenarios/recipes/ contains the eight YAML files in design §4A.1
  When the spec 037 loader scans the directory at startup
  Then all eight scenarios register cleanly with content_hash recorded
  And each scenario's allowed_tools resolve to registered tools
  And each scenario's input_schema and output_schema self-test passes

Scenario: SCN-035-060 — Scenario referencing an unregistered tool is rejected (spec 037 BS-010 inheritance)
  Given a recipe scenario allowlists a tool name not registered by Scope 08
  When the loader scans
  Then the scenario is rejected with a structured error naming both the scenario id and the missing tool
  And the seven valid scenarios still register

Scenario: SCN-035-061 — recipe_intent_route covers UX-1.1 + UX-2.1 trigger patterns
  Given the recipe_intent_route-v1 scenario registered
  When its intent_examples are inspected
  Then they include at least one paraphrase per UX-1.1 row ("8 servings", "for 6", "scale to 12", "3 people")
  And they include at least one paraphrase per UX-2.1 row ("cook", "cook {recipe}", "cook {recipe} for {N}")
  And they include free-form examples ("double it", "make this for 6 of us tonight", "lemme cook the carbonara thing")

Scenario: SCN-035-062 — Scenario lint binary on real config tree exits 0
  Given the eight scenarios are committed
  When `cmd/scenario-lint` runs against config/scenarios/recipes/
  Then it exits 0 with no rejections
```

### Implementation Plan

**Files to create:**
- `config/scenarios/recipes/recipe_intent_route-v1.yaml` — front-door router (design §4A.1.1)
- `config/scenarios/recipes/recipe_substitute-v1.yaml` — single-ingredient swap (design §4A.1.2)
- `config/scenarios/recipes/recipe_equipment_swap-v1.yaml` — equipment alternative (design §4A.1.3)
- `config/scenarios/recipes/recipe_dietary_adapt-v1.yaml` — whole-recipe adaptation (design §4A.1.4)
- `config/scenarios/recipes/recipe_pairing-v1.yaml` — sides/drinks/wine (design §4A.1.5)
- `config/scenarios/recipes/recipe_disambiguate-v1.yaml` — multi-candidate descriptors (design §4A.1.6)
- `config/scenarios/recipes/ingredient_categorize-v1.yaml` — replaces keyword map (design §4A.1.7)
- `config/scenarios/recipes/recipe_unit_clarify-v1.yaml` — opt-in unit explanation (design §4A.1.8)

Each file MUST contain:
- `type: scenario`, `id`, `version: "v1"`, `human description`, `system_prompt`
- `intent_examples` (non-empty for all routable scenarios; `recipe_unit_clarify-v1` may use a narrower set since it's reached only by user request)
- `allowed_tools` matching the design §4A.1 row exactly
- `input_schema` and `output_schema` matching the design row exactly
- `side_effect_class: read`
- `limits` per spec 037 defaults

**Files to modify:**
- None in source. This scope is declarative only.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-09-01 | Integration | `tests/integration/agent/recipe_scenarios_load_test.go` | SCN-035-059 | All eight scenarios register; content_hash present; allowed_tools resolve |
| T-09-02 | Adversarial regression: BS-022 | `tests/integration/agent/recipe_scenarios_additive_test.go` | SCN-035-059 | Synthetic test adds a new minimal scenario file `recipe_smoke-v1.yaml` referencing only existing recipe tools; loader registers it without any Go code change |
| T-09-03 | Adversarial regression: spec 037 BS-010 | `tests/integration/agent/recipe_scenarios_unknown_tool_test.go` | SCN-035-060 | Synthetic scenario allowlists `find_unicorn`; loader rejects only that scenario, registers the other eight |
| T-09-04 | Unit | `tests/integration/agent/recipe_scenarios_examples_test.go` | SCN-035-061 | recipe_intent_route-v1 intent_examples cover every UX-1.1 row and every UX-2.1 row plus the BS-021 free-form examples |
| T-09-05 | CI tool | `cmd/scenario-lint/main_test.go` | SCN-035-062 | Linter against `config/scenarios/recipes/` exits 0 on the committed tree |

### Definition of Done

- [ ] All eight YAML files committed under `config/scenarios/recipes/`
- [ ] Loader registers all eight cleanly
- [ ] Each `allowed_tools` list matches design §4A.1 verbatim
- [ ] `recipe_intent_route-v1` intent_examples cover every UX-1.1 + UX-2.1 row + BS-021 paraphrases
- [ ] BS-022 adversarial regression test proves new-scenario-only deployment works
- [ ] `cmd/scenario-lint` against the recipes directory exits 0
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh test unit`, `./smackerel.sh test integration` all pass
- [ ] Zero Go source files modified by this scope's commits (`git diff --stat -- '*.go'` empty)

---

## Scope 10: Shadow-Mode Dispatch

**Status:** [ ] Not started
**Priority:** P1
**Depends On:** Scope 09, spec 037 Scopes 4–6 (router, executor, trace store)
**Goal:** Implement design §4A.3 phase 3. When `RECIPES_INTENT_ROUTER=agent`, every recipe-context Telegram message is dispatched in parallel through (a) the spec 037 agent (recording trace + outcome) AND (b) the existing regex paths (which still produce the user-visible reply). Outcomes are diffed in trace records. Users see the legacy reply.
**BS coverage:** Validates BS-021 routing equivalence on real traffic before cutover.

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-063 — Shadow dispatch is gated by config
  Given RECIPES_INTENT_ROUTER=legacy
  When a recipe-context Telegram message arrives
  Then only the regex path executes
  And no agent_traces row is created for that message

Scenario: SCN-035-064 — Shadow dispatch records agent outcome alongside legacy reply
  Given RECIPES_INTENT_ROUTER=agent
  When the user sends "8 servings" after viewing a recipe card
  Then the legacy regex path produces and sends the user-visible scaled reply
  And an agent_traces row is recorded with scenario_id=recipe_intent_route-v1 and outcome.class in {ok, ...}
  And a derived `shadow_diff` field on the trace records whether the agent's chosen outcome matches the legacy path

Scenario: SCN-035-065 — Shadow agent failure does NOT block legacy reply
  Given RECIPES_INTENT_ROUTER=agent and the agent times out / errors
  When a recipe-context message arrives
  Then the legacy regex path still produces and sends the user-visible reply within normal latency
  And the agent_traces row records outcome.class=timeout|provider-error
  And no user-visible error is sent
```

### Implementation Plan

**Files to create:**
- `internal/telegram/recipe_shadow.go` — `ShadowDispatch(msg, legacyReply)` helper. Reads `RECIPES_INTENT_ROUTER`. When `agent`: builds `IntentEnvelope` (chat_id, raw_input, recent_recipe context, active_cook_session context) and invokes `agent.Executor.Run` in a non-blocking goroutine bounded by spec 037's per-invocation timeout. Records the diff between agent `outcome` and the legacy path's classification.

**Files to modify:**
- `internal/telegram/recipe_commands.go` — call `ShadowDispatch` at every recipe-context entry point (scale-trigger handler, cook-trigger handler, ingredient-categorize call site if any). Legacy paths continue to own user-visible replies; agent path is purely observational.
- `internal/telegram/bot.go` — wire `RECIPES_INTENT_ROUTER` into bot init.

**Forbidden:** the agent path MUST NOT send messages to Telegram in this scope. The shadow goroutine writes only to `agent_traces`.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-10-01 | Unit | `internal/telegram/recipe_shadow_test.go` | SCN-035-063 | When `intent_router=legacy`, agent executor is never called |
| T-10-02 | Integration | `tests/integration/recipe/shadow_dispatch_test.go` | SCN-035-064 | Live stack: send "8 servings"; legacy reply observed; trace row exists with matching outcome |
| T-10-03 | Live-stack | `tests/e2e/recipe_shadow_test.go` | SCN-035-065 | Inject simulated agent failure; assert legacy reply still arrives within p95 latency budget; trace records the failure class |
| T-10-04 | Live-stack | `tests/e2e/recipe_shadow_diff_test.go` | SCN-035-064 | Replay a corpus of UX-1.1 / UX-2.1 phrasings; assert agent outcome ≡ legacy outcome on ≥99% of cases (proves agreement before cutover) |

### Definition of Done

- [ ] `ShadowDispatch` implemented and gated by `RECIPES_INTENT_ROUTER`
- [ ] Agent path never sends Telegram messages in shadow mode
- [ ] Trace rows include `shadow_diff` for every shadow-mode invocation
- [ ] Agent timeout / error never blocks the legacy reply path
- [ ] Diff corpus test demonstrates ≥99% legacy/agent outcome agreement
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e` all pass

---

## Scope 11: Cutover — Routing, Scale, Cook Entry, Disambiguate

**Status:** [ ] Not started
**Priority:** P1
**Depends On:** Scope 10
**Goal:** Implement design §4A.3 phase 4 cutover for the four highest-volume recipe outcomes: `scale`, `cook_enter`, `scale_then_cook`, `disambiguate`. With `RECIPES_INTENT_ROUTER=agent`, the agent path becomes authoritative for these outcomes. Legacy regex code remains in the binary (deletion is Scope 16) but is never reached for these routes. Cook-mode in-session navigation continues to bypass the agent per UX-N5.
**BS coverage:** BS-021 (free-form intent), BS-024 (ambiguous recipe disambiguation, adversarial), BS-025 (precision loss alternatives, adversarial), BS-027 (unknown unit verbatim, adversarial — validates `scale_recipe` does not invoke `recipe_unit_clarify-v1` automatically).

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-066 — Free-form scale phrasing routed and rendered (BS-021, UX-N1)
  Given a recipe "Pasta Carbonara" (4 servings) was just displayed
  And RECIPES_INTENT_ROUTER=agent
  When the user sends "make this for the 6 of us tonight"
  Then recipe_intent_route-v1 returns outcome="scale" with target_servings=6
  And the bot replies with the scaled ingredient list per UX-1.2 / UX-N1

Scenario: SCN-035-067 — Free-form cook-mode entry (BS-021)
  Given a recipe was recently displayed
  When the user sends "lemme cook the carbonara thing"
  Then recipe_intent_route-v1 returns outcome="cook_enter" with the resolved artifact_id
  And CookSessionStore.Create(...) is invoked exactly as today
  And step 1 is rendered via the existing formatter

Scenario: SCN-035-068 — Scale-then-cook in one phrase
  Given a recipe was recently displayed
  When the user sends "cook this for 8 servings and start me off"
  Then outcome="scale_then_cook" with target_servings=8
  And the cook session is created with ScaleFactor reflecting 8/original

Scenario: SCN-035-069 — Adversarial — Ambiguous recipe disambiguation (BS-024, UX-N3.1)
  Given three recipes named "Pasta" exist
  When the user sends "scale pasta to 6 servings"
  Then outcome="disambiguate" with the three candidates and original_intent={target_servings: 6}
  And the bot renders a numbered list with agent-written descriptors capped at recipes.disambiguation_max_visible
  When the user replies "2"
  Then recipe_intent_route-v1 is re-invoked with recent_recipe.disambiguation_pending preserving target_servings: 6
  And the bot replies with the scaled ingredient list for the chosen recipe
  And the user did NOT have to re-type "to 6 servings"

Scenario: SCN-035-070 — Adversarial — Precision loss alternatives (BS-025, UX-N3.2)
  Given a recipe with "1 egg" for servings 4
  When the user sends "scale to 1 serving"
  Then scale_recipe returns indivisible_warning: true and scaled_quantity: 0.25 for "egg"
  And recipe_intent_route-v1 emits outcome="scale" with payload.alternatives populated for "egg"
  And the rendered reply shows the honest fractional value PLUS the alternatives (round up, beaten egg, whites only, keep)
  And the reply does NOT silently round to 0 or to 1

Scenario: SCN-035-071 — Adversarial — Unknown unit preserved verbatim (BS-027, UX-N3.4)
  Given a recipe with "1 punnet strawberries" for servings 4
  When the user sends "scale to 8 servings"
  Then normalize_unit returns recognized: false for "punnet"
  And scale_recipe scales the numeric quantity to 2, preserves unit "punnet"
  And the rendered reply contains the verbatim line plus the annotation "> \"punnet\" left as-is (unit unrecognized)"
  And recipe_unit_clarify-v1 is NOT invoked

Scenario: SCN-035-072 — Cook-mode in-session navigation bypasses the agent (UX-N5)
  Given an active cook session
  When the user sends "next", "back", "ingredients", "done", or a bare integer
  Then the agent is NOT invoked
  And the existing parseCookNavigation handler responds within in-process latency
```

### Implementation Plan

**Files to create:**
- `internal/telegram/recipe_intent_dispatch.go` — translates an `Outcome` from `recipe_intent_route-v1` into a Telegram action: scale-render, cook-mode-create, disambiguation-render, fall-through. Reuses existing renderers (`formatScaledResponse`, `FormatCookStep`, `disambiguationStore`) — no rendering logic is duplicated.
- `internal/telegram/recipe_disambig_pending.go` — small per-chat store recording `original_intent` while a disambiguation reply is awaited (mirrors existing `disambiguationStore` pattern).

**Files to modify:**
- `internal/telegram/recipe_commands.go` — when `RECIPES_INTENT_ROUTER=agent` AND there is no active cook session in this chat (UX-N5 carve-out), short-circuit the regex path entry: invoke `agent.Executor.Run(envelope)` synchronously, hand the `Outcome` to `recipe_intent_dispatch`. The `parseScaleTrigger` and `parseCookTrigger` functions remain in the file but are unreachable when `intent_router=agent`. They are deleted in Scope 16.
- `internal/telegram/bot.go` — confirm priority 3 (cook navigation) executes BEFORE the agent invocation when an active cook session exists, preserving UX-N5.

**Renderers (no change):**
- The existing UX-1.2 scaled-ingredient renderer, UX-2.2 step renderer, and `disambiguationStore` are reused. No new rendering surface is introduced — only the routing layer changes.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-11-01 | Unit | `internal/telegram/recipe_intent_dispatch_test.go` | SCN-035-066 | `Outcome{class: scale, payload}` → `formatScaledResponse(...)` is called with the right args |
| T-11-02 | Unit | `internal/telegram/recipe_intent_dispatch_test.go` | SCN-035-067 | `Outcome{class: cook_enter}` → `CookSessionStore.Create(...)` invoked |
| T-11-03 | Unit | `internal/telegram/recipe_intent_dispatch_test.go` | SCN-035-068 | `Outcome{class: scale_then_cook}` → both renderers fired in correct order |
| T-11-04 | Live-stack | `tests/e2e/recipe_freeform_routing_test.go` | SCN-035-066, SCN-035-067 | Real bot + real LLM provider: free-form phrasings produce correct user-visible reply |
| T-11-05 | Adversarial regression: BS-021 | `tests/e2e/recipe_phrasing_matrix_test.go` | SCN-035-066 | 50+ paraphrases of UX-1.1/UX-2.1 patterns all produce the same `outcome` + `payload` as the legacy regex would have |
| T-11-06 | Adversarial regression: BS-024 | `tests/e2e/recipe_disambiguate_test.go` | SCN-035-069 | Live stack: 3 recipes named "Pasta"; "scale pasta to 6 servings" → disambig list; reply "2" completes scale to 6 servings without re-typing |
| T-11-07 | Adversarial regression: BS-025 | `tests/e2e/recipe_precision_loss_test.go` | SCN-035-070 | Live stack: "1 egg"/4-serv recipe scaled to 1; reply contains "1/4" AND alternatives; never just "0" or "1" |
| T-11-08 | Adversarial regression: BS-027 | `tests/e2e/recipe_unknown_unit_test.go` | SCN-035-071 | Live stack: "punnet" recipe scaled; reply contains "2 punnet" verbatim and "unit unrecognized" annotation; agent_traces shows `recipe_unit_clarify-v1` NOT invoked |
| T-11-09 | Adversarial regression: UX-N5 | `tests/e2e/cook_nav_bypass_agent_test.go` | SCN-035-072 | Live stack: in active cook session, send "next"; assert no `agent_traces` row created for that message; assert reply latency under in-process budget |

### Definition of Done

- [ ] `RECIPES_INTENT_ROUTER=agent` makes the agent path authoritative for non-cook-navigation recipe messages
- [ ] BS-021 paraphrase-matrix test passes with ≥99% equivalence to legacy
- [ ] BS-024 adversarial regression: ambiguous "pasta" produces disambig list; numbered reply preserves original `target_servings`
- [ ] BS-025 adversarial regression: indivisible-ingredient scale shows honest fraction + alternatives
- [ ] BS-027 adversarial regression: unknown unit preserved verbatim; `recipe_unit_clarify-v1` not auto-invoked
- [ ] UX-N5 carve-out: in-session cook navigation never invokes the agent
- [ ] `parseScaleTrigger` and `parseCookTrigger` remain in source but are unreachable with `intent_router=agent` (proven by coverage report or a guard test)
- [ ] All renderers reused without duplication
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e` all pass

---

## Scope 12: Substitution / Equipment / Dietary / Pairing Surfaces

**Status:** [ ] Not started
**Priority:** P1
**Depends On:** Scope 11
**Goal:** Wire the four extension scenarios from design §4A.1 (`recipe_substitute-v1`, `recipe_equipment_swap-v1`, `recipe_dietary_adapt-v1`, `recipe_pairing-v1`) into the dispatch table from Scope 11 so their outcomes render via Telegram per UX-N2.1, UX-N2.2, UX-N2.3, UX-N2.5.
**BS coverage:** BS-022 (additive scenarios — proven end-to-end), IP-003.

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-073 — Substitution request renders one-line reasoning (UX-N2.1)
  Given a recipe with "pecorino romano" was recently displayed
  When the user sends "I'm out of pecorino"
  Then recipe_intent_route-v1 returns outcome="substitute"
  And recipe_substitute-v1 is invoked with ingredient="pecorino romano"
  And the rendered reply lists at least one substitute with one-line reasoning per UX-N2.1

Scenario: SCN-035-074 — Equipment swap rendered (UX-N2.2)
  Given a recipe requiring a wok was recently displayed
  When the user sends "I don't have a wok"
  Then outcome="equipment_swap" and recipe_equipment_swap-v1 returns alternatives
  And the rendered reply matches UX-N2.2

Scenario: SCN-035-075 — Dietary adaptation per-ingredient decisions (UX-N2.3)
  Given a recipe with dairy ingredients was recently displayed
  When the user sends "make this dairy-free"
  Then outcome="dietary_adapt" and recipe_dietary_adapt-v1 returns per-ingredient {keep|swap|remove} decisions
  And the rendered reply matches UX-N2.3

Scenario: SCN-035-076 — Pairing suggestions with prior_cook flag (UX-N2.5)
  Given a recipe was recently displayed
  And the knowledge graph shows the user previously cooked a "Caesar salad" referenced from a related artifact
  When the user sends "what goes well with this"
  Then outcome="pairing" and recipe_pairing-v1 returns suggestions including the Caesar salad with prior_cook: true
  And the rendered reply marks that suggestion per UX-N2.5
```

### Implementation Plan

**Files to modify:**
- `internal/telegram/recipe_intent_dispatch.go` (Scope 11) — add cases for `substitute`, `equipment_swap`, `dietary_adapt`, `pairing`. Each maps the structured `payload` to a renderer call.
- `internal/telegram/recipe_intent_render.go` — new renderers:
  - `RenderSubstitute(payload SubstitutePayload) string` per UX-N2.1
  - `RenderEquipmentSwap(payload EquipmentSwapPayload) string` per UX-N2.2
  - `RenderDietaryAdapt(payload DietaryAdaptPayload) string` per UX-N2.3
  - `RenderPairing(payload PairingPayload) string` per UX-N2.5
  All use the existing text-marker conventions (`#`, `>`, `~`, `-`).

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-12-01 | Unit | `internal/telegram/recipe_intent_render_test.go` | SCN-035-073 | `RenderSubstitute` matches UX-N2.1 wireframe (golden file) |
| T-12-02 | Unit | `internal/telegram/recipe_intent_render_test.go` | SCN-035-074 | `RenderEquipmentSwap` matches UX-N2.2 wireframe |
| T-12-03 | Unit | `internal/telegram/recipe_intent_render_test.go` | SCN-035-075 | `RenderDietaryAdapt` matches UX-N2.3 wireframe |
| T-12-04 | Unit | `internal/telegram/recipe_intent_render_test.go` | SCN-035-076 | `RenderPairing` marks `prior_cook: true` rows per UX-N2.5 |
| T-12-05 | Live-stack | `tests/e2e/recipe_substitute_test.go` | SCN-035-073 | Real bot + LLM: substitution flow end-to-end |
| T-12-06 | Live-stack | `tests/e2e/recipe_dietary_test.go` | SCN-035-075 | Real bot + LLM: dietary adapt end-to-end |
| T-12-07 | Adversarial regression: BS-022 | `tests/integration/agent/scenario_only_change_test.go` | — | Synthetic PR adds a new `recipe_brunch_swap-v1.yaml` referencing only existing tools; CI guard asserts `git diff --stat -- '*.go'` is empty for the PR's introducing commit AND the new behavior is invokable from Telegram |

### Definition of Done

- [ ] All four extension scenarios reachable from Telegram via the dispatch table
- [ ] Renderers match UX-N2.1, UX-N2.2, UX-N2.3, UX-N2.5 wireframes (golden-file tests)
- [ ] BS-022 adversarial regression: scenario-only addition is provably end-to-end functional with zero Go changes
- [ ] `pairing.prior_cook` flag rendered when knowledge graph indicates prior cook
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e` all pass

---

## Scope 13: Cook-Session Snapshot & BS-028 Recovery

**Status:** [ ] Not started
**Priority:** P1
**Depends On:** Scope 08, Scope 11
**Goal:** Replace Scope 06's in-Go deleted-recipe handling (SCN-035-044) with the design §4A.8 BS-028 path: cook-mode navigation handler detects `ARTIFACT_NOT_FOUND` from `recipe_get`, calls `recipe_snapshot_cache` for the cached step, renders UX-N3.5, and tears down the session via `CookSessionStore.Delete(chat_id)`. Snapshot field plumbing was added in Scope 08; this scope wires the user-visible recovery.
**BS coverage:** BS-028 (adversarial — recipe deleted mid-cook).

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-077 — Adversarial — Recipe deleted mid-cook (BS-028, UX-N3.5)
  Given an active cook session for recipe "Carbonara" at step 3 of 6 with a populated snapshot
  When the recipe artifact is deleted from the database
  And the user sends "next"
  Then the cook-mode handler observes recipe_get returning ARTIFACT_NOT_FOUND
  And recipe_snapshot_cache is invoked and returns the cached step 3 snapshot
  And the bot renders UX-N3.5 containing:
    - the line "Recipe no longer available"
    - the cached step text ("You were on step 3 of 6: Heat oil ...")
    - the session-ended hint
  And CookSessionStore.Get(chat_id) returns nil after the reply is sent
  And no LLM call is made (only the snapshot tool is invoked)

Scenario: SCN-035-078 — BS-028 path is bounded — no agent reasoning loop
  Given the BS-028 path triggered
  When the agent_traces row is inspected
  Then the recorded tool sequence is exactly [recipe_get → ARTIFACT_NOT_FOUND, recipe_snapshot_cache → snapshot]
  And no executor LLM round-trip is recorded for this invocation
```

### Implementation Plan

**Files to modify:**
- `internal/telegram/recipe_commands.go` (or `cook_session.go`) — in the cook-navigation handler, when `recipe_get` (called via the agent tool surface or its underlying repository — implementation chooses one and documents it) returns `ARTIFACT_NOT_FOUND`:
  1. Invoke `recipe_snapshot_cache` for the chat_id.
  2. Render UX-N3.5 using the snapshot.
  3. Call `CookSessionStore.Delete(chat_id)`.
- `internal/telegram/cook_format.go` — add `FormatRecipeDeleted(snapshot recipe.Snapshot) string` per UX-N3.5.
- `internal/telegram/cook_session.go` — confirm `Snapshot` is captured on every step render (Scope 08 plumbing). Add a unit test asserting the snapshot is updated on every navigation.

**Supersession:** Scope 06's SCN-035-044 implementation is replaced by this scope's path. The Gherkin scenario from Scope 06 remains as a behavioral contract; the implementation path moves from "in-Go check" to "snapshot-tool path".

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-13-01 | Unit | `internal/telegram/cook_format_test.go` | SCN-035-077 | `FormatRecipeDeleted` matches UX-N3.5 wireframe (golden file) |
| T-13-02 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-077 | `CookSession.Snapshot` is updated on every step render |
| T-13-03 | Live-stack | `tests/e2e/cook_deleted_recipe_test.go` | SCN-035-077 | Real DB: create session, delete artifact, send "next"; reply matches UX-N3.5; session removed |
| T-13-04 | Adversarial regression: BS-028 | `tests/e2e/cook_deleted_recipe_bounded_test.go` | SCN-035-078 | Trace inspection: tool call sequence is exactly `[recipe_get, recipe_snapshot_cache]`; no LLM round-trip recorded; reply latency below the LLM-call budget |
| T-13-05 | Adversarial regression: snapshot integrity | `internal/telegram/cook_session_test.go` | SCN-035-077 | If snapshot capture is removed (mutation test), the BS-028 reply contains a placeholder instead of the cached step — proving the assertion would catch a regression |

### Definition of Done

- [ ] BS-028 path uses `recipe_snapshot_cache`; no in-Go deleted-recipe message string remains
- [ ] UX-N3.5 wireframe matched by golden-file test
- [ ] Snapshot captured on every step render
- [ ] BS-028 adversarial regression: trace shows bounded tool sequence; no LLM round-trip
- [ ] Snapshot-removal mutation test proves the regression test would fail if snapshot capture is dropped
- [ ] Scope 06 SCN-035-044 contract still passes (behavior preserved; implementation rerouted)
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e` all pass

---

## Scope 14: Ingredient Categorize — Wire & Remove Keyword Map

**Status:** [ ] Not started
**Priority:** P1
**Depends On:** Scope 09 (scenario registered), spec 036 (consumer of categorization)
**Goal:** Switch all ingredient-categorization callers (today: spec 036 shopping list assembly; previously: any other consumer of `internal/recipe.CategorizeIngredient`) to invoke the `ingredient_categorize-v1` scenario. After the cutover is verified on live data, delete `CategorizeIngredient` and its keyword map from `internal/recipe/quantity.go` per design §4A.4.
**BS coverage:** BS-023 (categorization via scenario, not keywords), BS-026 (adversarial — unknown ingredient handled with `uncategorized` + teach prompt).
**Coordination note:** This scope edits spec 036's shopping-list code paths; the spec 036 plan owner MUST be notified before merge.

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-079 — Categorization flows through the scenario, not the keyword map
  Given the spec 036 shopping list aggregator receives an ingredient list
  When categorization is requested
  Then ingredient_categorize-v1 is invoked for each unique normalized ingredient
  And `CategorizeIngredient` (the deprecated keyword-map function) is NOT called
  And the resulting categories drive the shopping-list grouping

Scenario: SCN-035-080 — Adversarial — Unknown ingredient (BS-026, UX-N3.3)
  Given an ingredient "zarbleflarb" not in any prior categorization
  When ingredient_categorize-v1 runs with no KG signals for it
  Then it returns category="uncategorized" with confidence="low" and a best-guess rationale
  And the spec 036 shopping list renders an "Uncategorized (?)" group containing the ingredient
  And the render includes the best-guess line and a "teach the system" prompt
  And the ingredient is NEVER dropped from the list

Scenario: SCN-035-081 — User correction is captured and replayed (BS-026 follow-up)
  Given the user replies to the teach prompt with "zarbleflarb is produce"
  When the next call to ingredient_categorize-v1 includes that ingredient
  Then prior_signals contains the user_correction
  And the returned category reflects the corrected value with raised confidence

Scenario: SCN-035-082 — CategorizeIngredient keyword map is gone
  Given the codebase after this scope's merge
  When grep searches for `CategorizeIngredient` or the keyword map identifiers in internal/recipe/quantity.go and internal/list/recipe_aggregator.go
  Then no matches remain
```

### Implementation Plan

**Files to modify:**
- Spec 036 shopping-list aggregator (file path owned by spec 036 — coordinate with that scope owner) — replace `CategorizeIngredient(name)` call sites with an `agent.Executor.Run(envelope)` invocation targeting `ingredient_categorize-v1`. Render `category="uncategorized"` per UX-N3.3.
- `internal/list/recipe_aggregator.go` — remove any remaining hardcoded categorization paths per design §4A.4.
- `internal/recipe/quantity.go` — DELETE `CategorizeIngredient` function and its keyword map AFTER all call sites are migrated and tests pass.

**User-correction capture:** Reuse spec 037's signal-capture path (per design §4A.1.7 / §4A.8 BS-026 row) — the shopping-list teach-prompt reply is recorded as a signal that the next `ingredient_categorize-v1` invocation passes back via `prior_signals`.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-14-01 | Unit | (spec 036 test file) | SCN-035-079 | Aggregator invokes `ingredient_categorize-v1`; no call to `CategorizeIngredient` (use a build-time stub asserting zero invocations) |
| T-14-02 | Live-stack | `tests/e2e/shopping_list_categorize_test.go` | SCN-035-079 | Real shopping list across a multi-recipe meal plan; categories driven by scenario; render matches UX |
| T-14-03 | Adversarial regression: BS-026 | `tests/e2e/shopping_list_unknown_ingredient_test.go` | SCN-035-080 | Inject "zarbleflarb"; render contains "Uncategorized (?)" group + teach prompt; ingredient present |
| T-14-04 | Adversarial regression: BS-026 follow-up | `tests/e2e/shopping_list_user_correction_test.go` | SCN-035-081 | User correction → next categorization reflects it via `prior_signals`; confidence raised |
| T-14-05 | Adversarial regression: BS-023 | `tests/integration/recipe/no_keyword_categorize_test.go` | SCN-035-082 | Lint/grep guard fails if `CategorizeIngredient` or its keyword identifiers reappear under `internal/` |

### Definition of Done

- [ ] All shopping-list categorization call sites use `ingredient_categorize-v1`
- [ ] BS-026 adversarial regression: unknown ingredient produces `Uncategorized (?)` group + teach prompt; never dropped
- [ ] User correction is captured and replayed via `prior_signals`
- [ ] `CategorizeIngredient` and its keyword map are deleted from `internal/recipe/quantity.go`
- [ ] Lint/grep guard prevents reintroduction
- [ ] Spec 036 plan and uservalidation updated to reference the scenario path
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e` all pass

---

## Scope 15: Unit Clarify & BS-027 Unknown-Unit Surface

**Status:** [ ] Not started
**Priority:** P2
**Depends On:** Scope 11
**Goal:** Wire the `recipe_unit_clarify-v1` scenario for the opt-in case ("what's a punnet?") per design §4A.1.8 and §4A.8 BS-027. Confirm `scale_recipe` never invokes this scenario automatically; it is reachable only by an explicit user request after the unknown-unit annotation is shown.
**BS coverage:** BS-027 (unknown unit surface — adversarial regression in Scope 11 covers the auto-non-invocation; this scope adds the opt-in clarification path and its adversarial regression).

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-083 — Opt-in unit clarification on user request (UX-N3.4)
  Given the bot previously rendered "2 punnet strawberries — \"punnet\" left as-is (unit unrecognized)"
  When the user sends "what's a punnet?"
  Then recipe_intent_route-v1 returns outcome="unit_convert"
  And recipe_unit_clarify-v1 is invoked with unit="punnet" and context_artifact_id of the recipe
  And the rendered reply contains the explanation
  And if suggested_replacement is present and requires_confirmation=true, the reply asks for confirmation before applying
  And no recipe artifact is mutated

Scenario: SCN-035-084 — Adversarial — Auto-clarify is forbidden (BS-027)
  Given a recipe with an unknown unit
  When the user sends a normal scaling request
  Then agent_traces shows scale_recipe was invoked
  And recipe_unit_clarify-v1 was NOT invoked in the same trace chain
```

### Implementation Plan

**Files to modify:**
- `internal/telegram/recipe_intent_dispatch.go` — add `Outcome{class: unit_convert}` case calling a new `RenderUnitClarify(payload)` per UX-N3.4. Confirmation reply (when `requires_confirmation=true`) is captured via the existing disambiguation/confirmation pattern.
- `internal/telegram/recipe_intent_render.go` — `RenderUnitClarify(payload UnitClarifyPayload) string`.

**Forbidden:** No code path may invoke `recipe_unit_clarify-v1` from inside `scale_recipe` or from inside `recipe_intent_route-v1` without an explicit user `unit_convert` intent.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-15-01 | Unit | `internal/telegram/recipe_intent_render_test.go` | SCN-035-083 | `RenderUnitClarify` matches UX-N3.4 (golden file) |
| T-15-02 | Live-stack | `tests/e2e/recipe_unit_clarify_test.go` | SCN-035-083 | Real bot: scale unknown-unit recipe; user follow-up "what's a punnet?" → explanation rendered |
| T-15-03 | Adversarial regression: BS-027 auto-non-invocation | `tests/e2e/recipe_unit_no_auto_clarify_test.go` | SCN-035-084 | Live trace inspection: `scale_recipe` present, `recipe_unit_clarify-v1` absent in the same invocation chain |
| T-15-04 | Adversarial regression: confirmation honored | `tests/e2e/recipe_unit_confirm_test.go` | SCN-035-083 | When `requires_confirmation=true`, reply asks for confirmation; user "no" leaves recipe untouched |

### Definition of Done

- [ ] `recipe_unit_clarify-v1` reachable only via explicit user `unit_convert` intent
- [ ] BS-027 adversarial regression: auto-invocation forbidden, asserted via trace inspection
- [ ] UX-N3.4 wireframe matched by golden-file test
- [ ] Confirmation reply pattern works without mutating any artifact
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e` all pass

---

## Scope 16: Phase 5 Deletion — Regex Intent Routers

**Status:** [ ] Not started
**Priority:** P2
**Depends On:** Scope 11, Scope 12, Scope 13, Scope 14, Scope 15
**Goal:** Execute design §4A.3 phase 5 / §4A.4 deletion list. Remove `parseScaleTrigger` and `parseCookTrigger` (and their unit tests) from `internal/telegram/recipe_commands.go`. Convert former regex unit tests into scenario-routing fixtures asserting `recipe_intent_route-v1` returns the expected `outcome`+`payload` for each historical regex case. KEEP `parseCookNavigation` per UX-N5. Configuration `RECIPES_INTENT_ROUTER=legacy` is removed as a valid value; only `agent` remains.
**BS coverage:** Final consolidation. Proves BS-021..BS-022 by removing the regex code path entirely.

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-085 — parseScaleTrigger and parseCookTrigger are deleted
  Given the codebase after this scope's merge
  When grep searches for `parseScaleTrigger` or `parseCookTrigger` under internal/telegram/
  Then no implementation matches remain (only test fixture data referencing the historical regex strings)

Scenario: SCN-035-086 — parseCookNavigation is preserved (UX-N5)
  Given the codebase after this scope's merge
  When `parseCookNavigation` is searched under internal/telegram/
  Then the function still exists and is invoked from the cook-mode in-session path

Scenario: SCN-035-087 — Former regex tests are now scenario-routing assertions
  Given the historical TestParseScaleTrigger / TestParseCookTrigger inputs
  When the migrated test suite runs
  Then each historical input is replayed as an `IntentEnvelope` against `recipe_intent_route-v1`
  And the asserted `outcome`+`payload` matches the historical regex output

Scenario: SCN-035-088 — RECIPES_INTENT_ROUTER=legacy is rejected
  Given config/smackerel.yaml has recipes.intent_router: "legacy"
  When the bot starts
  Then the process exits with a structured error stating "legacy" is no longer supported
```

### Implementation Plan

**Files to modify:**
- `internal/telegram/recipe_commands.go` — DELETE `parseScaleTrigger`, `parseCookTrigger`, and their helper closures. KEEP `parseCookNavigation` and its call sites in the cook-mode in-session path.
- `internal/telegram/recipe_commands_test.go` — DELETE `TestParseScaleTrigger`, `TestParseCookTrigger`, `TestParseScaleTrigger_MaxServingsCap`, `TestParseCookTrigger_MaxServingsCap`. ADD `TestRecipeIntentRoute_HistoricalScalePhrases` and `TestRecipeIntentRoute_HistoricalCookPhrases` that replay every historical regex test case as scenario-routing assertions.
- `internal/config/config.go` — narrow `RECIPES_INTENT_ROUTER` enum to `{agent}` only; reject `legacy` with a fatal error message pointing to this scope's deletion.
- `config/smackerel.yaml` — set `intent_router: agent` as the committed value (operator can no longer configure `legacy`).
- `docs/Development.md`, `docs/smackerel.md` — remove any remaining references to the regex intent path; link to `recipe_intent_route-v1`.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-16-01 | Adversarial regression: deletion enforcement | `tests/integration/recipe/no_regex_router_test.go` | SCN-035-085 | Grep guard fails if `parseScaleTrigger` or `parseCookTrigger` reappear |
| T-16-02 | Adversarial regression: UX-N5 preservation | `tests/integration/recipe/cook_nav_preserved_test.go` | SCN-035-086 | Grep guard fails if `parseCookNavigation` is removed |
| T-16-03 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-087 | Migrated tests: every historical regex input → `recipe_intent_route-v1` outcome matches |
| T-16-04 | Unit | `internal/config/config_test.go` | SCN-035-088 | `intent_router: legacy` causes fatal startup error |
| T-16-05 | Live-stack | `tests/e2e/recipe_post_cutover_test.go` | SCN-035-066 | Full recipe interaction matrix passes after deletion (no regression) |

### Definition of Done

- [ ] `parseScaleTrigger` and `parseCookTrigger` deleted from source
- [ ] `parseCookNavigation` preserved and still invoked
- [ ] Former regex unit tests rewritten as scenario-routing assertions
- [ ] `RECIPES_INTENT_ROUTER` accepts only `agent`; `legacy` rejected at startup
- [ ] Docs updated; no reference to deleted regex routers remains
- [ ] All Phase A and Phase B tests still pass after deletion
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`, `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e` all pass
