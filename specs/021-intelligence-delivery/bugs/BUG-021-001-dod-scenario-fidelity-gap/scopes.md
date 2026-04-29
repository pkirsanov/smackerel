# Scopes: BUG-021-001 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 021

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-021-FIX-001 Trace guard accepts SCN-021-003/004/005/006/007/009/010/012/013/015 as faithfully covered
  Given specs/021-intelligence-delivery/scopes.md DoD entries that name each Gherkin scenario by ID
  And specs/021-intelligence-delivery/scenario-manifest.json mapping all 15 SCN-021-* scenarios
  And specs/021-intelligence-delivery/report.md referencing internal/api/search_test.go, internal/intelligence/lookups_test.go, internal/intelligence/alert_producers_test.go, internal/intelligence/engine_test.go, and internal/scheduler/jobs_test.go by full relative path
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/021-intelligence-delivery`
  Then Gate G068 reports "15 scenarios checked, 15 mapped to DoD, 0 unmapped"
  And the overall result is PASSED
```

### Implementation Plan

1. Append SCN-021-003 / -004 / -005 / -006 / -007 / -015 DoD bullets to Scope 1 DoD in `specs/021-intelligence-delivery/scopes.md` with raw `go test` evidence and source pointers (`alert_producers.go`, `alerts.go`, `jobs.go`)
2. Append SCN-021-009 / -010 DoD bullets to Scope 2 DoD with raw `go test` evidence and source pointers (`search.go::searchHandler`, `lookups.go::DetectFrequentLookups`)
3. Append SCN-021-012 / -013 DoD bullets to Scope 3 DoD with raw `go test` evidence and source pointers (`health.go::healthHandler`, `synthesis.go::GetLastSynthesisTime`)
4. Generate `specs/021-intelligence-delivery/scenario-manifest.json` covering all 15 `SCN-021-*` scenarios with `linkedTests`, `evidenceRefs`, and `linkedDoD`
5. Insert Test Plan proxy row `T-1-PROXY-003` at the top of the Scope 1 Test Plan table mapping `SCN-021-003` to `internal/scheduler/jobs_test.go::TestDeliverAlertBatch_HappyPath`; insert proxy row `T-2-PROXY-010` at the top of the Scope 2 Test Plan table mapping `SCN-021-010` to `internal/intelligence/lookups_test.go::TestFrequentLookup_MinimumThreshold`
6. Append a "BUG-021-001 — DoD Scenario Fidelity Gap" section to `specs/021-intelligence-delivery/report.md` with per-scenario classification, raw `go test` evidence, and full-path test file references for the affected files
7. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and bug folder; run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/021-intelligence-delivery` and confirm PASS

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 15 mapped, 0 unmapped` | SCN-021-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/021-intelligence-delivery` | SCN-021-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/021-intelligence-delivery/bugs/BUG-021-001-dod-scenario-fidelity-gap` | SCN-021-FIX-001 |
| T-FIX-1-04 | Underlying behavior tests still pass | unit | `internal/intelligence/alert_producers_test.go`, `internal/intelligence/engine_test.go`, `internal/intelligence/lookups_test.go`, `internal/api/search_test.go`, `internal/api/health_test.go`, `internal/scheduler/jobs_test.go` | `go test -count=1 -v -run '<filtered set>' ./...` exit 0; the named tests for SCN-021-003/004/005/006/007/009/010/012/013/015 all PASS | SCN-021-FIX-001 |

### Definition of Done

- [x] Scope 1 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-021-003`, `SCN-021-004`, `SCN-021-005`, `SCN-021-006`, `SCN-021-007`, `SCN-021-015` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-021-003\|Scenario SCN-021-004\|Scenario SCN-021-005\|Scenario SCN-021-006\|Scenario SCN-021-007\|Scenario SCN-021-015" specs/021-intelligence-delivery/scopes.md` returns six matches in the Scope 1 DoD section; full raw test output recorded inline.
- [x] Scope 2 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-021-009`, `SCN-021-010` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-021-009\|Scenario SCN-021-010" specs/021-intelligence-delivery/scopes.md` returns two matches in the Scope 2 DoD section; full raw test output recorded inline.
- [x] Scope 3 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-021-012`, `SCN-021-013` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-021-012\|Scenario SCN-021-013" specs/021-intelligence-delivery/scopes.md` returns two matches in the Scope 3 DoD section; full raw test output recorded inline.
- [x] `specs/021-intelligence-delivery/scenario-manifest.json` exists and lists all 15 `SCN-021-*` scenarios — **Phase:** implement
  > Evidence: `grep -c '"scenarioId"' specs/021-intelligence-delivery/scenario-manifest.json` returns `15`.
- [x] `specs/021-intelligence-delivery/report.md` references `internal/api/search_test.go`, `internal/intelligence/lookups_test.go`, `internal/intelligence/alert_producers_test.go`, `internal/intelligence/engine_test.go`, and `internal/scheduler/jobs_test.go` by full relative path — **Phase:** implement
  > Evidence: `grep -n "internal/api/search_test.go\|internal/intelligence/lookups_test.go\|internal/intelligence/alert_producers_test.go\|internal/intelligence/engine_test.go\|internal/scheduler/jobs_test.go" specs/021-intelligence-delivery/report.md` returns multiple matches in the new BUG-021-001 section.
- [x] Test Plan proxy rows `T-1-PROXY-003` and `T-2-PROXY-010` exist and point at existing test files — **Phase:** implement
  > Evidence: `grep -n "T-1-PROXY-003\|T-2-PROXY-010" specs/021-intelligence-delivery/scopes.md` returns matches at the top of Scope 1 and Scope 2 Test Plan tables respectively, ahead of the legacy live-stack rows.
- [x] Underlying behavior tests still pass — **Phase:** test
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestProduceBillAlerts|TestBillingTitleFormat|TestMonthlyBillingRollover|TestBillingDate_LocalMidnightNotUTCTruncate|TestProduceTripPrepAlerts|TestTripPrepDaysUntil|TestProduceReturnWindowAlerts|TestDeliverAlertBatch_EmptyList_NoOp|TestDeliverPendingAlerts_NilEngine|TestSearchHandler_SuccessWithResults|TestSearchHandler_LogSearchCalledWithMultipleResults|TestSearchHandler_LogSearchCalledWithZeroResults|TestSearchHandler_LogSearchTopResultIDFromFirstResult|TestSearchHandler_LogSearchFailureNonBlocking|TestDetectFrequentLookups_NilPool|TestFrequentLookup_MinimumThreshold|TestNormalizeQuery|TestHashQuery|TestGetLastSynthesisTime|TestHealthHandler_IntelligenceStalenessThreshold|TestHealthHandler_IntelligenceFreshInstallNotStale|TestHealthHandler_IntelligenceDownDegrades' ./internal/intelligence/... ./internal/api/... ./internal/scheduler/...
  > === RUN   TestBillingTitleFormat_WithAmount
  > --- PASS: TestBillingTitleFormat_WithAmount (0.00s)
  > === RUN   TestBillingTitleFormat_ZeroAmount
  > --- PASS: TestBillingTitleFormat_ZeroAmount (0.00s)
  > === RUN   TestMonthlyBillingRollover
  > --- PASS: TestMonthlyBillingRollover (0.00s)
  > === RUN   TestGetLastSynthesisTime_ValidatesPoolFirst
  > --- PASS: TestGetLastSynthesisTime_ValidatesPoolFirst (0.00s)
  > === RUN   TestBillingDate_LocalMidnightNotUTCTruncate
  > --- PASS: TestBillingDate_LocalMidnightNotUTCTruncate (0.00s)
  > === RUN   TestTripPrepDaysUntil_UsesCalendarDays
  > --- PASS: TestTripPrepDaysUntil_UsesCalendarDays (0.00s)
  > === RUN   TestTripPrepDaysUntil_DSTSpringForward
  > --- PASS: TestTripPrepDaysUntil_DSTSpringForward (0.00s)
  > === RUN   TestNormalizeQuery
  > --- PASS: TestNormalizeQuery (0.00s)
  > === RUN   TestHashQuery
  > --- PASS: TestHashQuery (0.00s)
  > === RUN   TestDetectFrequentLookups_NilPool
  > --- PASS: TestDetectFrequentLookups_NilPool (0.00s)
  > === RUN   TestFrequentLookup_MinimumThreshold
  > --- PASS: TestFrequentLookup_MinimumThreshold (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/intelligence    0.136s
  > === RUN   TestHealthHandler_IntelligenceFreshInstallNotStale
  > --- PASS: TestHealthHandler_IntelligenceFreshInstallNotStale (0.00s)
  > === RUN   TestHealthHandler_IntelligenceStalenessThreshold
  > --- PASS: TestHealthHandler_IntelligenceStalenessThreshold (0.00s)
  > === RUN   TestHealthHandler_IntelligenceDownDegrades
  > --- PASS: TestHealthHandler_IntelligenceDownDegrades (0.00s)
  > === RUN   TestSearchHandler_SuccessWithResults
  > --- PASS: TestSearchHandler_SuccessWithResults (0.00s)
  > === RUN   TestSearchHandler_LogSearchFailureNonBlocking
  > --- PASS: TestSearchHandler_LogSearchFailureNonBlocking (0.00s)
  > === RUN   TestSearchHandler_LogSearchCalledWithZeroResults
  > --- PASS: TestSearchHandler_LogSearchCalledWithZeroResults (0.00s)
  > === RUN   TestSearchHandler_LogSearchCalledWithMultipleResults
  > --- PASS: TestSearchHandler_LogSearchCalledWithMultipleResults (0.05s)
  > === RUN   TestSearchHandler_LogSearchTopResultIDFromFirstResult
  > --- PASS: TestSearchHandler_LogSearchTopResultIDFromFirstResult (0.05s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/api     0.249s
  > === RUN   TestDeliverAlertBatch_EmptyList_NoOp
  > --- PASS: TestDeliverAlertBatch_EmptyList_NoOp (0.00s)
  > === RUN   TestDeliverPendingAlerts_NilEngine
  > --- PASS: TestDeliverPendingAlerts_NilEngine (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/scheduler       0.051s
  > ```
- [x] Traceability-guard PASSES against `specs/021-intelligence-delivery` — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence` for the full guard output. Final lines:
  > ```
  > ℹ️  DoD fidelity: 15 scenarios checked, 15 mapped to DoD, 0 unmapped
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/021-intelligence-delivery/scopes.md`, `specs/021-intelligence-delivery/report.md`, `specs/021-intelligence-delivery/scenario-manifest.json`, and the bug folder. No files under `internal/`, `cmd/`, `ml/`, `config/` are touched.
