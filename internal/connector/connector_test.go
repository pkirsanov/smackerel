package connector

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testConnector is a mock connector for testing.
type testConnector struct {
	id       string
	health   HealthStatus
	items    []RawArtifact
	closed   bool
	closeErr error
	syncFn   func(ctx context.Context, cursor string) ([]RawArtifact, string, error)
}

func newTestConnector(id string) *testConnector {
	return &testConnector{id: id, health: HealthHealthy}
}

func (c *testConnector) ID() string                                         { return c.id }
func (c *testConnector) Connect(_ context.Context, _ ConnectorConfig) error { return nil }
func (c *testConnector) Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
	if c.syncFn != nil {
		return c.syncFn(ctx, cursor)
	}
	return c.items, "cursor-1", nil
}
func (c *testConnector) Health(_ context.Context) HealthStatus { return c.health }
func (c *testConnector) Close() error {
	c.closed = true
	return c.closeErr
}

func TestConnectorInterface(t *testing.T) {
	var _ Connector = newTestConnector("test")
}

func TestRegistry_Register(t *testing.T) {
	reg := NewRegistry()
	conn := newTestConnector("test-1")

	if err := reg.Register(conn); err != nil {
		t.Fatalf("register: %v", err)
	}

	if reg.Count() != 1 {
		t.Errorf("expected 1 connector, got %d", reg.Count())
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	reg := NewRegistry()
	conn := newTestConnector("test-1")

	reg.Register(conn)
	err := reg.Register(conn)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistry_Get(t *testing.T) {
	reg := NewRegistry()
	conn := newTestConnector("test-1")
	reg.Register(conn)

	got, ok := reg.Get("test-1")
	if !ok {
		t.Fatal("expected to find connector")
	}
	if got.ID() != "test-1" {
		t.Errorf("expected ID test-1, got %s", got.ID())
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	reg := NewRegistry()
	_, ok := reg.Get("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	reg := NewRegistry()
	conn := newTestConnector("test-1")
	reg.Register(conn)

	if err := reg.Unregister("test-1"); err != nil {
		t.Fatalf("unregister: %v", err)
	}

	if reg.Count() != 0 {
		t.Errorf("expected 0 connectors, got %d", reg.Count())
	}
	if !conn.closed {
		t.Error("expected connector to be closed")
	}
}

func TestRegistry_Unregister_NotFound(t *testing.T) {
	reg := NewRegistry()
	err := reg.Unregister("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent connector")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := NewRegistry()
	for i := 0; i < 3; i++ {
		reg.Register(newTestConnector(fmt.Sprintf("test-%d", i)))
	}

	ids := reg.List()
	if len(ids) != 3 {
		t.Errorf("expected 3 IDs, got %d", len(ids))
	}
}

// Improve trigger I-001: List returns deterministic sorted order.
func TestRegistry_List_Sorted(t *testing.T) {
	reg := NewRegistry()
	// Register in reverse to ensure sort is actually applied
	for _, name := range []string{"zebra", "alpha", "mango", "beta"} {
		reg.Register(newTestConnector(name))
	}

	ids := reg.List()
	expected := []string{"alpha", "beta", "mango", "zebra"}
	if len(ids) != len(expected) {
		t.Fatalf("expected %d IDs, got %d", len(expected), len(ids))
	}
	for i, id := range ids {
		if id != expected[i] {
			t.Errorf("ids[%d] = %q, want %q", i, id, expected[i])
		}
	}
}

func TestConnectorSync(t *testing.T) {
	conn := newTestConnector("test")
	conn.items = []RawArtifact{
		{SourceID: "test", SourceRef: "ref-1", Title: "Item 1"},
		{SourceID: "test", SourceRef: "ref-2", Title: "Item 2"},
	}

	items, cursor, err := conn.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if cursor != "cursor-1" {
		t.Errorf("expected cursor-1, got %s", cursor)
	}
}

func TestConnectorHealth(t *testing.T) {
	conn := newTestConnector("test")

	if conn.Health(context.Background()) != HealthHealthy {
		t.Error("expected healthy status")
	}

	conn.health = HealthError
	if conn.Health(context.Background()) != HealthError {
		t.Error("expected error status")
	}
}

// --- Registry: Unregister with Close() error ---

func TestRegistry_Unregister_CloseError(t *testing.T) {
	reg := NewRegistry()
	conn := &testConnector{id: "fail-close", health: HealthHealthy, closeErr: errors.New("close failed")}
	reg.Register(conn)

	err := reg.Unregister("fail-close")
	if err == nil {
		t.Fatal("expected error when Close() fails")
	}
	if !conn.closed {
		t.Error("Close() should have been called even if it returns error")
	}
}

// --- Registry: concurrent access ---

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewRegistry()
	var wg sync.WaitGroup

	// Concurrent registrations
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			conn := newTestConnector(fmt.Sprintf("concurrent-%d", n))
			reg.Register(conn)
		}(i)
	}
	wg.Wait()

	if reg.Count() != 50 {
		t.Errorf("expected 50 connectors after concurrent register, got %d", reg.Count())
	}

	// Concurrent reads + list
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			reg.Get(fmt.Sprintf("concurrent-%d", n))
		}(i)
		go func(n int) {
			defer wg.Done()
			reg.List()
		}(i)
	}
	wg.Wait()
}

// --- HealthStatus enum coverage ---

func TestHealthStatus_AllValues(t *testing.T) {
	statuses := []HealthStatus{HealthHealthy, HealthSyncing, HealthDegraded, HealthFailing, HealthError, HealthDisconnected}
	expected := []string{"healthy", "syncing", "degraded", "failing", "error", "disconnected"}
	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("HealthStatus[%d]: expected %q, got %q", i, expected[i], string(s))
		}
	}
}

func TestHealthFromErrorCount(t *testing.T) {
	tests := []struct {
		count    int
		expected HealthStatus
	}{
		{0, HealthHealthy},
		{1, HealthHealthy},
		{4, HealthHealthy},
		{5, HealthDegraded},
		{9, HealthDegraded},
		{10, HealthFailing},
		{100, HealthFailing},
	}
	for _, tt := range tests {
		got := HealthFromErrorCount(tt.count)
		if got != tt.expected {
			t.Errorf("HealthFromErrorCount(%d) = %q, want %q", tt.count, got, tt.expected)
		}
	}
}

func TestHealthStatus_Transitions(t *testing.T) {
	conn := newTestConnector("transition-test")
	ctx := context.Background()

	transitions := []HealthStatus{HealthHealthy, HealthSyncing, HealthError, HealthDisconnected, HealthHealthy}
	for _, s := range transitions {
		conn.health = s
		got := conn.Health(ctx)
		if got != s {
			t.Errorf("expected health %q after transition, got %q", s, got)
		}
	}
}

// --- Supervisor tests ---

// panicConnector is a connector that panics on Sync to test crash recovery.
type panicConnector struct {
	id        string
	panicMsg  string
	syncCount atomic.Int32
}

func (c *panicConnector) ID() string                                         { return c.id }
func (c *panicConnector) Connect(_ context.Context, _ ConnectorConfig) error { return nil }
func (c *panicConnector) Sync(_ context.Context, _ string) ([]RawArtifact, string, error) {
	c.syncCount.Add(1)
	panic(c.panicMsg)
}
func (c *panicConnector) Health(_ context.Context) HealthStatus { return HealthError }
func (c *panicConnector) Close() error                          { return nil }

func TestSupervisor_NewSupervisor(t *testing.T) {
	reg := NewRegistry()
	sup := NewSupervisor(reg, nil)
	if sup == nil {
		t.Fatal("expected non-nil supervisor")
	}
	if sup.registry != reg {
		t.Error("expected supervisor to reference the provided registry")
	}
	if sup.stopped {
		t.Error("new supervisor should not be stopped")
	}
}

func TestSupervisor_SetPublisher(t *testing.T) {
	reg := NewRegistry()
	sup := NewSupervisor(reg, nil)
	if sup.publisher != nil {
		t.Error("publisher should be nil by default")
	}
	mp := &mockPublisher{}
	sup.SetPublisher(mp)
	if sup.publisher == nil {
		t.Error("publisher should be set after SetPublisher")
	}
}

type mockPublisher struct {
	published []RawArtifact
}

func (m *mockPublisher) PublishRawArtifact(_ context.Context, a RawArtifact) (string, error) {
	m.published = append(m.published, a)
	return "mock-id", nil
}

func TestSupervisor_StartConnector_NotInRegistry(t *testing.T) {
	reg := NewRegistry()
	sup := NewSupervisor(reg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Starting a connector that isn't registered should not panic
	sup.StartConnector(ctx, "nonexistent")
	time.Sleep(50 * time.Millisecond)
	// Should exit gracefully — connector not found in registry
}

func TestSupervisor_StartConnector_AlreadyRunning(t *testing.T) {
	reg := NewRegistry()
	blocker := &testConnector{
		id:     "blocker",
		health: HealthHealthy,
		syncFn: func(ctx context.Context, _ string) ([]RawArtifact, string, error) {
			<-ctx.Done()
			return nil, "", ctx.Err()
		},
	}
	reg.Register(blocker)
	sup := NewSupervisor(reg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "blocker")
	time.Sleep(20 * time.Millisecond)

	sup.mu.Lock()
	_, running := sup.running["blocker"]
	sup.mu.Unlock()
	if !running {
		t.Error("expected blocker to be running")
	}

	// Starting again should be a no-op (already running)
	sup.StartConnector(ctx, "blocker")

	sup.StopAll()
}

func TestSupervisor_StopConnector(t *testing.T) {
	reg := NewRegistry()
	blocker := &testConnector{
		id:     "stop-test",
		health: HealthHealthy,
		syncFn: func(ctx context.Context, _ string) ([]RawArtifact, string, error) {
			<-ctx.Done()
			return nil, "", ctx.Err()
		},
	}
	reg.Register(blocker)
	sup := NewSupervisor(reg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "stop-test")
	time.Sleep(20 * time.Millisecond)

	sup.StopConnector("stop-test")
	time.Sleep(20 * time.Millisecond)

	sup.mu.Lock()
	_, stillRunning := sup.running["stop-test"]
	sup.mu.Unlock()
	if stillRunning {
		t.Error("expected connector to be stopped after StopConnector")
	}
}

func TestSupervisor_StopConnector_NotRunning(t *testing.T) {
	reg := NewRegistry()
	sup := NewSupervisor(reg, nil)

	// Should not panic when stopping a connector that isn't running
	sup.StopConnector("nonexistent")
}

func TestSupervisor_StopAll(t *testing.T) {
	reg := NewRegistry()
	for i := 0; i < 3; i++ {
		blocker := &testConnector{
			id:     fmt.Sprintf("stop-all-%d", i),
			health: HealthHealthy,
			syncFn: func(ctx context.Context, _ string) ([]RawArtifact, string, error) {
				<-ctx.Done()
				return nil, "", ctx.Err()
			},
		}
		reg.Register(blocker)
	}
	sup := NewSupervisor(reg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < 3; i++ {
		sup.StartConnector(ctx, fmt.Sprintf("stop-all-%d", i))
	}
	time.Sleep(30 * time.Millisecond)

	sup.StopAll()
	time.Sleep(30 * time.Millisecond)

	sup.mu.Lock()
	runningCount := len(sup.running)
	stopped := sup.stopped
	sup.mu.Unlock()

	if runningCount != 0 {
		t.Errorf("expected 0 running connectors after StopAll, got %d", runningCount)
	}
	if !stopped {
		t.Error("expected supervisor stopped flag to be true after StopAll")
	}
}

func TestSupervisor_StopAll_RejectsNewStarts(t *testing.T) {
	reg := NewRegistry()
	conn := newTestConnector("after-stop")
	reg.Register(conn)
	sup := NewSupervisor(reg, nil)

	sup.StopAll()

	ctx := context.Background()
	sup.StartConnector(ctx, "after-stop")
	time.Sleep(30 * time.Millisecond)

	sup.mu.Lock()
	_, running := sup.running["after-stop"]
	sup.mu.Unlock()
	if running {
		t.Error("supervisor should reject new starts after StopAll")
	}
}

func TestSupervisor_PanicRecovery(t *testing.T) {
	reg := NewRegistry()
	pc := &panicConnector{id: "panic-test", panicMsg: "test panic"}
	reg.Register(pc)
	sup := NewSupervisor(reg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "panic-test")

	// Wait enough for at least one panic + recovery cycle
	time.Sleep(200 * time.Millisecond)

	count := pc.syncCount.Load()
	if count < 1 {
		t.Errorf("expected at least 1 sync attempt (panic trigger), got %d", count)
	}

	sup.mu.Lock()
	panicCount := sup.panicCounts["panic-test"]
	sup.mu.Unlock()
	if panicCount < 1 {
		t.Errorf("expected panic count >= 1, got %d", panicCount)
	}

	sup.StopAll()
}

func TestSupervisor_CircuitBreaker(t *testing.T) {
	reg := NewRegistry()
	pc := &panicConnector{id: "breaker-test", panicMsg: "repeated panic"}
	reg.Register(pc)
	sup := NewSupervisor(reg, nil)

	// Manually set panic count just below the threshold
	sup.mu.Lock()
	sup.panicCounts["breaker-test"] = maxPanicsBeforeDisable - 1
	sup.panicResetAt["breaker-test"] = time.Now()
	sup.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "breaker-test")
	time.Sleep(200 * time.Millisecond)

	sup.mu.Lock()
	panicCount := sup.panicCounts["breaker-test"]
	_, stillRunning := sup.running["breaker-test"]
	sup.mu.Unlock()

	if panicCount < maxPanicsBeforeDisable {
		t.Errorf("expected panic count >= %d, got %d", maxPanicsBeforeDisable, panicCount)
	}
	if stillRunning {
		t.Error("connector should not be running after circuit breaker trips")
	}

	sup.StopAll()
}

func TestSupervisor_ParentContextCancelled_NoRestart(t *testing.T) {
	reg := NewRegistry()
	pc := &panicConnector{id: "ctx-cancel-test", panicMsg: "shutdown panic"}
	reg.Register(pc)
	sup := NewSupervisor(reg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	sup.StartConnector(ctx, "ctx-cancel-test")

	// Cancel parent context before recovery can restart
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	sup.mu.Lock()
	_, running := sup.running["ctx-cancel-test"]
	sup.mu.Unlock()
	if running {
		t.Error("connector should not restart when parent context is cancelled")
	}

	sup.StopAll()
}

// Chaos regression: connector that always fails Sync() must eventually be disabled
// by the sync error circuit breaker, not loop forever.
func TestSupervisor_SyncErrorCircuitBreaker(t *testing.T) {
	reg := NewRegistry()
	errorCount := &atomic.Int32{}
	conn := &testConnector{
		id:     "error-loop",
		health: HealthHealthy,
		syncFn: func(_ context.Context, _ string) ([]RawArtifact, string, error) {
			errorCount.Add(1)
			return nil, "", errors.New("persistent auth failure")
		},
	}
	reg.Register(conn)

	sup := NewSupervisor(reg, nil)
	sup.maxSyncErrors = 5
	sup.backoffFactory = func() *Backoff {
		return &Backoff{
			BaseDelay:  time.Millisecond,
			MaxDelay:   5 * time.Millisecond,
			MaxRetries: 100, // high enough to not exhaust before circuit trips
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "error-loop")

	// Wait for the circuit breaker to trip
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for sync error circuit breaker to trip")
		default:
		}
		sup.mu.Lock()
		_, stillRunning := sup.running["error-loop"]
		sup.mu.Unlock()
		if !stillRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	count := errorCount.Load()
	if count < 5 {
		t.Errorf("expected at least 5 sync errors before circuit breaker, got %d", count)
	}

	sup.StopAll()
}

// Chaos regression: cursor must survive in memory when state store is nil,
// so successive sync cycles use the latest cursor rather than empty string.
func TestSupervisor_InMemoryCursorFallback(t *testing.T) {
	reg := NewRegistry()
	cursorsSeen := make([]string, 0)
	var mu sync.Mutex
	syncCalls := &atomic.Int32{}

	conn := &testConnector{
		id:     "cursor-test",
		health: HealthHealthy,
		syncFn: func(_ context.Context, cursor string) ([]RawArtifact, string, error) {
			call := syncCalls.Add(1)
			mu.Lock()
			cursorsSeen = append(cursorsSeen, cursor)
			mu.Unlock()
			if call >= 3 {
				// After 3 syncs, block until cancelled to let test inspect
				return nil, fmt.Sprintf("cursor-%d", call), nil
			}
			return []RawArtifact{{SourceID: "cursor-test", SourceRef: "ref"}}, fmt.Sprintf("cursor-%d", call), nil
		},
	}
	reg.Register(conn)

	// No state store — relies entirely on in-memory cursor fallback
	sup := NewSupervisor(reg, nil)
	sup.backoffFactory = func() *Backoff {
		return &Backoff{BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, MaxRetries: 5}
	}
	// Override sync interval to be very short
	sup.SetConfig("cursor-test", ConnectorConfig{SyncSchedule: "1ms"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "cursor-test")

	// Wait for at least 3 sync calls
	deadline := time.After(3 * time.Second)
	for {
		if syncCalls.Load() >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for sync calls, got %d", syncCalls.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	cancel()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// First call should have empty cursor (no state store, no previous sync)
	if len(cursorsSeen) < 2 {
		t.Fatalf("expected at least 2 cursor observations, got %d", len(cursorsSeen))
	}
	if cursorsSeen[0] != "" {
		t.Errorf("first sync should have empty cursor, got %q", cursorsSeen[0])
	}
	// Second call should have the cursor returned from first sync
	if cursorsSeen[1] != "cursor-1" {
		t.Errorf("second sync should use in-memory cursor 'cursor-1', got %q", cursorsSeen[1])
	}
}

// --- Hardening: H1 — ListConnectorHealth does not hold lock during Health() calls ---

// slowHealthConnector blocks in Health() for the configured duration.
type slowHealthConnector struct {
	id    string
	delay time.Duration
}

func (c *slowHealthConnector) ID() string                                         { return c.id }
func (c *slowHealthConnector) Connect(_ context.Context, _ ConnectorConfig) error { return nil }
func (c *slowHealthConnector) Sync(_ context.Context, _ string) ([]RawArtifact, string, error) {
	return nil, "", nil
}
func (c *slowHealthConnector) Health(_ context.Context) HealthStatus {
	time.Sleep(c.delay)
	return HealthHealthy
}
func (c *slowHealthConnector) Close() error { return nil }

func TestRegistry_ListConnectorHealth_DoesNotBlockRegister(t *testing.T) {
	reg := NewRegistry()
	slow := &slowHealthConnector{id: "slow-health", delay: 200 * time.Millisecond}
	reg.Register(slow)

	done := make(chan struct{})
	go func() {
		reg.ListConnectorHealth(context.Background())
		close(done)
	}()

	// While ListConnectorHealth is blocked in Health(), Register should succeed quickly
	time.Sleep(20 * time.Millisecond)
	start := time.Now()
	err := reg.Register(newTestConnector("quick-register"))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("Register took %v — should not be blocked by slow Health() call", elapsed)
	}

	<-done
	reg.Unregister("slow-health")
	reg.Unregister("quick-register")
}

// --- Hardening: H2 — Unregister recovers from Close() panic ---

// panicCloseConnector panics in Close().
type panicCloseConnector struct {
	id string
}

func (c *panicCloseConnector) ID() string                                         { return c.id }
func (c *panicCloseConnector) Connect(_ context.Context, _ ConnectorConfig) error { return nil }
func (c *panicCloseConnector) Sync(_ context.Context, _ string) ([]RawArtifact, string, error) {
	return nil, "", nil
}
func (c *panicCloseConnector) Health(_ context.Context) HealthStatus { return HealthHealthy }
func (c *panicCloseConnector) Close() error                          { panic("Close() exploded") }

func TestRegistry_Unregister_ClosePanicRecovery(t *testing.T) {
	reg := NewRegistry()
	pc := &panicCloseConnector{id: "panic-close"}
	reg.Register(pc)

	err := reg.Unregister("panic-close")
	if err == nil {
		t.Fatal("expected error from panicking Close()")
	}
	if reg.Count() != 0 {
		t.Errorf("connector should still be removed from registry even when Close panics, got count=%d", reg.Count())
	}
}
