# Bug: BUG-035-002 — DoD scenario fidelity gap (G068 + scenario-manifest creation)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on a feature in progress; no runtime impact)
- **Parent Spec:** 035 — Recipe Enhancements
- **Parent Spec Status:** in_progress (Phase A delivered; Phase B Not Started)
- **Workflow Mode:** bugfix-fastlane
- **Status:** In Progress (within-boundary work complete; remaining failures structurally out-of-boundary)

## Problem Statement

`bash .github/bubbles/scripts/traceability-guard.sh specs/035-recipe-enhancements` reported `RESULT: FAILED (65 failures, 0 warnings)`. The failures break down into the following categories:

| Category | Count | Within boundary? |
|---|---|---|
| Gate G068 — Gherkin scenario has no faithful matching DoD item | 21 | YES — prefix DoD bullets in `scopes.md` with `Scenario SCN-035-NNN (<title>):` |
| Gate G068 — content-fidelity rollup line | 1 | YES (auto-resolved when the 21 above are mapped) |
| Gate G057/G059 — `scenario-manifest.json` is missing | 1 | YES — boundary explicitly extended to allow `scenario-manifest.json` creation |
| Test Plan row — Scope 14 row T-14-01 has no concrete file path (`(spec 036 test file)`) | 1 | YES — replace placeholder with concrete spec-036-owned consumer path |
| Test Plan row — mapped row references no existing concrete test file (Phase B Not Started + Scope 01 SCN-035-006) | 36 | NO — requires creating production test files (boundary forbids `internal/`, `cmd/`, `tests/` work) |
| Test Plan row — `report.md` is missing evidence reference for an existing concrete test file | 6 | NO — requires editing parent `report.md` (boundary forbids parent-artifact edits other than `scopes.md` + `scenario-manifest.json`) |
| **Total** | **65** | **24 fixable, 42 out-of-boundary** |

The 21 G068 unmapped Gherkin scenarios are listed in [`design.md`](design.md). Each Gherkin behavior is **delivered or planned** by the parent feature; the only gap is that the corresponding DoD bullets in `scopes.md` did not embed the `SCN-035-NNN` trace ID and the fuzzy fallback's significant-word overlap (≥3) was not satisfied. The fix adds one DoD bullet per unmapped scenario carrying `Scenario SCN-035-NNN (<full Gherkin title>):` so the guard's `scenario_matches_dod` trace-ID branch fires.

The missing `scenario-manifest.json` is now created with all 88 scenarios (Phase A `delivered` with linked existing test files; Phase B `planned` with empty `linkedTests` so the guard's "linked test exists" check has nothing to fail). Every `file` entry in the manifest resolves to an existing repo path.

## Reproduction (Pre-fix)

```
$ bash .github/bubbles/scripts/traceability-guard.sh specs/035-recipe-enhancements 2>&1 | tail -10
ℹ️  DoD fidelity: 88 scenarios checked, 67 mapped to DoD, 21 unmapped
❌ DoD content fidelity gap: 21 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)

--- Traceability Summary ---
ℹ️  Scenarios checked: 88
ℹ️  Test rows checked: 131
ℹ️  Scenario-to-row mappings: 88
ℹ️  Concrete test file references: 51
ℹ️  Report evidence references: 46
ℹ️  DoD fidelity scenarios: 88 (mapped: 67, unmapped: 21)

RESULT: FAILED (65 failures, 0 warnings)
```

## Out-of-Boundary Findings (NOT fixed by this bug)

The user constraint for this work is: **"Boundary: ONLY specs/035-recipe-enhancements/scopes.md, scenario-manifest.json, and the new bug folder. NO production code. NO sibling specs."** Combined with **"Treat scope DoD as immutable in semantics; only add trace IDs and path tokens"**, that boundary makes 42 of the 65 failures structurally unfixable inside this bug:

1. **36 × Test Plan rows reference test files that do not exist on disk.** These are: 35 rows for Phase B scopes 07–16 (all marked `Status: Not Started` — the implementation work has not begun) + 1 row for Scope 01 SCN-035-006 (`internal/config/config_test.go` was never authored; coverage is provided indirectly by `internal/config/validate_test.go`). Resolving requires either implementing those tests (boundary forbids `internal/`, `cmd/`, `tests/` work) or rewriting the Test Plan row file paths to existing-but-unrelated files (would mis-document the planned tests and violates "preserve original DoD intent / scope content immutable beyond trace IDs and path tokens").

2. **6 × `specs/035-recipe-enhancements/report.md` is missing evidence references** for these existing concrete test files: `internal/list/recipe_aggregator_test.go` (1), `internal/api/domain_test.go` (3 row mappings), `cmd/scenario-lint/main_test.go` (1), `internal/mealplan/shopping_test.go` (1). Resolving requires editing the parent `report.md`, which the boundary forbids.

These 42 findings are recorded for the user to triage as a follow-up scope expansion decision; this bug does not attempt to resolve them.

## Acceptance Criteria

- [x] All 21 originally-failing G068 scenarios in `specs/035-recipe-enhancements/scopes.md` (Scopes 01, 02, 04, 07, 09, 10, 11, 12, 13, 14, 15) have a DoD bullet prefixed with `Scenario SCN-035-NNN (<full Gherkin title>):`
- [x] `specs/035-recipe-enhancements/scenario-manifest.json` exists and covers all 88 scope-defined scenarios; every `file` entry resolves to an existing repo path; manifest contains `evidenceRefs`
- [x] Scope 14 Test Plan row T-14-01 has a concrete slash-bearing path token (`internal/mealplan/shopping_test.go`) replacing the `(spec 036 test file)` placeholder
- [x] No Gherkin scenario body, Test Plan row Assertion text, or DoD claim text is removed or rewritten — only trace-ID-bearing DoD bullets are added and a single placeholder path is replaced
- [x] No production code changed (boundary preserved — only `specs/035-recipe-enhancements/scopes.md`, `specs/035-recipe-enhancements/scenario-manifest.json`, and the new bug folder)
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/035-recipe-enhancements/bugs/BUG-035-002-dod-scenario-fidelity-gap` PASSES
- [ ] `bash .github/bubbles/scripts/traceability-guard.sh specs/035-recipe-enhancements` returns `RESULT: PASSED` — **NOT MET**: 65 → 42 failures (24 within-boundary fixed: 21 G068 + 1 G068 rollup + 1 manifest + 1 placeholder path), but 42 out-of-boundary failures remain (36 missing test files + 6 missing report.md references). PASSED is unattainable inside the user-specified boundary; reaching it requires either authoring Phase B test files (production code work) or editing `report.md` (parent-artifact edit excluded from the boundary).

## Disposition

**In Progress.** Within-boundary G068 + manifest + placeholder-path work is complete (65 → 42 failures, all DoD fidelity restored, `scenario-manifest.json` created, T-14-01 placeholder resolved). Promotion to `done` is blocked on the user's decision about the 42 out-of-boundary failures (36 missing Phase B test files + 6 missing `report.md` evidence references).
