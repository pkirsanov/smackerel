# Scopes: BUG-017-001 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 017

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GA-FIX-001 Trace guard accepts all 13 SCN-GA-* scenarios as faithfully covered
  Given specs/017-gov-alerts-connector/scopes.md has a Test Plan table on every scope
  And every SCN-GA-* scenario has a DoD bullet that names the SCN-GA-NNN trace ID and the concrete test file
  And specs/017-gov-alerts-connector/report.md references tests/integration/weather_alerts_test.go and tests/e2e/weather_alerts_e2e_test.go by full relative path
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/017-gov-alerts-connector`
  Then Gate G068 reports "13 scenarios checked, 13 mapped to DoD, 0 unmapped"
  And every scope reports "scenario maps to concrete test file"
  And every scope reports "report references concrete test evidence"
  And the overall result is PASSED
```

### Implementation Plan

1. Insert a `### Test Plan` table block (Markdown table with `ID | Test Name | Type | Location | Assertion | Mapped Scenario` columns) into each of the 6 scopes in `specs/017-gov-alerts-connector/scopes.md`, between the Gherkin block and the existing Definition of Done.
2. Each Test Plan row carries the `SCN-GA-*` trace ID and points at an existing concrete test file: `internal/connector/alerts/alerts_test.go` for unit coverage; `tests/integration/weather_alerts_test.go` and `tests/e2e/weather_alerts_e2e_test.go` for Scope 06 NOTIF live-stack proxies.
3. Append one new DoD bullet per `SCN-GA-*` scenario to the existing DoD list of the owning scope. Each bullet cites the scenario by ID, names the concrete test file, and quotes raw `go test` PASS output captured in step 4.
4. Run the SCN-anchor unit tests via `go test -count=1 -v -run '...' ./internal/connector/alerts/` and capture raw PASS output for every scenario; embed inline.
5. Append a "BUG-017-001 — DoD Scenario Fidelity Gap" section to `specs/017-gov-alerts-connector/report.md` with per-scenario classification, raw guard before/after output, and full-path test-file references for `internal/connector/alerts/alerts_test.go`, `tests/integration/weather_alerts_test.go`, and `tests/e2e/weather_alerts_e2e_test.go`.
6. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and bug folder; run `bash .github/bubbles/scripts/traceability-guard.sh specs/017-gov-alerts-connector` and confirm `RESULT: PASSED`.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 13 mapped, 0 unmapped` | SCN-GA-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/017-gov-alerts-connector` | SCN-GA-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/017-gov-alerts-connector/bugs/BUG-017-001-dod-scenario-fidelity-gap` | SCN-GA-FIX-001 |
| T-FIX-1-04 | Underlying behavior tests still pass | unit | `internal/connector/alerts/alerts_test.go` | `go test -count=1 ./internal/connector/alerts/` exit 0; the SCN-anchor tests for all 13 scenarios PASS | SCN-GA-FIX-001 |

### Definition of Done

- [x] Parent `scopes.md` has a `### Test Plan` table on every one of the 6 scopes — **Phase:** implement
  > Evidence: `grep -c "^### Test Plan" specs/017-gov-alerts-connector/scopes.md` returns `6`.
- [x] Parent `scopes.md` has a DoD bullet citing `Scenario SCN-GA-PROX-001` and 12 more sibling SCN-GA-* IDs — **Phase:** implement
  > Evidence: `grep -c "Scenario SCN-GA-" specs/017-gov-alerts-connector/scopes.md` returns `13` (one new DoD bullet per scenario).
- [x] Parent `report.md` references `internal/connector/alerts/alerts_test.go`, `tests/integration/weather_alerts_test.go`, and `tests/e2e/weather_alerts_e2e_test.go` by full relative path — **Phase:** implement
  > Evidence: `grep -E "tests/integration/weather_alerts_test\.go|tests/e2e/weather_alerts_e2e_test\.go|internal/connector/alerts/alerts_test\.go" specs/017-gov-alerts-connector/report.md` returns matches for all three paths in the new BUG-017-001 cross-reference section.
- [x] Underlying behavior tests still pass — **Phase:** test
  > Evidence: see report.md `### Test Evidence` for raw `go test` output covering the 13 SCN-anchor tests; all PASS.
- [x] Traceability-guard PASSES against `specs/017-gov-alerts-connector` — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence` for full guard output. Final lines: `ℹ️  DoD fidelity: 13 scenarios checked, 13 mapped to DoD, 0 unmapped` and `RESULT: PASSED (0 warnings)`.
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/017-gov-alerts-connector/scopes.md`, `specs/017-gov-alerts-connector/report.md`, and `specs/017-gov-alerts-connector/bugs/BUG-017-001-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` are touched.
