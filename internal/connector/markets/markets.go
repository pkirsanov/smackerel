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
	// maxCoinGeckoBatchSize caps coin IDs per CoinGecko request to avoid URL length rejection.
	maxCoinGeckoBatchSize = 50
	// maxErrorBodySnippet limits error response body read for diagnostic logging.
	maxErrorBodySnippet = 512
)

var (
	// validSymbolRe matches standard stock/ETF ticker symbols (1-10 alphanumeric chars, dots, hyphens).
	validSymbolRe = regexp.MustCompile(`^[A-Za-z0-9.\-]{1,10}$`)
	// validCoinIDRe matches CoinGecko coin IDs (lowercase alphanumeric, hyphens).
	validCoinIDRe = regexp.MustCompile(`^[a-z0-9\-]{1,64}$`)
	// validForexPairRe matches forex pair format like USD/JPY, EUR/USD (3-letter/3-letter).
	validForexPairRe = regexp.MustCompile(`^[A-Z]{3}/[A-Z]{3}$`)

	// providerRateLimits is the single source of truth for per-provider rate limits (calls/minute).
	providerRateLimits = map[string]int{"finnhub": 55, "coingecko": 25, "fred": 100}
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

	c.mu.Lock()
	c.config = cfg
	c.health = connector.HealthHealthy
	c.mu.Unlock()
	slog.Info("financial-markets connector connected", "id", c.id,
		"stocks", len(cfg.Watchlist.Stocks), "crypto", len(cfg.Watchlist.Crypto))
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	// Snapshot config under lock to prevent data race with concurrent Connect().
	c.mu.Lock()
	c.health = connector.HealthSyncing
	cfg := c.config
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.health = connector.HealthHealthy
		c.mu.Unlock()
	}()

	var artifacts []connector.RawArtifact
	now := time.Now()

	// Warn if the entire watchlist is empty — likely misconfiguration.
	if len(cfg.Watchlist.Stocks) == 0 && len(cfg.Watchlist.ETFs) == 0 &&
		len(cfg.Watchlist.Crypto) == 0 && len(cfg.Watchlist.ForexPairs) == 0 {
		slog.Warn("financial-markets sync: watchlist is empty, no symbols to fetch")
	}

	// Safe copy — avoid append to cfg.Watchlist.Stocks backing array.
	allSymbols := make([]string, 0, len(cfg.Watchlist.Stocks)+len(cfg.Watchlist.ETFs))
	allSymbols = append(allSymbols, cfg.Watchlist.Stocks...)
	allSymbols = append(allSymbols, cfg.Watchlist.ETFs...)
	for _, symbol := range allSymbols {
		// Check context between HTTP calls for prompt cancellation.
		select {
		case <-ctx.Done():
			return artifacts, cursor, ctx.Err()
		default:
		}
		if !c.tryRecordCall("finnhub") {
			slog.Warn("finnhub rate limit reached, skipping remaining symbols")
			break
		}
		quote, err := c.fetchFinnhubQuote(ctx, symbol)
		if err != nil {
			slog.Warn("finnhub quote failed", "symbol", symbol, "error", err)
			continue
		}

		tier := classifyTier(cfg.AlertThreshold, quote.ChangePercent)

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
	if cfg.CoinGeckoEnabled && len(cfg.Watchlist.Crypto) > 0 {
		select {
		case <-ctx.Done():
			return artifacts, cursor, ctx.Err()
		default:
		}
		if c.tryRecordCall("coingecko") {
			prices, err := c.fetchCoinGeckoPrices(ctx, cfg.Watchlist.Crypto)
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
							"processing_tier": classifyTier(cfg.AlertThreshold, p.ChangePct24h),
						},
						CapturedAt: now,
					})
				}
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
	c.mu.Lock()
	c.health = connector.HealthDisconnected
	// Reset rate limit tracking so a subsequent Connect() starts fresh.
	c.callCounts = make(map[string][]time.Time)
	c.mu.Unlock()
	// Release HTTP transport idle connections to prevent resource leak.
	c.httpClient.CloseIdleConnections()
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
		// Read a small snippet for diagnostic logging, then drain remainder.
		snippet := make([]byte, maxErrorBodySnippet)
		n, _ := io.ReadFull(resp.Body, snippet)
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
		return nil, fmt.Errorf("finnhub returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet[:n])))
	}

	var quote StockQuote
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodyBytes)).Decode(&quote); err != nil {
		return nil, fmt.Errorf("decode finnhub response: %w", err)
	}
	quote.Symbol = symbol

	// Detect Finnhub "no data" response: all-zero fields indicate unknown/delisted symbol.
	if quote.CurrentPrice == 0 && quote.High == 0 && quote.Low == 0 && quote.PreviousClose == 0 {
		return nil, fmt.Errorf("finnhub returned no data for symbol %q (possibly delisted or invalid)", symbol)
	}

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
	if len(sanitizedIDs) > maxCoinGeckoBatchSize {
		slog.Warn("coingecko batch truncated to max size", "requested", len(sanitizedIDs), "max", maxCoinGeckoBatchSize)
		sanitizedIDs = sanitizedIDs[:maxCoinGeckoBatchSize]
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
		// Read a small snippet for diagnostic logging, then drain remainder.
		snippet := make([]byte, maxErrorBodySnippet)
		n, _ := io.ReadFull(resp.Body, snippet)
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
		return nil, fmt.Errorf("coingecko returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet[:n])))
	}

	var result map[string]map[string]float64
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodyBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode coingecko response: %w", err)
	}

	var prices []CryptoPrice
	for id, data := range result {
		price := data["usd"]
		changePct := data["usd_24h_change"]
		// Calculate absolute 24h change from percentage and current price.
		var change24h float64
		if changePct != 0 {
			change24h = price - (price / (1 + changePct/100))
		}
		prices = append(prices, CryptoPrice{
			ID:           id,
			CurrentPrice: price,
			Change24h:    change24h,
			ChangePct24h: changePct,
		})
	}

	return prices, nil
}

// classifyTier returns the processing tier based on threshold and change percent.
func classifyTier(threshold, changePct float64) string {
	if threshold > 0 && (changePct >= threshold || changePct <= -threshold) {
		return "full"
	}
	return "light"
}

// cryptoTier returns the processing tier for a crypto asset based on alert threshold.
func (c *Connector) cryptoTier(changePct24h float64) string {
	c.mu.RLock()
	threshold := c.config.AlertThreshold
	c.mu.RUnlock()
	return classifyTier(threshold, changePct24h)
}

// allowCall checks if a provider call is within rate limits.
func (c *Connector) allowCall(provider string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	maxPerMin := providerRateLimits[provider]
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

// tryRecordCall atomically checks the rate limit and records the call if allowed.
// This prevents the TOCTOU race between separate allowCall/recordCall calls.
func (c *Connector) tryRecordCall(provider string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	maxPerMin := providerRateLimits[provider]
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

	if len(valid) >= maxPerMin {
		c.callCounts[provider] = valid
		return false
	}

	valid = append(valid, time.Now())
	c.callCounts[provider] = valid
	return true
}

func parseMarketsConfig(config connector.ConnectorConfig) (MarketsConfig, error) {
	cfg := MarketsConfig{
		CoinGeckoEnabled: false,
		AlertThreshold:   5.0,
	}

	// Read coingecko_enabled from config — defaults to false (explicit opt-in)
	if cgEnabled, ok := config.SourceConfig["coingecko_enabled"].(bool); ok {
		cfg.CoinGeckoEnabled = cgEnabled
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
		if pairs, ok := wl["forex_pairs"].([]interface{}); ok {
			for _, s := range pairs {
				if str, ok := s.(string); ok {
					if !validForexPairRe.MatchString(str) {
						return MarketsConfig{}, fmt.Errorf("invalid forex pair: %q (expected format: USD/JPY)", str)
					}
					cfg.Watchlist.ForexPairs = append(cfg.Watchlist.ForexPairs, str)
				}
			}
			if len(cfg.Watchlist.ForexPairs) > maxWatchlistSymbols {
				return MarketsConfig{}, fmt.Errorf("forex pairs watchlist exceeds maximum of %d entries", maxWatchlistSymbols)
			}
		}
	}

	if threshold, ok := config.SourceConfig["alert_threshold"].(float64); ok {
		if threshold < 0 {
			return MarketsConfig{}, fmt.Errorf("alert_threshold must be non-negative, got %v", threshold)
		}
		cfg.AlertThreshold = threshold
	}

	return cfg, nil
}
