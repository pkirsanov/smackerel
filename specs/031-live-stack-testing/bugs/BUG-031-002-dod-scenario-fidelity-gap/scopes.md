# Scopes: BUG-031-002 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 031

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-LST-FIX-002 Trace guard accepts SCN-LST-001/002/003/004 as faithfully covered
  Given specs/031-live-stack-testing/scopes.md Gherkin scenario titles in Scopes 2, 5, 6 prefixed with their SCN-LST-NNN IDs
  And specs/031-live-stack-testing/scopes.md DoD entries for those scopes prefixed with "Scenario SCN-LST-NNN"
  And specs/031-live-stack-testing/scenario-manifest.json already mapping all 12 SCN-LST-* scenarios
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing`
  Then Gate G068 reports "12 scenarios checked, 12 mapped to DoD, 0 unmapped"
  And every Test Plan row mapping (12) is reported as ✅
  And the overall result is PASSED
```

### Implementation Plan

1. Edit `specs/031-live-stack-testing/scopes.md` Scope 2 Gherkin block: prefix `All migrations apply cleanly` with `SCN-LST-001 ` and `Schema DDL resilience` with `SCN-LST-002 `.
2. Edit `specs/031-live-stack-testing/scopes.md` Scope 5 Gherkin block: prefix `Full pipeline flow` with `SCN-LST-003 `.
3. Edit `specs/031-live-stack-testing/scopes.md` Scope 6 Gherkin block: prefix `Search works after cold start` with `SCN-LST-004 `.
4. Edit `specs/031-live-stack-testing/scopes.md` Scope 2 DoD: prefix the "All consolidated migrations verified..." bullet with `Scenario SCN-LST-001 (All migrations apply cleanly):` and the "Schema DDL resilience tested..." bullet with `Scenario SCN-LST-002 (Schema DDL resilience):`.
5. Edit `specs/031-live-stack-testing/scopes.md` Scope 5 DoD: prefix the "Text capture → processing verified end-to-end..." bullet with `Scenario SCN-LST-003 (Full pipeline flow):`.
6. Edit `specs/031-live-stack-testing/scopes.md` Scope 6 DoD: prefix the "`WaitForMLReady` implemented in..." bullet with `Scenario SCN-LST-004 (Search works after cold start):`.
7. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing` and the bug folder; run `bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing` and confirm PASS.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 12 mapped, 0 unmapped` for SCN-LST-FIX-002 | SCN-LST-FIX-002 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/031-live-stack-testing` for SCN-LST-FIX-002 | SCN-LST-FIX-002 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/031-live-stack-testing/bugs/BUG-031-002-dod-scenario-fidelity-gap` for SCN-LST-FIX-002 | SCN-LST-FIX-002 |

### Definition of Done

- [x] Scenario SCN-LST-FIX-002: Parent `scopes.md` Scope 2 Gherkin prefixes `SCN-LST-001` / `SCN-LST-002` on the scenario titles — **Phase:** implement
  > Evidence: `grep -nE 'Scenario: SCN-LST-001|Scenario: SCN-LST-002' specs/031-live-stack-testing/scopes.md` returns matches in the Scope 02 Gherkin block.
- [x] Scenario SCN-LST-FIX-002: Parent `scopes.md` Scope 5 Gherkin prefixes `SCN-LST-003` on the scenario title — **Phase:** implement
  > Evidence: `grep -n 'Scenario: SCN-LST-003' specs/031-live-stack-testing/scopes.md` returns one match in the Scope 05 Gherkin block.
- [x] Scenario SCN-LST-FIX-002: Parent `scopes.md` Scope 6 Gherkin prefixes `SCN-LST-004` on the scenario title — **Phase:** implement
  > Evidence: `grep -n 'Scenario: SCN-LST-004' specs/031-live-stack-testing/scopes.md` returns one match in the Scope 06 Gherkin block.
- [x] Scenario SCN-LST-FIX-002: Parent `scopes.md` DoD entries cite `Scenario SCN-LST-001`, `Scenario SCN-LST-002`, `Scenario SCN-LST-003`, `Scenario SCN-LST-004` — **Phase:** implement
  > Evidence: `grep -nE 'Scenario SCN-LST-(001|002|003|004)' specs/031-live-stack-testing/scopes.md` returns four matches inside the Scope 02/05/06 DoD sections.
- [x] Scenario SCN-LST-FIX-002: Traceability-guard PASSES against `specs/031-live-stack-testing` — **Phase:** validate
  > Evidence:
  > ```
  > $ bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing 2>&1 | tail -10
  > ℹ️  DoD fidelity: 12 scenarios checked, 12 mapped to DoD, 0 unmapped
  > 
  > --- Traceability Summary ---
  > ℹ️  Scenarios checked: 12
  > ℹ️  Test rows checked: 29
  > ℹ️  Scenario-to-row mappings: 12
  > ℹ️  Concrete test file references: 12
  > ℹ️  Report evidence references: 12
  > ℹ️  DoD fidelity scenarios: 12 (mapped: 12, unmapped: 0)
  > 
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Scenario SCN-LST-FIX-002: Artifact-lint PASSES against parent and bug folder — **Phase:** audit
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] Scenario SCN-LST-FIX-002: No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/031-live-stack-testing/scopes.md` and `specs/031-live-stack-testing/bugs/BUG-031-002-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or any other parent spec artifact are touched.
