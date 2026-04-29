# Design: BUG-035-001 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [035 spec](../../spec.md) | [035 scopes](../../scopes.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`scopes.md` for spec 035 was authored before Gate G068 (Gherkin → DoD Content Fidelity) became routinely enforced. Most Gherkin scenarios already embed their `SCN-035-NNN` IDs in the scenario title (`Scenario: SCN-035-001 — ParseQuantity handles integers, decimals, fractions, and mixed numbers`), but the corresponding DoD bullets describe *delivery* in their own words and do not embed the `SCN-035-NNN` ID. For 21 specific scenario↔DoD pairs the fuzzy-fallback word overlap (≥3 significant ≥4-character non-stop-words) is not satisfied (e.g., "Unparseable quantities return zero value" vs "ParseQuantity, NormalizeUnit, NormalizeIngredientName extracted from recipe_aggregator" only shares the word `parseable`/`ParseQuantity`).

The guard's `scenario_matches_dod` function (in `.github/bubbles/scripts/traceability-guard.sh`) tries `extract_trace_ids` on **both** the scenario and the DoD bullet first; if both find the same `SCN-035-NNN` ID, the match succeeds regardless of word overlap. The fix exploits this fast path.

## 21 Unmapped Scenarios — Full List

| # | Scope | Scenario ID | Title | Owning DoD bullet to prefix |
|---|---|---|---|---|
| 1 | 01 | SCN-035-003 | Unparseable quantities return zero value | `ParseQuantity, NormalizeUnit, NormalizeIngredientName extracted from recipe_aggregator` |
| 2 | 02 | SCN-035-011 | Zero or negative servings returns nil | `Scaling handles: integers, fractions, mixed numbers, unparseable quantities` |
| 3 | 02 | SCN-035-014 | Mixed units scale independently (BS-016) | `All 9 Gherkin scenarios pass with corresponding unit tests` |
| 4 | 04 | SCN-035-027 | Update session advances step position | `Session CRUD: Create, Get, Delete operations work correctly` |
| 5 | 07 | SCN-035-051 | Recipes block is the only source for recipe runtime values | `Zero hardcoded \`RECIPES_*\` defaults anywhere (grep guard CI test green)` |
| 6 | 07 | SCN-035-052 | Empty intent_router or zero ceilings cause startup fatal | `\`internal/config/config.go\` reads + validates all three keys with fail-loud` |
| 7 | 09 | SCN-035-059 | All eight recipe scenarios load without error | `Loader registers all eight cleanly` |
| 8 | 09 | SCN-035-060 | Scenario referencing an unregistered tool is rejected (spec 037 BS-010 inheritance) | `BS-022 adversarial regression test proves new-scenario-only deployment works` |
| 9 | 10 | SCN-035-063 | Shadow dispatch is gated by config | `\`ShadowDispatch\` implemented and gated by \`RECIPES_INTENT_ROUTER\`` |
| 10 | 11 | SCN-035-066 | Free-form scale phrasing routed and rendered (BS-021, UX-N1) | `\`RECIPES_INTENT_ROUTER=agent\` makes the agent path authoritative for non-cook-navigation recipe messages` |
| 11 | 11 | SCN-035-067 | Free-form cook-mode entry (BS-021) | `BS-021 paraphrase-matrix test passes with ≥99% equivalence to legacy` |
| 12 | 11 | SCN-035-068 | Scale-then-cook in one phrase | `All renderers reused without duplication` |
| 13 | 11 | SCN-035-069 | Adversarial — Ambiguous recipe disambiguation (BS-024, UX-N3.1) | `BS-024 adversarial regression: ambiguous "pasta" produces disambig list; numbered reply preserves original \`target_servings\`` |
| 14 | 11 | SCN-035-070 | Adversarial — Precision loss alternatives (BS-025, UX-N3.2) | `BS-025 adversarial regression: indivisible-ingredient scale shows honest fraction + alternatives` |
| 15 | 12 | SCN-035-073 | Substitution request renders one-line reasoning (UX-N2.1) | `All four extension scenarios reachable from Telegram via the dispatch table` |
| 16 | 12 | SCN-035-074 | Equipment swap rendered (UX-N2.2) | `Renderers match UX-N2.1, UX-N2.2, UX-N2.3, UX-N2.5 wireframes (golden-file tests)` |
| 17 | 12 | SCN-035-075 | Dietary adaptation per-ingredient decisions (UX-N2.3) | `BS-022 adversarial regression: scenario-only addition is provably end-to-end functional with zero Go changes` |
| 18 | 13 | SCN-035-077 | Adversarial — Recipe deleted mid-cook (BS-028, UX-N3.5) | `BS-028 path uses \`recipe_snapshot_cache\`; no in-Go deleted-recipe message string remains` |
| 19 | 13 | SCN-035-078 | BS-028 path is bounded — no agent reasoning loop | `BS-028 adversarial regression: trace shows bounded tool sequence; no LLM round-trip` |
| 20 | 14 | SCN-035-079 | Categorization flows through the scenario, not the keyword map | `All shopping-list categorization call sites use \`ingredient_categorize-v1\`` |
| 21 | 15 | SCN-035-083 | Opt-in unit clarification on user request (UX-N3.4) | `\`recipe_unit_clarify-v1\` reachable only via explicit user \`unit_convert\` intent` |

## Fix Approach (artifact-only)

For each row above, prepend the DoD bullet text with `Scenario SCN-035-NNN (<full Gherkin scenario title>): ` so the trace-ID fast path of `scenario_matches_dod` fires. The original DoD claim text is preserved verbatim immediately after the prefix. Existing checkbox state (`[x]` for Phase A "Done" scopes, `[ ]` for Phase B "Not Started" scopes) and existing evidence blocks are preserved unchanged.

**Why this is not "DoD rewriting":** Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." This fix preserves the original DoD claim text verbatim and only adds a trace-ID + scenario-name prefix. The Gherkin Given/When/Then bodies are unchanged. No DoD bullet is deleted or weakened.

## Out-of-Boundary Items (NOT in this fix — see [bug.md](bug.md))

- Creating `specs/035-recipe-enhancements/scenario-manifest.json` (1 failure)
- Editing `specs/035-recipe-enhancements/report.md` to cite 4 existing test files (4 failures)
- Resolving 38 Phase B Test Plan rows whose referenced files don't exist yet (38 failures)
- Resolving Scope 14 T-14-01 row's `(spec 036 test file)` non-path (1 failure)

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability-guard run itself.

- Pre-fix: `RESULT: FAILED (65 failures, 0 warnings)`; G068 = `21 unmapped`.
- Expected post-fix: `RESULT: FAILED (43 failures, 0 warnings)`; G068 = `0 unmapped` (the 21 G068 failures + 1 G068 rollup are resolved; the 43 out-of-boundary failures remain).

The guard run is captured in `report.md` under "Validation Evidence".
