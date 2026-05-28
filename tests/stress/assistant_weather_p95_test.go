//go:build stress

// Spec 061 SCOPE-07 — Weather skill p95 stress test (G026 budget).
//
// Drives a high-concurrency burst against the weather_lookup agent
// tool handler with an in-process *fake* Provider that returns a
// deterministic Forecast after a small synthetic latency tail. The
// tail (1ms..40ms) is intentional so a regression toward per-call
// serialization (e.g. accidental mutex contention in the cache or
// the handler) trips the assertion.
//
// Asserts:
//
//   - G1: every Invoke returns nil error.
//   - G2: per-call p95 < 3s manifest budget (design.md §5.2
//     LatencyBudget). v1 budget is 3s, NOT the retrieval 5s; the
//     weather skill is an external-provider read with a tight cache,
//     so the budget is tighter.
//   - G3: cache hit ratio matches expectation. The fake rotates
//     through `weatherStressLocationCount` locations; once the cache
//     has been warmed (one Lookup per unique location) every
//     subsequent call to the same location MUST be served from the
//     cache. Expectation: provider call count == location count,
//     regardless of total burst size (turns ≫ locations). A
//     regression that defeats the cache (e.g. a bad cache key, a
//     TTL of zero) collapses to provider.calls == turns and trips
//     the assertion.
//   - G4: p50/p95/p99/max are logged so a slow drift toward the
//     budget is visible to operators reading test output.
//
// Skips cleanly when STRESS_ASSISTANT_WEATHER_TURNS=0 so a
// contributor can shrink the burst for CI smoke runs.

package stress

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/tools/weather"
)

const (
	weatherStressTurnCount     = 800
	weatherStressWorkerCount   = 32
	weatherStressP95Budget     = 3 * time.Second // design §5.2 LatencyBudget (v1)
	weatherStressCacheCapacity = 64
	weatherStressLocationCount = 8 // distinct cache keys exercised
	// upstream timestamp held constant — the cache hit invariant
	// requires Get() to return THIS time on every hit, never the
	// wall-clock at hit time.
)

// fakeWeatherProvider is the stress double for weather.Provider.
// `calls` is incremented on every Lookup so the test can assert the
// cache short-circuited the long tail of the burst.
type fakeWeatherProvider struct {
	calls atomic.Int64
	r     *rand.Rand
	rMu   sync.Mutex
	stamp time.Time
}

func (f *fakeWeatherProvider) Name() string { return "stress-fake" }

func (f *fakeWeatherProvider) Lookup(ctx context.Context, location string, window weather.ForecastWindow) (weather.Forecast, error) {
	f.calls.Add(1)

	// 1ms..40ms synthetic upstream latency. Lock around *rand.Rand
	// (not goroutine-safe). Cost is dominated by time.After.
	f.rMu.Lock()
	jitterNs := f.r.Int63n(int64(40 * time.Millisecond))
	f.rMu.Unlock()

	select {
	case <-time.After(time.Duration(jitterNs) + time.Millisecond):
	case <-ctx.Done():
		return weather.Forecast{}, ctx.Err()
	}

	return weather.Forecast{
		ForecastLine: location + ": clear, 18.0°C",
		ProviderName: f.Name(),
		RetrievedAt:  f.stamp,
	}, nil
}

func TestAssistantWeatherStressP95(t *testing.T) {
	if v := os.Getenv("STRESS_ASSISTANT_WEATHER_TURNS"); v == "0" {
		t.Skip("stress: STRESS_ASSISTANT_WEATHER_TURNS=0 — weather stress disabled")
	}
	turns := weatherStressTurnCount
	if v := os.Getenv("STRESS_ASSISTANT_WEATHER_TURNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			turns = n
		}
	}

	// Wire the in-process fake. Cache TTL is generous (10m) so the
	// G3 cache-hit invariant is testable independently of TTL
	// expiration. Deterministic upstream timestamp so the cache-hit
	// invariant ("RetrievedAt is preserved from the original
	// upstream response, NEVER the wall clock at hit time") can be
	// asserted if a future regression flips that semantic.
	upstreamStamp := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	prov := &fakeWeatherProvider{
		r:     rand.New(rand.NewSource(1)),
		stamp: upstreamStamp,
	}
	weather.SetServices(&weather.Services{
		Provider: prov,
		Cache:    weather.NewCache(10*time.Minute, weatherStressCacheCapacity),
	})
	t.Cleanup(weather.ResetForTest)

	tool, ok := agent.ByName(weather.ToolName)
	if !ok {
		t.Fatalf("weather_lookup tool not registered; check blank-import wiring")
	}
	if tool.Handler == nil {
		t.Fatalf("weather_lookup tool has nil Handler")
	}

	// Pre-render the per-location JSON inputs so the hot path
	// doesn't include json.Marshal overhead.
	inputs := make([]json.RawMessage, weatherStressLocationCount)
	locations := make([]string, weatherStressLocationCount)
	for i := 0; i < weatherStressLocationCount; i++ {
		loc := fmt.Sprintf("StressCity-%d", i)
		locations[i] = loc
		inputs[i] = json.RawMessage(`{"location":"` + loc + `","forecast_window":"now"}`)
	}

	latencies := make([]time.Duration, turns)
	work := make(chan int, turns)
	for i := 0; i < turns; i++ {
		work <- i
	}
	close(work)

	var wg sync.WaitGroup
	var firstErr atomic.Value
	for w := 0; w < weatherStressWorkerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for i := range work {
				// Rotate locations so the cache sees a bounded
				// working set; once warm, every subsequent call
				// MUST be a cache hit (assertion G3).
				input := inputs[i%weatherStressLocationCount]
				start := time.Now()
				out, err := tool.Handler(ctx, input)
				latencies[i] = time.Since(start)
				if err != nil {
					firstErr.CompareAndSwap(nil, err)
					continue
				}
				// Sanity-check the cache-hit invariant on a sampled
				// subset (every 64th call) so a regression that
				// silently overwrites RetrievedAt with the wall
				// clock at hit time would fail the test even
				// though latency is fine.
				if i%64 == 0 {
					var got struct {
						RetrievedAt string `json:"retrieved_at"`
					}
					if jerr := json.Unmarshal(out, &got); jerr != nil {
						firstErr.CompareAndSwap(nil, jerr)
						continue
					}
					if got.RetrievedAt != upstreamStamp.Format(time.RFC3339) {
						firstErr.CompareAndSwap(nil, fmt.Errorf("cache hit overwrote retrieved_at: got %q, want %q",
							got.RetrievedAt, upstreamStamp.Format(time.RFC3339)))
					}
				}
			}
		}()
	}
	wg.Wait()

	if e := firstErr.Load(); e != nil {
		t.Fatalf("first handler error: %v", e)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]
	maxL := latencies[len(latencies)-1]

	providerCalls := prov.calls.Load()
	cacheHits := int64(turns) - providerCalls
	hitRatio := float64(cacheHits) / float64(turns)

	t.Logf("Weather Skill burst — turns=%d workers=%d locations=%d p50=%v p95=%v p99=%v max=%v provider_calls=%d cache_hits=%d hit_ratio=%.3f",
		turns, weatherStressWorkerCount, weatherStressLocationCount, p50, p95, p99, maxL, providerCalls, cacheHits, hitRatio)

	if p95 > weatherStressP95Budget {
		t.Errorf("G026 budget breach: weather skill p95=%v exceeds manifest budget=%v (design.md §5.2 LatencyBudget)",
			p95, weatherStressP95Budget)
	}

	// G3 cache-hit invariant: after warming, the provider must NOT
	// be called more than once per unique location, regardless of
	// burst size. We allow a small tolerance for the warm-up race
	// (concurrent workers may both hit the same cold location
	// before either has populated the cache).
	maxProviderCalls := int64(weatherStressLocationCount * weatherStressWorkerCount) // worst-case warm-up race
	if providerCalls > maxProviderCalls {
		t.Errorf("cache invariant breach: provider_calls=%d exceeds worst-case warm-up bound %d (locations=%d × workers=%d). Cache may be defeated.",
			providerCalls, maxProviderCalls, weatherStressLocationCount, weatherStressWorkerCount)
	}
	// Expected hit ratio is at least (turns - maxProviderCalls) / turns.
	// For turns=800, locations=8, workers=32 → expected ≥ (800-256)/800 = 0.68.
	const minExpectedHitRatio = 0.5
	if hitRatio < minExpectedHitRatio {
		t.Errorf("cache hit ratio %.3f below minimum %.3f — burst not amortizing through the cache as expected",
			hitRatio, minExpectedHitRatio)
	}
}
