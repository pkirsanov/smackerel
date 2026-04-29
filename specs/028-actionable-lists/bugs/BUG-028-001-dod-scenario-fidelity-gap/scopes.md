# Scopes: BUG-028-001 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 028

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-AL-FIX-001 Trace guard accepts all 34 SCN-AL-* scenarios as faithfully covered
  Given specs/028-actionable-lists/scopes.md DoD entries that name each previously-unmapped Gherkin scenario by SCN-AL-NNN ID
  And specs/028-actionable-lists/scenario-manifest.json mapping all 34 SCN-AL-* scenarios with linkedTests
  And specs/028-actionable-lists/report.md referencing internal/list/types_test.go and internal/intelligence/lists_test.go in a BUG-028-001 cross-reference section
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/028-actionable-lists`
  Then Gate G068 reports "34 scenarios checked, 34 mapped to DoD, 0 unmapped"
  And the overall result is PASSED
```

### Implementation Plan

1. Append trace-ID-bearing DoD bullets to Scope 1 DoD in `specs/028-actionable-lists/scopes.md` for `SCN-AL-002`.
2. Append trace-ID-bearing DoD bullets to Scope 2 DoD for `SCN-AL-003`, `SCN-AL-004`, `SCN-AL-005`, `SCN-AL-006`, `SCN-AL-007`, `SCN-AL-008`.
3. Append trace-ID-bearing DoD bullets to Scope 3 DoD for `SCN-AL-009`, `SCN-AL-010`, `SCN-AL-011`, `SCN-AL-012`, `SCN-AL-013`, `SCN-AL-014`.
4. Append trace-ID-bearing DoD bullets to Scope 4 DoD for `SCN-AL-015`, `SCN-AL-016`, `SCN-AL-017`.
5. Append trace-ID-bearing DoD bullets to Scope 5 DoD for `SCN-AL-018`, `SCN-AL-019`, `SCN-AL-020`, `SCN-AL-021`.
6. Append trace-ID-bearing DoD bullets to Scope 6 DoD for `SCN-AL-022`, `SCN-AL-023`, `SCN-AL-025`, `SCN-AL-026`.
7. Append trace-ID-bearing DoD bullets to Scope 7 DoD for `SCN-AL-028`, `SCN-AL-029`, `SCN-AL-030`, `SCN-AL-031`, `SCN-AL-032`.
8. Append trace-ID-bearing DoD bullets to Scope 8 DoD for `SCN-AL-033`, `SCN-AL-034`.
9. Append a `## BUG-028-001 Cross-Reference` section to `specs/028-actionable-lists/report.md` enumerating every concrete test file with its mapped SCN-AL-NNN ranges (covers the 3 evidence-reference failures for `internal/list/types_test.go` and `internal/intelligence/lists_test.go`).
10. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and the bug folder, plus `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/028-actionable-lists`; confirm both PASS.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 34 mapped, 0 unmapped` | SCN-AL-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/028-actionable-lists` | SCN-AL-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug folder) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/028-actionable-lists/bugs/BUG-028-001-dod-scenario-fidelity-gap` | SCN-AL-FIX-001 |
| T-FIX-1-04 | Boundary preserved | artifact | `git diff --name-only` | No changes under `internal/`, `cmd/`, `ml/`, `config/`, `tests/` | SCN-AL-FIX-001 |

### Definition of Done

- [x] Scope 1 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-AL-002` with inline `**Evidence:**` token — **Phase:** implement
  > Evidence: `grep -c "Scenario SCN-AL-002" specs/028-actionable-lists/scopes.md` returns `1`; the bullet contains `**Evidence:** delivered via internal/list/types.go ...` and `**Claim Source:** interpreted (manifest-anchored)`.
- [x] Scope 2 DoD in parent `scopes.md` contains bullets for `Scenario SCN-AL-003`, `SCN-AL-004`, `SCN-AL-005`, `SCN-AL-006`, `SCN-AL-007`, `SCN-AL-008` with inline `**Evidence:**` tokens — **Phase:** implement
  > Evidence: `grep -cE "Scenario SCN-AL-(003|004|005|006|007|008) "` returns `6` against the new Scope 2 DoD block.
- [x] Scope 3 DoD in parent `scopes.md` contains bullets for `Scenario SCN-AL-009..014` — **Phase:** implement
  > Evidence: `grep -cE "Scenario SCN-AL-(009|010|011|012|013|014) "` returns `6` against the new Scope 3 DoD block.
- [x] Scope 4 DoD in parent `scopes.md` contains bullets for `Scenario SCN-AL-015..017` — **Phase:** implement
  > Evidence: `grep -cE "Scenario SCN-AL-(015|016|017) "` returns `3`.
- [x] Scope 5 DoD in parent `scopes.md` contains bullets for `Scenario SCN-AL-018..021` — **Phase:** implement
  > Evidence: `grep -cE "Scenario SCN-AL-(018|019|020|021) "` returns `4`.
- [x] Scope 6 DoD in parent `scopes.md` contains bullets for `Scenario SCN-AL-022/023/025/026` — **Phase:** implement
  > Evidence: `grep -cE "Scenario SCN-AL-(022|023|025|026) "` returns `4`.
- [x] Scope 7 DoD in parent `scopes.md` contains bullets for `Scenario SCN-AL-028..032` — **Phase:** implement
  > Evidence: `grep -cE "Scenario SCN-AL-(028|029|030|031|032) "` returns `5`.
- [x] Scope 8 DoD in parent `scopes.md` contains bullets for `Scenario SCN-AL-033/034` — **Phase:** implement
  > Evidence: `grep -cE "Scenario SCN-AL-(033|034) "` returns `2`.
- [x] `specs/028-actionable-lists/report.md` references `internal/list/types_test.go` and `internal/intelligence/lists_test.go` by full relative path — **Phase:** implement
  > Evidence: `grep -cE "internal/list/types_test.go|internal/intelligence/lists_test.go" specs/028-actionable-lists/report.md` returns ≥2 matches in the new BUG-028-001 Cross-Reference section.
- [x] Traceability-guard PASSES against `specs/028-actionable-lists` — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence` for the full guard output. Final lines:
  > ```
  > ℹ️  DoD fidelity: 34 scenarios checked, 34 mapped to DoD, 0 unmapped
  > ℹ️  Concrete test file references: 34
  > ℹ️  Report evidence references: 34
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Artifact-lint PASSES against parent — **Phase:** validate
  > Evidence: `bash .github/bubbles/scripts/artifact-lint.sh specs/028-actionable-lists 2>&1 | tail -1` returns `Artifact lint PASSED.`
- [x] Artifact-lint PASSES against bug folder — **Phase:** validate
  > Evidence: `bash .github/bubbles/scripts/artifact-lint.sh specs/028-actionable-lists/bugs/BUG-028-001-dod-scenario-fidelity-gap 2>&1 | tail -1` returns `Artifact lint PASSED.`
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/028-actionable-lists/scopes.md`, `specs/028-actionable-lists/report.md`, and `specs/028-actionable-lists/bugs/BUG-028-001-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` are touched.
