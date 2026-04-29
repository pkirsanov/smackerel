# Bug: BUG-035-001 — DoD scenario fidelity gap (21 G068 unmapped scenarios across spec 035)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure — Gate G068 + scenario-manifest cross-check + Test Plan path/evidence checks; no runtime impact)
- **Parent Spec:** 035 — Recipe Enhancements
- **Workflow Mode:** bugfix-fastlane
- **Status:** In Progress (G068 fix tractable within boundary; remaining failures require boundary expansion — see "Out-of-Boundary Findings")

## Problem Statement

`bash .github/bubbles/scripts/traceability-guard.sh specs/035-recipe-enhancements` returned `RESULT: FAILED (65 failures, 0 warnings)` against spec 035. The failures break down into the following categories:

| Category | Count | Fixable within boundary? |
|---|---|---|
| Gate G068 — Gherkin scenario has no faithful matching DoD item | 21 | YES — prefix existing DoD bullets in `scopes.md` with `Scenario SCN-035-NNN (<title>):` |
| Gate G068 — content-fidelity rollup | 1 | YES (auto-resolved when the 21 above are mapped) |
| Gate G057/G059 — `scenario-manifest.json` is missing | 1 | NO — requires creating a parent artifact (boundary forbids "no other parent artifacts") |
| Test Plan row — report.md is missing evidence reference for an existing concrete test file | 4 | NO — requires editing parent `report.md` |
| Test Plan row — mapped row references no existing concrete test file | 38 | NO — Phase B (Scopes 07–16) aspirational test files for **Not Started** work; would require either creating those test files (forbidden — code change) or remapping Test Plan rows away from the planned-but-not-yet-existing files (changes the original DoD intent that the user explicitly told us to preserve) |
| Test Plan row — Scope 14 row has no concrete test file path (file column is "(spec 036 test file)") | 1 | NO — same Phase B / cross-spec coordination problem as above |
| **Total** | **65** | **22 fixable, 43 out-of-boundary** |

The 21 G068 unmapped Gherkin scenarios are listed in [`design.md`](design.md). Each is **delivered or planned** by the parent feature; the only gap is that the corresponding DoD bullets in `scopes.md` do not embed the `SCN-035-NNN` trace ID and the fuzzy fallback's significant-word overlap (≥3) is not satisfied for these specific pairs. The fix prefixes one existing DoD bullet per unmapped scenario with `Scenario SCN-035-NNN (<full Gherkin title>):` so the guard's `scenario_matches_dod` trace-ID branch fires.

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

## Out-of-Boundary Findings (NOT fixed by this bug — requires user decision)

The user constraint for this work is: **"Boundary: ONLY specs/035-recipe-enhancements/scopes.md and the new bug folder. No code, no other parent artifacts."** That boundary makes 43 of the 65 failures structurally unfixable inside this bug:

1. **`specs/035-recipe-enhancements/scenario-manifest.json` is missing.** Spec 035 declares 88 Gherkin scenarios but has no scenario-manifest. Creating it would require a new parent artifact, which the boundary forbids.
2. **`specs/035-recipe-enhancements/report.md` is missing evidence references** for 4 existing concrete test files (`internal/list/recipe_aggregator_test.go`, `internal/api/domain_test.go` ×3, `cmd/scenario-lint/main_test.go`). Resolving requires editing the parent `report.md`, which the boundary forbids.
3. **38 × Test Plan rows in Phase B scopes (07–16, all marked `Status: Not Started`) reference test files that do not exist yet** because the work hasn't been built. Resolving would require either implementing those tests (boundary forbids code changes) or rewriting the Test Plan row file paths to existing-but-unrelated files (would mis-document the planned tests and violates "preserve original DoD intent").
4. **1 × Scope 14 Test Plan row (T-14-01) has the file column `(spec 036 test file)`** with no concrete path. Same Phase B / cross-spec issue.

These four findings are recorded for the user to triage as a follow-up scope expansion decision; this bug does not attempt to resolve them.

## Acceptance Criteria

- [x] All 21 originally-failing G068 scenarios in `specs/035-recipe-enhancements/scopes.md` (Scopes 01, 02, 04, 07, 09, 10, 11, 12, 13, 14, 15) have at least one DoD bullet prefixed with `Scenario SCN-035-NNN (<full Gherkin title>):`
- [x] No Gherkin scenario body, Test Plan row, or DoD claim text is removed or rewritten — only trace-ID prefixes are added
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/035-recipe-enhancements/bugs/BUG-035-001-dod-scenario-fidelity-gap` PASSES
- [x] No production code changed (boundary preserved — only `specs/035-recipe-enhancements/scopes.md` and the new bug folder)
- [ ] `bash .github/bubbles/scripts/traceability-guard.sh specs/035-recipe-enhancements` returns `RESULT: PASSED` — **NOT MET**: G068 portion goes from 21 unmapped → 0 unmapped, but 43 out-of-boundary failures remain (see "Out-of-Boundary Findings"). Bug status remains `in_progress` until the user decides whether to expand the boundary.

## Disposition

**In Progress.** The G068 fidelity gap is fixed (22/65 failures resolved). Promotion to `done` is blocked on the user's decision about the 43 out-of-boundary failures.
