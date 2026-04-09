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
│  │  ┌────────────┐  ┌───────────────┐   │                       │
│  │  │ markets.go │  │ finnhub.go    │   │                       │
│  │  │ (Connector │  │ (Stocks/Forex)│   │                       │
│  │  │  iface)    │  └───────────────┘   │                       │
│  │  └─────┬──────┘  ┌───────────────┐   │                       │
│  │        │         │ coingecko.go  │   │                       │
│  │        │         │ (Crypto)      │   │                       │
│  │        │         └───────────────┘   │                       │
│  │        │         ┌───────────────┐   │                       │
│  │        │         │ fred.go       │   │                       │
│  │        │         │ (Econ data)   │   │                       │
│  │        │         └───────────────┘   │                       │
│  │  ┌─────▼─────────────────────────┐   │                       │
│  │  │  normalizer.go               │   │                       │
│  │  │  (MarketData → RawArtifact)  │   │                       │
│  │  └─────┬─────────────────────────┘   │                       │
│  │  ┌─────▼─────────────────────────┐   │                       │
│  │  │  ratelimiter.go              │   │                       │
│  │  │  (Per-provider rate budgets) │   │                       │
│  │  └─────┬─────────────────────────┘   │                       │
│  │  ┌─────▼─────────────────────────┐   │                       │
│  │  │  symbolresolver.go           │   │                       │
│  │  │  (Cross-artifact linking)    │   │                       │
│  │  └───────────────────────────────┘   │                       │
│  └──────────────┬───────────────────────┘                       │
│                 │                                               │
│        ┌────────▼────────┐                                      │
│        │  NATS JetStream │                                      │
│        │ artifacts.process│                                     │
│        └─────────────────┘                                      │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow — Periodic Sync

1. Scheduled sync triggers (default: every 4 hours during market hours)
2. For each watchlist symbol: fetch latest quote via provider-specific client
3. For configured market indices: fetch index values
4. For configured forex pairs: fetch exchange rates
5. For configured crypto: fetch prices via CoinGecko
6. `normalizer.go` converts provider responses to `connector.RawArtifact`
7. Artifacts published to `artifacts.process` on NATS JetStream
8. Significant movers (±5% daily) generate `market/alert` artifacts
9. ML sidecar processes; Go core stores and links to knowledge graph

### Data Flow — Daily Summary

1. Daily summary trigger fires (default: 4 PM on trading days)
2. Aggregate watchlist performance, index changes, notable events
3. Generate `market/summary` artifact with day's highlights
4. Published to pipeline for digest integration

---

## Component Design

### 1. `internal/connector/markets/markets.go` — Connector Interface

```go
package markets

import (
    "context"
    "fmt"
    "log/slog"
    "sync"
    "time"

    "github.com/smackerel/smackerel/internal/connector"
)

type MarketsConfig struct {
    Watchlist           WatchlistConfig
    FinnhubAPIKey       string
    CoinGeckoEnabled    bool
    FREDAPIKey          string
    PollInterval        time.Duration
    AlertThreshold      float64 // percentage change for alert (default: 5.0)
    DailySummaryEnabled bool
    DailySummaryTime    string // e.g., "16:00"
    MarketHoursOnly     bool   // only poll during US market hours
}

type WatchlistConfig struct {
    Stocks     []string `json:"stocks"`      // e.g., ["AAPL", "GOOGL", "TSLA"]
    ETFs       []string `json:"etfs"`        // e.g., ["SPY", "QQQ", "VTI"]
    Crypto     []string `json:"crypto"`      // e.g., ["bitcoin", "ethereum"]
    ForexPairs []string `json:"forex_pairs"` // e.g., ["USD/JPY", "EUR/USD"]
    Indices    []string `json:"indices"`     // e.g., ["^GSPC", "^IXIC", "^DJI"]
}

type Connector struct {
    id          string
    health      connector.HealthStatus
    mu          sync.RWMutex
    config      MarketsConfig
    finnhub     *FinnhubClient
    coingecko   *CoinGeckoClient
    fred        *FREDClient
    normalizer  *Normalizer
    rateLimiter *ProviderRateLimiter
}

func New(id string) *Connector {
    return &Connector{id: id, health: connector.HealthDisconnected}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
    cfg, err := parseMarketsConfig(config)
    if err != nil { return fmt.Errorf("parse markets config: %w", err) }

    if cfg.FinnhubAPIKey == "" {
        return fmt.Errorf("finnhub_api_key is required for financial markets connector")
    }

    c.config = cfg
    c.finnhub = NewFinnhubClient(cfg.FinnhubAPIKey)
    c.normalizer = NewNormalizer()
    c.rateLimiter = NewProviderRateLimiter()

    if cfg.CoinGeckoEnabled {
        c.coingecko = NewCoinGeckoClient()
    }
    if cfg.FREDAPIKey != "" {
        c.fred = NewFREDClient(cfg.FREDAPIKey)
    }

    c.health = connector.HealthHealthy
    return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
    c.mu.Lock()
    c.health = connector.HealthSyncing
    c.mu.Unlock()
    defer func() {
        c.mu.Lock()
        c.health = connector.HealthHealthy
        c.mu.Unlock()
    }()

    var artifacts []connector.RawArtifact
    now := time.Now()

    // Skip if market-hours-only and markets are closed
    if c.config.MarketHoursOnly && !isMarketHours(now) {
        return nil, cursor, nil
    }

    // Stock/ETF quotes
    for _, symbol := range append(c.config.Watchlist.Stocks, c.config.Watchlist.ETFs...) {
        if !c.rateLimiter.Allow("finnhub") { break }
        quote, err := c.finnhub.GetQuote(ctx, symbol)
        if err != nil {
            slog.Warn("finnhub quote failed", "symbol", symbol, "error", err)
            continue
        }
        artifacts = append(artifacts, c.normalizer.NormalizeQuote(quote, symbol))

        // Alert on significant movement
        if quote.ChangePercent >= c.config.AlertThreshold || quote.ChangePercent <= -c.config.AlertThreshold {
            artifacts = append(artifacts, c.normalizer.NormalizeAlert(quote, symbol))
        }
    }

    // Crypto quotes
    if c.coingecko != nil {
        if c.rateLimiter.Allow("coingecko") {
            prices, err := c.coingecko.GetPrices(ctx, c.config.Watchlist.Crypto)
            if err != nil {
                slog.Warn("coingecko fetch failed", "error", err)
            } else {
                for _, p := range prices {
                    artifacts = append(artifacts, c.normalizer.NormalizeCryptoQuote(p))
                }
            }
        }
    }

    // Forex rates
    for _, pair := range c.config.Watchlist.ForexPairs {
        if !c.rateLimiter.Allow("finnhub") { break }
        rate, err := c.finnhub.GetForexRate(ctx, pair)
        if err != nil {
            slog.Warn("forex rate failed", "pair", pair, "error", err)
            continue
        }
        artifacts = append(artifacts, c.normalizer.NormalizeForex(rate, pair))
    }

    // Economic indicators (FRED) — daily poll only
    if c.fred != nil && c.rateLimiter.Allow("fred") {
        indicators, err := c.fred.FetchLatest(ctx)
        if err != nil {
            slog.Warn("FRED fetch failed", "error", err)
        } else {
            for _, ind := range indicators {
                artifacts = append(artifacts, c.normalizer.NormalizeEconomic(ind))
            }
        }
    }

    return artifacts, now.Format(time.RFC3339), nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.health
}

func (c *Connector) Close() error {
    c.health = connector.HealthDisconnected
    return nil
}
```

### 2. `internal/connector/markets/finnhub.go` — Finnhub API Client

```go
package markets

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type FinnhubClient struct {
    apiKey     string
    httpClient *http.Client
    baseURL    string
}

type StockQuote struct {
    Symbol        string
    CurrentPrice  float64 `json:"c"`
    Change        float64 `json:"d"`
    ChangePercent float64 `json:"dp"`
    High          float64 `json:"h"`
    Low           float64 `json:"l"`
    Open          float64 `json:"o"`
    PreviousClose float64 `json:"pc"`
    Timestamp     int64   `json:"t"`
}

type ForexRate struct {
    Pair string
    Rate float64
    Time time.Time
}

func NewFinnhubClient(apiKey string) *FinnhubClient {
    return &FinnhubClient{
        apiKey:     apiKey,
        httpClient: &http.Client{Timeout: 10 * time.Second},
        baseURL:    "https://finnhub.io/api/v1",
    }
}

func (c *FinnhubClient) GetQuote(ctx context.Context, symbol string) (*StockQuote, error) {
    // GET /quote?symbol=SYMBOL&token=API_KEY
    return nil, nil
}

func (c *FinnhubClient) GetForexRate(ctx context.Context, pair string) (*ForexRate, error) {
    // GET /forex/rates?base=USD&token=API_KEY
    return nil, nil
}

func (c *FinnhubClient) GetCompanyNews(ctx context.Context, symbol string, from, to time.Time) ([]NewsItem, error) {
    // GET /company-news?symbol=SYMBOL&from=DATE&to=DATE&token=API_KEY
    return nil, nil
}

type NewsItem struct {
    ID       int64  `json:"id"`
    Headline string `json:"headline"`
    Summary  string `json:"summary"`
    Source   string `json:"source"`
    URL      string `json:"url"`
    Symbol   string
    Time     int64  `json:"datetime"`
}
```

### 3. `internal/connector/markets/coingecko.go` — CoinGecko API Client

```go
package markets

import (
    "context"
    "net/http"
    "time"
)

type CoinGeckoClient struct {
    httpClient *http.Client
    baseURL    string
}

type CryptoPrice struct {
    ID            string
    Symbol        string
    Name          string
    CurrentPrice  float64
    Change24h     float64
    ChangePercent float64
    MarketCap     float64
    Volume24h     float64
    Time          time.Time
}

func NewCoinGeckoClient() *CoinGeckoClient {
    return &CoinGeckoClient{
        httpClient: &http.Client{Timeout: 10 * time.Second},
        baseURL:    "https://api.coingecko.com/api/v3",
    }
}

func (c *CoinGeckoClient) GetPrices(ctx context.Context, coinIDs []string) ([]CryptoPrice, error) {
    // GET /simple/price?ids=bitcoin,ethereum&vs_currencies=usd&include_24hr_change=true&include_market_cap=true
    return nil, nil
}
```

### 4. `internal/connector/markets/fred.go` — FRED Economic Data Client

```go
package markets

import (
    "context"
    "net/http"
    "time"
)

type FREDClient struct {
    apiKey     string
    httpClient *http.Client
    baseURL    string
}

type EconomicIndicator struct {
    SeriesID    string  // e.g., "CPIAUCSL", "UNRATE", "GDP"
    Name        string
    Value       float64
    Date        string
    Units       string
    Frequency   string
    LastUpdated time.Time
}

// Default tracked indicators
var defaultIndicators = []string{
    "CPIAUCSL",   // CPI
    "UNRATE",     // Unemployment Rate
    "GDP",        // GDP
    "FEDFUNDS",   // Federal Funds Rate
    "T10Y2Y",     // 10Y-2Y Treasury Spread
}

func NewFREDClient(apiKey string) *FREDClient {
    return &FREDClient{
        apiKey:     apiKey,
        httpClient: &http.Client{Timeout: 10 * time.Second},
        baseURL:    "https://api.stlouisfed.org/fred",
    }
}

func (c *FREDClient) FetchLatest(ctx context.Context) ([]EconomicIndicator, error) {
    // For each indicator: GET /series/observations?series_id=X&api_key=KEY&sort_order=desc&limit=1&file_type=json
    return nil, nil
}
```

### 5. `internal/connector/markets/ratelimiter.go` — Per-Provider Rate Budgets

```go
package markets

import (
    "sync"
    "time"
)

type ProviderRateLimiter struct {
    mu       sync.Mutex
    budgets  map[string]*rateBudget
}

type rateBudget struct {
    maxPerMinute int
    calls        []time.Time
}

func NewProviderRateLimiter() *ProviderRateLimiter {
    return &ProviderRateLimiter{
        budgets: map[string]*rateBudget{
            "finnhub":   {maxPerMinute: 55}, // 60/min limit, leave 5 buffer
            "coingecko": {maxPerMinute: 25}, // 30/min limit, leave 5 buffer
            "fred":      {maxPerMinute: 100}, // 120/min limit, leave 20 buffer
        },
    }
}

func (r *ProviderRateLimiter) Allow(provider string) bool {
    r.mu.Lock()
    defer r.mu.Unlock()

    b, ok := r.budgets[provider]
    if !ok { return true }

    now := time.Now()
    cutoff := now.Add(-time.Minute)

    // Remove old calls
    valid := b.calls[:0]
    for _, t := range b.calls {
        if t.After(cutoff) { valid = append(valid, t) }
    }
    b.calls = valid

    if len(b.calls) >= b.maxPerMinute { return false }
    b.calls = append(b.calls, now)
    return true
}
```

---

## Configuration Schema Addition

```yaml
# config/smackerel.yaml — connectors section
connectors:
  financial-markets:
    enabled: false
    finnhub_api_key: ""    # REQUIRED when enabled: free API key from finnhub.io
    coingecko_enabled: true
    fred_api_key: ""       # Optional: free key from fred.stlouisfed.org
    sync_schedule: "0 */4 * * *"  # Every 4 hours
    market_hours_only: false
    alert_threshold: 5.0   # Percentage change to trigger alert
    daily_summary: true
    daily_summary_time: "16:00"
    watchlist:
      stocks: []           # e.g., ["AAPL", "GOOGL", "TSLA", "NVDA"]
      etfs: []             # e.g., ["SPY", "QQQ", "VTI"]
      crypto: []           # e.g., ["bitcoin", "ethereum"]
      forex_pairs: []      # e.g., ["USD/JPY", "EUR/USD"]
      indices: []          # e.g., ["^GSPC", "^IXIC"]
    processing_tier: light
```

---

## Database, NATS & Dependencies

- **No new database tables** — market data uses existing artifact/sync_state tables
- **No new NATS subjects** — market artifacts flow through standard `artifacts.process`
- **No Python sidecar changes** — market text is processed by standard ML pipeline
- **No new Go dependencies** — all APIs are plain REST/JSON, parsed with standard library
