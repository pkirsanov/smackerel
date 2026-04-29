# Report: BUG-019-001 — DoD Scenario Fidelity Gap

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Traceability-guard reported 7 failures against `specs/019-connector-wiring` (a feature already marked `done`): two G068 DoD-fidelity gaps for `SCN-019-002` and `SCN-019-003`, three "no concrete test file path" failures for `SCN-019-002`/`003`/`004` Test Plan rows that used wildcard `*_test.go` paths, one missing `report.md` reference for `internal/api/health_test.go` (SCN-019-005), and the aggregate G068 summary failure. Investigation confirmed the gap is artifact-only — every scenario is fully delivered in production code (`cmd/core/connectors.go`, `internal/connector/{discord,twitter,markets,alerts}/*.go`, `internal/api/health.go`) and exercised by passing unit and integration tests. The DoD bullets simply did not embed the `SCN-019-NNN` trace IDs, the Test Plan rows used wildcard paths the guard's path-extraction regex does not match, and the parent `report.md` never spelled `internal/api/health_test.go` verbatim.

The fix added 2 trace-ID-bearing DoD bullets to `specs/019-connector-wiring/scopes.md` (`Scenario SCN-019-002` and `Scenario SCN-019-003` with raw `go test` output and source pointers), replaced the 3 wildcard Test Plan rows with concrete file-path rows, and appended a "BUG-019-001 — DoD Scenario Fidelity Gap" cross-reference section to `specs/019-connector-wiring/report.md` that spells `internal/api/health_test.go` verbatim. No production code was modified; the boundary clause in the user prompt was honored.

## Completion Statement

All 8 DoD items in `scopes.md` Scope 1 are checked `[x]` with inline raw evidence. The traceability-guard's pre-fix state (4 unmapped/path-broken scenarios + 1 missing report path + aggregate, 7 failures) has been replaced with a clean `RESULT: PASSED (0 warnings)` post-fix. Both `artifact-lint.sh` invocations (parent and bug folder) succeed. The 14 underlying behavior tests for the previously-flagged scenarios still pass with no regressions.

## Test Evidence

### Underlying behavior tests (regression-protection for the artifact fix)

```
$ go test -count=1 -v -run TestHealthHandler_ConnectorHealth ./internal/api/
=== RUN   TestHealthHandler_ConnectorHealth
--- PASS: TestHealthHandler_ConnectorHealth (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/api     0.065s
```

```
$ go test -count=1 -v -run TestAllConnectorsRegistered ./cmd/core/
=== RUN   TestAllConnectorsRegistered
--- PASS: TestAllConnectorsRegistered (0.00s)
PASS
ok      github.com/smackerel/smackerel/cmd/core 0.058s
```

```
$ go test -count=1 -v -run 'TestConnect_ValidConfig$|TestConnect_MissingToken$|TestConnector_GatewayStartsOnConnectWithEnabledFlag$' ./internal/connector/discord/
=== RUN   TestConnect_MissingToken
--- PASS: TestConnect_MissingToken (0.00s)
=== RUN   TestConnect_ValidConfig
--- PASS: TestConnect_ValidConfig (0.03s)
=== RUN   TestConnector_GatewayStartsOnConnectWithEnabledFlag
--- PASS: TestConnector_GatewayStartsOnConnectWithEnabledFlag (0.03s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/discord       0.074s
```

```
$ go test -count=1 -v -run 'TestConnect_APIModeRequiresBearerToken$|TestConnect_InvalidSyncMode$|TestConnect_MissingAPIKey$|TestConnect_SetsHealthErrorOnInvalidConfig$|TestConnect_NoLocations$|TestParseAlertsConfig_InvalidCoordinates$' ./internal/connector/twitter/ ./internal/connector/markets/ ./internal/connector/alerts/
=== RUN   TestConnect_InvalidSyncMode
--- PASS: TestConnect_InvalidSyncMode (0.00s)
=== RUN   TestConnect_APIModeRequiresBearerToken
--- PASS: TestConnect_APIModeRequiresBearerToken (0.00s)
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.118s
=== RUN   TestConnect_MissingAPIKey
--- PASS: TestConnect_MissingAPIKey (0.00s)
=== RUN   TestConnect_SetsHealthErrorOnInvalidConfig
--- PASS: TestConnect_SetsHealthErrorOnInvalidConfig (0.00s)
ok      github.com/smackerel/smackerel/internal/connector/markets       0.059s
=== RUN   TestConnect_NoLocations
--- PASS: TestConnect_NoLocations (0.00s)
=== RUN   TestParseAlertsConfig_InvalidCoordinates
--- PASS: TestParseAlertsConfig_InvalidCoordinates (0.00s)
ok      github.com/smackerel/smackerel/internal/connector/alerts        0.067s
```

**Claim Source:** executed.

### Validation Evidence

> Phase agent: bubbles.validate
> Executed: YES

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/019-connector-wiring 2>&1 | tail -10
ℹ️  DoD fidelity: 6 scenarios checked, 6 mapped to DoD, 0 unmapped
ℹ️  Concrete test file references: 6
ℹ️  Report evidence references: 6
RESULT: PASSED (0 warnings)
```

**Claim Source:** executed. Pre-fix run on the same revision (with the unfixed artifacts) reported `RESULT: FAILED (7 failures, 0 warnings)` including `DoD fidelity: 6 scenarios checked, 4 mapped to DoD, 2 unmapped` — see Section "Pre-fix Reproduction" below.

### Audit Evidence

> Phase agent: bubbles.audit
> Executed: YES

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring 2>&1 | tail -3
=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring/bugs/BUG-019-001-dod-scenario-fidelity-gap 2>&1 | tail -3
=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

```
$ git diff --name-only
specs/019-connector-wiring/bugs/BUG-019-001-dod-scenario-fidelity-gap/design.md
specs/019-connector-wiring/bugs/BUG-019-001-dod-scenario-fidelity-gap/report.md
specs/019-connector-wiring/bugs/BUG-019-001-dod-scenario-fidelity-gap/scopes.md
specs/019-connector-wiring/bugs/BUG-019-001-dod-scenario-fidelity-gap/spec.md
specs/019-connector-wiring/bugs/BUG-019-001-dod-scenario-fidelity-gap/state.json
specs/019-connector-wiring/bugs/BUG-019-001-dod-scenario-fidelity-gap/uservalidation.md
specs/019-connector-wiring/report.md
specs/019-connector-wiring/scopes.md
```

**Claim Source:** executed. Boundary preserved: zero changes under `internal/`, `cmd/`, `ml/`, `config/`, `scripts/`, `tests/`, or any other production-code path.

## Pre-fix Reproduction

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/019-connector-wiring 2>&1 | tail -20
❌ Scope 1: Wire All 5 Connectors mapped row has no concrete test file path: SCN-019-002 Enabling Discord connector makes it operational
❌ Scope 1: Wire All 5 Connectors mapped row has no concrete test file path: SCN-019-003 Missing credentials produce clear startup errors
❌ Scope 1: Wire All 5 Connectors mapped row has no concrete test file path: SCN-019-004 Config entries exist for all 5 connectors in smackerel.yaml
❌ Scope 1: Wire All 5 Connectors report is missing evidence reference for concrete test file: internal/api/health_test.go
❌ Scope 1: Wire All 5 Connectors Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-019-002 Enabling Discord connector makes it operational
❌ Scope 1: Wire All 5 Connectors Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-019-003 Missing credentials produce clear startup errors
❌ DoD content fidelity gap: 2 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)
RESULT: FAILED (7 failures, 0 warnings)
```

**Claim Source:** executed (initial guard invocation captured at the start of this bug investigation, before any artifact edits).
