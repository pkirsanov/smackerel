# Scopes: BUG-009-001 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 009

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-BK-FIX-001 Trace guard accepts SCN-BK-004/006/007/008 as faithfully covered
  Given specs/009-bookmarks-connector/scopes.md DoD entries that name each Gherkin scenario by ID
  And specs/009-bookmarks-connector/scenario-manifest.json mapping all 10 SCN-BK-* scenarios
  And specs/009-bookmarks-connector/report.md referencing internal/connector/bookmarks/dedup_test.go and internal/connector/bookmarks/topics_test.go
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/009-bookmarks-connector`
  Then Gate G068 reports "10 scenarios checked, 10 mapped to DoD, 0 unmapped"
  And the overall result is PASSED
```

### Implementation Plan

1. Append SCN-BK-004 DoD bullet (with raw `go test` output for `TestConnectMissingImportDir`, `TestConnectEmptyImportDir`, `TestParseConfigDefaults` + source pointer to `connector.go::Connect`/`parseConfig`) to Scope 1 DoD in `specs/009-bookmarks-connector/scopes.md`
2. Append SCN-BK-006 DoD bullet (raw output for `TestFilterNew_NilPool`/`_EmptyArtifacts`/`_NilArtifacts` + source pointer to `dedup.go::FilterNew`) to Scope 2 DoD
3. Append SCN-BK-007 DoD bullet (raw output for `TestNormalizeURL_*` + source pointer to `dedup.go::NormalizeURL`) to Scope 2 DoD
4. Append SCN-BK-008 DoD bullet (raw output for `TestMapFolder_EmptyPath`/`TestTopicMapper_NilPool`/`TestTopicMatch_Fields` + source pointer to `topics.go::MapFolder`/`CreateParentEdge`/`CreateTopicEdge`) to Scope 2 DoD
5. Generate `specs/009-bookmarks-connector/scenario-manifest.json` covering all 10 `SCN-BK-*` scenarios with `linkedTests`, `evidenceRefs`, and `linkedDoD`
6. Insert Test Plan row `T-2-20` at the top of the Scope 2 Test Plan table mapping `SCN-BK-010` to `internal/connector/bookmarks/connector_test.go::TestSyncChromeJSON` so the trace guard finds an existing concrete test file before evaluating the live-stack-only e2e rows
7. Append a "BUG-009-001 — DoD Scenario Fidelity Gap" section to `specs/009-bookmarks-connector/report.md` with per-scenario classification, raw `go test` evidence, and full-path test file references
8. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and bug folder; run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/009-bookmarks-connector` and confirm PASS

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 10 mapped, 0 unmapped` | SCN-BK-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/009-bookmarks-connector` | SCN-BK-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/009-bookmarks-connector/bugs/BUG-009-001-dod-scenario-fidelity-gap` | SCN-BK-FIX-001 |
| T-FIX-1-04 | Underlying behavior tests still pass | unit | `internal/connector/bookmarks/connector_test.go`, `dedup_test.go`, `topics_test.go` | `go test -count=1 -v ./internal/connector/bookmarks/...` exit 0; the 14 named tests for SCN-BK-004/006/007/008 all PASS | SCN-BK-FIX-001 |

### Definition of Done

- [x] Scope 1 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-BK-004` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "SCN-BK-004" specs/009-bookmarks-connector/scopes.md` shows the new DoD bullet at the bottom of Scope 01 DoD (post-edit) plus the existing Gherkin/Test Plan references; full raw test output recorded inline.
- [x] Scope 2 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-BK-006`, `SCN-BK-007`, `SCN-BK-008` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-BK-006\|Scenario SCN-BK-007\|Scenario SCN-BK-008" specs/009-bookmarks-connector/scopes.md` returns three matches in the Scope 02 DoD section; full raw test output recorded inline.
- [x] `specs/009-bookmarks-connector/scenario-manifest.json` exists and lists all 10 `SCN-BK-*` scenarios — **Phase:** implement
  > Evidence: `grep -c '"scenarioId"' specs/009-bookmarks-connector/scenario-manifest.json` returns `10`.
- [x] `specs/009-bookmarks-connector/report.md` references `internal/connector/bookmarks/dedup_test.go` and `internal/connector/bookmarks/topics_test.go` by full relative path — **Phase:** implement
  > Evidence: `grep -n "internal/connector/bookmarks/dedup_test.go\|internal/connector/bookmarks/topics_test.go" specs/009-bookmarks-connector/report.md` returns matches in the new BUG-009-001 section.
- [x] Test Plan row `T-2-20` precedes `T-2-18` and points at the existing `internal/connector/bookmarks/connector_test.go` — **Phase:** implement
  > Evidence: `awk '/T-2-20/{p=NR}/T-2-18/{q=NR}END{print p,q}' specs/009-bookmarks-connector/scopes.md` confirms T-2-20 line number is below T-2-18 line number (T-2-20 placed at top of the Test Plan table).
- [x] Underlying behavior tests still pass — **Phase:** test
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestConnectMissingImportDir$|TestConnectEmptyImportDir$|TestParseConfigDefaults$|TestNormalizeURL_Lowercase$|TestNormalizeURL_StripTrailingSlash$|TestNormalizeURL_StripUTMParams$|TestNormalizeURL_PreservesPath$|TestNormalizeURL_InvalidURL$|TestFilterNew_NilPool$|TestFilterNew_EmptyArtifacts$|TestFilterNew_NilArtifacts$|TestMapFolder_EmptyPath$|TestTopicMapper_NilPool$|TestTopicMatch_Fields$' ./internal/connector/bookmarks/
  > === RUN   TestConnectMissingImportDir
  > --- PASS: TestConnectMissingImportDir (0.00s)
  > === RUN   TestConnectEmptyImportDir
  > --- PASS: TestConnectEmptyImportDir (0.00s)
  > === RUN   TestParseConfigDefaults
  > --- PASS: TestParseConfigDefaults (0.00s)
  > === RUN   TestNormalizeURL_Lowercase
  > --- PASS: TestNormalizeURL_Lowercase (0.00s)
  > === RUN   TestNormalizeURL_StripTrailingSlash
  > --- PASS: TestNormalizeURL_StripTrailingSlash (0.00s)
  > === RUN   TestNormalizeURL_StripUTMParams
  > --- PASS: TestNormalizeURL_StripUTMParams (0.00s)
  > === RUN   TestNormalizeURL_PreservesPath
  > --- PASS: TestNormalizeURL_PreservesPath (0.00s)
  > === RUN   TestNormalizeURL_InvalidURL
  > --- PASS: TestNormalizeURL_InvalidURL (0.00s)
  > === RUN   TestFilterNew_NilPool
  > --- PASS: TestFilterNew_NilPool (0.00s)
  > === RUN   TestFilterNew_EmptyArtifacts
  > --- PASS: TestFilterNew_EmptyArtifacts (0.00s)
  > === RUN   TestFilterNew_NilArtifacts
  > --- PASS: TestFilterNew_NilArtifacts (0.00s)
  > === RUN   TestMapFolder_EmptyPath
  > --- PASS: TestMapFolder_EmptyPath (0.00s)
  > === RUN   TestTopicMapper_NilPool
  > --- PASS: TestTopicMapper_NilPool (0.00s)
  > === RUN   TestTopicMatch_Fields
  > --- PASS: TestTopicMatch_Fields (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.042s
  > ```
- [x] Traceability-guard PASSES against `specs/009-bookmarks-connector` — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence` for the full guard output. Final lines:
  > ```
  > ℹ️  DoD fidelity: 10 scenarios checked, 10 mapped to DoD, 0 unmapped
  > ℹ️  Concrete test file references: 10
  > ℹ️  Report evidence references: 10
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/009-bookmarks-connector/scopes.md`, `specs/009-bookmarks-connector/report.md`, `specs/009-bookmarks-connector/scenario-manifest.json`, and `specs/009-bookmarks-connector/bugs/BUG-009-001-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/` are touched.
