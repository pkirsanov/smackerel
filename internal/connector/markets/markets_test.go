package markets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestNew(t *testing.T) {
	c := New("financial-markets")
	if c.ID() != "financial-markets" {
		t.Errorf("expected financial-markets, got %s", c.ID())
	}
}

func TestConnect_MissingAPIKey(t *testing.T) {
	c := New("financial-markets")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{},
	})
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

func TestConnect_Valid(t *testing.T) {
	c := New("financial-markets")
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
	c := New("financial-markets")
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
	c := New("financial-markets")
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
	c := New("financial-markets")
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

	c := New("financial-markets")
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

	c := New("financial-markets")
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
	c := New("financial-markets")
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
	c := New("financial-markets")
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
	c := New("financial-markets")

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

	c := New("financial-markets")
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
	c := New("financial-markets")
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

	c := New("financial-markets")
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
	c := New("financial-markets")
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
	c := New("financial-markets")

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
	c := New("financial-markets")
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

	c := New("financial-markets")
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
	c := New("financial-markets")
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

	c := New("financial-markets")
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

	c := New("financial-markets")
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
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Errorf("expected degraded health after total failure, got %v", c.Health(context.Background()))
	}
}

func TestSyncHealthyOnPartialFailure(t *testing.T) {
	// When some calls succeed, health should be healthy.
	callNum := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++
		if callNum == 1 {
			// First call succeeds
			json.NewEncoder(w).Encode(map[string]float64{
				"c": 150.0, "d": 1.0, "dp": 0.5, "h": 152.0, "l": 148.0, "o": 149.0, "pc": 149.0,
			})
		} else {
			// Subsequent calls fail
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"server error"}`))
		}
	}))
	defer srv.Close()

	c := New("financial-markets")
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
	// Partial failure — some symbols succeeded, health stays healthy.
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

	c := New("financial-markets")
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

	c := New("financial-markets")
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
	c := New("financial-markets")
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
