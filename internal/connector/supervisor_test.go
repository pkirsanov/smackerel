package connector

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test helpers / mocks
// ---------------------------------------------------------------------------

// supervisorMockConnector implements the Connector interface for supervisor testing.
type supervisorMockConnector struct {
	id        string
	syncFunc  func(ctx context.Context, cursor string) ([]RawArtifact, string, error)
	syncCount atomic.Int32
}

func (m *supervisorMockConnector) ID() string { return m.id }
func (m *supervisorMockConnector) Connect(_ context.Context, _ ConnectorConfig) error {
	return nil
}
func (m *supervisorMockConnector) Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
	m.syncCount.Add(1)
	if m.syncFunc != nil {
		return m.syncFunc(ctx, cursor)
	}
	return nil, cursor, nil
}
func (m *supervisorMockConnector) Health(_ context.Context) HealthStatus {
	return HealthHealthy
}
func (m *supervisorMockConnector) Close() error { return nil }

// supervisorMockPublisher implements the ArtifactPublisher interface for supervisor testing.
type supervisorMockPublisher struct {
	mu        sync.Mutex
	published []RawArtifact
	err       error
}

func (p *supervisorMockPublisher) PublishRawArtifact(_ context.Context, a RawArtifact) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return "", p.err
	}
	p.published = append(p.published, a)
	return "art-" + a.SourceRef, nil
}

func (p *supervisorMockPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.published)
}

// ---------------------------------------------------------------------------
// NewSupervisor
// ---------------------------------------------------------------------------

func TestNewSupervisor(t *testing.T) {
	reg := NewRegistry()
	sup := NewSupervisor(reg, nil)
	if sup == nil {
		t.Fatal("NewSupervisor returned nil")
	}
	if sup.registry != reg {
		t.Fatal("registry not set")
	}
	if sup.running == nil || sup.panicCounts == nil || sup.panicResetAt == nil || sup.connectorConfigs == nil {
		t.Fatal("internal maps not initialised")
	}
}

// ---------------------------------------------------------------------------
// SetPublisher / SetConfig
// ---------------------------------------------------------------------------

func TestSetPublisher(t *testing.T) {
	sup := NewSupervisor(NewRegistry(), nil)
	pub := &supervisorMockPublisher{}
	sup.SetPublisher(pub)

	sup.mu.RLock()
	defer sup.mu.RUnlock()
	if sup.publisher == nil {
		t.Fatal("publisher not set")
	}
}

func TestSetConfig(t *testing.T) {
	sup := NewSupervisor(NewRegistry(), nil)
	cfg := ConnectorConfig{SyncSchedule: "30m"}
	sup.SetConfig("test", cfg)

	sup.mu.RLock()
	defer sup.mu.RUnlock()
	stored, ok := sup.connectorConfigs["test"]
	if !ok {
		t.Fatal("config not stored")
	}
	if stored.SyncSchedule != "30m" {
		t.Fatalf("expected schedule 30m, got %s", stored.SyncSchedule)
	}
}

// ---------------------------------------------------------------------------
// StartConnector / StopConnector basics
// ---------------------------------------------------------------------------

func TestStartConnector_UnknownID(t *testing.T) {
	sup := NewSupervisor(NewRegistry(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Starting an unregistered connector should not panic, just log and return.
	sup.StartConnector(ctx, "nonexistent")

	// Give goroutine time to run and exit
	time.Sleep(50 * time.Millisecond)

	// The goroutine exits but the running map entry persists because
	// runWithRecovery does not clean up on registry miss. The important
	// thing is no panic occurred.
	cancel()
	sup.StopAll()
}

func TestStartConnector_AlreadyRunning(t *testing.T) {
	reg := NewRegistry()
	synced := make(chan struct{}, 10)
	conn := &supervisorMockConnector{
		id: "c1",
		syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
			synced <- struct{}{}
			// Block until cancelled
			<-ctx.Done()
			return nil, cursor, ctx.Err()
		},
	}
	reg.Register(conn)

	sup := NewSupervisor(reg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "c1")
	<-synced // Wait for first sync to start

	// Second start should be a no-op
	sup.StartConnector(ctx, "c1")

	sup.mu.RLock()
	count := len(sup.running)
	sup.mu.RUnlock()

	if count != 1 {
		t.Fatalf("expected 1 running connector, got %d", count)
	}

	cancel()
	sup.StopAll()
}

func TestStartConnector_RejectsAfterStopAll(t *testing.T) {
	reg := NewRegistry()
	conn := &supervisorMockConnector{
		id: "c1",
		syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
			<-ctx.Done()
			return nil, cursor, ctx.Err()
		},
	}
	reg.Register(conn)

	sup := NewSupervisor(reg, nil)
	sup.StopAll()

	ctx := context.Background()
	sup.StartConnector(ctx, "c1")

	sup.mu.RLock()
	_, running := sup.running["c1"]
	sup.mu.RUnlock()
	if running {
		t.Fatal("should not allow start after StopAll")
	}
}

func TestStopConnector(t *testing.T) {
	reg := NewRegistry()
	conn := &supervisorMockConnector{
		id: "c1",
		syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
			<-ctx.Done()
			return nil, cursor, ctx.Err()
		},
	}
	reg.Register(conn)

	sup := NewSupervisor(reg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "c1")
	time.Sleep(20 * time.Millisecond) // let goroutine start

	sup.StopConnector("c1")

	sup.mu.RLock()
	_, running := sup.running["c1"]
	sup.mu.RUnlock()
	if running {
		t.Fatal("connector should be removed from running map after stop")
	}

	cancel()
	sup.StopAll()
}

func TestStopConnector_NonRunning(t *testing.T) {
	sup := NewSupervisor(NewRegistry(), nil)
	// Should not panic
	sup.StopConnector("doesnotexist")
}

// ---------------------------------------------------------------------------
// TriggerSync
// ---------------------------------------------------------------------------

func TestTriggerSync_RestartsRunning(t *testing.T) {
	reg := NewRegistry()
	var syncCalls atomic.Int32
	conn := &supervisorMockConnector{
		id: "c1",
		syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
			syncCalls.Add(1)
			<-ctx.Done()
			return nil, cursor, ctx.Err()
		},
	}
	reg.Register(conn)

	sup := NewSupervisor(reg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "c1")
	time.Sleep(30 * time.Millisecond)

	before := syncCalls.Load()
	sup.TriggerSync(ctx, "c1")
	time.Sleep(30 * time.Millisecond)

	after := syncCalls.Load()
	if after <= before {
		t.Fatalf("expected new sync call after trigger, before=%d after=%d", before, after)
	}

	cancel()
	sup.StopAll()
}

// ---------------------------------------------------------------------------
// StopAll
// ---------------------------------------------------------------------------

func TestStopAll_CancelsAll(t *testing.T) {
	reg := NewRegistry()
	var stopped atomic.Int32
	makeConn := func(id string) *supervisorMockConnector {
		return &supervisorMockConnector{
			id: id,
			syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
				<-ctx.Done()
				stopped.Add(1)
				return nil, cursor, ctx.Err()
			},
		}
	}

	for _, id := range []string{"a", "b", "c"} {
		c := makeConn(id)
		reg.Register(c)
	}

	sup := NewSupervisor(reg, nil)
	ctx := context.Background()
	for _, id := range []string{"a", "b", "c"} {
		sup.StartConnector(ctx, id)
	}
	time.Sleep(30 * time.Millisecond) // let goroutines start

	sup.StopAll()

	if stopped.Load() != 3 {
		t.Fatalf("expected 3 stopped, got %d", stopped.Load())
	}

	sup.mu.RLock()
	count := len(sup.running)
	sup.mu.RUnlock()
	if count != 0 {
		t.Fatalf("expected 0 running after StopAll, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Sync error circuit breaker
// ---------------------------------------------------------------------------

func TestSyncErrorCircuitBreaker(t *testing.T) {
	reg := NewRegistry()
	syncErr := errors.New("sync failure")
	conn := &supervisorMockConnector{
		id: "c1",
		syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
			return nil, cursor, syncErr
		},
	}
	reg.Register(conn)

	sup := NewSupervisor(reg, nil)
	sup.maxSyncErrors = 3
	// Use instant backoff for fast testing
	sup.backoffFactory = func() *Backoff {
		return &Backoff{
			BaseDelay:  1 * time.Millisecond,
			MaxDelay:   1 * time.Millisecond,
			MaxRetries: 100, // high retries so circuit breaker triggers first
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "c1")

	// Wait for circuit breaker to trip
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for circuit breaker to trip")
		default:
		}
		sup.mu.RLock()
		_, running := sup.running["c1"]
		sup.mu.RUnlock()
		if !running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	calls := conn.syncCount.Load()
	if calls < 3 {
		t.Fatalf("expected at least 3 sync calls before circuit breaker, got %d", calls)
	}

	cancel()
	sup.StopAll()
}

// ---------------------------------------------------------------------------
// Successful sync with artifact publishing
// ---------------------------------------------------------------------------

func TestSuccessfulSync_PublishesArtifacts(t *testing.T) {
	reg := NewRegistry()
	artifacts := []RawArtifact{
		{SourceRef: "item1", Title: "Test 1"},
		{SourceRef: "item2", Title: "Test 2"},
	}
	syncDone := make(chan struct{}, 1)
	conn := &supervisorMockConnector{
		id: "c1",
		syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
			select {
			case syncDone <- struct{}{}:
			default:
			}
			// Return items once, then block
			if cursor == "" {
				return artifacts, "cursor1", nil
			}
			<-ctx.Done()
			return nil, cursor, ctx.Err()
		},
	}
	reg.Register(conn)

	pub := &supervisorMockPublisher{}
	sup := NewSupervisor(reg, nil)
	sup.SetPublisher(pub)
	sup.SetConfig("c1", ConnectorConfig{SyncSchedule: "1h"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "c1")
	<-syncDone
	// Give time for publish loop
	time.Sleep(50 * time.Millisecond)

	if pub.count() != 2 {
		t.Fatalf("expected 2 published artifacts, got %d", pub.count())
	}

	cancel()
	sup.StopAll()
}

func TestSuccessfulSync_PublishError_DoesNotCrash(t *testing.T) {
	reg := NewRegistry()
	syncDone := make(chan struct{}, 1)
	conn := &supervisorMockConnector{
		id: "c1",
		syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
			select {
			case syncDone <- struct{}{}:
			default:
			}
			if cursor == "" {
				return []RawArtifact{{SourceRef: "x"}}, "c1", nil
			}
			<-ctx.Done()
			return nil, cursor, ctx.Err()
		},
	}
	reg.Register(conn)

	pub := &supervisorMockPublisher{err: errors.New("publish fail")}
	sup := NewSupervisor(reg, nil)
	sup.SetPublisher(pub)
	sup.SetConfig("c1", ConnectorConfig{SyncSchedule: "1h"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "c1")
	<-syncDone
	time.Sleep(50 * time.Millisecond)

	// Should not crash — 0 published since all failed
	if pub.count() != 0 {
		t.Fatalf("expected 0 published (all errored), got %d", pub.count())
	}

	cancel()
	sup.StopAll()
}

// ---------------------------------------------------------------------------
// Panic recovery / circuit breaker
// ---------------------------------------------------------------------------

func TestPanicRecovery_RestartAfterPanic(t *testing.T) {
	reg := NewRegistry()
	var callCount atomic.Int32
	conn := &supervisorMockConnector{
		id: "c1",
		syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
			n := callCount.Add(1)
			if n == 1 {
				panic("test panic")
			}
			// Second call: block until cancelled
			<-ctx.Done()
			return nil, cursor, ctx.Err()
		},
	}
	reg.Register(conn)

	sup := NewSupervisor(reg, nil)
	// Use tiny backoff so panic restart delay doesn't dominate
	sup.backoffFactory = func() *Backoff {
		return &Backoff{BaseDelay: 1 * time.Millisecond, MaxDelay: 1 * time.Millisecond, MaxRetries: 5}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "c1")

	// Wait for restart (panic → delay → restart)
	deadline := time.After(15 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for restart; sync calls: %d", callCount.Load())
		default:
		}
		if callCount.Load() >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	sup.StopAll()
}

func TestPanicCircuitBreaker_TripsAfterMaxPanics(t *testing.T) {
	reg := NewRegistry()
	var callCount atomic.Int32
	conn := &supervisorMockConnector{
		id: "c1",
		syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
			callCount.Add(1)
			panic("always panic")
		},
	}
	reg.Register(conn)

	sup := NewSupervisor(reg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "c1")

	// Wait for circuit breaker to stop restarts
	deadline := time.After(60 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for panic circuit breaker")
		default:
		}

		sup.mu.RLock()
		count := sup.panicCounts["c1"]
		_, running := sup.running["c1"]
		sup.mu.RUnlock()

		if count >= maxPanicsBeforeDisable && !running {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	calls := callCount.Load()
	if calls < int32(maxPanicsBeforeDisable) {
		t.Fatalf("expected at least %d sync calls before circuit breaker, got %d", maxPanicsBeforeDisable, calls)
	}

	cancel()
	sup.StopAll()
}

func TestPanicRecovery_SkipsRestartWhenStopped(t *testing.T) {
	reg := NewRegistry()
	panicked := make(chan struct{}, 1)
	conn := &supervisorMockConnector{
		id: "c1",
		syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
			select {
			case panicked <- struct{}{}:
			default:
			}
			panic("test panic")
		},
	}
	reg.Register(conn)

	sup := NewSupervisor(reg, nil)
	ctx := context.Background()

	sup.StartConnector(ctx, "c1")
	<-panicked

	// Stop supervisor immediately — the restart delay should check stopped flag
	sup.StopAll()

	time.Sleep(100 * time.Millisecond)

	sup.mu.RLock()
	count := len(sup.running)
	sup.mu.RUnlock()
	if count != 0 {
		t.Fatalf("expected 0 running after StopAll + panic, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Backoff max retries exhausted → wait for next cycle
// ---------------------------------------------------------------------------

func TestSyncError_BackoffExhausted_WaitsForNextCycle(t *testing.T) {
	reg := NewRegistry()
	var callCount atomic.Int32
	conn := &supervisorMockConnector{
		id: "c1",
		syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
			callCount.Add(1)
			return nil, cursor, errors.New("transient error")
		},
	}
	reg.Register(conn)

	sup := NewSupervisor(reg, nil)
	sup.maxSyncErrors = 1000 // high enough to not trip circuit breaker
	sup.backoffFactory = func() *Backoff {
		return &Backoff{BaseDelay: 1 * time.Millisecond, MaxDelay: 1 * time.Millisecond, MaxRetries: 2}
	}
	sup.SetConfig("c1", ConnectorConfig{SyncSchedule: "100ms"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "c1")

	// Wait for a few cycles — backoff exhausts after 2 retries, then waits for next cycle
	time.Sleep(500 * time.Millisecond)

	calls := callCount.Load()
	if calls < 3 {
		t.Fatalf("expected at least 3 calls (backoff retries + next cycle), got %d", calls)
	}

	cancel()
	sup.StopAll()
}

// ---------------------------------------------------------------------------
// No publisher — artifacts counted but not published
// ---------------------------------------------------------------------------

func TestSync_NoPublisher_NoError(t *testing.T) {
	reg := NewRegistry()
	syncDone := make(chan struct{}, 1)
	conn := &supervisorMockConnector{
		id: "c1",
		syncFunc: func(ctx context.Context, cursor string) ([]RawArtifact, string, error) {
			select {
			case syncDone <- struct{}{}:
			default:
			}
			if cursor == "" {
				return []RawArtifact{{SourceRef: "item1"}}, "c1", nil
			}
			<-ctx.Done()
			return nil, cursor, ctx.Err()
		},
	}
	reg.Register(conn)

	sup := NewSupervisor(reg, nil) // no publisher set
	sup.SetConfig("c1", ConnectorConfig{SyncSchedule: "1h"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup.StartConnector(ctx, "c1")
	<-syncDone
	time.Sleep(50 * time.Millisecond)

	// Should not crash — no publisher means items are counted but not published
	cancel()
	sup.StopAll()
}
