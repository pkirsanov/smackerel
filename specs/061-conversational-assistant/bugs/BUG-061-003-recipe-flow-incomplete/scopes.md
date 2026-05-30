# Scopes: [BUG-061-003] Recipe end-to-end flow incomplete

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md) | [report.md](report.md)

## Scope 1: recipe_search skill end-to-end + router misspelling normalization

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: [Bug] Recipe retrieval routes to recipe_search instead of idea-capture

  Scenario: SCN-BUG061003-S01 Clean recipe utterance routes to recipe_search
    Given the recipe_search skill is enabled and the graph contains recipe artifacts
    When a user sends "find best recipe"
    Then the assistant returns a sourced recipe response with Sources[] non-empty
    And the reply is NOT the BandLow idea-capture string

  Scenario: SCN-BUG061003-S02 Misspelled recipe utterance routes via normalization
    Given the recipe_search skill is enabled
    When a user sends "find best recepie"
    Then the router normalizes the embed input to "find best recipe"
    And envelope.RawInput is preserved as "find best recepie"
    And routing reaches the recipe_search scenario at BandHigh

  Scenario: SCN-BUG061003-S03 Empty-graph zero-hit returns StatusUnavailable (adversarial)
    Given the recipe_search skill is enabled and the graph has no recipe artifacts
    When a user sends "find best recipe"
    Then Status=StatusUnavailable, ErrorCause=ErrNoMatch, CaptureRoute=false
    And the body names a next concrete action (capture | connector | import)
    And the body does NOT contain "saved as an idea"

  Scenario: SCN-BUG061003-S04 Telegram adapter does not render idea-capture for recipe path
    Given a recipe_search happy-path response (CaptureRoute=false)
    When the Telegram adapter renders the reply
    Then the sent message does NOT match /^\. Saved: ".*" \(idea\)$/

  Scenario: SCN-BUG061003-S05 Live-stack meal-plan -> shopping loop unaffected
    Given the live test stack is up
    When the /api/search endpoint is queried with filters.domain="recipe"
    Then the response is well-formed and does not contain the pre-fix idea-capture artifact title
```

### Implementation Plan
1. Add SST keys (`assistant.skills.recipe_search.*`, `assistant.rate_limit.recipe_search.*`, tier matrix entries) to `config/smackerel.yaml`; resolve in `scripts/commands/config.sh`.
2. Register `recipe_search` in `config/assistant/scenarios.yaml`; add `config/prompt_contracts/recipe-search-v1.yaml`.
3. Implement `internal/agent/normalize.go` (closed 4-entry alias map) and wire into `router.go` at the embed seam.
4. Implement `internal/agent/tools/recipesearch/tool.go` delegating to `api.SearchEngine` with `SearchFilters{Domain:"recipe"}`.
5. Add `contracts.ResponseOverride` + `contracts.SourceAssembly.Override` + `contracts.ErrNoMatch`.
6. Implement `internal/assistant/skills/recipesearch/assembler.go` (Override on empty Final, delegate to `retrieval.AssembleSources` on non-empty, zero-value on non-OK / malformed JSON).
7. Facade applies Override verbatim + skips provenance gate.
8. Wire registration in `cmd/core/wiring_{agent,assistant_facade,assistant_scenarios,assistant_skills}.go`; blank-import in `cmd/scenario-lint/main.go`.
9. Land tests S01–S05 + manifest D7 assertion + golden fixture for `unavailable_no_match_no_capture`.
10. (Post-gaps) Drop unpopulated `score` field from `recipeSearchHit` (Principle 8).
11. (Post-harden) Add assembler malformed-JSON adversarial test (this scope).

### Test Plan

| # | Type | Label | Test File / Command | Scenario |
|---|------|-------|---------------------|----------|
| 1 | unit | Router alias normalization | `internal/agent/normalize_test.go::TestNormalizeForRouting_AliasMap` | SCN-BUG061003-S02 |
| 2 | unit | Router pre-pass adversarial | `internal/agent/normalize_test.go::TestRouter_NormalizesBeforeEmbed_BUG061003` | SCN-BUG061003-S02 |
| 3 | unit | Assembler S01 populated | `internal/assistant/skills/recipesearch/assembler_test.go::TestRecipeAssembler_S01_PopulatesSources` | SCN-BUG061003-S01 |
| 4 | unit | Assembler S03 empty-graph adversarial | `assembler_test.go::TestRecipeAssembler_S03_EmptyGraph_OverrideUnavailable_Adversarial` | SCN-BUG061003-S03 |
| 5 | unit | Assembler non-OK guard | `assembler_test.go::TestRecipeAssembler_NonOKOutcome_NoOverride` | SCN-BUG061003-S03 |
| 6 | unit | Assembler malformed-JSON adversarial | `assembler_test.go::TestRecipeAssembler_OKOutcome_MalformedJSON_NoOverride_Adversarial` | SCN-BUG061003-S03 |
| 7 | unit | Scenario contract pin | `internal/assistant/skills/recipesearch/scenario_test.go::TestRecipeSearchScenarioContract_BUG061003` | SCN-BUG061003-S01 |
| 8 | unit (adapter) | Telegram adapter regression | `internal/telegram/assistant_adapter/bot_recipe_search_test.go::TestHandleUpdate_RecipeSearch_NotSavedAsIdea_BUG061003_S04` | SCN-BUG061003-S04 |
| 9 | unit (regex) | Pre-fix reply regex adversarial | `bot_recipe_search_test.go::TestSavedAsIdeaRegex_AdversarialMatchesPreFixReply_BUG061003` | SCN-BUG061003-S04 |
| 10 | unit (manifest) | D7 manifest shape | `internal/assistant/skills_manifest_test.go::TestLoadSkillsManifest_HappyPath` | SCN-BUG061003-S01 |
| 11 | e2e | Live-stack meal-plan→shopping loop | `tests/e2e/assistant_recipe_flow_test.go::TestE2E_MealPlanShoppingList_PopulatedAfterRecipeAssign` | SCN-BUG061003-S05 |

### Definition of Done — 3-Part Validation

#### Part A — Implementation
- [x] `recipe_search` scenario registered in `config/assistant/scenarios.yaml` with all required fields
   - **Evidence:** report.md → Phase: implement → Files Changed (owned) lists the scenarios.yaml entry; `./smackerel.sh check` shows `scenarios registered: 9, rejected: 0`.
- [x] SST keys declared in `config/smackerel.yaml` and emitted by `scripts/commands/config.sh`
   - **Evidence:** report.md → Phase: implement → §1 Config generation shows `RECIPE_SEARCH_TIMEOUT_MS=15000`, `ASSISTANT_SKILLS_RECIPE_SEARCH_ENABLED=true`, etc. in `config/generated/dev.env`.
- [x] Router NormalizeForRouting wired at embed seam with envelope.RawInput preserved
   - **Evidence:** report.md → Phase: regression → Step 5 "Router input pipeline" row marked 🟢 CLEAN.
- [x] Empty-graph Override path implemented and facade skips provenance gate
   - **Evidence:** `internal/assistant/skills/recipesearch/assembler.go` + report.md → Phase: regression → Step 3 design coherence.
- [x] Unpopulated `score` field removed from `recipeSearchHit` (Principle 8 remediation)
   - **Evidence:** report.md → Phase: gaps → Finding 1 (closed via option b).

#### Part B — Tests
- [x] S01 PASS — `TestRecipeAssembler_S01_PopulatesSources`
   - **Evidence:** report.md → Phase: test (re-verification at d0266558) → S01–S04 targeted unit run, raw log `/tmp/bug061003-s1234.out`.
- [x] S02 PASS — `TestNormalizeForRouting_AliasMap` + `TestRouter_NormalizesBeforeEmbed_BUG061003`
   - **Evidence:** Same log.
- [x] S03 PASS — `TestRecipeAssembler_S03_EmptyGraph_OverrideUnavailable_Adversarial` + non-OK guard
   - **Evidence:** Same log.
- [x] S04 PASS — `TestHandleUpdate_RecipeSearch_NotSavedAsIdea_BUG061003_S04` + adversarial regex
   - **Evidence:** Same log.
- [x] S05 PASS — `TestE2E_MealPlanShoppingList_PopulatedAfterRecipeAssign` against live stack
   - **Evidence:** report.md → Phase: test (re-verification) → S05 targeted e2e run (2.53s), raw log `/tmp/bug061003-s5.out`.
- [x] Adversarial malformed-JSON assembler test added (harden round)
   - **Evidence:** `internal/assistant/skills/recipesearch/assembler_test.go::TestRecipeAssembler_OKOutcome_MalformedJSON_NoOverride_Adversarial` + report.md → Phase: harden.
- [x] No silent-pass bailout patterns in regression tests (verified by `regression-quality-guard.sh --bugfix` in prior rounds).
- [x] All existing unit tests pass (no regressions)
   - **Evidence:** report.md → Phase: regression → Step 1 baseline `[go-unit] go test ./... finished OK` on `d0266558`.

#### Part C — Documentation & Closure
- [x] All 8 bug template artifacts present (bug.md, spec.md, design.md, scopes.md, scenario-manifest.json, uservalidation.md, report.md, state.json)
   - **Evidence:** harden-round backfill commit; `artifact-lint.sh` 8/8 PASS.
- [x] report.md contains pre-fix failure proof AND post-fix success proof
   - **Evidence:** report.md → Bug Reproduction — Before Fix (transcript) + Phase: test verdicts.
- [x] uservalidation.md initialized with checked-by-default entries
   - **Evidence:** `cat specs/061-conversational-assistant/bugs/BUG-061-003-recipe-flow-incomplete/uservalidation.md` shows 9 checked entries under `## Checklist` (harden-round backfill).
- [x] scenario-manifest.json maps S01–S05 to concrete test paths
   - **Evidence:** `cat specs/061-conversational-assistant/bugs/BUG-061-003-recipe-flow-incomplete/scenario-manifest.json` lists 5 scenarios each with `testMapping.file` + `testMapping.test` pointing at the on-disk test functions verified by `bubbles.test` round on `d0266558`.
- [ ] state.json transitioned to terminal via `bubbles.validate` (next phase per fastlane.phaseOrder).
