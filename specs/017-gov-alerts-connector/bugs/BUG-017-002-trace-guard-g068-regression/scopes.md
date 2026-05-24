# Scopes: BUG-017-002 — Trace-guard G068 regression on SCN-GA-NWS-002

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore G068 fidelity for SCN-GA-NWS-002 under framework v3.8.0

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GA-FIX-NWS-002 Trace guard accepts SCN-GA-NWS-002 as faithfully covered under G068 v3.8.0
  Given specs/017-gov-alerts-connector/scopes.md Scope 03 contains a DoD bullet
        explicitly prefixed with `Scenario "SCN-GA-NWS-002 NWS severity and event classification":`
  And the bullet text contains the words `NWS`, `severity`, `event`, and `classification`
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/017-gov-alerts-connector`
  Then Gate G068 reports `13 scenarios checked, 13 mapped to DoD, 0 unmapped`
  And the overall result is `RESULT: PASSED (0 warnings)`
```

### Implementation Plan

1. Insert a single new scenario-prefix DoD bullet for `SCN-GA-NWS-002` into `specs/017-gov-alerts-connector/scopes.md` Scope 03, between the existing `Event types classified …` bullet and the `17 unit tests pass …` bullet. Preserve every other Scope 03 bullet byte-identical.
2. Re-run `bash .github/bubbles/scripts/traceability-guard.sh specs/017-gov-alerts-connector`; confirm `RESULT: PASSED (0 warnings)` and `13/13 mapped`.
3. Re-run `bash .github/bubbles/scripts/artifact-lint.sh specs/017-gov-alerts-connector`; confirm PASS.
4. Re-run `go test ./internal/connector/alerts/ -count=1`; confirm 175 tests PASS (no regression from R2 simplify or this docs-only edit).
5. Append a one-line cross-reference to BUG-017-002 in `specs/017-gov-alerts-connector/report.md` under the existing BUG-017-001 cross-reference section.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-NWS002-1-01 | traceability-guard.sh PASS post-fix | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 13 mapped, 0 unmapped` | SCN-GA-FIX-NWS-002 |
| T-FIX-NWS002-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/017-gov-alerts-connector` | SCN-GA-FIX-NWS-002 |
| T-FIX-NWS002-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/017-gov-alerts-connector/bugs/BUG-017-002-trace-guard-g068-regression` | SCN-GA-FIX-NWS-002 |
| T-FIX-NWS002-1-04 | Underlying NWS behavior tests still pass | unit | `internal/connector/alerts/alerts_test.go` | `go test -count=1 ./internal/connector/alerts/` exit 0; TestMapNWSSeverity + TestClassifyNWSEventType PASS | SCN-GA-FIX-NWS-002 |

### Definition of Done

- [x] Parent `scopes.md` Scope 03 contains a DoD bullet prefixed `Scenario "SCN-GA-NWS-002 NWS severity and event classification":` — **Phase:** implement
  > Evidence: `grep -c 'Scenario "SCN-GA-NWS-002' specs/017-gov-alerts-connector/scopes.md` returns `1`.
- [x] Existing Scope 03 DoD bullets are preserved unchanged — **Phase:** implement
  > Evidence: `git diff specs/017-gov-alerts-connector/scopes.md` shows additions only (one new bullet + its evidence line); no deletions, no in-place edits to the `Severity mapped …`, `Event types classified …`, or `17 unit tests pass …` bullets.
- [x] Scenario "SCN-GA-FIX-NWS-002 Trace guard accepts SCN-GA-NWS-002 as faithfully covered under G068 v3.8.0": traceability-guard PASSES with `13/13 mapped` — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence` for full guard output. Final lines: `ℹ️  DoD fidelity: 13 scenarios checked, 13 mapped to DoD, 0 unmapped` and `RESULT: PASSED (0 warnings)`.
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] Underlying NWS behavior tests still pass — **Phase:** test
  > Evidence: see report.md `### Test Evidence` for raw `go test` output covering the alerts package (175 tests PASS) and the race-detector subset (clean).
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix, pre-commit) shows changes confined to `specs/017-gov-alerts-connector/scopes.md`, `specs/017-gov-alerts-connector/report.md`, `specs/017-gov-alerts-connector/state.json`, and `specs/017-gov-alerts-connector/bugs/BUG-017-002-trace-guard-g068-regression/*`. No files under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` are touched.
