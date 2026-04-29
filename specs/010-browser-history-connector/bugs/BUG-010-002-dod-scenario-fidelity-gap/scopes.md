# Scopes: BUG-010-002 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 010

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-BH-FIX-001 Trace guard accepts SCN-BH-001/002/003/004/008 as faithfully covered
  Given specs/010-browser-history-connector/scopes.md DoD entries that name each Gherkin scenario by ID
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/010-browser-history-connector`
  Then Gate G068 reports "11 scenarios checked, 11 mapped to DoD, 0 unmapped"
  And the overall result is PASSED
```

### Implementation Plan

1. Append SCN-BH-001 DoD bullet (with raw `go test` output for `TestProcessEntries_DwellTimeTiering`, `TestSync_EmptyCursor_UsesLookback` + source pointer to `connector.go::Connect`/`Sync`/`processEntries`, `browser.go::DwellTimeTier`) to Scope 01 DoD in `specs/010-browser-history-connector/scopes.md`
2. Append SCN-BH-002 DoD bullet (raw output for `TestParseChromeHistorySince_HasLimit`/`TestCursorConversion_RoundTrip`/`TestProcessEntries_CursorAdvances` + source pointer to `browser.go::ParseChromeHistorySince`, `connector.go::processEntries`/`parseCursorToChromeSafe`) to Scope 01 DoD
3. Append SCN-BH-003 DoD bullet (raw output for `TestProcessEntries_SkipFiltering`/`TestShouldSkip`/`TestShouldSkip_SchemePrefixedLocalhost` + source pointer to `browser.go::ShouldSkip`, `connector.go::processEntries`) to Scope 01 DoD
4. Append SCN-BH-004 DoD bullet (raw output for `TestConnect_HistoryFileNotFound`/`TestConnect_HistoryFileNotReadable`/`TestHealth_FileDisappearsAfterConnect` + source pointer to `connector.go::Connect`/`Health`) to Scope 01 DoD
5. Append SCN-BH-008 DoD bullet (raw output for `TestDetectRepeatVisits_TierEscalation`/`TestEscalateTier_AllTransitions`/`TestDetectRepeatVisits_BelowThreshold_NoEscalation`/`TestDetectRepeatVisits_RespectsWindow` + source pointer to `connector.go::detectRepeatVisits`/`escalateTier`) to Scope 02 DoD
6. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and bug folder; run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/010-browser-history-connector` and confirm PASS

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 11 mapped, 0 unmapped` | SCN-BH-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/010-browser-history-connector` | SCN-BH-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/010-browser-history-connector/bugs/BUG-010-002-dod-scenario-fidelity-gap` | SCN-BH-FIX-001 |
| T-FIX-1-04 | Underlying behavior tests still pass | unit | `internal/connector/browser/connector_test.go`, `browser_test.go` | `go test -count=1 -v -run '...' ./internal/connector/browser/` exit 0; the 15 named tests for SCN-BH-001/002/003/004/008 all PASS | SCN-BH-FIX-001 |

### Definition of Done

- [x] Scope 01 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-BH-001` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-BH-001" specs/010-browser-history-connector/scopes.md` shows the new DoD bullet inside Scope 01 DoD; full raw test output recorded inline.
- [x] Scope 01 DoD contains bullets citing `Scenario SCN-BH-002`, `SCN-BH-003`, `SCN-BH-004` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-BH-002\|Scenario SCN-BH-003\|Scenario SCN-BH-004" specs/010-browser-history-connector/scopes.md` returns three matches in the Scope 01 DoD section; full raw test output recorded inline.
- [x] Scope 02 DoD contains a bullet citing `Scenario SCN-BH-008` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-BH-008" specs/010-browser-history-connector/scopes.md` returns one match in the Scope 02 DoD section; full raw test output recorded inline.
- [x] Underlying behavior tests still pass — **Phase:** test
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestProcessEntries_DwellTimeTiering$|TestSync_EmptyCursor_UsesLookback$|TestParseChromeHistorySince_HasLimit$|TestCursorConversion_RoundTrip$|TestProcessEntries_CursorAdvances$|TestProcessEntries_SkipFiltering$|TestShouldSkip$|TestShouldSkip_SchemePrefixedLocalhost$|TestConnect_HistoryFileNotFound$|TestConnect_HistoryFileNotReadable$|TestHealth_FileDisappearsAfterConnect$|TestDetectRepeatVisits_TierEscalation$|TestEscalateTier_AllTransitions$|TestDetectRepeatVisits_BelowThreshold_NoEscalation$|TestDetectRepeatVisits_RespectsWindow$' ./internal/connector/browser/
  > === RUN   TestShouldSkip
  > --- PASS: TestShouldSkip (0.00s)
  > === RUN   TestShouldSkip_SchemePrefixedLocalhost
  > --- PASS: TestShouldSkip_SchemePrefixedLocalhost (0.00s)
  > === RUN   TestParseChromeHistorySince_HasLimit
  > --- PASS: TestParseChromeHistorySince_HasLimit (0.00s)
  > === RUN   TestProcessEntries_DwellTimeTiering
  > --- PASS: TestProcessEntries_DwellTimeTiering (0.00s)
  > === RUN   TestProcessEntries_SkipFiltering
  > --- PASS: TestProcessEntries_SkipFiltering (0.00s)
  > === RUN   TestConnect_HistoryFileNotFound
  > --- PASS: TestConnect_HistoryFileNotFound (0.00s)
  > === RUN   TestCursorConversion_RoundTrip
  > --- PASS: TestCursorConversion_RoundTrip (0.00s)
  > === RUN   TestSync_EmptyCursor_UsesLookback
  > --- PASS: TestSync_EmptyCursor_UsesLookback (0.00s)
  > === RUN   TestProcessEntries_CursorAdvances
  > --- PASS: TestProcessEntries_CursorAdvances (0.00s)
  > === RUN   TestDetectRepeatVisits_TierEscalation
  > --- PASS: TestDetectRepeatVisits_TierEscalation (0.00s)
  > === RUN   TestEscalateTier_AllTransitions
  > --- PASS: TestEscalateTier_AllTransitions (0.00s)
  > === RUN   TestDetectRepeatVisits_BelowThreshold_NoEscalation
  > --- PASS: TestDetectRepeatVisits_BelowThreshold_NoEscalation (0.00s)
  > === RUN   TestDetectRepeatVisits_RespectsWindow
  > --- PASS: TestDetectRepeatVisits_RespectsWindow (0.00s)
  > === RUN   TestConnect_HistoryFileNotReadable
  > --- PASS: TestConnect_HistoryFileNotReadable (0.01s)
  > === RUN   TestHealth_FileDisappearsAfterConnect
  > --- PASS: TestHealth_FileDisappearsAfterConnect (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/browser     0.092s
  > ```
- [x] Traceability-guard PASSES against `specs/010-browser-history-connector` — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence` for the full guard output. Final lines:
  > ```
  > ℹ️  DoD fidelity: 11 scenarios checked, 11 mapped to DoD, 0 unmapped
  > ℹ️  Concrete test file references: 11
  > ℹ️  Report evidence references: 11
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/010-browser-history-connector/scopes.md` and the bug folder. No files under `internal/`, `cmd/`, `ml/`, `config/`, `tests/` are touched.
