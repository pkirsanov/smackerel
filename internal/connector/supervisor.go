package connector

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Supervisor manages connector goroutines with crash recovery.
type Supervisor struct {
	registry    *Registry
	stateStore  *StateStore
	mu          sync.Mutex
	running     map[string]context.CancelFunc
}

// NewSupervisor creates a new connector supervisor.
func NewSupervisor(registry *Registry, stateStore *StateStore) *Supervisor {
	return &Supervisor{
		registry:   registry,
		stateStore: stateStore,
		running:    make(map[string]context.CancelFunc),
	}
}

// StartConnector starts a connector's sync loop in a supervised goroutine.
func (s *Supervisor) StartConnector(ctx context.Context, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, running := s.running[id]; running {
		return // Already running
	}

	connCtx, cancel := context.WithCancel(ctx)
	s.running[id] = cancel

	go s.runWithRecovery(connCtx, id)
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

// runWithRecovery runs a connector sync loop and recovers from panics.
func (s *Supervisor) runWithRecovery(ctx context.Context, id string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("connector panicked, restarting",
				"connector", id,
				"panic", r,
			)
			// Restart after a brief delay
			time.Sleep(5 * time.Second)
			s.mu.Lock()
			delete(s.running, id)
			s.mu.Unlock()
			s.StartConnector(ctx, id)
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
		case <-ctx.Done():
			return
		default:
		}

		// Get current sync state
		var cursor string
		if s.stateStore != nil {
			state, err := s.stateStore.Get(ctx, id)
			if err == nil {
				cursor = state.SyncCursor
			}
		}

		// Run sync
		items, newCursor, err := conn.Sync(ctx, cursor)
		if err != nil {
			slog.Error("connector sync failed",
				"connector", id,
				"error", err,
			)

			if s.stateStore != nil {
				_ = s.stateStore.RecordError(ctx, id, err.Error())
			}

			delay, hasMore := backoff.Next()
			if !hasMore {
				slog.Warn("connector max retries reached, skipping cycle",
					"connector", id,
				)
				backoff.Reset()
				// Wait for next scheduled cycle
				select {
				case <-ctx.Done():
					return
				case <-time.After(60 * time.Second):
				}
				continue
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			continue
		}

		// Success — reset backoff
		backoff.Reset()

		if s.stateStore != nil && len(items) > 0 {
			_ = s.stateStore.Save(ctx, &SyncState{
				SourceID:    id,
				Enabled:     true,
				SyncCursor:  newCursor,
				ItemsSynced: len(items),
			})
		}

		slog.Info("connector sync complete",
			"connector", id,
			"items", len(items),
			"cursor", newCursor,
		)

		// Wait for next cycle (connector-specific schedule)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Minute):
		}
	}
}
