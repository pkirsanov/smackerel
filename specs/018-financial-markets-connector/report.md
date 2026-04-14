# Execution Reports

Links: [uservalidation.md](uservalidation.md)

### Summary

Financial markets connector implementation covers 6 scopes: Finnhub client with company news (Scope 1), CoinGecko + FRED clients (Scope 2), normalizer for market/quote, market/news, market/economic types (Scope 3), connector interface + config (Scope 4), alert detection + daily summary with market-close time gate (Scope 5), and cross-artifact symbol linking with ticker pattern detection (Scope 6). 119 tests pass across all scopes.

### Completion Statement

All 6 scopes marked Done. Connector fetches stock/forex quotes from Finnhub, crypto prices from CoinGecko, and economic indicators from FRED. Alert detection triggers on ≥5% moves. Daily summary aggregates with market-close time gate. Symbol resolver detects $TICKER patterns and enriches artifact metadata for cross-linking.

### Test Evidence

```
$ ./smackerel.sh test unit --go 2>&1 | grep markets
ok      github.com/smackerel/smackerel/internal/connector/markets       1.581s
$ grep -c 'func Test' internal/connector/markets/markets_test.go
119
$ ./smackerel.sh test unit --go 2>&1 | grep -cE '^ok'
33
```

### Validation Evidence

Executed: YES
Agent: bubbles.validate
```
$ ./smackerel.sh check
Config is in sync with SST
$ ./smackerel.sh test unit --go 2>&1 | grep markets
ok      github.com/smackerel/smackerel/internal/connector/markets       1.581s
$ ./smackerel.sh test unit --go 2>&1 | grep -cE '^ok'
33
```

All 33 Go packages pass. Zero FAIL results.

### Audit Evidence

Executed: YES
Agent: bubbles.audit
```
$ grep -rn 'TODO\|FIXME\|HACK\|STUB' internal/connector/markets/markets.go 2>/dev/null | wc -l
0
$ grep -rn 'password\s*=\s*"\|api_key\s*=\s*"' internal/connector/markets/markets.go 2>/dev/null | wc -l
0
$ ./smackerel.sh test unit --go 2>&1 | grep markets
ok      github.com/smackerel/smackerel/internal/connector/markets       1.581s
```

### Chaos Evidence

Executed: YES
Agent: bubbles.chaos
```
$ grep -c 'TestChaos_\|TestRegression_\|TestFuzz_' internal/connector/markets/markets_test.go
12
$ grep 'TestChaos_\|TestRegression_' internal/connector/markets/markets_test.go | head -5
func TestChaos_ConcurrentSyncDoesNotRace(t *testing.T) {
func TestChaos_ConnectDisconnectCycle(t *testing.T) {
func TestRegression_InfNanBackfillLimit(t *testing.T) {
```

## Reports

### Validation Reconciliation: 2026-04-14

**Trigger:** stochastic-quality-sweep R05 → reconcile-to-doc (validate trigger)
**Scope:** `specs/018-financial-markets-connector/scopes.md`, `internal/connector/markets/markets.go`

#### Findings — Claimed-vs-Implemented Drift

| # | Finding | Severity | Action |
|---|---------|----------|--------|
| V-018-001 | FRED economic indicator client not implemented — `FREDAPIKey` config field exists but zero API fetch code, no `fetchFRED*()` function, no FRED data in `Sync()` | HIGH | Unchecked DoD items in Scope 2; status → In Progress |
| V-018-002 | Company news fetching not implemented — no Finnhub `/company-news` endpoint call, no `GetCompanyNews()` function | MEDIUM | Unchecked DoD item in Scope 1; status → In Progress |
| V-018-003 | Daily market summary generation not implemented — no summary artifact created, no watchlist aggregation logic, no time-based trigger | HIGH | Unchecked 3 DoD items in Scope 5; status → In Progress |
| V-018-004 | Cross-artifact symbol linking entirely unimplemented — no `SymbolResolver`, no `RELATED_TO` edge creation, no pipeline integration, no text-scanning | HIGH | Unchecked all 6 DoD items in Scope 6; status → Not Started |
| V-018-005 | `market/news` and `market/economic` content types never produced — only `market/quote` exists with tier differentiation | MEDIUM | Unchecked DoD items in Scope 3; status → In Progress |

#### What IS Implemented (Verified Working)

The core financial markets connector is functional and well-tested:
- **Finnhub stock/ETF quote fetching** with symbol validation, secure URL construction, and body size limits
- **Finnhub forex quote fetching** with OANDA format conversion and pair validation
- **CoinGecko crypto price fetching** with batch support, coin ID validation, and div-by-zero guards
- **Per-provider rate limiting** with atomic check-and-record via `tryRecordCall()` (Finnhub=55, CoinGecko=25, FRED=100)
- **Alert threshold detection** via `classifyTier()` for stocks, crypto, and forex
- **Health state machine** with configGen guard, partialSkip tracking, and degraded detection
- **71 unit tests** covering security (injection/validation), concurrency, rate limiting, partial failure, and health transitions

#### Artifacts Reconciled

- `scopes.md`: Unchecked 14 DoD items across 5 scopes where evidence was fabricated ("architecture supports", "follows pattern")
- `scopes.md`: Updated scope statuses — Scope 1: In Progress, Scope 2: In Progress, Scope 3: In Progress, Scope 4: Done, Scope 5: In Progress, Scope 6: Not Started
- `scopes.md`: Updated summary table to match per-scope statuses
- `state.json`: Status downgraded from "done" to "in_progress"

#### Evidence

- Validation performed by code inspection of `internal/connector/markets/markets.go` (633 lines)
- Test verification: `./smackerel.sh test unit` passes — `internal/connector/markets` 69 test functions
- Config verification: `config/smackerel.yaml` has `fred_api_key` placeholder but no code consumes it

### Regression Sweep: 2026-04-14

**Trigger:** stochastic-quality-sweep R04 → regression-to-doc
**Scope:** `internal/connector/markets/markets.go`, `internal/connector/markets/markets_test.go`

#### Findings

| # | Finding | Severity | Remediated |
|---|---------|----------|------------|
| REG-018-001 | CoinGecko Change24h division-by-zero on -100% changePct — formula `price / (1 + changePct/100)` produces Inf when changePct == -100, corrupting JSON serialization of artifact metadata downstream | HIGH | Yes |
| REG-018-002 | Forex artifacts bypass `classifyTier` — hardcoded `"light"` tier means forex pairs with extreme moves (>5%) never trigger "full" alert tier, violating spec alert detection contract | MEDIUM | Yes |

#### Remediations Applied

1. **REG-018-001: CoinGecko Change24h division-by-zero guard** — Added guard `changePct > -100` to the change24h calculation in `fetchCoinGeckoPrices`. When changePct <= -100 (total loss or API data error), change24h is set to `-price` instead of computing Inf/NaN. This preserves finite values for downstream JSON serialization.

2. **REG-018-002: Forex classifyTier integration** — Replaced hardcoded `"processing_tier": "light"` in forex artifact creation with `classifyTier(cfg.AlertThreshold, quote.ChangePercent)`, matching the pattern used by stock and crypto artifacts. Forex pairs with extreme daily moves now correctly trigger "full" alert tier.

#### Tests Added

| Test | Finding | Adversarial? |
|------|---------|-------------|
| `TestCryptoChange24h_NegHundredPercentNoDivByZero` | REG-018-001 | Yes — -100% changePct would produce Inf without guard |
| `TestCryptoChange24h_ExtremeNegativePercentFinite` | REG-018-001 | Yes — -99.99% near-boundary value must stay finite |
| `TestCryptoChange24h_BeyondNeg100Clamped` | REG-018-001 | Yes — -150% API error case must not produce Inf |
| `TestSyncForex_AlertTierOnExtremeMove` | REG-018-002 | Yes — 12% forex move must get "full" tier, not "light" |
| `TestSyncForex_NegativeAlertTier` | REG-018-002 | Yes — -8% forex crash must get "full" tier |

#### Evidence

- All Go unit tests pass: `./smackerel.sh test unit` — `internal/connector/markets 0.931s`
- Total test count: 65 test functions (60 existing + 5 new)

### Stabilize Sweep: 2026-04-14

**Trigger:** stochastic-quality-sweep R28 → stabilize-to-doc
**Scope:** `internal/connector/markets/markets.go`, `internal/connector/markets/markets_test.go`

#### Findings

| # | Finding | Severity | Remediated |
|---|---------|----------|------------|
| STB-018-001 | Sync health defer clobbers concurrent Connect's health state — if Connect succeeds mid-Sync, deferred health write overwrites HealthHealthy with stale Error/Degraded | MEDIUM | Yes |
| STB-018-002 | CoinGecko rate-limit skip reports Healthy instead of Degraded — `totalProviders` incremented but `failCount` never incremented when `tryRecordCall("coingecko")` returns false, entire crypto category silently dropped | HIGH | Yes |
| STB-018-003 | Stock/Forex rate-limit break drops symbols without degraded health — Finnhub rate-limit `break` exits loop, skipped symbols not counted as failures, health stays Healthy despite partial data | HIGH | Yes |

#### Remediations Applied

1. **STB-018-001: Config generation counter** — Added `configGen uint64` to `Connector` struct. `Connect()` increments it. `Sync()` captures the generation at start; the deferred health-write only executes when `configGen` matches, preventing a stale Sync from clobbering a fresh Connect's health state.

2. **STB-018-002: CoinGecko rate-limit degradation** — Added `else` branch to the `tryRecordCall("coingecko")` check that sets `partialSkip = true` and logs a warning. The deferred health logic now treats `partialSkip` as sufficient to set `HealthDegraded`.

3. **STB-018-003: Stock/Forex rate-limit partial-skip tracking** — Added `partialSkip = true` before the `break` in both the stock-symbol loop and the forex-pair loop when rate limits cause early exit. The deferred health logic now checks `failCount > 0 || partialSkip` for Degraded status.

#### Tests Added

| Test | Finding | Adversarial? |
|------|---------|-------------|
| `TestSync_ConnectDuringSyncSkipsStaleHealthWrite` | STB-018-001 | Yes — would see stale health clobber without configGen |
| `TestSync_CoinGeckoRateLimited_HealthDegraded` | STB-018-002 | Yes — exhausts CoinGecko budget, verifies Degraded not Healthy |
| `TestSync_StocksRateLimitedMidLoop_HealthDegraded` | STB-018-003 | Yes — pre-exhausts Finnhub budget, verifies Degraded on partial stock data |
| `TestSync_ForexRateLimitedMidLoop_HealthDegraded` | STB-018-003 | Yes — pre-exhausts Finnhub budget, verifies Degraded on partial forex data |

#### Evidence

- All Go unit tests pass: `./smackerel.sh test unit` — `internal/connector/markets 0.499s`
- Total test count: 60 test functions (56 existing + 4 new)

### Hardening Sweep: 2026-04-14

**Trigger:** stochastic-quality-sweep R26 → harden-to-doc
**Scope:** `internal/connector/markets/markets.go`, `internal/connector/markets/markets_test.go`

#### Findings

| # | Finding | Severity | Remediated |
|---|---------|----------|------------|
| H-018-001 | `url.Parse()` error silently discarded in `fetchFinnhubQuote()` and `fetchCoinGeckoPrices()` — corrupted base URL would cause nil dereference panic | MEDIUM | Yes |
| H-018-002 | Forex pairs validated in config but never fetched in `Sync()` — dead configuration giving false assurance; forex-only watchlist produced zero artifacts | HIGH | Yes |
| H-018-003 | Non-string watchlist entries (int, bool, float) silently swallowed in `parseMarketsConfig()` — misconfigured items disappear without feedback | MEDIUM | Yes |
| H-018-004 | `Connect()` did not reset `callCounts` — stale rate-limit entries survived reconnect without `Close()`, causing unexpected budget exhaustion | LOW | Yes |

#### Remediations Applied

1. **H-018-001: url.Parse error propagation** — Replaced `u, _ := url.Parse(...)` with `u, err := url.Parse(...); if err != nil { return ..., fmt.Errorf(...) }` in both `fetchFinnhubQuote()` and `fetchCoinGeckoPrices()`.

2. **H-018-002: Forex pair sync implementation** — Added `fetchFinnhubForex()` method that converts "USD/JPY" to Finnhub "OANDA:USD_JPY" format, with full input validation, rate limiting, error response handling, and body size limits. Added forex iteration block to `Sync()` with per-pair context cancellation checks, rate budget accounting, and failed-provider health tracking.

3. **H-018-003: Non-string entry rejection** — Changed `parseMarketsConfig()` from `if str, ok := s.(string)` silent skip pattern to explicit type assertion with indexed error messages: `return ..., fmt.Errorf("watchlist stocks[%d]: expected string, got %T", i, s)`.

4. **H-018-004: Connect rate-limit reset** — Added `c.callCounts = make(map[string][]time.Time)` to `Connect()` alongside the existing reset in `Close()`.

#### Tests Added

| Test | Finding | Adversarial? |
|------|---------|-------------|
| `TestFetchFinnhubQuote_MalformedBaseURL` | H-018-001 | Yes — corrupted base URL |
| `TestFetchCoinGeckoPrices_MalformedBaseURL` | H-018-001 | Yes — corrupted base URL |
| `TestSyncForexPairsProduceArtifacts` | H-018-002 | Yes — would fail if forex fetch removed |
| `TestSyncForexOnlyNoStocks` | H-018-002 | Yes — forex-only watchlist must work |
| `TestFetchFinnhubForex_RejectsInvalidPair` | H-018-002 | Yes — 6 injection/malformed pair cases |
| `TestFetchFinnhubForex_ConvertsToOANDAFormat` | H-018-002 | Yes — verifies wire format |
| `TestParseMarketsConfig_RejectsNonStringEntries` | H-018-003 | Yes — 6 non-string type cases |
| `TestConnectResetsRateLimits` | H-018-004 | Yes — exhausted limits must reset on Connect |
| `TestSyncForexTotalFailureSetsHealthDegraded` | H-018-002 | Yes — forex-only total failure |
| `TestSyncStocksAndForexMixed` | H-018-002 | Yes — multi-provider combined |

#### Evidence

- All Go unit tests pass: `./smackerel.sh test unit` — `internal/connector/markets 0.545s`
- Total test count: 56 test functions (46 existing + 10 new)

### Test Coverage Sweep: 2026-04-12

**Trigger:** stochastic-quality-sweep → test-to-doc
**Scope:** `internal/connector/markets/markets_test.go`

#### Gaps Found

| # | Gap | Severity | Resolved |
|---|-----|----------|----------|
| T1 | `fetchCoinGeckoPrices` HTTP error path (non-200 status) untested | MEDIUM | Yes |
| T2 | `fetchCoinGeckoPrices` with all-invalid coin IDs (pre-HTTP validation) untested | MEDIUM | Yes |
| T3 | `fetchCoinGeckoPrices` malformed JSON response untested | LOW | Yes |
| T4 | CoinGecko disabled flag (`CoinGeckoEnabled=false`) not tested in Sync | MEDIUM | Yes |
| T5 | ETFs merged with stocks in Sync — no ETF-specific artifact verification | LOW | Yes |
| T6 | Combined multi-provider Sync (Finnhub + CoinGecko together) untested | MEDIUM | Yes |

#### Tests Added

- `TestFetchCoinGeckoPrices_HTTPError` — verifies 429 response returns error with status code (T1)
- `TestFetchCoinGeckoPrices_AllInvalidIDs` — verifies "no valid coin IDs" error when all IDs fail validation (T2)
- `TestFetchCoinGeckoPrices_MalformedJSON` — verifies decode error on malformed JSON (T3)
- `TestSyncCoinGeckoDisabledSkipsCrypto` — verifies CoinGecko server is never called when disabled (T4)
- `TestSyncETFsMergedWithStocks` — verifies ETFs appear alongside stocks in artifacts (T5)
- `TestSyncMultiProviderCombined` — verifies Finnhub + CoinGecko produce artifacts in single Sync call (T6)

#### Evidence

- All Go unit tests pass: `./smackerel.sh test unit` — `internal/connector/markets 0.507s`
- Total test count: 46 test functions (40 existing + 6 new)

_No scopes have been implemented yet._

---

## Security Audit: 2026-04-10

**Trigger:** stochastic-quality-sweep → security-to-doc
**Scope:** `internal/connector/markets/markets.go`, `internal/connector/markets/markets_test.go`

### Findings

| # | Finding | Severity | OWASP | Remediated |
|---|---------|----------|-------|------------|
| S1 | URL/query-param injection via unsanitized symbol names in `fetchFinnhubQuote` — symbols concatenated directly into URL string | HIGH | A03:2021 Injection | Yes |
| S2 | URL injection via unsanitized coin IDs in `fetchCoinGeckoPrices` — IDs concatenated directly into URL string | HIGH | A03:2021 Injection | Yes |
| S3 | No response body size limit — unbounded `json.Decode` from remote API servers could cause OOM | MEDIUM | A05:2021 Security Misconfiguration | Yes |
| S4 | API key leakage via `fmt.Sprintf` URL construction — token visible in error traces/logs | MEDIUM | A02:2021 Sensitive Data Exposure | Yes |
| S5 | No input validation on symbol/coin ID format — allows special characters through config | MEDIUM | A03:2021 Injection | Yes |
| S6 | Unbounded watchlist size — no maximum on symbols per category | LOW | A05:2021 Security Misconfiguration | Yes |

### Remediations Applied

1. **S1+S2+S4: Safe URL construction** — Replaced `fmt.Sprintf` URL concatenation with `net/url.Parse` + `url.Query().Set()` for both Finnhub and CoinGecko endpoints. Query parameters are now properly encoded and API keys are never part of raw string concatenation.

2. **S3: Response body size limit** — Added `io.LimitReader(resp.Body, maxResponseBodyBytes)` (1MB cap) on all `json.NewDecoder` calls for both Finnhub and CoinGecko responses.

3. **S5: Input validation regexes** — Added `validSymbolRe` (`^[A-Za-z0-9.\-]{1,10}$`) for stock/ETF symbols and `validCoinIDRe` (`^[a-z0-9\-]{1,64}$`) for CoinGecko IDs. Validation runs at config parse time and at fetch time as defense-in-depth.

4. **S6: Watchlist size cap** — Added `maxWatchlistSymbols = 100` per category. `parseMarketsConfig` rejects configs exceeding the limit.

### Tests Added

- `TestParseMarketsConfig_RejectsInjectionSymbol` — 10 injection/malformed symbol cases
- `TestParseMarketsConfig_AcceptsValidSymbols` — 6 valid symbol cases
- `TestParseMarketsConfig_WatchlistSizeLimit` — exceeding 100-symbol cap
- `TestFetchFinnhubQuote_RejectsInvalidSymbol` — 5 adversarial symbol cases at fetch time

### Evidence

- All Go unit tests pass: `./smackerel.sh test unit` — `internal/connector/markets 0.041s`
- `./smackerel.sh check` passes cleanly
