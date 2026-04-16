package markets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/stringutil"
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
	// nyLocation caches the America/New_York timezone, loaded once at first use.
	nyLocation     *time.Location
	nyLocationOnce sync.Once
	nyLocationErr  error

	// validSymbolRe matches standard stock/ETF ticker symbols (1-10 alphanumeric chars, dots, hyphens).
	validSymbolRe = regexp.MustCompile(`^[A-Za-z0-9.\-]{1,10}$`)
	// validCoinIDRe matches CoinGecko coin IDs (lowercase alphanumeric, hyphens).
	validCoinIDRe = regexp.MustCompile(`^[a-z0-9\-]{1,64}$`)
	// validForexPairRe matches forex pair format like USD/JPY, EUR/USD (3-letter/3-letter).
	validForexPairRe = regexp.MustCompile(`^[A-Z]{3}/[A-Z]{3}$`)
	// validFREDSeriesRe matches FRED series IDs (uppercase alphanumeric, 1-20 chars).
	validFREDSeriesRe = regexp.MustCompile(`^[A-Z0-9]{1,20}$`)

	// providerRateLimits is the single source of truth for per-provider rate limits (calls/minute).
	providerRateLimits = map[string]int{"finnhub": 55, "coingecko": 25, "fred": 100}

	// defaultFREDSeries is the default set of FRED economic indicator series.
	defaultFREDSeries = []string{"GDP", "UNRATE", "CPIAUCSL", "DFF", "FEDFUNDS"}
)

// Compile-time interface check.
var _ connector.Connector = (*Connector)(nil)

// Connector implements the Financial Markets connector using Finnhub, CoinGecko, and FRED.
type Connector struct {
	id         string
	health     connector.HealthStatus
	mu         sync.RWMutex
	config     MarketsConfig
	httpClient *http.Client
	callCounts map[string][]time.Time // per-provider rate tracking
	configGen  uint64                 // incremented on Connect; Sync uses it to skip stale health writes

	// Base URLs for API providers — overridable for testing via httptest.
	finnhubBaseURL   string
	coingeckoBaseURL string
	fredBaseURL      string

	// lastSummaryDate tracks the last date (YYYY-MM-DD in ET) a daily summary was generated.
	lastSummaryDate string
	// nowFunc overrides time.Now for testing time-dependent behavior.
	nowFunc func() time.Time
}

// MarketsConfig holds parsed markets-specific configuration.
type MarketsConfig struct {
	FinnhubAPIKey    string
	CoinGeckoEnabled bool
	FREDAPIKey       string
	FREDEnabled      bool
	FREDSeries       []string
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

// NewsArticle represents a Finnhub company news article.
type NewsArticle struct {
	Category string `json:"category"`
	Datetime int64  `json:"datetime"`
	Headline string `json:"headline"`
	ID       int64  `json:"id"`
	Image    string `json:"image"`
	Related  string `json:"related"`
	Source   string `json:"source"`
	Summary  string `json:"summary"`
	URL      string `json:"url"`
}

// FREDObservation represents a single FRED economic data observation.
type FREDObservation struct {
	SeriesID string
	Date     string  `json:"date"`
	Value    string  `json:"value"`
	NumValue float64 // parsed from Value
}

// New creates a new Financial Markets connector.
func New(id string) *Connector {
	return &Connector{
		id:               id,
		health:           connector.HealthDisconnected,
		httpClient:       &http.Client{Timeout: 10 * time.Second},
		callCounts:       make(map[string][]time.Time),
		finnhubBaseURL:   "https://finnhub.io",
		coingeckoBaseURL: "https://api.coingecko.com",
		fredBaseURL:      "https://api.stlouisfed.org",
	}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	cfg, err := parseMarketsConfig(config)
	if err != nil {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("parse markets config: %w", err)
	}
	if cfg.FinnhubAPIKey == "" {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("finnhub_api_key is required")
	}

	c.mu.Lock()
	c.config = cfg
	c.health = connector.HealthHealthy
	// Reset rate limit tracking so a fresh Connect() starts with clean budgets.
	c.callCounts = make(map[string][]time.Time)
	c.configGen++
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
	gen := c.configGen
	c.mu.Unlock()

	var failCount int
	var totalProviders int
	var partialSkip bool
	defer func() {
		c.mu.Lock()
		// Only update health if no concurrent Connect() has occurred since Sync started.
		if c.configGen == gen {
			if totalProviders > 0 && failCount >= totalProviders {
				c.health = connector.HealthError
			} else if failCount > 0 || partialSkip {
				c.health = connector.HealthDegraded
			} else {
				c.health = connector.HealthHealthy
			}
		}
		c.mu.Unlock()
	}()

	var artifacts []connector.RawArtifact
	now := time.Now()
	if c.nowFunc != nil {
		now = c.nowFunc()
	}

	// Warn if the entire watchlist is empty — likely misconfiguration.
	if len(cfg.Watchlist.Stocks) == 0 && len(cfg.Watchlist.ETFs) == 0 &&
		len(cfg.Watchlist.Crypto) == 0 && len(cfg.Watchlist.ForexPairs) == 0 {
		slog.Warn("financial-markets sync: watchlist is empty, no symbols to fetch")
	}

	// Safe copy — avoid append to cfg.Watchlist.Stocks backing array.
	allSymbols := make([]string, 0, len(cfg.Watchlist.Stocks)+len(cfg.Watchlist.ETFs))
	allSymbols = append(allSymbols, cfg.Watchlist.Stocks...)
	allSymbols = append(allSymbols, cfg.Watchlist.ETFs...)
	if len(allSymbols) > 0 {
		totalProviders++
	}
	var finnhubFails int
	for _, symbol := range allSymbols {
		// Check context between HTTP calls for prompt cancellation.
		select {
		case <-ctx.Done():
			return artifacts, cursor, ctx.Err()
		default:
		}
		if !c.tryRecordCall("finnhub") {
			slog.Warn("finnhub rate limit reached, skipping remaining symbols")
			partialSkip = true
			break
		}
		quote, err := c.fetchFinnhubQuote(ctx, symbol)
		if err != nil {
			slog.Warn("finnhub quote failed", "symbol", symbol, "error", err)
			finnhubFails++
			continue
		}

		tier := classifyTier(cfg.AlertThreshold, quote.ChangePercent)

		// Determine asset_type: ETFs are listed separately in config.
		assetType := "stock"
		for _, etf := range cfg.Watchlist.ETFs {
			if etf == symbol {
				assetType = "etf"
				break
			}
		}

		artifacts = append(artifacts, connector.RawArtifact{
			SourceID:    "financial-markets",
			SourceRef:   fmt.Sprintf("quote-%s-%s", symbol, now.Format("2006-01-02")),
			ContentType: "market/quote",
			Title:       fmt.Sprintf("%s: $%.2f (%+.1f%%)", symbol, quote.CurrentPrice, quote.ChangePercent),
			RawContent:  fmt.Sprintf("%s: $%.2f (change: %+.2f / %+.1f%%), range: $%.2f–$%.2f", symbol, quote.CurrentPrice, quote.Change, quote.ChangePercent, quote.Low, quote.High),
			Metadata: map[string]interface{}{
				"symbol":          symbol,
				"asset_type":      assetType,
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

	if finnhubFails > 0 && finnhubFails >= len(allSymbols) {
		failCount++
	}

	// Fetch crypto prices via CoinGecko
	if cfg.CoinGeckoEnabled && len(cfg.Watchlist.Crypto) > 0 {
		totalProviders++
		select {
		case <-ctx.Done():
			return artifacts, cursor, ctx.Err()
		default:
		}
		if c.tryRecordCall("coingecko") {
			prices, err := c.fetchCoinGeckoPrices(ctx, cfg.Watchlist.Crypto)
			if err != nil {
				slog.Warn("coingecko fetch failed", "error", err)
				failCount++
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
		} else {
			slog.Warn("coingecko rate limit reached, skipping crypto fetch")
			partialSkip = true
		}
	}

	// Fetch forex rates via Finnhub
	if len(cfg.Watchlist.ForexPairs) > 0 {
		totalProviders++ // count forex as a distinct provider dimension
		var forexFails int
		for _, pair := range cfg.Watchlist.ForexPairs {
			select {
			case <-ctx.Done():
				return artifacts, cursor, ctx.Err()
			default:
			}
			if !c.tryRecordCall("finnhub") {
				slog.Warn("finnhub rate limit reached, skipping remaining forex pairs")
				partialSkip = true
				break
			}
			quote, err := c.fetchFinnhubForex(ctx, pair)
			if err != nil {
				slog.Warn("finnhub forex failed", "pair", pair, "error", err)
				forexFails++
				continue
			}
			artifacts = append(artifacts, connector.RawArtifact{
				SourceID:    "financial-markets",
				SourceRef:   fmt.Sprintf("forex-%s-%s", strings.ReplaceAll(pair, "/", "-"), now.Format("2006-01-02")),
				ContentType: "market/quote",
				Title:       fmt.Sprintf("%s: %.4f", pair, quote.CurrentPrice),
				RawContent:  fmt.Sprintf("%s: %.4f (change: %+.4f / %+.2f%%)", pair, quote.CurrentPrice, quote.Change, quote.ChangePercent),
				Metadata: map[string]interface{}{
					"symbol":          pair,
					"asset_type":      "forex",
					"price":           quote.CurrentPrice,
					"change":          quote.Change,
					"change_percent":  quote.ChangePercent,
					"processing_tier": classifyTier(cfg.AlertThreshold, quote.ChangePercent),
				},
				CapturedAt: now,
			})
		}
		if forexFails > 0 && forexFails >= len(cfg.Watchlist.ForexPairs) {
			failCount++
		}
	}

	// Fetch company news for watchlist stocks via Finnhub.
	// Use today's date as the range for fresh news.
	newsDate := now.Format("2006-01-02")
	watchlistSet := make(map[string]bool, len(cfg.Watchlist.Stocks))
	for _, s := range cfg.Watchlist.Stocks {
		watchlistSet[s] = true
	}
	for _, symbol := range cfg.Watchlist.Stocks {
		select {
		case <-ctx.Done():
			return artifacts, cursor, ctx.Err()
		default:
		}
		if !c.tryRecordCall("finnhub") {
			slog.Warn("finnhub rate limit reached, skipping remaining company news")
			partialSkip = true
			break
		}
		articles, err := c.fetchFinnhubCompanyNews(ctx, symbol, newsDate, newsDate)
		if err != nil {
			slog.Warn("finnhub company news failed", "symbol", symbol, "error", err)
			continue
		}
		for _, article := range articles {
			tier := "standard"
			if watchlistSet[symbol] {
				tier = "standard"
			}
			// IMP-018-SQS-001: Sanitize API-supplied text fields (CWE-116).
			headline := stringutil.SanitizeControlChars(article.Headline)
			summary := stringutil.SanitizeControlChars(article.Summary)
			source := stringutil.SanitizeControlChars(article.Source)
			category := stringutil.SanitizeControlChars(article.Category)
			artifacts = append(artifacts, connector.RawArtifact{
				SourceID:    "financial-markets",
				SourceRef:   fmt.Sprintf("news-%s-%d", symbol, article.ID),
				ContentType: "market/news",
				Title:       headline,
				RawContent:  summary,
				URL:         article.URL,
				Metadata: map[string]interface{}{
					"symbol":          symbol,
					"source":          source,
					"category":        category,
					"related":         article.Related,
					"datetime":        article.Datetime,
					"processing_tier": tier,
				},
				CapturedAt: time.Unix(article.Datetime, 0),
			})
		}
	}

	// Fetch FRED economic indicators.
	if cfg.FREDEnabled && len(cfg.FREDSeries) > 0 {
		totalProviders++
		var fredFails int
		for _, seriesID := range cfg.FREDSeries {
			select {
			case <-ctx.Done():
				return artifacts, cursor, ctx.Err()
			default:
			}
			if !c.tryRecordCall("fred") {
				slog.Warn("FRED rate limit reached, skipping remaining series")
				partialSkip = true
				break
			}
			obs, err := c.fetchFREDLatest(ctx, seriesID)
			if err != nil {
				slog.Warn("FRED fetch failed", "series", seriesID, "error", err)
				fredFails++
				continue
			}
			artifacts = append(artifacts, connector.RawArtifact{
				SourceID:    "financial-markets",
				SourceRef:   fmt.Sprintf("fred-%s-%s", seriesID, obs.Date),
				ContentType: "market/economic",
				Title:       fmt.Sprintf("%s: %s (as of %s)", seriesID, obs.Value, obs.Date),
				RawContent:  fmt.Sprintf("FRED %s: %s (observation date: %s)", seriesID, obs.Value, obs.Date),
				Metadata: map[string]interface{}{
					"series_id":       seriesID,
					"value":           obs.NumValue,
					"date":            obs.Date,
					"processing_tier": "standard",
				},
				CapturedAt: now,
			})
		}
		if fredFails > 0 && fredFails >= len(cfg.FREDSeries) {
			failCount++
		}
	}

	// Enrich artifacts with detected and related symbol metadata (Scope 6).
	enrichArtifactsWithSymbols(artifacts, cfg)

	// Generate daily summary if time gate passes (Scope 5).
	if c.shouldGenerateDailySummary(now) {
		summary := buildDailySummary(artifacts, now)
		artifacts = append(artifacts, summary)
		c.mu.Lock()
		// nyLocation is guaranteed non-nil here because shouldGenerateDailySummary
		// returned true, which requires successful LoadLocation.
		c.lastSummaryDate = now.In(nyLocation).Format("2006-01-02")
		c.mu.Unlock()
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

	u, err := url.Parse(c.finnhubBaseURL + "/api/v1/quote")
	if err != nil {
		return nil, fmt.Errorf("parse finnhub URL: %w", err)
	}
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

// fetchFinnhubForex gets a forex exchange rate from Finnhub.
// The pair must be in "BASE/QUOTE" format (e.g., "USD/JPY").
// Finnhub's forex endpoint uses the format "OANDA:BASE_QUOTE".
func (c *Connector) fetchFinnhubForex(ctx context.Context, pair string) (*StockQuote, error) {
	if !validForexPairRe.MatchString(pair) {
		return nil, fmt.Errorf("invalid forex pair format: %q", pair)
	}

	// Convert "USD/JPY" → "OANDA:USD_JPY" for Finnhub forex endpoint.
	finnhubSymbol := "OANDA:" + strings.ReplaceAll(pair, "/", "_")

	u, err := url.Parse(c.finnhubBaseURL + "/api/v1/quote")
	if err != nil {
		return nil, fmt.Errorf("parse finnhub URL: %w", err)
	}
	q := u.Query()
	q.Set("symbol", finnhubSymbol)
	q.Set("token", c.config.FinnhubAPIKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finnhub forex request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet := make([]byte, maxErrorBodySnippet)
		n, _ := io.ReadFull(resp.Body, snippet)
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
		return nil, fmt.Errorf("finnhub forex returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet[:n])))
	}

	var quote StockQuote
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodyBytes)).Decode(&quote); err != nil {
		return nil, fmt.Errorf("decode finnhub forex response: %w", err)
	}
	quote.Symbol = pair

	if quote.CurrentPrice == 0 && quote.High == 0 && quote.Low == 0 && quote.PreviousClose == 0 {
		return nil, fmt.Errorf("finnhub returned no forex data for pair %q", pair)
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

	u, err := url.Parse(c.coingeckoBaseURL + "/api/v3/simple/price")
	if err != nil {
		return nil, fmt.Errorf("parse coingecko URL: %w", err)
	}
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
		// Guard: if changePct <= -100, the denominator (1 + pct/100) is zero or negative,
		// producing Inf/NaN which corrupts JSON serialization downstream.
		var change24h float64
		if changePct != 0 && changePct > -100 {
			change24h = price - (price / (1 + changePct/100))
		} else if changePct <= -100 {
			// Total loss: change equals the entire previous price.
			change24h = -price
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

// fetchFinnhubCompanyNews gets recent news articles for a symbol from Finnhub.
func (c *Connector) fetchFinnhubCompanyNews(ctx context.Context, symbol, fromDate, toDate string) ([]NewsArticle, error) {
	if !validSymbolRe.MatchString(symbol) {
		return nil, fmt.Errorf("invalid symbol format: %q", symbol)
	}

	u, err := url.Parse(c.finnhubBaseURL + "/api/v1/company-news")
	if err != nil {
		return nil, fmt.Errorf("parse finnhub news URL: %w", err)
	}
	q := u.Query()
	q.Set("symbol", symbol)
	q.Set("from", fromDate)
	q.Set("to", toDate)
	q.Set("token", c.config.FinnhubAPIKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finnhub news request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet := make([]byte, maxErrorBodySnippet)
		n, _ := io.ReadFull(resp.Body, snippet)
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
		return nil, fmt.Errorf("finnhub news returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet[:n])))
	}

	var articles []NewsArticle
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodyBytes)).Decode(&articles); err != nil {
		return nil, fmt.Errorf("decode finnhub news response: %w", err)
	}

	return articles, nil
}

// fetchFREDLatest gets the latest observation for a FRED economic series.
func (c *Connector) fetchFREDLatest(ctx context.Context, seriesID string) (*FREDObservation, error) {
	if !validFREDSeriesRe.MatchString(seriesID) {
		return nil, fmt.Errorf("invalid FRED series ID: %q", seriesID)
	}

	u, err := url.Parse(c.fredBaseURL + "/fred/series/observations")
	if err != nil {
		return nil, fmt.Errorf("parse FRED URL: %w", err)
	}
	q := u.Query()
	q.Set("series_id", seriesID)
	q.Set("api_key", c.config.FREDAPIKey)
	q.Set("file_type", "json")
	q.Set("limit", "1")
	q.Set("sort_order", "desc")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FRED request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet := make([]byte, maxErrorBodySnippet)
		n, _ := io.ReadFull(resp.Body, snippet)
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
		return nil, fmt.Errorf("FRED returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet[:n])))
	}

	var result struct {
		Observations []struct {
			Date  string `json:"date"`
			Value string `json:"value"`
		} `json:"observations"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodyBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode FRED response: %w", err)
	}

	if len(result.Observations) == 0 {
		return nil, fmt.Errorf("FRED returned no observations for series %q", seriesID)
	}

	obs := result.Observations[0]
	// FRED uses "." for missing data in some series.
	if obs.Value == "." {
		return nil, fmt.Errorf("FRED returned missing data marker for series %q", seriesID)
	}

	numVal, err := strconv.ParseFloat(obs.Value, 64)
	if err != nil {
		return nil, fmt.Errorf("parse FRED value %q for series %q: %w", obs.Value, seriesID, err)
	}

	return &FREDObservation{
		SeriesID: seriesID,
		Date:     obs.Date,
		Value:    obs.Value,
		NumValue: numVal,
	}, nil
}

// classifyTier returns the processing tier based on threshold and change percent.
// NaN/Inf changePct is treated as "full" to avoid silently suppressing alerts on corrupt data.
func classifyTier(threshold, changePct float64) string {
	if math.IsNaN(changePct) || math.IsInf(changePct, 0) {
		return "full"
	}
	if threshold > 0 && (changePct >= threshold || changePct <= -threshold) {
		return "full"
	}
	return "light"
}

// tryRecordCall atomically checks the rate limit and records the call if allowed.
// This prevents the TOCTOU race between separate check/record calls.
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
			for i, s := range stocks {
				str, ok := s.(string)
				if !ok {
					return MarketsConfig{}, fmt.Errorf("watchlist stocks[%d]: expected string, got %T", i, s)
				}
				if !validSymbolRe.MatchString(str) {
					return MarketsConfig{}, fmt.Errorf("invalid stock symbol: %q", str)
				}
				cfg.Watchlist.Stocks = append(cfg.Watchlist.Stocks, str)
			}
			if len(cfg.Watchlist.Stocks) > maxWatchlistSymbols {
				return MarketsConfig{}, fmt.Errorf("stocks watchlist exceeds maximum of %d symbols", maxWatchlistSymbols)
			}
		}
		if etfs, ok := wl["etfs"].([]interface{}); ok {
			for i, s := range etfs {
				str, ok := s.(string)
				if !ok {
					return MarketsConfig{}, fmt.Errorf("watchlist etfs[%d]: expected string, got %T", i, s)
				}
				if !validSymbolRe.MatchString(str) {
					return MarketsConfig{}, fmt.Errorf("invalid ETF symbol: %q", str)
				}
				cfg.Watchlist.ETFs = append(cfg.Watchlist.ETFs, str)
			}
			if len(cfg.Watchlist.ETFs) > maxWatchlistSymbols {
				return MarketsConfig{}, fmt.Errorf("ETFs watchlist exceeds maximum of %d symbols", maxWatchlistSymbols)
			}
		}
		if crypto, ok := wl["crypto"].([]interface{}); ok {
			for i, s := range crypto {
				str, ok := s.(string)
				if !ok {
					return MarketsConfig{}, fmt.Errorf("watchlist crypto[%d]: expected string, got %T", i, s)
				}
				if !validCoinIDRe.MatchString(str) {
					return MarketsConfig{}, fmt.Errorf("invalid crypto coin ID: %q", str)
				}
				cfg.Watchlist.Crypto = append(cfg.Watchlist.Crypto, str)
			}
			if len(cfg.Watchlist.Crypto) > maxWatchlistSymbols {
				return MarketsConfig{}, fmt.Errorf("crypto watchlist exceeds maximum of %d symbols", maxWatchlistSymbols)
			}
		}
		if pairs, ok := wl["forex_pairs"].([]interface{}); ok {
			for i, s := range pairs {
				str, ok := s.(string)
				if !ok {
					return MarketsConfig{}, fmt.Errorf("watchlist forex_pairs[%d]: expected string, got %T", i, s)
				}
				if !validForexPairRe.MatchString(str) {
					return MarketsConfig{}, fmt.Errorf("invalid forex pair: %q (expected format: USD/JPY)", str)
				}
				cfg.Watchlist.ForexPairs = append(cfg.Watchlist.ForexPairs, str)
			}
			if len(cfg.Watchlist.ForexPairs) > maxWatchlistSymbols {
				return MarketsConfig{}, fmt.Errorf("forex pairs watchlist exceeds maximum of %d entries", maxWatchlistSymbols)
			}
		}
	}

	if threshold, ok := config.SourceConfig["alert_threshold"].(float64); ok {
		if math.IsNaN(threshold) || math.IsInf(threshold, 0) {
			return MarketsConfig{}, fmt.Errorf("alert_threshold must be a finite number, got %v", threshold)
		}
		if threshold < 0 {
			return MarketsConfig{}, fmt.Errorf("alert_threshold must be non-negative, got %v", threshold)
		}
		cfg.AlertThreshold = threshold
	}

	// FRED configuration: enabled when API key is provided.
	if cfg.FREDAPIKey != "" {
		cfg.FREDEnabled = true
	}
	if fredEnabled, ok := config.SourceConfig["fred_enabled"].(bool); ok {
		cfg.FREDEnabled = fredEnabled
		// If explicitly enabled but no API key, that's an error.
		if fredEnabled && cfg.FREDAPIKey == "" {
			return MarketsConfig{}, fmt.Errorf("fred_enabled is true but fred_api_key is empty")
		}
	}
	// Parse FRED series list from config, fallback to defaults.
	cfg.FREDSeries = defaultFREDSeries
	if series, ok := config.SourceConfig["fred_series"].([]interface{}); ok {
		cfg.FREDSeries = nil
		for i, s := range series {
			str, ok := s.(string)
			if !ok {
				return MarketsConfig{}, fmt.Errorf("fred_series[%d]: expected string, got %T", i, s)
			}
			if !validFREDSeriesRe.MatchString(str) {
				return MarketsConfig{}, fmt.Errorf("invalid FRED series ID: %q", str)
			}
			cfg.FREDSeries = append(cfg.FREDSeries, str)
		}
	}

	return cfg, nil
}

// --- Scope 5: Daily Summary ---

var (
	// tickerInTextRe matches $TICKER patterns in text (e.g., $AAPL, $BTC).
	tickerInTextRe = regexp.MustCompile(`\$([A-Z]{1,5})\b`)

	// falsePositiveSymbols are common words that look like tickers but aren't.
	falsePositiveSymbols = map[string]bool{
		"IT": true, "A": true, "I": true, "AT": true, "TO": true,
		"IS": true, "ON": true, "OR": true, "AN": true, "AS": true,
		"BY": true, "IF": true, "IN": true, "OF": true, "SO": true,
		"UP": true, "US": true, "DO": true, "GO": true, "NO": true,
		"AM": true, "PM": true, "TV": true, "UK": true, "EU": true,
		"OK": true, "HE": true, "ME": true, "WE": true,
		"ALL": true, "THE": true, "FOR": true, "CEO": true, "IPO": true,
	}

	// companyNameMap maps common company/crypto names (lowercase) to their ticker symbols.
	companyNameMap = map[string]string{
		"apple":     "AAPL",
		"google":    "GOOGL",
		"alphabet":  "GOOGL",
		"microsoft": "MSFT",
		"amazon":    "AMZN",
		"tesla":     "TSLA",
		"meta":      "META",
		"facebook":  "META",
		"nvidia":    "NVDA",
		"netflix":   "NFLX",
		"bitcoin":   "BTC",
		"ethereum":  "ETH",
	}
)

// shouldGenerateDailySummary returns true if a daily summary should be appended.
// Summary is generated on weekdays after 16:30 ET if not already generated today.
func (c *Connector) shouldGenerateDailySummary(now time.Time) bool {
	nyLocationOnce.Do(func() {
		nyLocation, nyLocationErr = time.LoadLocation("America/New_York")
	})
	if nyLocationErr != nil {
		slog.Warn("failed to load America/New_York timezone, skipping daily summary", "error", nyLocationErr)
		return false
	}
	nowET := now.In(nyLocation)

	// Skip weekends.
	day := nowET.Weekday()
	if day == time.Saturday || day == time.Sunday {
		return false
	}

	// Only after 16:30 ET (market close).
	hour, min, _ := nowET.Clock()
	if hour < 16 || (hour == 16 && min < 30) {
		return false
	}

	// Check if already generated today.
	today := nowET.Format("2006-01-02")
	c.mu.RLock()
	lastDate := c.lastSummaryDate
	c.mu.RUnlock()

	return lastDate != today
}

// buildDailySummary aggregates sync artifacts into a single market/daily-summary artifact.
func buildDailySummary(artifacts []connector.RawArtifact, now time.Time) connector.RawArtifact {
	var gainers, losers, unchanged, alerts, newsHeadlines, economic, allSymbols []string
	hasAlert := false

	for _, a := range artifacts {
		switch a.ContentType {
		case "market/quote":
			sym, _ := a.Metadata["symbol"].(string)
			changePct, _ := a.Metadata["change_percent"].(float64)
			// For crypto, use change_pct_24h.
			if cpct, ok := a.Metadata["change_pct_24h"].(float64); ok {
				changePct = cpct
			}
			if sym != "" {
				allSymbols = append(allSymbols, sym)
			}
			switch {
			case changePct > 0:
				gainers = append(gainers, fmt.Sprintf("%s (%+.1f%%)", sym, changePct))
			case changePct < 0:
				losers = append(losers, fmt.Sprintf("%s (%+.1f%%)", sym, changePct))
			default:
				unchanged = append(unchanged, sym)
			}
			if tier, _ := a.Metadata["processing_tier"].(string); tier == "full" {
				alerts = append(alerts, fmt.Sprintf("%s: %+.1f%%", sym, changePct))
				hasAlert = true
			}
		case "market/news":
			newsHeadlines = append(newsHeadlines, a.Title)
		case "market/economic":
			seriesID, _ := a.Metadata["series_id"].(string)
			value, _ := a.Metadata["value"].(float64)
			date, _ := a.Metadata["date"].(string)
			economic = append(economic, fmt.Sprintf("%s: %.2f (%s)", seriesID, value, date))
		}
	}

	var sb strings.Builder
	sb.WriteString("Daily Market Summary\n\n")
	if len(alerts) > 0 {
		sb.WriteString("ALERTS:\n")
		for _, a := range alerts {
			sb.WriteString("  ! " + a + "\n")
		}
		sb.WriteString("\n")
	}
	if len(gainers) > 0 {
		sb.WriteString("Gainers:\n")
		for _, g := range gainers {
			sb.WriteString("  + " + g + "\n")
		}
		sb.WriteString("\n")
	}
	if len(losers) > 0 {
		sb.WriteString("Losers:\n")
		for _, l := range losers {
			sb.WriteString("  - " + l + "\n")
		}
		sb.WriteString("\n")
	}
	if len(unchanged) > 0 {
		sb.WriteString("Unchanged: " + strings.Join(unchanged, ", ") + "\n\n")
	}
	if len(newsHeadlines) > 0 {
		sb.WriteString("News:\n")
		for _, h := range newsHeadlines {
			sb.WriteString("  * " + h + "\n")
		}
		sb.WriteString("\n")
	}
	if len(economic) > 0 {
		sb.WriteString("Economic Indicators:\n")
		for _, e := range economic {
			sb.WriteString("  * " + e + "\n")
		}
	}

	tier := "standard"
	if hasAlert {
		tier = "full"
	}

	return connector.RawArtifact{
		SourceID:    "financial-markets",
		SourceRef:   fmt.Sprintf("daily-summary-%s", now.Format("2006-01-02")),
		ContentType: "market/daily-summary",
		Title:       fmt.Sprintf("Market Summary — %s", now.Format("Jan 2, 2006")),
		RawContent:  sb.String(),
		Metadata: map[string]interface{}{
			"processing_tier": tier,
			"gainers_count":   len(gainers),
			"losers_count":    len(losers),
			"alerts_count":    len(alerts),
			"news_count":      len(newsHeadlines),
			"related_symbols": allSymbols,
		},
		CapturedAt: now,
	}
}

// --- Scope 6: Cross-Artifact Symbol Linking ---

// ResolveSymbols scans text for financial symbols ($TICKER patterns and company names).
// Returns a deduplicated list of resolved ticker symbols. Conservative: prefers precision over recall.
func ResolveSymbols(text string) []string {
	seen := make(map[string]bool)
	var symbols []string

	// Match $TICKER patterns.
	matches := tickerInTextRe.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) >= 2 {
			sym := m[1]
			if !falsePositiveSymbols[sym] && !seen[sym] {
				seen[sym] = true
				symbols = append(symbols, sym)
			}
		}
	}

	// Match company names (case-insensitive prefix match).
	lower := strings.ToLower(text)
	for name, ticker := range companyNameMap {
		if strings.Contains(lower, name) && !seen[ticker] {
			seen[ticker] = true
			symbols = append(symbols, ticker)
		}
	}

	return symbols
}

// enrichArtifactsWithSymbols adds related_symbols and detected_symbols metadata to artifacts.
func enrichArtifactsWithSymbols(artifacts []connector.RawArtifact, cfg MarketsConfig) {
	// Collect all watchlist symbols for economic artifacts.
	allWatchlistSymbols := make([]string, 0, len(cfg.Watchlist.Stocks)+len(cfg.Watchlist.ETFs)+len(cfg.Watchlist.Crypto))
	allWatchlistSymbols = append(allWatchlistSymbols, cfg.Watchlist.Stocks...)
	allWatchlistSymbols = append(allWatchlistSymbols, cfg.Watchlist.ETFs...)
	for _, c := range cfg.Watchlist.Crypto {
		allWatchlistSymbols = append(allWatchlistSymbols, strings.ToUpper(c))
	}

	for i := range artifacts {
		a := &artifacts[i]
		switch a.ContentType {
		case "market/quote":
			if sym, ok := a.Metadata["symbol"].(string); ok {
				a.Metadata["related_symbols"] = []string{sym}
			}
		case "market/news":
			text := a.Title + " " + a.RawContent
			detected := ResolveSymbols(text)
			// Ensure the primary symbol is always included.
			if sym, ok := a.Metadata["symbol"].(string); ok {
				found := false
				for _, d := range detected {
					if d == sym {
						found = true
						break
					}
				}
				if !found {
					detected = append([]string{sym}, detected...)
				}
			}
			a.Metadata["related_symbols"] = detected
			a.Metadata["detected_symbols"] = detected
		case "market/economic":
			a.Metadata["related_symbols"] = allWatchlistSymbols
		}
	}
}
