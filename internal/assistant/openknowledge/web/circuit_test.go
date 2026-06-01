package web

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeProvider returns a programmed sequence of (snippets, err) pairs
// indexed by call number. Callers are responsible for queueing
// enough entries; running off the end is a test failure.
type fakeProvider struct {
	name     string
	mu       sync.Mutex
	results  []fakeResult
	calls    int
	tb       testing.TB
	notCalls int // count of attempted calls past queue end (for assertions)
}

type fakeResult struct {
	snippets []WebSnippet
	err      error
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Search(_ context.Context, _ string, _ int) ([]WebSnippet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls >= len(f.results) {
		f.tb.Fatalf("fakeProvider: unexpected call #%d past queue length %d", f.calls+1, len(f.results))
	}
	r := f.results[f.calls]
	f.calls++
	return r.snippets, r.err
}

// callCount returns the number of times Search was actually invoked.
// Used by adversarial tests that prove Open short-circuits do NOT
// reach the inner provider.
func (f *fakeProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeRecorder captures state-transition + trip events so tests can
// assert on the emitted metric stream without standing up a real
// Prometheus registry.
type fakeRecorder struct {
	mu     sync.Mutex
	states []recordedState
	trips  []string
}

type recordedState struct {
	provider string
	code     int
}

func (r *fakeRecorder) SetCircuitState(provider string, code int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states = append(r.states, recordedState{provider: provider, code: code})
}

func (r *fakeRecorder) IncCircuitTrip(provider string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.trips = append(r.trips, provider)
}

func (r *fakeRecorder) lastState() recordedState {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.states) == 0 {
		return recordedState{}
	}
	return r.states[len(r.states)-1]
}

func (r *fakeRecorder) tripCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.trips)
}

// fakeClock returns a controllable monotonic time source so the
// HalfOpen transition can be exercised without time.Sleep.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock { return &fakeClock{now: start} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func baseCircuitCfg() CircuitConfig {
	return CircuitConfig{
		FailureThreshold: 5,
		OpenWindow:       60 * time.Second,
		HalfOpenAfter:    30 * time.Second,
	}
}

func newTestBreaker(t *testing.T, results []fakeResult, opts ...CircuitOption) (*CircuitBreaker, *fakeProvider, *fakeRecorder, *fakeClock) {
	t.Helper()
	fp := &fakeProvider{name: "fake-provider", results: results, tb: t}
	rec := &fakeRecorder{}
	clk := newFakeClock(time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC))
	all := append([]CircuitOption{
		WithCircuitStateRecorder(rec),
		WithCircuitClock(clk.Now),
	}, opts...)
	cb, err := NewCircuitBreaker(fp, baseCircuitCfg(), all...)
	if err != nil {
		t.Fatalf("NewCircuitBreaker: %v", err)
	}
	return cb, fp, rec, clk
}

// TestCircuit_New_ValidatesConfig covers G028 — every required
// CircuitConfig field is rejected when missing / non-positive.
func TestCircuit_New_ValidatesConfig(t *testing.T) {
	fp := &fakeProvider{name: "fake", tb: t}
	cases := []struct {
		name string
		cfg  CircuitConfig
		want string
	}{
		{"zero threshold", CircuitConfig{FailureThreshold: 0, OpenWindow: time.Second, HalfOpenAfter: time.Second}, "FailureThreshold"},
		{"neg threshold", CircuitConfig{FailureThreshold: -1, OpenWindow: time.Second, HalfOpenAfter: time.Second}, "FailureThreshold"},
		{"zero open window", CircuitConfig{FailureThreshold: 5, OpenWindow: 0, HalfOpenAfter: time.Second}, "OpenWindow"},
		{"zero half-open", CircuitConfig{FailureThreshold: 5, OpenWindow: time.Second, HalfOpenAfter: 0}, "HalfOpenAfter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewCircuitBreaker(fp, tc.cfg)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("expected ErrInvalidConfig wrap, got: %v", err)
			}
			if !contains(err.Error(), tc.want) {
				t.Errorf("error should name %q, got: %v", tc.want, err)
			}
		})
	}
	t.Run("nil inner", func(t *testing.T) {
		_, err := NewCircuitBreaker(nil, baseCircuitCfg())
		if err == nil || !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("expected ErrInvalidConfig for nil inner, got: %v", err)
		}
	})
}

// TestCircuit_StaysClosedOnSuccess covers the baseline — no failures,
// no state change, no trips emitted.
func TestCircuit_StaysClosedOnSuccess(t *testing.T) {
	results := make([]fakeResult, 4)
	for i := range results {
		results[i] = fakeResult{snippets: []WebSnippet{{URL: "https://example.test/x"}}}
	}
	cb, _, rec, _ := newTestBreaker(t, results)
	for i := 0; i < 4; i++ {
		if _, err := cb.Search(context.Background(), "q", 1); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := cb.State(); got != CircuitClosed {
		t.Errorf("State=%s want closed", got)
	}
	if rec.tripCount() != 0 {
		t.Errorf("tripCount=%d want 0", rec.tripCount())
	}
}

// TestCircuit_TripsAtThreshold covers the canonical trip — exactly
// FailureThreshold consecutive countable failures flip the breaker
// from Closed → Open with one trip emitted.
func TestCircuit_TripsAtThreshold(t *testing.T) {
	results := make([]fakeResult, 5)
	for i := range results {
		results[i] = fakeResult{err: ErrProviderUnreachable}
	}
	cb, fp, rec, _ := newTestBreaker(t, results)
	for i := 0; i < 5; i++ {
		_, err := cb.Search(context.Background(), "q", 1)
		if !errors.Is(err, ErrProviderUnreachable) {
			t.Fatalf("call %d: err=%v want ErrProviderUnreachable", i, err)
		}
	}
	if got := cb.State(); got != CircuitOpen {
		t.Fatalf("State=%s want open", got)
	}
	if fp.callCount() != 5 {
		t.Errorf("inner calls=%d want 5", fp.callCount())
	}
	if rec.tripCount() != 1 {
		t.Errorf("tripCount=%d want 1", rec.tripCount())
	}
	if last := rec.lastState(); last.code != int(CircuitOpen) || last.provider != "fake-provider" {
		t.Errorf("lastState=%+v want {fake-provider, 2}", last)
	}
}

// TestCircuit_OpenShortCircuits_AdversarialG021 — once Open, the
// breaker MUST NOT invoke the inner provider until HalfOpenAfter has
// elapsed. A regression that "leaked through" calls in Open would
// inflate the inner call count and ALSO fail because the fake
// provider's queue would underflow with t.Fatalf.
func TestCircuit_OpenShortCircuits_AdversarialG021(t *testing.T) {
	// 5 failures to trip, then NO more queued entries — any leak-through
	// will hit f.tb.Fatalf inside fakeProvider.Search.
	results := make([]fakeResult, 5)
	for i := range results {
		results[i] = fakeResult{err: ErrProviderUnreachable}
	}
	cb, fp, _, _ := newTestBreaker(t, results)
	for i := 0; i < 5; i++ {
		_, _ = cb.Search(context.Background(), "q", 1)
	}
	// Three subsequent calls — all must short-circuit without touching
	// the inner provider.
	for i := 0; i < 3; i++ {
		_, err := cb.Search(context.Background(), "q", 1)
		if !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("post-trip call %d: err=%v want ErrCircuitOpen", i, err)
		}
	}
	if fp.callCount() != 5 {
		t.Fatalf("inner calls=%d want 5 (Open MUST NOT forward)", fp.callCount())
	}
}

// TestCircuit_HalfOpenAfterWindow_SuccessRecovers — time advances
// HalfOpenAfter, the next call is forwarded as a probe, a successful
// probe restores Closed.
func TestCircuit_HalfOpenAfterWindow_SuccessRecovers(t *testing.T) {
	// 5 failures + 1 success probe.
	results := []fakeResult{
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{snippets: []WebSnippet{{URL: "https://example.test/recovered"}}},
	}
	cb, fp, rec, clk := newTestBreaker(t, results)
	for i := 0; i < 5; i++ {
		_, _ = cb.Search(context.Background(), "q", 1)
	}
	if cb.State() != CircuitOpen {
		t.Fatalf("State=%s want open", cb.State())
	}
	clk.Advance(30 * time.Second)
	snippets, err := cb.Search(context.Background(), "q", 1)
	if err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if len(snippets) != 1 {
		t.Errorf("probe snippets=%d want 1", len(snippets))
	}
	if cb.State() != CircuitClosed {
		t.Errorf("State=%s want closed after probe success", cb.State())
	}
	if fp.callCount() != 6 {
		t.Errorf("inner calls=%d want 6", fp.callCount())
	}
	if rec.tripCount() != 1 {
		t.Errorf("tripCount=%d want 1 (single Closed→Open)", rec.tripCount())
	}
}

// TestCircuit_HalfOpenProbeFailure_Reopens — a failed probe rearms
// the Open state and emits a second trip.
func TestCircuit_HalfOpenProbeFailure_Reopens(t *testing.T) {
	results := []fakeResult{
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable}, // probe failure
	}
	cb, _, rec, clk := newTestBreaker(t, results)
	for i := 0; i < 5; i++ {
		_, _ = cb.Search(context.Background(), "q", 1)
	}
	clk.Advance(30 * time.Second)
	_, err := cb.Search(context.Background(), "q", 1)
	if !errors.Is(err, ErrProviderUnreachable) {
		t.Fatalf("probe: err=%v want ErrProviderUnreachable", err)
	}
	if cb.State() != CircuitOpen {
		t.Fatalf("State=%s want open after probe failure", cb.State())
	}
	if rec.tripCount() != 2 {
		t.Errorf("tripCount=%d want 2 (initial trip + probe re-trip)", rec.tripCount())
	}
}

// TestCircuit_MixedFailureSuccess_ResetsCounter — a successful call
// resets the consecutive-failure counter so 4 failures + 1 success +
// 4 failures does NOT trip (threshold=5).
func TestCircuit_MixedFailureSuccess_ResetsCounter(t *testing.T) {
	results := []fakeResult{
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{snippets: []WebSnippet{{URL: "https://example.test/ok"}}},
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
		{err: ErrProviderUnreachable},
	}
	cb, _, rec, _ := newTestBreaker(t, results)
	for i := 0; i < len(results); i++ {
		_, _ = cb.Search(context.Background(), "q", 1)
	}
	if cb.State() != CircuitClosed {
		t.Fatalf("State=%s want closed (success reset failure counter)", cb.State())
	}
	if rec.tripCount() != 0 {
		t.Errorf("tripCount=%d want 0", rec.tripCount())
	}
}

// TestCircuit_QuotaCountsAsFailure — ErrQuotaExceeded is in the
// countable failure set per the design's failure classification.
func TestCircuit_QuotaCountsAsFailure(t *testing.T) {
	results := make([]fakeResult, 5)
	for i := range results {
		results[i] = fakeResult{err: ErrQuotaExceeded}
	}
	cb, _, rec, _ := newTestBreaker(t, results)
	for i := 0; i < 5; i++ {
		_, _ = cb.Search(context.Background(), "q", 1)
	}
	if cb.State() != CircuitOpen {
		t.Fatalf("State=%s want open", cb.State())
	}
	if rec.tripCount() != 1 {
		t.Errorf("tripCount=%d want 1", rec.tripCount())
	}
}

// TestCircuit_InvalidQueryDoesNotCount_AdversarialG021 — ErrInvalidQuery
// is a caller-side bug, not a provider problem. The breaker MUST
// NOT count it toward the threshold. 4 ErrInvalidQuery + 1
// ErrProviderUnreachable yields exactly ONE recorded failure (the
// real one); the breaker stays Closed. A regression that lumped
// ErrInvalidQuery into the failure set would observe 5 failures and
// trip — this test would fail loud.
func TestCircuit_InvalidQueryDoesNotCount_AdversarialG021(t *testing.T) {
	results := []fakeResult{
		{err: ErrInvalidQuery},
		{err: ErrInvalidQuery},
		{err: ErrInvalidQuery},
		{err: ErrInvalidQuery},
		{err: ErrProviderUnreachable},
	}
	cb, _, rec, _ := newTestBreaker(t, results)
	for i := 0; i < len(results); i++ {
		_, _ = cb.Search(context.Background(), "q", 1)
	}
	if cb.State() != CircuitClosed {
		t.Fatalf("State=%s want closed (4 InvalidQuery do NOT count)", cb.State())
	}
	if rec.tripCount() != 0 {
		t.Errorf("tripCount=%d want 0 (no trip)", rec.tripCount())
	}
}

// TestCircuit_NonCountableErrorsLeaveStateAlone — ErrProviderNotConfigured,
// ErrMalformedResponse, ErrInvalidConfig are config/protocol bugs,
// not provider outages. They MUST neither count nor reset.
func TestCircuit_NonCountableErrorsLeaveStateAlone(t *testing.T) {
	results := []fakeResult{
		{err: ErrProviderUnreachable},   // 1
		{err: ErrProviderUnreachable},   // 2
		{err: ErrProviderNotConfigured}, // no-op
		{err: ErrMalformedResponse},     // no-op
		{err: ErrInvalidConfig},         // no-op
		{err: ErrProviderUnreachable},   // 3
		{err: ErrProviderUnreachable},   // 4
		{err: ErrProviderUnreachable},   // 5 -> trip
	}
	cb, _, rec, _ := newTestBreaker(t, results)
	for i := 0; i < len(results); i++ {
		_, _ = cb.Search(context.Background(), "q", 1)
	}
	if cb.State() != CircuitOpen {
		t.Fatalf("State=%s want open (5 countable failures interleaved with no-ops)", cb.State())
	}
	if rec.tripCount() != 1 {
		t.Errorf("tripCount=%d want 1", rec.tripCount())
	}
}

// TestCircuit_StateAccessor_ConcurrencySafe — calling State() while
// another goroutine drives Search MUST NOT race. Run under `go test
// -race` in CI.
func TestCircuit_StateAccessor_ConcurrencySafe(t *testing.T) {
	results := make([]fakeResult, 200)
	for i := range results {
		if i%2 == 0 {
			results[i] = fakeResult{snippets: []WebSnippet{{URL: "https://example.test/ok"}}}
		} else {
			results[i] = fakeResult{err: ErrProviderUnreachable}
		}
	}
	cb, _, _, _ := newTestBreaker(t, results)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_, _ = cb.Search(context.Background(), "q", 1)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = cb.State()
		}
	}()
	wg.Wait()
}

// TestCircuit_NameDelegatesToInner — the breaker exposes the inner
// provider's name so metrics + the agent log carry one stable label.
func TestCircuit_NameDelegatesToInner(t *testing.T) {
	fp := &fakeProvider{name: "searxng", tb: t}
	cb, err := NewCircuitBreaker(fp, baseCircuitCfg())
	if err != nil {
		t.Fatalf("NewCircuitBreaker: %v", err)
	}
	if cb.Name() != "searxng" {
		t.Errorf("Name=%q want searxng", cb.Name())
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
