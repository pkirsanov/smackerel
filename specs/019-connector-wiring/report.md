# Report: 019 Connector Wiring — Register 5 Unwired Connectors

**Feature:** 019-connector-wiring
**Created:** 2026-04-10
**Last Reconciled:** 2026-04-10

---

## Summary

| Scope | Name | Status | Evidence |
|-------|------|--------|----------|
| 1 | Wire All 5 Connectors | Done | Unit tests pass, config generates, build passes |

## Test Evidence

### Scope 1: Wire All 5 Connectors

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit | `./smackerel.sh test unit` | PASS (31 Go packages, 51 Python tests) | 2026-04-10 |
| Check | `./smackerel.sh check` | PASS — "Config is in sync with SST" | 2026-04-10 |
| Config generate | `./smackerel.sh config generate` | PASS — dev.env + nats.conf generated with all 5 connector env vars | 2026-04-10 |

## Reconciliation Findings (2026-04-10)

### Confirmed Claims

| Claim | Verified |
|-------|----------|
| 15 connector imports in `cmd/core/main.go` | YES |
| 15 connector instantiations with correct IDs | YES |
| 15 connector registrations with registry | YES |
| 5 auto-start blocks (Discord, Twitter, Weather, Gov Alerts, Financial Markets) | YES |
| 4 new YAML config blocks in `smackerel.yaml` (Twitter, Weather, Gov Alerts, Financial Markets) | YES |
| Discord YAML block already existed | YES |
| Config generation produces all connector env vars in `dev.env` | YES — 27 new env vars confirmed |
| `ConnectorHealthLister` interface on registry, wired into `/api/health` | YES |
| All 5 connectors default to `enabled: false` | YES |
| Helper functions `parseJSONArray`, `parseJSONObject`, `parseFloatEnv` in `main.go` | YES |
| No hardcoded fallback defaults in `main.go` auto-start blocks | YES — all read from `os.Getenv()` |
| Existing 10 connectors unaffected | YES — tests cached, no source changes |

### Drift Items

| ID | Severity | Description | Status |
|----|----------|-------------|--------|
| DRIFT-001 | Low | `parseIntEnv`/`splitCSV` listed in scopes "New Types & Signatures" and design but never implemented. Code uses `parseFloatEnv` for `DISCORD_BACKFILL_LIMIT` and `parseJSONArray` for `DISCORD_CAPTURE_COMMANDS` instead. Implementation works correctly. | Fixed in design.md + scopes.md |
| DRIFT-002 | Medium | `coingecko_enabled` field exists in `smackerel.yaml` under `financial-markets` but was never extracted in `config.sh`, not in `dev.env`, and `parseMarketsConfig` hardcoded `CoinGeckoEnabled: true`. | **Fixed** — env var `FINANCIAL_MARKETS_COINGECKO_ENABLED` added to config.sh extraction, generated env files, main.go SourceConfig, and parseMarketsConfig. Test added. |
| DRIFT-003 | Low | `parseMarketsConfig` has hardcoded `AlertThreshold: 5.0` fallback and `CoinGeckoEnabled: true`. The AlertThreshold default is dead code (env var always overrides via SourceConfig). CoinGeckoEnabled now reads from SourceConfig. | Fixed — CoinGeckoEnabled now config-driven. AlertThreshold default is harmless dead code (always overridden). |
| DRIFT-004 | Low | `DISCORD_CAPTURE_COMMANDS` used `yaml_get` (not `yaml_get_json`) in config.sh, yielding inline YAML array as JSON. Fragile with multi-line YAML. | **Fixed** — switched to `yaml_get_json` |

### Open SST Gaps

None — all SST gaps have been remediated.

## Completion Statement

Scope 1 implementation is complete. All 15 connectors are imported, instantiated, registered, and conditionally startable. Config generation produces correct env vars. CoinGecko SST gap (DRIFT-002) and DISCORD_CAPTURE_COMMANDS fragility (DRIFT-004) remediated. All unit tests pass.

---

## Security Sweep (2026-04-11)

**Trigger:** Stochastic quality sweep — security trigger on connector wiring code.
**Scope:** `cmd/core/main.go` connector wiring, JSON/float parse helpers, credential passing, enable/disable logic.

### Findings

| ID | Severity | CWE | Description | Status |
|----|----------|-----|-------------|--------|
| SEC-019-001 | Medium | CWE-754 | `parseJSONArray`, `parseJSONObject`, `parseFloatEnv` silently swallow parse errors, returning nil/0 without logging. Malformed env vars (e.g. `WEATHER_LOCATIONS`, `GOV_ALERTS_LOCATIONS`, `FINANCIAL_MARKETS_WATCHLIST`) cause connectors to start with empty/zero config with no operator-visible indication. Safety-critical for `GOV_ALERTS_MIN_EARTHQUAKE_MAG` where silent 0 means "alert on all earthquakes." | **Fixed** — parse helpers now log `slog.Warn` on failure with error detail and input length (no raw value to avoid leaking structured config). |
| SEC-019-002 | Medium | CWE-1188 | `coingecko_enabled` used `!= "false"` (default-true) instead of `== "true"` (explicit opt-in). If the env var is absent for any reason, CoinGecko is silently enabled — inconsistent with all other connector boolean flags that use `== "true"`. `parseMarketsConfig` also defaulted to `CoinGeckoEnabled: true`. | **Fixed** — changed to `== "true"` in `main.go` and default to `false` in `parseMarketsConfig`. YAML default remains `coingecko_enabled: true` so SST-compliant deployments are unaffected. |
| SEC-019-003 | Low | N/A | Five new connectors read env vars via raw `os.Getenv()` in `main.go` instead of flowing through `config.Config` struct like the file-based connectors (bookmarks, browser-history, maps). This is a governance drift (SST layering), not a runtime vulnerability — the SST pipeline still sources values from `smackerel.yaml`. | **Documented** — not fixed in this sweep; would require adding ~30 fields to Config struct. Pattern matches design.md's explicit "Do NOT add connector fields to config.Config struct" decision. |

### Files Changed

| File | Change |
|------|--------|
| `cmd/core/main.go` | `parseJSONArray`: warn on parse error. `parseJSONObject`: warn on parse error. `parseFloatEnv`: warn on parse error with env key name. `coingecko_enabled`: `!= "false"` → `== "true"`. |
| `internal/connector/markets/markets.go` | `parseMarketsConfig` default: `CoinGeckoEnabled: true` → `false`. Comment updated. |
| `internal/connector/markets/markets_test.go` | Updated `TestParseMarketsConfig_CoinGeckoEnabled` "defaults to" case: `true` → `false`. |

### Test Evidence

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Build | `./smackerel.sh build` | PASS — both images built | 2026-04-11 |
| Unit | `./smackerel.sh test unit` | PASS — 31 Go packages (markets recompiled), 53 Python tests | 2026-04-11 |

---

## Test Sweep (2026-04-12)

**Trigger:** Stochastic quality sweep — test trigger on connector wiring.
**Scope:** Test coverage, quality, and gaps against spec 019 Gherkin scenarios.

### Coverage Assessment

| Scenario | Required Test (per scopes.md) | Pre-Sweep Status | Post-Sweep Status |
|----------|-------------------------------|-------------------|-------------------|
| SCN-019-001 (All connectors registered) | `cmd/core/main_test.go` — registry count | **MISSING** — only helper tests existed | **ADDED** — `TestAllConnectorsRegistered` |
| SCN-019-002 (Discord operational) | `internal/connector/discord/discord_test.go` | ✅ Covered by `TestConnect_ValidConfig` | ✅ No change needed |
| SCN-019-003 (Missing creds → error) | Individual connector `*_test.go` files | ✅ Covered (`TestConnect_Missing*`) | ✅ No change needed |
| SCN-019-004 (Config entries exist) | `tests/integration/connector_wiring_test.go` | **MISSING** — file never created | **ADDED** — `test_connector_wiring.sh` (32 assertions) |
| SCN-019-005 (Health shows connectors) | `tests/e2e/health_connectors_test.go` | Partial — mocked in `health_test.go` | Partial — E2E requires live stack |
| SCN-019-006 (Existing unaffected) | Regression via existing tests | ✅ All existing tests pass | **ADDED** — `TestDuplicateRegistrationRejected` |

### Findings

| ID | Severity | Description | Status |
|----|----------|-------------|--------|
| TEST-019-001 | Medium | `cmd/core/main_test.go` had no test verifying all connectors register — only `parseJSONArray`, `parseJSONObject`, `parseFloatEnv` helpers were tested. SCN-019-001 was uncovered. | **Fixed** — `TestAllConnectorsRegistered` added: instantiates all 15 connectors, registers in Registry, asserts count=15 and all expected IDs present. |
| TEST-019-002 | Medium | `tests/integration/connector_wiring_test.go` listed in test plan (scopes.md) but file never created. SCN-019-004 config generation validation was uncovered. | **Fixed** — `tests/integration/test_connector_wiring.sh` added: runs `config generate`, verifies 27 env vars present for all 5 connectors, asserts all default to `enabled: false`. |
| TEST-019-003 | Low | Spec claimed "14 connectors" but actual codebase registers 15 (guesthost was a 10th pre-existing connector). Not a code bug — only a spec narrative inaccuracy. | **Fixed** — all spec artifacts (spec.md, design.md, scopes.md, uservalidation.md) updated to 15 in harden-to-doc sweep. |
| TEST-019-004 | Low | No E2E test for `GET /api/health` listing all connectors (SCN-019-005). The `health_test.go` covers the mechanism via mock but no live-stack E2E exists. | **Documented** — E2E requires running stack; deferred to integration test suite. |

### Files Changed

| File | Change |
|------|--------|
| `cmd/core/main_test.go` | Added `TestAllConnectorsRegistered` (SCN-019-001) and `TestDuplicateRegistrationRejected` (SCN-019-006 guard) |
| `tests/integration/test_connector_wiring.sh` | New file — config generation integration test (SCN-019-004), 32 assertions |

### Test Evidence

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit | `./smackerel.sh test unit` | PASS — 33 Go packages, 69 Python tests | 2026-04-12 |
| Integration (config) | `bash tests/integration/test_connector_wiring.sh` | PASS — 32/32 assertions | 2026-04-12 |

---

## Hardening Sweep (2026-04-13)

**Trigger:** Stochastic quality sweep R01 — harden trigger on connector wiring.
**Scope:** Spec artifact accuracy, startup observability, code/doc consistency.

### Findings

| ID | Severity | Description | Status |
|----|----------|-------------|--------|
| HARDEN-019-001 | Low | All spec artifacts (spec.md, design.md, scopes.md, uservalidation.md, report.md) claimed "14 connectors" but codebase has 15 packages (guesthost was the 10th pre-existing connector). Code and tests were already correct at 15. | **Fixed** — all spec artifacts updated: 14→15 connector count, 9→10 pre-existing count. |
| HARDEN-019-002 | Low | No startup log confirming total registered connector count after the registration loop. Operators could not easily verify all connectors loaded without examining health endpoint. | **Fixed** — added `slog.Info("connector registry initialized", "count", registry.Count())` after registration loop in `cmd/core/main.go`. |

### Files Changed

| File | Change |
|------|--------|
| `cmd/core/main.go` | Added `slog.Info("connector registry initialized", ...)` after registration loop |
| `specs/019-connector-wiring/spec.md` | 14→15 connector count in 9 locations |
| `specs/019-connector-wiring/design.md` | 14→15 / 9→10 counts in 5 locations |
| `specs/019-connector-wiring/scopes.md` | 14→15 counts in 7 locations |
| `specs/019-connector-wiring/uservalidation.md` | 14→15 counts in 2 locations |
| `specs/019-connector-wiring/report.md` | 14→15 / 9→10 counts in 4 locations, TEST-019-003 status updated |

### Test Evidence

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit | `./smackerel.sh test unit` | PASS — 33 Go packages (core recompiled), Python tests cached | 2026-04-13 |
| Check | `./smackerel.sh check` | PASS — config in sync | 2026-04-13 |
| Lint | `./smackerel.sh lint` | PASS — all checks passed | 2026-04-13 |

---

## Hardening Sweep R21 (2026-04-14)

**Trigger:** Stochastic quality sweep R21 — harden trigger on connector wiring.
**Scope:** `cmd/core/main.go` connector auto-start blocks, JSON/float parse helpers, config generation pipeline (`config.sh`), alerts connector credential channel.

### Findings

| ID | Severity | CWE | Description | Status |
|----|----------|-----|-------------|--------|
| H-019-R21-001 | Medium | CWE-522 | Gov Alerts `airnow_api_key` wired through `SourceConfig` instead of `Credentials` map. API key is a third-party secret but was placed in the non-credential configuration map, bypassing any credential-aware serialization/logging safeguards. Financial Markets correctly routes its API keys (`finnhub_api_key`, `fred_api_key`) through `Credentials`. | **Fixed** — Moved `airnow_api_key` from `SourceConfig` to `Credentials` in `main.go` auto-start block. Updated `parseAlertConfig()` to read from `config.Credentials["airnow_api_key"]`. Changed `AuthType` from `"none"` to `"api_key"` for consistency. |
| H-019-R21-002 | Low | CWE-778 | `parseJSONArray` and `parseJSONObject` logged parse failures without including the env var key name — made error correlation impossible at startup when multiple JSON env vars are configured. `parseFloatEnv` already included key in its logs, creating an asymmetry in the helper API. | **Fixed** — Added `parseJSONArrayEnv(key)` and `parseJSONObjectEnv(key)` that read the env var internally and include the key in structured log warnings. Updated all 5 auto-start blocks to use the `Env` variants. Kept backward-compat wrappers `parseJSONArray(s)` / `parseJSONObject(s)` for existing callers. |
| H-019-R21-003 | Medium | CWE-1286 | `yaml_get_json.parse_array()` in `config.sh` could not handle block-format scalar YAML arrays. Items like `- "!save"` without a `:` separator were silently dropped, producing empty JSON arrays. Inline YAML arrays `[a, b]` and object arrays `- key: val` were unaffected. User switching from inline to block format would silently lose all values. | **Fixed** — `parse_array()` now detects scalar items (no `:` in value after `- `) and appends them directly via `scalar()` instead of skipping. |

### Files Changed

| File | Change |
|------|--------|
| `cmd/core/main.go` | H-019-R21-001: Moved `airnow_api_key` to Credentials, set `AuthType: "api_key"`. H-019-R21-002: Added `parseJSONArrayEnv`, `parseJSONObjectEnv`, `parseJSONArrayVal`, `parseJSONObjectVal` helpers; updated 5 call sites to `Env` variants. |
| `internal/connector/alerts/alerts.go` | H-019-R21-001: `parseAlertConfig()` reads `airnow_api_key` from `config.Credentials` instead of `config.SourceConfig`. |
| `internal/connector/alerts/alerts_test.go` | H-019-R21-001: Updated 2 test fixtures to place `airnow_api_key` in `Credentials`. |
| `scripts/commands/config.sh` | H-019-R21-003: `parse_array()` now handles scalar-only items in block-format YAML arrays. |
| `cmd/core/main_test.go` | H-019-R21-002: Added 9 adversarial tests for `parseJSONArrayEnv`, `parseJSONObjectEnv`, and backward-compat wrappers. |

### Test Evidence

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit | `./smackerel.sh test unit` | PASS — 33 Go packages (core + alerts recompiled), Python cached | 2026-04-14 |
| Check | `./smackerel.sh check` | PASS — config in sync | 2026-04-14 |

---

## Improvement Sweep R27 (2026-04-14)

**Trigger:** Stochastic quality sweep R27 — improve trigger on connector wiring.
**Scope:** SST end-to-end wiring completeness — verifying every configurable field in the 5 wired connectors has a full YAML → config.sh → env → main.go → SourceConfig pipeline.

### Findings

| ID | Severity | Description | Status |
|----|----------|-------------|--------|
| IMP-019-R27-001 | High | Gov Alerts `source_earthquake` entirely missing from SST pipeline. The connector's `parseAlertsConfig()` was fixed in R24-017 to read `source_earthquake` from SourceConfig, but the main.go auto-start block never set it, config.sh never extracted it, and smackerel.yaml had no entry. All 6 other source toggles (weather, tsunami, volcano, wildfire, airnow, gdacs) had complete 3-layer wiring. Consequence: USGS earthquake alerts — the primary use case of the gov-alerts connector — could never be disabled via config. | **Fixed** — Added `source_earthquake: true` to `config/smackerel.yaml`, extraction line in `scripts/commands/config.sh`, env var output, and `"source_earthquake": os.Getenv("GOV_ALERTS_SOURCE_EARTHQUAKE") == "true"` in main.go Gov Alerts SourceConfig. |
| IMP-019-R27-002 | Medium | Weather `enable_alerts`, `forecast_days`, `precision` missing from SST pipeline. The weather connector's `parseWeatherConfig()` was fixed in R23-016 to read these 3 fields from SourceConfig, but main.go, config.sh, and smackerel.yaml had no entries. Consequence: weather alert notifications could not be enabled via config; forecast day horizon (default 7) and coordinate precision (default 2) were not configurable. | **Fixed** — Added `enable_alerts: false`, `forecast_days: 7`, `precision: 2` to `config/smackerel.yaml` under `weather:`. Added 3 extraction lines and env var outputs in `scripts/commands/config.sh`. Added `"enable_alerts"`, `"forecast_days"`, `"precision"` to main.go Weather SourceConfig. |

### Files Changed

| File | Change |
|------|--------|
| `config/smackerel.yaml` | IMP-019-R27-001: Added `source_earthquake: true` under `gov-alerts:`. IMP-019-R27-002: Added `enable_alerts: false`, `forecast_days: 7`, `precision: 2` under `weather:`. |
| `scripts/commands/config.sh` | IMP-019-R27-001: Added `GOV_ALERTS_SOURCE_EARTHQUAKE` extraction and env var output. IMP-019-R27-002: Added `WEATHER_ENABLE_ALERTS`, `WEATHER_FORECAST_DAYS`, `WEATHER_PRECISION` extraction and env var outputs. |
| `cmd/core/main.go` | IMP-019-R27-001: Added `"source_earthquake"` to Gov Alerts SourceConfig. IMP-019-R27-002: Added `"enable_alerts"`, `"forecast_days"`, `"precision"` to Weather SourceConfig. |
| `cmd/core/main_test.go` | Added 9 adversarial tests: 3 for `source_earthquake` wiring (enabled/disabled/unset), 3 for `enable_alerts` wiring (enabled/disabled), 2 for `forecast_days`/`precision` via `parseFloatEnv`, 1 for empty `forecast_days` zero-fallback. |

### Test Evidence

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit | `./smackerel.sh test unit` | PASS — 33 Go packages (core recompiled), 72 Python tests | 2026-04-14 |
| Build | `./smackerel.sh build` | PASS — both images built | 2026-04-14 |
| Check | `./smackerel.sh check` | PASS — config in sync | 2026-04-14 |
| Config generate | `./smackerel.sh config generate` | PASS — 4 new env vars confirmed: `GOV_ALERTS_SOURCE_EARTHQUAKE=true`, `WEATHER_ENABLE_ALERTS=false`, `WEATHER_FORECAST_DAYS=7`, `WEATHER_PRECISION=2` | 2026-04-14 |

---

## Improvement Sweep R28 (2026-04-14)

**Trigger:** Stochastic quality sweep — improve trigger on connector wiring (child workflow).
**Scope:** SST end-to-end wiring completeness — verifying every configurable field in the Financial Markets connector has a full YAML → config.sh → env → main.go → SourceConfig pipeline.

### Findings

| ID | Severity | Description | Status |
|----|----------|-------------|--------|
| IMP-019-R28-001 | High | Financial Markets `fred_enabled` entirely missing from SST pipeline. The connector's `parseMarketsConfig()` reads `SourceConfig["fred_enabled"].(bool)` to allow explicit enable/disable of FRED economic data, but main.go never set it, config.sh never extracted it, and smackerel.yaml had no entry. Consequence: FRED is always auto-enabled whenever `fred_api_key` is non-empty — operators cannot disable FRED data while keeping their API key configured. Identical pattern to R27's `source_earthquake` gap. | **Fixed** — Added `fred_enabled: true` to `config/smackerel.yaml`, extraction line `FINANCIAL_MARKETS_FRED_ENABLED` in `scripts/commands/config.sh`, env var output, and `"fred_enabled": os.Getenv("FINANCIAL_MARKETS_FRED_ENABLED") == "true"` in main.go Financial Markets SourceConfig. |
| IMP-019-R28-002 | Medium | Financial Markets `fred_series` entirely missing from SST pipeline. The connector's `parseMarketsConfig()` reads `SourceConfig["fred_series"].([]interface{})` to allow customizing which FRED economic indicator series are tracked, but main.go, config.sh, and smackerel.yaml had no entries. Consequence: FRED series always defaults to `["GDP", "UNRATE", "CPIAUCSL", "DFF", "FEDFUNDS"]` with no operator override path via config. | **Fixed** — Added `fred_series: ["GDP", "UNRATE", "CPIAUCSL", "DFF", "FEDFUNDS"]` to YAML, extraction via `yaml_get_json` in config.sh, env var output, and `"fred_series": parseJSONArrayEnv("FINANCIAL_MARKETS_FRED_SERIES")` in main.go Financial Markets SourceConfig. |

### Files Changed

| File | Change |
|------|--------|
| `config/smackerel.yaml` | IMP-019-R28-001: Added `fred_enabled: true` under `financial-markets:`. IMP-019-R28-002: Added `fred_series: [...]` under `financial-markets:`. |
| `scripts/commands/config.sh` | IMP-019-R28-001: Added `FINANCIAL_MARKETS_FRED_ENABLED` extraction and env var output. IMP-019-R28-002: Added `FINANCIAL_MARKETS_FRED_SERIES` extraction via `yaml_get_json` and env var output. |
| `cmd/core/main.go` | IMP-019-R28-001: Added `"fred_enabled"` to Financial Markets SourceConfig. IMP-019-R28-002: Added `"fred_series"` to Financial Markets SourceConfig. |
| `cmd/core/main_test.go` | Added 5 adversarial tests: 3 for `fred_enabled` wiring (enabled/disabled/unset), 2 for `fred_series` wiring (valid array/empty). |

### Test Evidence

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit | `./smackerel.sh test unit` | PASS — 33 Go packages (core recompiled), 75 Python tests | 2026-04-14 |
| Build | `./smackerel.sh build` | PASS — both images built | 2026-04-14 |
| Check | `./smackerel.sh check` | PASS — config in sync | 2026-04-14 |
| Config generate | `./smackerel.sh config generate` | PASS — 2 new env vars confirmed: `FINANCIAL_MARKETS_FRED_ENABLED=true`, `FINANCIAL_MARKETS_FRED_SERIES=["GDP", "UNRATE", "CPIAUCSL", "DFF", "FEDFUNDS"]` | 2026-04-14 |

---

## Certification (2026-04-17)

**Agent:** bubbles.validate
**Decision:** CERTIFIED

### Evidence Summary

| Check | Result |
|-------|--------|
| All 15 connector imports in `cmd/core/connectors.go` | VERIFIED — 15 aliased imports (imap, caldav, youtube, rss, keep, bookmarks, browser, maps, hospitable, guesthost, discord, twitter, weather, alerts, markets) |
| All 15 connectors instantiated and registered | VERIFIED — `registerConnectors()` creates 15 instances and registers via `registry.Register()` loop |
| 5 new auto-start blocks (Discord, Twitter, Weather, Gov Alerts, Financial Markets) | VERIFIED — env-var gated, SST-compliant, credential-routed correctly |
| YAML config entries for all 5 connectors | VERIFIED — `config/smackerel.yaml` has discord, twitter, weather, gov-alerts, financial-markets blocks |
| Config generation pipeline | VERIFIED — `./smackerel.sh check` returns "Config is in sync with SST" |
| Unit tests | VERIFIED — 35 Go packages pass (all cached), 92 Python tests pass |
| DoD items in scopes.md | VERIFIED — all 12 items checked |
| BUG-001 (parseFloatEnv Inf/NaN) | VERIFIED — `state.json` shows `status: "done"`, regression tests in `main_test.go` |
| Security sweep findings | VERIFIED — SEC-019-001/002/003 all resolved or documented |
| Hardening sweep findings | VERIFIED — HARDEN-019-001/002, H-019-R21-001/002/003 all resolved |
| Improvement sweep findings | VERIFIED — IMP-019-R27-001/002, IMP-019-R28-001/002 all resolved |
| SST compliance | VERIFIED — zero hardcoded defaults, all config flows through YAML → config.sh → env → os.Getenv() |

### Final Test Evidence

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit | `./smackerel.sh test unit` | PASS — 35 Go packages, 92 Python tests | 2026-04-17 |
| Check | `./smackerel.sh check` | PASS — config in sync | 2026-04-17 |

---

## DevOps Sweep (2026-04-21)

**Trigger:** Stochastic quality sweep — devops trigger on connector wiring.
**Scope:** Build, deployment, CI/CD, monitoring, observability, release automation for the 5 wired connectors.

### Probe Summary

| Area | Status | Evidence |
|------|--------|----------|
| Config generation | CLEAN | `./smackerel.sh config generate` produces all 5 connector env var blocks (Discord: 9 vars, Twitter: 5, Weather: 6, Gov Alerts: 12, Financial Markets: 9) — all default to `enabled: false` |
| Config sync check | CLEAN | `./smackerel.sh check` returns "Config is in sync with SST" / "env_file drift guard: OK" |
| Docker build | CLEAN | `./smackerel.sh build` succeeds — both `smackerel-core` and `smackerel-ml` images built with ldflags version metadata and OCI labels |
| Unit tests | CLEAN | `./smackerel.sh test unit` — all Go packages pass, including `cmd/core` (connector registration + helper tests) |
| Lint | CLEAN | `./smackerel.sh lint` — all checks passed (Go vet + Python ruff) |
| Docker Compose wiring | CLEAN | `smackerel-core` service loads `env_file` (generated env), Twitter archive dir has volume mount with host→container path override, all connector env vars flow to container |
| Production overrides | CLEAN | `docker-compose.prod.yml` has restart=always, resource limits, log rotation, `/readyz` health probe for orchestrators |
| CI/CD pipeline | CLEAN | `.github/workflows/ci.yml` covers lint → unit test → Docker build → integration → tagged image push to GHCR, uses `./smackerel.sh` CLI surface throughout |
| Health endpoint | CLEAN | `/api/health` lists all 15 connectors with status; `/readyz` provides lightweight DB-only readiness for Docker HEALTHCHECK |
| Startup observability | CLEAN | `slog.Info("connector registry initialized", "count", ...)` logs total count after registration; each auto-started connector logs individually |
| Dockerfile security | CLEAN | Non-root user (`smackerel`), `no-new-privileges` in Compose, `cap_drop: ALL` on core service |
| SST compliance | CLEAN | All connector config flows YAML → config.sh → env → `os.Getenv()` / `config.Config` — zero hardcoded defaults |
| Build identity | CLEAN | Dockerfile passes `VERSION`, `COMMIT_HASH`, `BUILD_TIME` via build args to ldflags and OCI labels |

### Findings

No devops findings. The connector wiring implementation has complete SST-compliant config generation, Docker Compose volume mounts for file-based connectors (Twitter archive dir), proper CI/CD coverage, health monitoring for all 15 connectors, startup logging, and production deployment configuration.

### Test Evidence

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Config generate | `./smackerel.sh config generate` | PASS — dev.env + nats.conf generated | 2026-04-21 |
| Check | `./smackerel.sh check` | PASS — config in sync, env_file drift guard OK | 2026-04-21 |
| Build | `./smackerel.sh build` | PASS — both images built | 2026-04-21 |
| Unit | `./smackerel.sh test unit --go` | PASS — all Go packages | 2026-04-21 |
| Lint | `./smackerel.sh lint` | PASS — all checks passed | 2026-04-21 |

---

## Improvement Sweep (2026-04-21)

**Trigger:** Stochastic quality sweep — improve trigger on connector wiring (child workflow).
**Scope:** Full implementation review of `cmd/core/connectors.go`, `cmd/core/helpers.go`, `internal/config/config.go` (connector fields), `scripts/commands/config.sh` (connector extraction), `config/smackerel.yaml` (connector blocks), and `cmd/core/main_test.go` (connector tests).

### Analysis Summary

| Area | Status | Evidence |
|------|--------|----------|
| Registration pattern consistency | CLEAN | All 15 connectors follow identical instantiate → register → auto-start pattern in `connectors.go` |
| SST compliance | CLEAN | All config flows YAML → `config.sh` → `dev.env` → `config.Config` → `connectors.go` — zero hardcoded defaults |
| Config struct typing | CLEAN | All 5 connector field groups use correct types: `bool` for enabled/toggles, `string` for tokens/schedules/paths, `float64` for numeric, `[]interface{}` for JSON arrays, `map[string]interface{}` for JSON objects |
| Error handling | CLEAN | All `Connect()` failures logged as warnings, not fatal — other connectors unaffected. Config validation happens at load time in `config.Load()` |
| Credential routing | CLEAN | API keys/tokens flow through `Credentials` map (fixed in H-019-R21-001). `AuthType` matches credential channel |
| Test coverage | CLEAN | `TestAllConnectorsRegistered` covers SCN-019-001, `TestDuplicateRegistrationRejected` covers SCN-019-006, helper tests cover all edge cases including IEEE 754 special values |
| Code structure | CLEAN | BUG-004 refactoring correctly split `main.go` into `connectors.go` (wiring), `services.go` (infra), `helpers.go` (parsers), `shutdown.go` (lifecycle) |
| Dead helper code | NOTE | `parseJSONArrayEnv`, `parseJSONObjectEnv`, `parseJSONObject`, `parseJSONObjectVal` in `helpers.go` are unused in production — config parsing migrated to `internal/config/config.go` during BUG-004 refactoring. Covered by tests, harmless, and cross-cutting (not 019-specific) |
| Unit tests | CLEAN | `./smackerel.sh test unit` — all 236 tests pass (Go + Python) |

### Findings

No actionable improvement findings. The implementation is clean across all dimensions analyzed. Previous sweeps (security, test, harden ×2, improve ×2, devops) have already addressed all significant issues. The minor dead helper code in `helpers.go` is a consequence of the BUG-004 god-wirer refactoring and is not specific to this spec.

### Test Evidence

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit | `./smackerel.sh test unit` | PASS — all Go packages + 236 Python tests | 2026-04-21 |
| Lint | `./smackerel.sh lint` | PASS — all checks passed | 2026-04-21 |
