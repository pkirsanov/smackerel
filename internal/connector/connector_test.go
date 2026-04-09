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
	statuses := []HealthStatus{HealthHealthy, HealthSyncing, HealthError, HealthDisconnected}
	expected := []string{"healthy", "syncing", "error", "disconnected"}
	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("HealthStatus[%d]: expected %q, got %q", i, expected[i], string(s))
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
