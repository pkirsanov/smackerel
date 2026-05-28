// BUG-022-003 — markets connector 429/Retry-After recovery regression.
// SCN-422-003-H: a 429 with Retry-After must be retried by the shared helper.
package markets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestFetchFinnhubQuote_HonorsRetryAfter(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"c":150.0,"h":151.0,"l":149.0,"pc":148.0}`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.config.FinnhubAPIKey = "test-key"
	c.httpClient = srv.Client()
	c.finnhubBaseURL = srv.URL

	quote, err := c.fetchFinnhubQuote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("expected recovered fetch after 429+Retry-After, got: %v", err)
	}
	if quote == nil || quote.CurrentPrice != 150.0 {
		t.Fatalf("unexpected quote: %+v", quote)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("hits = %d, want 2 (proves DoWithRetry retried)", got)
	}
}

func TestFetchCoinGeckoPrices_HonorsRetryAfter(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"bitcoin":{"usd":30000,"usd_24h_change":1.5}}`))
	}))
	defer srv.Close()

	c := newTestConnector("financial-markets")
	c.httpClient = srv.Client()
	c.coingeckoBaseURL = srv.URL

	prices, err := c.fetchCoinGeckoPrices(context.Background(), []string{"bitcoin"})
	if err != nil {
		t.Fatalf("expected recovered fetch, got: %v", err)
	}
	if len(prices) != 1 || prices[0].ID != "bitcoin" {
		t.Fatalf("unexpected prices: %+v", prices)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("hits = %d, want 2", got)
	}
}
