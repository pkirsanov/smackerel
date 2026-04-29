# Design: BUG-031-002 — DoD scenario fidelity gap

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [031 spec](../../spec.md) | [031 scopes](../../scopes.md) | [031 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Root Cause

`scopes.md` for spec 031 was authored before Gate G068 (Gherkin → DoD Content Fidelity) and the upgraded Test-Plan-row trace check became routinely run on this feature. The Gherkin scenarios in Scopes 2, 5, and 6 used short titles ("All migrations apply cleanly", "Schema DDL resilience", "Full pipeline flow", "Search works after cold start") without embedding their `SCN-LST-NNN` IDs. The corresponding Test Plan rows already carried the IDs in their `Scenario ID` column, but the guard's `scenario_matches_row` function tries `extract_trace_ids` on the **scenario** first; if no ID is found there, the trace-ID fast path cannot fire and the matcher falls back to fuzzy word overlap. Because the scenario titles share only one ≥4-character non-stop-word with the relevant Test Plan row Assertion text (or DoD bullet text), the fuzzy threshold (≥2 for rows, ≥3 for DoD) was not met for these four scenarios.

The same logic governs `scenario_matches_dod`. The pre-existing DoD bullets accurately described the delivered behavior (migrations, DDL resilience, capture→process→search pipeline, ML readiness gate) but did not embed the trace IDs and did not happen to share enough significant words with the scenario titles, so 3 of the 4 scenarios were also flagged as G068-unmapped.

## Fix Approach (artifact-only)

This is an **artifact-only** fix. No production code is modified. The user's boundary clause — "only specs/031-live-stack-testing/scopes.md and the new bug folder. No code, no other parent artifacts." — is honored.

The fix has two parts:

1. **Embed `SCN-LST-NNN` in the four Gherkin scenario titles** in `specs/031-live-stack-testing/scopes.md`:
   - Scope 2: `Scenario: All migrations apply cleanly` → `Scenario: SCN-LST-001 All migrations apply cleanly`
   - Scope 2: `Scenario: Schema DDL resilience` → `Scenario: SCN-LST-002 Schema DDL resilience`
   - Scope 5: `Scenario: Full pipeline flow` → `Scenario: SCN-LST-003 Full pipeline flow`
   - Scope 6: `Scenario: Search works after cold start` → `Scenario: SCN-LST-004 Search works after cold start`
   This makes `extract_trace_ids` find each scenario's ID, so `scenario_matches_row` matches against the Test Plan rows that already carry `SCN-LST-NNN` in the `Scenario ID` column.

2. **Prefix one existing DoD bullet per scenario with `Scenario SCN-LST-NNN (<title>):`** so `scenario_matches_dod` finds the same trace ID in both the scenario and the DoD item. The DoD claims themselves are unchanged — no behavior, no test reference, and no evidence pointer is removed or rewritten. The four affected DoD bullets are:
   - Scope 2 bullet "All consolidated migrations verified..." → prefixed with `Scenario SCN-LST-001 (All migrations apply cleanly):`
   - Scope 2 bullet "Schema DDL resilience tested..." → prefixed with `Scenario SCN-LST-002 (Schema DDL resilience):`
   - Scope 5 bullet "Text capture → processing verified end-to-end..." → prefixed with `Scenario SCN-LST-003 (Full pipeline flow):`
   - Scope 6 bullet "`WaitForMLReady` implemented in `internal/api/ml_readiness.go`..." → prefixed with `Scenario SCN-LST-004 (Search works after cold start):`

## Why this is not "DoD rewriting"

Gate G068's stated failure mode is "DoD may have been rewritten to match delivery instead of the spec." The bullets edited by this fix preserve the original DoD claims verbatim — only the trace-ID + scenario-name prefix is added. The Gherkin Given/When/Then bodies are unchanged. The behavior the Gherkin describes is the behavior the production code already implements (verified by the existing passing tests in `tests/integration/db_migration_test.go`, `tests/integration/ml_readiness_test.go`, `tests/e2e/capture_process_search_test.go`). No DoD bullet was deleted or weakened, and no Gherkin scenario was edited beyond a trace-ID prefix on the title line.

## Regression Test

Because this fix is artifact-only, the regression "test" is the traceability guard itself.

- Pre-fix: `RESULT: FAILED (7 failures, 0 warnings)`; `DoD fidelity: 12 scenarios checked, 9 mapped to DoD, 3 unmapped`.
- Post-fix: `RESULT: PASSED (0 warnings)`; `DoD fidelity: 12 scenarios checked, 12 mapped to DoD, 0 unmapped`; `Scenario-to-row mappings: 12`.

The guard run is captured in `report.md` under "Validation Evidence".
