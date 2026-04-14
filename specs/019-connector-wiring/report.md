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
