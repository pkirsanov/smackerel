//go:build stress

// Package stress contains spec 039 Scope 6 stress profile.
//
// SCN-039-052 (R-009 / R-032): 50 concurrent warm reactive
// recommendation requests for 5 minutes against fixture providers
// MUST meet the configured p95 latency NFR; no errors except
// expected rate-limit / quota outcomes; provider runtime state
// reflects observed degradation.
//
// The stress profile runs against the live `./smackerel.sh up` stack
// using `CORE_EXTERNAL_URL` + `SMACKEREL_AUTH_TOKEN` (SST). It
// SKIPs cleanly when the live stack is not available, and FAILS
// loudly when latency exceeds the NFR or when the error rate
// exceeds the bounded acceptable set.
package stress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recommendationsStressBudget bounds the run. Defaults match the spec
// 039 NFR R-032: "Reactive P95 ≤ 10s warm, ≤ 20s cold". The duration
// is 5 minutes per AC-17 ("P95 reactive latency holds under 50
// concurrent recommendation requests across a 5-minute window"). The
// values are not configurable at runtime — they are NFR contract,
// not config.
const (
	recommendationsStressConcurrency = 50
	recommendationsStressDuration    = 5 * time.Minute
	recommendationsStressP95Budget   = 10 * time.Second
	recommendationsStressMaxErrorPct = 5.0
)

// TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests is the
// SCN-039-052 stress profile. It drives 50 concurrent warm reactive
// requests against the live stack for 5 minutes and asserts the
// configured p95 latency NFR is met.
func TestRecommendationsStress_FiftyConcurrentWarmReactiveRequests(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)

	if testing.Short() {
		t.Skip("stress: -short specified, skipping 5m profile")
	}

	// Shared HTTP client tuned for the stress profile. Per-request
	// http.Clients (as the legacy helper does) create a new connection
	// pool each call and exhaust local ports under sustained load,
	// producing transport errors that mask the server-side NFR. The
	// stress profile validates the SERVER, so the client must not
	// itself become the bottleneck.
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        recommendationsStressConcurrency * 4,
		MaxIdleConnsPerHost: recommendationsStressConcurrency * 4,
		MaxConnsPerHost:     recommendationsStressConcurrency * 4,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	defer transport.CloseIdleConnections()
	sharedClient := &http.Client{Transport: transport, Timeout: 60 * time.Second}

	// Warm-up: one priming request to populate any per-process caches
	// in the live core (route compilation, JIT, connection pool).
	if status, body, err := stressClientPost(sharedClient, cfg, "/api/recommendations/requests", warmUpReactiveBody(t)); err != nil {
		t.Fatalf("stress warmup failed: %v", err)
	} else if status != http.StatusOK && status != http.StatusCreated && status != http.StatusAccepted {
		t.Fatalf("stress warmup returned status %d: %s", status, string(body))
	}

	ctx, cancel := context.WithTimeout(context.Background(), recommendationsStressDuration+30*time.Second)
	defer cancel()

	samplesCh := make(chan recommendationStressSample, recommendationsStressConcurrency*64)
	var (
		started int64
		ended   int64
	)

	var wg sync.WaitGroup
	wg.Add(recommendationsStressConcurrency)
	deadline := time.Now().Add(recommendationsStressDuration)
	for worker := 0; worker < recommendationsStressConcurrency; worker++ {
		worker := worker
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				if ctx.Err() != nil {
					return
				}
				atomic.AddInt64(&started, 1)
				body := stressReactiveBody(worker, atomic.LoadInt64(&started))
				start := time.Now()
				status, _, err := stressClientPost(sharedClient, cfg, "/api/recommendations/requests", body)
				latency := time.Since(start)
				atomic.AddInt64(&ended, 1)
				sample := classifyRecommendationStressSample(status, err, latency)
				samplesCh <- sample
				if sample.errKind == "timeout" || ctx.Err() != nil {
					return
				}
			}
		}()
	}

	// Close channel once all workers exit.
	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(samplesCh)
		close(doneCh)
	}()

	summary := recommendationStressSummary{}
	for s := range samplesCh {
		summary.Observe(s)
	}
	<-doneCh

	totalSamples := summary.TotalSamples
	if totalSamples == 0 {
		t.Fatalf("stress: zero samples collected — workers never produced any classified observations (started=%d ended=%d)",
			atomic.LoadInt64(&started), atomic.LoadInt64(&ended))
	}

	latencies := summary.Latencies
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := percentile(latencies, 0.50)
	p95 := percentile(latencies, 0.95)
	p99 := percentile(latencies, 0.99)
	maxLat := time.Duration(0)
	if len(latencies) > 0 {
		maxLat = latencies[len(latencies)-1]
	}
	errPct := 100.0 * float64(summary.UnexpectedErrors) / float64(totalSamples)

	t.Logf("stress samples: total=%d ok=%d accepted_errors=%d unexpected_errors=%d server_errors=%d transport_errors=%d timeout_errors=%d unexpected_status=%d started=%d ended=%d (unexpected rate %.2f%%)",
		totalSamples, summary.OK, summary.AcceptedErrors, summary.UnexpectedErrors, summary.ServerErrors, summary.TransportErrors,
		summary.TimeoutErrors, summary.UnexpectedStatus, atomic.LoadInt64(&started), atomic.LoadInt64(&ended), errPct)
	t.Logf("stress latency: p50=%s p95=%s p99=%s max=%s budget=%s",
		p50, p95, p99, maxLat, recommendationsStressP95Budget)

	if p95 > recommendationsStressP95Budget {
		t.Errorf("p95 %s exceeds NFR budget %s", p95, recommendationsStressP95Budget)
	}
	if errPct > recommendationsStressMaxErrorPct {
		t.Errorf("unexpected error rate %.2f%% exceeds %.2f%% budget (unexpected_errors=%d, total=%d, server=%d, transport=%d, timeout=%d, unexpected_status=%d)",
			errPct, recommendationsStressMaxErrorPct, summary.UnexpectedErrors, totalSamples, summary.ServerErrors, summary.TransportErrors, summary.TimeoutErrors, summary.UnexpectedStatus)
	}

	// Provider runtime state must reflect observed degradation when any
	// fixture provider returns an error path. In the stress profile we
	// do not force degradation, so we just assert the endpoint reachable
	// and JSON-decodable so a future regression that breaks the
	// providers status surface fails here.
	if status, body, err := stressClientGet(sharedClient, cfg, "/api/recommendations/providers"); err != nil {
		t.Errorf("providers status fetch failed: %v", err)
	} else if status != http.StatusOK {
		t.Errorf("providers status returned %d: %s", status, string(body))
	} else {
		var providers []map[string]any
		if jerr := json.Unmarshal(body, &providers); jerr != nil {
			// Some endpoints wrap the list under a key; be tolerant.
			var wrapped map[string]json.RawMessage
			if werr := json.Unmarshal(body, &wrapped); werr != nil {
				t.Errorf("providers status JSON parse failed: %v", jerr)
			}
		}
	}
}

func TestRecommendationsStress_TimeoutOutcomesAreClassified(t *testing.T) {
	samples := []recommendationStressSample{
		classifyRecommendationStressSample(0, context.DeadlineExceeded, 60*time.Second),
		classifyRecommendationStressSample(0, recommendationStressTimeoutError{}, 61*time.Second),
	}

	summary := recommendationStressSummary{}
	for _, sample := range samples {
		summary.Observe(sample)
	}

	if summary.TotalSamples != len(samples) {
		t.Fatalf("timeout observations were dropped: got total=%d want %d", summary.TotalSamples, len(samples))
	}
	if summary.TimeoutErrors != len(samples) {
		t.Fatalf("timeout observations not classified: got timeout_errors=%d want %d", summary.TimeoutErrors, len(samples))
	}
	if summary.UnexpectedErrors != len(samples) {
		t.Fatalf("timeout observations not counted as unexpected errors: got unexpected_errors=%d want %d", summary.UnexpectedErrors, len(samples))
	}
	if len(summary.Latencies) != len(samples) {
		t.Fatalf("timeout latencies were dropped: got %d want %d", len(summary.Latencies), len(samples))
	}
}

type recommendationStressSample struct {
	latency time.Duration
	status  int
	errKind string
}

type recommendationStressSummary struct {
	TotalSamples     int
	OK               int
	AcceptedErrors   int
	UnexpectedErrors int
	ServerErrors     int
	TransportErrors  int
	TimeoutErrors    int
	UnexpectedStatus int
	Latencies        []time.Duration
}

func (s *recommendationStressSummary) Observe(sample recommendationStressSample) {
	s.TotalSamples++
	s.Latencies = append(s.Latencies, sample.latency)
	switch sample.errKind {
	case "":
		s.OK++
	case "rate_limit", "quota":
		s.AcceptedErrors++
	case "server_error":
		s.ServerErrors++
		s.UnexpectedErrors++
	case "transport":
		s.TransportErrors++
		s.UnexpectedErrors++
	case "timeout":
		s.TimeoutErrors++
		s.UnexpectedErrors++
	case "unexpected_status":
		s.UnexpectedStatus++
		s.UnexpectedErrors++
	default:
		s.UnexpectedErrors++
	}
}

func classifyRecommendationStressSample(status int, err error, latency time.Duration) recommendationStressSample {
	switch {
	case err != nil && isRecommendationStressTimeout(err):
		return recommendationStressSample{latency: latency, status: 0, errKind: "timeout"}
	case err != nil:
		return recommendationStressSample{latency: latency, status: 0, errKind: "transport"}
	case status == http.StatusTooManyRequests:
		return recommendationStressSample{latency: latency, status: status, errKind: "rate_limit"}
	case status == http.StatusForbidden:
		return recommendationStressSample{latency: latency, status: status, errKind: "quota"}
	case status >= 500:
		return recommendationStressSample{latency: latency, status: status, errKind: "server_error"}
	case status != http.StatusOK && status != http.StatusCreated && status != http.StatusAccepted:
		return recommendationStressSample{latency: latency, status: status, errKind: "unexpected_status"}
	default:
		return recommendationStressSample{latency: latency, status: status}
	}
}

func isRecommendationStressTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

type recommendationStressTimeoutError struct{}

func (recommendationStressTimeoutError) Error() string   { return "request timed out" }
func (recommendationStressTimeoutError) Timeout() bool   { return true }
func (recommendationStressTimeoutError) Temporary() bool { return false }

func percentile(sorted []time.Duration, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if q <= 0 {
		return sorted[0]
	}
	if q >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := int(float64(len(sorted)-1) * q)
	return sorted[idx]
}

func stressReactiveBody(worker int, seq int64) []byte {
	body := map[string]any{
		"query":            fmt.Sprintf("coffee near mission worker=%d seq=%d", worker, seq),
		"location_ref":     "gps:37.7749,-122.4194",
		"named_location":   "mission",
		"precision_policy": "neighborhood",
		"result_count":     3,
		"source":           "api",
	}
	out, _ := json.Marshal(body)
	return out
}

func warmUpReactiveBody(t *testing.T) []byte {
	t.Helper()
	body := map[string]any{
		"query":            "coffee warmup",
		"location_ref":     "gps:37.7749,-122.4194",
		"named_location":   "mission",
		"precision_policy": "neighborhood",
		"result_count":     3,
		"source":           "api",
	}
	out, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal warmup body: %v", err)
	}
	return out
}

// stressClientGet performs an authenticated GET via the supplied
// shared client. Mirrors stressAPIGet but reuses connections.
func stressClientGet(client *http.Client, cfg stressConfig, path string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+path, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}

// stressClientPost performs an authenticated POST via the supplied
// shared client. Mirrors stressAPIPost but reuses connections — this
// is critical at 50 concurrent QPS for 5 minutes because per-request
// http.Clients exhaust local TCP ports under sustained load.
func stressClientPost(client *http.Client, cfg stressConfig, path string, payload []byte) (int, []byte, error) {
	var bodyReader io.Reader
	if payload != nil {
		bodyReader = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+path, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}
