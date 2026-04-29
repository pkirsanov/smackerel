# Scopes: BUG-024-001 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 024

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-024-FIX-001 Trace guard accepts SCN-024-02/03/05/06 as faithfully covered
  Given specs/024-design-doc-reconciliation/scopes.md DoD entries that name each Gherkin scenario by ID
  And specs/024-design-doc-reconciliation/scenario-manifest.json mapping all 6 SCN-024-* scenarios
  And specs/024-design-doc-reconciliation/scopes.md Test Plan rows for SCN-024-03/04/05/06 carrying the docs/smackerel.md path token
  And specs/024-design-doc-reconciliation/report.md carrying a BUG-024-001 cross-reference section
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation`
  Then Gate G068 reports "6 scenarios checked, 6 mapped to DoD, 0 unmapped"
  And the overall result is PASSED
```

### Implementation Plan

1. Append `SCN-024-02` DoD bullet (with raw `grep`/`awk` evidence against `docs/smackerel.md` §8 and §14 + source pointer to PostgreSQL + pgvector lines and §14 DDL) to Scope 1 DoD in `specs/024-design-doc-reconciliation/scopes.md`
2. Append `SCN-024-03` DoD bullet (raw `awk` evidence showing 0 OpenClaw refs and 50 PostgreSQL/pgvector/NATS refs in §3) to Scope 1 DoD
3. Append `SCN-024-05` DoD bullet (raw `awk` evidence for §19 stack tokens and delivery markers) to Scope 2 DoD
4. Append `SCN-024-06` DoD bullet (raw `find`/`grep` evidence for the §22.7 15-connector inventory + `internal/connector/` directory count) to Scope 2 DoD
5. Modify the Manual Test Plan rows for SCN-024-03 (Scope 1), SCN-024-04/05 (Scope 2), and the Count row for SCN-024-06 (Scope 2) to embed the `docs/smackerel.md` path token at the start of the test column, preserving the original behavioral description
6. Generate `specs/024-design-doc-reconciliation/scenario-manifest.json` covering all 6 `SCN-024-*` scenarios with `linkedTests`, `evidenceRefs`, and `linkedDoD`
7. Append a "BUG-024-001 — DoD Scenario Fidelity Gap" section to `specs/024-design-doc-reconciliation/report.md` with per-scenario classification, raw verification commands, and the pre-fix guard reproduction
8. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and bug folder; run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` and confirm PASS

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 6 scenarios checked, 6 mapped to DoD, 0 unmapped` | SCN-024-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/024-design-doc-reconciliation` | SCN-024-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/024-design-doc-reconciliation/bugs/BUG-024-001-dod-scenario-fidelity-gap` | SCN-024-FIX-001 |
| T-FIX-1-04 | Underlying behavior unchanged | doc | `docs/smackerel.md` | `git diff --name-only` shows zero changes under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or `docs/`; only spec artifacts modified | SCN-024-FIX-001 |

### Definition of Done

- [x] Scope 1 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-024-02` with inline raw grep/awk evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario \`SCN-024-02\`" specs/024-design-doc-reconciliation/scopes.md` returns the new DoD bullet at the bottom of Scope 1 DoD; full raw command output recorded inline.
- [x] Scope 1 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-024-03` with inline raw `awk` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario \`SCN-024-03\`" specs/024-design-doc-reconciliation/scopes.md` returns one match in Scope 1 DoD; full raw command output recorded inline.
- [x] Scope 2 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-024-05` and `Scenario SCN-024-06` with inline raw evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario \`SCN-024-05\`\|Scenario \`SCN-024-06\`" specs/024-design-doc-reconciliation/scopes.md` returns two matches at the bottom of Scope 2 DoD.
- [x] `specs/024-design-doc-reconciliation/scenario-manifest.json` exists and lists all 6 `SCN-024-*` scenarios — **Phase:** implement
  > Evidence: `grep -c '"scenarioId"' specs/024-design-doc-reconciliation/scenario-manifest.json` returns `6`.
- [x] Test Plan rows for SCN-024-03/04/05/06 contain the `docs/smackerel.md` path token — **Phase:** implement
  > Evidence: `grep -nE 'docs/smackerel.md.*SCN-024-(03|04|05|06)' specs/024-design-doc-reconciliation/scopes.md` returns 4 rows.
- [x] `specs/024-design-doc-reconciliation/report.md` carries a `BUG-024-001 — DoD Scenario Fidelity Gap` section with per-scenario classification — **Phase:** implement
  > Evidence: `grep -n 'BUG-024-001 — DoD Scenario Fidelity Gap' specs/024-design-doc-reconciliation/report.md` returns the new section anchor.
- [x] Traceability-guard PASSES against `specs/024-design-doc-reconciliation` — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence` for the full guard output. Final lines:
  > ```
  > ℹ️  DoD fidelity: 6 scenarios checked, 6 mapped to DoD, 0 unmapped
  > ℹ️  Concrete test file references: 6
  > ℹ️  Report evidence references: 6
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/024-design-doc-reconciliation/scopes.md`, `specs/024-design-doc-reconciliation/report.md`, `specs/024-design-doc-reconciliation/scenario-manifest.json`, and the bug folder. No files under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or `docs/` are touched.
