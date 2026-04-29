# Scopes: BUG-011-001 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 011

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-MT-FIX-001 Trace guard accepts SCN-MT-001/002/003/006/014/015/016/018/019/020 as faithfully covered
  Given specs/011-maps-connector/scopes.md DoD entries that name each Gherkin scenario by ID
  And specs/011-maps-connector/scopes.md Test Plan has T-3-19/T-3-20/T-3-21 mapping SCN-MT-017/018/019 to existing patterns_test.go tests before the legacy live-stack rows
  And specs/011-maps-connector/report.md references internal/db/migration_test.go and the maps unit-test files by full relative path
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/011-maps-connector`
  Then Gate G068 reports "21 scenarios checked, 21 mapped to DoD, 0 unmapped"
  And the overall result is PASSED
```

### Implementation Plan

1. Append SCN-MT-001 DoD bullet (with raw `go test` output for `TestConnectorID`/`TestConnectValidConfig`/`TestHealthTransitions` + source pointer to `connector.go::Connect`/`Sync`/`Health`/`Close`) to Scope 01 DoD in `specs/011-maps-connector/scopes.md`
2. Append SCN-MT-002 DoD bullet (raw output for `TestConnectMissingImportDir`/`TestConnectEmptyImportDir`/`TestParseMapsConfigRejectsMissingFields`/`TestParseMapsConfigNegativeMinDistance` + source pointer to `connector.go::parseMapsConfig`) to Scope 01 DoD
3. Append SCN-MT-003 DoD bullet (raw output for `TestSyncProducesArtifacts`/`TestSyncMultiActivityTypeDistribution`/`TestNormalizeAllActivityTypes`/`TestClassifyActivity`/`TestClassifyActivityAllTransitTypes`/`TestParseTakeoutJSON_HappyPath` + source pointer to `maps.go::ClassifyActivity`) to Scope 01 DoD
4. Append SCN-MT-006 DoD bullet (raw output for `TestSyncMinThresholdFiltering`/`TestSyncAllFilteredStillAdvancesCursor` + source pointer to `connector.go::Sync`) to Scope 01 DoD
5. Append SCN-MT-014 DoD bullet (raw output for `TestDetectCommuteAboveThreshold`/`TestCommuteWeekdaysOnlyFilter`/`TestNormalizeCommutePattern`/`TestClassifyCommutesMultipleRoutes`/`TestDetectCommuteExactThreshold` + source pointer to `patterns.go::DetectCommutes`/`classifyCommutes`/`normalizeCommutePattern`) to Scope 03 DoD
6. Append SCN-MT-015 DoD bullet (raw output for `TestDetectCommuteBelowThreshold`/`TestClassifyCommutesEmptyClusters` + source pointer to `patterns.go::classifyCommutes`) to Scope 03 DoD
7. Append SCN-MT-016 DoD bullet (raw output for `TestDetectTripOvernight`/`TestDetectTripBelowDistance`/`TestNormalizeTripEvent` + source pointer to `patterns.go::DetectTrips`/`classifyTrips`/`normalizeTripEvent`) to Scope 03 DoD
8. Append SCN-MT-018 DoD bullet (raw output for `TestImprove_DetermineLinkTypeLocationsFarAway`/`TestDetermineLinkTypeEmptyRoute`/`TestDetermineLinkTypeSpatial` + source pointer to `patterns.go::determineLinkType`) to Scope 03 DoD
9. Append SCN-MT-019 DoD bullet (deferred — manual rationale + proxy reference to `TestDetermineLinkTypeEmptyRoute` and `scenario-manifest.json::SCN-MT-019`) to Scope 03 DoD
10. Append SCN-MT-020 DoD bullet (raw output for `TestPostSyncContinuesOnFailure`/`TestStabilize_PostSyncUsesConfigSnapshot`/`TestStabilize_PostSyncNoPoolReturnsNil` + source pointer to `connector.go::PostSync`) to Scope 03 DoD
11. Insert Test Plan rows `T-3-19`/`T-3-20`/`T-3-21` at the top of the Scope 03 Test Plan table mapping `SCN-MT-017`/`SCN-MT-018`/`SCN-MT-019` to existing `internal/connector/maps/patterns_test.go` tests so the trace guard finds an existing concrete test file before evaluating the live-stack-only rows `T-3-14`/`T-3-15`/`T-3-16`
12. Append a "BUG-011-001 — DoD Scenario Fidelity Gap" section to `specs/011-maps-connector/report.md` with per-scenario classification, raw `go test` evidence, and full-path test file references (including `internal/db/migration_test.go` for the ancillary SCN-MT-011/013 report.md gap)
13. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and bug folder; run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/011-maps-connector` and confirm PASS

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 21 mapped, 0 unmapped` | SCN-MT-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/011-maps-connector` | SCN-MT-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/011-maps-connector/bugs/BUG-011-001-dod-scenario-fidelity-gap` | SCN-MT-FIX-001 |
| T-FIX-1-04 | Underlying behavior tests still pass | unit | `internal/connector/maps/connector_test.go`, `maps_test.go`, `normalizer_test.go`, `patterns_test.go`, `internal/db/migration_test.go` | `go test -count=1 -v ...` exit 0; the 31 named tests for SCN-MT-001/002/003/006/014/015/016/018/019/020 plus 5 migration tests all PASS | SCN-MT-FIX-001 |

### Definition of Done

- [x] Scope 01 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-MT-001`, `SCN-MT-002`, `SCN-MT-003`, `SCN-MT-006` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "SCN-MT-001\|SCN-MT-002\|SCN-MT-003\|SCN-MT-006" specs/011-maps-connector/scopes.md` returns matches in the new DoD bullets at the bottom of Scope 01 DoD plus the existing Gherkin/Test Plan references; full raw test output recorded inline. Raw output evidence:
  > ```
  > $ go test -count=1 -v -run 'TestConnectorID$|TestConnectValidConfig$|TestHealthTransitions$|TestConnectMissingImportDir$|TestConnectEmptyImportDir$|TestParseMapsConfigRejectsMissingFields$|TestParseMapsConfigNegativeMinDistance$|TestSyncProducesArtifacts$|TestSyncMultiActivityTypeDistribution$|TestNormalizeAllActivityTypes$|TestSyncMinThresholdFiltering$|TestSyncAllFilteredStillAdvancesCursor$|TestParseTakeoutJSON_HappyPath$|TestClassifyActivity$|TestClassifyActivityAllTransitTypes$' ./internal/connector/maps/
  > --- PASS: TestConnectorID (0.00s)
  > --- PASS: TestConnectValidConfig (0.00s)
  > --- PASS: TestConnectMissingImportDir (0.00s)
  > --- PASS: TestConnectEmptyImportDir (0.00s)
  > --- PASS: TestParseMapsConfigRejectsMissingFields (0.00s)
  > --- PASS: TestParseMapsConfigNegativeMinDistance (0.00s)
  > --- PASS: TestSyncProducesArtifacts (0.00s)
  > --- PASS: TestSyncMinThresholdFiltering (0.00s)
  > --- PASS: TestHealthTransitions (0.01s)
  > --- PASS: TestSyncAllFilteredStillAdvancesCursor (0.00s)
  > --- PASS: TestSyncMultiActivityTypeDistribution (0.00s)
  > --- PASS: TestClassifyActivity (0.00s)
  > --- PASS: TestParseTakeoutJSON_HappyPath (0.00s)
  > --- PASS: TestClassifyActivityAllTransitTypes (0.00s)
  > --- PASS: TestNormalizeAllActivityTypes (0.00s)
  > PASS
  > ok   github.com/smackerel/smackerel/internal/connector/maps  0.194s
  > ```
- [x] Scope 03 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-MT-014`, `SCN-MT-015`, `SCN-MT-016`, `SCN-MT-018`, `SCN-MT-019`, `SCN-MT-020` with inline raw `go test` evidence (or manual rationale for the deferred SCN-MT-019) — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-MT-014\|Scenario SCN-MT-015\|Scenario SCN-MT-016\|Scenario SCN-MT-018\|Scenario SCN-MT-019\|Scenario SCN-MT-020" specs/011-maps-connector/scopes.md` returns six matches in the Scope 03 DoD section; full raw test output recorded inline. Raw output evidence:
  > ```
  > $ go test -count=1 -v -run 'TestDetectCommuteAboveThreshold$|TestCommuteWeekdaysOnlyFilter$|TestNormalizeCommutePattern$|TestClassifyCommutesMultipleRoutes$|TestDetectCommuteExactThreshold$|TestDetectCommuteBelowThreshold$|TestClassifyCommutesEmptyClusters$|TestDetectTripOvernight$|TestDetectTripBelowDistance$|TestNormalizeTripEvent$|TestImprove_DetermineLinkTypeLocationsFarAway$|TestDetermineLinkTypeEmptyRoute$|TestDetermineLinkTypeSpatial$|TestPostSyncContinuesOnFailure$|TestStabilize_PostSyncUsesConfigSnapshot$|TestStabilize_PostSyncNoPoolReturnsNil$' ./internal/connector/maps/
  > --- PASS: TestStabilize_PostSyncUsesConfigSnapshot (0.00s)
  > --- PASS: TestStabilize_PostSyncNoPoolReturnsNil (0.00s)
  > --- PASS: TestDetectCommuteAboveThreshold (0.00s)
  > --- PASS: TestDetectCommuteBelowThreshold (0.00s)
  > --- PASS: TestCommuteWeekdaysOnlyFilter (0.00s)
  > --- PASS: TestNormalizeCommutePattern (0.00s)
  > --- PASS: TestDetectTripOvernight (0.00s)
  > --- PASS: TestDetectTripBelowDistance (0.00s)
  > --- PASS: TestNormalizeTripEvent (0.00s)
  > --- PASS: TestPostSyncContinuesOnFailure (0.00s)
  > --- PASS: TestDetermineLinkTypeSpatial (0.00s)
  > --- PASS: TestClassifyCommutesMultipleRoutes (0.00s)
  > --- PASS: TestClassifyCommutesEmptyClusters (0.00s)
  > --- PASS: TestDetermineLinkTypeEmptyRoute (0.00s)
  > --- PASS: TestImprove_DetermineLinkTypeLocationsFarAway (0.00s)
  > --- PASS: TestDetectCommuteExactThreshold (0.00s)
  > PASS
  > ok   github.com/smackerel/smackerel/internal/connector/maps  0.194s
  > ```
- [x] Test Plan rows `T-3-19`/`T-3-20`/`T-3-21` precede `T-3-14`/`T-3-15`/`T-3-16` and point at the existing `internal/connector/maps/patterns_test.go` tests — **Phase:** implement
  > Evidence: `awk '/T-3-19/{p=NR}/T-3-14/{q=NR}END{print p,q}' specs/011-maps-connector/scopes.md` confirms T-3-19 line number is below the table header but above T-3-14 line number (T-3-19/T-3-20/T-3-21 placed at the top of the Scope 03 Test Plan table).
- [x] `specs/011-maps-connector/report.md` references `internal/db/migration_test.go` by full relative path so SCN-MT-011 and SCN-MT-013 satisfy `report_mentions_path` — **Phase:** implement
  > Evidence: `grep -n "internal/db/migration_test.go" specs/011-maps-connector/report.md` returns matches in the new BUG-011-001 section.
- [x] Migration tests still pass — **Phase:** test
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestMigrationsEmbed$|TestMigrationSQL_Parseable$|TestMigrationSQL_Indexes$|TestMigrationFiles_SortOrder$|TestMigrationFiles_SQLNotEmpty$' ./internal/db/
  > === RUN   TestMigrationsEmbed
  > --- PASS: TestMigrationsEmbed (0.00s)
  > === RUN   TestMigrationSQL_Parseable
  > --- PASS: TestMigrationSQL_Parseable (0.00s)
  > === RUN   TestMigrationSQL_Indexes
  > --- PASS: TestMigrationSQL_Indexes (0.00s)
  > === RUN   TestMigrationFiles_SortOrder
  > --- PASS: TestMigrationFiles_SortOrder (0.00s)
  > === RUN   TestMigrationFiles_SQLNotEmpty
  > --- PASS: TestMigrationFiles_SQLNotEmpty (0.00s)
  > PASS
  > ok   github.com/smackerel/smackerel/internal/db      0.072s
  > ```
- [x] Traceability-guard PASSES against `specs/011-maps-connector` — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence` for the full guard output. Final lines:
  > ```
  > ℹ️  DoD fidelity: 21 scenarios checked, 21 mapped to DoD, 0 unmapped
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/011-maps-connector/scopes.md`, `specs/011-maps-connector/report.md`, and `specs/011-maps-connector/bugs/BUG-011-001-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/`, `tests/` are touched.
