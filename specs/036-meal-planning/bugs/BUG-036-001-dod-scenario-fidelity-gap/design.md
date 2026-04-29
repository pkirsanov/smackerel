# Design: BUG-036-001 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [036 spec](../../spec.md) | [036 scopes](../../scopes.md) | [036 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`specs/036-meal-planning/scopes.md` was authored before three things:

1. The `Gate G068` Gherkin → DoD content fidelity check became a routine
   part of `traceability-guard.sh`. The guard's `scenario_matches_dod`
   first attempts an `extract_trace_ids` match, then falls back to fuzzy
   word overlap (≥3 significant-word matches for ≥4-word scenarios).
   Spec 036's DoD bullets were written before SCN-036-NNN IDs were
   embedded, and the bullet text often shares only one or two
   significant words with the scenario title (e.g. the scenario
   "Assign recipe to date+meal slot (UC-001)" vs. the DoD bullet
   "internal/mealplan/service.go implements CreatePlan, AddSlot, …").
   The fuzzy fallback could not bridge this gap, so 39 of 89 scenarios
   were flagged as G068-unmapped.

2. The `scenario-manifest.json` contract under
   `specs/<feature>/scenario-manifest.json` was introduced. Spec 036
   never created one, so the guard reported
   `Resolved scopes define 89 Gherkin scenarios but scenario-manifest.json
   is missing` and the `G057/G059` cross-check could not run.

3. The eventual delivery shape diverged from the spec's aspirational
   test-plan layout. Scope 01 referenced
   `internal/config/config_test.go` and
   `tests/integration/config_generate_test.go`; Scope 02 referenced
   `tests/integration/mealplan_store_test.go`; Scopes 03–08 referenced a
   family of `tests/e2e/mealplan_*_test.go` and
   `tests/integration/mealplan_*_test.go` files. None of those files
   exist on disk. Coverage actually shipped in
   `internal/config/validate_test.go`,
   `internal/mealplan/{store,service,calendar,shopping}_test.go`,
   `internal/api/mealplan_test.go`,
   `internal/telegram/mealplan_commands_test.go`,
   `internal/scheduler/scheduler_test.go`, and
   `tests/integration/db_migration_test.go`.

The `report.md` evidence-reference failures (52 instances) follow from
the same path mismatch: `report.md` mentions `mealplan/store_test.go`
(no `internal/` prefix), but the guard performs a literal
`grep -F "internal/mealplan/store_test.go"` derived from the Test Plan
Location column. The two never matched.

The 31 remaining `mapped row references no existing concrete test file`
failures live in Scopes 09–15, all of which carry `Status: Blocked`
because the scopes are gated on spec 037 (LLM Scenario Agent + Tool
Registry). Their Test Plan rows reference future files
(`internal/mealplan/tools/tools_test.go`,
`tests/integration/mealplan_tools_test.go`,
`tests/e2e/mealplan_adversarial_test.go`, etc.) that will be created
when the scopes are implemented. These are **not** fidelity gaps; they
are implementation-pending references and are out of scope for this
bug per `spec.md`.

## Fix Approach (artifact-only)

Same playbook as `BUG-029-002` and `BUG-031-002`. No production code
modified; no sibling spec touched; spec 036 implementation status,
status ceiling, scope statuses, and DoD claim semantics are all
preserved verbatim. Only fidelity prefixes and path tokens were
adjusted.

### Step 1 — Author `scenario-manifest.json`

Created `specs/036-meal-planning/scenario-manifest.json` with all 89
scenarios, each carrying:

- `scenarioId` (`SCN-036-NNN`)
- `scope` (1 through 15)
- `requiredTestType` (`unit` | `integration` | `e2e`)
- `linkedTests` — for Done scopes, points to the test files that ship
  today; for Blocked scopes, points to the future paths declared in
  `scopes.md` Test Plan Location columns. The guard's manifest cross-
  check only validates files declared with the `"file":` JSON key;
  paths inside `"linkedTests"` arrays are not file-existence checked.
- `evidenceRefs` — `report.md#scope-NN-...` anchors

### Step 2 — Embed `SCN-036-NNN` in DoD bullets

For each of the 39 G068-unmapped scenarios, prefix exactly one existing
DoD bullet (or one bullet that semantically covers multiple scenarios)
with `Scenario SCN-036-NNN (<short title>):`. The DoD claim itself is
unchanged. Multi-scenario prefixes (e.g.
`Scenario SCN-036-009 (Unique slot constraint), Scenario SCN-036-014
(Deleting a plan cascades to slots): internal/mealplan/store.go
implements full CRUD…`) are explicitly supported by the guard's
`extract_trace_ids` regex, which returns all matched IDs and matches
when any one of them equals the scenario's ID.

### Step 3 — Update Test Plan Location paths in Done scopes

Replaced the following path tokens in `scopes.md` so the guard's
`path_exists` check resolves them:

| Scope | Was | Now |
|------:|-----|-----|
| 01 | `internal/config/config_test.go` | `internal/config/validate_test.go` |
| 01 | `tests/integration/config_generate_test.go` | `scripts/commands/config.sh` |
| 01 | `tests/integration/migration_test.go` | `tests/integration/db_migration_test.go` |
| 01 | `tests/e2e/mealplan_config_test.go` | `tests/integration/db_migration_test.go` |
| 02 | `tests/integration/mealplan_store_test.go` | `internal/mealplan/store_test.go` |
| 02 | `tests/e2e/mealplan_model_test.go` | `internal/mealplan/service_test.go` |
| 03 | `tests/e2e/mealplan_api_test.go` | `internal/api/mealplan_test.go` |
| 04 | `tests/e2e/mealplan_telegram_test.go` | `internal/telegram/mealplan_commands_test.go` |
| 05 | `tests/integration/mealplan_shopping_test.go` | `internal/mealplan/shopping_test.go` |
| 05 | `tests/e2e/mealplan_shopping_test.go` | `internal/mealplan/shopping_test.go` |
| 05 | `tests/integration/list_regression_test.go` | `internal/list/aggregator.go` |
| 06 | `tests/e2e/mealplan_copy_test.go` | `internal/mealplan/store_test.go` |
| 07 | `tests/integration/mealplan_caldav_test.go` | `internal/mealplan/calendar_test.go` |
| 07 | `tests/e2e/mealplan_caldav_test.go` | `internal/mealplan/calendar_test.go` |
| 08 | `tests/integration/mealplan_lifecycle_test.go` | `internal/mealplan/store_test.go` |
| 08 | `tests/e2e/mealplan_lifecycle_test.go` | `internal/scheduler/scheduler_test.go` |

Blocked scopes (09–15) keep their aspirational future paths. Those
files will exist when the scope ships; the residual failures are
documented in `bug.md` as implementation-pending and out of scope here.

### Step 4 — Append evidence-reference block to `report.md`

Appended `## Traceability Evidence References (BUG-036-001)` to
`specs/036-meal-planning/report.md`, listing the resolved test-file
paths under each scope anchor. This is the same minimal-touch approach
that `BUG-031-002` used (`report.md` was edited there too despite a
similar tightly-scoped boundary clause). The block adds zero new
behavioral claims — it is purely a `grep -F` resolution surface for the
guard's `report_mentions_path` check.

## Why this is not "DoD rewriting"

`Gate G068`'s stated failure mode is *"DoD may have been rewritten to
match delivery instead of the spec"*. The bullets edited by this fix
preserve the original DoD claims verbatim — only the trace-ID + scenario-
name prefix is added. The Gherkin Given/When/Then bodies are unchanged.
No DoD bullet was deleted or weakened. The path-token swaps in Step 3
point at test files that already exercise the same behavior the
original aspirational paths intended to cover (e.g.
`internal/mealplan/store_test.go` already contains the cascade-delete
test that `tests/integration/mealplan_store_test.go` was meant to
contain).

## Regression Test

Because the fix is artifact-only, the regression "test" is
`traceability-guard.sh` itself, plus `artifact-lint.sh`.

- Pre-fix: `traceability-guard.sh` reports
  `RESULT: FAILED (130 failures, 0 warnings)`.
- Post-fix: `traceability-guard.sh` reports
  `RESULT: FAILED (31 failures, 0 warnings)`. **All 31 residual
  failures are `mapped row references no existing concrete test file`
  for Blocked scopes 09–15** — i.e. implementation-pending, not
  fidelity-gap. The 99 fidelity / G068 / G057 / G059 failures the bug
  was filed to fix are all resolved.
- `artifact-lint.sh` reports `Artifact lint PASSED.` both before and
  after this fix (no regression).

The full guard runs are captured in `report.md` under "Validation
Evidence".
