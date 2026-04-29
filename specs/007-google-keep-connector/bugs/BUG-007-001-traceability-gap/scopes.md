# Scopes: BUG-007-001 — Traceability gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Register SCN-GK-030 in scenario-manifest.json

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GK-FIX-007-001 Trace guard sees 30/30 scenarios in manifest for spec 007
  Given specs/007-google-keep-connector/scopes.md defines 30 SCN-GK-* scenarios
  And specs/007-google-keep-connector/scenario-manifest.json now lists 30 scenarioId entries including SCN-007-030 mapped to TestQualifierRecentArchivedGetsLight
  When the workflow runs `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/007-google-keep-connector`
  Then the guard does not emit "scenario-manifest.json covers only N scenarios but scopes define M"
  And the overall result is PASSED with 0 failures
```

### Implementation Plan

1. Append a 30th entry `SCN-007-030` to `specs/007-google-keep-connector/scenario-manifest.json` mapping to the existing `internal/connector/keep/qualifiers_test.go::TestQualifierRecentArchivedGetsLight` and source `internal/connector/keep/qualifiers.go::Evaluate`.
2. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and the bug folder.
3. Run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/007-google-keep-connector` and confirm `RESULT: PASSED (0 warnings)`.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and no manifest count failure | SCN-GK-FIX-007-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/007-google-keep-connector` | SCN-GK-FIX-007-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/007-google-keep-connector/bugs/BUG-007-001-traceability-gap` | SCN-GK-FIX-007-001 |
| T-FIX-1-04 | Underlying behavior test still passes | unit | `internal/connector/keep/qualifiers_test.go` | `go test -count=1 -run TestQualifierRecentArchivedGetsLight ./internal/connector/keep/` exits 0 | SCN-GK-FIX-007-001 |

### Definition of Done

- [x] `specs/007-google-keep-connector/scenario-manifest.json` contains 30 `scenarioId` entries — **Phase:** implement
  > Evidence:
  > ```
  > $ grep -c '"scenarioId"' specs/007-google-keep-connector/scenario-manifest.json
  > 30
  > ```
- [x] The new `SCN-007-030` entry maps to the existing test `TestQualifierRecentArchivedGetsLight` in `internal/connector/keep/qualifiers_test.go` — **Phase:** implement
  > Evidence:
  > ```
  > $ grep -nE '"SCN-007-030"|TestQualifierRecentArchivedGetsLight|qualifiers\.go::Evaluate' specs/007-google-keep-connector/scenario-manifest.json
  > "scenarioId": "SCN-007-030",
  >         {"file": "internal/connector/keep/qualifiers_test.go", "function": "TestQualifierRecentArchivedGetsLight"}
  >         {"type": "unit-test", "location": "internal/connector/keep/qualifiers_test.go::TestQualifierRecentArchivedGetsLight"},
  >         {"type": "source", "location": "internal/connector/keep/qualifiers.go::Evaluate"}
  > ```
- [x] Underlying behavior test `TestQualifierRecentArchivedGetsLight` still passes — **Phase:** test
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestQualifierRecentArchivedGetsLight$' ./internal/connector/keep/
  > === RUN   TestQualifierRecentArchivedGetsLight
  > --- PASS: TestQualifierRecentArchivedGetsLight (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/keep  0.012s
  > ```
- [x] Traceability-guard PASSES against `specs/007-google-keep-connector` — **Phase:** validate
  > Evidence:
  > ```
  > $ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/007-google-keep-connector 2>&1 | tail -5
  > ℹ️  Concrete test file references: 30
  > ℹ️  Report evidence references: 30
  > ℹ️  DoD fidelity scenarios: 30 (mapped: 30, unmapped: 0)
  > 
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence:
  > ```
  > $ bash .github/bubbles/scripts/artifact-lint.sh specs/007-google-keep-connector 2>&1 | tail -3
  > === End Anti-Fabrication Checks ===
  > 
  > Artifact lint PASSED.
  > $ bash .github/bubbles/scripts/artifact-lint.sh specs/007-google-keep-connector/bugs/BUG-007-001-traceability-gap 2>&1 | tail -3
  > === End Anti-Fabrication Checks ===
  > 
  > Artifact lint PASSED.
  > ```
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/007-google-keep-connector/scenario-manifest.json` and the new bug folder. No files under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` are touched.
