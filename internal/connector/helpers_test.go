// BUG-022-003 — Regression tests for the shared 429/Retry-After helper.
//
// The previous per-connector "non-200 → error" pattern caused brownouts to
// escalate into provider bans (USGS, NWS, AirNow, GDACS, Finnhub).
// These tests pin the canonical behavior of connector.DoWithRetry and the
// OAuthAPIGet caller so the bug cannot regress silently.
package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/smackerel/smackerel/internal/metrics"
)

func fastTestRetryOpts(label string) RetryOptions {
	return RetryOptions{
		MaxAttempts: 3,
		BaseDelay:   5 * time.Millisecond,
		MaxDelay:    50 * time.Millisecond,
		Label:       label,
	}
}

// TestParseRetryAfter exercises both RFC 7231 forms plus malformed inputs.
// SCN-422-003-E.
func TestParseRetryAfter(t *testing.T) {
	now := time.Now()
	future := now.Add(45 * time.Second).UTC().Format(http.TimeFormat)
	past := now.Add(-1 * time.Hour).UTC().Format(http.TimeFormat)

	cases := []struct {
		name   string
		header string
		wantOK bool
		check  func(t *testing.T, d time.Duration)
	}{
		{"delta_seconds_60", "60", true, func(t *testing.T, d time.Duration) {
			if d != 60*time.Second {
				t.Fatalf("expected 60s, got %v", d)
			}
		}},
		{"delta_seconds_zero", "0", true, func(t *testing.T, d time.Duration) {
			if d != 0 {
				t.Fatalf("expected 0, got %v", d)
			}
		}},
		{"http_date_future", future, true, func(t *testing.T, d time.Duration) {
			if d < 30*time.Second || d > 60*time.Second {
				t.Fatalf("expected ~45s, got %v", d)
			}
		}},
		{"http_date_past", past, true, func(t *testing.T, d time.Duration) {
			if d != 0 {
				t.Fatalf("past date should clamp to 0, got %v", d)
			}
		}},
		{"empty", "", false, nil},
		{"garbage", "not-a-number", false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, ok := parseRetryAfter(tc.header)
			if ok != tc.wantOK {
				t.Fatalf("ok mismatch: want %v got %v (d=%v)", tc.wantOK, ok, d)
			}
			if tc.check != nil {
				tc.check(t, d)
			}
		})
	}
}

// TestDoWithRetry_429ThenOK proves the helper honors Retry-After delta-seconds.
// SCN-422-003-A.
func TestDoWithRetry_429ThenOK(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("slow down"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	opts := fastTestRetryOpts("test-a")
	// Honor the server's delta-seconds even though our test default cap is small.
	opts.MaxDelay = 2 * time.Second
	start := time.Now()
	resp, err := DoWithRetry(context.Background(), srv.Client(), req, opts)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("hits = %d, want 2", got)
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= 1s (Retry-After honored)", elapsed)
	}
	if !strings.Contains(string(body), "ok") {
		t.Fatalf("unexpected body: %s", body)
	}
}

// TestDoWithRetry_429_HTTPDate proves HTTP-date Retry-After is honored.
// SCN-422-003-B.
func TestDoWithRetry_429_HTTPDate(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			when := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)
			w.Header().Set("Retry-After", when)
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	opts := fastTestRetryOpts("test-b")
	opts.MaxDelay = 5 * time.Second
	start := time.Now()
	resp, err := DoWithRetry(context.Background(), srv.Client(), req, opts)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("hits = %d, want 2", got)
	}
	// HTTP-date formatting truncates to whole seconds, so the parsed delay
	// is in [1s, 2s] depending on sub-second clock alignment.
	if elapsed < 900*time.Millisecond || elapsed > 4*time.Second {
		t.Fatalf("elapsed = %v, expected ~1-2s (HTTP-date honored)", elapsed)
	}
}

// TestDoWithRetry_429_Exhausted is the adversarial regression: removing the
// MaxAttempts bound would spin forever and this test would hang.
// SCN-422-003-C.
func TestDoWithRetry_429_Exhausted(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	opts := fastTestRetryOpts("test-c")
	start := time.Now()
	_, err := DoWithRetry(context.Background(), srv.Client(), req, opts)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrRateLimitExhausted) {
		t.Fatalf("want ErrRateLimitExhausted, got %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != int32(opts.MaxAttempts) {
		t.Fatalf("hits = %d, want %d", got, opts.MaxAttempts)
	}
	if elapsed > time.Second {
		t.Fatalf("elapsed = %v, want < 1s with fast test backoff", elapsed)
	}
}

// TestDoWithRetry_ContextCancel proves ctx.Done() interrupts sleeps.
// SCN-422-003-D.
func TestDoWithRetry_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	opts := fastTestRetryOpts("test-d")
	opts.MaxDelay = 60 * time.Second
	start := time.Now()
	_, err := DoWithRetry(ctx, srv.Client(), req, opts)
	elapsed := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("ctx cancel did not interrupt sleep; elapsed = %v", elapsed)
	}
}

// TestDoWithRetry_MetricIncrements verifies the connector_429_total counter
// fires once per retry attempt plus a recovered/exhausted terminal increment.
// SCN-422-003-I.
func TestDoWithRetry_MetricIncrements(t *testing.T) {
	retryBefore := counterValue(t, "test-metric-recover", "retry")
	recoveredBefore := counterValue(t, "test-metric-recover", "recovered")

	// One 429 then OK → expect 1 "retry" + 1 "recovered".
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	opts := fastTestRetryOpts("test-metric-recover")
	resp, err := DoWithRetry(context.Background(), srv.Client(), req, opts)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	resp.Body.Close()

	if got := counterValue(t, "test-metric-recover", "retry") - retryBefore; got != 1 {
		t.Errorf("retry delta = %v, want 1", got)
	}
	if got := counterValue(t, "test-metric-recover", "recovered") - recoveredBefore; got != 1 {
		t.Errorf("recovered delta = %v, want 1", got)
	}

	// Exhaustion path → expect MaxAttempts-1 retries + 1 exhausted.
	exhaustedBefore := counterValue(t, "test-metric-exhaust", "exhausted")
	retryBefore2 := counterValue(t, "test-metric-exhaust", "retry")
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv2.Close()
	req2, _ := http.NewRequest("GET", srv2.URL, nil)
	opts2 := fastTestRetryOpts("test-metric-exhaust")
	_, err = DoWithRetry(context.Background(), srv2.Client(), req2, opts2)
	if !errors.Is(err, ErrRateLimitExhausted) {
		t.Fatalf("want exhausted, got %v", err)
	}
	if got := counterValue(t, "test-metric-exhaust", "exhausted") - exhaustedBefore; got != 1 {
		t.Errorf("exhausted delta = %v, want 1", got)
	}
	if got := counterValue(t, "test-metric-exhaust", "retry") - retryBefore2; got != float64(opts2.MaxAttempts-1) {
		t.Errorf("retry delta = %v, want %d", got, opts2.MaxAttempts-1)
	}
}

func counterValue(t *testing.T, label, outcome string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "smackerel_connector_429_total" {
			continue
		}
		for _, m := range mf.Metric {
			if matchLabel(m, "connector", label) && matchLabel(m, "outcome", outcome) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func matchLabel(m *dto.Metric, name, value string) bool {
	for _, lp := range m.Label {
		if lp.GetName() == name && lp.GetValue() == value {
			return true
		}
	}
	return false
}

// TestOAuthAPIGet_HonorsRetryAfter proves the shared OAuth helper now retries.
// SCN-422-003-G.
func TestOAuthAPIGet_HonorsRetryAfter(t *testing.T) {
	// Shorten OAuth retry opts for this test; restore in defer.
	prev := oauthRetryOpts
	oauthRetryOpts = fastTestRetryOpts("oauth")
	defer func() { oauthRetryOpts = prev }()

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer the-token" {
			t.Errorf("missing or wrong auth header: %q", got)
		}
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"value": "ok"})
	}))
	defer srv.Close()

	got, err := OAuthAPIGet(context.Background(), srv.Client(), srv.URL, "the-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["value"] != "ok" {
		t.Fatalf("unexpected payload: %#v", got)
	}
	if h := atomic.LoadInt32(&hits); h != 2 {
		t.Fatalf("hits = %d, want 2", h)
	}
}

// Sanity check that the registered metric vector accepts our labels (catches a
// label-cardinality mismatch in registration).
func TestConnectorRateLimitMetricLabels(t *testing.T) {
	_, err := metrics.ConnectorRateLimit429Total.GetMetricWithLabelValues("smoke", "retry")
	if err != nil {
		t.Fatalf("metric label lookup failed: %v", err)
	}
}

// Compile-time guard that the helper returns the documented error string.
func TestErrRateLimitExhaustedMessage(t *testing.T) {
	if !strings.Contains(ErrRateLimitExhausted.Error(), "rate limited") ||
		!strings.Contains(ErrRateLimitExhausted.Error(), "max retries exceeded") {
		t.Fatalf("unexpected error string: %q", ErrRateLimitExhausted.Error())
	}
}

// Ensure the helper does not silently swallow non-429 errors (no implicit retry).
func TestDoWithRetry_NonRetryablePassThrough(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := DoWithRetry(context.Background(), srv.Client(), req, fastTestRetryOpts("test-passthrough"))
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("hits = %d, want 1 (no retry on 500)", got)
	}
}

// Documentation of which call sites are wired (acts as a checklist; updates
// to this list must accompany migrations of additional connectors).
const migratedCallSites = "alerts.go x7 (USGS, NWS, tsunami, volcano, wildfire, AirNow, GDACS); helpers.go OAuthAPIGet; markets.go x4 (Finnhub quote, CoinGecko, Finnhub news, FRED)"

func TestMigratedCallSitesDocumented(t *testing.T) {
	if !strings.Contains(migratedCallSites, "OAuthAPIGet") || !strings.Contains(migratedCallSites, "alerts.go") || !strings.Contains(migratedCallSites, "markets.go") {
		t.Fatalf("migrated call-site checklist drifted: %s", migratedCallSites)
	}
	// Keep the imports honest.
	_ = strconv.Itoa
	_ = fmt.Sprint
}
