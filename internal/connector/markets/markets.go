package markets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

const (
	// maxResponseBodyBytes limits response body reads to 1MB to prevent OOM from malicious servers.
	maxResponseBodyBytes = 1 * 1024 * 1024
	// maxWatchlistSymbols limits watchlist entries per category to prevent excessive API calls.
	maxWatchlistSymbols = 100
)

var (
	// validSymbolRe matches standard stock/ETF ticker symbols (1-10 alphanumeric chars, dots, hyphens).
	validSymbolRe = regexp.MustCompile(`^[A-Za-z0-9.\-]{1,10}$`)
	// validCoinIDRe matches CoinGecko coin IDs (lowercase alphanumeric, hyphens).
	validCoinIDRe = regexp.MustCompile(`^[a-z0-9\-]{1,64}$`)
)

// Connector implements the Financial Markets connector using Finnhub, CoinGecko, and FRED.
type Connector struct {
	id         string
	health     connector.HealthStatus
	mu         sync.RWMutex
	config     MarketsConfig
	httpClient *http.Client
	callCounts map[string][]time.Time // per-provider rate tracking
}

// MarketsConfig holds parsed markets-specific configuration.
type MarketsConfig struct {
	FinnhubAPIKey    string
	CoinGeckoEnabled bool
	FREDAPIKey       string
	AlertThreshold   float64
	Watchlist        WatchlistConfig
}

// WatchlistConfig specifies what to track.
type WatchlistConfig struct {
	Stocks     []string `json:"stocks"`
	ETFs       []string `json:"etfs"`
	Crypto     []string `json:"crypto"`
	ForexPairs []string `json:"forex_pairs"`
}

// StockQuote represents a stock/ETF price quote.
type StockQuote struct {
	Symbol        string  `json:"symbol"`
	CurrentPrice  float64 `json:"c"`
	Change        float64 `json:"d"`
	ChangePercent float64 `json:"dp"`
	High          float64 `json:"h"`
	Low           float64 `json:"l"`
	Open          float64 `json:"o"`
	PreviousClose float64 `json:"pc"`
}

// CryptoPrice represents a cryptocurrency price.
type CryptoPrice struct {
	ID           string  `json:"id"`
	CurrentPrice float64 `json:"current_price"`
	Change24h    float64 `json:"price_change_24h"`
	ChangePct24h float64 `json:"price_change_percentage_24h"`
}

// New creates a new Financial Markets connector.
func New(id string) *Connector {
	return &Connector{
		id:         id,
		health:     connector.HealthDisconnected,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		callCounts: make(map[string][]time.Time),
	}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	cfg, err := parseMarketsConfig(config)
	if err != nil {
		return fmt.Errorf("parse markets config: %w", err)
	}
	if cfg.FinnhubAPIKey == "" {
		return fmt.Errorf("finnhub_api_key is required")
	}

	c.config = cfg
	c.health = connector.HealthHealthy
	slog.Info("financial-markets connector connected", "id", c.id,
		"stocks", len(cfg.Watchlist.Stocks), "crypto", len(cfg.Watchlist.Crypto))
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

	// Fetch stock/ETF quotes via Finnhub
	allSymbols := append(c.config.Watchlist.Stocks, c.config.Watchlist.ETFs...)
	for _, symbol := range allSymbols {
		if !c.allowCall("finnhub") {
			slog.Warn("finnhub rate limit reached, skipping remaining symbols")
			break
		}
		quote, err := c.fetchFinnhubQuote(ctx, symbol)
		if err != nil {
			slog.Warn("finnhub quote failed", "symbol", symbol, "error", err)
			continue
		}

		tier := "light"
		if c.config.AlertThreshold > 0 && (quote.ChangePercent >= c.config.AlertThreshold || quote.ChangePercent <= -c.config.AlertThreshold) {
			tier = "full"
		}

		artifacts = append(artifacts, connector.RawArtifact{
			SourceID:    "financial-markets",
			SourceRef:   fmt.Sprintf("quote-%s-%s", symbol, now.Format("2006-01-02")),
			ContentType: "market/quote",
			Title:       fmt.Sprintf("%s: $%.2f (%+.1f%%)", symbol, quote.CurrentPrice, quote.ChangePercent),
			RawContent:  fmt.Sprintf("%s: $%.2f (change: %+.2f / %+.1f%%), range: $%.2f–$%.2f", symbol, quote.CurrentPrice, quote.Change, quote.ChangePercent, quote.Low, quote.High),
			Metadata: map[string]interface{}{
				"symbol":          symbol,
				"price":           quote.CurrentPrice,
				"change":          quote.Change,
				"change_percent":  quote.ChangePercent,
				"high":            quote.High,
				"low":             quote.Low,
				"processing_tier": tier,
			},
			CapturedAt: now,
		})
	}

	// Fetch crypto prices via CoinGecko
	if c.config.CoinGeckoEnabled && len(c.config.Watchlist.Crypto) > 0 && c.allowCall("coingecko") {
		prices, err := c.fetchCoinGeckoPrices(ctx, c.config.Watchlist.Crypto)
		if err != nil {
			slog.Warn("coingecko fetch failed", "error", err)
		} else {
			for _, p := range prices {
				artifacts = append(artifacts, connector.RawArtifact{
					SourceID:    "financial-markets",
					SourceRef:   fmt.Sprintf("crypto-%s-%s", p.ID, now.Format("2006-01-02")),
					ContentType: "market/quote",
					Title:       fmt.Sprintf("%s: $%.2f (%+.1f%%)", p.ID, p.CurrentPrice, p.ChangePct24h),
					RawContent:  fmt.Sprintf("%s: $%.2f (24h change: %+.1f%%)", p.ID, p.CurrentPrice, p.ChangePct24h),
					Metadata: map[string]interface{}{
						"symbol":          p.ID,
						"asset_type":      "crypto",
						"price":           p.CurrentPrice,
						"change_24h":      p.Change24h,
						"change_pct_24h":  p.ChangePct24h,
						"processing_tier": "light",
					},
					CapturedAt: now,
				})
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

// fetchFinnhubQuote gets a stock quote from Finnhub.
func (c *Connector) fetchFinnhubQuote(ctx context.Context, symbol string) (*StockQuote, error) {
	if !validSymbolRe.MatchString(symbol) {
		return nil, fmt.Errorf("invalid symbol format: %q", symbol)
	}

	u, _ := url.Parse("https://finnhub.io/api/v1/quote")
	q := u.Query()
	q.Set("symbol", symbol)
	q.Set("token", c.config.FinnhubAPIKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finnhub request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("finnhub returned status %d", resp.StatusCode)
	}

	var quote StockQuote
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodyBytes)).Decode(&quote); err != nil {
		return nil, fmt.Errorf("decode finnhub response: %w", err)
	}
	quote.Symbol = symbol

	c.recordCall("finnhub")
	return &quote, nil
}

// fetchCoinGeckoPrices gets crypto prices from CoinGecko (no API key needed).
func (c *Connector) fetchCoinGeckoPrices(ctx context.Context, coinIDs []string) ([]CryptoPrice, error) {
	var sanitizedIDs []string
	for _, id := range coinIDs {
		if !validCoinIDRe.MatchString(id) {
			slog.Warn("skipping invalid coin ID", "id", id)
			continue
		}
		sanitizedIDs = append(sanitizedIDs, id)
	}
	if len(sanitizedIDs) == 0 {
		return nil, fmt.Errorf("no valid coin IDs provided")
	}

	u, _ := url.Parse("https://api.coingecko.com/api/v3/simple/price")
	q := u.Query()
	q.Set("ids", strings.Join(sanitizedIDs, ","))
	q.Set("vs_currencies", "usd")
	q.Set("include_24hr_change", "true")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("coingecko request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("coingecko returned status %d", resp.StatusCode)
	}

	var result map[string]map[string]float64
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodyBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode coingecko response: %w", err)
	}

	var prices []CryptoPrice
	for id, data := range result {
		prices = append(prices, CryptoPrice{
			ID:           id,
			CurrentPrice: data["usd"],
			ChangePct24h: data["usd_24h_change"],
		})
	}

	c.recordCall("coingecko")
	return prices, nil
}

// allowCall checks if a provider call is within rate limits.
func (c *Connector) allowCall(provider string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	limits := map[string]int{"finnhub": 55, "coingecko": 25, "fred": 100}
	maxPerMin := limits[provider]
	if maxPerMin == 0 {
		return true
	}

	cutoff := time.Now().Add(-time.Minute)
	valid := c.callCounts[provider][:0]
	for _, t := range c.callCounts[provider] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	c.callCounts[provider] = valid

	return len(valid) < maxPerMin
}

func (c *Connector) recordCall(provider string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callCounts[provider] = append(c.callCounts[provider], time.Now())
}

func parseMarketsConfig(config connector.ConnectorConfig) (MarketsConfig, error) {
	cfg := MarketsConfig{
		CoinGeckoEnabled: true,
		AlertThreshold:   5.0,
	}

	if key, ok := config.Credentials["finnhub_api_key"]; ok {
		cfg.FinnhubAPIKey = key
	}
	if key, ok := config.Credentials["fred_api_key"]; ok {
		cfg.FREDAPIKey = key
	}

	if wl, ok := config.SourceConfig["watchlist"].(map[string]interface{}); ok {
		if stocks, ok := wl["stocks"].([]interface{}); ok {
			for _, s := range stocks {
				if str, ok := s.(string); ok {
					if !validSymbolRe.MatchString(str) {
						return MarketsConfig{}, fmt.Errorf("invalid stock symbol: %q", str)
					}
					cfg.Watchlist.Stocks = append(cfg.Watchlist.Stocks, str)
				}
			}
			if len(cfg.Watchlist.Stocks) > maxWatchlistSymbols {
				return MarketsConfig{}, fmt.Errorf("stocks watchlist exceeds maximum of %d symbols", maxWatchlistSymbols)
			}
		}
		if etfs, ok := wl["etfs"].([]interface{}); ok {
			for _, s := range etfs {
				if str, ok := s.(string); ok {
					if !validSymbolRe.MatchString(str) {
						return MarketsConfig{}, fmt.Errorf("invalid ETF symbol: %q", str)
					}
					cfg.Watchlist.ETFs = append(cfg.Watchlist.ETFs, str)
				}
			}
			if len(cfg.Watchlist.ETFs) > maxWatchlistSymbols {
				return MarketsConfig{}, fmt.Errorf("ETFs watchlist exceeds maximum of %d symbols", maxWatchlistSymbols)
			}
		}
		if crypto, ok := wl["crypto"].([]interface{}); ok {
			for _, s := range crypto {
				if str, ok := s.(string); ok {
					if !validCoinIDRe.MatchString(str) {
						return MarketsConfig{}, fmt.Errorf("invalid crypto coin ID: %q", str)
					}
					cfg.Watchlist.Crypto = append(cfg.Watchlist.Crypto, str)
				}
			}
			if len(cfg.Watchlist.Crypto) > maxWatchlistSymbols {
				return MarketsConfig{}, fmt.Errorf("crypto watchlist exceeds maximum of %d symbols", maxWatchlistSymbols)
			}
		}
	}

	if threshold, ok := config.SourceConfig["alert_threshold"].(float64); ok {
		cfg.AlertThreshold = threshold
	}

	return cfg, nil
}
