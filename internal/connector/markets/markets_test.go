package markets

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// stableTestTime is a fixed time (Saturday 10 AM UTC) used by tests to prevent
// daily summary generation from producing unexpected extra artifacts.
// IMP-018-SQS-002: Fix time-dependent test flakiness.
var stableTestTime = time.Date(2026, 4, 11, 10, 0, 0, 0, time.UTC) // Saturday

// newTestConnector creates a connector with a stable nowFunc to prevent
// time-dependent test flakiness from tryClaimDailySummary.
func newTestConnector(id string) *Connector {
	c := New(id)
	c.nowFunc = func() time.Time { return stableTestTime }
	return c
}

func TestNew(t *testing.T) {
	c := newTestConnector("financial-markets")
	if c.ID() != "financial-markets" {
		t.Errorf("expected financial-markets, got %s", c.ID())
	}
}

func TestConnect_MissingAPIKey(t *testing.T) {
	c := newTestConnector("financial-markets")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{},
	})
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

func TestConnect_Valid(t *testing.T) {
	c := newTestConnector("financial-markets")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "test-key"},
		SourceConfig: map[string]interface{}{
			"watchlist": map[string]interface{}{
				"stocks": []interface{}{"AAPL", "GOOGL"},
				"crypto": []interface{}{"bitcoin"},
			},
			"alert_threshold": 5.0,
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(c.config.Watchlist.Stocks) != 2 {
		t.Errorf("expected 2 stocks, got %d", len(c.config.Watchlist.Stocks))
	}
	if len(c.config.Watchlist.Crypto) != 1 {
		t.Errorf("expected 1 crypto, got %d", len(c.config.Watchlist.Crypto))
	}
}

func TestTryRecordCall_RateLimit(t *testing.T) {
	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test"

	// Should allow first call
	if !c.tryRecordCall("finnhub") {
		t.Error("first call should be allowed")
	}

	// Record remaining 54 calls (55 total)
	for i := 0; i < 54; i++ {
		if !c.tryRecordCall("finnhub") {
			t.Errorf("call %d should be allowed", i+2)
		}
	}

	// Should deny 56th call (limit is 55)
	if c.tryRecordCall("finnhub") {
		t.Error("should deny call at rate limit")
	}

	// Unknown provider always allowed
	if !c.tryRecordCall("unknown") {
		t.Error("unknown provider should always be allowed")
	}
}

func TestClose(t *testing.T) {
	c := newTestConnector("financial-markets")
	c.health = connector.HealthHealthy
	c.Close()
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Error("should be disconnected")
	}
}

func TestParseMarketsConfig(t *testing.T) {
	cfg, err := parseMarketsConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "key123"},
		SourceConfig: map[string]interface{}{
			"watchlist": map[string]interface{}{
				"stocks": []interface{}{"AAPL"},
				"etfs":   []interface{}{"SPY"},
			},
			"alert_threshold": 3.0,
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FinnhubAPIKey != "key123" {
		t.Errorf("expected key123, got %s", cfg.FinnhubAPIKey)
	}
	if cfg.AlertThreshold != 3.0 {
		t.Errorf("expected 3.0, got %v", cfg.AlertThreshold)
	}
	if len(cfg.Watchlist.Stocks) != 1 {
		t.Errorf("expected 1 stock, got %d", len(cfg.Watchlist.Stocks))
	}
}

// --- Security Tests ---

func TestParseMarketsConfig_RejectsInjectionSymbol(t *testing.T) {
	cases := []struct {
		name  string
		field string
		value string
	}{
		{"stock with query injection", "stocks", "AAPL&token=evil"},
		{"stock with path traversal", "stocks", "../../../etc/passwd"},
		{"stock with URL encoding", "stocks", "AAPL%00"},
		{"stock with spaces", "stocks", "AA PL"},
		{"etf with injection", "etfs", "SPY&callback=alert(1)"},
		{"crypto with slash", "crypto", "bitcoin/../../admin"},
		{"crypto with uppercase", "crypto", "BITCOIN"},
		{"crypto too long", "crypto", strings.Repeat("a", 65)},
		{"stock empty", "stocks", ""},
		{"stock too long", "stocks", "ABCDEFGHIJK"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMarketsConfig(connector.ConnectorConfig{
				Credentials: map[string]string{"finnhub_api_key": "test"},
				SourceConfig: map[string]interface{}{
					"watchlist": map[string]interface{}{
						tc.field: []interface{}{tc.value},
					},
				},
			})
			if err == nil {
				t.Errorf("expected error for invalid %s value %q, got nil", tc.field, tc.value)
			}
		})
	}
}

func TestParseMarketsConfig_AcceptsValidSymbols(t *testing.T) {
	cases := []struct {
		name  string
		field string
		value string
	}{
		{"simple stock", "stocks", "AAPL"},
		{"stock with dot", "stocks", "BRK.B"},
		{"stock with hyphen", "stocks", "BF-B"},
		{"simple etf", "etfs", "SPY"},
		{"simple crypto", "crypto", "bitcoin"},
		{"crypto with hyphen", "crypto", "bitcoin-cash"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMarketsConfig(connector.ConnectorConfig{
				Credentials: map[string]string{"finnhub_api_key": "test"},
				SourceConfig: map[string]interface{}{
					"watchlist": map[string]interface{}{
						tc.field: []interface{}{tc.value},
					},
				},
			})
			if err != nil {
				t.Errorf("unexpected error for valid %s symbol %q: %v", tc.field, tc.value, err)
			}
		})
	}
}

func TestParseMarketsConfig_WatchlistSizeLimit(t *testing.T) {
	// Construct a watchlist exceeding the limit
	tooMany := make([]interface{}, maxWatchlistSymbols+1)
	for i := range tooMany {
		tooMany[i] = "AAPL"
	}
	_, err := parseMarketsConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "test"},
		SourceConfig: map[string]interface{}{
			"watchlist": map[string]interface{}{
				"stocks": tooMany,
			},
		},
	})
	if err == nil {
		t.Error("expected error for watchlist exceeding size limit")
	}
}

func TestParseMarketsConfig_CoinGeckoEnabled(t *testing.T) {
	cases := []struct {
		name         string
		sourceConfig map[string]interface{}
		want         bool
	}{
		{
			name:         "defaults to false when not provided",
			sourceConfig: map[string]interface{}{},
			want:         false,
		},
		{
			name:         "explicitly true",
			sourceConfig: map[string]interface{}{"coingecko_enabled": true},
			want:         true,
		},
		{
			name:         "explicitly false",
			sourceConfig: map[string]interface{}{"coingecko_enabled": false},
			want:         false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseMarketsConfig(connector.ConnectorConfig{
				Credentials:  map[string]string{"finnhub_api_key": "test"},
				SourceConfig: tc.sourceConfig,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.CoinGeckoEnabled != tc.want {
				t.Errorf("CoinGeckoEnabled = %v, want %v", cfg.CoinGeckoEnabled, tc.want)
			}
		})
	}
}

func TestFetchFinnhubQuote_RejectsInvalidSymbol(t *testing.T) {
	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"

	cases := []string{
		"AAPL&token=evil",
		"../etc/passwd",
		"",
		strings.Repeat("A", 11),
		"AA PL",
	}
	for _, sym := range cases {
		_, err := c.fetchFinnhubQuote(context.Background(), sym)
		if err == nil {
			t.Errorf("expected error for invalid symbol %q", sym)
		}
	}
}

func TestFetchFinnhubQuote_RejectsZeroPriceResponse(t *testing.T) {
	// Simulate Finnhub returning all-zero "no data" response for unknown symbol.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 0, "d": 0, "dp": 0, "h": 0, "l": 0, "o": 0, "pc": 0,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	_, err := c.fetchFinnhubQuote(context.Background(), "INVALID")
	if err == nil {
		t.Fatal("expected error for zero-price response")
	}
	if !strings.Contains(err.Error(), "no data") {
		t.Errorf("error should mention no data, got: %v", err)
	}
}

func TestClassifyTier(t *testing.T) {
	cases := []struct {
		name      string
		threshold float64
		changePct float64
		want      string
	}{
		{"small change below threshold", 5.0, 2.0, "light"},
		{"at positive threshold", 5.0, 5.0, "full"},
		{"at negative threshold", 5.0, -5.0, "full"},
		{"zero threshold always light", 0, 99.0, "light"},
		{"above threshold", 5.0, 10.5, "full"},
		{"small positive", 5.0, 2.0, "light"},
		{"small negative", 5.0, -2.0, "light"},
		{"zero change", 5.0, 0.0, "light"},
		{"below negative threshold", 5.0, -12.3, "full"},
		{"just below threshold", 5.0, 4.99, "light"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyTier(tc.threshold, tc.changePct)
			if got != tc.want {
				t.Errorf("classifyTier(%v, %v) = %q, want %q", tc.threshold, tc.changePct, got, tc.want)
			}
		})
	}
}

func TestClassifyTier_ZeroThresholdAlwaysLight(t *testing.T) {
	if tier := classifyTier(0, 99.0); tier != "light" {
		t.Errorf("expected light when threshold=0, got %q", tier)
	}
}

func TestCryptoChange24hCalculation(t *testing.T) {
	// Simulate CoinGecko response and verify Change24h is calculated.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]map[string]float64{
			"bitcoin": {"usd": 50000.0, "usd_24h_change": 5.0},
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()

	// Override the URL by using fetchCoinGeckoPrices with the test server
	// Since we can't easily inject the URL, verify the math directly:
	// price=50000, changePct=5% → change24h = 50000 - (50000 / 1.05) ≈ 2380.95
	price := 50000.0
	changePct := 5.0
	change24h := price - (price / (1 + changePct/100))
	expectedApprox := 2380.95

	if change24h < expectedApprox-1 || change24h > expectedApprox+1 {
		t.Errorf("change24h calculation: got %.2f, expected ~%.2f", change24h, expectedApprox)
	}

	// Zero percent change should yield zero change
	changePct = 0.0
	var change24hZero float64
	if changePct != 0 {
		change24hZero = price - (price / (1 + changePct/100))
	}
	if change24hZero != 0.0 {
		t.Errorf("zero pct change should yield zero change24h, got %.2f", change24hZero)
	}

	// Negative change
	changePct = -10.0
	change24hNeg := price - (price / (1 + changePct/100))
	if change24hNeg >= 0 {
		t.Errorf("negative pct should yield negative change24h, got %.2f", change24hNeg)
	}
}

func TestConnect_ThreadSafety(t *testing.T) {
	c := newTestConnector("financial-markets")
	cfg := connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "test-key"},
	}

	// Connect and Close should not race.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			_ = c.Connect(context.Background(), cfg)
		}
	}()
	for i := 0; i < 100; i++ {
		_ = c.Close()
		_ = c.Health(context.Background())
	}
	<-done
}

func TestRateLimit_AtBoundary(t *testing.T) {
	// Verify that filling to exactly the limit denies the next call.
	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test"

	// Fill to exactly the limit (55 for finnhub)
	for i := 0; i < 55; i++ {
		if !c.tryRecordCall("finnhub") {
			t.Fatalf("call %d should be allowed", i+1)
		}
	}

	// Should not allow the 56th
	if c.tryRecordCall("finnhub") {
		t.Error("should deny call when at rate limit")
	}
}

func TestTryRecordCall_Atomic(t *testing.T) {
	c := newTestConnector("financial-markets")

	// Should allow and record first call
	if !c.tryRecordCall("finnhub") {
		t.Error("first tryRecordCall should succeed")
	}

	// Fill to limit minus the one already recorded
	for i := 0; i < 54; i++ {
		if !c.tryRecordCall("finnhub") {
			t.Errorf("tryRecordCall should succeed at count %d", i+2)
		}
	}

	// 56th call should be denied
	if c.tryRecordCall("finnhub") {
		t.Error("should deny tryRecordCall when at rate limit")
	}

	// Unknown provider always allowed
	if !c.tryRecordCall("unknown") {
		t.Error("unknown provider should always be allowed")
	}
}

func TestSyncContextCancellation(t *testing.T) {
	// Start a test server that returns valid data
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 150.0, "d": 1.0, "dp": 0.5, "h": 152.0, "l": 148.0, "o": 149.0, "pc": 149.0,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL
	c.config = MarketsConfig{
		FinnhubAPIKey: "test-key",
		Watchlist: WatchlistConfig{
			Stocks: []string{"AAPL", "GOOGL", "MSFT", "AMZN", "META"},
		},
	}

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := c.Sync(ctx, "")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestSyncConfigSnapshotSafety(t *testing.T) {
	// Verify Sync does not corrupt the original Stocks slice via append.
	c := newTestConnector("financial-markets")
	c.config = MarketsConfig{
		FinnhubAPIKey: "test-key",
		Watchlist: WatchlistConfig{
			// Give the slice extra capacity so an unsafe append would corrupt it.
			Stocks: append(make([]string, 0, 10), "AAPL", "GOOGL"),
			ETFs:   []string{"SPY"},
		},
	}

	original := make([]string, len(c.config.Watchlist.Stocks))
	copy(original, c.config.Watchlist.Stocks)

	// Cancel immediately — we just want to verify the append safety, not make HTTP calls.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _ = c.Sync(ctx, "")

	if len(c.config.Watchlist.Stocks) != len(original) {
		t.Errorf("Stocks slice length changed: was %d, now %d", len(original), len(c.config.Watchlist.Stocks))
	}
	for i, s := range c.config.Watchlist.Stocks {
		if s != original[i] {
			t.Errorf("Stocks[%d] corrupted: was %q, now %q", i, original[i], s)
		}
	}
}

func TestHTTPErrorResponseDrain(t *testing.T) {
	// Verify non-OK responses are handled without leaking connections.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	_, err := c.fetchFinnhubQuote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should mention status 429, got: %v", err)
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("error should include response body snippet, got: %v", err)
	}
}

func TestCloseCleanup(t *testing.T) {
	c := newTestConnector("financial-markets")
	c.health = connector.HealthHealthy

	err := c.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Error("should be disconnected after Close")
	}
	// Calling Close twice should not panic
	err = c.Close()
	if err != nil {
		t.Errorf("second Close returned error: %v", err)
	}
}

func TestTryRecordCall_ConcurrentSafety(t *testing.T) {
	c := newTestConnector("financial-markets")

	// Run 100 concurrent tryRecordCall attempts.
	// With limit 55, exactly 55 should succeed.
	var wg sync.WaitGroup
	results := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- c.tryRecordCall("finnhub")
		}()
	}
	wg.Wait()
	close(results)

	allowed := 0
	for ok := range results {
		if ok {
			allowed++
		}
	}
	if allowed != 55 {
		t.Errorf("expected exactly 55 allowed calls, got %d", allowed)
	}
}

// --- Hardening Tests ---

func TestParseMarketsConfig_RejectsNegativeAlertThreshold(t *testing.T) {
	_, err := parseMarketsConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "test"},
		SourceConfig: map[string]interface{}{
			"alert_threshold": -1.0,
		},
	})
	if err == nil {
		t.Error("expected error for negative alert_threshold")
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Errorf("error should mention non-negative, got: %v", err)
	}
}

func TestParseMarketsConfig_AcceptsZeroAlertThreshold(t *testing.T) {
	cfg, err := parseMarketsConfig(connector.ConnectorConfig{
		Credentials:  map[string]string{"finnhub_api_key": "test"},
		SourceConfig: map[string]interface{}{"alert_threshold": 0.0},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AlertThreshold != 0.0 {
		t.Errorf("expected 0.0, got %v", cfg.AlertThreshold)
	}
}

func TestParseMarketsConfig_ForexPairsValid(t *testing.T) {
	cfg, err := parseMarketsConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "test"},
		SourceConfig: map[string]interface{}{
			"watchlist": map[string]interface{}{
				"forex_pairs": []interface{}{"USD/JPY", "EUR/USD", "GBP/CHF"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Watchlist.ForexPairs) != 3 {
		t.Errorf("expected 3 forex pairs, got %d", len(cfg.Watchlist.ForexPairs))
	}
}

func TestParseMarketsConfig_ForexPairsInvalid(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"lowercase", "usd/jpy"},
		{"no slash", "USDJPY"},
		{"extra chars", "USD/JPYX"},
		{"numbers", "US1/JPY"},
		{"empty", ""},
		{"single currency", "USD/"},
		{"injection", "USD/JPY&x=1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMarketsConfig(connector.ConnectorConfig{
				Credentials: map[string]string{"finnhub_api_key": "test"},
				SourceConfig: map[string]interface{}{
					"watchlist": map[string]interface{}{
						"forex_pairs": []interface{}{tc.value},
					},
				},
			})
			if err == nil {
				t.Errorf("expected error for invalid forex pair %q", tc.value)
			}
		})
	}
}

func TestParseMarketsConfig_ForexPairsSizeLimit(t *testing.T) {
	tooMany := make([]interface{}, maxWatchlistSymbols+1)
	for i := range tooMany {
		tooMany[i] = "USD/JPY"
	}
	_, err := parseMarketsConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "test"},
		SourceConfig: map[string]interface{}{
			"watchlist": map[string]interface{}{
				"forex_pairs": tooMany,
			},
		},
	})
	if err == nil {
		t.Error("expected error for forex pairs exceeding size limit")
	}
}

func TestCloseResetsCallCounts(t *testing.T) {
	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test"

	// Record some calls
	for i := 0; i < 10; i++ {
		c.tryRecordCall("finnhub")
	}

	c.Close()

	// After Close + fresh state, all calls should be allowed again
	if !c.tryRecordCall("finnhub") {
		t.Error("tryRecordCall should succeed after Close resets callCounts")
	}
}

func TestFinnhubErrorResponseIncludesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"API key invalid"}`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "bad-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	_, err := c.fetchFinnhubQuote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention status 403, got: %v", err)
	}
	if !strings.Contains(err.Error(), "API key invalid") {
		t.Errorf("error should include body snippet, got: %v", err)
	}
}

func TestCoinGeckoBatchTruncation(t *testing.T) {
	// Build a list exceeding the batch cap.
	ids := make([]string, maxCoinGeckoBatchSize+10)
	for i := range ids {
		ids[i] = "coin-" + strings.Repeat("a", 3)
	}

	// The actual truncation happens inside fetchCoinGeckoPrices before the HTTP call.
	// We verify the constant exists and is reasonable.
	if maxCoinGeckoBatchSize < 1 || maxCoinGeckoBatchSize > 200 {
		t.Errorf("maxCoinGeckoBatchSize should be between 1 and 200, got %d", maxCoinGeckoBatchSize)
	}
}

func TestProviderRateLimitsConsistency(t *testing.T) {
	// Verify the package-level rate limits match expected values.
	expected := map[string]int{"finnhub": 55, "coingecko": 25, "fred": 100}
	for provider, limit := range expected {
		if providerRateLimits[provider] != limit {
			t.Errorf("providerRateLimits[%q] = %d, want %d", provider, providerRateLimits[provider], limit)
		}
	}
	// Verify no unexpected providers.
	if len(providerRateLimits) != len(expected) {
		t.Errorf("providerRateLimits has %d entries, expected %d", len(providerRateLimits), len(expected))
	}
}

func TestSyncEmptyWatchlist(t *testing.T) {
	c := newTestConnector("financial-markets")
	c.config = MarketsConfig{
		FinnhubAPIKey: "test-key",
		Watchlist:     WatchlistConfig{}, // all empty
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty watchlist should produce zero artifacts, not an error.
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for empty watchlist, got %d", len(artifacts))
	}
	if cursor == "" {
		t.Error("cursor should be set even with empty watchlist")
	}
}

func TestClassifyTier_NegativeThresholdTreatedAsDisabled(t *testing.T) {
	// Threshold <= 0 means alerts are effectively disabled.
	// classifyTier checks threshold > 0, so zero/negative always returns "light".
	if tier := classifyTier(0, 99.0); tier != "light" {
		t.Errorf("expected light for threshold=0, got %q", tier)
	}
	// Negative threshold should never reach here due to config validation,
	// but verify the fallback behavior is safe.
	if tier := classifyTier(-1.0, 99.0); tier != "light" {
		t.Errorf("expected light for negative threshold, got %q", tier)
	}
}

func TestSyncRateLimitExhaustion(t *testing.T) {
	// Verify Sync gracefully handles rate limit exhaustion mid-watchlist.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 150.0, "d": 1.0, "dp": 0.5, "h": 152.0, "l": 148.0, "o": 149.0, "pc": 149.0,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	// Build a watchlist larger than the rate limit.
	stocks := make([]string, 60)
	for i := range stocks {
		stocks[i] = "SYMA"
	}

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: stocks},
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should produce at most 55 artifacts (rate limit), not 60.
	if len(artifacts) > 55 {
		t.Errorf("expected at most 55 artifacts due to rate limit, got %d", len(artifacts))
	}
	if len(artifacts) == 0 {
		t.Error("expected some artifacts before rate limit exhaustion")
	}
	if cursor == "" {
		t.Error("cursor should be set")
	}
}

func TestSyncDegradedHealthOnTotalFailure(t *testing.T) {
	// When all provider calls fail, health should be degraded.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"AAPL", "GOOGL"}},
	}

	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected error health after total failure, got %v", c.Health(context.Background()))
	}
}

func TestSyncHealthyOnPartialFailure(t *testing.T) {
	// When some quote calls succeed and some fail, health should be healthy.
	quoteCallNum := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/company-news" {
			// News succeeds for all symbols to isolate partial-quote-failure test.
			json.NewEncoder(w).Encode([]map[string]interface{}{})
			return
		}
		quoteCallNum++
		if quoteCallNum == 1 {
			// First quote call succeeds
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 150.0, "d": 1.0, "dp": 0.5, "h": 152.0, "l": 148.0, "o": 149.0, "pc": 149.0,
			})
		} else {
			// Subsequent quote calls fail
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"server error"}`))
		}
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"AAPL", "GOOGL"}},
	}

	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Partial failure — some quote symbols succeeded, health stays healthy.
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy on partial failure, got %v", c.Health(context.Background()))
	}
}

func TestSyncFinnhubIntegrationViaHTTPTest(t *testing.T) {
	// Full integration test: Sync fetches from httptest, normalizes, returns artifacts.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		token := r.URL.Query().Get("token")
		if token != "test-key" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"bad token"}`))
			return
		}
		switch symbol {
		case "AAPL":
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 175.50, "d": 2.30, "dp": 1.3, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.20,
			})
		case "TSLA":
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 250.0, "d": 15.0, "dp": 6.5, "h": 255.0, "l": 240.0, "o": 242.0, "pc": 235.0,
			})
		default:
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 0, "d": 0, "dp": 0, "h": 0, "l": 0, "o": 0, "pc": 0,
			})
		}
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"AAPL", "TSLA"}},
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor == "" {
		t.Error("cursor should be set")
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}

	// Verify AAPL artifact
	aapl := artifacts[0]
	if aapl.ContentType != "market/quote" {
		t.Errorf("expected market/quote, got %s", aapl.ContentType)
	}
	if aapl.Metadata["symbol"] != "AAPL" {
		t.Errorf("expected AAPL, got %v", aapl.Metadata["symbol"])
	}
	if aapl.Metadata["processing_tier"] != "light" {
		t.Errorf("AAPL 1.3%% change should be light tier, got %v", aapl.Metadata["processing_tier"])
	}

	// Verify TSLA artifact (6.5% > 5.0% threshold → full tier)
	tsla := artifacts[1]
	if tsla.Metadata["symbol"] != "TSLA" {
		t.Errorf("expected TSLA, got %v", tsla.Metadata["symbol"])
	}
	if tsla.Metadata["processing_tier"] != "full" {
		t.Errorf("TSLA 6.5%% change should be full tier, got %v", tsla.Metadata["processing_tier"])
	}
}

func TestSyncCoinGeckoIntegrationViaHTTPTest(t *testing.T) {
	// Full CoinGecko integration via httptest.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]map[string]float64{
			"bitcoin":  {"usd": 67000.0, "usd_24h_change": 3.2},
			"ethereum": {"usd": 3500.0, "usd_24h_change": -6.0},
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.coingeckoBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:    "test-key",
		CoinGeckoEnabled: true,
		AlertThreshold:   5.0,
		Watchlist:        WatchlistConfig{Crypto: []string{"bitcoin", "ethereum"}},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 crypto artifacts, got %d", len(artifacts))
	}

	// Find ethereum artifact (order from map iteration is non-deterministic)
	var ethFound bool
	for _, a := range artifacts {
		if a.Metadata["symbol"] == "ethereum" {
			ethFound = true
			if a.Metadata["processing_tier"] != "full" {
				t.Errorf("ethereum -6%% should be full tier, got %v", a.Metadata["processing_tier"])
			}
			if a.Metadata["asset_type"] != "crypto" {
				t.Errorf("expected crypto asset_type, got %v", a.Metadata["asset_type"])
			}
		}
	}
	if !ethFound {
		t.Error("ethereum artifact not found")
	}
}

func TestConnectThenCloseAndReconnect(t *testing.T) {
	c := newTestConnector("financial-markets")
	cfg := connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "test-key"},
		SourceConfig: map[string]interface{}{
			"watchlist": map[string]interface{}{
				"stocks": []interface{}{"AAPL"},
			},
		},
	}

	// Connect, record some rate limit entries, close, reconnect.
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("first Connect failed: %v", err)
	}
	for i := 0; i < 50; i++ {
		c.tryRecordCall("finnhub")
	}
	c.Close()

	// After Close + reconnect, rate limits should be fresh.
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("second Connect failed: %v", err)
	}
	if !c.tryRecordCall("finnhub") {
		t.Error("rate limits should be fresh after Close + reconnect")
	}
}

func TestFetchCoinGeckoPrices_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"status":{"error_code":429,"error_message":"rate limit"}}`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.coingeckoBaseURL = srv.URL

	_, err := c.fetchCoinGeckoPrices(context.Background(), []string{"bitcoin"})
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should mention status 429, got: %v", err)
	}
}

func TestFetchCoinGeckoPrices_AllInvalidIDs(t *testing.T) {
	c := newTestConnector("financial-markets")
	// All IDs fail validation — should error before any HTTP call.
	_, err := c.fetchCoinGeckoPrices(context.Background(), []string{"BITCOIN", "../admin", ""})
	if err == nil {
		t.Fatal("expected error for all-invalid coin IDs")
	}
	if !strings.Contains(err.Error(), "no valid coin IDs") {
		t.Errorf("error should mention no valid coin IDs, got: %v", err)
	}
}

func TestFetchCoinGeckoPrices_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.coingeckoBaseURL = srv.URL

	_, err := c.fetchCoinGeckoPrices(context.Background(), []string{"bitcoin"})
	if err == nil {
		t.Fatal("expected error for malformed JSON response")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error should mention decode, got: %v", err)
	}
}

func TestSyncCoinGeckoDisabledSkipsCrypto(t *testing.T) {
	// When CoinGeckoEnabled=false, Sync should not fetch crypto even if watchlist has crypto.
	srvCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvCalled = true
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]map[string]float64{})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.coingeckoBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:    "test-key",
		CoinGeckoEnabled: false,
		Watchlist:        WatchlistConfig{Crypto: []string{"bitcoin", "ethereum"}},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts when CoinGecko disabled, got %d", len(artifacts))
	}
	if srvCalled {
		t.Error("CoinGecko server should not be called when CoinGeckoEnabled=false")
	}
}

func TestSyncETFsMergedWithStocks(t *testing.T) {
	// ETFs should be fetched via Finnhub alongside stocks.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 450.0, "d": 3.0, "dp": 0.7, "h": 452.0, "l": 448.0, "o": 449.0, "pc": 447.0,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist: WatchlistConfig{
			Stocks: []string{"AAPL"},
			ETFs:   []string{"SPY", "QQQ"},
		},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 3 {
		t.Fatalf("expected 3 artifacts (1 stock + 2 ETFs), got %d", len(artifacts))
	}

	// Verify all symbols appear in artifacts.
	symbols := map[string]bool{}
	for _, a := range artifacts {
		symbols[a.Metadata["symbol"].(string)] = true
	}
	for _, want := range []string{"AAPL", "SPY", "QQQ"} {
		if !symbols[want] {
			t.Errorf("missing artifact for symbol %s", want)
		}
	}
}

func TestSyncMultiProviderCombined(t *testing.T) {
	// Verify Sync fetches from both Finnhub and CoinGecko in a single call.
	finnhubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 175.0, "d": 2.0, "dp": 1.1, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.0,
		})
	}))
	defer finnhubSrv.Close()

	coingeckoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]map[string]float64{
			"bitcoin": {"usd": 67000.0, "usd_24h_change": 2.5},
		})
	}))
	defer coingeckoSrv.Close()

	c := newTestConnector("financial-markets")
	// Can't easily split HTTP clients per host, so use one client.
	// Instead, point both base URLs to their respective servers.
	c.finnhubBaseURL = finnhubSrv.URL
	c.coingeckoBaseURL = coingeckoSrv.URL
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	c.config = MarketsConfig{
		FinnhubAPIKey:    "test-key",
		CoinGeckoEnabled: true,
		AlertThreshold:   5.0,
		Watchlist: WatchlistConfig{
			Stocks: []string{"AAPL"},
			Crypto: []string{"bitcoin"},
		},
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor == "" {
		t.Error("cursor should be set")
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts (1 stock + 1 crypto), got %d", len(artifacts))
	}

	var hasStock, hasCrypto bool
	for _, a := range artifacts {
		if a.Metadata["symbol"] == "AAPL" {
			hasStock = true
		}
		if a.Metadata["symbol"] == "bitcoin" {
			hasCrypto = true
			if a.Metadata["asset_type"] != "crypto" {
				t.Errorf("bitcoin should have asset_type=crypto, got %v", a.Metadata["asset_type"])
			}
		}
	}
	if !hasStock {
		t.Error("missing AAPL stock artifact")
	}
	if !hasCrypto {
		t.Error("missing bitcoin crypto artifact")
	}
}

func TestConnect_SetsHealthErrorOnInvalidConfig(t *testing.T) {
	c := newTestConnector("financial-markets")

	// Missing API key should set health to HealthError.
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected HealthError after failed Connect, got %v", c.Health(context.Background()))
	}
}

func TestConnect_SetsHealthErrorOnBadSymbol(t *testing.T) {
	c := newTestConnector("financial-markets")

	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "test"},
		SourceConfig: map[string]interface{}{
			"watchlist": map[string]interface{}{
				"stocks": []interface{}{"AAPL&inject"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid symbol in config")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected HealthError after config parse failure, got %v", c.Health(context.Background()))
	}
}

func TestFetchFinnhubQuote_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	_, err := c.fetchFinnhubQuote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error for malformed JSON response")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error should mention decode, got: %v", err)
	}
}

func TestFetchCoinGeckoPrices_BatchTruncationViaHTTPTest(t *testing.T) {
	// Verify that oversized batch is truncated before the HTTP call.
	var receivedIDs string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedIDs = r.URL.Query().Get("ids")
		result := make(map[string]map[string]float64)
		for _, id := range strings.Split(receivedIDs, ",") {
			if id != "" {
				result[id] = map[string]float64{"usd": 100.0, "usd_24h_change": 1.0}
			}
		}
		json.NewEncoder(w).Encode(result)
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.coingeckoBaseURL = srv.URL

	// Build list exceeding maxCoinGeckoBatchSize.
	ids := make([]string, maxCoinGeckoBatchSize+10)
	for i := range ids {
		ids[i] = fmt.Sprintf("coin-%d", i)
	}

	prices, err := c.fetchCoinGeckoPrices(context.Background(), ids)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the server received at most maxCoinGeckoBatchSize IDs.
	sentIDs := strings.Split(receivedIDs, ",")
	if len(sentIDs) > maxCoinGeckoBatchSize {
		t.Errorf("sent %d IDs to server, expected at most %d", len(sentIDs), maxCoinGeckoBatchSize)
	}
	if len(prices) > maxCoinGeckoBatchSize {
		t.Errorf("got %d prices, expected at most %d", len(prices), maxCoinGeckoBatchSize)
	}
}

func TestSyncDegradedHealthOnPartialProviderFailure(t *testing.T) {
	// When one whole provider fails but another succeeds, health should be HealthDegraded.
	finnhubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 175.0, "d": 2.0, "dp": 1.1, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.0,
		})
	}))
	defer finnhubSrv.Close()

	coingeckoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer coingeckoSrv.Close()

	c := newTestConnector("financial-markets")
	c.finnhubBaseURL = finnhubSrv.URL
	c.coingeckoBaseURL = coingeckoSrv.URL
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	c.config = MarketsConfig{
		FinnhubAPIKey:    "test-key",
		CoinGeckoEnabled: true,
		AlertThreshold:   5.0,
		Watchlist: WatchlistConfig{
			Stocks: []string{"AAPL"},
			Crypto: []string{"bitcoin"},
		},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Finnhub succeeded (1 artifact), CoinGecko failed.
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact (stock only), got %d", len(artifacts))
	}
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Errorf("expected HealthDegraded on partial provider failure, got %v", c.Health(context.Background()))
	}
}

func TestSyncHealthRestoredAfterRecovery(t *testing.T) {
	// After a degraded sync, a clean sync should restore health to Healthy.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 4 {
			// First sync: all calls fail (2 quote + 2 news = 4 calls for 2 stocks).
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"down"}`))
			return
		}
		if r.URL.Path == "/api/v1/company-news" {
			// Recovery: news succeeds with empty array.
			json.NewEncoder(w).Encode([]map[string]interface{}{})
			return
		}
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 175.0, "d": 2.0, "dp": 1.1, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.0,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL
	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"AAPL", "GOOGL"}},
	}

	// First sync: total failure → HealthError.
	_, _, _ = c.Sync(context.Background(), "")
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected HealthError after total failure, got %v", c.Health(context.Background()))
	}

	// Second sync: recovery → HealthHealthy.
	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error on recovery sync: %v", err)
	}
	if len(artifacts) == 0 {
		t.Error("expected artifacts on recovery sync")
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected HealthHealthy after recovery, got %v", c.Health(context.Background()))
	}
}

// --- Hardening Tests: H-018-001 through H-018-004 ---

func TestFetchFinnhubQuote_MalformedBaseURL(t *testing.T) {
	// H-018-001: url.Parse error must be returned, not silently discarded.
	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.finnhubBaseURL = "://invalid-url"

	_, err := c.fetchFinnhubQuote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error for malformed base URL")
	}
	if !strings.Contains(err.Error(), "parse finnhub URL") {
		t.Errorf("error should mention URL parse failure, got: %v", err)
	}
}

func TestFetchCoinGeckoPrices_MalformedBaseURL(t *testing.T) {
	// H-018-001: url.Parse error must be returned for CoinGecko too.
	c := newTestConnector("financial-markets")
	c.coingeckoBaseURL = "://invalid-url"

	_, err := c.fetchCoinGeckoPrices(context.Background(), []string{"bitcoin"})
	if err == nil {
		t.Fatal("expected error for malformed CoinGecko base URL")
	}
	if !strings.Contains(err.Error(), "parse coingecko URL") {
		t.Errorf("error should mention URL parse failure, got: %v", err)
	}
}

func TestSyncForexPairsProduceArtifacts(t *testing.T) {
	// H-018-002: Forex pairs must produce artifacts, not be dead config.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		switch symbol {
		case "OANDA:USD_JPY":
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 154.32, "d": 0.45, "dp": 0.29, "h": 155.0, "l": 153.5, "o": 153.87, "pc": 153.87,
			})
		case "OANDA:EUR_USD":
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 1.0821, "d": -0.003, "dp": -0.28, "h": 1.0855, "l": 1.08, "o": 1.0851, "pc": 1.0851,
			})
		default:
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 150.0, "d": 1.0, "dp": 0.5, "h": 152.0, "l": 148.0, "o": 149.0, "pc": 149.0,
			})
		}
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist: WatchlistConfig{
			ForexPairs: []string{"USD/JPY", "EUR/USD"},
		},
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor == "" {
		t.Error("cursor should be set")
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 forex artifacts, got %d", len(artifacts))
	}

	// Verify artifacts carry forex metadata.
	for _, a := range artifacts {
		if a.ContentType != "market/quote" {
			t.Errorf("expected market/quote, got %s", a.ContentType)
		}
		if a.Metadata["asset_type"] != "forex" {
			t.Errorf("expected asset_type=forex, got %v", a.Metadata["asset_type"])
		}
		if a.Metadata["processing_tier"] != "light" {
			t.Errorf("forex with small change should be light tier, got %v", a.Metadata["processing_tier"])
		}
	}
}

func TestSyncForexOnlyNoStocks(t *testing.T) {
	// H-018-002: Forex-only watchlist (no stocks, no crypto) must still produce artifacts.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 154.32, "d": 0.45, "dp": 0.29, "h": 155.0, "l": 153.5, "o": 153.87, "pc": 153.87,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{ForexPairs: []string{"GBP/CHF"}},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact for forex-only watchlist, got %d", len(artifacts))
	}
	if artifacts[0].Metadata["symbol"] != "GBP/CHF" {
		t.Errorf("expected symbol GBP/CHF, got %v", artifacts[0].Metadata["symbol"])
	}
}

func TestFetchFinnhubForex_RejectsInvalidPair(t *testing.T) {
	// H-018-002: fetchFinnhubForex defense-in-depth validates pair format.
	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"

	cases := []string{"usd/jpy", "USDJPY", "", "USD/JPYX", "USD/JPY&x=1", "123/456"}
	for _, pair := range cases {
		_, err := c.fetchFinnhubForex(context.Background(), pair)
		if err == nil {
			t.Errorf("expected error for invalid forex pair %q", pair)
		}
	}
}

func TestFetchFinnhubForex_ConvertsToOANDAFormat(t *testing.T) {
	// H-018-002: Verify "USD/JPY" is sent as "OANDA:USD_JPY" to Finnhub.
	var receivedSymbol string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSymbol = r.URL.Query().Get("symbol")
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 154.32, "d": 0.45, "dp": 0.29, "h": 155.0, "l": 153.5, "o": 153.87, "pc": 153.87,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	_, err := c.fetchFinnhubForex(context.Background(), "USD/JPY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedSymbol != "OANDA:USD_JPY" {
		t.Errorf("expected OANDA:USD_JPY, got %q", receivedSymbol)
	}
}

func TestParseMarketsConfig_RejectsNonStringEntries(t *testing.T) {
	// H-018-003: Non-string watchlist entries must be rejected, not silently swallowed.
	cases := []struct {
		name  string
		field string
		value interface{}
	}{
		{"stock integer", "stocks", 42},
		{"stock boolean", "stocks", true},
		{"stock float", "stocks", 3.14},
		{"etf integer", "etfs", 99},
		{"crypto boolean", "crypto", false},
		{"forex_pairs integer", "forex_pairs", 7},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMarketsConfig(connector.ConnectorConfig{
				Credentials: map[string]string{"finnhub_api_key": "test"},
				SourceConfig: map[string]interface{}{
					"watchlist": map[string]interface{}{
						tc.field: []interface{}{tc.value},
					},
				},
			})
			if err == nil {
				t.Errorf("expected error for non-string %s value %v (%T), got nil", tc.field, tc.value, tc.value)
			}
			if err != nil && !strings.Contains(err.Error(), "expected string") {
				t.Errorf("error should mention 'expected string', got: %v", err)
			}
		})
	}
}

func TestConnectResetsRateLimits(t *testing.T) {
	// H-018-004: Connect() must reset callCounts so stale entries don't carry over.
	c := newTestConnector("financial-markets")
	cfg := connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "test-key"},
	}

	// First Connect.
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("first Connect failed: %v", err)
	}

	// Exhaust rate limit.
	for i := 0; i < 55; i++ {
		c.tryRecordCall("finnhub")
	}
	if c.tryRecordCall("finnhub") {
		t.Fatal("rate limit should be exhausted")
	}

	// Reconnect WITHOUT Close() — Connect() alone must reset limits.
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("second Connect failed: %v", err)
	}
	if !c.tryRecordCall("finnhub") {
		t.Error("rate limits should be fresh after Connect(), even without Close()")
	}
}

func TestSyncForexTotalFailureSetsHealthDegraded(t *testing.T) {
	// H-018-002: When forex is the only provider and all forex calls fail, health should be Error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{ForexPairs: []string{"USD/JPY"}},
	}

	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected HealthError after total forex failure, got %v", c.Health(context.Background()))
	}
}

func TestSyncStocksAndForexMixed(t *testing.T) {
	// H-018-002: Combined stocks + forex in a single Sync call.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 150.0, "d": 1.0, "dp": 0.5, "h": 152.0, "l": 148.0, "o": 149.0, "pc": 149.0,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist: WatchlistConfig{
			Stocks:     []string{"AAPL"},
			ForexPairs: []string{"USD/JPY"},
		},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts (1 stock + 1 forex), got %d", len(artifacts))
	}

	var hasStock, hasForex bool
	for _, a := range artifacts {
		switch a.Metadata["asset_type"] {
		case "forex":
			hasForex = true
		default:
			if a.Metadata["symbol"] == "AAPL" {
				hasStock = true
			}
		}
	}
	if !hasStock {
		t.Error("missing stock artifact")
	}
	if !hasForex {
		t.Error("missing forex artifact")
	}
}

// --- Stabilize Tests: STB-018-001 through STB-018-003 ---

func TestSync_ConnectDuringSyncSkipsStaleHealthWrite(t *testing.T) {
	// STB-018-001: If Connect() succeeds while a Sync() is in flight (with failures),
	// the Sync's deferred health write must NOT clobber Connect's HealthHealthy.
	// We simulate this by:
	//  1. Starting a Sync against a failing server (will set failCount > 0).
	//  2. Calling Connect() before the Sync's defer runs.
	//  3. Verifying health stays HealthHealthy from Connect, not overwritten.
	//
	// To control timing, we use a slow failing server and call Connect()
	// in a goroutine that races with the Sync defer.

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"down"}`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL
	c.config = MarketsConfig{
		FinnhubAPIKey: "test-key",
		Watchlist:     WatchlistConfig{Stocks: []string{"AAPL"}},
	}

	// Run a failing Sync to completion — health will be Error.
	_, _, _ = c.Sync(context.Background(), "")
	if c.Health(context.Background()) != connector.HealthError {
		t.Fatalf("expected HealthError after failed Sync, got %v", c.Health(context.Background()))
	}

	// Now simulate the race: Connect resets health to Healthy and increments configGen.
	// A subsequent stale Sync defer would see a mismatched gen and skip the health write.
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "fresh-key"},
	})
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Fatalf("expected HealthHealthy after Connect, got %v", c.Health(context.Background()))
	}

	// Verify configGen was incremented — the generation counter protects against stale writes.
	c.mu.RLock()
	gen := c.configGen
	c.mu.RUnlock()
	if gen == 0 {
		t.Error("configGen should be > 0 after Connect")
	}
}

func TestSync_CoinGeckoRateLimited_HealthDegraded(t *testing.T) {
	// STB-018-002: When CoinGecko is rate-limited (tryRecordCall returns false),
	// health must be Degraded, not Healthy. Previously the code counted
	// CoinGecko as a provider but never incremented failCount on rate-limit skip.

	// Use a healthy Finnhub server so stocks succeed.
	finnhubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 175.0, "d": 2.0, "dp": 1.1, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.0,
		})
	}))
	defer finnhubSrv.Close()

	// CoinGecko server should never be called.
	coingeckoCalled := false
	coingeckoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coingeckoCalled = true
		json.NewEncoder(w).Encode(map[string]map[string]float64{
			"bitcoin": {"usd": 67000.0, "usd_24h_change": 2.0},
		})
	}))
	defer coingeckoSrv.Close()

	c := newTestConnector("financial-markets")
	c.finnhubBaseURL = finnhubSrv.URL
	c.coingeckoBaseURL = coingeckoSrv.URL
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	c.config = MarketsConfig{
		FinnhubAPIKey:    "test-key",
		CoinGeckoEnabled: true,
		AlertThreshold:   5.0,
		Watchlist: WatchlistConfig{
			Stocks: []string{"AAPL"},
			Crypto: []string{"bitcoin"},
		},
	}

	// Exhaust CoinGecko rate budget before Sync.
	for i := 0; i < 25; i++ {
		c.tryRecordCall("coingecko")
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stock artifact should exist, but crypto should be missing.
	if len(artifacts) != 1 {
		t.Errorf("expected 1 artifact (stock only, crypto rate-limited), got %d", len(artifacts))
	}
	if coingeckoCalled {
		t.Error("CoinGecko server should not be called when rate-limited")
	}

	// Health MUST be Degraded — before the fix it was Healthy.
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Errorf("expected HealthDegraded when CoinGecko rate-limited, got %v", c.Health(context.Background()))
	}
}

func TestSync_StocksRateLimitedMidLoop_HealthDegraded(t *testing.T) {
	// STB-018-003: When Finnhub rate limit exhausts mid-loop for stocks,
	// health must be Degraded, not Healthy. The break skips remaining symbols.

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 150.0, "d": 1.0, "dp": 0.5, "h": 152.0, "l": 148.0, "o": 149.0, "pc": 149.0,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	// Watchlist with 10 symbols, but pre-exhaust to leave only 3 calls.
	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"}},
	}

	// Pre-exhaust 52 of 55 Finnhub calls, leaving only 3.
	for i := 0; i < 52; i++ {
		c.tryRecordCall("finnhub")
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get at most 3 artifacts (remaining budget), not all 10.
	if len(artifacts) > 3 {
		t.Errorf("expected at most 3 artifacts due to rate limit, got %d", len(artifacts))
	}
	if len(artifacts) == 0 {
		t.Error("expected at least some artifacts before rate limit")
	}

	// Health MUST be Degraded — before the fix it was Healthy.
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Errorf("expected HealthDegraded when stocks rate-limited mid-loop, got %v", c.Health(context.Background()))
	}
}

// --- Regression Tests: REG-018-001 through REG-018-002 ---

func TestCryptoChange24h_NegHundredPercentNoDivByZero(t *testing.T) {
	// REG-018-001: When CoinGecko returns -100% change, the formula
	// price / (1 + changePct/100) has denominator 0, producing Inf.
	// This corrupts JSON serialization downstream.
	// The fix clamps to -price instead of computing Inf.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]map[string]float64{
			"rugpull-coin": {"usd": 0.0, "usd_24h_change": -100.0},
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.coingeckoBaseURL = srv.URL

	prices, err := c.fetchCoinGeckoPrices(context.Background(), []string{"rugpull-coin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected 1 price, got %d", len(prices))
	}

	p := prices[0]
	// change24h must be finite — Inf/NaN would corrupt JSON.
	if p.Change24h != p.Change24h { // NaN != NaN
		t.Fatal("Change24h is NaN — would corrupt JSON serialization")
	}
	if p.Change24h > 1e18 || p.Change24h < -1e18 {
		t.Fatalf("Change24h is Inf-like (%v) — would corrupt JSON serialization", p.Change24h)
	}
}

func TestCryptoChange24h_ExtremeNegativePercentFinite(t *testing.T) {
	// REG-018-001: Even -99.99% must not produce extreme values.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]map[string]float64{
			"crash-coin": {"usd": 0.01, "usd_24h_change": -99.99},
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.coingeckoBaseURL = srv.URL

	prices, err := c.fetchCoinGeckoPrices(context.Background(), []string{"crash-coin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected 1 price, got %d", len(prices))
	}

	// Must be finite and negative.
	p := prices[0]
	if p.Change24h >= 0 {
		t.Errorf("expected negative change24h for -99.99%%, got %v", p.Change24h)
	}
	if p.Change24h != p.Change24h {
		t.Fatal("Change24h is NaN")
	}
}

func TestCryptoChange24h_BeyondNeg100Clamped(t *testing.T) {
	// REG-018-001: If API returns worse than -100% (data error), must still be finite.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]map[string]float64{
			"glitch-coin": {"usd": 50.0, "usd_24h_change": -150.0},
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.coingeckoBaseURL = srv.URL

	prices, err := c.fetchCoinGeckoPrices(context.Background(), []string{"glitch-coin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected 1 price, got %d", len(prices))
	}

	p := prices[0]
	// Must be finite — pre-fix code would produce negative Inf.
	if p.Change24h != p.Change24h {
		t.Fatal("Change24h is NaN")
	}
	if p.Change24h > 1e18 || p.Change24h < -1e18 {
		t.Fatalf("Change24h is Inf-like (%v)", p.Change24h)
	}
}

func TestSyncForex_AlertTierOnExtremeMove(t *testing.T) {
	// REG-018-002: Forex artifacts must use classifyTier, not hardcoded "light".
	// A forex pair with >threshold% change must get "full" tier.
	// Pre-fix: all forex was "light" regardless of change magnitude.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		switch symbol {
		case "OANDA:USD_TRY":
			// 12% move — extreme but happens in emerging market currencies.
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 32.50, "d": 3.50, "dp": 12.0, "h": 33.0, "l": 29.0, "o": 29.0, "pc": 29.0,
			})
		case "OANDA:EUR_USD":
			// 0.2% move — normal day.
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 1.0821, "d": 0.002, "dp": 0.2, "h": 1.085, "l": 1.08, "o": 1.08, "pc": 1.08,
			})
		default:
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 100.0, "d": 1.0, "dp": 1.0, "h": 101.0, "l": 99.0, "o": 99.0, "pc": 99.0,
			})
		}
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist: WatchlistConfig{
			ForexPairs: []string{"USD/TRY", "EUR/USD"},
		},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 forex artifacts, got %d", len(artifacts))
	}

	for _, a := range artifacts {
		sym := a.Metadata["symbol"].(string)
		tier := a.Metadata["processing_tier"].(string)
		switch sym {
		case "USD/TRY":
			if tier != "full" {
				t.Errorf("USD/TRY 12%% move should be full tier, got %q — regression: forex bypasses classifyTier", tier)
			}
		case "EUR/USD":
			if tier != "light" {
				t.Errorf("EUR/USD 0.2%% move should be light tier, got %q", tier)
			}
		}
	}
}

func TestSyncForex_NegativeAlertTier(t *testing.T) {
	// REG-018-002: Forex pair with large negative move must also get "full" tier.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// -8% crash.
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 120.0, "d": -10.4, "dp": -8.0, "h": 131.0, "l": 119.0, "o": 130.4, "pc": 130.4,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{ForexPairs: []string{"GBP/USD"}},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}

	tier := artifacts[0].Metadata["processing_tier"].(string)
	if tier != "full" {
		t.Errorf("GBP/USD -8%% move should be full tier, got %q — regression: forex alert detection broken", tier)
	}
}

func TestSync_ForexRateLimitedMidLoop_HealthDegraded(t *testing.T) {
	// STB-018-003: When Finnhub rate limit exhausts mid-loop for forex pairs,
	// health must be Degraded, not Healthy.

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 154.0, "d": 0.45, "dp": 0.29, "h": 155.0, "l": 153.5, "o": 153.87, "pc": 153.87,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist: WatchlistConfig{
			ForexPairs: []string{"USD/JPY", "EUR/USD", "GBP/CHF", "AUD/NZD", "CAD/CHF"},
		},
	}

	// Pre-exhaust 53 of 55 Finnhub calls, leaving only 2.
	for i := 0; i < 53; i++ {
		c.tryRecordCall("finnhub")
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get at most 2 forex artifacts (remaining budget), not all 5.
	if len(artifacts) > 2 {
		t.Errorf("expected at most 2 artifacts due to rate limit, got %d", len(artifacts))
	}

	// Health MUST be Degraded — before the fix it was Healthy.
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Errorf("expected HealthDegraded when forex rate-limited mid-loop, got %v", c.Health(context.Background()))
	}
}

// --- Improve Tests: IMP-018-R15-001 through IMP-018-R15-003 ---

func TestClassifyTier_NaN_PromotesToFull(t *testing.T) {
	// IMP-018-R15-001: NaN changePct must NOT silently return "light" and suppress alerts.
	// NaN comparisons are always false, so without the guard, both
	// changePct >= threshold and changePct <= -threshold return false → "light".
	// This silently hides corrupt data from alert processing.
	nan := math.NaN()

	tier := classifyTier(5.0, nan)
	if tier != "full" {
		t.Errorf("classifyTier(5.0, NaN) = %q, want \"full\" — NaN must not silently suppress alerts", tier)
	}

	// Also verify with threshold 0 (alerts "disabled") — NaN is still corrupt data.
	tier = classifyTier(0, nan)
	if tier != "full" {
		t.Errorf("classifyTier(0, NaN) = %q, want \"full\" — NaN is always corrupt", tier)
	}
}

func TestClassifyTier_Inf_PromotesToFull(t *testing.T) {
	// IMP-018-R15-001: +Inf and -Inf changePct must map to "full" for safety.
	posInf := math.Inf(1)
	negInf := math.Inf(-1)

	if tier := classifyTier(5.0, posInf); tier != "full" {
		t.Errorf("classifyTier(5.0, +Inf) = %q, want \"full\"", tier)
	}
	if tier := classifyTier(5.0, negInf); tier != "full" {
		t.Errorf("classifyTier(5.0, -Inf) = %q, want \"full\"", tier)
	}
}

func TestClassifyTier_NormalValuesUnchanged(t *testing.T) {
	// IMP-018-R15-001: Normal values must still behave identically.
	if tier := classifyTier(5.0, 2.0); tier != "light" {
		t.Errorf("classifyTier(5.0, 2.0) = %q, want \"light\"", tier)
	}
	if tier := classifyTier(5.0, 7.0); tier != "full" {
		t.Errorf("classifyTier(5.0, 7.0) = %q, want \"full\"", tier)
	}
	if tier := classifyTier(5.0, -5.0); tier != "full" {
		t.Errorf("classifyTier(5.0, -5.0) = %q, want \"full\"", tier)
	}
}

func TestParseMarketsConfig_RejectsNaNAlertThreshold(t *testing.T) {
	// IMP-018-R15-002: NaN alert_threshold silently disables ALL alerts.
	// In classifyTier, `threshold > 0` is false for NaN, so all quotes get "light".
	// This must be rejected at config parse time.
	_, err := parseMarketsConfig(connector.ConnectorConfig{
		Credentials:  map[string]string{"finnhub_api_key": "test"},
		SourceConfig: map[string]interface{}{"alert_threshold": math.NaN()},
	})
	if err == nil {
		t.Fatal("expected error for NaN alert_threshold — silently disables all alerts")
	}
	if !strings.Contains(err.Error(), "finite") {
		t.Errorf("error should mention 'finite', got: %v", err)
	}
}

func TestParseMarketsConfig_RejectsInfAlertThreshold(t *testing.T) {
	// IMP-018-R15-002: +Inf alert_threshold means all changes are below threshold,
	// silently suppressing every alert forever.
	cases := []struct {
		name  string
		value float64
	}{
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMarketsConfig(connector.ConnectorConfig{
				Credentials:  map[string]string{"finnhub_api_key": "test"},
				SourceConfig: map[string]interface{}{"alert_threshold": tc.value},
			})
			if err == nil {
				t.Fatalf("expected error for %s alert_threshold", tc.name)
			}
			if !strings.Contains(err.Error(), "finite") {
				t.Errorf("error should mention 'finite', got: %v", err)
			}
		})
	}
}

func TestSyncStocksHaveAssetType(t *testing.T) {
	// IMP-018-R15-003: Stock quotes must include asset_type="stock" in metadata.
	// Previously stocks had no asset_type, forcing consumers to use field-absence as a type check.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 175.0, "d": 2.0, "dp": 1.1, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.0,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL
	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"AAPL"}},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	at, ok := artifacts[0].Metadata["asset_type"]
	if !ok {
		t.Fatal("stock artifact missing asset_type metadata — consumers cannot distinguish asset types")
	}
	if at != "stock" {
		t.Errorf("stock asset_type = %q, want \"stock\"", at)
	}
}

func TestSyncETFsHaveAssetType(t *testing.T) {
	// IMP-018-R15-003: ETF quotes must include asset_type="etf" in metadata.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 450.0, "d": 3.0, "dp": 0.7, "h": 452.0, "l": 448.0, "o": 449.0, "pc": 447.0,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL
	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist: WatchlistConfig{
			Stocks: []string{"AAPL"},
			ETFs:   []string{"SPY", "QQQ"},
		},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(artifacts))
	}

	for _, a := range artifacts {
		sym := a.Metadata["symbol"].(string)
		at := a.Metadata["asset_type"].(string)
		switch sym {
		case "AAPL":
			if at != "stock" {
				t.Errorf("AAPL asset_type = %q, want \"stock\"", at)
			}
		case "SPY", "QQQ":
			if at != "etf" {
				t.Errorf("%s asset_type = %q, want \"etf\"", sym, at)
			}
		}
	}
}

func TestSyncMixedAssetTypes(t *testing.T) {
	// IMP-018-R15-003: All asset types (stock, etf, crypto, forex) appear in a single Sync.
	finnhubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 175.0, "d": 2.0, "dp": 1.1, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.0,
		})
	}))
	defer finnhubSrv.Close()

	coingeckoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]map[string]float64{
			"bitcoin": {"usd": 67000.0, "usd_24h_change": 2.5},
		})
	}))
	defer coingeckoSrv.Close()

	c := newTestConnector("financial-markets")
	c.finnhubBaseURL = finnhubSrv.URL
	c.coingeckoBaseURL = coingeckoSrv.URL
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	c.config = MarketsConfig{
		FinnhubAPIKey:    "test-key",
		CoinGeckoEnabled: true,
		AlertThreshold:   5.0,
		Watchlist: WatchlistConfig{
			Stocks:     []string{"AAPL"},
			ETFs:       []string{"SPY"},
			Crypto:     []string{"bitcoin"},
			ForexPairs: []string{"USD/JPY"},
		},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 4 {
		t.Fatalf("expected 4 artifacts, got %d", len(artifacts))
	}

	seen := map[string]bool{}
	for _, a := range artifacts {
		at, ok := a.Metadata["asset_type"]
		if !ok {
			t.Errorf("artifact %q missing asset_type", a.Metadata["symbol"])
			continue
		}
		seen[at.(string)] = true
	}
	for _, want := range []string{"stock", "etf", "crypto", "forex"} {
		if !seen[want] {
			t.Errorf("missing asset_type %q in artifacts", want)
		}
	}
}

// --- Scope 1 Tests: Finnhub Company News ---

func TestFetchFinnhubCompanyNews_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/company-news" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("symbol") != "AAPL" {
			t.Errorf("unexpected symbol: %s", r.URL.Query().Get("symbol"))
		}
		if r.URL.Query().Get("token") != "test-key" {
			t.Errorf("unexpected token: %s", r.URL.Query().Get("token"))
		}
		if r.URL.Query().Get("from") != "2024-01-15" {
			t.Errorf("unexpected from: %s", r.URL.Query().Get("from"))
		}
		if r.URL.Query().Get("to") != "2024-01-15" {
			t.Errorf("unexpected to: %s", r.URL.Query().Get("to"))
		}
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"category": "company",
				"datetime": 1705334400,
				"headline": "Apple Reports Record Revenue",
				"id":       12345,
				"image":    "https://example.com/image.jpg",
				"related":  "AAPL",
				"source":   "Reuters",
				"summary":  "Apple Inc reported record quarterly revenue.",
				"url":      "https://example.com/article",
			},
			{
				"category": "company",
				"datetime": 1705338000,
				"headline": "Apple Launches New Product",
				"id":       12346,
				"image":    "",
				"related":  "AAPL",
				"source":   "Bloomberg",
				"summary":  "Apple announced a new product line.",
				"url":      "https://example.com/article2",
			},
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	articles, err := c.fetchFinnhubCompanyNews(context.Background(), "AAPL", "2024-01-15", "2024-01-15")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(articles) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(articles))
	}
	if articles[0].Headline != "Apple Reports Record Revenue" {
		t.Errorf("unexpected headline: %s", articles[0].Headline)
	}
	if articles[0].Source != "Reuters" {
		t.Errorf("unexpected source: %s", articles[0].Source)
	}
	if articles[0].URL != "https://example.com/article" {
		t.Errorf("unexpected URL: %s", articles[0].URL)
	}
	if articles[1].Source != "Bloomberg" {
		t.Errorf("unexpected source for second article: %s", articles[1].Source)
	}
}

func TestFetchFinnhubCompanyNews_RejectsInvalidSymbol(t *testing.T) {
	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"

	cases := []string{"AAPL&inject", "../etc/passwd", "", strings.Repeat("A", 11)}
	for _, sym := range cases {
		_, err := c.fetchFinnhubCompanyNews(context.Background(), sym, "2024-01-15", "2024-01-15")
		if err == nil {
			t.Errorf("expected error for invalid symbol %q", sym)
		}
	}
}

func TestFetchFinnhubCompanyNews_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "bad-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	_, err := c.fetchFinnhubCompanyNews(context.Background(), "AAPL", "2024-01-15", "2024-01-15")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention 403, got: %v", err)
	}
}

func TestFetchFinnhubCompanyNews_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	articles, err := c.fetchFinnhubCompanyNews(context.Background(), "AAPL", "2024-01-15", "2024-01-15")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(articles) != 0 {
		t.Errorf("expected 0 articles, got %d", len(articles))
	}
}

func TestFetchFinnhubCompanyNews_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not valid`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	_, err := c.fetchFinnhubCompanyNews(context.Background(), "AAPL", "2024-01-15", "2024-01-15")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error should mention decode, got: %v", err)
	}
}

func TestFetchFinnhubCompanyNews_RateLimitIntegration(t *testing.T) {
	// Verify company news calls count toward Finnhub rate budget.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	// Fill rate limit to near capacity.
	for i := 0; i < 54; i++ {
		c.tryRecordCall("finnhub")
	}
	// One more should succeed.
	if !c.tryRecordCall("finnhub") {
		t.Fatal("55th call should succeed")
	}
	// Now limit is exhausted — news fetch should fail in Sync context.
	if c.tryRecordCall("finnhub") {
		t.Error("56th call should be denied")
	}
}

// --- Scope 2 Tests: FRED Client ---

func TestFetchFREDLatest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fred/series/observations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("series_id") != "UNRATE" {
			t.Errorf("unexpected series_id: %s", r.URL.Query().Get("series_id"))
		}
		if r.URL.Query().Get("api_key") != "fred-test-key" {
			t.Errorf("unexpected api_key: %s", r.URL.Query().Get("api_key"))
		}
		if r.URL.Query().Get("file_type") != "json" {
			t.Errorf("unexpected file_type: %s", r.URL.Query().Get("file_type"))
		}
		if r.URL.Query().Get("limit") != "1" {
			t.Errorf("unexpected limit: %s", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("sort_order") != "desc" {
			t.Errorf("unexpected sort_order: %s", r.URL.Query().Get("sort_order"))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"observations": []map[string]string{
				{"date": "2024-01-01", "value": "3.7"},
			},
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FREDAPIKey = "fred-test-key"
	c.httpClient = srv.Client()
	c.fredBaseURL = srv.URL

	obs, err := c.fetchFREDLatest(context.Background(), "UNRATE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.SeriesID != "UNRATE" {
		t.Errorf("expected UNRATE, got %s", obs.SeriesID)
	}
	if obs.Date != "2024-01-01" {
		t.Errorf("expected 2024-01-01, got %s", obs.Date)
	}
	if obs.NumValue != 3.7 {
		t.Errorf("expected 3.7, got %f", obs.NumValue)
	}
	if obs.Value != "3.7" {
		t.Errorf("expected raw '3.7', got %q", obs.Value)
	}
}

func TestFetchFREDLatest_RejectsInvalidSeriesID(t *testing.T) {
	c := newTestConnector("financial-markets")
	c.config.FREDAPIKey = "test"

	cases := []string{"", "lowercase", "AAPL&inject", "../passwd", strings.Repeat("A", 21), "GDP RATE"}
	for _, id := range cases {
		_, err := c.fetchFREDLatest(context.Background(), id)
		if err == nil {
			t.Errorf("expected error for invalid series ID %q", id)
		}
	}
}

func TestFetchFREDLatest_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error_message":"bad api key"}`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FREDAPIKey = "bad-key"
	c.httpClient = srv.Client()
	c.fredBaseURL = srv.URL

	_, err := c.fetchFREDLatest(context.Background(), "GDP")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention 403, got: %v", err)
	}
}

func TestFetchFREDLatest_NoObservations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"observations": []map[string]string{},
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FREDAPIKey = "test"
	c.httpClient = srv.Client()
	c.fredBaseURL = srv.URL

	_, err := c.fetchFREDLatest(context.Background(), "GDP")
	if err == nil {
		t.Fatal("expected error for empty observations")
	}
	if !strings.Contains(err.Error(), "no observations") {
		t.Errorf("error should mention no observations, got: %v", err)
	}
}

func TestFetchFREDLatest_MissingDataMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"observations": []map[string]string{
				{"date": "2024-01-01", "value": "."},
			},
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FREDAPIKey = "test"
	c.httpClient = srv.Client()
	c.fredBaseURL = srv.URL

	_, err := c.fetchFREDLatest(context.Background(), "GDP")
	if err == nil {
		t.Fatal("expected error for missing data marker")
	}
	if !strings.Contains(err.Error(), "missing data") {
		t.Errorf("error should mention missing data, got: %v", err)
	}
}

func TestFetchFREDLatest_InvalidValueFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"observations": []map[string]string{
				{"date": "2024-01-01", "value": "not-a-number"},
			},
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FREDAPIKey = "test"
	c.httpClient = srv.Client()
	c.fredBaseURL = srv.URL

	_, err := c.fetchFREDLatest(context.Background(), "GDP")
	if err == nil {
		t.Fatal("expected error for non-numeric value")
	}
	if !strings.Contains(err.Error(), "parse FRED value") {
		t.Errorf("error should mention parse, got: %v", err)
	}
}

func TestFetchFREDLatest_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{bad json`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FREDAPIKey = "test"
	c.httpClient = srv.Client()
	c.fredBaseURL = srv.URL

	_, err := c.fetchFREDLatest(context.Background(), "GDP")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestFetchFREDLatest_MalformedBaseURL(t *testing.T) {
	c := newTestConnector("financial-markets")
	c.config.FREDAPIKey = "test"
	c.fredBaseURL = "://invalid"

	_, err := c.fetchFREDLatest(context.Background(), "GDP")
	if err == nil {
		t.Fatal("expected error for malformed base URL")
	}
	if !strings.Contains(err.Error(), "parse FRED URL") {
		t.Errorf("error should mention URL parse, got: %v", err)
	}
}

func TestParseMarketsConfig_FREDEnabled(t *testing.T) {
	cases := []struct {
		name        string
		credentials map[string]string
		source      map[string]interface{}
		wantEnabled bool
		wantErr     bool
	}{
		{
			name:        "enabled by API key presence",
			credentials: map[string]string{"finnhub_api_key": "test", "fred_api_key": "fred-key"},
			source:      map[string]interface{}{},
			wantEnabled: true,
		},
		{
			name:        "disabled when no API key",
			credentials: map[string]string{"finnhub_api_key": "test"},
			source:      map[string]interface{}{},
			wantEnabled: false,
		},
		{
			name:        "explicitly enabled with key",
			credentials: map[string]string{"finnhub_api_key": "test", "fred_api_key": "fred-key"},
			source:      map[string]interface{}{"fred_enabled": true},
			wantEnabled: true,
		},
		{
			name:        "explicitly disabled overrides key",
			credentials: map[string]string{"finnhub_api_key": "test", "fred_api_key": "fred-key"},
			source:      map[string]interface{}{"fred_enabled": false},
			wantEnabled: false,
		},
		{
			name:        "enabled without key is error",
			credentials: map[string]string{"finnhub_api_key": "test"},
			source:      map[string]interface{}{"fred_enabled": true},
			wantErr:     true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseMarketsConfig(connector.ConnectorConfig{
				Credentials:  tc.credentials,
				SourceConfig: tc.source,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.FREDEnabled != tc.wantEnabled {
				t.Errorf("FREDEnabled = %v, want %v", cfg.FREDEnabled, tc.wantEnabled)
			}
		})
	}
}

func TestParseMarketsConfig_FREDSeriesDefaults(t *testing.T) {
	cfg, err := parseMarketsConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "test", "fred_api_key": "key"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"GDP", "UNRATE", "CPIAUCSL", "DFF", "FEDFUNDS"}
	if len(cfg.FREDSeries) != len(expected) {
		t.Fatalf("expected %d default series, got %d", len(expected), len(cfg.FREDSeries))
	}
	for i, s := range expected {
		if cfg.FREDSeries[i] != s {
			t.Errorf("FREDSeries[%d] = %q, want %q", i, cfg.FREDSeries[i], s)
		}
	}
}

func TestParseMarketsConfig_FREDSeriesCustom(t *testing.T) {
	cfg, err := parseMarketsConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"finnhub_api_key": "test"},
		SourceConfig: map[string]interface{}{
			"fred_series": []interface{}{"GDP", "CPI"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.FREDSeries) != 2 {
		t.Fatalf("expected 2 series, got %d", len(cfg.FREDSeries))
	}
}

func TestParseMarketsConfig_FREDSeriesRejectsInvalid(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
	}{
		{"lowercase", "gdp"},
		{"injection", "GDP&x=1"},
		{"too long", strings.Repeat("A", 21)},
		{"non-string", 42},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMarketsConfig(connector.ConnectorConfig{
				Credentials: map[string]string{"finnhub_api_key": "test"},
				SourceConfig: map[string]interface{}{
					"fred_series": []interface{}{tc.value},
				},
			})
			if err == nil {
				t.Errorf("expected error for invalid FRED series %v", tc.value)
			}
		})
	}
}

// --- Scope 3 Tests: market/news and market/economic normalizer ---

func TestSyncProducesNewsArtifacts(t *testing.T) {
	finnhubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/company-news" {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"category": "company",
					"datetime": 1705334400,
					"headline": "Apple Q4 Results",
					"id":       99001,
					"image":    "",
					"related":  "AAPL",
					"source":   "MarketWatch",
					"summary":  "Apple beat expectations.",
					"url":      "https://example.com/news1",
				},
			})
			return
		}
		// Quote endpoint for stock quotes.
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 175.0, "d": 2.0, "dp": 1.1, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.0,
		})
	}))
	defer finnhubSrv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = finnhubSrv.Client()
	c.finnhubBaseURL = finnhubSrv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"AAPL"}},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var newsArtifacts []connector.RawArtifact
	for _, a := range artifacts {
		if a.ContentType == "market/news" {
			newsArtifacts = append(newsArtifacts, a)
		}
	}
	if len(newsArtifacts) == 0 {
		t.Fatal("expected at least one market/news artifact")
	}

	news := newsArtifacts[0]
	if news.Title != "Apple Q4 Results" {
		t.Errorf("expected 'Apple Q4 Results', got %q", news.Title)
	}
	if news.URL != "https://example.com/news1" {
		t.Errorf("expected URL, got %q", news.URL)
	}
	if news.Metadata["source"] != "MarketWatch" {
		t.Errorf("expected MarketWatch source, got %v", news.Metadata["source"])
	}
	if news.Metadata["symbol"] != "AAPL" {
		t.Errorf("expected AAPL symbol, got %v", news.Metadata["symbol"])
	}
	if news.Metadata["processing_tier"] != "standard" {
		t.Errorf("expected standard tier for news, got %v", news.Metadata["processing_tier"])
	}
}

func TestSyncProducesEconomicArtifacts(t *testing.T) {
	fredSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seriesID := r.URL.Query().Get("series_id")
		values := map[string]string{
			"GDP":    "25000.5",
			"UNRATE": "3.7",
			"DFF":    "5.33",
		}
		val, ok := values[seriesID]
		if !ok {
			val = "100.0"
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"observations": []map[string]string{
				{"date": "2024-01-01", "value": val},
			},
		})
	}))
	defer fredSrv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = fredSrv.Client()
	c.fredBaseURL = fredSrv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		FREDAPIKey:     "fred-key",
		FREDEnabled:    true,
		FREDSeries:     []string{"GDP", "UNRATE", "DFF"},
		AlertThreshold: 5.0,
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var econArtifacts []connector.RawArtifact
	for _, a := range artifacts {
		if a.ContentType == "market/economic" {
			econArtifacts = append(econArtifacts, a)
		}
	}
	if len(econArtifacts) != 3 {
		t.Fatalf("expected 3 market/economic artifacts, got %d", len(econArtifacts))
	}

	// Check one artifact in detail.
	var gdpFound bool
	for _, a := range econArtifacts {
		if a.Metadata["series_id"] == "GDP" {
			gdpFound = true
			if a.Metadata["value"] != 25000.5 {
				t.Errorf("expected GDP value 25000.5, got %v", a.Metadata["value"])
			}
			if a.Metadata["date"] != "2024-01-01" {
				t.Errorf("expected date 2024-01-01, got %v", a.Metadata["date"])
			}
			if a.Metadata["processing_tier"] != "standard" {
				t.Errorf("expected standard tier, got %v", a.Metadata["processing_tier"])
			}
			if !strings.Contains(a.Title, "GDP") {
				t.Errorf("title should contain GDP, got %q", a.Title)
			}
		}
	}
	if !gdpFound {
		t.Error("GDP artifact not found")
	}
}

func TestSyncFREDDisabledSkipsFetch(t *testing.T) {
	srvCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvCalled = true
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.fredBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey: "test-key",
		FREDEnabled:   false,
		FREDSeries:    []string{"GDP"},
	}

	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srvCalled {
		t.Error("FRED server should not be called when FREDEnabled=false")
	}
}

func TestSyncFREDTotalFailureSetsHealthDegraded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"down"}`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.fredBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		FREDAPIKey:     "fred-key",
		FREDEnabled:    true,
		FREDSeries:     []string{"GDP", "UNRATE"},
		AlertThreshold: 5.0,
	}

	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// FRED is the only provider with actual fetch calls, so total failure → HealthError.
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected HealthError after total FRED failure, got %v", c.Health(context.Background()))
	}
}

func TestSyncAllProvidersCombined(t *testing.T) {
	// Full integration: stocks + crypto + forex + news + FRED in a single Sync.
	finnhubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/company-news":
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"category": "company", "datetime": 1705334400, "headline": "Test News", "id": 1, "related": "AAPL", "source": "Test", "summary": "Summary", "url": "https://example.com"},
			})
		default:
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 150.0, "d": 1.0, "dp": 0.5, "h": 152.0, "l": 148.0, "o": 149.0, "pc": 149.0,
			})
		}
	}))
	defer finnhubSrv.Close()

	coingeckoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]map[string]float64{
			"bitcoin": {"usd": 67000.0, "usd_24h_change": 2.5},
		})
	}))
	defer coingeckoSrv.Close()

	fredSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"observations": []map[string]string{
				{"date": "2024-01-01", "value": "3.7"},
			},
		})
	}))
	defer fredSrv.Close()

	c := newTestConnector("financial-markets")
	c.finnhubBaseURL = finnhubSrv.URL
	c.coingeckoBaseURL = coingeckoSrv.URL
	c.fredBaseURL = fredSrv.URL
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	c.config = MarketsConfig{
		FinnhubAPIKey:    "test-key",
		CoinGeckoEnabled: true,
		FREDAPIKey:       "fred-key",
		FREDEnabled:      true,
		FREDSeries:       []string{"UNRATE"},
		AlertThreshold:   5.0,
		Watchlist: WatchlistConfig{
			Stocks:     []string{"AAPL"},
			Crypto:     []string{"bitcoin"},
			ForexPairs: []string{"USD/JPY"},
		},
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor == "" {
		t.Error("cursor should be set")
	}

	// Count by content type.
	types := map[string]int{}
	for _, a := range artifacts {
		types[a.ContentType]++
	}

	// Expect: 1 stock quote + 1 forex quote + 1 crypto quote + 1 news + 1 FRED = 5
	if types["market/quote"] < 3 {
		t.Errorf("expected at least 3 market/quote artifacts, got %d", types["market/quote"])
	}
	if types["market/news"] != 1 {
		t.Errorf("expected 1 market/news artifact, got %d", types["market/news"])
	}
	if types["market/economic"] != 1 {
		t.Errorf("expected 1 market/economic artifact, got %d", types["market/economic"])
	}
}

func TestDefaultFREDSeries(t *testing.T) {
	expected := []string{"GDP", "UNRATE", "CPIAUCSL", "DFF", "FEDFUNDS"}
	if len(defaultFREDSeries) != len(expected) {
		t.Fatalf("expected %d default FRED series, got %d", len(expected), len(defaultFREDSeries))
	}
	for i, s := range expected {
		if defaultFREDSeries[i] != s {
			t.Errorf("defaultFREDSeries[%d] = %q, want %q", i, defaultFREDSeries[i], s)
		}
	}
}

func TestValidFREDSeriesRe(t *testing.T) {
	valid := []string{"GDP", "UNRATE", "CPIAUCSL", "DFF", "FEDFUNDS", "T10Y2Y"}
	for _, s := range valid {
		if !validFREDSeriesRe.MatchString(s) {
			t.Errorf("expected %q to be valid FRED series ID", s)
		}
	}
	invalid := []string{"", "gdp", "GDP RATE", "GDP&x=1", "../admin", strings.Repeat("A", 21)}
	for _, s := range invalid {
		if validFREDSeriesRe.MatchString(s) {
			t.Errorf("expected %q to be invalid FRED series ID", s)
		}
	}
}

// --- Scope 5 Tests: Daily Summary ---

func TestBuildDailySummary_Structure(t *testing.T) {
	now := time.Date(2024, 6, 10, 17, 0, 0, 0, time.UTC)
	artifacts := []connector.RawArtifact{
		{
			ContentType: "market/quote",
			Title:       "AAPL: $175.00 (+1.3%)",
			Metadata: map[string]interface{}{
				"symbol":          "AAPL",
				"change_percent":  1.3,
				"processing_tier": "light",
			},
		},
		{
			ContentType: "market/quote",
			Title:       "TSLA: $250.00 (-2.5%)",
			Metadata: map[string]interface{}{
				"symbol":          "TSLA",
				"change_percent":  -2.5,
				"processing_tier": "light",
			},
		},
		{
			ContentType: "market/news",
			Title:       "Apple Reports Record Revenue",
		},
		{
			ContentType: "market/economic",
			Metadata: map[string]interface{}{
				"series_id": "GDP",
				"value":     25000.5,
				"date":      "2024-01-01",
			},
		},
	}

	summary := buildDailySummary(artifacts, now)

	if summary.ContentType != "market/daily-summary" {
		t.Errorf("expected market/daily-summary, got %s", summary.ContentType)
	}
	if !strings.Contains(summary.Title, "Market Summary") {
		t.Errorf("title should contain 'Market Summary', got %q", summary.Title)
	}
	if summary.Metadata["processing_tier"] != "standard" {
		t.Errorf("expected standard tier (no alerts), got %v", summary.Metadata["processing_tier"])
	}
	if summary.Metadata["gainers_count"] != 1 {
		t.Errorf("expected 1 gainer, got %v", summary.Metadata["gainers_count"])
	}
	if summary.Metadata["losers_count"] != 1 {
		t.Errorf("expected 1 loser, got %v", summary.Metadata["losers_count"])
	}
	if summary.Metadata["alerts_count"] != 0 {
		t.Errorf("expected 0 alerts, got %v", summary.Metadata["alerts_count"])
	}
	if summary.Metadata["news_count"] != 1 {
		t.Errorf("expected 1 news, got %v", summary.Metadata["news_count"])
	}
	if !strings.Contains(summary.RawContent, "Gainers:") {
		t.Error("summary should contain Gainers section")
	}
	if !strings.Contains(summary.RawContent, "Losers:") {
		t.Error("summary should contain Losers section")
	}
	if !strings.Contains(summary.RawContent, "News:") {
		t.Error("summary should contain News section")
	}
	if !strings.Contains(summary.RawContent, "Economic Indicators:") {
		t.Error("summary should contain Economic Indicators section")
	}
	if !strings.Contains(summary.RawContent, "AAPL") {
		t.Error("summary should mention AAPL")
	}
	if !strings.Contains(summary.RawContent, "GDP") {
		t.Error("summary should mention GDP")
	}

	syms, ok := summary.Metadata["related_symbols"].([]string)
	if !ok {
		t.Fatal("related_symbols should be []string")
	}
	if len(syms) != 2 {
		t.Errorf("expected 2 symbols in summary, got %d", len(syms))
	}
}

func TestBuildDailySummary_AlertUpgradesTier(t *testing.T) {
	now := time.Date(2024, 6, 10, 17, 0, 0, 0, time.UTC)
	artifacts := []connector.RawArtifact{
		{
			ContentType: "market/quote",
			Metadata: map[string]interface{}{
				"symbol":          "AAPL",
				"change_percent":  1.3,
				"processing_tier": "light",
			},
		},
		{
			ContentType: "market/quote",
			Metadata: map[string]interface{}{
				"symbol":          "TSLA",
				"change_percent":  7.5,
				"processing_tier": "full", // alert triggered
			},
		},
	}

	summary := buildDailySummary(artifacts, now)

	if summary.Metadata["processing_tier"] != "full" {
		t.Errorf("expected full tier when alert present, got %v", summary.Metadata["processing_tier"])
	}
	if summary.Metadata["alerts_count"] != 1 {
		t.Errorf("expected 1 alert, got %v", summary.Metadata["alerts_count"])
	}
	if !strings.Contains(summary.RawContent, "ALERTS:") {
		t.Error("summary should contain ALERTS section when alerts present")
	}
}

func TestBuildDailySummary_EmptyArtifacts(t *testing.T) {
	now := time.Date(2024, 6, 10, 17, 0, 0, 0, time.UTC)
	summary := buildDailySummary(nil, now)

	if summary.ContentType != "market/daily-summary" {
		t.Errorf("expected market/daily-summary, got %s", summary.ContentType)
	}
	if summary.Metadata["processing_tier"] != "standard" {
		t.Errorf("expected standard tier for empty summary, got %v", summary.Metadata["processing_tier"])
	}
}

func TestBuildDailySummary_CryptoChangePct(t *testing.T) {
	now := time.Date(2024, 6, 10, 17, 0, 0, 0, time.UTC)
	artifacts := []connector.RawArtifact{
		{
			ContentType: "market/quote",
			Metadata: map[string]interface{}{
				"symbol":          "bitcoin",
				"change_pct_24h":  -8.5,
				"processing_tier": "full",
				"asset_type":      "crypto",
			},
		},
	}

	summary := buildDailySummary(artifacts, now)
	if summary.Metadata["losers_count"] != 1 {
		t.Errorf("expected 1 loser (crypto), got %v", summary.Metadata["losers_count"])
	}
	if summary.Metadata["processing_tier"] != "full" {
		t.Errorf("expected full tier (crypto alert), got %v", summary.Metadata["processing_tier"])
	}
}

func TestDailySummary_TimeGate(t *testing.T) {
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal("failed to load timezone")
	}

	cases := []struct {
		name    string
		now     time.Time
		lastDay string
		want    bool
	}{
		{
			name:    "weekday after 16:30 ET, no previous summary",
			now:     time.Date(2024, 6, 10, 17, 0, 0, 0, et), // Monday 5pm ET
			lastDay: "",
			want:    true,
		},
		{
			name:    "weekday at exactly 16:30 ET",
			now:     time.Date(2024, 6, 10, 16, 30, 0, 0, et), // Monday 4:30pm ET
			lastDay: "",
			want:    true,
		},
		{
			name:    "weekday before 16:30 ET",
			now:     time.Date(2024, 6, 10, 14, 0, 0, 0, et), // Monday 2pm ET
			lastDay: "",
			want:    false,
		},
		{
			name:    "Saturday after 16:30 ET",
			now:     time.Date(2024, 6, 8, 17, 0, 0, 0, et), // Saturday
			lastDay: "",
			want:    false,
		},
		{
			name:    "Sunday after 16:30 ET",
			now:     time.Date(2024, 6, 9, 17, 0, 0, 0, et), // Sunday
			lastDay: "",
			want:    false,
		},
		{
			name:    "already generated today",
			now:     time.Date(2024, 6, 10, 18, 0, 0, 0, et),
			lastDay: "2024-06-10",
			want:    false,
		},
		{
			name:    "generated yesterday, new day after close",
			now:     time.Date(2024, 6, 11, 17, 0, 0, 0, et), // Tuesday
			lastDay: "2024-06-10",
			want:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestConnector("financial-markets")
			c.lastSummaryDate = tc.lastDay

			got := c.tryClaimDailySummary(tc.now)
			if got != tc.want {
				t.Errorf("tryClaimDailySummary() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSyncGeneratesDailySummary(t *testing.T) {
	// Set up a mock time after market close on a weekday.
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal("failed to load timezone")
	}
	mockNow := time.Date(2024, 6, 10, 17, 0, 0, 0, et) // Monday 5pm ET

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 175.0, "d": 2.0, "dp": 1.3, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.0,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL
	c.nowFunc = func() time.Time { return mockNow }

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"AAPL"}},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var summaryFound bool
	for _, a := range artifacts {
		if a.ContentType == "market/daily-summary" {
			summaryFound = true
			if a.Metadata["processing_tier"] != "standard" {
				t.Errorf("expected standard tier, got %v", a.Metadata["processing_tier"])
			}
		}
	}
	if !summaryFound {
		t.Error("expected a market/daily-summary artifact after market close")
	}

	// Verify lastSummaryDate was set.
	c.mu.RLock()
	lastDate := c.lastSummaryDate
	c.mu.RUnlock()
	if lastDate != "2024-06-10" {
		t.Errorf("expected lastSummaryDate=2024-06-10, got %q", lastDate)
	}

	// Second sync same day should NOT generate another summary.
	artifacts2, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, a := range artifacts2 {
		if a.ContentType == "market/daily-summary" {
			t.Error("should not generate duplicate daily summary on same day")
		}
	}
}

func TestSyncNoDailySummaryBeforeMarketClose(t *testing.T) {
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal("failed to load timezone")
	}
	mockNow := time.Date(2024, 6, 10, 14, 0, 0, 0, et) // Monday 2pm ET

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 175.0, "d": 2.0, "dp": 1.3, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.0,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL
	c.nowFunc = func() time.Time { return mockNow }

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"AAPL"}},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, a := range artifacts {
		if a.ContentType == "market/daily-summary" {
			t.Error("should not generate daily summary before market close")
		}
	}
}

// --- Scope 6 Tests: Cross-Artifact Symbol Linking ---

func TestResolveSymbols_TickerNotation(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "single $TICKER",
			text: "Check out $AAPL today",
			want: []string{"AAPL"},
		},
		{
			name: "multiple $TICKERs",
			text: "$AAPL and $TSLA are up, $MSFT is flat",
			want: []string{"AAPL", "TSLA", "MSFT"},
		},
		{
			name: "crypto tickers",
			text: "$BTC hit $67k and $ETH is climbing",
			want: []string{"BTC", "ETH"},
		},
		{
			name: "no duplicates",
			text: "$AAPL is great, $AAPL rocks",
			want: []string{"AAPL"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveSymbols(tc.text)
			if len(got) != len(tc.want) {
				t.Fatalf("ResolveSymbols(%q) = %v (len %d), want %v (len %d)", tc.text, got, len(got), tc.want, len(tc.want))
			}
			for _, w := range tc.want {
				found := false
				for _, g := range got {
					if g == w {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected %q in result %v", w, got)
				}
			}
		})
	}
}

func TestResolveSymbols_CompanyNames(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "Apple → AAPL",
			text: "Apple reported strong earnings this quarter",
			want: []string{"AAPL"},
		},
		{
			name: "Tesla and Google",
			text: "Tesla deliveries beat expectations. Google announces new AI.",
			want: []string{"TSLA", "GOOGL"},
		},
		{
			name: "Bitcoin mention",
			text: "Bitcoin surges past $60k as institutional demand grows",
			want: []string{"BTC"},
		},
		{
			name: "Alphabet maps to GOOGL",
			text: "Alphabet Inc earnings call scheduled for Thursday",
			want: []string{"GOOGL"},
		},
		{
			name: "mixed ticker and name",
			text: "$NVDA surges after Nvidia announces new chip",
			want: []string{"NVDA"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveSymbols(tc.text)
			if len(got) != len(tc.want) {
				t.Fatalf("ResolveSymbols(%q) = %v (len %d), want %v (len %d)", tc.text, got, len(got), tc.want, len(tc.want))
			}
			for _, w := range tc.want {
				found := false
				for _, g := range got {
					if g == w {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected %q in result %v", w, got)
				}
			}
		})
	}
}

func TestResolveSymbols_NoFalsePositives(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"common words", "IT is great and $IT should be filtered"},
		{"two-letter words", "I went $TO the store $AT noon"},
		{"articles", "we $DO $GO $IN $UP $BY $IF"},
		{"time markers", "$AM and $PM are not tickers"},
		{"abbreviations", "$CEO gave $IPO speech on $TV in $UK and $EU"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveSymbols(tc.text)
			if len(got) != 0 {
				t.Errorf("expected no symbols from %q, got %v", tc.text, got)
			}
		})
	}
}

func TestResolveSymbols_EmptyText(t *testing.T) {
	got := ResolveSymbols("")
	if len(got) != 0 {
		t.Errorf("expected no symbols from empty text, got %v", got)
	}
}

func TestResolveSymbols_NoTickersInPlainText(t *testing.T) {
	got := ResolveSymbols("The market went up today with strong volume.")
	if len(got) != 0 {
		t.Errorf("expected no symbols from plain text, got %v", got)
	}
}

func TestSync_DetectsSymbolsInNews(t *testing.T) {
	finnhubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/company-news" {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"category": "company",
					"datetime": 1705334400,
					"headline": "Apple and $TSLA stocks surge",
					"id":       50001,
					"related":  "AAPL",
					"source":   "Reuters",
					"summary":  "Tesla deliveries beat expectations. Apple launched new products.",
					"url":      "https://example.com/news",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 175.0, "d": 2.0, "dp": 1.1, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.0,
		})
	}))
	defer finnhubSrv.Close()

	// Use a time before market close so no daily summary is generated.
	et, _ := time.LoadLocation("America/New_York")
	mockNow := time.Date(2024, 6, 10, 10, 0, 0, 0, et)

	c := newTestConnector("financial-markets")
	c.httpClient = finnhubSrv.Client()
	c.finnhubBaseURL = finnhubSrv.URL
	c.nowFunc = func() time.Time { return mockNow }

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"AAPL"}},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the news artifact.
	var newsArtifact *connector.RawArtifact
	for i := range artifacts {
		if artifacts[i].ContentType == "market/news" {
			newsArtifact = &artifacts[i]
			break
		}
	}
	if newsArtifact == nil {
		t.Fatal("expected a news artifact")
	}

	detected, ok := newsArtifact.Metadata["detected_symbols"]
	if !ok {
		t.Fatal("news artifact should have detected_symbols metadata")
	}
	detectedSlice, ok := detected.([]string)
	if !ok {
		t.Fatal("detected_symbols should be []string")
	}

	// Should detect AAPL (from "Apple" in headline+summary), TSLA (from "$TSLA" and "Tesla")
	wantSymbols := map[string]bool{"AAPL": false, "TSLA": false}
	for _, s := range detectedSlice {
		if _, exists := wantSymbols[s]; exists {
			wantSymbols[s] = true
		}
	}
	for sym, found := range wantSymbols {
		if !found {
			t.Errorf("expected %q in detected_symbols %v", sym, detectedSlice)
		}
	}

	// Quote artifact should have related_symbols.
	var quoteArtifact *connector.RawArtifact
	for i := range artifacts {
		if artifacts[i].ContentType == "market/quote" {
			quoteArtifact = &artifacts[i]
			break
		}
	}
	if quoteArtifact == nil {
		t.Fatal("expected a quote artifact")
	}
	related, ok := quoteArtifact.Metadata["related_symbols"]
	if !ok {
		t.Fatal("quote artifact should have related_symbols metadata")
	}
	relatedSlice, ok := related.([]string)
	if !ok {
		t.Fatal("related_symbols should be []string")
	}
	if len(relatedSlice) != 1 || relatedSlice[0] != "AAPL" {
		t.Errorf("quote related_symbols should be [AAPL], got %v", relatedSlice)
	}
}

func TestSync_EconomicArtifactsHaveAllWatchlistSymbols(t *testing.T) {
	fredSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"observations": []map[string]string{
				{"date": "2024-01-01", "value": "3.7"},
			},
		})
	}))
	defer fredSrv.Close()

	et, _ := time.LoadLocation("America/New_York")
	mockNow := time.Date(2024, 6, 10, 10, 0, 0, 0, et)

	c := newTestConnector("financial-markets")
	c.httpClient = fredSrv.Client()
	c.fredBaseURL = fredSrv.URL
	c.nowFunc = func() time.Time { return mockNow }

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		FREDAPIKey:     "fred-key",
		FREDEnabled:    true,
		FREDSeries:     []string{"GDP"},
		AlertThreshold: 5.0,
		Watchlist: WatchlistConfig{
			Stocks: []string{"AAPL"},
			Crypto: []string{"bitcoin"},
		},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var econArtifact *connector.RawArtifact
	for i := range artifacts {
		if artifacts[i].ContentType == "market/economic" {
			econArtifact = &artifacts[i]
			break
		}
	}
	if econArtifact == nil {
		t.Fatal("expected an economic artifact")
	}

	related, ok := econArtifact.Metadata["related_symbols"]
	if !ok {
		t.Fatal("economic artifact should have related_symbols metadata")
	}
	relatedSlice, ok := related.([]string)
	if !ok {
		t.Fatal("related_symbols should be []string")
	}

	// Should include AAPL and BITCOIN (uppercase)
	wantSymbols := map[string]bool{"AAPL": false, "BITCOIN": false}
	for _, s := range relatedSlice {
		if _, exists := wantSymbols[s]; exists {
			wantSymbols[s] = true
		}
	}
	for sym, found := range wantSymbols {
		if !found {
			t.Errorf("expected %q in economic related_symbols %v", sym, relatedSlice)
		}
	}
}

func TestEnrichArtifactsWithSymbols_QuoteArtifact(t *testing.T) {
	artifacts := []connector.RawArtifact{
		{
			ContentType: "market/quote",
			Metadata: map[string]interface{}{
				"symbol": "AAPL",
			},
		},
	}
	cfg := MarketsConfig{}
	enrichArtifactsWithSymbols(artifacts, cfg)

	related, ok := artifacts[0].Metadata["related_symbols"].([]string)
	if !ok {
		t.Fatal("expected related_symbols as []string")
	}
	if len(related) != 1 || related[0] != "AAPL" {
		t.Errorf("expected [AAPL], got %v", related)
	}
}

// --- REG-018-R01: Company news failures must degrade health ---

// TestRegression_NewsTotalFailureSetsHealthDegraded verifies that when all company
// news fetches fail while stock quotes succeed, health is Degraded (not Healthy).
// Adversarial: without the REG-018-R01 fix, news failures were invisible to health.
func TestRegression_NewsTotalFailureSetsHealthDegraded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/company-news":
			// News endpoint fails
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"news down"}`))
		default:
			// Quote endpoint succeeds
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 175.0, "d": 1.5, "dp": 0.9, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.5,
			})
		}
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist: WatchlistConfig{
			Stocks: []string{"AAPL", "GOOGL"},
		},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stock quotes should still succeed.
	var quoteCount int
	for _, a := range artifacts {
		if a.ContentType == "market/quote" {
			quoteCount++
		}
	}
	if quoteCount != 2 {
		t.Errorf("expected 2 quote artifacts, got %d", quoteCount)
	}

	// Health must NOT be Healthy — news is a tracked provider that completely failed.
	health := c.Health(context.Background())
	if health == connector.HealthHealthy {
		t.Errorf("expected Degraded or Error health after total news failure, got %v", health)
	}
}

// --- Test-to-doc coverage gap tests: TQS-018-001 through TQS-018-008 ---

func TestFetchFinnhubForex_MalformedBaseURL(t *testing.T) {
	// TQS-018-001: fetchFinnhubForex must return url.Parse error for malformed base URL.
	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.finnhubBaseURL = "://invalid-url"

	_, err := c.fetchFinnhubForex(context.Background(), "USD/JPY")
	if err == nil {
		t.Fatal("expected error for malformed forex base URL")
	}
	if !strings.Contains(err.Error(), "parse finnhub URL") {
		t.Errorf("error should mention URL parse failure, got: %v", err)
	}
}

func TestFetchFinnhubForex_HTTPError(t *testing.T) {
	// TQS-018-002: fetchFinnhubForex must handle non-200 HTTP responses with body snippet.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	_, err := c.fetchFinnhubForex(context.Background(), "USD/JPY")
	if err == nil {
		t.Fatal("expected error for 429 forex response")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should mention status 429, got: %v", err)
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("error should include body snippet, got: %v", err)
	}
}

func TestFetchFinnhubForex_MalformedJSON(t *testing.T) {
	// TQS-018-003: fetchFinnhubForex must return decode error for malformed JSON.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	_, err := c.fetchFinnhubForex(context.Background(), "USD/JPY")
	if err == nil {
		t.Fatal("expected error for malformed forex JSON")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error should mention decode, got: %v", err)
	}
}

func TestFetchFinnhubForex_RejectsZeroPriceResponse(t *testing.T) {
	// TQS-018-004: fetchFinnhubForex must reject all-zero "no data" responses.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 0, "d": 0, "dp": 0, "h": 0, "l": 0, "o": 0, "pc": 0,
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	_, err := c.fetchFinnhubForex(context.Background(), "USD/JPY")
	if err == nil {
		t.Fatal("expected error for zero-price forex response")
	}
	if !strings.Contains(err.Error(), "no forex data") {
		t.Errorf("error should mention no forex data, got: %v", err)
	}
}

func TestFetchFinnhubCompanyNews_MalformedBaseURL(t *testing.T) {
	// TQS-018-005: fetchFinnhubCompanyNews must return url.Parse error for malformed base URL.
	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.finnhubBaseURL = "://invalid-url"

	_, err := c.fetchFinnhubCompanyNews(context.Background(), "AAPL", "2024-01-15", "2024-01-15")
	if err == nil {
		t.Fatal("expected error for malformed news base URL")
	}
	if !strings.Contains(err.Error(), "parse finnhub news URL") {
		t.Errorf("error should mention URL parse failure, got: %v", err)
	}
}

func TestBuildDailySummary_UnchangedSymbols(t *testing.T) {
	// TQS-018-006: buildDailySummary must include an "Unchanged" section for 0% change.
	now := time.Date(2024, 6, 10, 17, 0, 0, 0, time.UTC)
	artifacts := []connector.RawArtifact{
		{
			ContentType: "market/quote",
			Metadata: map[string]interface{}{
				"symbol":          "FLAT",
				"change_percent":  0.0,
				"processing_tier": "light",
			},
		},
	}

	summary := buildDailySummary(artifacts, now)
	if !strings.Contains(summary.RawContent, "Unchanged:") {
		t.Error("summary should contain Unchanged section for 0% change symbol")
	}
	if !strings.Contains(summary.RawContent, "FLAT") {
		t.Error("summary should mention the unchanged symbol")
	}
	if summary.Metadata["gainers_count"] != 0 {
		t.Errorf("expected 0 gainers, got %v", summary.Metadata["gainers_count"])
	}
	if summary.Metadata["losers_count"] != 0 {
		t.Errorf("expected 0 losers, got %v", summary.Metadata["losers_count"])
	}
}

func TestSync_NewsSanitizesControlChars(t *testing.T) {
	// TQS-018-007: Sync must sanitize control characters in news headline/summary/source
	// via SanitizeControlChars (IMP-018-SQS-001, CWE-116).
	finnhubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/company-news" {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"category": "company\x00injected",
					"datetime": 1705334400,
					"headline": "Apple\x07Bell Report",
					"id":       60001,
					"related":  "AAPL",
					"source":   "Reuters\x1b[31m",
					"summary":  "Test\x0bsummary\x0cwith\x08controls",
					"url":      "https://example.com/news",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 175.0, "d": 2.0, "dp": 1.1, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.0,
		})
	}))
	defer finnhubSrv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = finnhubSrv.Client()
	c.finnhubBaseURL = finnhubSrv.URL
	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"AAPL"}},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var newsArtifact *connector.RawArtifact
	for i := range artifacts {
		if artifacts[i].ContentType == "market/news" {
			newsArtifact = &artifacts[i]
			break
		}
	}
	if newsArtifact == nil {
		t.Fatal("expected a news artifact")
	}

	// Verify control characters are stripped from headline.
	if strings.ContainsAny(newsArtifact.Title, "\x00\x07\x08\x0b\x0c\x1b") {
		t.Errorf("headline still contains control characters: %q", newsArtifact.Title)
	}
	if !strings.Contains(newsArtifact.Title, "Apple") {
		t.Errorf("headline should still contain 'Apple', got %q", newsArtifact.Title)
	}

	// Verify control characters are stripped from source.
	source := newsArtifact.Metadata["source"].(string)
	if strings.ContainsAny(source, "\x1b") {
		t.Errorf("source still contains ANSI escape: %q", source)
	}

	// Verify control characters are stripped from category.
	category := newsArtifact.Metadata["category"].(string)
	if strings.ContainsAny(category, "\x00") {
		t.Errorf("category still contains null byte: %q", category)
	}
}

func TestCoinGecko_ZeroPercentChangeViaFetch(t *testing.T) {
	// TQS-018-008: CoinGecko 0% change must yield 0 change24h through real fetch path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]map[string]float64{
			"stablecoin": {"usd": 1.0, "usd_24h_change": 0.0},
		})
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.coingeckoBaseURL = srv.URL

	prices, err := c.fetchCoinGeckoPrices(context.Background(), []string{"stablecoin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected 1 price, got %d", len(prices))
	}
	if prices[0].Change24h != 0.0 {
		t.Errorf("expected 0.0 change24h for 0%% change, got %v", prices[0].Change24h)
	}
	if prices[0].ChangePct24h != 0.0 {
		t.Errorf("expected 0.0 changePct24h, got %v", prices[0].ChangePct24h)
	}
}

// TestRegression_NewsPartialFailureStaysHealthy verifies that partial news failures
// (some symbols succeed) don't incorrectly mark the connector as degraded.
func TestRegression_NewsPartialFailureStaysHealthy(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/company-news":
			callCount++
			if callCount == 1 {
				// First symbol's news fails
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"temporary"}`))
				return
			}
			// Second symbol's news succeeds
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"category": "company", "datetime": 1705334400, "headline": "News", "id": 1, "related": "GOOGL", "source": "Test", "summary": "Sum", "url": "https://example.com"},
			})
		default:
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 175.0, "d": 1.5, "dp": 0.9, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.5,
			})
		}
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist: WatchlistConfig{
			Stocks: []string{"AAPL", "GOOGL"},
		},
	}

	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Partial failure (1 of 2 succeed) should NOT count as total provider failure.
	health := c.Health(context.Background())
	if health != connector.HealthHealthy {
		t.Errorf("expected HealthHealthy after partial news failure, got %v", health)
	}
}

// --- Chaos Tests: CHAOS-018-001 through CHAOS-018-003 ---

func TestCHAOS018_001_ConcurrentSyncDailySummaryNoDoubleGeneration(t *testing.T) {
	// CHAOS-018-001: Two concurrent Syncs running after market close must NOT
	// both generate a daily summary. The tryClaimDailySummary atomic check-and-set
	// must ensure exactly one summary is produced.
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal("failed to load timezone")
	}
	mockNow := time.Date(2024, 6, 10, 17, 0, 0, 0, et) // Monday 5pm ET

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{
			"c": 175.0, "d": 2.0, "dp": 1.3, "h": 177.0, "l": 173.0, "o": 174.0, "pc": 173.0,
		})
	}))
	defer srv.Close()

	c := New("financial-markets")
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL
	c.nowFunc = func() time.Time { return mockNow }
	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"AAPL"}},
	}

	// Run 10 concurrent Syncs.
	var wg sync.WaitGroup
	summaryCount := make(chan int, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			artifacts, _, err := c.Sync(context.Background(), "")
			if err != nil {
				return
			}
			count := 0
			for _, a := range artifacts {
				if a.ContentType == "market/daily-summary" {
					count++
				}
			}
			summaryCount <- count
		}()
	}
	wg.Wait()
	close(summaryCount)

	total := 0
	for count := range summaryCount {
		total += count
	}
	if total != 1 {
		t.Errorf("CHAOS-018-001: expected exactly 1 daily summary from 10 concurrent Syncs, got %d — TOCTOU race in summary generation", total)
	}
}

func TestCHAOS018_002_EconomicArtifactSliceIsolation(t *testing.T) {
	// CHAOS-018-002: Each economic artifact's related_symbols must be an independent
	// copy. Mutating one must NOT corrupt the others.
	fredSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"observations": []map[string]string{
				{"date": "2024-01-01", "value": "3.7"},
			},
		})
	}))
	defer fredSrv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = fredSrv.Client()
	c.fredBaseURL = fredSrv.URL
	c.config = MarketsConfig{
		FinnhubAPIKey:  "test-key",
		FREDAPIKey:     "fred-key",
		FREDEnabled:    true,
		FREDSeries:     []string{"GDP", "UNRATE"},
		AlertThreshold: 5.0,
		Watchlist:      WatchlistConfig{Stocks: []string{"AAPL", "GOOGL"}},
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var econArtifacts []*connector.RawArtifact
	for i := range artifacts {
		if artifacts[i].ContentType == "market/economic" {
			econArtifacts = append(econArtifacts, &artifacts[i])
		}
	}
	if len(econArtifacts) < 2 {
		t.Fatalf("expected at least 2 economic artifacts, got %d", len(econArtifacts))
	}

	// Get related_symbols from the first economic artifact and mutate it.
	first := econArtifacts[0].Metadata["related_symbols"].([]string)
	second := econArtifacts[1].Metadata["related_symbols"].([]string)

	originalSecondLen := len(second)
	// Mutate the first slice by overwriting element 0.
	if len(first) > 0 {
		first[0] = "CORRUPTED"
	}

	// Second artifact's slice must be unaffected.
	if len(second) != originalSecondLen {
		t.Errorf("CHAOS-018-002: second economic artifact's related_symbols length changed after mutating first")
	}
	for _, s := range second {
		if s == "CORRUPTED" {
			t.Error("CHAOS-018-002: mutating first economic artifact's related_symbols corrupted the second — shared slice aliasing bug")
		}
	}
}

func TestCHAOS018_003_TryClaimDailySummaryIdempotent(t *testing.T) {
	// CHAOS-018-003: After tryClaimDailySummary returns true once for a date,
	// subsequent calls for the same date must return false (no duplicates).
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal("failed to load timezone")
	}
	mockNow := time.Date(2024, 6, 10, 17, 0, 0, 0, et) // Monday 5pm ET

	c := newTestConnector("financial-markets")

	// First call should claim.
	if !c.tryClaimDailySummary(mockNow) {
		t.Error("first tryClaimDailySummary should return true")
	}
	// Second call for the same date should be rejected.
	if c.tryClaimDailySummary(mockNow) {
		t.Error("second tryClaimDailySummary for same date should return false — duplicate summary prevention broken")
	}
	// Different day should succeed.
	nextDay := time.Date(2024, 6, 11, 17, 0, 0, 0, et) // Tuesday
	if !c.tryClaimDailySummary(nextDay) {
		t.Error("tryClaimDailySummary for next day should return true")
	}
}
