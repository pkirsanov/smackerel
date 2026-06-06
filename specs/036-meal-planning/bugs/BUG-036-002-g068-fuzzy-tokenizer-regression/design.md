# Design: BUG-036-002 — G068 fuzzy-tokenizer regression

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [036 spec](../../spec.md) | [036 scopes](../../scopes.md) | [036 report](../../report.md)
> **Date:** June 6, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`traceability-guard.sh` Gate G068 (`scenario_matches_dod`) matches a Gherkin
scenario to a DoD item in two stages:

1. **Trace-ID match** — if the scenario's `SCN-036-NNN` appears in the DoD
   item's trace IDs, it matches immediately.
2. **Fuzzy word overlap** — otherwise, significant-word overlap (score ≥ 3 AND
   score ≥ `ceil(50%)` of the scenario's significant words).

The in-script comments document a v3.8.0 "G068 false-positive fix" that
(a) lowered the minimum significant-word length from 4 to 3 and (b) trimmed the
stop-word exclusion list. Both change which tokens count toward the fuzzy
`score` / `word_count` / threshold. For 12 Done-scope scenarios whose DoD
bullets shared only a few significant words with the scenario title, this
tipped the fuzzy score below `ceil(50%)`, flipping them from mapped to
unmapped.

BUG-036-001 (April 2026) prefixed the 39 scenarios that were G068-unmapped at
that time with `Scenario SCN-036-NNN (...)` trace IDs. The 12 scenarios in this
bug were among the ~50 that passed via fuzzy matching back then, so they never
received a trace-ID prefix — and regressed when the tokenizer changed.
(Scope-count context: the active-scope analysis now sees 56 scenarios because
Iter 11 parked Scopes 09–15 under `## Parked Scope NN:` headings, so the 31
prior Blocked-scope residuals are no longer in the active set.)

## Fix Strategy

Make the 12 scenarios **trace-ID-matched** (immune to fuzzy-scoring drift) by
adding `Scenario SCN-036-NNN (<title>):` to the single faithful DoD bullet that
already preserves each scenario's behavioral claim. The DoD claim text is
preserved verbatim after the prefix. This is the exact pattern established by
BUG-036-001 / BUG-031-002 / BUG-034-001.

| Scenario | Faithful covering DoD bullet (scope) |
|----------|--------------------------------------|
| SCN-036-003 | `config.go` `MealPlanConfig` fail-loud validation (01) |
| SCN-036-005 | `smackerel.yaml` `meal_planning` `meal_times` (01) |
| SCN-036-017 | Batch slot creation, one-per-day `batch_flag` (02) |
| SCN-036-030 | Daily "what's for dinner?" query (04) |
| SCN-036-037 | Plan repeat "repeat last week's plan" (04) |
| SCN-036-041 | Missing `domain_data` recipes skipped (05) |
| SCN-036-044 | `CopyPlan` slot-date shift by `dayOffset` (06) |
| SCN-036-048 | CalDAV bridge maps slots to VEVENTs (07) |
| SCN-036-050 | CalDAV-disabled returns 422 (07) |
| SCN-036-053 | `AutoCompletePastPlans` transitions past plans (08) |
| SCN-036-054 | Future-dated active plans not affected (08) |
| SCN-036-055 | Job registered only when auto-complete enabled (08) |

Secondary: reconcile the 8 stale `_Not started._` per-scope stubs in
`report.md` to point at the consolidated Completion Statement / Test Evidence
(spec-036-unique drift contradicting the certified-Done status).

## Non-goals

No production code, no sibling specs, no spec-036 status mutation, no
framework-script edits, no force-closing Parked Scopes 09–15.
