# Feature: 018 — Financial Markets Connector

> **Author:** bubbles.analyst
> **Date:** April 9, 2026
> **Status:** Draft
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md) — Section 16.8 Financial Awareness (Light Touch)

---

## Problem Statement

People consume financial information across dozens of surfaces — news articles about companies they follow, YouTube videos about market trends, emails about earnings reports, saved bookmarks on investment strategies. Smackerel already captures all of this content, but it lacks the **quantitative context** that makes this content actionable.

Without a financial markets connector, the knowledge graph has a critical blind spot:

1. **Articles lack price context.** A user saves an article titled "Why Tesla is Undervalued" but the knowledge graph has no idea what TSLA was trading at when the article was saved, or what it trades at now. The article floats in isolation without the numerical reality it references.
2. **Temporal financial queries are impossible.** "What was the market doing when I was researching EV companies last March?" is unanswerable because price data doesn't exist in the graph.
3. **Travel planning misses forex.** The user captures articles about trips to Japan, has calendar events for flights, but doesn't see that USD/JPY is at a favorable rate — information that's trivially available but disconnected from their planning context.
4. **Earnings and economic events are invisible.** The user follows 10 companies across various saved content, but major earnings reports and Fed rate decisions pass without any connection to their captured knowledge.
5. **Company mentions in artifacts are unresolved.** When the knowledge graph detects "Apple" in an article, podcast transcript, or email, there's no structured entity to link it to — no ticker, no sector classification, no price history.

This connector provides **market data as knowledge context**, not as a trading tool. It follows the design doc's "Financial Awareness (Light Touch)" principle: enrich what the user already captures with quantitative financial reality.

---

## Outcome Contract

**Intent:** Enrich the Smackerel knowledge graph with structured financial market data — stock quotes, forex rates, crypto prices, economic indicators, and market news — so that financial context is automatically linked to the user's captured articles, videos, bookmarks, and notes about companies, markets, and economic topics.

**Success Signal:** A user configures a watchlist of 10 stock symbols and 3 crypto assets. Within 48 hours: (1) each symbol has daily price snapshots stored as artifacts, (2) a search for "how did Apple do last week" returns both saved articles mentioning Apple AND the AAPL price chart for that period, (3) an article about "Federal Reserve rate hike" is auto-linked to the corresponding FRED economic indicator artifact, and (4) the daily digest mentions "NVDA moved +7% today — you have 3 saved articles about NVIDIA from the past month."

**Hard Constraints:**
- Read-only market data consumption — never place orders, manage portfolios, or connect to brokerage accounts
- All data from free-tier API providers — no paid subscriptions required for basic operation
- Must implement the standard `Connector` interface (ID, Connect, Sync, Health, Close)
- Rate limit compliance is non-negotiable — connectors must respect free-tier API limits and never exceed them
- No financial advice, trading signals, buy/sell recommendations, or portfolio optimization in any artifact or digest content
- All data stored locally — no cloud persistence beyond API calls to fetch public market data
- Timestamp-based sync tracking — Sync returns a cursor timestamp for scheduler tracking; point-in-time market data queries are inherently current-only and do not require cursor-based deduplication; news queries use today's date as the range filter

**Failure Condition:** If a user has 10 symbols on their watchlist and after 48 hours of operation: price data is stale by more than 24 hours, no cross-references exist between saved financial articles and market data artifacts, or the connector exhausts free API limits within hours and goes silent for the rest of the day — the connector has failed regardless of technical health status.

---

## Goals

1. **Watchlist-driven data ingestion** — Sync price snapshots, market news, and event data for a user-configured watchlist of stock symbols, ETFs, forex pairs, and cryptocurrencies
2. **Multi-provider API strategy** — Use multiple free-tier data providers (Finnhub for stocks/forex, CoinGecko for crypto, FRED for economic data) with intelligent rate limit management and failover
3. **Daily market summary artifacts** — Generate daily summary artifacts that highlight watchlist movers, market index changes, and relevant economic events
4. **Event-driven alerts** — Detect significant price movements (configurable threshold, default: ±5% daily change) and earnings releases for watchlist companies, producing `full`-tier artifacts
5. **Cross-artifact linking** — Automatically link company ticker symbols mentioned in existing knowledge graph artifacts (articles, videos, bookmarks, notes) to corresponding market data entities
6. **Economic indicator tracking** — Sync key economic indicators (CPI, unemployment, GDP, Fed funds rate) from FRED, creating artifacts that contextualize the user's financial content
7. **Forex travel integration** — Surface relevant forex rates when the knowledge graph contains travel-related artifacts for specific countries
8. **Historical data for temporal queries** — Maintain sufficient price history to answer temporal queries like "what was AAPL doing when I saved that article about the iPhone launch?"
9. **Processing pipeline integration** — Route all market data through the standard NATS JetStream pipeline with appropriate tier assignment based on data significance

---

## Non-Goals

- **Trading or order execution** — This connector never connects to a brokerage, places orders, or facilitates any transaction (design doc non-goal 1.5: "Financial advice or automated transactions")
- **Portfolio tracking or management** — No portfolio value calculation, asset allocation analysis, P&L tracking, or account balance monitoring
- **Financial advice or recommendations** — No buy/sell signals, price targets, analyst ratings, or investment recommendations in any artifact or digest
- **Tax calculation** — No capital gains computation, tax-loss harvesting analysis, or tax reporting
- **Real-time streaming data** — Free-tier APIs provide delayed or snapshot data; sub-second price feeds are out of scope
- **Technical analysis** — No chart patterns, moving averages, RSI calculations, or other technical trading indicators
- **Options, futures, or derivatives data** — Only spot prices for equities, ETFs, forex, and crypto
- **Paid API tier features** — The connector operates on free-tier API limits; premium data feeds are a future enhancement, not a v1 requirement
- **Social sentiment analysis** — No Reddit/Twitter/StockTwits sentiment scoring; the connector provides data, not opinions
- **Backtesting or simulation** — No historical strategy testing or "what-if" portfolio analysis

---

## Future Work (Not Scoped in v1)

The following goals from the spec are acknowledged but intentionally deferred from v1 scopes:

| Item | Spec Goal Ref | Reason Deferred |
|------|--------------|----------------|
| **Earnings calendar integration** | Goal 4 | Finnhub earnings endpoint is available but v1 prioritizes price/quote data; add as separate scope when demand is validated |
| **Market indices tracking** | Goal 3 | Index data (S&P 500, Dow, NASDAQ) available via Finnhub but not watchlist-driven; add when daily summary needs index context |
| **Historical data for temporal queries** | Goal 8 | Requires historical price storage and query API; significant scope beyond point-in-time quotes; design as a separate feature |
| **Market-hours-only sync option** | Scope 4 (optional) | Configurable sync suppression during off-hours; removed from Scope 4 DoD as unimplemented optional feature |
| **Forex-travel artifact linking** | Scope 6 | Requires pipeline package changes (foreign surface); cross-connector artifact scanning is an architecture change |
| **Pipeline symbol detection hook** | Scope 6 | Requires changes to pipeline package (foreign surface); deferred until pipeline extensibility is designed |

---

## API Strategy — Provider Comparison & Selection

### The Landscape

Financial market data APIs have wildly varying free tiers. The connector must maximize data coverage while staying within free-tier limits across all providers.

### Provider Comparison

| Provider | Free Tier Limits | Data Coverage | Go SDK | Reliability | Best For |
|----------|-----------------|---------------|--------|-------------|----------|
| **Finnhub** | 60 calls/min | US/intl stocks, forex, crypto, earnings, news | `github.com/Finnhub-Stock-API/finnhub-go` | High — official API, well-documented | Primary stocks, forex, earnings, news |
| **Alpha Vantage** | 25 calls/day | Stocks, forex, crypto, economic indicators | `github.com/RobotsAndPencils/go-alphavantage` | Medium — strict rate limits | Backup stocks, limited by daily cap |
| **Polygon.io** | 5 calls/min, delayed | US stocks, options, forex, crypto | REST client (no official Go SDK) | High — professional-grade | Backup for US equities |
| **IEX Cloud** | 50,000 msgs/month | US stocks, news, earnings | REST client | Medium — credit-based model | Backup US data |
| **CoinGecko** | 30 calls/min | Comprehensive crypto (10,000+ coins) | REST client | High — de facto crypto standard | Primary crypto data |
| **FRED** | Unlimited (with key) | US economic indicators (700k+ series) | REST client | Very High — Federal Reserve operated | Primary economic data |
| **Yahoo Finance** | Unofficial, no limit | Everything | Python only (`yfinance`) | Low — can break without notice | Not recommended for Go connector |

### Recommended Provider Strategy

| Data Type | Primary Provider | Fallback Provider | Rationale |
|-----------|-----------------|-------------------|-----------|
| **US Stock Quotes** | Finnhub | Alpha Vantage | Finnhub has generous per-minute limits; Alpha Vantage as daily backup |
| **International Stocks** | Finnhub | — | Best free international coverage |
| **Forex Rates** | Finnhub | — | Included in Finnhub free tier |
| **Crypto Prices** | CoinGecko | Finnhub | CoinGecko is the crypto data standard; Finnhub covers basics |
| **Market News** | Finnhub | — | News endpoint included in free tier |
| **Earnings Calendar** | Finnhub | — | Earnings calendar endpoint included |
| **Economic Indicators** | FRED | — | Only viable free source for economic data |
| **Market Indices** | Finnhub | — | Major indices available in free tier |

### Rate Limit Budget (per sync cycle)

With a default 4-hour sync interval (6 cycles/day during market hours):

| Provider | Limit | Budget Per Cycle | Allocation |
|----------|-------|-----------------|------------|
| **Finnhub** | 60/min (=3,600/hr) | ~14,400 calls per 4-hr window | 50 watchlist quotes + 10 news + 5 earnings + 5 indices + 5 forex = ~75 calls (well within budget) |
| **CoinGecko** | 30/min (=1,800/hr) | ~7,200 calls per 4-hr window | 20 crypto prices + 5 market data = ~25 calls |
| **FRED** | Unlimited | Unlimited | 10-20 indicator checks per cycle |
| **Alpha Vantage** | 25/day | ~4 calls per cycle | Emergency fallback only |

### Implementation Architecture

```
┌──────────────────────────────────────────────────┐
│            Go Financial Markets Connector          │
│            (Connector interface)                   │
│                                                    │
│  ┌────────────┐ ┌────────────┐ ┌──────────────┐  │
│  │  Finnhub   │ │ CoinGecko  │ │    FRED      │  │
│  │  Client    │ │  Client    │ │   Client     │  │
│  │ (stocks,   │ │ (crypto)   │ │ (economic    │  │
│  │  forex,    │ │            │ │  indicators) │  │
│  │  news,     │ │            │ │              │  │
│  │  earnings) │ │            │ │              │  │
│  └─────┬──────┘ └─────┬──────┘ └──────┬───────┘  │
│        │              │               │           │
│  ┌─────▼──────────────▼───────────────▼───────┐  │
│  │          Rate Limiter / Budget Manager       │  │
│  │   (per-provider token bucket + daily cap)    │  │
│  └──────────────────┬──────────────────────────┘  │
│                     │                              │
│  ┌──────────────────▼──────────────────────────┐  │
│  │          Artifact Normalizer                  │  │
│  │   (→ RawArtifact per content type)            │  │
│  └──────────────────┬──────────────────────────┘  │
│                     │                              │
│  ┌──────────────────▼──────────────────────────┐  │
│  │          NATS JetStream Publish               │  │
│  │   (pipeline processing)                       │  │
│  └───────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────┘
```

---

## Requirements

### R-001: Connector Interface Compliance

The financial markets connector MUST implement the standard `Connector` interface:

```go
type Connector interface {
    ID() string
    Connect(ctx context.Context, config ConnectorConfig) error
    Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error)
    Health(ctx context.Context) HealthStatus
    Close() error
}
```

- `ID()` returns `"financial-markets"`
- `Connect()` validates configuration, verifies API keys for each enabled provider, tests connectivity to each provider endpoint, and sets health to `healthy`
- `Sync()` fetches market data for the configured watchlist since the cursor timestamp, returns `[]RawArtifact` and a new cursor
- `Health()` reports per-provider health status and aggregate connector health
- `Close()` releases HTTP clients, stops rate limiters, and sets health to `disconnected`

### R-002: Watchlist Management

The connector MUST support user-configured watchlists:

- **Stocks/ETFs:** List of ticker symbols (e.g., `["AAPL", "GOOGL", "NVDA", "SPY", "QQQ"]`)
- **Crypto:** List of CoinGecko IDs (e.g., `["bitcoin", "ethereum", "solana"]`)
- **Forex:** List of currency pairs (e.g., `["USD/EUR", "USD/JPY", "USD/GBP"]`)
- **Indices:** List of index symbols (e.g., `["^GSPC", "^IXIC", "^DJI"]`)
- **Economic indicators:** List of FRED series IDs (e.g., `["CPIAUCSL", "UNRATE", "GDP", "FEDFUNDS"]`)

Watchlist changes take effect on the next sync cycle without requiring connector restart.

Maximum watchlist sizes for free-tier compliance:
- Stocks/ETFs: 50 symbols
- Crypto: 20 assets
- Forex: 10 pairs
- Indices: 10 indices
- FRED indicators: 20 series

### R-003: Price Quote Sync (Stocks, ETFs, Crypto, Forex)

For each symbol on the watchlist:

- Fetch current/latest price data from the appropriate provider
- Produce a `RawArtifact` with `content_type: "market/quote"`
- Include: symbol, price, previous close, change (absolute and percent), volume, market cap (where available), day high/low, 52-week high/low (where available)
- Populate `RawArtifact.Metadata` with structured price fields for graph linking
- `RawArtifact.CapturedAt` is the timestamp of the price data point, not the fetch time
- `RawArtifact.SourceID` is `"financial-markets"`
- `RawArtifact.SourceRef` format: `"{provider}:{symbol}:{date}"` (e.g., `"finnhub:AAPL:2026-04-09"`)

### R-004: Market Summary Artifact (Daily)

Generate one daily market summary artifact per sync day:

- `content_type: "market/summary"`
- Content includes: watchlist top movers (biggest gainers/losers), market index performance, notable economic events scheduled, earnings releases for watchlist companies
- Summary is generated from aggregated data across all providers
- Summary is human-readable prose suitable for digest inclusion
- Published once per day (first sync cycle of the day generates it; subsequent cycles update if significant changes occur)
- Processing tier: `standard`

### R-005: Market News Ingestion

Fetch market news related to watchlist symbols:

- Use Finnhub's company news endpoint for watchlist stock symbols
- Produce `RawArtifact` with `content_type: "market/news"` for each newsworthy headline
- Dedup news by headline hash + source to avoid processing the same story from multiple outlets
- Include: headline, summary, source name, publication date, related symbols, news URL
- Cap: maximum 20 news artifacts per sync cycle to avoid flooding the pipeline
- News artifacts older than 7 days are not fetched (stale news is not knowledge)

### R-006: Economic Indicator Sync (FRED)

For each configured FRED series:

- Fetch the latest observation(s) from the FRED API
- Produce `RawArtifact` with `content_type: "market/economic"`
- Include: series ID, series title (human-readable, e.g., "Consumer Price Index for All Urban Consumers"), observation date, value, units, frequency (monthly/quarterly/annual), previous value, change from previous
- FRED data updates infrequently (monthly/quarterly) — the connector must track last-known observation date per series and only produce new artifacts when new data is released
- Processing tier: `standard` (economic releases are broadly relevant context)

### R-007: Earnings Report Detection

For watchlist stock symbols:

- Check the Finnhub earnings calendar endpoint for upcoming and recent earnings
- Produce `RawArtifact` with `content_type: "market/earnings"` when a watchlist company reports earnings
- Include: symbol, company name, reporting date, EPS estimate, EPS actual (when available), revenue estimate, revenue actual (when available), surprise percentage
- Earnings artifacts are produced on the reporting date, not in advance (future dates are tracked internally but not published as artifacts until the report date)
- Processing tier: `full` (earnings directly relate to companies the user follows)

### R-008: Significant Movement Alerts

Detect and flag significant price movements for watchlist symbols:

- **Threshold:** Configurable percentage (default: ±5% daily change)
- When a symbol exceeds the threshold, produce `RawArtifact` with `content_type: "market/alert"`
- Include: symbol, current price, previous close, change percent, movement direction (up/down), time of detection
- Alert artifacts are produced at most once per symbol per trading day (no re-alerting on the same day's move)
- Processing tier: `full` (significant movements deserve full entity extraction and graph linking)

### R-009: Metadata Preservation

Each market data artifact MUST carry the following metadata in `RawArtifact.Metadata`:

| Field | Source | Type | Purpose |
|-------|--------|------|---------|
| `symbol` | Ticker/asset ID | `string` | Dedup key, graph entity link |
| `asset_type` | Classification | `string` | One of: `stock`, `etf`, `crypto`, `forex`, `index`, `economic` |
| `provider` | Data source | `string` | One of: `finnhub`, `coingecko`, `fred` |
| `price` | Latest price | `float64` | Current value |
| `change_pct` | Daily change | `float64` | Percentage change from previous close |
| `market_cap` | Market capitalization | `float64` | Where available (stocks, crypto) |
| `volume` | Trading volume | `float64` | Where available |
| `currency` | Price currency | `string` | e.g., `"USD"`, `"EUR"` |
| `exchange` | Exchange name | `string` | e.g., `"NASDAQ"`, `"NYSE"` |
| `sector` | Industry sector | `string` | For stocks: technology, healthcare, etc. |
| `data_timestamp` | Price data time | `string` (ISO 8601) | When the price was recorded |
| `is_delayed` | Data delay flag | `bool` | True if data is delayed (free-tier limitation) |
| `related_symbols` | Linked tickers | `[]string` | For news/earnings: which watchlist symbols are mentioned |

### R-010: Dedup Strategy

Market data deduplication follows these rules:

- **Price quotes:** Dedup key is `{symbol}:{date}` — one price snapshot per symbol per calendar date. If a newer quote arrives for the same day, update the existing artifact in place.
- **News:** Dedup key is `sha256(headline + source)` — same headline from same source is skipped.
- **Earnings:** Dedup key is `{symbol}:{reporting_date}` — one earnings artifact per company per reporting period.
- **Economic indicators:** Dedup key is `{series_id}:{observation_date}` — one artifact per series per observation.
- **Alerts:** Dedup key is `{symbol}:{date}` — maximum one alert per symbol per trading day.
- **Daily summary:** Dedup key is `{date}` — one summary per calendar date (may be updated).

### R-011: Cursor-Based Incremental Sync

- **Cursor format:** ISO 8601 timestamp of the most recent successful sync completion
- Initial sync (empty cursor): fetch latest price snapshot for all watchlist symbols + last 7 days of news + latest FRED observations + upcoming earnings calendar
- Incremental sync: fetch data that has changed or been published since cursor timestamp
- Cursor is persisted via the existing `StateStore` (PostgreSQL `sync_state` table)
- If cursor is corrupted or missing, fall back to initial sync behavior (safe because dedup prevents reprocessing)

### R-012: Processing Tier Assignment

Apply processing tiers based on content significance:

| Content Type | Trigger | Processing Tier | Rationale |
|-------------|---------|----------------|-----------|
| `market/alert` | Significant price movement (>threshold) | `full` | Directly relevant event |
| `market/earnings` | Watchlist company earnings release | `full` | High-signal company event |
| `market/news` | News headline for watchlist symbol | `standard` | Context-enriching content |
| `market/summary` | Daily market summary | `standard` | Digest-ready overview |
| `market/economic` | New economic indicator release | `standard` | Broad macroeconomic context |
| `market/quote` | Routine price update | `light` | Data point, not narrative |
| Historical backfill (initial sync) | First-time price history | `metadata` | Bulk data, minimal processing |

Processing tiers map to the existing pipeline tier definitions:
- `full`: Summarize, extract entities, generate embedding, cross-link in knowledge graph
- `standard`: Summarize, extract entities, generate embedding
- `light`: Extract title/metadata, generate embedding (no LLM summarization)
- `metadata`: Title + source only (no embedding generation for bulk quotes)

### R-013: Rate Limit Management

Rate limiting is critical — free-tier API abuse results in key revocation:

- **Per-provider token bucket:** Each provider has an independent rate limiter implementing a token-bucket algorithm matching its free-tier limits
- **Finnhub:** 60 requests/minute token bucket, refill rate 1/second
- **CoinGecko:** 30 requests/minute token bucket, refill rate 0.5/second
- **FRED:** No per-minute limit, but implement a courtesy limit of 120 requests/minute
- **Alpha Vantage (fallback):** 25 requests/day hard cap, tracked daily
- **Backoff on 429:** When a provider returns HTTP 429 (rate limit exceeded), use the existing exponential backoff infrastructure (`internal/connector/backoff.go`) with provider-specific defaults (initial: 60s, max: 15min, multiplier: 2.0)
- **Budget tracking:** Each sync cycle logs the number of API calls made per provider and the remaining budget
- **Graceful degradation:** If a provider's rate limit is exhausted mid-cycle, skip remaining requests for that provider and continue with other providers. Report the partial sync in health status.

### R-014: Cross-Artifact Linking (Knowledge Graph Integration)

The connector MUST enable cross-referencing between market data and other knowledge graph artifacts:

- **Symbol detection in existing artifacts:** When the processing pipeline encounters ticker symbols (e.g., "$AAPL", "AAPL", "Apple Inc.") in articles, bookmarks, videos, or notes, it creates `RELATED_TO` edges to the corresponding market data entity
- **Company name resolution:** Map common company names to ticker symbols using Finnhub's symbol lookup endpoint (cached locally, refreshed weekly)
- **Forex-to-travel linking:** When travel-related artifacts mention a destination country, link to the relevant forex pair (e.g., Japan trip artifacts → USD/JPY rate data)
- **Earnings-to-content linking:** When an earnings artifact is created for a watchlist company, scan for existing artifacts mentioning that company and create temporal `RELATED_TO` edges
- **Temporal correlation:** Store the `data_timestamp` on each market artifact so the graph can answer "what was the price when this article was captured?"

### R-015: Error Handling and Resilience

- **API key invalid/expired:** Report via `Health()` as `HealthError` with the specific provider identified. Do not retry authentication automatically.
- **Provider endpoint down:** Mark the specific provider as temporarily unavailable. Continue syncing from other providers. Retry with exponential backoff on subsequent cycles.
- **Rate limit exceeded:** Apply backoff, skip provider for remainder of cycle, log the event, report in health.
- **Network failure:** Retry with backoff, report via health status, do not lose cursor position.
- **Malformed API response:** Log the raw response, skip the specific data point, continue processing remaining items. Report count of failures in sync summary.
- **Partial sync failure:** Persist cursor at the last successfully processed batch. Report which providers succeeded and which failed.
- **Watchlist symbol not found:** Log a warning for unrecognized symbols, continue syncing valid symbols. Report invalid symbols in health so the user can correct their watchlist.

### R-016: Configuration

The connector is configured via `config/smackerel.yaml`:

```yaml
connectors:
  financial-markets:
    enabled: false
    sync_schedule: "0 */4 * * *"   # Every 4 hours

    # API Keys (REQUIRED when enabled)
    finnhub_api_key: ""             # Get free key at finnhub.io/register
    fred_api_key: ""                # Get free key at fred.stlouisfed.org/docs/api/api_key.html
    # CoinGecko does not require an API key for free tier

    # Watchlist configuration
    watchlist:
      stocks: []                    # e.g., ["AAPL", "GOOGL", "MSFT", "NVDA", "TSLA"]
      etfs: []                      # e.g., ["SPY", "QQQ", "VTI"]
      crypto: []                    # e.g., ["bitcoin", "ethereum", "solana"]
      forex: []                     # e.g., ["USD/EUR", "USD/JPY", "USD/GBP"]
      indices: []                   # e.g., ["^GSPC", "^IXIC", "^DJI"]

    # FRED economic indicator series
    economic_indicators: []         # e.g., ["CPIAUCSL", "UNRATE", "GDP", "FEDFUNDS"]

    # Alert thresholds
    alerts:
      price_change_pct: 5.0         # Alert when daily change exceeds ±5%
      enabled: true

    # Market hours awareness
    market_hours:
      timezone: "America/New_York"
      active_only: false            # If true, only sync during market hours (9:30-16:00 ET)

    # Processing defaults
    processing_tier: "light"        # Default tier for routine quotes
    news_max_per_cycle: 20          # Cap news artifacts per sync
    news_max_age_days: 7            # Skip news older than this

    # Rate limit overrides (advanced)
    rate_limits:
      finnhub_rpm: 60               # Requests per minute
      coingecko_rpm: 30
      fred_rpm: 120
```

### R-017: Health Reporting

The connector MUST report granular per-provider health status:

| Status | Condition |
|--------|-----------|
| `healthy` | All enabled providers responding, last sync completed successfully |
| `syncing` | Sync operation currently in progress |
| `error` | One or more providers failing — include specific provider and error in state |
| `disconnected` | Connector not initialized or explicitly closed |

Health checks MUST include:
- Last successful sync timestamp (per provider and aggregate)
- Number of artifacts produced in last cycle (by content type)
- Number of errors in last cycle (by provider)
- Rate limit budget remaining (per provider)
- Watchlist symbol count and any invalid symbols detected
- Data freshness: age of the most recent price quote

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| **Casual Investor** | Individual who follows a few stocks and crypto casually, reads financial news but doesn't actively trade | See market context alongside saved articles about companies; get notified when a followed stock moves significantly; understand macro context (inflation, rates) | Read-only market data; configures personal watchlist |
| **Knowledge Worker** | Professional who encounters financial topics in their work (consulting, journalism, research) | Have company/market context when reviewing saved articles and reports; link financial data to research topics in knowledge graph | Read-only market data; broader watchlist including indices and economic indicators |
| **Travel Planner** | User who captures travel planning content and benefits from forex context | See relevant currency exchange rates linked to travel destination artifacts; understand forex trends for upcoming trips | Read-only forex data; forex pairs auto-suggested from travel artifacts |
| **Self-Hoster** | Privacy-conscious user managing their own Smackerel instance | Full control over API keys, watchlist, sync frequency, and data retention; understand rate limits and provider reliability | Docker admin, config management, API key management |

---

## Use Cases

### UC-001: Configure Watchlist and Connect

- **Actor:** Casual Investor
- **Preconditions:** Smackerel running, financial markets connector configuration available in `smackerel.yaml`
- **Main Flow:**
  1. User adds stock symbols, crypto assets, and forex pairs to the watchlist in `smackerel.yaml`
  2. User provides Finnhub and FRED API keys in configuration
  3. Connector initializes, validates API keys against each provider
  4. Connector verifies each watchlist symbol is valid by testing a lookup
  5. Health status reports `healthy` with per-provider confirmation
  6. First sync cycle fetches initial data for all watchlist items
- **Alternative Flows:**
  - Invalid API key → health reports `error` with specific provider identified; connector does not start syncing
  - Unrecognized symbol → warning logged, symbol flagged in health, valid symbols sync normally
  - CoinGecko needs no API key → crypto syncing works even if no keys are configured
- **Postconditions:** Watchlist configured, all valid symbols syncing, health is `healthy`

### UC-002: Routine Price Sync

- **Actor:** System (automated)
- **Preconditions:** Connector enabled, watchlist configured, previous sync completed
- **Main Flow:**
  1. Scheduled sync fires at configured interval (default: every 4 hours)
  2. Connector fetches current quotes for all stock/ETF watchlist symbols from Finnhub
  3. Connector fetches current prices for all crypto watchlist assets from CoinGecko
  4. Connector fetches latest forex rates from Finnhub
  5. Each price point is normalized to a `RawArtifact` with `content_type: "market/quote"`
  6. Dedup check: if a quote for the same symbol and date already exists, update in place
  7. Artifacts are published to NATS JetStream for pipeline processing
  8. Cursor advances to sync completion timestamp
- **Alternative Flows:**
  - Finnhub rate limit hit mid-cycle → skip remaining Finnhub calls, complete CoinGecko and FRED, report partial sync
  - Provider endpoint timeout → retry once, then skip provider for this cycle, report in health
- **Postconditions:** Latest price data stored for all available symbols, cursor advanced

### UC-003: Significant Price Movement Alert

- **Actor:** System (automated, triggers user-visible digest item)
- **Preconditions:** Connector has previous-day close data for watchlist symbols
- **Main Flow:**
  1. During routine sync, connector calculates daily change percentage for each symbol
  2. NVDA shows +8.5% change, exceeding the configured 5% threshold
  3. Connector produces `RawArtifact` with `content_type: "market/alert"`
  4. Alert artifact includes: symbol, prices, change %, and is assigned `full` processing tier
  5. Processing pipeline extracts entities, generates embedding, links to NVIDIA-related artifacts
  6. Daily digest includes: "NVDA moved +8.5% today — you have 3 saved articles about NVIDIA"
- **Alternative Flows:**
  - No symbols exceed threshold → no alert artifacts produced (normal operation)
  - Multiple symbols exceed threshold → one alert artifact per qualifying symbol
  - Alert already produced for this symbol today → skip (one alert per symbol per day)
- **Postconditions:** Alert artifact stored, cross-linked to company artifacts, digest enriched

### UC-004: Cross-Reference Article with Market Data

- **Actor:** System (automated)
- **Preconditions:** User has saved an article titled "Why TSLA Could Double by 2027" and TSLA is on the watchlist
- **Main Flow:**
  1. Processing pipeline extracts entity "$TSLA" / "Tesla" from the article artifact
  2. Entity resolver maps "Tesla" → ticker symbol "TSLA"
  3. Knowledge graph creates `RELATED_TO` edge between the article artifact and the TSLA market data entity
  4. When the user later searches "Tesla" or "TSLA", results include both the article AND current market data
  5. The article's detail view shows the TSLA price at the time the article was captured vs. current price
- **Alternative Flows:**
  - Symbol not on watchlist → no market data entity exists, no cross-reference created (entity is stored for future linking if symbol is later added)
  - Ambiguous company name (e.g., "Apple" could be the company or the fruit) → resolved via context and sector classification
- **Postconditions:** Financial articles linked to market data, enabling contextual retrieval

### UC-005: Economic Indicator in Context

- **Actor:** Knowledge Worker
- **Preconditions:** FRED series configured (CPI, unemployment, Fed funds rate), user has saved articles about inflation
- **Main Flow:**
  1. FRED connector fetches the latest CPI release showing 3.2% year-over-year inflation
  2. Artifact produced with `content_type: "market/economic"`, `full` processing
  3. Entity extraction identifies "inflation", "consumer price index", "3.2%"
  4. Knowledge graph links this economic artifact to 5 existing articles the user saved about inflation
  5. User searches "what is inflation doing?" → gets the FRED data artifact AND their saved articles, showing the narrative alongside the numbers
- **Alternative Flows:**
  - FRED data not yet updated (monthly lag) → no new artifact produced, previous observation remains current
- **Postconditions:** Economic data enriches the knowledge graph's understanding of macroeconomic topics

### UC-006: Forex Context for Travel Planning

- **Actor:** Travel Planner
- **Preconditions:** User has saved travel articles about Japan, USD/JPY is on forex watchlist
- **Main Flow:**
  1. Knowledge graph contains travel artifacts mentioning "Japan", "Tokyo", "Kyoto"
  2. Financial markets connector syncs USD/JPY forex rate
  3. Cross-artifact linking detects the Japan → JPY relationship
  4. Daily digest or proactive surfacing: "Planning Japan trip? USD/JPY is at 148.5 — the yen is near a 2-year low, making it 12% cheaper than last year"
  5. User searches "Japan trip budget" → results include both travel articles AND forex rate data
- **Alternative Flows:**
  - Destination country not on forex watchlist → no forex linking (manual watchlist management required in v1)
- **Postconditions:** Travel planning enriched with real-time forex context

### UC-007: Earnings Report Notification

- **Actor:** Casual Investor
- **Preconditions:** Watchlist includes AAPL, earnings calendar shows AAPL reporting this week
- **Main Flow:**
  1. Connector checks Finnhub earnings calendar during sync
  2. Detects AAPL reported earnings: EPS $2.10 (estimate: $1.95), revenue $94.8B (estimate: $92.5B)
  3. Produces `RawArtifact` with `content_type: "market/earnings"`, `full` processing tier
  4. Entity extraction links to all existing Apple-related artifacts in the knowledge graph
  5. Digest mentions: "Apple beat earnings estimates — EPS $2.10 vs $1.95 expected. You have 4 saved articles about Apple"
- **Alternative Flows:**
  - Earnings not yet reported (future date) → tracked internally, no artifact produced until report date
  - Earnings data incomplete (estimate only, no actual yet) → wait for actual data before producing artifact
- **Postconditions:** Earnings data stored, linked to company artifacts, digest enriched

---

## Business Scenarios (Gherkin)

### Connector Setup & Configuration

```gherkin
Scenario: BS-001 Connector initializes with valid API keys
  Given the financial markets connector is enabled in configuration
  And valid Finnhub and FRED API keys are provided
  And the watchlist contains 5 stock symbols and 3 crypto assets
  When the connector initializes
  Then all provider connections are verified
  And all watchlist symbols are validated
  And the connector health reports "healthy" with per-provider status
  And the first sync cycle is scheduled

Scenario: BS-002 Missing API key fails loudly
  Given the financial markets connector is enabled
  But the Finnhub API key is empty
  When the connector attempts to initialize
  Then the connector reports "error" health with message "finnhub_api_key is required"
  And no sync cycles are started
  And other provider connections are not attempted

Scenario: BS-003 Invalid watchlist symbol is reported
  Given the connector is configured with watchlist symbol "XYZNOTREAL"
  When the connector validates the watchlist
  Then "XYZNOTREAL" is flagged as unrecognized in health status
  And all other valid symbols sync normally
  And the user can correct the watchlist without connector restart
```

### Price Data Sync

```gherkin
Scenario: BS-004 Routine price sync produces quote artifacts
  Given the connector has a watchlist of 10 stock symbols
  And the last sync cursor is "2026-04-08T16:00:00Z"
  When the scheduled sync cycle runs
  Then 10 quote artifacts are produced with content_type "market/quote"
  And each artifact contains symbol, price, change_pct, volume
  And the cursor advances to the sync completion timestamp
  And all artifacts are published to NATS JetStream

Scenario: BS-005 Duplicate quotes for the same day update in place
  Given AAPL was synced at 10:00 with price $185.50
  And the next sync at 14:00 shows AAPL at $187.20
  When the connector produces the new quote
  Then the existing AAPL artifact for today is updated (not duplicated)
  And the artifact's price reflects $187.20
  And the knowledge graph edges from the original artifact are preserved

Scenario: BS-006 Crypto prices sync from CoinGecko
  Given the watchlist includes crypto assets "bitcoin" and "ethereum"
  When the sync cycle runs
  Then CoinGecko is queried for BTC and ETH prices
  And quote artifacts are produced with asset_type "crypto"
  And provider metadata shows "coingecko"
  And prices include market_cap and 24h volume
```

### Rate Limit Handling

```gherkin
Scenario: BS-007 Finnhub rate limit triggers graceful degradation
  Given the connector is mid-sync fetching stock quotes
  And Finnhub returns HTTP 429 (rate limit exceeded) after 45 symbols
  When the rate limiter detects the 429 response
  Then exponential backoff is applied (initial: 60s)
  And the remaining 5 stock symbols are skipped for this cycle
  Then CoinGecko and FRED syncs continue unaffected
  And health reports "healthy" with warning "finnhub: 5 symbols deferred (rate limit)"
  And the cursor still advances for successfully synced data

Scenario: BS-008 Daily API budget is not exceeded
  Given Alpha Vantage has a 25 calls/day limit
  And 20 calls have been made today
  When the sync cycle would require 10 more Alpha Vantage calls
  Then only 5 calls are made (staying within daily budget)
  And the remaining symbols use the primary provider (Finnhub)
  And the daily call count is persisted across sync cycles
```

### Significant Movement Detection

```gherkin
Scenario: BS-009 Significant price movement triggers alert
  Given NVDA's previous close was $800
  And the current price is $860 (+7.5%, exceeding the 5% threshold)
  When the connector calculates daily change percentage
  Then a "market/alert" artifact is produced for NVDA
  And the alert artifact is assigned "full" processing tier
  And the alert contains: symbol "NVDA", change_pct 7.5, direction "up"
  And no second alert is produced for NVDA on the same trading day

Scenario: BS-010 No alert when movement is within threshold
  Given AAPL's previous close was $185
  And the current price is $187 (+1.08%, within the 5% threshold)
  When the connector calculates daily change percentage
  Then no "market/alert" artifact is produced for AAPL
  And a normal "market/quote" artifact is produced with "light" processing tier
```

### Cross-Artifact Linking

```gherkin
Scenario: BS-011 Article mentioning a ticker links to market data
  Given the user has a saved article with text mentioning "$TSLA" and "Tesla"
  And TSLA is on the watchlist with current market data
  When the processing pipeline extracts entities from the article
  Then "TSLA" is resolved to the financial markets entity
  And a RELATED_TO edge is created between the article and the TSLA market entity
  And searching "Tesla" returns both the article and current TSLA price data

Scenario: BS-012 Earnings report links to company's existing artifacts
  Given AAPL reports earnings
  And the user has 4 saved articles mentioning "Apple" or "AAPL"
  When the earnings artifact is processed through the pipeline
  Then RELATED_TO edges are created between the earnings artifact and all 4 Apple articles
  And the daily digest mentions "Apple beat earnings — you have 4 saved articles about Apple"

Scenario: BS-013 Forex rate links to travel destination artifacts
  Given the user has saved articles about traveling to Japan
  And USD/JPY is on the forex watchlist
  When the forex rate is synced
  Then the JPY rate artifact is linked to Japan-related travel artifacts
  And temporal queries about "yen rate when I planned my trip" are answerable
```

### Economic Data

```gherkin
Scenario: BS-014 New CPI release creates an economic artifact
  Given CPIAUCSL is in the economic_indicators list
  And the last known observation was for February 2026
  When FRED releases the March 2026 CPI data
  Then a "market/economic" artifact is produced with the new value
  And the artifact includes: series title, observation date, value, change from previous
  And the artifact is assigned "standard" processing tier
  And existing inflation-related artifacts are cross-linked

Scenario: BS-015 No duplicate economic artifacts for unchanged data
  Given the March 2026 CPI data was synced in the previous cycle
  When the next sync cycle checks FRED
  Then FRED returns the same March 2026 observation
  And no new artifact is produced (dedup by series_id + observation_date)
```

### Error Handling

```gherkin
Scenario: BS-016 Provider outage does not block other providers
  Given Finnhub is experiencing a service outage
  And CoinGecko and FRED are operating normally
  When the sync cycle runs
  Then Finnhub calls fail with timeout after retry
  And Finnhub health is marked as "error"
  Then CoinGecko crypto quotes are synced successfully
  And FRED economic data is synced successfully
  And aggregate health reports "error" with detail "finnhub: unavailable"
  And cursor advances for the successful providers

Scenario: BS-017 Malformed API response is logged and skipped
  Given Finnhub returns malformed JSON for the GOOGL quote
  When the connector attempts to parse the response
  Then the parse error is logged with the raw response
  And the GOOGL quote is skipped for this cycle
  And all other symbols are processed normally
  And health reports the count of parse failures
```

### Daily Summary

```gherkin
Scenario: BS-018 Daily market summary is generated
  Given the connector has synced quotes for 10 stocks, 3 crypto, and 2 forex pairs
  And NVDA is the biggest gainer (+4.2%), TSLA is the biggest loser (-3.1%)
  And AAPL has earnings scheduled tomorrow
  When the first sync cycle of the day completes
  Then a "market/summary" artifact is produced
  And the summary mentions top movers: "NVDA +4.2%, TSLA -3.1%"
  And the summary mentions upcoming earnings: "AAPL reports tomorrow"
  And the summary includes index performance: "S&P 500 +0.3%, NASDAQ +0.5%"
  And the summary artifact is assigned "standard" processing tier
```

---

## Competitive Landscape

### How Other Tools Handle Financial Market Data

| Tool | Financial Data Integration | Approach | Limitations |
|------|---------------------------|----------|-------------|
| **Notion** | None built-in | Manual tables or third-party embeds | No automated data sync, no semantic linking |
| **Obsidian** | Community plugins (Stock Tracker, Finance) | Inline price quotes via plugin API calls | No knowledge graph linking, manual setup per note |
| **Apple Notes** | None | N/A | No financial awareness |
| **Readwise** | None | Focused on reading highlights | No market data |
| **Mem.ai** | None | AI notes, no financial data | No external data source connectors |
| **Capacities** | None | Object-based notes, no market data | Manual only |
| **Personal Capital / Empower** | Full portfolio tracking | Brokerage account linking | Financial tool, not knowledge tool — no article/content linking |
| **Yahoo Finance** | Full market data | Standalone finance app | Silo — no connection to user's knowledge or content |
| **Google Finance** | Portfolio + quotes | Standalone web app | Silo — no semantic search or cross-content linking |
| **Bloomberg Terminal** | Comprehensive market data | Professional terminal | $24k/year, not a personal knowledge tool |

### Competitive Gap Assessment

**No existing personal knowledge tool connects financial market data to the user's captured content.** The market is split into two silos:

1. **Knowledge tools** (Notion, Obsidian, Readwise) — have content but no market data
2. **Finance tools** (Yahoo Finance, Personal Capital, Bloomberg) — have data but no content awareness

**Smackerel's differentiation:**
- **Market data as knowledge context** — prices, earnings, and economic data are artifacts in the same graph as articles, videos, and notes
- **Cross-domain financial linking** — an article about Tesla automatically links to TSLA price data
- **Temporal financial queries** — "what was AAPL doing when I saved that article?" is answerable
- **Economic context enrichment** — CPI releases link to saved inflation articles
- **Travel-forex integration** — trip planning artifacts connect to relevant currency data
- **Zero portfolio access** — purely contextual, no brokerage connection required

---

## Improvement Proposals

### IP-001: Smart Watchlist Suggestions ⭐ Competitive Edge
- **Impact:** High
- **Effort:** M
- **Competitive Advantage:** No tool auto-suggests financial symbols based on the user's reading patterns
- **Actors Affected:** Casual Investor, Knowledge Worker
- **Business Scenarios:** System detects the user saved 5 articles about NVIDIA in the past month and suggests: "You seem interested in NVIDIA. Add NVDA to your financial watchlist?" Watchlist grows organically from reading behavior.

### IP-002: Sector-Level Market Context
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** Beyond individual stock tracking, provide sector-level intelligence tied to the user's topic interests
- **Actors Affected:** Knowledge Worker
- **Business Scenarios:** User's knowledge graph has heavy coverage of AI/ML topics. System detects this maps to the "Technology — Semiconductors" sector and surfaces sector performance alongside individual stocks.

### IP-003: Historical Price Overlay on Artifact Timeline
- **Impact:** High
- **Effort:** L
- **Competitive Advantage:** Visualize what the market was doing when the user was researching a topic
- **Actors Affected:** Casual Investor, Knowledge Worker
- **Business Scenarios:** User views their "Electric Vehicles" topic. The timeline shows not just their saved articles chronologically, but overlays TSLA/RIVN/NIO prices at each article's capture date. Narrative + numbers in one view.

### IP-004: Alert Sensitivity Auto-Tuning
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** Static percentage thresholds produce too many alerts for volatile assets and too few for stable ones
- **Actors Affected:** Casual Investor
- **Business Scenarios:** BTC (naturally volatile) gets a wider threshold (10%) while JNJ (stable) keeps 5%. Thresholds auto-adjust based on 30-day historical volatility per symbol.

### IP-005: Earnings Surprise Correlation with Content
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** No tool connects earnings surprises to the user's research timeline
- **Actors Affected:** Casual Investor, Knowledge Worker
- **Business Scenarios:** "Apple beat earnings by 8% on AI services revenue. You saved 3 articles about Apple's AI strategy in the 2 months before earnings — your research anticipated the theme."

### IP-006: Multi-Currency Portfolio Context (Without Portfolio Tracking)
- **Impact:** Low
- **Effort:** S
- **Competitive Advantage:** Forex context for users who hold assets in multiple currencies (expats, digital nomads)
- **Actors Affected:** Travel Planner
- **Business Scenarios:** User captures content in multiple languages/currencies. System surfaces relevant cross-rate trends without ever knowing the user's actual holdings.

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| Configure financial markets connector | Self-Hoster | Settings → Connectors | Select Financial Markets → add API keys → configure watchlist → save | Connector enabled, providers verified, health check passes | Settings, Connector Config |
| View connector sync status | Casual Investor | Dashboard → Connectors | View Financial Markets connector card | Last sync time, artifacts by type, provider health, rate limit budget | Dashboard |
| Search for company with market context | Casual Investor | Search bar | Search "Apple" or "AAPL" | Results include saved articles AND current market data, cross-linked | Search Results |
| View price data alongside article | Knowledge Worker | Artifact detail | View an article about a company | Article view shows linked market data: price at capture time vs. now | Artifact Detail |
| Browse daily market summary | Casual Investor | Digest or search | View daily digest or search "market summary" | Summary of watchlist movers, indices, upcoming earnings | Digest, Search Results |
| View economic indicator with linked articles | Knowledge Worker | Search "inflation" or "CPI" | Enter search | Results include FRED CPI data artifact AND inflation-related articles from knowledge graph | Search Results |
| Review significant movement alert | Casual Investor | Digest notification | Read daily digest | Digest highlights: "NVDA +8.5% — you have 3 saved NVIDIA articles" | Digest |

---

## Non-Functional Requirements

- **Performance:** Sync cycle for a 50-symbol watchlist completes within 2 minutes (excluding pipeline processing). Individual API calls timeout after 10 seconds with one retry.
- **Scalability:** Connector handles watchlists up to the configured maximums (50 stocks, 20 crypto, 10 forex, 10 indices, 20 FRED series) without degradation. Larger watchlists require paid API tiers (future enhancement).
- **Reliability:** Connector survives restart without data loss — sync cursor and rate limit state persisted in PostgreSQL. Supervisor auto-recovers crashed connector goroutines. Provider outages do not cascade to other providers.
- **Data Freshness:** Quotes are at most one sync interval old (default: 4 hours). Free-tier data may have 15-minute delay from exchanges. Economic indicators reflect the latest FRED observation.
- **Accessibility:** All market data artifacts are accessible via the same search and browse interfaces as other artifact types. No financial-markets-specific UI required beyond the connector configuration screen.
- **Security:** API keys are stored in the configured secrets backend, never in plaintext config files. API keys are passed via environment variables in generated env files. No brokerage credentials, no account access, no sensitive financial data.
- **Privacy:** Only public market data is fetched. No user financial accounts are accessed. The watchlist itself is the only user-specific financial information stored. All data is stored locally.
- **Compliance:** The connector produces market data context only. No content generated by the connector or its downstream pipeline constitutes financial advice. Content warnings are appended to digest items derived from market data: "Market data is for informational context only."
- **Observability:** Sync metrics (artifacts_produced, api_calls_per_provider, rate_limit_remaining, errors, duration) are emitted as structured log entries. Health endpoint includes per-provider granular status.
