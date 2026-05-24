# Scopes: BUG-019-002 — Gherkin scenario fidelity gap (SCN-019-004 + SCN-019-005)

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin scenario fidelity for SCN-019-004 and SCN-019-005

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-019-002-FIX-001 Trace guard accepts SCN-019-004 and SCN-019-005 as faithfully covered
  Given specs/019-connector-wiring/scopes.md Scope 1 DoD contains 2 new bullets carrying the literal tokens "Scenario SCN-019-004" and "Scenario SCN-019-005"
  And each new bullet preserves enough scenario-distinguishing words to satisfy the v3.8.0 G068 matcher threshold (>=5 for SCN-019-004, >=4 for SCN-019-005)
  And no pre-existing DoD bullet in specs/019-connector-wiring/scopes.md is deleted or weakened
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/019-connector-wiring`
  Then Gate G068 reports "DoD fidelity: 6 scenarios checked, 6 mapped to DoD, 0 unmapped"
  And the overall result is PASSED
```

### Implementation Plan

1. Append a `Scenario SCN-019-004 (Config entries exist for all 5 connectors in smackerel.yaml): …` DoD bullet at the end of Scope 1 DoD in `specs/019-connector-wiring/scopes.md` with raw evidence (`tests/integration/test_connector_wiring.sh` 32/32 PASS) and source pointers (`config/smackerel.yaml`, `scripts/commands/config.sh`).
2. Append a `Scenario SCN-019-005 (Health endpoint shows all 15 connectors): …` DoD bullet at the end of Scope 1 DoD in `specs/019-connector-wiring/scopes.md` with raw evidence (`internal/api/health_test.go::TestHealthHandler_ConnectorHealth` PASS) and source pointers (`internal/api/health.go::ConnectorHealthLister`).
3. Run `bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring/bugs/BUG-019-002-scn-019-004-005-fidelity-gap` and `bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring`; expect PASS on both.
4. Run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/019-connector-wiring`; expect `RESULT: PASSED (0 warnings)` with `DoD fidelity: 6 mapped, 0 unmapped`.
5. Re-run the 2 underlying behavior tests to confirm no green→red drift: `bash tests/integration/test_connector_wiring.sh` (expect `SCN-019-004: PASS`) and `go test -count=1 -run TestHealthHandler_ConnectorHealth ./internal/api/...` (expect `PASS`).

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-BUG-019-002-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 6 mapped, 0 unmapped` | SCN-019-002-FIX-001 |
| T-BUG-019-002-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/019-connector-wiring` | SCN-019-002-FIX-001 |
| T-BUG-019-002-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/019-connector-wiring/bugs/BUG-019-002-scn-019-004-005-fidelity-gap` | SCN-019-002-FIX-001 |
| T-BUG-019-002-1-04 | SCN-019-004 underlying test still PASS | integration | `tests/integration/test_connector_wiring.sh` | exit 0, last line `SCN-019-004: PASS`, 32/32 PASS | SCN-019-002-FIX-001 |
| T-BUG-019-002-1-05 | SCN-019-005 underlying test still PASS | unit | `internal/api/health_test.go` | `TestHealthHandler_ConnectorHealth` PASS at `go test -count=1` | SCN-019-002-FIX-001 |
| T-BUG-019-002-1-06 | Production source unchanged | artifact | `git diff --name-only` | zero entries under `internal/`, `cmd/`, `ml/`, `config/`, `scripts/`, `tests/` | SCN-019-002-FIX-001 |

### Definition of Done

- [x] Scope 1 DoD in parent `scopes.md` contains a `Scenario SCN-019-004 (Config entries exist for all 5 connectors in smackerel.yaml): …` bullet with inline raw evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-019-004" specs/019-connector-wiring/scopes.md` returns the new DoD bullet at the end of Scope 1 DoD; bullet cites `tests/integration/test_connector_wiring.sh` 32/32 PASS plus `connectors.discord/twitter/weather/gov-alerts/financial-markets` entries in `config/smackerel.yaml`.
- [x] Scope 1 DoD in parent `scopes.md` contains a `Scenario SCN-019-005 (Health endpoint shows all 15 connectors): …` bullet with inline raw evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-019-005" specs/019-connector-wiring/scopes.md` returns the new DoD bullet; bullet cites `internal/api/health.go::ConnectorHealthLister` and `internal/api/health_test.go::TestHealthHandler_ConnectorHealth` PASS.
- [x] No pre-existing DoD bullet in parent `scopes.md` is deleted, weakened, or rewritten — **Phase:** implement
  > Evidence: `git diff specs/019-connector-wiring/scopes.md` shows only insertions (2 added lines, 0 deletions, 0 modifications to pre-existing bullets).
- [x] Traceability-guard PASSES against parent spec — **Phase:** validate
  > Evidence: see `report.md` `### Post-fix Validation Evidence`. Pre-fix `RESULT: FAILED (3 failures, 0 warnings)`; post-fix `RESULT: PASSED (0 warnings)` with `DoD fidelity: 6 scenarios checked, 6 mapped to DoD, 0 unmapped`.
- [x] Underlying behavior tests still PASS (no green→red drift) — **Phase:** test
  > Evidence: `bash tests/integration/test_connector_wiring.sh` returns 32/32 PASS, exit 0, last line `SCN-019-004: PASS`; `go test -count=1 -run TestHealthHandler_ConnectorHealth ./internal/api/...` returns `PASS` at exit 0. Both runs captured in `report.md` `### Test Evidence`.
- [x] Bug-packet artifact-lint PASSES — **Phase:** validate
  > Evidence: `bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring/bugs/BUG-019-002-scn-019-004-005-fidelity-gap` returns exit 0; captured in `report.md` `### Audit Evidence`.
- [x] Parent artifact-lint PASSES — **Phase:** validate
  > Evidence: `bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring` returns exit 0; captured in `report.md` `### Audit Evidence`.
- [x] Production source unchanged (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` shows zero entries under `internal/`, `cmd/`, `ml/`, `config/`, `scripts/`, `tests/`; only `specs/019-connector-wiring/scopes.md` and `specs/019-connector-wiring/bugs/BUG-019-002-scn-019-004-005-fidelity-gap/*` appear. Captured in `report.md` `### Audit Evidence`.
- [x] SCN-019-002-FIX-001: Trace guard accepts SCN-019-004 and SCN-019-005 as faithfully covered (post-fix `RESULT: PASSED`, 0 warnings, 6 mapped) — **Phase:** validate
  > Evidence: `bash .github/bubbles/scripts/traceability-guard.sh specs/019-connector-wiring` returns `RESULT: PASSED (0 warnings)`; `DoD fidelity: 6 scenarios checked, 6 mapped to DoD, 0 unmapped`. Captured verbatim in `report.md` `### Post-fix Validation Evidence`.
