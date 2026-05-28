// Spec 061 SCOPE-09 — active-threads gauge refresher tests.
//
// Drives ActiveThreadsRefresher against an in-memory Store stub and
// verifies:
//
//   - Per-transport Set() invocations match the underlying row counts.
//   - Transports in the supplied vocabulary that have zero rows still
//     receive Set(0) — the gauge must reflect emptiness rather than
//     freezing at the previous sample.
//   - Store errors are logged but do not crash the loop (a subsequent
//     Refresh after the error returns the gauge to truth).
//   - The constructor enforces every required dependency (no silent
//     fallbacks).
//
// All assertions are pure equality on captured Set() calls; no
// Prometheus globals are touched so this test cannot collide with
// other tests in the suite.

package assistantctx

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// captureStore is an in-memory Store stub. Only the methods exercised
// by the refresher (CountActiveByTransport) carry behaviour; the
// others satisfy the interface with zero values.
type captureStore struct {
	mu     sync.Mutex
	counts map[string]int
	err    error
}

func newCaptureStore() *captureStore { return &captureStore{counts: map[string]int{}} }

func (s *captureStore) setCounts(counts map[string]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts = make(map[string]int, len(counts))
	for k, v := range counts {
		s.counts[k] = v
	}
}

func (s *captureStore) setErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *captureStore) Load(context.Context, string, string) (Conversation, bool, error) {
	return Conversation{}, false, nil
}
func (s *captureStore) Persist(context.Context, Conversation) error             { return nil }
func (s *captureStore) DeleteByKey(context.Context, string, string) error       { return nil }
func (s *captureStore) SweepIdle(context.Context, time.Duration) (int64, error) { return 0, nil }
func (s *captureStore) CountActiveByTransport(context.Context) (map[string]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[string]int, len(s.counts))
	for k, v := range s.counts {
		out[k] = v
	}
	return out, nil
}

// captureSetter records every (transport, count) pair the refresher
// pushes through the setGauge callback.
type captureSetter struct {
	mu    sync.Mutex
	calls []setCall
}

type setCall struct {
	transport string
	count     float64
}

func (c *captureSetter) set(transport string, count float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, setCall{transport: transport, count: count})
}

func (c *captureSetter) snapshot() []setCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]setCall, len(c.calls))
	copy(out, c.calls)
	return out
}

// latestPerTransport returns the most recent count seen for each
// transport (the refresher MAY call Set multiple times across ticks;
// the last value is what the gauge would hold).
func (c *captureSetter) latestPerTransport() map[string]float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]float64{}
	for _, call := range c.calls {
		out[call.transport] = call.count
	}
	return out
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewActiveThreadsRefresher_ValidatesDependencies(t *testing.T) {
	store := newCaptureStore()
	setter := &captureSetter{}
	logger := discardLogger()
	transports := []string{"telegram", "fake"}

	cases := []struct {
		name   string
		build  func() error
		errSub string
	}{
		{
			name: "nil store",
			build: func() error {
				_, err := NewActiveThreadsRefresher(nil, transports, time.Second, setter.set, logger)
				return err
			},
			errSub: "non-nil Store",
		},
		{
			name: "empty transports",
			build: func() error {
				_, err := NewActiveThreadsRefresher(store, nil, time.Second, setter.set, logger)
				return err
			},
			errSub: "transports vocabulary",
		},
		{
			name: "zero interval",
			build: func() error {
				_, err := NewActiveThreadsRefresher(store, transports, 0, setter.set, logger)
				return err
			},
			errSub: "positive interval",
		},
		{
			name: "nil setGauge",
			build: func() error {
				_, err := NewActiveThreadsRefresher(store, transports, time.Second, nil, logger)
				return err
			},
			errSub: "setGauge callback",
		},
		{
			name: "nil logger",
			build: func() error {
				_, err := NewActiveThreadsRefresher(store, transports, time.Second, setter.set, nil)
				return err
			},
			errSub: "non-nil logger",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.build()
			if err == nil {
				t.Fatalf("expected constructor to reject %s", tc.name)
			}
			if !contains(err.Error(), tc.errSub) {
				t.Fatalf("error %q does not contain %q", err, tc.errSub)
			}
		})
	}
}

func TestActiveThreadsRefresher_Refresh_PerTransportCounts(t *testing.T) {
	store := newCaptureStore()
	store.setCounts(map[string]int{"telegram": 2, "fake": 1})
	setter := &captureSetter{}
	transports := []string{"telegram", "fake"}

	r, err := NewActiveThreadsRefresher(store, transports, time.Second, setter.set, discardLogger())
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	r.Refresh(context.Background())

	got := setter.latestPerTransport()
	if got["telegram"] != 2 {
		t.Fatalf("telegram count: got %v, want 2", got["telegram"])
	}
	if got["fake"] != 1 {
		t.Fatalf("fake count: got %v, want 1", got["fake"])
	}
}

func TestActiveThreadsRefresher_Refresh_ZeroFillsAbsentTransport(t *testing.T) {
	store := newCaptureStore()
	// Only "telegram" has rows; "fake" must still receive Set(0).
	store.setCounts(map[string]int{"telegram": 4})
	setter := &captureSetter{}
	transports := []string{"telegram", "fake"}

	r, err := NewActiveThreadsRefresher(store, transports, time.Second, setter.set, discardLogger())
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	r.Refresh(context.Background())

	got := setter.latestPerTransport()
	if got["telegram"] != 4 {
		t.Fatalf("telegram count: got %v, want 4", got["telegram"])
	}
	if got["fake"] != 0 {
		t.Fatalf("fake count: got %v, want 0 (zero-fill)", got["fake"])
	}

	calls := setter.snapshot()
	if len(calls) != 2 {
		t.Fatalf("expected exactly 2 Set calls (one per transport), got %d: %+v", len(calls), calls)
	}
}

func TestActiveThreadsRefresher_Refresh_RowsDrainToZero(t *testing.T) {
	store := newCaptureStore()
	store.setCounts(map[string]int{"telegram": 3})
	setter := &captureSetter{}
	transports := []string{"telegram", "fake"}

	r, err := NewActiveThreadsRefresher(store, transports, time.Second, setter.set, discardLogger())
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}

	r.Refresh(context.Background())
	if got := setter.latestPerTransport()["telegram"]; got != 3 {
		t.Fatalf("after first refresh: got %v, want 3", got)
	}

	// All rows drained.
	store.setCounts(map[string]int{})
	r.Refresh(context.Background())
	if got := setter.latestPerTransport()["telegram"]; got != 0 {
		t.Fatalf("after drain: telegram count %v, want 0 (refresher must reflect empty store)", got)
	}
	if got := setter.latestPerTransport()["fake"]; got != 0 {
		t.Fatalf("after drain: fake count %v, want 0", got)
	}
}

func TestActiveThreadsRefresher_Refresh_StoreErrorDoesNotCrash(t *testing.T) {
	store := newCaptureStore()
	store.setCounts(map[string]int{"telegram": 7})
	setter := &captureSetter{}
	transports := []string{"telegram", "fake"}

	r, err := NewActiveThreadsRefresher(store, transports, time.Second, setter.set, discardLogger())
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}

	// First refresh succeeds and seeds the gauge.
	r.Refresh(context.Background())
	if got := setter.latestPerTransport()["telegram"]; got != 7 {
		t.Fatalf("seed refresh: telegram count %v, want 7", got)
	}

	// Store starts erroring; refresher must NOT push (gauge holds
	// stale value), but loop continues.
	store.setErr(errors.New("transient db blip"))
	r.Refresh(context.Background())
	if got := setter.latestPerTransport()["telegram"]; got != 7 {
		t.Fatalf("during error: telegram count %v, want 7 (gauge unchanged on error)", got)
	}

	// Error clears; refresher returns to truth.
	store.setErr(nil)
	store.setCounts(map[string]int{"telegram": 9})
	r.Refresh(context.Background())
	if got := setter.latestPerTransport()["telegram"]; got != 9 {
		t.Fatalf("after recovery: telegram count %v, want 9", got)
	}
}

func TestActiveThreadsRefresher_Run_PerformsImmediateRefresh(t *testing.T) {
	store := newCaptureStore()
	store.setCounts(map[string]int{"telegram": 5})
	setter := &captureSetter{}
	transports := []string{"telegram"}

	// Use an absurdly long interval so any Set we observe came from
	// the immediate refresh at Run start, not from a tick.
	r, err := NewActiveThreadsRefresher(store, transports, time.Hour, setter.set, discardLogger())
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	// Wait for the immediate refresh to land. The refresher writes
	// before blocking on the ticker, so a short poll suffices.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := setter.latestPerTransport()["telegram"]; got == 5 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := setter.latestPerTransport()["telegram"]; got != 5 {
		cancel()
		<-done
		t.Fatalf("Run did not perform immediate refresh: got %v, want 5", got)
	}

	cancel()
	<-done
}

// contains is a tiny substring helper to keep the test file free of
// strings.Contains in the assertion error builders (saves an import
// in the test for a single use).
func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
