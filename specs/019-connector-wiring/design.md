# Design: Connector Wiring — Register 5 Unwired Connectors

**Feature:** 019-connector-wiring
**Status:** Draft
**Created:** 2026-04-10

---

## Design Brief

**Current State:** 10 of 15 connector packages are registered in `cmd/core/main.go`. Five packages (discord, twitter, weather, alerts, markets) exist with full implementations and tests but are never imported, instantiated, or registered. Discord has a YAML config entry; the other four have none.

**Target State:** All 15 connectors are imported, instantiated, registered, and conditionally started in `cmd/core/main.go`. All 5 have config entries in `config/smackerel.yaml` and env var wiring in `scripts/commands/config.sh`. Each connector follows the exact same instantiation → register → conditional-start pattern as the existing 10.

**Patterns to Follow:**
- Import aliasing: `<name>Connector "github.com/smackerel/smackerel/internal/connector/<pkg>"` (see [main.go](../../cmd/core/main.go#L14-L25))
- Instantiation: `<name>Conn := <name>Connector.New("<id>")` (see [main.go](../../cmd/core/main.go#L128-L137))
- Registration: `registry.Register(<name>Conn)` (see [main.go](../../cmd/core/main.go#L138-L146))
- Auto-start: env-var gated `if` block → build `ConnectorConfig` → `Connect()` → `supervisor.StartConnector()` (see bookmarks/browser/maps blocks at [main.go](../../cmd/core/main.go#L149-L195))
- YAML config: `connectors.<name>:` block with `enabled: false` default (see existing entries in [smackerel.yaml](../../config/smackerel.yaml))
- Config generation: `yaml_get` extraction → env var → written to `config/generated/*.env` (see [config.sh](../../scripts/commands/config.sh#L183-L238))

**Patterns to Avoid:**
- Do NOT add automatic token validation at startup — connectors validate their own credentials in `Connect()`.

**Implementation Note (reconciled 2026-04-22):** Connector config fields were added to `config.Config` struct (e.g., `DiscordEnabled`, `DiscordBotToken`, etc.) and loaded via `os.Getenv()` in `config.Load()`. This is cleaner than raw `os.Getenv()` in `connectors.go` and follows the same pattern as file-based connectors. The original "Do NOT add connector fields" guidance was superseded during implementation.

**Resolved Decisions:**
- All 5 connectors default to `enabled: false`
- Discord: token-based auth (`AuthType: "token"`)
- Twitter: mixed auth — archive mode needs no auth, API mode uses bearer token
- Weather: no auth (`AuthType: "none"`) — Open-Meteo is free
- Gov Alerts: no auth (`AuthType: "none"`) — USGS/NWS are free
- Financial Markets: API key auth (`AuthType: "api_key"`)
- Env var naming follows `CONNECTOR_<UPPERCASE_NAME>_<FIELD>` convention established by `BOOKMARKS_IMPORT_DIR`, `MAPS_IMPORT_DIR`, `BROWSER_HISTORY_PATH`

**Open Questions:** None — this is mechanical wiring following established patterns.

---

## Architecture Overview

No architectural changes. This work adds 5 import lines, 5 instantiation lines, 5 registration lines, and 5 conditional-start blocks to [cmd/core/connectors.go](../../cmd/core/connectors.go). Helper functions (`parseJSONArray`, `parseJSONObject`, `parseFloatEnv`) are in [cmd/core/helpers.go](../../cmd/core/helpers.go). It adds 4 YAML config blocks to [config/smackerel.yaml](../../config/smackerel.yaml) (Discord already exists). It extends the config generation script to extract new env vars.

The data flow for each new connector is identical to existing ones:

```
smackerel.yaml → config generate → dev.env → os.Getenv() in config.Load() → config.Config → connectors.go → ConnectorConfig → connector.Connect() → supervisor.StartConnector()
```

---

## Registration Pattern

### Imports (5 new lines in the import block)

```go
discordConnector "github.com/smackerel/smackerel/internal/connector/discord"
twitterConnector "github.com/smackerel/smackerel/internal/connector/twitter"
weatherConnector "github.com/smackerel/smackerel/internal/connector/weather"
alertsConnector  "github.com/smackerel/smackerel/internal/connector/alerts"
marketsConnector "github.com/smackerel/smackerel/internal/connector/markets"
```

### Instantiation (5 new lines after existing `hospitableConn`)

```go
discordConn := discordConnector.New("discord")
twitterConn := twitterConnector.New("twitter")
weatherConn := weatherConnector.New("weather")
alertsConn  := alertsConnector.New("gov-alerts")
marketsConn := marketsConnector.New("financial-markets")
```

Connector IDs match the YAML config key names and the existing naming convention.

### Registration (5 new lines after existing `registry.Register(hospitableConn)`)

```go
registry.Register(discordConn)
registry.Register(twitterConn)
registry.Register(weatherConn)
registry.Register(alertsConn)
registry.Register(marketsConn)
```

---

## Per-Connector Config Wiring

### 1. Discord

**YAML config:** Already exists at `connectors.discord` in smackerel.yaml.

**Env vars needed:**

| Env Var | YAML Path | Purpose |
|---------|-----------|---------|
| `DISCORD_ENABLED` | `connectors.discord.enabled` | Enable/disable |
| `DISCORD_BOT_TOKEN` | `connectors.discord.bot_token` | Bot authentication |
| `DISCORD_SYNC_SCHEDULE` | `connectors.discord.sync_schedule` | Cron schedule |
| `DISCORD_ENABLE_GATEWAY` | `connectors.discord.enable_gateway` | WebSocket gateway |
| `DISCORD_BACKFILL_LIMIT` | `connectors.discord.backfill_limit` | History depth |
| `DISCORD_INCLUDE_THREADS` | `connectors.discord.include_threads` | Thread capture |
| `DISCORD_INCLUDE_PINS` | `connectors.discord.include_pins` | Pin capture |
| `DISCORD_CAPTURE_COMMANDS` | `connectors.discord.capture_commands` | Trigger commands (comma-separated) |
| `DISCORD_MONITORED_CHANNELS` | `connectors.discord.monitored_channels` | JSON array of channel configs |

**ConnectorConfig mapping:**

```go
ConnectorConfig{
    AuthType:     "token",
    Credentials:  map[string]string{"bot_token": os.Getenv("DISCORD_BOT_TOKEN")},
    Enabled:      os.Getenv("DISCORD_ENABLED") == "true",
    SyncSchedule: os.Getenv("DISCORD_SYNC_SCHEDULE"),
    SourceConfig: map[string]interface{}{
        "enable_gateway":     os.Getenv("DISCORD_ENABLE_GATEWAY") == "true",
        "backfill_limit":     parseFloatEnv("DISCORD_BACKFILL_LIMIT"),
        "include_threads":    os.Getenv("DISCORD_INCLUDE_THREADS") == "true",
        "include_pins":       os.Getenv("DISCORD_INCLUDE_PINS") == "true",
        "capture_commands":   parseJSONArray(os.Getenv("DISCORD_CAPTURE_COMMANDS")),
        "monitored_channels": parseJSONArray(os.Getenv("DISCORD_MONITORED_CHANNELS")),
    },
}
```

**`parseDiscordConfig` reads from:** `Credentials["bot_token"]`, `SourceConfig["monitored_channels"]`, `SourceConfig["backfill_limit"]`, `SourceConfig["enable_gateway"]`, `SourceConfig["include_threads"]`, `SourceConfig["include_pins"]`, `SourceConfig["capture_commands"]`.

**Connect() validates:** `BotToken` must not be empty.

**Enable gate:** `DISCORD_ENABLED == "true"` AND `DISCORD_BOT_TOKEN != ""`.

### 2. Twitter/X

**YAML config:** New entry needed under `connectors.twitter`.

**YAML block:**

```yaml
twitter:
  enabled: false
  sync_mode: archive  # archive, api, or hybrid
  archive_dir: ""     # REQUIRED for archive/hybrid mode: path to Twitter data export
  bearer_token: ""    # REQUIRED for api/hybrid mode: Twitter API v2 bearer token
  sync_schedule: "0 */6 * * *"
```

**Env vars needed:**

| Env Var | YAML Path | Purpose |
|---------|-----------|---------|
| `TWITTER_ENABLED` | `connectors.twitter.enabled` | Enable/disable |
| `TWITTER_SYNC_MODE` | `connectors.twitter.sync_mode` | archive/api/hybrid |
| `TWITTER_ARCHIVE_DIR` | `connectors.twitter.archive_dir` | Archive import path |
| `TWITTER_BEARER_TOKEN` | `connectors.twitter.bearer_token` | API auth |
| `TWITTER_SYNC_SCHEDULE` | `connectors.twitter.sync_schedule` | Cron schedule |

**ConnectorConfig mapping:**

```go
ConnectorConfig{
    AuthType:     "token",
    Credentials:  map[string]string{"bearer_token": os.Getenv("TWITTER_BEARER_TOKEN")},
    Enabled:      os.Getenv("TWITTER_ENABLED") == "true",
    SyncSchedule: os.Getenv("TWITTER_SYNC_SCHEDULE"),
    SourceConfig: map[string]interface{}{
        "sync_mode":   os.Getenv("TWITTER_SYNC_MODE"),
        "archive_dir": os.Getenv("TWITTER_ARCHIVE_DIR"),
    },
}
```

**`parseTwitterConfig` reads from:** `SourceConfig["sync_mode"]`, `SourceConfig["archive_dir"]`, `Credentials["bearer_token"]`.

**Connect() validates:** For archive/hybrid mode, `archive_dir` must exist on filesystem. For api/hybrid mode, `bearer_token` must be present.

**Enable gate:** `TWITTER_ENABLED == "true"`. The connector's own `Connect()` validates mode-specific requirements.

### 3. Weather

**YAML config:** New entry needed under `connectors.weather`.

**YAML block:**

```yaml
weather:
  enabled: false
  sync_schedule: "0 */3 * * *"  # Every 3 hours
  locations:
    - name: ""
      latitude: 0.0
      longitude: 0.0
```

**Env vars needed:**

| Env Var | YAML Path | Purpose |
|---------|-----------|---------|
| `WEATHER_ENABLED` | `connectors.weather.enabled` | Enable/disable |
| `WEATHER_SYNC_SCHEDULE` | `connectors.weather.sync_schedule` | Cron schedule |
| `WEATHER_LOCATIONS` | `connectors.weather.locations` | JSON array of {name, latitude, longitude} |

**ConnectorConfig mapping:**

```go
ConnectorConfig{
    AuthType: "none",
    Enabled:  os.Getenv("WEATHER_ENABLED") == "true",
    SyncSchedule: os.Getenv("WEATHER_SYNC_SCHEDULE"),
    SourceConfig: map[string]interface{}{
        "locations": parseJSONArray(os.Getenv("WEATHER_LOCATIONS")),
    },
}
```

**`parseWeatherConfig` reads from:** `SourceConfig["locations"]` (array of `{name, latitude, longitude}` maps).

**Connect() validates:** At least one location must be configured. Latitude must be in [-90, 90], longitude in [-180, 180].

**Enable gate:** `WEATHER_ENABLED == "true"` AND `WEATHER_LOCATIONS != ""`.

### 4. Gov Alerts

**YAML config:** New entry needed under `connectors.gov-alerts`.

**YAML block:**

```yaml
gov-alerts:
  enabled: false
  sync_schedule: "*/30 * * * *"  # Every 30 minutes
  min_earthquake_magnitude: 2.5
  locations:
    - name: ""
      latitude: 0.0
      longitude: 0.0
      radius_km: 200
```

**Env vars needed:**

| Env Var | YAML Path | Purpose |
|---------|-----------|---------|
| `GOV_ALERTS_ENABLED` | `connectors.gov-alerts.enabled` | Enable/disable |
| `GOV_ALERTS_SYNC_SCHEDULE` | `connectors.gov-alerts.sync_schedule` | Cron schedule |
| `GOV_ALERTS_MIN_EARTHQUAKE_MAG` | `connectors.gov-alerts.min_earthquake_magnitude` | Magnitude threshold |
| `GOV_ALERTS_LOCATIONS` | `connectors.gov-alerts.locations` | JSON array of {name, lat, lon, radius_km} |

**ConnectorConfig mapping:**

```go
ConnectorConfig{
    AuthType: "none",
    Enabled:  os.Getenv("GOV_ALERTS_ENABLED") == "true",
    SyncSchedule: os.Getenv("GOV_ALERTS_SYNC_SCHEDULE"),
    SourceConfig: map[string]interface{}{
        "locations":                parseJSONArray(os.Getenv("GOV_ALERTS_LOCATIONS")),
        "min_earthquake_magnitude": parseFloatEnv("GOV_ALERTS_MIN_EARTHQUAKE_MAG"),
    },
}
```

**`parseAlertsConfig` reads from:** `SourceConfig["locations"]` (array of `{name, latitude, longitude, radius_km}` maps), `SourceConfig["min_earthquake_magnitude"]`.

**Connect() validates:** At least one location must be configured with valid finite coordinates and positive radius.

**Enable gate:** `GOV_ALERTS_ENABLED == "true"` AND `GOV_ALERTS_LOCATIONS != ""`.

### 5. Financial Markets

**YAML config:** New entry needed under `connectors.financial-markets`.

**YAML block:**

```yaml
financial-markets:
  enabled: false
  sync_schedule: "*/15 * * * *"  # Every 15 minutes
  finnhub_api_key: ""    # REQUIRED when enabled: free tier at finnhub.io
  fred_api_key: ""       # Optional: FRED economic data
  coingecko_enabled: true
  alert_threshold: 5.0   # Percentage change to trigger alert
  watchlist:
    stocks: []
    etfs: []
    crypto: []
    forex_pairs: []
```

**Env vars needed:**

| Env Var | YAML Path | Purpose |
|---------|-----------|---------|
| `FINANCIAL_MARKETS_ENABLED` | `connectors.financial-markets.enabled` | Enable/disable |
| `FINANCIAL_MARKETS_SYNC_SCHEDULE` | `connectors.financial-markets.sync_schedule` | Cron schedule |
| `FINANCIAL_MARKETS_FINNHUB_API_KEY` | `connectors.financial-markets.finnhub_api_key` | Finnhub auth |
| `FINANCIAL_MARKETS_FRED_API_KEY` | `connectors.financial-markets.fred_api_key` | FRED auth (optional) |
| `FINANCIAL_MARKETS_ALERT_THRESHOLD` | `connectors.financial-markets.alert_threshold` | Alert pct |
| `FINANCIAL_MARKETS_WATCHLIST` | `connectors.financial-markets.watchlist` | JSON object `{stocks, etfs, crypto, forex_pairs}` |
| `FINANCIAL_MARKETS_FRED_ENABLED` | `connectors.financial-markets.fred_enabled` | Enable FRED economic indicators |
| `FINANCIAL_MARKETS_FRED_SERIES` | `connectors.financial-markets.fred_series` | JSON array of FRED series IDs |

> **SST Gap (fixed 2026-04-10):** `connectors.financial-markets.coingecko_enabled` is now extracted as `FINANCIAL_MARKETS_COINGECKO_ENABLED` in the config pipeline, passed via `SourceConfig["coingecko_enabled"]` in main.go, and read by `parseMarketsConfig` instead of hardcoding `true`.

> **SST Gap (fixed 2026-04-14, IMP-019-R28):** `connectors.financial-markets.fred_enabled` and `connectors.financial-markets.fred_series` are now extracted as `FINANCIAL_MARKETS_FRED_ENABLED` and `FINANCIAL_MARKETS_FRED_SERIES` in the config pipeline, passed via `SourceConfig["fred_enabled"]` and `SourceConfig["fred_series"]` in main.go, and read by `parseMarketsConfig`. Previously, FRED was always auto-enabled when `fred_api_key` was non-empty with no operator disable path.

**ConnectorConfig mapping:**

```go
ConnectorConfig{
    AuthType: "api_key",
    Credentials: map[string]string{
        "finnhub_api_key": os.Getenv("FINANCIAL_MARKETS_FINNHUB_API_KEY"),
        "fred_api_key":    os.Getenv("FINANCIAL_MARKETS_FRED_API_KEY"),
    },
    Enabled:      os.Getenv("FINANCIAL_MARKETS_ENABLED") == "true",
    SyncSchedule: os.Getenv("FINANCIAL_MARKETS_SYNC_SCHEDULE"),
    SourceConfig: map[string]interface{}{
        "watchlist":         parseJSONObject(os.Getenv("FINANCIAL_MARKETS_WATCHLIST")),
        "alert_threshold":   parseFloatEnv("FINANCIAL_MARKETS_ALERT_THRESHOLD"),
        "coingecko_enabled": os.Getenv("FINANCIAL_MARKETS_COINGECKO_ENABLED") == "true",
        "fred_enabled":     os.Getenv("FINANCIAL_MARKETS_FRED_ENABLED") == "true",
        "fred_series":      parseJSONArrayEnv("FINANCIAL_MARKETS_FRED_SERIES"),
    },
}
```

**`parseMarketsConfig` reads from:** `Credentials["finnhub_api_key"]`, `Credentials["fred_api_key"]`, `SourceConfig["watchlist"]` (map with `stocks`, `etfs`, `crypto` arrays), `SourceConfig["alert_threshold"]`, `SourceConfig["coingecko_enabled"]`, `SourceConfig["fred_enabled"]`, `SourceConfig["fred_series"]`.

**Connect() validates:** `finnhub_api_key` must not be empty. Watchlist symbols must match `^[A-Za-z0-9.\-]{1,10}$` (stocks/ETFs) or `^[a-z0-9\-]{1,64}$` (crypto). Each category capped at 100 entries.

**Enable gate:** `FINANCIAL_MARKETS_ENABLED == "true"` AND `FINANCIAL_MARKETS_FINNHUB_API_KEY != ""`.

---

## Config Summary

### YAML entries status

| Connector | YAML Entry Exists | Action Needed |
|-----------|------------------|---------------|
| Discord | Yes (`connectors.discord`) | None — already present |
| Twitter/X | No | Add `connectors.twitter` |
| Weather | No | Add `connectors.weather` |
| Gov Alerts | No | Add `connectors.gov-alerts` |
| Financial Markets | No | Add `connectors.financial-markets` |

### Enable/Disable Logic

All 5 connectors follow the same pattern established by bookmarks/browser/maps:

```
1. Read CONNECTOR_ENABLED env var
2. If "true", build ConnectorConfig from env vars
3. Call connector.Connect(ctx, config)
4. If Connect succeeds, call supervisor.StartConnector(ctx, id)
5. If Connect fails, log warning and continue (don't crash startup)
```

Disabled connectors are still instantiated and registered (for health visibility) but never have `Connect()` or `StartConnector()` called.

---

## Config Generation Script Changes

[scripts/commands/config.sh](../../scripts/commands/config.sh) needs new `yaml_get` extractions and env var output for the 5 connectors. The existing pattern for connector env vars (lines 183-185) extends naturally:

```bash
# Discord connector
DISCORD_ENABLED="$(yaml_get connectors.discord.enabled 2>/dev/null)" || DISCORD_ENABLED="false"
DISCORD_BOT_TOKEN="$(yaml_get connectors.discord.bot_token 2>/dev/null)" || DISCORD_BOT_TOKEN=""
DISCORD_SYNC_SCHEDULE="$(yaml_get connectors.discord.sync_schedule 2>/dev/null)" || DISCORD_SYNC_SCHEDULE=""
DISCORD_ENABLE_GATEWAY="$(yaml_get connectors.discord.enable_gateway 2>/dev/null)" || DISCORD_ENABLE_GATEWAY=""
DISCORD_BACKFILL_LIMIT="$(yaml_get connectors.discord.backfill_limit 2>/dev/null)" || DISCORD_BACKFILL_LIMIT=""
DISCORD_INCLUDE_THREADS="$(yaml_get connectors.discord.include_threads 2>/dev/null)" || DISCORD_INCLUDE_THREADS=""
DISCORD_INCLUDE_PINS="$(yaml_get connectors.discord.include_pins 2>/dev/null)" || DISCORD_INCLUDE_PINS=""
DISCORD_CAPTURE_COMMANDS="$(yaml_get connectors.discord.capture_commands 2>/dev/null)" || DISCORD_CAPTURE_COMMANDS=""
DISCORD_MONITORED_CHANNELS="$(yaml_get_json connectors.discord.monitored_channels 2>/dev/null)" || DISCORD_MONITORED_CHANNELS=""

# Twitter connector
TWITTER_ENABLED="$(yaml_get connectors.twitter.enabled 2>/dev/null)" || TWITTER_ENABLED="false"
TWITTER_SYNC_MODE="$(yaml_get connectors.twitter.sync_mode 2>/dev/null)" || TWITTER_SYNC_MODE=""
TWITTER_ARCHIVE_DIR="$(yaml_get connectors.twitter.archive_dir 2>/dev/null)" || TWITTER_ARCHIVE_DIR=""
TWITTER_BEARER_TOKEN="$(yaml_get connectors.twitter.bearer_token 2>/dev/null)" || TWITTER_BEARER_TOKEN=""
TWITTER_SYNC_SCHEDULE="$(yaml_get connectors.twitter.sync_schedule 2>/dev/null)" || TWITTER_SYNC_SCHEDULE=""

# Weather connector
WEATHER_ENABLED="$(yaml_get connectors.weather.enabled 2>/dev/null)" || WEATHER_ENABLED="false"
WEATHER_SYNC_SCHEDULE="$(yaml_get connectors.weather.sync_schedule 2>/dev/null)" || WEATHER_SYNC_SCHEDULE=""
WEATHER_LOCATIONS="$(yaml_get_json connectors.weather.locations 2>/dev/null)" || WEATHER_LOCATIONS=""

# Gov Alerts connector
GOV_ALERTS_ENABLED="$(yaml_get connectors.gov-alerts.enabled 2>/dev/null)" || GOV_ALERTS_ENABLED="false"
GOV_ALERTS_SYNC_SCHEDULE="$(yaml_get connectors.gov-alerts.sync_schedule 2>/dev/null)" || GOV_ALERTS_SYNC_SCHEDULE=""
GOV_ALERTS_MIN_EARTHQUAKE_MAG="$(yaml_get connectors.gov-alerts.min_earthquake_magnitude 2>/dev/null)" || GOV_ALERTS_MIN_EARTHQUAKE_MAG=""
GOV_ALERTS_LOCATIONS="$(yaml_get_json connectors.gov-alerts.locations 2>/dev/null)" || GOV_ALERTS_LOCATIONS=""

# Financial Markets connector
FINANCIAL_MARKETS_ENABLED="$(yaml_get connectors.financial-markets.enabled 2>/dev/null)" || FINANCIAL_MARKETS_ENABLED="false"
FINANCIAL_MARKETS_SYNC_SCHEDULE="$(yaml_get connectors.financial-markets.sync_schedule 2>/dev/null)" || FINANCIAL_MARKETS_SYNC_SCHEDULE=""
FINANCIAL_MARKETS_FINNHUB_API_KEY="$(yaml_get connectors.financial-markets.finnhub_api_key 2>/dev/null)" || FINANCIAL_MARKETS_FINNHUB_API_KEY=""
FINANCIAL_MARKETS_FRED_API_KEY="$(yaml_get connectors.financial-markets.fred_api_key 2>/dev/null)" || FINANCIAL_MARKETS_FRED_API_KEY=""
FINANCIAL_MARKETS_ALERT_THRESHOLD="$(yaml_get connectors.financial-markets.alert_threshold 2>/dev/null)" || FINANCIAL_MARKETS_ALERT_THRESHOLD=""
FINANCIAL_MARKETS_WATCHLIST="$(yaml_get_json connectors.financial-markets.watchlist 2>/dev/null)" || FINANCIAL_MARKETS_WATCHLIST=""
```

These values then get written into the generated env file alongside the existing `BOOKMARKS_IMPORT_DIR`, `MAPS_IMPORT_DIR`, and `BROWSER_HISTORY_PATH` entries.

---

## Helper Functions

The auto-start blocks for Weather, Gov Alerts, and Financial Markets need JSON array/object parsing from env vars. Two small helpers are needed in main.go (or a shared location):

- `parseJSONArray(s string) []interface{}` — parses a JSON array string, returns nil on empty/error
- `parseJSONObject(s string) map[string]interface{}` — parses a JSON object string, returns nil on empty/error
- `parseFloatEnv(key string) float64` — parses env var as float64, returns 0 on empty/error

**Originally planned but not implemented:** `parseIntEnv` and `splitCSV` were deemed unnecessary — `parseFloatEnv` covers numeric fields (backfill_limit), and `parseJSONArray` covers YAML arrays (capture_commands are serialized as JSON by `yaml_get`).

These are trivial parsing utilities, not business logic. They exist purely to bridge the env-var string representation to the `map[string]interface{}` types that `ConnectorConfig.SourceConfig` expects.

---

## Testing Strategy

| Test Type | What to Validate | How |
|-----------|-----------------|-----|
| Unit (Go) | All 15 connectors present in registry after startup | Test `main.go` wiring or registry count |
| Unit (Go) | Config parsing for each connector handles empty/missing values correctly | Existing `*_test.go` in each connector package already cover this |
| Config generation | New env vars appear in generated env files | `./smackerel.sh config generate` then check output |
| E2E | Health endpoint lists 15 connectors | `GET /api/health` shows all connectors |
| Regression | Existing 10 connectors still work | Existing unit + integration tests pass unchanged |

---

## Security Considerations

- Bot tokens and API keys are secrets — they flow through `smackerel.yaml` → generated env files (gitignored) → env vars. Never logged at INFO level.
- Empty credentials cause `Connect()` to fail with descriptive error, not silent fallback.
- No new network surfaces — connectors only make outbound connections to their respective APIs.

---

## Alternatives Considered

**Centralized connector config in `config.Config` struct:** Rejected. The existing pattern uses `os.Getenv()` directly in main.go for connector-specific env vars. Adding 30+ fields to the central Config struct for optional connectors would bloat it unnecessarily. The connector packages already own their own config parsing via `parseXxxConfig()`.

**Auto-discovery of connector packages:** Rejected. Explicit registration is intentional — it provides compile-time guarantees that connector packages exist and their interfaces are satisfied.
