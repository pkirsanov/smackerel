package connector

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// Supervisor manages connector goroutines with crash recovery.
type Supervisor struct {
	registry     *Registry
	stateStore   *StateStore
	mu           sync.Mutex
	running      map[string]context.CancelFunc
	panicCounts  map[string]int       // circuit breaker: panic count per connector
	panicResetAt map[string]time.Time // time when panic count was first incremented
	stopped      bool                 // set by StopAll to prevent panic-recovery restarts
}

const (
	maxPanicsBeforeDisable = 5                // max panics before circuit breaker trips
	panicWindowDuration    = 10 * time.Minute // rolling window for panic counting
)

// NewSupervisor creates a new connector supervisor.
func NewSupervisor(registry *Registry, stateStore *StateStore) *Supervisor {
	return &Supervisor{
		registry:     registry,
		stateStore:   stateStore,
		running:      make(map[string]context.CancelFunc),
		panicCounts:  make(map[string]int),
		panicResetAt: make(map[string]time.Time),
	}
}

// StartConnector starts a connector's sync loop in a supervised goroutine.
func (s *Supervisor) StartConnector(ctx context.Context, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return // Supervisor has been stopped, reject new starts
	}

	if _, running := s.running[id]; running {
		return // Already running
	}

	connCtx, cancel := context.WithCancel(ctx)
	s.running[id] = cancel

	go s.runWithRecovery(ctx, connCtx, id)
}

// StopConnector stops a running connector.
func (s *Supervisor) StopConnector(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cancel, ok := s.running[id]; ok {
		cancel()
		delete(s.running, id)
	}
}

// StopAll stops all running connectors and clears the running map.
func (s *Supervisor) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopped = true
	for id, cancel := range s.running {
		cancel()
		delete(s.running, id)
	}
}

// runWithRecovery runs a connector sync loop and recovers from panics.
// parentCtx is the original caller context used for restart; connCtx is the
// per-attempt child context that is cancelled on panic recovery so a fresh
// child can be created by StartConnector.
func (s *Supervisor) runWithRecovery(parentCtx context.Context, connCtx context.Context, id string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("connector panicked",
				"connector", id,
				"panic", r,
				"stack", string(debug.Stack()),
			)

			// Circuit breaker: count panics and disable if threshold exceeded
			s.mu.Lock()
			now := time.Now()
			if resetAt, ok := s.panicResetAt[id]; ok && now.Sub(resetAt) > panicWindowDuration {
				// Reset window
				s.panicCounts[id] = 0
				s.panicResetAt[id] = now
			}
			if _, ok := s.panicResetAt[id]; !ok {
				s.panicResetAt[id] = now
			}
			s.panicCounts[id]++
			count := s.panicCounts[id]
			delete(s.running, id)
			s.mu.Unlock()

			if count >= maxPanicsBeforeDisable {
				slog.Error("connector circuit breaker tripped — too many panics, disabling",
					"connector", id,
					"panic_count", count,
					"window", panicWindowDuration,
				)
				return // Do NOT restart
			}

			// Skip restart if supervisor has been stopped
			if s.stopped {
				slog.Warn("skipping restart — supervisor stopped", "connector", id)
				return
			}

			// Skip restart if parent context is cancelled (shutdown in progress)
			if parentCtx.Err() != nil {
				slog.Warn("skipping restart during shutdown", "connector", id)
				return
			}

			slog.Warn("restarting connector after panic",
				"connector", id,
				"panic_count", count,
				"max_before_disable", maxPanicsBeforeDisable,
			)
			time.Sleep(5 * time.Second)
			s.StartConnector(parentCtx, id)
		}
	}()

	conn, ok := s.registry.Get(id)
	if !ok {
		slog.Error("connector not found in registry", "connector", id)
		return
	}

	backoff := DefaultBackoff()

	for {
		select {
		case <-connCtx.Done():
			return
		default:
		}

		// Get current sync state
		var cursor string
		if s.stateStore != nil {
			state, err := s.stateStore.Get(connCtx, id)
			if err == nil {
				cursor = state.SyncCursor
			}
		}

		// Run sync
		items, newCursor, err := conn.Sync(connCtx, cursor)
		if err != nil {
			slog.Error("connector sync failed",
				"connector", id,
				"error", err,
			)

			if s.stateStore != nil {
				if err := s.stateStore.RecordError(connCtx, id, err.Error()); err != nil {
					slog.Warn("failed to record connector error in state store", "connector", id, "error", err)
				}
			}

			delay, hasMore := backoff.Next()
			if !hasMore {
				slog.Warn("connector max retries reached, skipping cycle",
					"connector", id,
				)
				backoff.Reset()
				// Wait for next scheduled cycle
				select {
				case <-connCtx.Done():
					return
				case <-time.After(60 * time.Second):
				}
				continue
			}

			select {
			case <-connCtx.Done():
				return
			case <-time.After(delay):
			}
			continue
		}

		// Success — reset backoff
		backoff.Reset()

		if s.stateStore != nil && len(items) > 0 {
			if err := s.stateStore.Save(connCtx, &SyncState{
				SourceID:    id,
				Enabled:     true,
				SyncCursor:  newCursor,
				ItemsSynced: len(items),
			}); err != nil {
				slog.Warn("failed to save connector sync state", "connector", id, "error", err)
			}
		}

		slog.Info("connector sync complete",
			"connector", id,
			"items", len(items),
			"cursor", newCursor,
		)

		// Wait for next cycle (connector-specific schedule)
		select {
		case <-connCtx.Done():
			return
		case <-time.After(5 * time.Minute):
		}
	}
}
