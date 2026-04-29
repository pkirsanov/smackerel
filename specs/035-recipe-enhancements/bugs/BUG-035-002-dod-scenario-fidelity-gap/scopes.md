# Scopes: BUG-035-002 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity, create scenario-manifest, fix T-14-01 placeholder

**Status:** Done (within boundary)
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-035-FIX-002 Trace guard accepts all 88 SCN-035-NNN scenarios as faithfully covered and the manifest cross-check passes
  Given specs/035-recipe-enhancements/scopes.md has 88 Gherkin scenarios already prefixed with SCN-035-NNN trace IDs
  And specs/035-recipe-enhancements/scopes.md DoD blocks for Scopes 01, 02, 04, 07, 09, 10, 11, 12, 13, 14, 15 carry one new "Scenario SCN-035-NNN (<title>):" bullet per previously unmapped scenario
  And specs/035-recipe-enhancements/scenario-manifest.json exists with all 88 scenarioId entries, evidenceRefs present, and every "file" linked test resolves on disk
  And specs/035-recipe-enhancements/scopes.md Scope 14 Test Plan row T-14-01 carries the concrete path internal/mealplan/shopping_test.go in place of the placeholder "(spec 036 test file)"
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/035-recipe-enhancements`
  Then Gate G068 reports "88 scenarios checked, 88 mapped to DoD, 0 unmapped"
  And the scenario-manifest cross-check reports the four pass lines (covers ≥88, all linked tests exist, evidenceRefs recorded, per-file linked-test exists)
  And the within-boundary failure count is reduced from 65 to 42 (24 within-boundary failures resolved)
  And the only remaining failures are out-of-boundary (36 missing Phase B test files + 6 missing report.md evidence references)
```

### Implementation Plan

1. Edit `specs/035-recipe-enhancements/scopes.md` Scope 01 DoD: add `Scenario SCN-035-003 (Unparseable quantities return zero value):` bullet pointing at `internal/recipe/quantity_test.go`.
2. Edit `specs/035-recipe-enhancements/scopes.md` Scope 02 DoD: add bullets for `Scenario SCN-035-011` and `Scenario SCN-035-014` pointing at `internal/recipe/scaler_test.go`.
3. Edit `specs/035-recipe-enhancements/scopes.md` Scope 04 DoD: add `Scenario SCN-035-027` bullet pointing at `internal/telegram/cook_session_test.go`.
4. Edit `specs/035-recipe-enhancements/scopes.md` Scope 07 DoD: add bullets for `Scenario SCN-035-051` and `Scenario SCN-035-052` (planned).
5. Edit `specs/035-recipe-enhancements/scopes.md` Scope 09 DoD: add bullets for `Scenario SCN-035-059` and `Scenario SCN-035-060` (planned).
6. Edit `specs/035-recipe-enhancements/scopes.md` Scope 10 DoD: add `Scenario SCN-035-063` bullet (planned).
7. Edit `specs/035-recipe-enhancements/scopes.md` Scope 11 DoD: add bullets for `Scenario SCN-035-066/067/068/069/070` (planned).
8. Edit `specs/035-recipe-enhancements/scopes.md` Scope 12 DoD: add bullets for `Scenario SCN-035-073/074/075` (planned).
9. Edit `specs/035-recipe-enhancements/scopes.md` Scope 13 DoD: add bullets for `Scenario SCN-035-077/078` (planned).
10. Edit `specs/035-recipe-enhancements/scopes.md` Scope 14 DoD: add `Scenario SCN-035-079` bullet (planned).
11. Edit `specs/035-recipe-enhancements/scopes.md` Scope 15 DoD: add `Scenario SCN-035-083` bullet (planned).
12. Edit `specs/035-recipe-enhancements/scopes.md` Scope 14 Test Plan row T-14-01: replace `(spec 036 test file)` with `internal/mealplan/shopping_test.go (spec 036 owned consumer)`.
13. Create `specs/035-recipe-enhancements/scenario-manifest.json` with all 88 scenarios; Phase A delivered scenarios link to existing test files; Phase B planned scenarios use empty `linkedTests`; every scenario carries `evidenceRefs`.
14. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/035-recipe-enhancements/bugs/BUG-035-002-dod-scenario-fidelity-gap` and confirm PASS.
15. Run `bash .github/bubbles/scripts/traceability-guard.sh specs/035-recipe-enhancements` and confirm DoD fidelity is 88/88 and within-boundary failures (G068, manifest, T-14-01) are eliminated.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh DoD fidelity 88/88 | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `DoD fidelity: 88 scenarios checked, 88 mapped to DoD, 0 unmapped` for SCN-035-FIX-002 | SCN-035-FIX-002 |
| T-FIX-1-02 | traceability-guard.sh manifest cross-check passes | artifact | `.github/bubbles/scripts/traceability-guard.sh` | scenario-manifest cross-check emits four ✅ lines (covers 88, all linked tests exist, evidenceRefs recorded, per-file linked-test exists) for SCN-035-FIX-002 | SCN-035-FIX-002 |
| T-FIX-1-03 | T-14-01 path is concrete and resolves | artifact | `specs/035-recipe-enhancements/scopes.md` | `grep -n "T-14-01" specs/035-recipe-enhancements/scopes.md` shows `internal/mealplan/shopping_test.go` and `[ -f internal/mealplan/shopping_test.go ]` exits 0 for SCN-035-FIX-002 | SCN-035-FIX-002 |
| T-FIX-1-04 | artifact-lint.sh PASS (bug folder) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/035-recipe-enhancements/bugs/BUG-035-002-dod-scenario-fidelity-gap` for SCN-035-FIX-002 | SCN-035-FIX-002 |
| T-FIX-1-05 | Boundary preserved | artifact | `git diff --name-only` | no files under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or any sibling spec are touched for SCN-035-FIX-002 | SCN-035-FIX-002 |

### Definition of Done — 3-Part Validation

- [x] Scenario SCN-035-FIX-002: 21 trace-ID-bearing DoD bullets added across Scopes 01, 02, 04, 07, 09, 10, 11, 12, 13, 14, 15 — **Phase:** implement
  > Evidence:
  > ```
  > $ grep -cE '^- \[[ x]\] Scenario SCN-035-' specs/035-recipe-enhancements/scopes.md
  > 21
  > ```
- [x] Scenario SCN-035-FIX-002: `specs/035-recipe-enhancements/scenario-manifest.json` exists with 88 scenarioId entries and evidenceRefs — **Phase:** implement
  > Evidence:
  > ```
  > $ grep -cE '"scenarioId"' specs/035-recipe-enhancements/scenario-manifest.json
  > 88
  > $ grep -cE '"evidenceRefs"' specs/035-recipe-enhancements/scenario-manifest.json
  > 88
  > ```
- [x] Scenario SCN-035-FIX-002: Scope 14 T-14-01 carries concrete existing path `internal/mealplan/shopping_test.go` — **Phase:** implement
  > Evidence:
  > ```
  > $ grep -n 'T-14-01' specs/035-recipe-enhancements/scopes.md
  > 1281:| T-14-01 | Unit | `internal/mealplan/shopping_test.go` (spec 036 owned consumer) | SCN-035-079 | Aggregator invokes `ingredient_categorize-v1`; no call to `CategorizeIngredient` (use a build-time stub asserting zero invocations) |
  > $ ls internal/mealplan/shopping_test.go
  > internal/mealplan/shopping_test.go
  > ```
- [x] Scenario SCN-035-FIX-002: Traceability-guard reports 88/88 DoD fidelity and manifest cross-check passes against `specs/035-recipe-enhancements` — **Phase:** validate
  > Evidence: see [report.md → Validation Evidence](report.md#validation-evidence)
- [x] Scenario SCN-035-FIX-002: Artifact-lint PASSES against bug folder — **Phase:** audit
  > Evidence: see [report.md → Audit Evidence](report.md#audit-evidence)
- [x] Scenario SCN-035-FIX-002: No production code changed (boundary preserved — only `scopes.md`, `scenario-manifest.json`, and the new bug folder) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/035-recipe-enhancements/scopes.md`, `specs/035-recipe-enhancements/scenario-manifest.json`, and `specs/035-recipe-enhancements/bugs/BUG-035-002-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or any other parent spec artifact are touched. See [report.md → Audit Evidence](report.md#audit-evidence).
- [x] Scenario SCN-035-FIX-002: Traceability-guard residual is fully classified and within authorized boundary — **Phase:** validate
  > **Met (with documented residual).** With the user-authorized boundary expansion to permit appending a `Traceability Evidence References (BUG-035-002)` appendix to `specs/035-recipe-enhancements/report.md`, the post-fix guard run reports `RESULT: FAILED (36 failures, 0 warnings)`. All 36 residual failures are of category `mapped row references no existing concrete test file` and correspond to Phase B Not Started production test files (35 rows) plus Scope 01 SCN-035-006 indirect coverage (1 row). They are classified `deferred-blocked-on-Phase-B-implementation` and tracked in [report.md → Failure Decomposition (Post-Appendix)](report.md#failure-decomposition-post-appendix). No within-boundary failure remains: G068 fidelity is 88/88, scenario-manifest cross-check passes, T-14-01 path resolves, and all six Phase A `report is missing evidence reference` failures are resolved by the appendix. Pre→Post: 64 → 36 (28 within-boundary failures resolved).
