# Scopes: BUG-019-001 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 019

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-019-FIX-001 Trace guard accepts SCN-019-002/003/004/005 as faithfully covered
  Given specs/019-connector-wiring/scopes.md Test Plan rows for SCN-019-002/003/004 carry concrete test file paths
  And specs/019-connector-wiring/scopes.md DoD entries name SCN-019-002 and SCN-019-003 by ID with raw go test evidence
  And specs/019-connector-wiring/report.md references internal/api/health_test.go by full relative path
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/019-connector-wiring`
  Then Gate G068 reports "6 scenarios checked, 6 mapped to DoD, 0 unmapped"
  And the overall result is PASSED
```

### Implementation Plan

1. Replace 3 wildcard `*_test.go` Test Plan rows in `specs/019-connector-wiring/scopes.md` with concrete file paths so `extract_path_candidates()` succeeds for SCN-019-002, SCN-019-003, and SCN-019-004.
2. Append a `Scenario SCN-019-002` DoD bullet to Scope 1 DoD with raw `go test` output for `TestConnect_ValidConfig`, `TestConnect_MissingToken`, `TestConnector_GatewayStartsOnConnectWithEnabledFlag` and source pointers to `internal/connector/discord/discord.go::Connect` and `cmd/core/connectors.go::registerConnectors`.
3. Append a `Scenario SCN-019-003` DoD bullet to Scope 1 DoD with raw `go test` output for the 7 missing-credential tests across `twitter`, `markets`, `alerts`, `discord` and source pointers to the `fmt.Errorf` lines and the per-connector auto-start blocks.
4. Append a "BUG-019-001 — DoD Scenario Fidelity Gap" section to `specs/019-connector-wiring/report.md` that spells `internal/api/health_test.go` verbatim and embeds the raw `go test` evidence for all 14 underlying behavior tests.
5. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and bug folder; run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/019-connector-wiring` and confirm PASS.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 6 mapped, 0 unmapped` | SCN-019-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/019-connector-wiring` | SCN-019-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/019-connector-wiring/bugs/BUG-019-001-dod-scenario-fidelity-gap` | SCN-019-FIX-001 |
| T-FIX-1-04 | Underlying behavior tests still pass | unit | `internal/connector/discord/discord_test.go`, `internal/connector/discord/gateway_test.go`, `internal/connector/twitter/twitter_test.go`, `internal/connector/markets/markets_test.go`, `internal/connector/alerts/alerts_test.go`, `internal/api/health_test.go`, `cmd/core/main_test.go` | The 14 named tests for SCN-019-002/003/005 + `TestAllConnectorsRegistered` PASS | SCN-019-FIX-001 |

### Definition of Done

- [x] Test Plan rows in parent `scopes.md` for SCN-019-002, SCN-019-003, SCN-019-004 contain concrete file paths matching the guard's path regex — **Phase:** implement
  > Evidence: `grep -nE 'internal/connector/(discord|twitter|markets|alerts)/[a-z_]+\.go|tests/integration/test_connector_wiring\.sh' specs/019-connector-wiring/scopes.md` returns the new rows with explicit paths.
- [x] Scope 1 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-019-002` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-019-002" specs/019-connector-wiring/scopes.md` returns the new DoD bullet at the bottom of Scope 1 DoD; full raw test output recorded inline.
- [x] Scope 1 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-019-003` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-019-003" specs/019-connector-wiring/scopes.md` returns the new DoD bullet; full raw test output recorded inline.
- [x] `specs/019-connector-wiring/report.md` references `internal/api/health_test.go` by full relative path — **Phase:** implement
  > Evidence: `grep -n "internal/api/health_test.go" specs/019-connector-wiring/report.md` returns matches in the new BUG-019-001 section.
- [x] Underlying behavior tests still pass — **Phase:** test
  > Evidence:
  > ```
  > $ go test -count=1 -v -run TestHealthHandler_ConnectorHealth ./internal/api/
  > === RUN   TestHealthHandler_ConnectorHealth
  > --- PASS: TestHealthHandler_ConnectorHealth (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/api     0.065s
  > ```
  > ```
  > $ go test -count=1 -v -run TestAllConnectorsRegistered ./cmd/core/
  > === RUN   TestAllConnectorsRegistered
  > --- PASS: TestAllConnectorsRegistered (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/cmd/core 0.058s
  > ```
  > ```
  > $ go test -count=1 -v -run 'TestConnect_ValidConfig$|TestConnect_MissingToken$|TestConnector_GatewayStartsOnConnectWithEnabledFlag$' ./internal/connector/discord/
  > === RUN   TestConnect_MissingToken
  > --- PASS: TestConnect_MissingToken (0.00s)
  > === RUN   TestConnect_ValidConfig
  > --- PASS: TestConnect_ValidConfig (0.03s)
  > === RUN   TestConnector_GatewayStartsOnConnectWithEnabledFlag
  > --- PASS: TestConnector_GatewayStartsOnConnectWithEnabledFlag (0.03s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/discord       0.074s
  > ```
  > ```
  > $ go test -count=1 -v -run 'TestConnect_APIModeRequiresBearerToken$|TestConnect_InvalidSyncMode$|TestConnect_MissingAPIKey$|TestConnect_SetsHealthErrorOnInvalidConfig$|TestConnect_NoLocations$|TestParseAlertsConfig_InvalidCoordinates$' ./internal/connector/twitter/ ./internal/connector/markets/ ./internal/connector/alerts/
  > === RUN   TestConnect_InvalidSyncMode
  > --- PASS: TestConnect_InvalidSyncMode (0.00s)
  > === RUN   TestConnect_APIModeRequiresBearerToken
  > --- PASS: TestConnect_APIModeRequiresBearerToken (0.00s)
  > ok      github.com/smackerel/smackerel/internal/connector/twitter       0.118s
  > === RUN   TestConnect_MissingAPIKey
  > --- PASS: TestConnect_MissingAPIKey (0.00s)
  > === RUN   TestConnect_SetsHealthErrorOnInvalidConfig
  > --- PASS: TestConnect_SetsHealthErrorOnInvalidConfig (0.00s)
  > ok      github.com/smackerel/smackerel/internal/connector/markets       0.059s
  > === RUN   TestConnect_NoLocations
  > --- PASS: TestConnect_NoLocations (0.00s)
  > === RUN   TestParseAlertsConfig_InvalidCoordinates
  > --- PASS: TestParseAlertsConfig_InvalidCoordinates (0.00s)
  > ok      github.com/smackerel/smackerel/internal/connector/alerts        0.067s
  > ```
- [x] Traceability-guard PASSES against `specs/019-connector-wiring` — **Phase:** validate
  > Evidence: see report.md `### Post-fix Validation`. Final lines:
  > ```
  > ℹ️  DoD fidelity: 6 scenarios checked, 6 mapped to DoD, 0 unmapped
  > ℹ️  Concrete test file references: 6
  > ℹ️  Report evidence references: 6
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see bug `report.md > Audit Evidence` for both runs.
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/019-connector-wiring/scopes.md`, `specs/019-connector-wiring/report.md`, and `specs/019-connector-wiring/bugs/BUG-019-001-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/`, `scripts/`, or `tests/` were touched.
