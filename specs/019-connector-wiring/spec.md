# Spec: Connector Wiring — Register 5 Unwired Connectors

**Feature:** 019-connector-wiring
**Status:** Draft
**Created:** 2026-04-10

---

## Problem Statement

Smackerel has 14 connector packages under `internal/connector/`, but only 9 are registered in `cmd/core/main.go`. Five connectors — Discord, Twitter/X, Weather, Gov Alerts (USGS), and Financial Markets (Finnhub/CoinGecko) — have fully implemented code packages with tests but are **never instantiated or registered** with the connector supervisor. They are dead code that cannot be reached at runtime.

Additionally, four of these five connectors (Twitter/X, Weather, Gov Alerts, Financial Markets) have no configuration entries in `config/smackerel.yaml`, so even with registration they would have no config-driven enable/disable surface. Discord already has a YAML entry but is still never instantiated.

This is a wiring-only fix. No new connector logic is needed — only registration in `main.go` and config entries in `smackerel.yaml`.

### Findings Addressed

| ID | Severity | Description |
|----|----------|-------------|
| PRD-001 | Critical | 5 connectors exist as code but are never registered with the supervisor |
| ENG-003 | Medium | 5 connector import paths in `main.go` that are never used (Discord, Twitter, Weather, Alerts, Markets have no import) |
| ENG-019 | Low | Unreachable connector packages create confusion about what is actually active |

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| Self-Hoster | Individual running Smackerel locally via Docker Compose | Enable/disable connectors via config; have all advertised connectors reachable at runtime | Full system access |
| Connector Supervisor | Internal runtime component managing connector lifecycle | Register, start, stop, and health-check all connectors | System-internal |
| Operator | Person monitoring system health via `/api/health` | See all registered connectors and their health status | Read-only API access |

---

## Outcome Contract

**Intent:** All 14 connector packages are reachable at runtime — the 5 currently dead connectors (Discord, Twitter/X, Weather, Gov Alerts, Financial Markets) are registered with the supervisor and configurable via `smackerel.yaml`, following the same pattern as the existing 9 connectors.

**Success Signal:** When `enabled: true` is set for any of the 5 connectors in `smackerel.yaml` with valid credentials/config, the connector starts, syncs, and reports health through the supervisor — identical to the existing 9 connectors.

**Hard Constraints:**
- All connectors default to `enabled: false` — no connector auto-starts without explicit opt-in
- All config values originate from `config/smackerel.yaml` — zero hardcoded defaults (SST policy)
- Existing 9 connectors are unaffected — no behavioral regression
- No new connector logic — only registration, instantiation, and config wiring

**Failure Condition:** Any of the 5 connectors remains unreachable at runtime after this work, or enabling one causes a regression in existing connectors.

---

## Use Cases

### UC-001: Enable Discord Connector

- **Actor:** Self-Hoster
- **Preconditions:** Smackerel is deployed; Discord bot token obtained from Developer Portal
- **Main Flow:**
  1. Self-Hoster sets `connectors.discord.enabled: true` and `connectors.discord.bot_token` in `smackerel.yaml`
  2. Self-Hoster runs `./smackerel.sh config generate` then `./smackerel.sh up`
  3. Core runtime loads Discord config from environment
  4. Core instantiates Discord connector and registers with supervisor
  5. Supervisor starts Discord connector; connector authenticates with Discord API
  6. Connector syncs monitored channels and reports healthy status
- **Alternative Flows:**
  - A1: `enabled: false` → connector is instantiated + registered but not started
  - A2: `bot_token` empty → connector fails connect with clear error, supervisor marks degraded
- **Postconditions:** Discord connector appears in `/api/health` with accurate status

### UC-002: Enable Twitter/X Connector

- **Actor:** Self-Hoster
- **Preconditions:** Smackerel is deployed; Twitter data archive exported OR API bearer token obtained
- **Main Flow:**
  1. Self-Hoster sets `connectors.twitter.enabled: true` and configures `sync_mode` + credentials in `smackerel.yaml`
  2. Config generate + restart
  3. Core instantiates Twitter connector, registers with supervisor
  4. Supervisor starts connector; connector processes archive or polls API
  5. Connector syncs tweets/threads and reports healthy status
- **Alternative Flows:**
  - A1: `sync_mode: archive` with empty `archive_dir` → connector fails connect with clear error
  - A2: `enabled: false` → registered but not auto-started
- **Postconditions:** Twitter connector visible in health endpoint

### UC-003: Enable Weather Connector

- **Actor:** Self-Hoster
- **Preconditions:** Smackerel is deployed; at least one location configured with lat/lon
- **Main Flow:**
  1. Self-Hoster sets `connectors.weather.enabled: true` and adds location entries in `smackerel.yaml`
  2. Config generate + restart
  3. Core instantiates Weather connector, registers with supervisor
  4. Supervisor starts connector; connector polls Open-Meteo API for configured locations
  5. Connector syncs weather data and reports healthy status
- **Alternative Flows:**
  - A1: No locations configured → connector fails connect with "at least one location" error
  - A2: Open-Meteo unreachable → connector degrades gracefully per backoff policy
- **Postconditions:** Weather connector visible in health endpoint

### UC-004: Enable Gov Alerts Connector

- **Actor:** Self-Hoster
- **Preconditions:** Smackerel is deployed; at least one location configured
- **Main Flow:**
  1. Self-Hoster sets `connectors.gov-alerts.enabled: true` and adds locations in `smackerel.yaml`
  2. Config generate + restart
  3. Core instantiates Gov Alerts connector, registers with supervisor
  4. Supervisor starts connector; connector polls USGS earthquake API + NWS alerts
  5. Connector syncs alerts and reports healthy status
- **Alternative Flows:**
  - A1: No locations → fails connect with clear error
  - A2: USGS/NWS unreachable → degrades per backoff
- **Postconditions:** Gov Alerts connector visible in health endpoint

### UC-005: Enable Financial Markets Connector

- **Actor:** Self-Hoster
- **Preconditions:** Smackerel is deployed; Finnhub API key obtained (free tier available)
- **Main Flow:**
  1. Self-Hoster sets `connectors.financial-markets.enabled: true`, provides `finnhub_api_key`, and configures watchlist in `smackerel.yaml`
  2. Config generate + restart
  3. Core instantiates Markets connector, registers with supervisor
  4. Supervisor starts connector; connector polls Finnhub/CoinGecko for watchlist items
  5. Connector syncs market data and reports healthy status
- **Alternative Flows:**
  - A1: No `finnhub_api_key` → connector fails connect with clear error
  - A2: CoinGecko enabled without watchlist crypto entries → no crypto data, stocks still work
  - A3: `enabled: false` → registered but not auto-started
- **Postconditions:** Financial Markets connector visible in health endpoint

### UC-006: Verify All Connectors in Health Endpoint

- **Actor:** Operator
- **Preconditions:** Smackerel is running with all 14 connectors registered
- **Main Flow:**
  1. Operator queries `GET /api/health`
  2. Response includes all 14 connectors with their current health status
  3. Disabled connectors show as `disconnected`; enabled ones show `healthy`/`syncing`/`degraded`
- **Postconditions:** Complete visibility into connector fleet status

---

## Business Scenarios

### BS-001: All 14 connectors appear in supervisor registry after startup

```
Given Smackerel core starts with default configuration (all connectors disabled)
When the supervisor registry is inspected
Then all 14 connectors are registered
And none are in the "running" state (all disabled by default)
```

### BS-002: Enabling a previously-dead connector makes it operational

```
Given the Discord connector was previously dead code
When a Self-Hoster sets discord.enabled=true with valid bot_token and restarts
Then the Discord connector starts, syncs channels, and reports healthy
And its behavior is identical to any of the existing 9 connectors
```

### BS-003: Existing connectors are unaffected by new registrations

```
Given the existing 9 connectors operate correctly
When the 5 new connectors are registered alongside them
Then no existing connector changes behavior, start order, health status, or sync schedule
```

### BS-004: Missing credentials produce clear startup errors

```
Given a connector requires credentials (e.g., Discord bot_token, Finnhub API key)
When the connector is enabled but credentials are empty
Then the connector fails to connect with a descriptive error message
And the supervisor marks it as degraded/failing (not silently ignored)
And other connectors continue operating normally
```

### BS-005: Config entries for all 5 connectors exist in smackerel.yaml

```
Given smackerel.yaml is the single source of truth for configuration
When a Self-Hoster opens config/smackerel.yaml
Then entries exist for discord, twitter, weather, gov-alerts, and financial-markets
And each entry has enabled: false as default
And each entry documents required fields (tokens, locations, watchlists)
```

### BS-006: Weather connector starts with valid location config

```
Given the Weather connector is enabled with at least one location (name, lat, lon)
When Smackerel starts
Then the Weather connector connects successfully and begins syncing
And weather data for configured locations flows into the pipeline
```

### BS-007: Financial Markets connector handles multiple data sources

```
Given the Financial Markets connector is enabled with a Finnhub API key
When the watchlist includes stocks, ETFs, and crypto
Then stock/ETF quotes are fetched via Finnhub
And crypto prices are fetched via CoinGecko
And all data flows through the standard pipeline
```

### BS-008: Twitter/X archive import works end-to-end

```
Given the Twitter connector is enabled with sync_mode=archive and a valid archive_dir
When Smackerel starts
Then the connector reads the Twitter data export from the specified directory
And tweets and threads are ingested into the pipeline
```

### BS-009: Gov Alerts connector deduplicates repeated alerts

```
Given the Gov Alerts connector is running and has previously ingested an earthquake alert
When the same alert appears on the next sync cycle
Then the alert is not duplicated in the system
```

### BS-010: Disabled connectors consume zero runtime resources

```
Given a connector is registered but has enabled=false
When the system is running
Then no goroutines, HTTP connections, or API calls are made for that connector
And it appears as "disconnected" in the health endpoint
```

---

## Requirements

### R-001: Register all 5 connectors in main.go

The 5 missing connectors MUST be instantiated and registered with the supervisor in `cmd/core/main.go`, following the exact same pattern as the existing 9 connectors:

```gherkin
Scenario: Discord connector is registered at startup
  Given Smackerel core starts
  When the connector registry is queried
  Then a connector with ID "discord" is present

Scenario: Twitter connector is registered at startup
  Given Smackerel core starts
  When the connector registry is queried
  Then a connector with ID "twitter" is present

Scenario: Weather connector is registered at startup
  Given Smackerel core starts
  When the connector registry is queried
  Then a connector with ID "weather" is present

Scenario: Gov Alerts connector is registered at startup
  Given Smackerel core starts
  When the connector registry is queried
  Then a connector with ID "gov-alerts" is present

Scenario: Financial Markets connector is registered at startup
  Given Smackerel core starts
  When the connector registry is queried
  Then a connector with ID "financial-markets" is present
```

### R-002: Config-driven enable/disable for each connector

Each connector MUST be controlled by its `connectors.<name>.enabled` field in `smackerel.yaml`. Disabled connectors are registered but not started.

```gherkin
Scenario: Disabled connector is registered but not started
  Given smackerel.yaml has connectors.discord.enabled=false
  When Smackerel core starts
  Then the discord connector is registered in the supervisor
  But the discord connector is not in the running set
  And no Discord API calls are made

Scenario: Enabled connector is registered and started
  Given smackerel.yaml has connectors.discord.enabled=true
  And connectors.discord.bot_token is set to a valid token
  When Smackerel core starts
  Then the discord connector is registered
  And the discord connector is started via the supervisor
```

### R-003: Add missing config entries to smackerel.yaml

`config/smackerel.yaml` MUST contain entries for all 5 connectors under the `connectors:` section. Discord already has an entry; the remaining 4 need entries matching their connector's expected config structure.

```gherkin
Scenario: Twitter config entry exists in smackerel.yaml
  Given config/smackerel.yaml is loaded
  When the connectors section is inspected
  Then a "twitter" entry exists with enabled, sync_mode, archive_dir, bearer_token fields

Scenario: Weather config entry exists in smackerel.yaml
  Given config/smackerel.yaml is loaded
  When the connectors section is inspected
  Then a "weather" entry exists with enabled, sync_schedule, and locations fields

Scenario: Gov Alerts config entry exists in smackerel.yaml
  Given config/smackerel.yaml is loaded
  When the connectors section is inspected
  Then a "gov-alerts" entry exists with enabled, sync_schedule, locations, and min_earthquake_mag fields

Scenario: Financial Markets config entry exists in smackerel.yaml
  Given config/smackerel.yaml is loaded
  When the connectors section is inspected
  Then a "financial-markets" entry exists with enabled, finnhub_api_key, and watchlist fields
```

### R-004: Wire connector-specific config to ConnectorConfig

Each connector's YAML config MUST be mapped to the appropriate `connector.ConnectorConfig` fields (AuthType, Credentials, SourceConfig, etc.) following the patterns established by existing connectors.

```gherkin
Scenario: Discord config maps bot_token to credentials
  Given smackerel.yaml has connectors.discord.bot_token="tok_abc"
  When the Discord connector is instantiated
  Then ConnectorConfig.AuthType is "token"
  And ConnectorConfig.Credentials["bot_token"] equals "tok_abc"
  And ConnectorConfig.SourceConfig contains monitored_channels, enable_gateway, etc.

Scenario: Weather config maps locations to source_config
  Given smackerel.yaml has weather locations configured
  When the Weather connector is instantiated
  Then ConnectorConfig.AuthType is "none"
  And ConnectorConfig.SourceConfig["locations"] contains the configured locations

Scenario: Financial Markets config maps API keys to credentials
  Given smackerel.yaml has financial-markets.finnhub_api_key="fk_123"
  When the Financial Markets connector is instantiated
  Then ConnectorConfig.AuthType is "api_key"
  And ConnectorConfig.Credentials["finnhub_api_key"] equals "fk_123"
```

### R-005: Health reporting for all 14 connectors

All connectors, including the 5 newly wired ones, MUST report health through the supervisor and be visible in the health API endpoint.

```gherkin
Scenario: Health endpoint shows all 14 connectors
  Given all 14 connectors are registered
  When GET /api/health is called
  Then the response includes health status for all 14 connectors
  And disabled connectors show status "disconnected"
  And enabled + healthy connectors show status "healthy"
```

### R-006: No regression in existing connectors

The registration of the 5 new connectors MUST NOT alter the behavior of the existing 9 connectors.

```gherkin
Scenario: Existing Gmail connector unaffected
  Given the 5 new connectors are registered
  When a Google OAuth token is present
  Then the Gmail connector starts and syncs exactly as before

Scenario: Existing Bookmarks connector unaffected
  Given the 5 new connectors are registered
  And BOOKMARKS_IMPORT_DIR is set
  When Smackerel starts
  Then the Bookmarks connector auto-starts with file-based import exactly as before
```

### R-007: Connector startup follows SST zero-defaults policy

No hardcoded fallback values for any connector config. All values come from `smackerel.yaml` via `config/generated/*.env`.

```gherkin
Scenario: No hardcoded API URLs in connector wiring
  Given the Financial Markets connector is being wired
  When its configuration is assembled
  Then no hardcoded Finnhub or CoinGecko URLs appear in main.go
  And all URLs originate from smackerel.yaml or the connector's internal constants

Scenario: No fallback defaults for required credentials
  Given the Discord connector is enabled but bot_token is empty
  When the connector attempts to connect
  Then it fails with a clear error rather than using a default token
```

---

## Non-Functional Requirements

- **Performance:** Registering 5 additional connectors adds negligible startup time (<100ms total)
- **Reliability:** Each connector's failure is isolated — one failing connector does not affect others (existing supervisor guarantee)
- **Maintainability:** The wiring pattern for all 14 connectors is consistent — no special-case logic for any connector
- **Observability:** All connectors produce structured log entries on connect, sync, and error events

---

## Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Registered connectors | 14 (up from 9) | Supervisor registry count at startup |
| Dead code packages | 0 (down from 5) | Static analysis: all connector imports used |
| Config coverage | 14/14 connectors in smackerel.yaml | YAML inspection |
| Regression tests | All existing connector tests pass | `./smackerel.sh test unit` |
| Health visibility | 14 connectors in `/api/health` | E2E health endpoint check |

---

## Scope & Boundaries

### In Scope

- Import and instantiate 5 connectors in `cmd/core/main.go`
- Register each with the supervisor via `registry.Register()`
- Add auto-start logic gated on `enabled` config + valid credentials
- Add YAML config entries for Twitter/X, Weather, Gov Alerts, Financial Markets
- Map YAML config → `connector.ConnectorConfig` for each
- Update `config/generated/*.env` templates if needed via `./smackerel.sh config generate`

### Out of Scope

- New connector implementation code (all 5 packages are complete)
- Changes to the connector interface or supervisor
- New API endpoints
- UI changes
- OAuth flow changes (Discord uses bot token, not OAuth)
