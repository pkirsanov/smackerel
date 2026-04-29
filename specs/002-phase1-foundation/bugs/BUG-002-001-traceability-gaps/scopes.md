# Scopes: BUG-002-001 — Traceability gaps in spec 002

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore traceability-guard linkage for spec 002 (manifest + report + Test Plan row)

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-002-FIX-001 Trace guard accepts spec 002 with full manifest, complete report file references, and SCN-002-080 Test Plan row
  Given specs/002-phase1-foundation/scenario-manifest.json lists all 82 SCN-002-* scenarios
  And specs/002-phase1-foundation/report.md literally contains "internal/scheduler/scheduler_test.go", "internal/auth/oauth_test.go", and "internal/connector/supervisor_test.go"
  And specs/002-phase1-foundation/scopes.md Scope 24 Test Plan has a row mapping SCN-002-080 to ml/tests/test_ocr.py
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/002-phase1-foundation`
  Then no scope reports a missing report evidence reference
  And no scenario is flagged as having no traceable Test Plan row
  And the manifest coverage check passes (82 of 82 scenarios)
  And the overall result is PASSED
```

### Implementation Plan

1. Append 38 new entries (`SCN-002-045`–`SCN-002-082`) to `specs/002-phase1-foundation/scenario-manifest.json`, each with `scenarioId`, `scope`, `requiredTestType`, `linkedTests`, `evidenceRefs`, and `linkedDoD` taken from the Test Plan tables of scopes 9–25 in `scopes.md`
2. Insert a Test Plan row in Scope 24 of `specs/002-phase1-foundation/scopes.md` mapping `SCN-002-080` to `ml/tests/test_ocr.py::TestExtractTextTesseract::test_returns_empty_on_exception`
3. Append a "BUG-002-001 — Traceability Gaps" section to `specs/002-phase1-foundation/report.md` containing the concrete test file paths (`internal/scheduler/scheduler_test.go`, `internal/auth/oauth_test.go`, `internal/connector/supervisor_test.go`, `ml/tests/test_ocr.py`) plus raw before/after guard output
4. Run `bash .github/bubbles/scripts/artifact-lint.sh` against parent and bug folder; run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/002-phase1-foundation` and confirm PASS

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and 82/82 manifest coverage | SCN-002-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/002-phase1-foundation` | SCN-002-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/002-phase1-foundation/bugs/BUG-002-001-traceability-gaps` | SCN-002-FIX-001 |
| T-FIX-1-04 | Underlying behavior tests still pass | unit | `internal/scheduler/scheduler_test.go`, `internal/auth/oauth_test.go`, `internal/connector/supervisor_test.go` | `go test -count=1` for the four flagged scopes' tests exits 0 | SCN-002-FIX-001 |

### Definition of Done

- [x] `specs/002-phase1-foundation/scenario-manifest.json` contains 82 scenario entries covering `SCN-002-001` through `SCN-002-082` — **Phase:** implement
  > Evidence:
  > ```
  > $ python3 -c "import json; m=json.load(open('specs/002-phase1-foundation/scenario-manifest.json')); ids=[s['scenarioId'] for s in m['scenarios']]; print('count:', len(ids)); print('first:', ids[0], 'last:', ids[-1])"
  > count: 82
  > first: SCN-002-001 last: SCN-002-082
  > ```
- [x] `specs/002-phase1-foundation/report.md` literally contains the four flagged test-file paths — **Phase:** implement
  > Evidence:
  > ```
  > $ grep -cE "internal/scheduler/scheduler_test\.go|internal/auth/oauth_test\.go|internal/connector/supervisor_test\.go|ml/tests/test_ocr\.py" specs/002-phase1-foundation/report.md
  > 8
  > ```
- [x] `specs/002-phase1-foundation/scopes.md` Scope 24 Test Plan has a row mapping `SCN-002-080` to an existing concrete test file — **Phase:** implement
  > Evidence:
  > ```
  > $ awk '/^## Scope 24:/,/^## Scope 25:/' specs/002-phase1-foundation/scopes.md | grep "SCN-002-080"
  > | 2 | OCR failure returns empty string (graceful fallback) | Unit | ml/tests/test_ocr.py | SCN-002-080 |
  > ```
- [x] Underlying behavior tests still pass — **Phase:** test
  > Evidence: `go test -count=1` runs for the affected files complete with exit 0; full output captured inline in `report.md` under "Test Evidence".
- [x] Traceability-guard PASSES against `specs/002-phase1-foundation` — **Phase:** validate
  > Evidence: see `report.md` "Validation Evidence" section. Final lines:
  > ```
  > ℹ️  Concrete test file references: 82
  > ℹ️  Report evidence references: 82
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see `report.md` "Audit Evidence" section.
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` after the fix shows changes confined to `specs/002-phase1-foundation/scenario-manifest.json`, `specs/002-phase1-foundation/scopes.md`, `specs/002-phase1-foundation/report.md`, `specs/002-phase1-foundation/state.json`, and `specs/002-phase1-foundation/bugs/BUG-002-001-traceability-gaps/*`. No files under `internal/`, `cmd/`, `ml/app/`, `config/`, `tests/` are touched.
