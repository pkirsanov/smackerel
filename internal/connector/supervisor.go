package connector

import (
	"context"
	"log/slog"
	"math/rand"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Supervisor manages connector goroutines with crash recovery.
type Supervisor struct {
	registry         *Registry
	stateStore       *StateStore
	publisher        ArtifactPublisher // optional: bridges RawArtifacts to NATS pipeline
	mu               sync.RWMutex
	wg               sync.WaitGroup // tracks running goroutines for graceful drain
	running          map[string]context.CancelFunc
	panicCounts      map[string]int             // circuit breaker: panic count per connector
	panicResetAt     map[string]time.Time       // time when panic count was first incremented
	stopped          bool                       // set by StopAll to prevent panic-recovery restarts
	connectorConfigs map[string]ConnectorConfig // per-connector config for schedule lookup
	maxSyncErrors    int                        // max consecutive sync errors before disabling (0 = default)
	backoffFactory   func() *Backoff            // optional: override backoff creation for testing
}

const (
	maxPanicsBeforeDisable          = 5                // max panics before circuit breaker trips
	panicWindowDuration             = 10 * time.Minute // rolling window for panic counting
	defaultMaxConsecutiveSyncErrors = 50               // max consecutive Sync() errors before disabling
)

// NewSupervisor creates a new connector supervisor.
// The optional publisher bridges connector-produced RawArtifacts into the
// processing pipeline. If nil, returned artifacts are counted but not published.
func NewSupervisor(registry *Registry, stateStore *StateStore) *Supervisor {
	return &Supervisor{
		registry:         registry,
		stateStore:       stateStore,
		running:          make(map[string]context.CancelFunc),
		panicCounts:      make(map[string]int),
		panicResetAt:     make(map[string]time.Time),
		connectorConfigs: make(map[string]ConnectorConfig),
	}
}

// SetPublisher sets the artifact publisher for bridging connector output to NATS.
func (s *Supervisor) SetPublisher(pub ArtifactPublisher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publisher = pub
}

// SetConfig records a connector's configuration for schedule lookup.
func (s *Supervisor) SetConfig(id string, cfg ConnectorConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connectorConfigs[id] = cfg
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

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runWithRecovery(ctx, connCtx, cancel, id)
	}()
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

// TriggerSync stops a running connector and restarts it, which causes
// an immediate sync cycle. If the connector is not running, it is started.
func (s *Supervisor) TriggerSync(ctx context.Context, id string) {
	s.mu.RLock()
	_, running := s.running[id]
	s.mu.RUnlock()

	if running {
		s.StopConnector(id)
	}
	s.StartConnector(ctx, id)
}

// StopAll stops all running connectors, waits for goroutines to drain, and clears the running map.
// Uses a bounded timeout (10s) to prevent blocking shutdown indefinitely if a connector's
// Sync() hangs on an unresponsive external API (IMP-022-R30-001).
func (s *Supervisor) StopAll() {
	s.mu.Lock()
	s.stopped = true
	for id, cancel := range s.running {
		cancel()
		delete(s.running, id)
	}
	s.mu.Unlock()

	// Wait for all goroutines to finish so downstream resources (NATS, DB)
	// are not closed while connectors are still in-flight.
	// Bounded at 10s to prevent indefinite hang if a Sync() call is stuck.
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// All connector goroutines exited cleanly
	case <-time.After(10 * time.Second):
		slog.Warn("connector supervisor stop timed out waiting for goroutines — proceeding with shutdown")
	}
}

// runWithRecovery runs a connector sync loop and recovers from panics.
// parentCtx is the original caller context used for restart; connCtx is the
// per-attempt child context that is cancelled on panic recovery so a fresh
// child can be created by StartConnector.
func (s *Supervisor) runWithRecovery(parentCtx context.Context, connCtx context.Context, connCancel context.CancelFunc, id string) {
	defer func() {
		if r := recover(); r != nil {
			// Cancel the stale connCtx before any restart so resources are released.
			connCancel()

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
			stopped := s.stopped
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
			if stopped {
				slog.Warn("skipping restart — supervisor stopped", "connector", id)
				return
			}

			// Skip restart if parent context is cancelled (shutdown in progress)
			if parentCtx.Err() != nil {
				slog.Warn("skipping restart during shutdown", "connector", id)
				return
			}

			// Jitter the restart delay (3-7s) to prevent thundering herd when
			// multiple connectors panic simultaneously (IMP-022-R30-002).
			restartDelay := 3*time.Second + time.Duration(rand.Int63n(int64(4*time.Second)))
			slog.Warn("restarting connector after panic",
				"connector", id,
				"panic_count", count,
				"max_before_disable", maxPanicsBeforeDisable,
				"restart_delay", restartDelay,
			)
			select {
			case <-parentCtx.Done():
				slog.Warn("skipping restart — context cancelled during delay", "connector", id)
				return
			case <-time.After(restartDelay):
			}
			// Re-check after sleep: shutdown may have started during the delay
			if s.stopped || parentCtx.Err() != nil {
				slog.Warn("skipping restart — supervisor stopped during delay", "connector", id)
				return
			}
			s.StartConnector(parentCtx, id)
		}
	}()

	conn, ok := s.registry.Get(id)
	if !ok {
		slog.Error("connector not found in registry", "connector", id)
		return
	}

	var backoff *Backoff
	if s.backoffFactory != nil {
		backoff = s.backoffFactory()
	} else {
		backoff = DefaultBackoff()
	}

	maxErrors := s.maxSyncErrors
	if maxErrors <= 0 {
		maxErrors = defaultMaxConsecutiveSyncErrors
	}
	var lastCursor string
	consecutiveErrors := 0

	for {
		select {
		case <-connCtx.Done():
			return
		default:
		}

		// Get current sync state; fall back to in-memory cursor on DB failure
		var cursor string
		if s.stateStore != nil {
			state, err := s.stateStore.Get(connCtx, id)
			if err == nil {
				cursor = state.SyncCursor
			} else if lastCursor != "" {
				cursor = lastCursor
				slog.Debug("using in-memory cursor after state store read failure", "connector", id)
			}
		} else if lastCursor != "" {
			cursor = lastCursor
		}

		// Run sync. Connectors return RawArtifacts; the supervisor publishes
		// them to the NATS processing pipeline via the ArtifactPublisher.
		items, newCursor, err := conn.Sync(connCtx, cursor)
		if err != nil {
			consecutiveErrors++
			slog.Error("connector sync failed",
				"connector", id,
				"error", err,
				"consecutive_errors", consecutiveErrors,
			)

			// Circuit breaker: disable connector after too many consecutive errors
			if consecutiveErrors >= maxErrors {
				slog.Error("connector sync error circuit breaker tripped — disabling",
					"connector", id,
					"consecutive_errors", consecutiveErrors,
				)
				s.mu.Lock()
				delete(s.running, id)
				s.mu.Unlock()
				return
			}

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
				// Wait for next scheduled cycle using configured interval
				interval := s.getSyncInterval(id)
				select {
				case <-connCtx.Done():
					return
				case <-time.After(interval):
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

		// Success — reset backoff and error counter
		backoff.Reset()
		consecutiveErrors = 0

		// Track cursor in memory as fallback for transient DB failures
		if newCursor != "" {
			lastCursor = newCursor
		}

		// Publish returned artifacts to the processing pipeline
		if s.publisher != nil && len(items) > 0 {
			published := 0
			for _, item := range items {
				if _, pubErr := s.publisher.PublishRawArtifact(connCtx, item); pubErr != nil {
					slog.Warn("failed to publish connector artifact",
						"connector", id,
						"source_ref", item.SourceRef,
						"error", pubErr,
					)
				} else {
					published++
				}
			}
			if published > 0 {
				slog.Info("connector artifacts published to pipeline",
					"connector", id,
					"published", published,
					"total", len(items),
				)
			}
		}

		if s.stateStore != nil {
			saveState := &SyncState{
				SourceID:    id,
				Enabled:     true,
				SyncCursor:  newCursor,
				ItemsSynced: len(items),
			}
			if err := s.stateStore.Save(connCtx, saveState); err != nil {
				slog.Warn("failed to save connector sync state", "connector", id, "error", err)
			}
		}

		slog.Info("connector sync complete",
			"connector", id,
			"items", len(items),
			"cursor", newCursor,
		)

		// Wait for next cycle using configured sync interval
		interval := s.getSyncInterval(id)
		slog.Debug("connector waiting for next sync cycle",
			"connector", id,
			"interval", interval,
		)
		select {
		case <-connCtx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// defaultSyncInterval is used when no per-connector schedule is configured.
const defaultSyncInterval = 5 * time.Minute

// getSyncInterval returns the sync interval for a connector from its config.
// Falls back to defaultSyncInterval when no schedule is configured.
func (s *Supervisor) getSyncInterval(id string) time.Duration {
	s.mu.RLock()
	cfg, ok := s.connectorConfigs[id]
	s.mu.RUnlock()

	if !ok {
		return defaultSyncInterval
	}

	// Try SyncSchedule field first
	if cfg.SyncSchedule != "" {
		if d := parseSyncInterval(cfg.SyncSchedule); d > 0 {
			return d
		}
	}

	// Try sync_interval from SourceConfig
	if cfg.SourceConfig != nil {
		if v, ok := cfg.SourceConfig["sync_interval"]; ok {
			if s, ok := v.(string); ok {
				if d := parseSyncInterval(s); d > 0 {
					return d
				}
			}
		}
	}

	return defaultSyncInterval
}

// parseSyncInterval parses a duration string or simplistic cron expression.
// Supported formats:
//   - Go duration: "30m", "4h", "1h30m"
//   - Cron minutes: "*/30 * * * *" → 30 minutes
//   - Cron hours: "0 */4 * * *" → 4 hours
func parseSyncInterval(s string) time.Duration {
	// Try Go duration first
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}

	// Try simplistic cron parsing
	fields := strings.Fields(s)
	if len(fields) != 5 {
		return 0
	}

	// Pattern: */N * * * * → every N minutes
	if strings.HasPrefix(fields[0], "*/") && fields[1] == "*" && fields[2] == "*" && fields[3] == "*" && fields[4] == "*" {
		if n, err := strconv.Atoi(fields[0][2:]); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}

	// Pattern: 0 */N * * * → every N hours
	if fields[0] == "0" && strings.HasPrefix(fields[1], "*/") && fields[2] == "*" && fields[3] == "*" && fields[4] == "*" {
		if n, err := strconv.Atoi(fields[1][2:]); err == nil && n > 0 {
			return time.Duration(n) * time.Hour
		}
	}

	return 0
}
