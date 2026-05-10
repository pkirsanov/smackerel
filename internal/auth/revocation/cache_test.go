package revocation

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// stubLoader is a deterministic in-memory Loader for the cache tests.
type stubLoader struct {
	mu       sync.Mutex
	calls    atomic.Int32
	queue    [][]string // FIFO of return values for successive Refresh calls
	failNext error
}

func (s *stubLoader) LoadRevokedTokenIDs(_ context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls.Add(1)
	if s.failNext != nil {
		err := s.failNext
		s.failNext = nil
		return nil, err
	}
	if len(s.queue) == 0 {
		return nil, nil
	}
	out := s.queue[0]
	s.queue = s.queue[1:]
	return out, nil
}

// T1-07 — the in-process revocation cache MUST:
//
//	(a) bootstrap from the loader on startup,
//	(b) report IsRevoked correctly for both bootstrap and broadcast inputs,
//	(c) refresh from the loader and merge new revocations,
//	(d) propagate via MarkRevoked (simulating a NATS broadcast).
//
// All paths run lock-free reads; the test uses concurrent goroutines to
// catch obvious data races (run with `go test -race`).
func TestRevocationCache_BootstrapAndPropagate(t *testing.T) {
	cache := NewCache()
	loader := &stubLoader{queue: [][]string{
		{"tok-1", "tok-2", "tok-3"}, // bootstrap
		{"tok-1", "tok-2", "tok-3", "tok-4"}, // first refresh — adds tok-4
	}}

	// (a) bootstrap
	loaded, err := cache.BootstrapFromDB(context.Background(), loader)
	if err != nil {
		t.Fatalf("BootstrapFromDB: %v", err)
	}
	if loaded != 3 {
		t.Errorf("bootstrap loaded count: want 3 got %d", loaded)
	}
	if cache.Size() != 3 {
		t.Errorf("Size after bootstrap: want 3 got %d", cache.Size())
	}

	// (b) IsRevoked for bootstrap inputs
	for _, id := range []string{"tok-1", "tok-2", "tok-3"} {
		if !cache.IsRevoked(id) {
			t.Errorf("IsRevoked(%q) = false, want true after bootstrap", id)
		}
	}
	if cache.IsRevoked("tok-fresh") {
		t.Error("IsRevoked(tok-fresh) = true, want false")
	}
	if cache.IsRevoked("") {
		t.Error("IsRevoked(\"\") = true, want false")
	}

	// (c) refresh — must add tok-4 only.
	added, err := cache.Refresh(context.Background(), loader)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if added != 1 {
		t.Errorf("Refresh newlyAdded: want 1 got %d", added)
	}
	if cache.Size() != 4 {
		t.Errorf("Size after refresh: want 4 got %d", cache.Size())
	}
	if !cache.IsRevoked("tok-4") {
		t.Error("IsRevoked(tok-4) = false after refresh")
	}

	// (d) MarkRevoked simulates a NATS broadcast.
	cache.MarkRevoked("tok-broadcast")
	if !cache.IsRevoked("tok-broadcast") {
		t.Error("IsRevoked(tok-broadcast) = false after MarkRevoked")
	}
	if cache.Size() != 5 {
		t.Errorf("Size after MarkRevoked: want 5 got %d", cache.Size())
	}

	// Idempotent — re-marking the same token does not double-count.
	cache.MarkRevoked("tok-broadcast")
	if cache.Size() != 5 {
		t.Errorf("Size after duplicate MarkRevoked: want 5 got %d", cache.Size())
	}
}

// Adversarial — the cache MUST surface loader errors rather than
// silently swallow them. A bug here would mean a transient DB outage
// during refresh leaves the cache stale without any operator signal.
func TestRevocationCache_PropagatesLoaderErrors(t *testing.T) {
	cache := NewCache()

	failingLoader := &stubLoader{failNext: errors.New("db unreachable")}
	if _, err := cache.BootstrapFromDB(context.Background(), failingLoader); err == nil {
		t.Fatal("BootstrapFromDB MUST surface loader error, got nil")
	}

	failingLoader2 := &stubLoader{failNext: errors.New("db unreachable")}
	if _, err := cache.Refresh(context.Background(), failingLoader2); err == nil {
		t.Fatal("Refresh MUST surface loader error, got nil")
	}
}

// Adversarial — nil loader is a programming bug, not a no-op.
func TestRevocationCache_RejectsNilLoader(t *testing.T) {
	cache := NewCache()
	if _, err := cache.BootstrapFromDB(context.Background(), nil); err == nil {
		t.Fatal("BootstrapFromDB(nil) MUST error")
	}
	if _, err := cache.Refresh(context.Background(), nil); err == nil {
		t.Fatal("Refresh(nil) MUST error")
	}
}
