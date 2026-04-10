package markets

import (
	"context"
	"strings"
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

func TestAllowCall_RateLimit(t *testing.T) {
	c := New("financial-markets")
	c.config.FinnhubAPIKey = "test"

	// Should allow first call
	if !c.allowCall("finnhub") {
		t.Error("first call should be allowed")
	}

	// Record 55 calls
	for i := 0; i < 55; i++ {
		c.recordCall("finnhub")
	}

	// Should deny 56th call (limit is 55)
	if c.allowCall("finnhub") {
		t.Error("should deny call at rate limit")
	}

	// Unknown provider always allowed
	if !c.allowCall("unknown") {
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
