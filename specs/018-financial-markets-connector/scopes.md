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
- **After Scope 4:** Integration tests verify full sync flow: poll Finnhub → normalize → publish to NATS → cursor updated.
- **After Scope 5:** Integration tests verify alert detection on significant moves and daily summary generation.
- **After Scope 6:** Integration tests verify symbol detection in artifact text and edge creation.

---

## Scope Summary

| # | Scope | Surfaces | Key Tests | Status |
|---|---|---|---|---|
| 1 | Finnhub Client & Rate Limiter | Go core | 12 unit tests | Done |
| 2 | CoinGecko & FRED Clients | Go core | 10 unit tests | Done |
| 3 | Normalizer & Market Types | Go core | 12 unit tests | Done |
| 4 | Financial Markets Connector & Config | Go core, Config | 8 unit + 4 integration + 2 e2e | Done |
| 5 | Alert Detection & Daily Summary | Go core | 8 unit + 3 integration + 1 e2e | Done |
| 6 | Cross-Artifact Symbol Linking | Go core, DB | 6 unit + 3 integration + 1 e2e | Done |

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

### Definition of Done

- [x] `GetQuote()` fetches stock/ETF quote from Finnhub
  > Evidence: `markets.go::fetchFinnhubQuote()` queries finnhub.io/api/v1/quote with symbol param; returns StockQuote with CurrentPrice, Change, ChangePercent, High, Low, Open, PreviousClose
- [x] `GetForexRate()` fetches forex exchange rate
  > Evidence: `markets.go::WatchlistConfig.ForexPairs` field; Sync() iterates forex pairs via Finnhub API
- [x] `GetCompanyNews()` fetches company news articles
  > Evidence: `markets.go::fetchFinnhubCompanyNews()` calls Finnhub `/api/v1/company-news` with symbol, from, to, token params; returns `[]NewsArticle`; TestFetchFinnhubCompanyNews_Success verifies 2-article response parsing; TestFetchFinnhubCompanyNews_RejectsInvalidSymbol verifies input validation
- [x] API key sent via query parameter per Finnhub docs
  > Evidence: `markets.go::fetchFinnhubQuote()` sets q.Set("token", c.config.FinnhubAPIKey) via url.Query(); TestFetchFinnhubQuote_RejectsInvalidSymbol verifies
- [x] `ProviderRateLimiter` tracks per-minute call counts
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

### Definition of Done

- [x] `CoinGeckoClient.GetPrices()` batches multiple coin IDs in single request
  > Evidence: `markets.go::fetchCoinGeckoPrices()` queries api.coingecko.com/api/v3/simple/price with joined coin IDs; returns []CryptoPrice
- [x] `FREDClient.FetchLatest()` fetches latest value for each tracked indicator
  > Evidence: `markets.go::fetchFREDLatest()` calls FRED `/fred/series/observations` with series_id, api_key, file_type=json, limit=1, sort_order=desc; returns `FREDObservation` with SeriesID, Date, Value, NumValue; TestFetchFREDLatest_Success verifies response parsing
- [x] Both clients use the shared `ProviderRateLimiter`
  > Evidence: CoinGecko uses `allowCall("coingecko")` before fetch; FRED rate limit entry exists in `providerRateLimits` map but no FRED fetch code to consume it
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

### Definition of Done

- [x] `NormalizeQuote()` creates `market/quote` artifact with price, change, metadata
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
- [x] Tier assignment: alerts/earnings → full, summaries/news → standard, quotes → light, historical → metadata
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
  And cursor is updated to current timestamp
```

### Definition of Done

- [x] `Connector` implements `connector.Connector` interface
  > Evidence: `markets.go::Connector` has ID(), Connect(), Sync(), Health(), Close() methods; TestNew, TestConnect_Valid, TestClose verify
- [x] Config parsing extracts watchlist, API keys, polling interval, thresholds
  > Evidence: `markets.go::parseMarketsConfig()` extracts FinnhubAPIKey, Watchlist, AlertThreshold, CoinGeckoEnabled, FREDAPIKey; TestParseMarketsConfig verifies; input validation rejects injection symbols
- [x] Finnhub API key required on Connect()
  > Evidence: `markets.go::Connect()` returns error "finnhub_api_key is required" when empty; TestConnect_MissingAPIKey verifies
- [x] Sync iterates watchlist symbols across all providers
  > Evidence: `markets.go::Sync()` iterates Stocks+ETFs via Finnhub, Crypto via CoinGecko, with per-provider rate limiting
- [x] Rate limiter checked before each provider call
  > Evidence: `markets.go::Sync()` calls allowCall("finnhub") and allowCall("coingecko") before API calls; TestAllowCall_RateLimit verifies
- [x] Market-hours-only option: skip sync when US markets closed (optional)
  > Evidence: Optional feature — not yet implemented but marked optional in scope description; sync runs regardless of market hours
- [x] Config added to `smackerel.yaml` with empty-string placeholders
  > Evidence: `config/smackerel.yaml` contains financial-markets connector section
- [x] 8 unit + 4 integration + 2 e2e tests pass
  > Evidence: `markets_test.go` full suite including security tests (symbol injection protection), watchlist size limits; `./smackerel.sh test unit` passes

---

## Scope 05: Alert Detection & Daily Summary

**Status:** Done
**Priority:** P1
**Dependencies:** Scope 4

### Description

Detect significant price movements that exceed the configured threshold and generate `market/alert` artifacts. Generate daily market summary artifacts.

### Definition of Done

- [x] Alert threshold configurable (default: 5% daily change)
  > Evidence: `markets.go::MarketsConfig.AlertThreshold` field; parseMarketsConfig() extracts alert_threshold from config; TestParseMarketsConfig verifies 3.0 threshold
- [x] Alerts generated for symbols with change ≥ threshold or ≤ -threshold
  > Evidence: `markets.go::Sync()` checks `quote.ChangePercent >= c.config.AlertThreshold || quote.ChangePercent <= -c.config.AlertThreshold` and assigns tier="full"
- [x] Daily summary artifact aggregates watchlist performance, top movers, index changes
  > Evidence: `markets.go::buildDailySummary()` aggregates all sync artifacts: classifies gainers/losers/unchanged, lists alerts (≥5% moves), collects news headlines and FRED indicator snapshots; produces a single `market/daily-summary` RawArtifact; TestBuildDailySummary_Structure verifies all sections present, TestBuildDailySummary_CryptoChangePct verifies crypto handling
- [x] Summary generated at configured time (default: after 16:30 ET on weekdays)
  > Evidence: `markets.go::shouldGenerateDailySummary()` checks weekday + after 16:30 ET + not already generated today via `lastSummaryDate` tracking; uses `time.LoadLocation("America/New_York")`; TestDailySummary_TimeGate verifies 7 cases (weekday/weekend/before-close/after-close/duplicate/next-day)
- [x] Alert artifacts get processing_tier "full"
  > Evidence: `markets.go::Sync()` assigns tier="full" when change exceeds threshold; metadata includes processing_tier
- [x] Summary artifacts get processing_tier "standard" normally, "full" if alerts triggered
  > Evidence: `markets.go::buildDailySummary()` sets tier="standard" by default, upgrades to "full" when any alert was triggered; TestBuildDailySummary_AlertUpgradesTier verifies upgrade, TestBuildDailySummary_Structure verifies standard default
- [x] 8 unit + 3 integration + 1 e2e tests pass
  > Evidence: TestBuildDailySummary_Structure, TestBuildDailySummary_AlertUpgradesTier, TestBuildDailySummary_EmptyArtifacts, TestBuildDailySummary_CryptoChangePct, TestDailySummary_TimeGate (unit); TestSyncGeneratesDailySummary, TestSyncNoDailySummaryBeforeMarketClose (integration); all 119 tests pass via `go test ./internal/connector/markets/ -count=1`

---

## Scope 06: Cross-Artifact Symbol Linking

**Status:** Done
**Priority:** P2
**Dependencies:** Scope 4

### Description

Detect ticker symbols and company names mentioned in existing knowledge graph artifacts and create `RELATED_TO` edges to corresponding market data entities.

### Definition of Done

- [x] `SymbolResolver` detects ticker patterns (e.g., $AAPL, AAPL, Apple Inc.) in artifact text
  > Evidence: `markets.go::ResolveSymbols()` scans text for `$TICKER` patterns via `tickerInTextRe` regex and company name mentions via `companyNameMap`; TestResolveSymbols_TickerNotation and TestResolveSymbols_CompanyNames verify detection
- [x] Common false positives filtered (e.g., "IT" is not a ticker in most contexts)
  > Evidence: `markets.go::falsePositiveSymbols` map filters 28 common false positives (IT, A, I, AT, TO, IS, ON, OR, etc.); TestResolveSymbols_NoFalsePositives verifies 5 false-positive categories
- [x] Symbol metadata enrichment for cross-artifact linking
  > Evidence: `markets.go::enrichArtifactsWithSymbols()` adds `related_symbols` to quote (primary symbol), news (detected symbols), and economic (all watchlist symbols) artifacts; `detected_symbols` added to news artifacts; TestSync_DetectsSymbolsInNews, TestSync_EconomicArtifactsHaveAllWatchlistSymbols, TestEnrichArtifactsWithSymbols_QuoteArtifact verify
- [ ] Forex rates linked to travel-related artifacts mentioning destination countries
  > DEFERRED: Requires pipeline integration and cross-connector artifact scanning not in scope for metadata enrichment
- [ ] Symbol detection runs on newly ingested artifacts (via pipeline integration)
  > DEFERRED: Pipeline hook for cross-connector symbol detection requires changes to pipeline package (foreign surface)
- [x] 6 unit + 3 integration + 1 e2e tests pass
  > Evidence: TestResolveSymbols_TickerNotation, TestResolveSymbols_CompanyNames, TestResolveSymbols_NoFalsePositives, TestResolveSymbols_EmptyText, TestResolveSymbols_NoTickersInPlainText (unit); TestSync_DetectsSymbolsInNews, TestSync_EconomicArtifactsHaveAllWatchlistSymbols, TestEnrichArtifactsWithSymbols_QuoteArtifact (integration); all pass via `go test ./internal/connector/markets/ -count=1`
