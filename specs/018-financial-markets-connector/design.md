# Design: 018 — Financial Markets Connector

> **Author:** bubbles.design
> **Date:** April 9, 2026
> **Status:** Draft
> **Spec:** [spec.md](spec.md)

---

## Design Brief

### Current State

Smackerel has a working connector framework and operational connectors for content sources (RSS, YouTube, email, notes) and contextual enrichment (Maps, Weather, Gov Alerts). Users capture financial articles, videos, and bookmarks, but the knowledge graph has no market data to provide quantitative context. No financial markets connector exists.

### Target State

Add a financial markets connector that fetches stock/ETF quotes, forex rates, crypto prices, economic indicators, and market news for a user-configured watchlist. Market data artifacts are linked to the user's existing captured content about companies and markets. The connector uses free-tier APIs (Finnhub for stocks/forex, CoinGecko for crypto, FRED for economic data) with careful rate limit management.

### Patterns to Follow

- **Weather connector pattern** (016): Periodic polling, multi-provider, caching, location/context-aware enrichment
- **RSS connector pattern** — iterative source processing with cursor filtering
- **Backoff** — for API retry and rate limit recovery

### Patterns to Avoid

- **Building a trading platform** — strictly knowledge enrichment, no portfolio tracking
- **Exceeding free-tier limits** — budget API calls carefully per provider
- **Financial advice** — no recommendations, predictions, or buy/sell signals

### Resolved Decisions

- **Connector ID:** `"financial-markets"`
- **Providers:** Finnhub (stocks/forex/news, free: 60 calls/min), CoinGecko (crypto, free: 30 calls/min), FRED (economic data, free: 120 calls/min)
- **Content types:** `market/quote`, `market/summary`, `market/news`, `market/economic`, `market/earnings`, `market/alert`
- **Polling default:** Every 4 hours during market hours, daily off-hours
- **Watchlist-driven:** User configures specific symbols to track
- **Cross-artifact linking:** Detect ticker symbols in existing artifacts and create `RELATED_TO` edges
- **No new NATS subjects** — artifacts flow through standard `artifacts.process`

### Open Questions

- None blocking design completion

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Core Runtime                          │
│                                                                 │
│  ┌──────────────────────────────────────┐                       │
│  │  internal/connector/markets/         │                       │
│  │                                      │                       │
│  │  ┌────────────────────────────────┐  │                       │
│  │  │ markets.go (single file)       │  │                       │
│  │  │                                │  │                       │
│  │  │ • Connector struct (iface)     │  │                       │
│  │  │ • fetchFinnhubQuote()          │  │                       │
│  │  │ • fetchFinnhubForex()          │  │                       │
│  │  │ •   doFinnhubQuote() (shared)  │  │                       │
│  │  │ • fetchFinnhubCompanyNews()    │  │                       │
│  │  │ • fetchCoinGeckoPrices()       │  │                       │
│  │  │ • fetchFREDLatest()            │  │                       │
│  │  │ • httpErrorWithSnippet()       │  │                       │
│  │  │ • tryRecordCall() (rate limit) │  │                       │
│  │  │ • classifyTier() (normalizer)  │  │                       │
│  │  │ • buildDailySummary()          │  │                       │
│  │  │ • ResolveSymbols()             │  │                       │
│  │  │ • enrichArtifactsWithSymbols() │  │                       │
│  │  │ • parseStringSlice() (config)  │  │                       │
│  │  └────────────────────────────────┘  │                       │
│  │  ┌────────────────────────────────┐  │                       │
│  │  │ markets_test.go (119+ tests)   │  │                       │
│  │  └────────────────────────────────┘  │                       │
│  └──────────────┬───────────────────────┘                       │
│                 │                                               │
│        ┌────────▼────────┐                                      │
│        │  NATS JetStream │                                      │
│        │ artifacts.process│                                     │
│        └─────────────────┘                                      │
└─────────────────────────────────────────────────────────────────┘
```

> **Implementation note:** The original design proposed separate files per provider and concern. The implementation consolidated everything into a single `markets.go` file since the total size (~1200 lines) is manageable and avoids inter-file coupling within the package.

### Content Types Produced

| Content Type | Source | Tier Logic |
|-------------|--------|------------|
| `market/quote` | Finnhub (stocks/ETFs/forex), CoinGecko (crypto) | `light` normally; `full` when change ≥ alert threshold (replaces the originally planned `market/alert` type) |
| `market/news` | Finnhub company news | `standard` |
| `market/economic` | FRED economic indicators | `standard` |
| `market/daily-summary` | Aggregated from sync artifacts | `standard` normally; `full` if any alert triggered |

> **Design decision:** The originally planned `market/alert` content type was not implemented as a separate type. Instead, significant price movements are signaled by setting `processing_tier: "full"` on the `market/quote` artifact. This avoids duplicating artifacts (a quote would need both a `market/quote` and a `market/alert`). The `market/earnings` content type is deferred (see spec.md Future Work).

### Data Flow — Periodic Sync

1. Scheduled sync triggers (configurable via `sync_schedule` in smackerel.yaml)
2. For each watchlist stock/ETF: fetch quote via Finnhub, normalize to `market/quote`
3. For configured forex pairs: fetch via Finnhub OANDA format, normalize to `market/quote`
4. For configured crypto: batch fetch via CoinGecko, normalize to `market/quote`
5. For each FRED series: fetch latest observation, normalize to `market/economic`
6. For each watchlist stock: fetch company news via Finnhub, normalize to `market/news`
7. `enrichArtifactsWithSymbols()` adds `related_symbols`/`detected_symbols` metadata
8. `tryClaimDailySummary()` generates `market/daily-summary` after 16:30 ET on weekdays
9. Artifacts returned to scheduler for publishing to `artifacts.process` on NATS JetStream

---

## Component Design

### `internal/connector/markets/markets.go` — Single-File Implementation

All connector logic is in a single file organized by concern:

**Core types:**
- `Connector` — implements `connector.Connector` (ID, Connect, Sync, Health, Close)
- `MarketsConfig` — parsed config with watchlist, API keys, thresholds, FRED settings
- `WatchlistConfig` — stocks, ETFs, crypto coin IDs, forex pairs
- `StockQuote` — Finnhub quote response (price, change, high/low/open/close)
- `CryptoPrice` — CoinGecko price (ID, price, 24h change, percentage)
- `NewsArticle` — Finnhub company news (headline, summary, source, URL)
- `FREDObservation` — FRED economic data point (series ID, date, value)

**Provider fetch methods (on Connector):**
- `fetchFinnhubQuote(ctx, symbol)` — stock/ETF quote, delegates to `doFinnhubQuote` after validation
- `fetchFinnhubForex(ctx, pair)` — forex via OANDA format conversion, delegates to `doFinnhubQuote`
- `doFinnhubQuote(ctx, finnhubSymbol, displaySymbol, label)` — shared Finnhub quote HTTP lifecycle
- `fetchFinnhubCompanyNews(ctx, symbol, from, to)` — news via `/api/v1/company-news`
- `fetchCoinGeckoPrices(ctx, coinIDs)` — batch crypto via `/api/v3/simple/price`
- `fetchFREDLatest(ctx, seriesID)` — economic data via `/fred/series/observations`

**Rate limiting:**
- `tryRecordCall(provider)` — atomic check-and-record using sliding window (calls/minute)
- Per-provider budgets: Finnhub=55, CoinGecko=25, FRED=100
- Budgets reset on `Connect()` and `Close()`

**Normalization (inline in Sync):**
- `classifyTier(threshold, changePct)` — returns `"full"` when |change| ≥ threshold or NaN/Inf, else `"light"`
- Artifact creation is inline in `Sync()` — each provider section builds `connector.RawArtifact` directly

**Daily summary:**
- `tryClaimDailySummary(now)` — atomic check-and-set under lock for duplicate prevention
- `buildDailySummary(artifacts, now)` — aggregates gainers/losers/alerts/news/economic into `market/daily-summary`
- Time gate: weekday + after 16:30 ET + not already generated today

**Symbol resolution:**
- `ResolveSymbols(text)` — scans for `$TICKER` patterns and company name matches, filters false positives
- `enrichArtifactsWithSymbols(artifacts, cfg)` — adds `related_symbols`/`detected_symbols` metadata

**Input validation:**
- `validSymbolRe` — stock/ETF symbols (1-10 alphanumeric/dot/hyphen)
- `validCoinIDRe` — CoinGecko IDs (lowercase alphanumeric/hyphen, 1-64)
- `validForexPairRe` — forex pairs (3-letter/3-letter)
- `validFREDSeriesRe` — FRED series (uppercase alphanumeric, 1-20)
- Size limits: `maxWatchlistSymbols=100` per category (stocks, ETFs, crypto, forex, FRED series)
- `maxCoinGeckoBatchSize=50`, `maxNewsArticlesPerSymbol=10`

**Health state machine:**
- `HealthDisconnected` → `Connect()` → `HealthHealthy`
- `HealthHealthy` → `Sync()` → `HealthSyncing` → deferred restore
- Degraded when `failCount > 0 || partialSkip` (rate-limit or partial provider failure)
- Error when all providers fail
- `configGen` counter prevents stale Sync from clobbering a concurrent Connect's health

---

## Configuration Schema

Actual configuration in `config/smackerel.yaml`:

```yaml
# config/smackerel.yaml — connectors section
connectors:
  financial-markets:
    enabled: false
    sync_schedule: "*/15 * * * *"
    finnhub_api_key: ""    # REQUIRED when enabled: free API key from finnhub.io
    fred_api_key: ""       # Optional: free key from fred.stlouisfed.org
    fred_enabled: true     # Enable FRED economic indicators (requires fred_api_key)
    fred_series: ["GDP", "UNRATE", "CPIAUCSL", "DFF", "FEDFUNDS"]
    coingecko_enabled: true
    alert_threshold: 5.0   # Percentage change to trigger alert
    watchlist:
      stocks: []           # e.g., ["AAPL", "GOOGL", "TSLA", "NVDA"]
      etfs: []             # e.g., ["SPY", "QQQ", "VTI"]
      crypto: []           # e.g., ["bitcoin", "ethereum"]
      forex_pairs: []      # e.g., ["USD/JPY", "EUR/USD"]
```

---

## Database, NATS & Dependencies

- **No new database tables** — market data uses existing artifact/sync_state tables
- **No new NATS subjects** — market artifacts flow through standard `artifacts.process`
- **No Python sidecar changes** — market text is processed by standard ML pipeline
- **No new Go dependencies** — all APIs are plain REST/JSON, parsed with standard library
