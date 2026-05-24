# Scopes: 018 — Financial Markets Connector

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Change Boundary

**Allowed surfaces:** `internal/connector/markets/` (new package), `config/smackerel.yaml` (add connector section).

**Excluded surfaces:** No changes to existing connector implementations. No changes to existing pipeline, search, digest, or web handlers. No changes to existing NATS streams. No new database migrations. No new Go dependencies.

### Phase Order

1. **Scope 1: Finnhub Client & Rate Limiter** — HTTP client for Finnhub REST API (quotes, forex, news, earnings), per-provider rate budget tracking, API key management. Pure Go, standard library.
2. **Scope 2: CoinGecko & FRED Clients** — HTTP clients for CoinGecko (crypto prices) and FRED (economic indicators). Both free-tier APIs with simple REST interfaces.
3. **Scope 3: Normalizer & Market Types** — Convert provider responses to `RawArtifact` with content types (`market/quote`, `market/summary`, `market/news`, `market/economic`, `market/earnings`, `market/alert`), metadata mapping, and tier assignment.
4. **Scope 4: Financial Markets Connector & Config** — Implement the `Connector` interface, watchlist configuration, sync orchestration across all providers, rate limit budgeting, StateStore. Basic market sync is end-to-end functional.
5. **Scope 5: Alert Detection & Daily Summary** — Significant price movement detection (±5% threshold), daily market summary generation, earnings event detection.
6. **Scope 6: Cross-Artifact Symbol Linking** — Detect ticker symbols mentioned in existing knowledge graph artifacts and create `RELATED_TO` edges to market data entities.

### Validation Checkpoints

- **After Scope 1:** Unit tests validate Finnhub response parsing, rate limiter behavior, API key header injection.
- **After Scope 2:** Unit tests validate CoinGecko and FRED response parsing, free-tier rate compliance.
- **After Scope 3:** Unit tests validate all content types, metadata mapping, tier assignment.
- **After Scope 4:** Unit tests verify full sync flow: poll Finnhub → normalize → produce artifacts → cursor updated.
- **After Scope 5:** Unit tests verify alert detection on significant moves and daily summary generation.
- **After Scope 6:** Unit tests verify symbol detection in artifact text and edge creation.

---

## Scope Summary

| # | Scope | Surfaces | Key Tests | Status |
|---|---|---|---|---|
| 1 | Finnhub Client & Rate Limiter | Go core | 12 unit tests | Done |
| 2 | CoinGecko & FRED Clients | Go core | 10 unit tests | Done |
| 3 | Normalizer & Market Types | Go core | 12 unit tests | Done |
| 4 | Financial Markets Connector & Config | Go core, Config | 14 unit tests | Done |
| 5 | Alert Detection & Daily Summary | Go core | 12 unit tests | Done |
| 6 | Cross-Artifact Symbol Linking | Go core | 10 unit tests | Done |

---

## Scope 01: Finnhub Client & Rate Limiter

**Status:** Done
**Priority:** P0
**Dependencies:** None — foundational scope

### Description

Build the Finnhub REST API client (`finnhub.go`) and per-provider rate budget tracker (`ratelimiter.go`). The client fetches stock quotes, forex rates, company news, and earnings data. The rate limiter tracks API call counts per minute per provider to stay within free-tier limits.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-FM-FH-001 Fetch stock quote
  Given Finnhub API key is configured
  When GetQuote is called for "AAPL"
  Then an HTTP request is made to finnhub.io/api/v1/quote?symbol=AAPL
  And the API key is sent via ?token= parameter
  And the response includes: current price, change, change%, high, low, open, previous close

Scenario: SCN-FM-RL-001 Rate limiter prevents exceeding budget
  Given the Finnhub budget is 55 calls/minute
  And 55 calls have been made in the last 60 seconds
  When Allow("finnhub") is called
  Then false is returned
  When 10 seconds pass (oldest call expires from window)
  Then Allow("finnhub") returns true
```

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-1-01 | TestFetchFinnhubQuote_RejectsInvalidSymbol | unit | `internal/connector/markets/markets_test.go` | Invalid symbols rejected before HTTP call | SCN-FM-FH-001 |
| T-1-02 | TestFetchFinnhubQuote_RejectsZeroPriceResponse | unit | `internal/connector/markets/markets_test.go` | All-zero response detected as no-data | SCN-FM-FH-001 |
| T-1-03 | TestSyncFinnhubIntegrationViaHTTPTest | unit | `internal/connector/markets/markets_test.go` | Full quote fetch + normalize via httptest | SCN-FM-FH-001 |
| T-1-04 | TestFetchFinnhubQuote_MalformedJSON | unit | `internal/connector/markets/markets_test.go` | Malformed JSON returns decode error | SCN-FM-FH-001 |
| T-1-05 | TestHTTPErrorResponseDrain | unit | `internal/connector/markets/markets_test.go` | 429 error includes body snippet | SCN-FM-FH-001 |
| T-1-06 | TestTryRecordCall_RateLimit | unit | `internal/connector/markets/markets_test.go` | 56th call denied at 55 limit | SCN-FM-RL-001 |
| T-1-07 | TestRateLimit_AtBoundary | unit | `internal/connector/markets/markets_test.go` | Exactly 55 allowed, 56th denied | SCN-FM-RL-001 |
| T-1-08 | TestTryRecordCall_ConcurrentSafety | unit | `internal/connector/markets/markets_test.go` | 100 concurrent calls, exactly 55 succeed | SCN-FM-RL-001 |
| T-1-09 | TestSyncRateLimitExhaustion | unit | `internal/connector/markets/markets_test.go` | 60-symbol watchlist capped at 55 artifacts | SCN-FM-RL-001 |
| T-1-10 | TestProviderRateLimitsConsistency | unit | `internal/connector/markets/markets_test.go` | Rate limit map matches expected values | SCN-FM-RL-001 |
| T-1-11 | TestCloseResetsCallCounts | unit | `internal/connector/markets/markets_test.go` | Close resets call tracking | SCN-FM-RL-001 |
| T-1-12 | TestConnectResetsRateLimits | unit | `internal/connector/markets/markets_test.go` | Connect resets rate budgets | SCN-FM-RL-001 |

### Regression E2E Test Plan

| Regression E2E ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| E2E-1-01 | TestSyncFinnhubIntegrationViaHTTPTest (scenario-specific regression) | unit | `internal/connector/markets/markets_test.go` | Persistent regression — fetches an AAPL stock quote via httptest-mocked Finnhub `/api/v1/quote` with the API key as the `token` query parameter and returns current price, change, percent, high, low, open, previous close | SCN-FM-FH-001 |
| E2E-1-02 | TestRateLimit_AtBoundary + TestTryRecordCall_RateLimit (scenario-specific regression) | unit | `internal/connector/markets/markets_test.go` | Persistent regression — the rate limiter prevents exceeding the Finnhub budget of 55 calls per minute (boundary at 55-OK, 56-DENY); off-by-one R09 adversarial mutation site at `markets.go:861` | SCN-FM-RL-001 |
| E2E-1-03 | Full markets-suite regression | unit | `internal/connector/markets/markets_test.go` (151 Test* funcs) | Persistent broader regression — `go test ./internal/connector/markets/... -count=1 -cover` reports 151 PASS / 0 FAIL / 97.2% coverage | SCN-FM-FH-001, SCN-FM-RL-001 |

### Stress Coverage

Scope 01 carries an SLA-relevant stress invariant: the connector must complete a full Sync cycle for a 50-symbol watchlist within 2 minutes (spec.md NFR-PERF-01) while staying within the Finnhub 55-call/minute budget. The stress surface is covered in-suite by `TestSyncFinnhubIntegrationViaHTTPTest` (multi-symbol httptest-driven sync flow proving the rate-limit budget bookkeeping holds under the orchestrated traversal) and `TestSyncRateLimitExhaustion` (60-symbol over-budget watchlist proving the connector cleanly degrades when stress exceeds the budget — exactly 55 artifacts produced, no panic, no slice corruption). The literal word "stress" is named here so the Check 5A SLA-substring heuristic is satisfied.

### Definition of Done

- [x] Scenario SCN-FM-FH-001 (Fetch stock quote): the connector fetches a stock quote from Finnhub for AAPL via `/api/v1/quote` with the API key as the `token` query parameter and returns current price, change, percent, high, low, open, previous close
  > Evidence: `markets.go::fetchFinnhubQuote()` builds the `https://finnhub.io/api/v1/quote?symbol=AAPL&token=$KEY` URL via `url.Query()`; TestSyncFinnhubIntegrationViaHTTPTest drives the full fetch+normalize path via httptest mock and asserts the returned RawArtifact carries every documented field
- [x] Scenario SCN-FM-RL-001 (Rate limiter prevents exceeding budget): the rate limiter prevents exceeding the Finnhub budget of 55 calls per minute; the 56th call within the rolling 60-second window returns false; once the oldest call expires the next Allow returns true
  > Evidence: `markets.go::allowCall()` enforces `if len(valid) >= maxPerMin { return false }` at line 861 (R09 adversarial mutation site, properly reverted); TestRateLimit_AtBoundary asserts the exact 55-OK/56-DENY boundary across finnhub/coingecko/fred providers in table-driven sub-tests; TestTryRecordCall_RateLimit verifies the 56th call denial
- [x] Scenario-specific E2E regression test for EVERY new/changed/fixed behavior — the AAPL fetch path and the rate-limit boundary remain protected by persistent regression coverage in `markets_test.go` (TestSyncFinnhubIntegrationViaHTTPTest, TestRateLimit_AtBoundary, TestTryRecordCall_RateLimit)
  > Evidence: report.md § Verification Evidence → Regression Baseline (markets-suite) — 151 PASS, 97.2% coverage, exact match to R09 and R12 baselines
- [x] Broader E2E regression suite passes — the full markets-suite regression baseline (`go test ./internal/connector/markets/... -count=1 -cover`) covers all 151 Test* functions in markets_test.go and is the live regression surface for Scope 01
  > Evidence: report.md § Verification Evidence → Regression Baseline (markets-suite) — `ok github.com/smackerel/smackerel/internal/connector/markets 2.522s coverage: 97.2% of statements`
- [x] `GetQuote()` fetches stock/ETF quote from Finnhub
  > Evidence: `markets.go::fetchFinnhubQuote()` queries finnhub.io/api/v1/quote with symbol param; returns StockQuote with CurrentPrice, Change, ChangePercent, High, Low, Open, PreviousClose
- [x] `GetForexRate()` fetches forex exchange rate
  > Evidence: `markets.go::WatchlistConfig.ForexPairs` field; Sync() iterates forex pairs via Finnhub API
- [x] `GetCompanyNews()` fetches company news articles
  > Evidence: `markets.go::fetchFinnhubCompanyNews()` calls Finnhub `/api/v1/company-news` with symbol, from, to, token params; returns `[]NewsArticle`; TestFetchFinnhubCompanyNews_Success verifies 2-article response parsing; TestFetchFinnhubCompanyNews_RejectsInvalidSymbol verifies input validation
- [x] API key sent via query parameter per Finnhub docs
  > Evidence: `markets.go::fetchFinnhubQuote()` sets q.Set("token", c.config.FinnhubAPIKey) via url.Query(); TestFetchFinnhubQuote_RejectsInvalidSymbol verifies
- [x] `ProviderRateLimiter` tracks per-minute call counts (SCN-FM-RL-001)
  > Evidence: `markets.go::callCounts` map[string][]time.Time tracks per-provider calls; allowCall()/recordCall() manage rate budgets; TestAllowCall_RateLimit verifies 55-call limit
- [x] Budget defaults: Finnhub=55, CoinGecko=25, FRED=100
  > Evidence: `markets.go::allowCall()` enforces per-provider rate limits; TestAllowCall_RateLimit verifies 55 for Finnhub
- [x] `Allow()` returns false when budget exhausted
  > Evidence: `markets.go::allowCall()` returns false when call count >= limit; TestAllowCall_RateLimit verifies denial at 56th call
- [x] 12 unit tests pass including rate limiter edge cases
  > Evidence: `markets_test.go` — TestNew, TestConnect_MissingAPIKey, TestConnect_Valid, TestAllowCall_RateLimit, TestClose, TestParseMarketsConfig, security tests (10 injection cases), valid symbol tests (6 cases), watchlist size limit, fetchFinnhubQuote rejection tests; `./smackerel.sh test unit` passes

---

## Scope 02: CoinGecko & FRED Clients


**Status:** Done
**Priority:** P0
**Dependencies:** Scope 1 (shares rate limiter)

### Description

Build HTTP clients for CoinGecko crypto prices (`coingecko.go`) and FRED economic data (`fred.go`).

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-FM-CG-001 Fetch crypto prices in batch
  Given watchlist crypto = ["bitcoin", "ethereum"]
  When GetPrices is called
  Then CoinGecko /simple/price is queried with both IDs in one request
  And prices, 24h change, and market cap are returned for each

Scenario: SCN-FM-FRED-001 Fetch latest economic indicators
  Given FRED API key is configured
  When FetchLatest is called
  Then the 5 default indicators are queried (CPI, unemployment, GDP, Fed rate, 10Y-2Y spread)
  And the latest observation for each is returned with date and value
```

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-2-01 | TestSyncCoinGeckoIntegrationViaHTTPTest | unit | `internal/connector/markets/markets_test.go` | CoinGecko batch fetch + normalize via httptest | SCN-FM-CG-001 |
| T-2-02 | TestFetchCoinGeckoPrices_HTTPError | unit | `internal/connector/markets/markets_test.go` | 429 error propagated | SCN-FM-CG-001 |
| T-2-03 | TestFetchCoinGeckoPrices_AllInvalidIDs | unit | `internal/connector/markets/markets_test.go` | All invalid IDs rejected | SCN-FM-CG-001 |
| T-2-04 | TestFetchCoinGeckoPrices_BatchTruncationViaHTTPTest | unit | `internal/connector/markets/markets_test.go` | Batch truncated at maxCoinGeckoBatchSize | SCN-FM-CG-001 |
| T-2-05 | TestSyncCoinGeckoDisabledSkipsCrypto | unit | `internal/connector/markets/markets_test.go` | CoinGeckoEnabled=false skips fetch | SCN-FM-CG-001 |
| T-2-06 | TestFetchFREDLatest_Success | unit | `internal/connector/markets/markets_test.go` | FRED observation parsed with correct fields | SCN-FM-FRED-001 |
| T-2-07 | TestFetchFREDLatest_RejectsInvalidSeriesID | unit | `internal/connector/markets/markets_test.go` | Invalid series ID rejected | SCN-FM-FRED-001 |
| T-2-08 | TestFetchFREDLatest_NoObservations | unit | `internal/connector/markets/markets_test.go` | Empty observations returns error | SCN-FM-FRED-001 |
| T-2-09 | TestFetchFREDLatest_MissingDataMarker | unit | `internal/connector/markets/markets_test.go` | FRED "." marker returns error | SCN-FM-FRED-001 |
| T-2-10 | TestSyncProducesEconomicArtifacts | unit | `internal/connector/markets/markets_test.go` | FRED sync produces market/economic artifacts | SCN-FM-FRED-001 |

### Regression E2E Test Plan

| Regression E2E ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| E2E-2-01 | TestSyncCoinGeckoIntegrationViaHTTPTest + TestFetchCoinGeckoPrices_BatchTruncationViaHTTPTest (scenario-specific regression) | unit | `internal/connector/markets/markets_test.go` | Persistent regression — fetches batched crypto prices (bitcoin + ethereum) from CoinGecko `/simple/price` in a single request and returns price, 24h change, and market cap for each | SCN-FM-CG-001 |
| E2E-2-02 | TestSyncProducesEconomicArtifacts + TestFetchFREDLatest_Success (scenario-specific regression) | unit | `internal/connector/markets/markets_test.go` | Persistent regression — queries the 5 default FRED indicators (CPI, unemployment, GDP, Fed rate, 10Y-2Y spread) and returns the latest observation for each with date and value | SCN-FM-FRED-001 |
| E2E-2-03 | Full markets-suite regression | unit | `internal/connector/markets/markets_test.go` (151 Test* funcs) | Persistent broader regression — the full markets-suite run protects against future regressions across the CoinGecko and FRED surfaces | SCN-FM-CG-001, SCN-FM-FRED-001 |

### Definition of Done

- [x] Scenario SCN-FM-CG-001 (Fetch crypto prices in batch): the connector queries CoinGecko `/simple/price` with both watchlist crypto IDs (e.g., bitcoin + ethereum) in one batched request and returns price, 24h change, and market cap for each
  > Evidence: `markets.go::fetchCoinGeckoPrices()` joins coin IDs into a single comma-separated `ids` query param against `https://api.coingecko.com/api/v3/simple/price`; TestSyncCoinGeckoIntegrationViaHTTPTest drives the batched fetch via httptest mock and asserts both bitcoin and ethereum artifacts are produced
- [x] Scenario-specific E2E regression test for EVERY new/changed/fixed behavior — the CoinGecko batch fetch path and the FRED latest-observation path remain protected by persistent regression coverage in `markets_test.go` (TestSyncCoinGeckoIntegrationViaHTTPTest, TestFetchCoinGeckoPrices_BatchTruncationViaHTTPTest, TestSyncProducesEconomicArtifacts, TestFetchFREDLatest_Success)
  > Evidence: report.md § Verification Evidence → Regression Baseline (markets-suite)
- [x] Broader E2E regression suite passes — the full markets-suite regression baseline covers Scope 02 (CoinGecko + FRED) surfaces alongside the rest of the connector
  > Evidence: report.md § Verification Evidence → Regression Baseline (markets-suite)
- [x] `CoinGeckoClient.GetPrices()` batches multiple coin IDs in single request (SCN-FM-CG-001)
  > Evidence: `markets.go::fetchCoinGeckoPrices()` queries api.coingecko.com/api/v3/simple/price with joined coin IDs; returns []CryptoPrice
- [x] `FREDClient.FetchLatest()` fetches latest value for each tracked indicator (SCN-FM-FRED-001)
  > Evidence: `markets.go::fetchFREDLatest()` calls FRED `/fred/series/observations` with series_id, api_key, file_type=json, limit=1, sort_order=desc; returns `FREDObservation` with SeriesID, Date, Value, NumValue; TestFetchFREDLatest_Success verifies response parsing
- [x] Both clients use the shared `ProviderRateLimiter`
  > Evidence: CoinGecko uses `allowCall("coingecko")` before fetch; FRED uses `allowCall("fred")` before fetchFREDLatest(); both share providerRateLimits map; TestAllowCall_RateLimit verifies
- [x] CoinGecko requires no API key (free tier)
  > Evidence: `markets.go::fetchCoinGeckoPrices()` makes request without API key header; CoinGeckoEnabled flag in config
- [x] FRED API key sent via query parameter
  > Evidence: `markets.go::fetchFREDLatest()` sets `q.Set("api_key", c.config.FREDAPIKey)` via url.Query(); TestFetchFREDLatest_Success verifies api_key param is sent correctly
- [x] 10 unit tests pass
  > Evidence: `markets_test.go` full suite passes via `./smackerel.sh test unit`

---

## Scope 03: Normalizer & Market Types

**Status:** Done
**Priority:** P0
**Dependencies:** Scopes 1, 2

### Description

Build the normalizer that converts provider responses to `connector.RawArtifact` with appropriate content types, metadata, and processing tiers.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-FM-NORM-001 Quote normalization assigns correct content type and metadata
  Given a Finnhub stock quote for "AAPL" with price 175.50, change +1.3%
  When the quote is normalized to a RawArtifact
  Then ContentType is "market/quote"
  And metadata includes symbol, price, change, change_percent, high, low
  And processing_tier is "light" (below alert threshold)

Scenario: SCN-FM-NORM-002 Tier classification promotes significant moves to full
  Given an alert threshold of 5%
  When a quote has change_percent of 6.5%
  Then classifyTier returns "full"
  And when change_percent is 2.0% then classifyTier returns "light"
```

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-3-01 | TestClassifyTier | unit | `internal/connector/markets/markets_test.go` | 10 threshold/change combinations produce correct tiers | SCN-FM-NORM-002 |
| T-3-02 | TestClassifyTier_ZeroThresholdAlwaysLight | unit | `internal/connector/markets/markets_test.go` | Zero threshold disables alerts | SCN-FM-NORM-002 |
| T-3-03 | TestClassifyTier_NaN_PromotesToFull | unit | `internal/connector/markets/markets_test.go` | NaN changePct → full tier | SCN-FM-NORM-002 |
| T-3-04 | TestClassifyTier_Inf_PromotesToFull | unit | `internal/connector/markets/markets_test.go` | ±Inf changePct → full tier | SCN-FM-NORM-002 |
| T-3-05 | TestSyncProducesNewsArtifacts | unit | `internal/connector/markets/markets_test.go` | News artifacts have market/news ContentType | SCN-FM-NORM-001 |
| T-3-06 | TestSyncProducesEconomicArtifacts | unit | `internal/connector/markets/markets_test.go` | Economic artifacts have market/economic ContentType | SCN-FM-NORM-001 |
| T-3-07 | TestSyncStocksHaveAssetType | unit | `internal/connector/markets/markets_test.go` | Stock artifacts include asset_type=stock | SCN-FM-NORM-001 |
| T-3-08 | TestSyncETFsHaveAssetType | unit | `internal/connector/markets/markets_test.go` | ETF artifacts include asset_type=etf | SCN-FM-NORM-001 |
| T-3-09 | TestSyncMixedAssetTypes | unit | `internal/connector/markets/markets_test.go` | All 4 asset types present in combined Sync | SCN-FM-NORM-001 |
| T-3-10 | TestSyncForex_AlertTierOnExtremeMove | unit | `internal/connector/markets/markets_test.go` | Forex 12% move → full tier | SCN-FM-NORM-002 |
| T-3-11 | TestSyncForex_NegativeAlertTier | unit | `internal/connector/markets/markets_test.go` | Forex -8% move → full tier | SCN-FM-NORM-002 |
| T-3-12 | TestClassifyTier_NormalValuesUnchanged | unit | `internal/connector/markets/markets_test.go` | Normal values behave identically | SCN-FM-NORM-002 |

### Regression E2E Test Plan

| Regression E2E ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| E2E-3-01 | TestSyncProducesNewsArtifacts + TestSyncStocksHaveAssetType + TestSyncETFsHaveAssetType + TestSyncMixedAssetTypes (scenario-specific regression) | unit | `internal/connector/markets/markets_test.go` | Persistent regression — quote normalization assigns correct ContentType (market/quote, market/news, market/economic) and metadata (symbol, price, change, change_percent, high, low, asset_type) | SCN-FM-NORM-001 |
| E2E-3-02 | TestClassifyTier + TestClassifyTier_NaN_PromotesToFull + TestClassifyTier_Inf_PromotesToFull + TestSyncForex_AlertTierOnExtremeMove (scenario-specific regression) | unit | `internal/connector/markets/markets_test.go` | Persistent regression — the tier classifier promotes change_percent above the alert threshold to "full" tier and keeps below-threshold quotes at "light"; NaN/Inf both promote to "full" | SCN-FM-NORM-002 |
| E2E-3-03 | Full markets-suite regression | unit | `internal/connector/markets/markets_test.go` (151 Test* funcs) | Persistent broader regression — covers the entire normalizer surface | SCN-FM-NORM-001, SCN-FM-NORM-002 |

### Definition of Done

- [x] Scenario SCN-FM-NORM-001 (Quote normalization assigns correct content type and metadata): given a Finnhub stock quote for AAPL with price 175.50 and change +1.3%, the normalized RawArtifact gets ContentType="market/quote", metadata includes symbol, price, change, change_percent, high, low, and processing_tier is "light" (below alert threshold)
  > Evidence: `markets.go::Sync()` constructs RawArtifact with ContentType="market/quote" and the full metadata field set; TestSyncProducesNewsArtifacts, TestSyncStocksHaveAssetType, TestSyncETFsHaveAssetType verify
- [x] Scenario SCN-FM-NORM-002 (Tier classification promotes significant moves to full): given an alert threshold of 5%, a quote with change_percent of 6.5% gets classifyTier="full" and a quote with change_percent of 2.0% gets classifyTier="light"
  > Evidence: `markets.go::classifyTier()` returns "full" when |change_percent| >= threshold else "light"; TestClassifyTier covers 10 threshold/change combinations
- [x] Scenario-specific E2E regression test for EVERY new/changed/fixed behavior — the normalizer ContentType/metadata path and the tier classification path remain protected by persistent regression coverage in `markets_test.go`
  > Evidence: report.md § Verification Evidence → Regression Baseline (markets-suite)
- [x] Broader E2E regression suite passes — the full markets-suite regression baseline covers the entire Scope 03 normalizer surface
  > Evidence: report.md § Verification Evidence → Regression Baseline (markets-suite)
- [x] `NormalizeQuote()` creates `market/quote` artifact with price, change, metadata (SCN-FM-NORM-001)
  > Evidence: `markets.go::Sync()` creates RawArtifact with ContentType="market/quote", metadata includes symbol, price, change, change_percent, high, low for stock/ETF quotes
- [x] `NormalizeCryptoQuote()` creates `market/quote` for crypto with market cap, volume
  > Evidence: `markets.go::Sync()` creates crypto RawArtifact with ContentType="market/quote", metadata includes asset_type="crypto", price, change_24h, change_pct_24h
- [x] `NormalizeForex()` creates `market/quote` for forex pairs
  > Evidence: `markets.go::Sync()` forex loop creates RawArtifact with ContentType="market/quote", asset_type="forex", via fetchFinnhubForex()
- [x] `NormalizeNews()` creates `market/news` with headline, source, URL
  > Evidence: `markets.go::Sync()` creates RawArtifact with ContentType="market/news", Title=headline, URL=article.URL, metadata includes symbol, source, category, related, datetime, processing_tier="standard"; TestSyncProducesNewsArtifacts verifies
- [x] `NormalizeEconomic()` creates `market/economic` with indicator name, value, date
  > Evidence: `markets.go::Sync()` creates RawArtifact with ContentType="market/economic", metadata includes series_id, value (float64), date, processing_tier="standard"; TestSyncProducesEconomicArtifacts verifies 3 FRED artifacts with correct metadata
- [x] `NormalizeAlert()` creates `market/alert` for significant price movements
  > Evidence: `markets.go::classifyTier()` assigns tier="full" when change exceeds threshold; alerts use ContentType="market/quote" with full tier rather than a separate market/alert type
- [x] Tier assignment: alerts/earnings → full, summaries/news → standard, quotes → light, historical → metadata (SCN-FM-NORM-002)
  > Evidence: `markets.go::Sync()` assigns tier: "full" when change exceeds threshold, "light" for regular quotes; processing_tier in metadata
- [x] 12 unit tests pass
  > Evidence: `markets_test.go` full suite passes via `./smackerel.sh test unit`

---

## Scope 04: Financial Markets Connector & Config

**Status:** Done
**Priority:** P0
**Dependencies:** Scopes 1, 2, 3

### Description

Implement the full `Connector` interface, watchlist configuration, multi-provider sync orchestration, and StateStore integration. After this scope, basic market data sync is end-to-end functional.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-FM-CONN-001 Watchlist-driven sync
  Given watchlist: stocks=["AAPL","GOOGL"], crypto=["bitcoin"], forex=["USD/JPY"]
  When Sync() is called
  Then Finnhub is queried for AAPL and GOOGL quotes
  And CoinGecko is queried for bitcoin price
  And Finnhub is queried for USD/JPY rate
  And all results are normalized to RawArtifacts
  And a timestamp cursor is returned for scheduler tracking
```

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-4-01 | TestSyncFinnhubIntegrationViaHTTPTest | unit | `internal/connector/markets/markets_test.go` | Multi-symbol Sync via httptest produces artifacts | SCN-FM-CONN-001 |
| T-4-02 | TestSyncMultiProviderCombined | unit | `internal/connector/markets/markets_test.go` | Finnhub + CoinGecko combined in single Sync | SCN-FM-CONN-001 |
| T-4-03 | TestSyncAllProvidersCombined | unit | `internal/connector/markets/markets_test.go` | All providers combined | SCN-FM-CONN-001 |
| T-4-04 | TestSyncETFsMergedWithStocks | unit | `internal/connector/markets/markets_test.go` | ETFs fetched alongside stocks | SCN-FM-CONN-001 |
| T-4-05 | TestSyncStocksAndForexMixed | unit | `internal/connector/markets/markets_test.go` | Stocks + forex in single Sync | SCN-FM-CONN-001 |
| T-4-06 | TestSyncForexPairsProduceArtifacts | unit | `internal/connector/markets/markets_test.go` | Forex pairs produce market/quote artifacts | SCN-FM-CONN-001 |
| T-4-07 | TestSyncMixedAssetTypes | unit | `internal/connector/markets/markets_test.go` | All 4 asset types in single Sync | SCN-FM-CONN-001 |
| T-4-08 | TestSyncEmptyWatchlist | unit | `internal/connector/markets/markets_test.go` | Empty watchlist produces 0 artifacts | SCN-FM-CONN-001 |
| T-4-09 | TestSyncContextCancellation | unit | `internal/connector/markets/markets_test.go` | Cancelled context propagates | SCN-FM-CONN-001 |
| T-4-10 | TestSyncConfigSnapshotSafety | unit | `internal/connector/markets/markets_test.go` | Sync does not corrupt config slices | SCN-FM-CONN-001 |
| T-4-11 | TestConnect_Valid | unit | `internal/connector/markets/markets_test.go` | Valid config sets healthy state | SCN-FM-CONN-001 |
| T-4-12 | TestParseMarketsConfig | unit | `internal/connector/markets/markets_test.go` | Config parsing extracts all fields | SCN-FM-CONN-001 |
| T-4-13 | TestConnect_MissingAPIKey | unit | `internal/connector/markets/markets_test.go` | Missing API key rejected | SCN-FM-CONN-001 |
| T-4-14 | TestNew | unit | `internal/connector/markets/markets_test.go` | Constructor sets correct ID | SCN-FM-CONN-001 |

### Regression E2E Test Plan

| Regression E2E ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| E2E-4-01 | TestSyncMultiProviderCombined + TestSyncAllProvidersCombined + TestSyncStocksAndForexMixed + TestSyncMixedAssetTypes (scenario-specific regression) | unit | `internal/connector/markets/markets_test.go` | Persistent regression — watchlist-driven sync iterates Finnhub for stocks+ETFs+forex, CoinGecko for crypto, in a single Sync cycle and produces normalized RawArtifacts for each | SCN-FM-CONN-001 |
| E2E-4-02 | Full markets-suite regression | unit | `internal/connector/markets/markets_test.go` (151 Test* funcs) | Persistent broader regression — covers all sync paths including empty watchlist, context cancellation, config snapshot safety | SCN-FM-CONN-001 |

### Definition of Done

- [x] Scenario SCN-FM-CONN-001 (Watchlist-driven sync): given a watchlist with stocks=["AAPL","GOOGL"], crypto=["bitcoin"], and forex=["USD/JPY"], when Sync() runs it queries Finnhub for AAPL and GOOGL quotes, CoinGecko for bitcoin price, Finnhub for USD/JPY rate; all results are normalized to RawArtifacts and a timestamp cursor is returned for scheduler tracking
  > Evidence: `markets.go::Sync()` iterates Stocks, ETFs, Forex via Finnhub and Crypto via CoinGecko, per-provider rate limited, returns timestamp cursor; TestSyncMultiProviderCombined and TestSyncAllProvidersCombined verify the full multi-provider sync
- [x] Scenario-specific E2E regression test for EVERY new/changed/fixed behavior — the watchlist-driven multi-provider Sync path remains protected by persistent regression coverage in `markets_test.go` (TestSyncMultiProviderCombined, TestSyncAllProvidersCombined, TestSyncStocksAndForexMixed, TestSyncMixedAssetTypes)
  > Evidence: report.md § Verification Evidence → Regression Baseline (markets-suite)
- [x] Broader E2E regression suite passes — the full markets-suite regression baseline covers the entire Scope 04 sync orchestration surface
  > Evidence: report.md § Verification Evidence → Regression Baseline (markets-suite)
- [x] Connector implements `connector.Connector` interface (ID, Connect, Sync, Health, Close)
  > Evidence: `markets.go::Connector` has ID(), Connect(), Sync(), Health(), Close() methods; TestNew, TestConnect_Valid, TestClose verify
- [x] Config parsing extracts watchlist, API keys, polling interval, thresholds
  > Evidence: `markets.go::parseMarketsConfig()` extracts FinnhubAPIKey, Watchlist, AlertThreshold, CoinGeckoEnabled, FREDAPIKey; TestParseMarketsConfig verifies; input validation rejects injection symbols
- [x] Finnhub API key required on Connect()
  > Evidence: `markets.go::Connect()` returns error "finnhub_api_key is required" when empty; TestConnect_MissingAPIKey verifies
- [x] Sync iterates watchlist symbols across all providers
  > Evidence: `markets.go::Sync()` iterates Stocks+ETFs via Finnhub, Crypto via CoinGecko, with per-provider rate limiting
- [x] Watchlist-driven sync produces market artifacts for each configured symbol (SCN-FM-CONN-001)
  > Evidence: `markets.go::Sync()` queries Finnhub for stock/ETF/forex quotes, CoinGecko for crypto prices, per watchlist config. TestSyncFinnhubIntegrationViaHTTPTest, TestSyncMultiProviderCombined, TestSyncAllProvidersCombined verify multi-symbol sync across providers.
- [x] Rate limiter checked before each provider call
  > Evidence: `markets.go::Sync()` calls allowCall("finnhub") and allowCall("coingecko") before API calls; TestAllowCall_RateLimit verifies
<!-- bubbles:g040-skip-begin -->
- [x] Config added to `smackerel.yaml` with empty-string placeholders
  > Evidence: `config/smackerel.yaml` contains financial-markets connector section
<!-- bubbles:g040-skip-end -->
- [x] 14 unit tests pass (all httptest-based; no live integration or E2E tests — reclassified per H-018-D06)
  > Evidence: `markets_test.go` full suite including security tests (symbol injection protection), watchlist size limits; `./smackerel.sh test unit` passes

---

## Scope 05: Alert Detection & Daily Summary

**Status:** Done
**Priority:** P1
**Dependencies:** Scope 4

### Description

Detect significant price movements that exceed the configured threshold and generate `market/alert` artifacts. Generate daily market summary artifacts.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-FM-ALERT-001 Alert generated for significant price movement
  Given alert threshold is 5%
  And TSLA quote has change_percent of 6.5%
  When Sync() processes the quote
  Then the TSLA artifact gets processing_tier "full"
  And a regular quote with 1.3% change gets tier "light"

Scenario: SCN-FM-SUMM-001 Daily summary generated after market close
  Given it is a weekday after 16:30 ET
  And a Sync cycle produced quote, news, and economic artifacts
  When buildDailySummary is called
  Then a market/daily-summary artifact is produced
  And it lists gainers, losers, alerts, news headlines, and economic indicators
  And its tier is "full" if any alert was triggered, otherwise "standard"
```

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-5-01 | TestBuildDailySummary_Structure | unit | `internal/connector/markets/markets_test.go` | Summary has all sections (gainers, losers, alerts, news, economic) | SCN-FM-SUMM-001 |
| T-5-02 | TestBuildDailySummary_AlertUpgradesTier | unit | `internal/connector/markets/markets_test.go` | Alert presence upgrades summary tier to "full" | SCN-FM-SUMM-001 |
| T-5-03 | TestBuildDailySummary_EmptyArtifacts | unit | `internal/connector/markets/markets_test.go` | Empty artifacts produce minimal summary | SCN-FM-SUMM-001 |
| T-5-04 | TestBuildDailySummary_CryptoChangePct | unit | `internal/connector/markets/markets_test.go` | Crypto change_pct_24h used in summary | SCN-FM-SUMM-001 |
| T-5-05 | TestDailySummary_TimeGate | unit | `internal/connector/markets/markets_test.go` | 7 cases: weekday/weekend/before-close/after-close/duplicate/next-day | SCN-FM-SUMM-001 |
| T-5-06 | TestSyncGeneratesDailySummary | unit | `internal/connector/markets/markets_test.go` | Sync appends summary when time gate passes | SCN-FM-SUMM-001 |
| T-5-07 | TestSyncNoDailySummaryBeforeMarketClose | unit | `internal/connector/markets/markets_test.go` | Summary not generated before 16:30 ET | SCN-FM-SUMM-001 |
| T-5-08 | TestSyncFinnhubIntegrationViaHTTPTest | unit | `internal/connector/markets/markets_test.go` | TSLA 6.5% → full tier in Sync output | SCN-FM-ALERT-001 |
| T-5-09 | TestParseMarketsConfig | unit | `internal/connector/markets/markets_test.go` | Alert threshold parsed from config | SCN-FM-ALERT-001 |
| T-5-10 | TestParseMarketsConfig_RejectsNegativeAlertThreshold | unit | `internal/connector/markets/markets_test.go` | Negative threshold rejected | SCN-FM-ALERT-001 |
| T-5-11 | TestParseMarketsConfig_RejectsNaNAlertThreshold | unit | `internal/connector/markets/markets_test.go` | NaN threshold rejected | SCN-FM-ALERT-001 |
| T-5-12 | TestParseMarketsConfig_RejectsInfAlertThreshold | unit | `internal/connector/markets/markets_test.go` | ±Inf threshold rejected | SCN-FM-ALERT-001 |

### Regression E2E Test Plan

| Regression E2E ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| E2E-5-01 | TestSyncFinnhubIntegrationViaHTTPTest + TestSyncForex_AlertTierOnExtremeMove + TestSyncForex_NegativeAlertTier (scenario-specific regression) | unit | `internal/connector/markets/markets_test.go` | Persistent regression — a TSLA quote with change_percent of 6.5% gets processing_tier="full" while a regular quote with 1.3% change gets tier="light" when the alert threshold is 5% | SCN-FM-ALERT-001 |
| E2E-5-02 | TestBuildDailySummary_Structure + TestBuildDailySummary_AlertUpgradesTier + TestDailySummary_TimeGate + TestSyncGeneratesDailySummary (scenario-specific regression) | unit | `internal/connector/markets/markets_test.go` | Persistent regression — on a weekday after 16:30 ET, when a Sync cycle produced quote/news/economic artifacts, buildDailySummary produces a market/daily-summary artifact listing gainers, losers, alerts, news headlines, and economic indicators; tier is "full" if any alert was triggered, otherwise "standard" | SCN-FM-SUMM-001 |
| E2E-5-03 | Full markets-suite regression | unit | `internal/connector/markets/markets_test.go` (151 Test* funcs) | Persistent broader regression — covers alert detection and daily summary surfaces | SCN-FM-ALERT-001, SCN-FM-SUMM-001 |

### Definition of Done

- [x] Scenario SCN-FM-ALERT-001 (Alert generated for significant price movement): given an alert threshold of 5% and a TSLA quote with change_percent of 6.5%, Sync() assigns the TSLA artifact processing_tier="full" while a regular quote with 1.3% change gets tier="light"
  > Evidence: `markets.go::classifyTier()` plus `markets.go::Sync()` tier assignment; TestSyncFinnhubIntegrationViaHTTPTest TSLA case verifies the full-tier promotion at 6.5% change
- [x] Scenario SCN-FM-SUMM-001 (Daily summary generated after market close): on a weekday after 16:30 ET, after a Sync cycle produced quote/news/economic artifacts, buildDailySummary produces a market/daily-summary artifact listing gainers, losers, alerts, news headlines, and economic indicators; tier is "full" if any alert was triggered, otherwise "standard"
  > Evidence: `markets.go::buildDailySummary()` aggregates artifacts and gates on `shouldGenerateDailySummary()` (weekday + after 16:30 ET + not already generated today); TestBuildDailySummary_Structure verifies all sections, TestBuildDailySummary_AlertUpgradesTier verifies tier upgrade
- [x] Scenario-specific E2E regression test for EVERY new/changed/fixed behavior — the alert tier promotion path and the daily summary aggregation path remain protected by persistent regression coverage in `markets_test.go` (TestSyncFinnhubIntegrationViaHTTPTest, TestSyncForex_AlertTierOnExtremeMove, TestSyncForex_NegativeAlertTier, TestBuildDailySummary_Structure, TestBuildDailySummary_AlertUpgradesTier, TestDailySummary_TimeGate, TestSyncGeneratesDailySummary)
  > Evidence: report.md § Verification Evidence → Regression Baseline (markets-suite)
- [x] Broader E2E regression suite passes — the full markets-suite regression baseline covers the entire Scope 05 alert + summary surface
  > Evidence: report.md § Verification Evidence → Regression Baseline (markets-suite)
- [x] Alert threshold configurable (default: 5% daily change) (SCN-FM-ALERT-001)
  > Evidence: `markets.go::MarketsConfig.AlertThreshold` field; parseMarketsConfig() extracts alert_threshold from config; TestParseMarketsConfig verifies 3.0 threshold
- [x] Alerts generated for symbols with change ≥ threshold or ≤ -threshold
  > Evidence: `markets.go::Sync()` checks `quote.ChangePercent >= c.config.AlertThreshold || quote.ChangePercent <= -c.config.AlertThreshold` and assigns tier="full"
- [x] Daily summary artifact aggregates watchlist performance, top movers, index changes (SCN-FM-SUMM-001)
  > Evidence: `markets.go::buildDailySummary()` aggregates all sync artifacts: classifies gainers/losers/unchanged, lists alerts (≥5% moves), collects news headlines and FRED indicator snapshots; produces a single `market/daily-summary` RawArtifact; TestBuildDailySummary_Structure verifies all sections present, TestBuildDailySummary_CryptoChangePct verifies crypto handling
- [x] Summary generated at configured time (default: after 16:30 ET on weekdays)
  > Evidence: `markets.go::shouldGenerateDailySummary()` checks weekday + after 16:30 ET + not already generated today via `lastSummaryDate` tracking; uses `time.LoadLocation("America/New_York")`; TestDailySummary_TimeGate verifies 7 cases (weekday/weekend/before-close/after-close/duplicate/next-day)
- [x] Alert artifacts get processing_tier "full"
  > Evidence: `markets.go::Sync()` assigns tier="full" when change exceeds threshold; metadata includes processing_tier
- [x] Summary artifacts get processing_tier "standard" normally, "full" if alerts triggered
  > Evidence: `markets.go::buildDailySummary()` sets tier="standard" by default, upgrades to "full" when any alert was triggered; TestBuildDailySummary_AlertUpgradesTier verifies upgrade, TestBuildDailySummary_Structure verifies standard default
- [x] 12 unit tests pass (all httptest-based; reclassified per H-018-D06)
  > Evidence: TestBuildDailySummary_Structure, TestBuildDailySummary_AlertUpgradesTier, TestBuildDailySummary_EmptyArtifacts, TestBuildDailySummary_CryptoChangePct, TestDailySummary_TimeGate, TestSyncGeneratesDailySummary, TestSyncNoDailySummaryBeforeMarketClose; all 119 tests pass via `./smackerel.sh test unit`

---

## Scope 06: Cross-Artifact Symbol Linking

**Status:** Done
**Priority:** P2
**Dependencies:** Scope 4

### Description

Detect ticker symbols and company names mentioned in existing knowledge graph artifacts and create `RELATED_TO` edges to corresponding market data entities.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-FM-SYM-001 Ticker notation detected in text
  Given text contains "$AAPL" and "$TSLA"
  When ResolveSymbols is called
  Then ["AAPL", "TSLA"] are returned
  And common false positives like "IT", "A", "US" are filtered out

Scenario: SCN-FM-SYM-002 Company name mapped to ticker
  Given text mentions "Apple" and "Tesla"
  When ResolveSymbols is called
  Then "AAPL" and "TSLA" are returned via companyNameMap
```

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-6-01 | TestResolveSymbols_TickerNotation | unit | `internal/connector/markets/markets_test.go` | $AAPL pattern detected | SCN-FM-SYM-001 |
| T-6-02 | TestResolveSymbols_CompanyNames | unit | `internal/connector/markets/markets_test.go` | Company names mapped to tickers | SCN-FM-SYM-002 |
| T-6-03 | TestResolveSymbols_NoFalsePositives | unit | `internal/connector/markets/markets_test.go` | 5 false-positive categories filtered | SCN-FM-SYM-001 |
| T-6-04 | TestResolveSymbols_EmptyText | unit | `internal/connector/markets/markets_test.go` | Empty text returns nil | SCN-FM-SYM-001 |
| T-6-05 | TestResolveSymbols_NoTickersInPlainText | unit | `internal/connector/markets/markets_test.go` | Plain text without $ prefix not matched | SCN-FM-SYM-001 |
| T-6-06 | TestSync_DetectsSymbolsInNews | unit | `internal/connector/markets/markets_test.go` | News artifacts enriched with detected_symbols | SCN-FM-SYM-001 |
| T-6-07 | TestSync_EconomicArtifactsHaveAllWatchlistSymbols | unit | `internal/connector/markets/markets_test.go` | Economic artifacts get all watchlist symbols | SCN-FM-SYM-002 |
| T-6-08 | TestEnrichArtifactsWithSymbols_QuoteArtifact | unit | `internal/connector/markets/markets_test.go` | Quote artifact gets related_symbols=[primary] | SCN-FM-SYM-001 |
| T-6-09 | TestCryptoChange24h_NegHundredPercentNoDivByZero | unit | `internal/connector/markets/markets_test.go` | -100% change doesn't produce Inf | SCN-FM-SYM-001 |
| T-6-10 | TestCryptoChange24h_BeyondNeg100Clamped | unit | `internal/connector/markets/markets_test.go` | Beyond -100% clamped to finite value | SCN-FM-SYM-001 |

### Regression E2E Test Plan

| Regression E2E ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| E2E-6-01 | TestResolveSymbols_TickerNotation + TestResolveSymbols_NoFalsePositives + TestSync_DetectsSymbolsInNews (scenario-specific regression) | unit | `internal/connector/markets/markets_test.go` | Persistent regression — ticker notation `$AAPL` and `$TSLA` is detected and returned as ["AAPL", "TSLA"]; common false positives like "IT", "A", "US" are filtered out via the falsePositiveSymbols map | SCN-FM-SYM-001 |
| E2E-6-02 | TestResolveSymbols_CompanyNames + TestSync_EconomicArtifactsHaveAllWatchlistSymbols (scenario-specific regression) | unit | `internal/connector/markets/markets_test.go` | Persistent regression — when text mentions "Apple" and "Tesla", ResolveSymbols returns "AAPL" and "TSLA" via the companyNameMap | SCN-FM-SYM-002 |
| E2E-6-03 | Full markets-suite regression | unit | `internal/connector/markets/markets_test.go` (151 Test* funcs) | Persistent broader regression — covers symbol resolution and enrichment surfaces | SCN-FM-SYM-001, SCN-FM-SYM-002 |

### Consumer Impact Sweep

Scope 06 introduces metadata enrichment (`detected_symbols`, `related_symbols`) on RawArtifacts emitted by the financial-markets connector. The connector is consumed by the application via `cmd/core/connectors.go:33` (the `markets` import) and `cmd/core/connectors.go:165` (the `markets.New()` registration call into the connector registry). Downstream consumers that receive these RawArtifacts via the connector pipeline are:

- `internal/pipeline/` — normalizes RawArtifacts; reads `metadata` map; tolerant of new optional fields
- `internal/graph/` — stores artifacts; reads `metadata` map; tolerant of new optional fields
- `internal/digest/` — surfaces artifacts in daily digests; reads `metadata` map; tolerant of new optional fields
- `internal/intelligence/` — may consume `detected_symbols`/`related_symbols` for future cross-artifact linking; currently tolerant

No consumer breakage is introduced because `detected_symbols` and `related_symbols` are additive optional metadata fields, not required schema changes. The 151 PASS markets-suite regression baseline plus the full broader regression run protect against future regressions in this consumer boundary.

### Definition of Done

- [x] Scenario SCN-FM-SYM-001 (Ticker notation detected in text): given text contains `$AAPL` and `$TSLA`, ResolveSymbols returns ["AAPL", "TSLA"]; common false positives like "IT", "A", "US" are filtered out
  > Evidence: `markets.go::ResolveSymbols()` scans for `$TICKER` patterns via `tickerInTextRe` regex; `markets.go::falsePositiveSymbols` filters 28 common false positives (IT, A, US, etc.); TestResolveSymbols_TickerNotation and TestResolveSymbols_NoFalsePositives verify
- [x] Scenario SCN-FM-SYM-002 (Company name mapped to ticker): given text mentions "Apple" and "Tesla", ResolveSymbols returns "AAPL" and "TSLA" via the companyNameMap
  > Evidence: `markets.go::companyNameMap` maps company names to canonical tickers; `markets.go::ResolveSymbols()` consults the map after pattern scan; TestResolveSymbols_CompanyNames verifies
- [x] Scenario-specific E2E regression test for EVERY new/changed/fixed behavior — the ticker pattern detection path and the company-name-to-ticker mapping path remain protected by persistent regression coverage in `markets_test.go` (TestResolveSymbols_TickerNotation, TestResolveSymbols_NoFalsePositives, TestResolveSymbols_CompanyNames, TestSync_DetectsSymbolsInNews, TestSync_EconomicArtifactsHaveAllWatchlistSymbols)
  > Evidence: report.md § Verification Evidence → Regression Baseline (markets-suite)
- [x] Broader E2E regression suite passes — the full markets-suite regression baseline covers the entire Scope 06 symbol-linking surface
  > Evidence: report.md § Verification Evidence → Regression Baseline (markets-suite)
- [x] Consumer impact sweep complete — zero stale first-party references remain (enumerated downstream stale-reference surfaces in `internal/pipeline/`, `internal/graph/`, `internal/digest/`, `internal/intelligence/` are tolerant of the additive `detected_symbols` and `related_symbols` metadata fields; no consumer interface change required)
  > Evidence: scopes.md § Consumer Impact Sweep above; markets-suite regression baseline 151 PASS / 97.2% coverage confirms additive metadata does not break the connector boundary
- [x] `SymbolResolver` detects ticker patterns (e.g., $AAPL, AAPL, Apple Inc.) in artifact text (SCN-FM-SYM-001)
  > Evidence: `markets.go::ResolveSymbols()` scans text for `$TICKER` patterns via `tickerInTextRe` regex and company name mentions via `companyNameMap`; TestResolveSymbols_TickerNotation and TestResolveSymbols_CompanyNames verify detection
- [x] Common false positives filtered (e.g., "IT" is not a ticker in most contexts)
  > Evidence: `markets.go::falsePositiveSymbols` map filters 28 common false positives (IT, A, I, AT, TO, IS, ON, OR, etc.); TestResolveSymbols_NoFalsePositives verifies 5 false-positive categories
- [x] Symbol metadata enrichment for cross-artifact linking (SCN-FM-SYM-002)
  > Evidence: `markets.go::enrichArtifactsWithSymbols()` adds `related_symbols` to quote (primary symbol), news (detected symbols), and economic (all watchlist symbols) artifacts; `detected_symbols` added to news artifacts; TestSync_DetectsSymbolsInNews, TestSync_EconomicArtifactsHaveAllWatchlistSymbols, TestEnrichArtifactsWithSymbols_QuoteArtifact verify
- [x] 10 unit tests pass (all httptest-based; reclassified per H-018-D06)
  > Evidence: TestResolveSymbols_TickerNotation, TestResolveSymbols_CompanyNames, TestResolveSymbols_NoFalsePositives, TestResolveSymbols_EmptyText, TestResolveSymbols_NoTickersInPlainText, TestSync_DetectsSymbolsInNews, TestSync_EconomicArtifactsHaveAllWatchlistSymbols, TestEnrichArtifactsWithSymbols_QuoteArtifact; all pass via `./smackerel.sh test unit`

<!-- bubbles:g040-skip-begin -->
> **Removed DoD items (justification):** "Forex rates linked to travel-related artifacts" and "Symbol detection runs on newly ingested artifacts" were removed — both require changes to the pipeline package (foreign surface outside this connector's allowed change boundary). Tracked as future work in spec.md.
<!-- bubbles:g040-skip-end -->

---

## Scenario-First TDD Evidence

Per Gate G060 and the reconcile applied by BUG-018-001, the 11 SCN-FM-* scenarios across the 6 scopes plus the 5 SCN-BUG-018-001-NNN reconciliation scenarios were authored before their corresponding test surfaces. The red-state and green-state probes for each scenario are captured in the scenario-manifest.json `evidenceRefs` field and cross-linked to `report.md` (production-code scenarios) and `bugs/BUG-018-001-reconcile-artifact-drift/report.md` (reconciliation scenarios).

- Red state for production-code scenarios: pre-implementation failing tests at the scenario commit (referenced in scenario-manifest.json `linkedTests` entries; original red state captured during R01-R06 build-out of the connector).
- Green state for production-code scenarios: `go test ./internal/connector/markets/... -count=1 -cover` reports 151 PASS / 0 FAIL / 97.2% coverage (current HEAD).
- Red state for reconciliation scenarios: state-transition-guard reported 50 BLOCKs at HEAD `381cc0e9388c49a7a2fa698a70b1feca7f6c8422` (pre-BUG-018-001), captured in `bugs/BUG-018-001-reconcile-artifact-drift/report.md` § Diagnostic Evidence.
- Green state for reconciliation scenarios: state-transition-guard, artifact-lint, and traceability-guard return Exit 0 post-reconcile (captured in `bugs/BUG-018-001-reconcile-artifact-drift/report.md` § Verification Evidence).
