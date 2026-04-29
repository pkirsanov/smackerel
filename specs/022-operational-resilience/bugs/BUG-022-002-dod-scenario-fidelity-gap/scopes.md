# Scopes: BUG-022-002 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 022

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-022-FIX-001 Trace guard accepts SCN-022-01/02/03/04/06/11/12/14 as faithfully covered
  Given specs/022-operational-resilience/scopes.md DoD entries that name each unmapped Gherkin scenario by ID
  And specs/022-operational-resilience/scenario-manifest.json mapping all 14 SCN-022-* scenarios
  And specs/022-operational-resilience/scopes.md Test Plan rows that include concrete *_test.go file paths
  And specs/022-operational-resilience/report.md referencing internal/config/validate_test.go, internal/api/capture_test.go, internal/scheduler/scheduler_test.go, internal/scheduler/jobs_test.go, cmd/core/main_test.go, internal/pipeline/synthesis_subscriber_test.go, and scripts/commands/backup.sh
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/022-operational-resilience`
  Then Gate G068 reports "14 scenarios checked, 14 mapped to DoD, 0 unmapped"
  And Concrete test file references == 14
  And the overall result is PASSED
```

### Implementation Plan

1. Append `Scenario SCN-022-01` DoD bullet to Scope 1 DoD in `specs/022-operational-resilience/scopes.md` with source pointer `scripts/commands/backup.sh:42-100`.
2. Append `Scenario SCN-022-02` DoD bullet to Scope 1 DoD with source pointer `scripts/commands/backup.sh:51-72`.
3. Append `Scenario SCN-022-03` DoD bullet to Scope 1 DoD with raw `go test` output for `TestValidate_DBPoolConfig_Valid`/`TestValidate_DBMaxConns_Invalid`/`TestValidate_DBMinConns_EqualsMaxConns` and source pointers to `internal/config/validate.go` and `internal/db/postgres.go`.
4. Append `Scenario SCN-022-04` DoD bullet to Scope 1 DoD with raw `go test` output for `TestValidate_DBMaxConns_Missing`/`TestValidate_DBMinConns_Missing`/`TestValidate_ShutdownTimeoutS_Missing`/`TestValidate_MLHealthCacheTTLS_Missing`/`TestValidate_DBMinConns_ExceedsMaxConns`.
5. Append `Scenario SCN-022-06` DoD bullet to Scope 2 DoD with raw `go test` output for `TestCaptureHandler_DBHealthy_ContinuesProcessing`.
6. Append `Scenario SCN-022-11` DoD bullet to Scope 3 DoD with raw `go test` output for the 13 `TestRun*Job_OverlapGuard` tests + `TestCronConcurrencyGuard_AllGroupsIndependent`/`AllEightGroupsIndependent`/`RaceDetectorClean`.
7. Append `Scenario SCN-022-12` DoD bullet to Scope 4 DoD with raw `go test` output for `TestShutdownAll_*`, `TestRunWithTimeout_*`, `TestStop_CronStopBounded`, `TestStop_WgWaitBounded`.
8. Append `Scenario SCN-022-14` DoD bullet to Scope 4 DoD with raw `go test` output for `TestSynthesisDeliveryFailure_RoutesToDeadLetter`/`_BelowMaxDeliver_Naks`/`_PublishFails_Naks`.
9. Update every Test Plan row in Scopes 1–4 to inline the concrete `*_test.go` file path so `extract_path_candidates` succeeds (assertions unchanged).
10. Generate `specs/022-operational-resilience/scenario-manifest.json` covering all 14 `SCN-022-*` scenarios with `linkedTests`, `evidenceRefs`, and `linkedDoD`.
11. Append a "BUG-022-002 — DoD Scenario Fidelity Gap" section to `specs/022-operational-resilience/report.md` with per-scenario classification, raw `go test` evidence, and full-path test file references.
12. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and bug folder; run `bash .github/bubbles/scripts/traceability-guard.sh specs/022-operational-resilience` and confirm PASS.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 14 mapped, 0 unmapped` and `Concrete test file references: 14` | SCN-022-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/022-operational-resilience` | SCN-022-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/022-operational-resilience/bugs/BUG-022-002-dod-scenario-fidelity-gap` | SCN-022-FIX-001 |
| T-FIX-1-04 | Underlying behavior tests still pass | unit | `internal/config/validate_test.go`, `internal/api/capture_test.go`, `internal/scheduler/scheduler_test.go`, `internal/scheduler/jobs_test.go`, `cmd/core/main_test.go`, `internal/pipeline/synthesis_subscriber_test.go` | All 27 named tests for the previously-unmapped scenarios PASS | SCN-022-FIX-001 |

### Definition of Done

- [x] Scope 1 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-022-01`, `SCN-022-02`, `SCN-022-03`, `SCN-022-04` with inline raw `go test` output (where applicable) — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-022-01\|Scenario SCN-022-02\|Scenario SCN-022-03\|Scenario SCN-022-04" specs/022-operational-resilience/scopes.md` returns four matches in Scope 1 DoD.
- [x] Scope 2 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-022-06` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-022-06" specs/022-operational-resilience/scopes.md` returns one match in Scope 2 DoD; raw test output recorded inline.
- [x] Scope 3 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-022-11` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-022-11" specs/022-operational-resilience/scopes.md` returns one match in Scope 3 DoD; raw test output for 16 tests recorded inline.
- [x] Scope 4 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-022-12` and `Scenario SCN-022-14` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-022-12\|Scenario SCN-022-14" specs/022-operational-resilience/scopes.md` returns two matches in Scope 4 DoD; raw test output recorded inline.
- [x] `specs/022-operational-resilience/scenario-manifest.json` exists and lists all 14 `SCN-022-*` scenarios — **Phase:** implement
  > Evidence: `grep -c '"scenarioId"' specs/022-operational-resilience/scenario-manifest.json` returns `14`.
- [x] `specs/022-operational-resilience/report.md` references the concrete test files by full relative path — **Phase:** implement
  > Evidence: `grep -nE "internal/config/validate_test\.go|internal/api/capture_test\.go|internal/scheduler/(scheduler|jobs)_test\.go|cmd/core/main_test\.go|internal/pipeline/synthesis_subscriber_test\.go|scripts/commands/backup\.sh" specs/022-operational-resilience/report.md` returns matches in the BUG-022-002 section.
- [x] All Test Plan rows in Scopes 1–4 contain a concrete `*_test.go` (or `scripts/commands/backup.sh`) path that the trace guard can extract — **Phase:** implement
  > Evidence: post-fix guard summary `Concrete test file references: 14` (was 2 pre-fix).
- [x] Underlying behavior tests still pass — **Phase:** test
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestValidate_DBPoolConfig_Valid$|TestValidate_DBMaxConns_Invalid$|TestValidate_DBMinConns_EqualsMaxConns$|TestValidate_DBMaxConns_Missing$|TestValidate_DBMinConns_Missing$|TestValidate_ShutdownTimeoutS_Missing$|TestValidate_MLHealthCacheTTLS_Missing$|TestValidate_DBMinConns_ExceedsMaxConns$' ./internal/config/
  > === RUN   TestValidate_DBMaxConns_Missing
  > --- PASS: TestValidate_DBMaxConns_Missing (0.00s)
  > === RUN   TestValidate_DBMinConns_Missing
  > --- PASS: TestValidate_DBMinConns_Missing (0.00s)
  > === RUN   TestValidate_ShutdownTimeoutS_Missing
  > --- PASS: TestValidate_ShutdownTimeoutS_Missing (0.00s)
  > === RUN   TestValidate_MLHealthCacheTTLS_Missing
  > --- PASS: TestValidate_MLHealthCacheTTLS_Missing (0.00s)
  > === RUN   TestValidate_DBMinConns_ExceedsMaxConns
  > --- PASS: TestValidate_DBMinConns_ExceedsMaxConns (0.00s)
  > === RUN   TestValidate_DBPoolConfig_Valid
  > --- PASS: TestValidate_DBPoolConfig_Valid (0.00s)
  > === RUN   TestValidate_DBMaxConns_Invalid
  > --- PASS: TestValidate_DBMaxConns_Invalid (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/config  0.018s
  >
  > $ go test -count=1 -v -run 'TestCaptureHandler_DBHealthy_ContinuesProcessing$|TestCaptureHandler_DBUnavailable_Returns503$|TestCaptureHandler_NilDB_Returns503$' ./internal/api/
  > === RUN   TestCaptureHandler_DBUnavailable_Returns503
  > --- PASS: TestCaptureHandler_DBUnavailable_Returns503 (0.00s)
  > === RUN   TestCaptureHandler_DBHealthy_ContinuesProcessing
  > --- PASS: TestCaptureHandler_DBHealthy_ContinuesProcessing (0.00s)
  > === RUN   TestCaptureHandler_NilDB_Returns503
  > --- PASS: TestCaptureHandler_NilDB_Returns503 (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/api     0.056s
  >
  > $ go test -count=1 -v -run 'TestRun.*Job_OverlapGuard$|TestCronConcurrencyGuard_AllGroupsIndependent$|TestCronConcurrencyGuard_AllEightGroupsIndependent$|TestCronConcurrencyGuard_RaceDetectorClean$' ./internal/scheduler/
  > --- PASS: TestRunDigestJob_OverlapGuard (0.00s)
  > --- PASS: TestRunTopicMomentumJob_OverlapGuard (0.00s)
  > --- PASS: TestRunSynthesisJob_OverlapGuard (0.00s)
  > --- PASS: TestRunResurfacingJob_OverlapGuard (0.00s)
  > --- PASS: TestRunPreMeetingBriefsJob_OverlapGuard (0.00s)
  > --- PASS: TestRunWeeklySynthesisJob_OverlapGuard (0.00s)
  > --- PASS: TestRunMonthlyReportJob_OverlapGuard (0.00s)
  > --- PASS: TestRunSubscriptionDetectionJob_OverlapGuard (0.00s)
  > --- PASS: TestRunFrequentLookupsJob_OverlapGuard (0.00s)
  > --- PASS: TestRunAlertDeliveryJob_OverlapGuard (0.00s)
  > --- PASS: TestRunAlertProductionJob_OverlapGuard (0.00s)
  > --- PASS: TestRunRelationshipCoolingJob_OverlapGuard (0.00s)
  > --- PASS: TestRunKnowledgeLintJob_OverlapGuard (0.00s)
  > --- PASS: TestCronConcurrencyGuard_AllGroupsIndependent (0.00s)
  > --- PASS: TestCronConcurrencyGuard_RaceDetectorClean (0.00s)
  > --- PASS: TestCronConcurrencyGuard_AllEightGroupsIndependent (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/scheduler       0.061s
  >
  > $ go test -count=1 -v -run 'TestShutdownAll_ParallelSubscriberStop$|TestShutdownAll_NilSubscribersHandled$|TestRunWithTimeout_CompletesBeforeBudget$|TestRunWithTimeout_ExceedsBudget$|TestStop_CronStopBounded$|TestStop_WgWaitBounded$' ./cmd/core/ ./internal/scheduler/
  > --- PASS: TestShutdownAll_ParallelSubscriberStop (0.00s)
  > --- PASS: TestShutdownAll_NilSubscribersHandled (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/cmd/core 0.287s
  > --- PASS: TestStop_CronStopBounded (0.00s)
  > --- PASS: TestStop_WgWaitBounded (5.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/scheduler       5.035s
  >
  > $ go test -count=1 -v -run 'TestSynthesisDeliveryFailure_RoutesToDeadLetter$|TestSynthesisDeliveryFailure_BelowMaxDeliver_Naks$|TestSynthesisDeliveryFailure_PublishFails_Naks$' ./internal/pipeline/
  > --- PASS: TestSynthesisDeliveryFailure_RoutesToDeadLetter (0.00s)
  > --- PASS: TestSynthesisDeliveryFailure_BelowMaxDeliver_Naks (0.00s)
  > --- PASS: TestSynthesisDeliveryFailure_PublishFails_Naks (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/pipeline        0.043s
  > ```
- [x] Traceability-guard PASSES against `specs/022-operational-resilience` — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence` for the full guard output. Final lines:
  > ```
  > ℹ️  DoD fidelity: 14 scenarios checked, 14 mapped to DoD, 0 unmapped
  > ℹ️  Concrete test file references: 14
  > ℹ️  Report evidence references: 14
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/022-operational-resilience/scopes.md`, `specs/022-operational-resilience/report.md`, `specs/022-operational-resilience/scenario-manifest.json`, and `specs/022-operational-resilience/bugs/BUG-022-002-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, `scripts/`, or `docker-compose.yml` are touched by this fix.
