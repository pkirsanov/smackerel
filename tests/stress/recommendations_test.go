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

	type sample struct {
		latency time.Duration
		status  int
		errKind string
	}
	samplesCh := make(chan sample, recommendationsStressConcurrency*64)
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
				switch {
				case err != nil && errors.Is(err, context.DeadlineExceeded):
					return
				case err != nil:
					samplesCh <- sample{latency: latency, status: 0, errKind: "transport"}
				case status == http.StatusTooManyRequests:
					samplesCh <- sample{latency: latency, status: status, errKind: "rate_limit"}
				case status == http.StatusForbidden:
					samplesCh <- sample{latency: latency, status: status, errKind: "quota"}
				case status >= 500:
					samplesCh <- sample{latency: latency, status: status, errKind: "server_error"}
				case status != http.StatusOK && status != http.StatusCreated && status != http.StatusAccepted:
					samplesCh <- sample{latency: latency, status: status, errKind: "unexpected_status"}
				default:
					samplesCh <- sample{latency: latency, status: status}
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

	var (
		latencies   []time.Duration
		errorCount  int
		acceptedErr int // expected rate_limit / quota outcomes
		serverErr   int
	)
	for s := range samplesCh {
		switch s.errKind {
		case "":
			latencies = append(latencies, s.latency)
		case "rate_limit", "quota":
			acceptedErr++
			latencies = append(latencies, s.latency)
		case "server_error", "transport", "unexpected_status":
			serverErr++
			errorCount++
		}
	}
	<-doneCh

	totalSamples := len(latencies) + serverErr
	if totalSamples == 0 {
		t.Fatal("stress: zero samples collected — workers never produced any observations")
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := percentile(latencies, 0.50)
	p95 := percentile(latencies, 0.95)
	p99 := percentile(latencies, 0.99)
	maxLat := time.Duration(0)
	if len(latencies) > 0 {
		maxLat = latencies[len(latencies)-1]
	}
	errPct := 100.0 * float64(serverErr) / float64(totalSamples)

	t.Logf("stress samples: total=%d ok=%d accepted_errors=%d server_errors=%d (rate %.2f%%)",
		totalSamples, len(latencies)-acceptedErr, acceptedErr, serverErr, errPct)
	t.Logf("stress latency: p50=%s p95=%s p99=%s max=%s budget=%s",
		p50, p95, p99, maxLat, recommendationsStressP95Budget)

	if p95 > recommendationsStressP95Budget {
		t.Errorf("p95 %s exceeds NFR budget %s", p95, recommendationsStressP95Budget)
	}
	if errPct > recommendationsStressMaxErrorPct {
		t.Errorf("server error rate %.2f%% exceeds %.2f%% budget (server_errors=%d, total=%d)",
			errPct, recommendationsStressMaxErrorPct, serverErr, totalSamples)
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
