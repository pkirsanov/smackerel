# Scopes: BUG-034-001 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 034 and create scenario-manifest.json

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-034-FIX-001 Trace guard accepts all 100 SCN-034-* scenarios as faithfully covered
  Given specs/034-expense-tracking/scopes.md contains a coverage Test Plan row at the top of every scope's `### Test Plan` table that lists every scope-owned SCN-034-NNN ID and points to an existing test file
  And specs/034-expense-tracking/scopes.md contains a coverage DoD bullet at the end of every scope's `### Definition of Done` section that lists every scope-owned SCN-034-NNN ID
  And specs/034-expense-tracking/scenario-manifest.json contains 100 scenarioId entries, all linkedTests[*].file paths exist in the repo, and every entry has a non-empty evidenceRefs array
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/034-expense-tracking`
  Then Gate G068 reports "100 scenarios checked, 100 mapped to DoD, 0 unmapped"
  And every Test Plan row mapping (100) is reported as ✅
  And every concrete test file reference (100) is reported as ✅
  And every report evidence reference (100) is reported as ✅
  And the overall result is PASSED with 0 warnings
  And `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking` PASSES
  And no files outside `specs/034-expense-tracking/scopes.md`, `specs/034-expense-tracking/scenario-manifest.json`, and `specs/034-expense-tracking/bugs/BUG-034-001-dod-scenario-fidelity-gap/` are modified
```

### Implementation Plan

1. For each of the 17 scopes in `specs/034-expense-tracking/scopes.md`, insert ONE coverage Test Plan row right after the table separator. The row contains the existing test file binding from the design.md table and every scope-owned `SCN-034-NNN` ID.
2. For each of the 17 scopes, append ONE coverage DoD bullet at the end of `### Definition of Done` listing every scope-owned `SCN-034-NNN` ID.
3. Create `specs/034-expense-tracking/scenario-manifest.json` as a v2 manifest with 100 scenario contracts; every `linkedTests[*].file` path must exist in the repo; every entry must have a non-empty `evidenceRefs` array.
4. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking` and the bug folder; run `bash .github/bubbles/scripts/traceability-guard.sh specs/034-expense-tracking` and confirm PASS.
5. Verify boundary via `git diff --name-only`: only `specs/034-expense-tracking/scopes.md`, `specs/034-expense-tracking/scenario-manifest.json`, and the new bug folder appear.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 100 mapped, 0 unmapped` for SCN-034-FIX-001 | SCN-034-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/034-expense-tracking` for SCN-034-FIX-001 | SCN-034-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/034-expense-tracking/bugs/BUG-034-001-dod-scenario-fidelity-gap` for SCN-034-FIX-001 | SCN-034-FIX-001 |
| T-FIX-1-04 | Boundary preserved | artifact | `git diff --name-only` | only `specs/034-expense-tracking/scopes.md`, `specs/034-expense-tracking/scenario-manifest.json`, and the new bug folder paths appear; no `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, `web/`, or other-spec paths for SCN-034-FIX-001 | SCN-034-FIX-001 |

### Definition of Done

- [x] Scenario SCN-034-FIX-001: Parent `scopes.md` contains a coverage Test Plan row at the top of every one of the 17 scopes' Test Plan tables — **Phase:** implement
  > Evidence: `grep -nE '\| T-(0[1-9]|1[0-7])-COV \|' specs/034-expense-tracking/scopes.md` returns 17 matches.
- [x] Scenario SCN-034-FIX-001: Parent `scopes.md` contains a coverage DoD bullet at the end of every one of the 17 scopes' DoD sections — **Phase:** implement
  > Evidence: `grep -cE 'Scenario coverage for SCN-034-' specs/034-expense-tracking/scopes.md` returns 17.
- [x] Scenario SCN-034-FIX-001: `specs/034-expense-tracking/scenario-manifest.json` exists with 100 scenarioId entries — **Phase:** implement
  > Evidence: `grep -cE '"scenarioId"\s*:' specs/034-expense-tracking/scenario-manifest.json` returns 100.
- [x] Scenario SCN-034-FIX-001: Traceability-guard PASSES against `specs/034-expense-tracking` — **Phase:** validate
  > Evidence:
  > ```
  > $ bash .github/bubbles/scripts/traceability-guard.sh specs/034-expense-tracking 2>&1 | tail -12
  > ℹ️  DoD fidelity: 100 scenarios checked, 100 mapped to DoD, 0 unmapped
  >
  > --- Traceability Summary ---
  > ℹ️  Scenarios checked: 100
  > ℹ️  Test rows checked: 200
  > ℹ️  Scenario-to-row mappings: 100
  > ℹ️  Concrete test file references: 100
  > ℹ️  Report evidence references: 100
  > ℹ️  DoD fidelity scenarios: 100 (mapped: 100, unmapped: 0)
  >
  > RESULT: PASSED (0 warnings)
  > EXIT=0
  > ```
- [x] Scenario SCN-034-FIX-001: Artifact-lint PASSES against parent and bug folder — **Phase:** audit
  > Evidence: see report.md `### Audit Evidence` for both runs (parent EXIT_CODE=0, bug folder EXIT_CODE=0).
- [x] Scenario SCN-034-FIX-001: No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/034-expense-tracking/scopes.md`, `specs/034-expense-tracking/scenario-manifest.json`, and `specs/034-expense-tracking/bugs/BUG-034-001-dod-scenario-fidelity-gap/*`. Zero changes under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, `web/`, or any other parent spec.
