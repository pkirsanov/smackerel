# Bug: BUG-019-001 — DoD scenario fidelity gap (SCN-019-002/003/004/005)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 019 — Connector Wiring (Register 5 Unwired Connectors)
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed (artifact-only)

## Problem Statement

Bubbles traceability-guard reported 7 failures against `specs/019-connector-wiring`:

1. **G068 DoD fidelity (×2):** `SCN-019-002` and `SCN-019-003` had no faithful matching DoD item — the existing DoD bullets described the delivered behavior but did not embed the `SCN-019-NNN` trace ID and did not share enough significant words to satisfy the fuzzy fallback.
2. **Concrete test path (×3):** Test Plan rows for `SCN-019-002`, `SCN-019-003`, and `SCN-019-004` used wildcard paths (`internal/connector/discord/*_test.go`, `internal/connector/twitter/*_test.go`) which the guard's path-extraction regex `([A-Za-z0-9_.-]+/)+[A-Za-z0-9_.-]+\.[A-Za-z0-9_.-]+` does not match. The guard therefore could not resolve any concrete file for those scenarios.
3. **Missing report evidence reference (×1):** `SCN-019-005` mapped to `internal/api/health_test.go` correctly, but the parent `report.md` never mentioned that path verbatim, so `report_mentions_path` failed.
4. **Aggregate G068 message (×1):** the guard emits a single summary failure when any scenario is unmapped to DoD.

## Reproduction (Pre-fix)

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

## Gap Analysis (per scenario)

For each flagged scenario, the bug investigator inspected the production wiring (`cmd/core/connectors.go`, `internal/connector/{discord,twitter,markets,alerts}/*.go`, `internal/api/health.go`) and the existing test files. All four scenarios are genuinely **delivered-but-undocumented at the trace-ID/concrete-path level** — no production code is missing, no test fixture is missing; the only gap is documentation linkage.

| Scenario | Behavior delivered? | Tests pass? | Concrete test file | Concrete source |
|---|---|---|---|---|
| SCN-019-002 Enabling Discord connector makes it operational | Yes — auto-start block at `cmd/core/connectors.go:205` (`if cfg.DiscordEnabled`) calls `Connect()` and `supervisor.StartConnector()` | Yes — `TestConnect_ValidConfig`, `TestConnect_MissingToken`, `TestConnector_GatewayStartsOnConnectWithEnabledFlag` PASS | `internal/connector/discord/discord_test.go`, `internal/connector/discord/gateway_test.go` | `internal/connector/discord/discord.go::Connect`, `cmd/core/connectors.go::registerConnectors` |
| SCN-019-003 Missing credentials produce clear startup errors | Yes — `Connect()` returns descriptive errors when required credentials are empty (`finnhub_api_key is required`, `discord bot_token is required`, etc.); other connectors are unaffected because each auto-start block logs and continues | Yes — `TestConnect_MissingAPIKey`, `TestConnect_NoLocations`, `TestConnect_APIModeRequiresBearerToken`, `TestConnect_MissingToken`, `TestParseAlertsConfig_InvalidCoordinates`, `TestConnect_InvalidSyncMode`, `TestConnect_SetsHealthErrorOnInvalidConfig` PASS | `internal/connector/discord/discord_test.go`, `internal/connector/twitter/twitter_test.go`, `internal/connector/markets/markets_test.go`, `internal/connector/alerts/alerts_test.go` | `internal/connector/markets/markets.go:172`, `internal/connector/discord/discord.go:254`, `cmd/core/connectors.go:205,230,251,273,308` |
| SCN-019-004 Config entries exist for all 5 connectors in smackerel.yaml | Yes — `config/smackerel.yaml` lines 263, 277, 284, 295, 318 define `discord/twitter/weather/gov-alerts/financial-markets`; each defaults to `enabled: false` | Yes — `tests/integration/test_connector_wiring.sh` (32 assertions) PASS | `tests/integration/test_connector_wiring.sh` | `config/smackerel.yaml`, `scripts/commands/config.sh` |
| SCN-019-005 Health endpoint shows all 15 connectors | Yes — `internal/api/health.go` defines `ConnectorHealthLister`; the registry holding all 15 connector instances is wired into the handler via `health.go:93` | Yes — `TestHealthHandler_ConnectorHealth` PASS | `internal/api/health_test.go` | `internal/api/health.go::ConnectorHealthLister` |

**Disposition:** All four scenarios are **delivered-but-undocumented** — artifact-only fix.

## Acceptance Criteria

- [x] Parent `specs/019-connector-wiring/scopes.md` Test Plan rows for `SCN-019-002`, `SCN-019-003`, `SCN-019-004` carry concrete test file paths matching the guard's path regex
- [x] Parent `specs/019-connector-wiring/scopes.md` Scope 1 DoD has trace-ID-bearing bullets for `Scenario SCN-019-002` and `Scenario SCN-019-003` with inline raw `go test` evidence
- [x] Parent `specs/019-connector-wiring/report.md` references `internal/api/health_test.go` by full relative path
- [x] Parent `specs/019-connector-wiring/scenario-manifest.json` already covers all 6 scenarios (no change needed; verified)
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring/bugs/BUG-019-001-dod-scenario-fidelity-gap` PASS
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/019-connector-wiring` PASS
- [x] No production code changed (boundary)
