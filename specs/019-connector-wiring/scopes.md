# Scopes: 019 Connector Wiring — Register 5 Unwired Connectors

**Feature:** 019-connector-wiring
**Created:** 2026-04-10
**Status:** Planning complete

---

## Execution Outline

### Phase Order

1. **Scope 1: Wire All 5 Connectors** — Register Discord, Twitter/X, Weather, Gov Alerts, and Financial Markets in `main.go`, add YAML config blocks for 4 missing connectors, extend `config.sh` env var extraction, verify all 14 connectors appear in health endpoint.

### New Types & Signatures

- No new types — uses existing `connector.ConnectorConfig`, `connector.New()`, `registry.Register()`
- 5 new import lines in `cmd/core/main.go` (discord, twitter, weather, alerts, markets)
- 5 new instantiation + 5 registration lines
- 5 new conditional-start blocks with `ConnectorConfig` assembly
- 4 new YAML config blocks in `config/smackerel.yaml` (Twitter, Weather, Gov Alerts, Financial Markets)
- Helper functions in `main.go`: `parseJSONArray`, `parseJSONObject`, `parseFloatEnv`, `parseIntEnv`, `splitCSV`

### Validation Checkpoints

- After Scope 1: `./smackerel.sh test unit` passes, `./smackerel.sh config generate` produces env vars for all 5 connectors, health endpoint lists 14 connectors.

---

## Scope Summary

| # | Name | Surfaces | Tests | DoD Summary | Status |
|---|------|----------|-------|-------------|--------|
| 1 | Wire All 5 Connectors | Go core (`main.go`), Config (`smackerel.yaml`, `config.sh`), Generated env | Unit, Integration, E2E-API | All 14 connectors registered, config entries exist, health reports all 14 | In Progress |

---

## Scope 1: Wire All 5 Connectors

**Status:** [x] In Progress

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-019-001 All 14 connectors registered at startup
  Given Smackerel core starts with default configuration (all connectors disabled)
  When the supervisor registry is inspected
  Then all 14 connectors are registered
  And none of the 5 newly wired connectors are in the running state

Scenario: SCN-019-002 Enabling Discord connector makes it operational
  Given smackerel.yaml has connectors.discord.enabled=true with valid bot_token
  When Smackerel core starts
  Then the Discord connector is registered, connected, and started via the supervisor

Scenario: SCN-019-003 Missing credentials produce clear startup errors
  Given the Financial Markets connector is enabled but finnhub_api_key is empty
  When the connector attempts to connect
  Then it fails with a descriptive error about missing API key
  And other connectors continue operating normally

Scenario: SCN-019-004 Config entries exist for all 5 connectors in smackerel.yaml
  Given config/smackerel.yaml is the single source of truth
  When the connectors section is inspected
  Then entries exist for discord, twitter, weather, gov-alerts, and financial-markets
  And each entry has enabled: false as default

Scenario: SCN-019-005 Health endpoint shows all 14 connectors
  Given all 14 connectors are registered
  When GET /api/health is called
  Then the response includes health status for all 14 connectors
  And disabled connectors show status "disconnected"

Scenario: SCN-019-006 Existing connectors unaffected by new registrations
  Given the existing 9 connectors operate correctly
  When the 5 new connectors are registered alongside them
  Then no existing connector changes behavior, start order, or health status
```

### Implementation Plan

**Components/files touched:**

| File | Change |
|------|--------|
| `cmd/core/main.go` | Add 5 imports, 5 instantiations, 5 registrations, 5 auto-start blocks, helper functions (`parseJSONArray`, `parseJSONObject`, `parseFloatEnv`, `parseIntEnv`, `splitCSV`) |
| `config/smackerel.yaml` | Add 4 new YAML config blocks under `connectors:` (twitter, weather, gov-alerts, financial-markets) |
| `scripts/commands/config.sh` | Add `yaml_get` extractions for all 5 connectors, write env vars to generated env files |

**Registration pattern (per design.md):**
- Import: `discordConnector "github.com/smackerel/smackerel/internal/connector/discord"`
- Instantiate: `discordConn := discordConnector.New("discord")`
- Register: `registry.Register(discordConn)`
- Auto-start: env-var gated → build `ConnectorConfig` → `Connect()` → `supervisor.StartConnector()`

**Per-connector config mapping:**
- Discord: `AuthType: "token"`, `Credentials: bot_token`, gateway/threads/pins in SourceConfig
- Twitter: `AuthType: "token"`, `Credentials: bearer_token`, sync_mode/archive_dir in SourceConfig
- Weather: `AuthType: "none"`, locations JSON in SourceConfig
- Gov Alerts: `AuthType: "none"`, locations + min_earthquake_magnitude in SourceConfig
- Financial Markets: `AuthType: "api_key"`, `Credentials: finnhub_api_key/fred_api_key`, watchlist/alert_threshold in SourceConfig

**SST compliance:** All values from `smackerel.yaml` → `config generate` → env vars → `os.Getenv()`. Zero hardcoded defaults.

### Test Plan

| Type | File/Location | Purpose | Scenarios Covered |
|------|---------------|---------|-------------------|
| Unit | `cmd/core/main_test.go` | Verify all 14 connectors present in registry after init | SCN-019-001, SCN-019-006 |
| Unit | Existing `internal/connector/discord/*_test.go` | Discord config parsing, Connect validation | SCN-019-002, SCN-019-003 |
| Unit | Existing `internal/connector/twitter/*_test.go` | Twitter config parsing, archive/API mode validation | SCN-019-003, SCN-019-004 |
| Unit | Existing `internal/connector/weather/*_test.go` | Weather location validation | SCN-019-003 |
| Unit | Existing `internal/connector/alerts/*_test.go` | Gov Alerts coordinate/radius validation | SCN-019-003 |
| Unit | Existing `internal/connector/markets/*_test.go` | Financial Markets API key + watchlist validation | SCN-019-003 |
| Integration | `tests/integration/connector_wiring_test.go` | Config generate produces env vars for all 5 connectors | SCN-019-004 |
| E2E-API | `tests/e2e/health_connectors_test.go` | `GET /api/health` lists all 14 connectors with correct status | SCN-019-005 |
| Regression | Existing connector unit + integration tests | All 9 existing connectors pass unchanged | SCN-019-006 |

### Definition of Done

- [x] All 5 connectors imported, instantiated, and registered in `cmd/core/main.go`
- [x] 5 conditional-start blocks follow exact pattern of existing 9 connectors
- [x] 4 new YAML config blocks added to `config/smackerel.yaml` (Discord already exists)
- [x] `scripts/commands/config.sh` extracts all new env vars via `yaml_get`
- [x] `./smackerel.sh config generate` produces env vars for all 5 connectors in `config/generated/dev.env`
- [x] `./smackerel.sh test unit` passes — all existing + new tests green
- [ ] Health endpoint lists all 14 connectors
- [x] No hardcoded fallback defaults anywhere (SST policy)
- [x] Empty credentials cause `Connect()` to fail with descriptive error, not silent fallback
- [x] All 5 connectors default to `enabled: false`
- [x] Existing 9 connectors are completely unaffected (regression tests pass)
- [x] Helper functions (`parseJSONArray`, `parseJSONObject`, `parseFloatEnv`) added to `main.go`
