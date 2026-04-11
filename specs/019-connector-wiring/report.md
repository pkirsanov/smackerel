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
| 14 connector imports in `cmd/core/main.go` | YES |
| 14 connector instantiations with correct IDs | YES |
| 14 connector registrations with registry | YES |
| 5 auto-start blocks (Discord, Twitter, Weather, Gov Alerts, Financial Markets) | YES |
| 4 new YAML config blocks in `smackerel.yaml` (Twitter, Weather, Gov Alerts, Financial Markets) | YES |
| Discord YAML block already existed | YES |
| Config generation produces all connector env vars in `dev.env` | YES — 27 new env vars confirmed |
| `ConnectorHealthLister` interface on registry, wired into `/api/health` | YES |
| All 5 connectors default to `enabled: false` | YES |
| Helper functions `parseJSONArray`, `parseJSONObject`, `parseFloatEnv` in `main.go` | YES |
| No hardcoded fallback defaults in `main.go` auto-start blocks | YES — all read from `os.Getenv()` |
| Existing 9 connectors unaffected | YES — tests cached, no source changes |

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

Scope 1 implementation is complete. All 14 connectors are imported, instantiated, registered, and conditionally startable. Config generation produces correct env vars. CoinGecko SST gap (DRIFT-002) and DISCORD_CAPTURE_COMMANDS fragility (DRIFT-004) remediated. All unit tests pass.
