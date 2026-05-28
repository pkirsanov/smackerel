// Spec 061 SCOPE-04 — idle-sweep ticker.
//
// The ticker periodically calls Store.SweepIdle to drop conversation
// rows whose last_activity_at falls outside the configured TTL. Both
// the period (assistant.context.idle_sweep_interval) and the TTL
// (assistant.context.idle_timeout) come from SCOPE-01 SST. The ticker
// itself owns NO config — it accepts both durations as parameters so
// it stays pure for testing.

package assistantctx

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// IdleSweepTicker periodically calls Store.SweepIdle and logs the
// per-tick removal count. Start blocks until ctx is cancelled.
type IdleSweepTicker struct {
	store    Store
	idleTTL  time.Duration
	interval time.Duration
	logger   *slog.Logger
}

// NewIdleSweepTicker constructs a ticker. The logger is REQUIRED so
// operators can correlate sweep removals with capability-layer
// telemetry; a no-op handler may be supplied for tests.
func NewIdleSweepTicker(store Store, idleTTL, interval time.Duration, logger *slog.Logger) (*IdleSweepTicker, error) {
	if store == nil {
		return nil, errors.New("assistantctx: NewIdleSweepTicker requires a non-nil Store")
	}
	if idleTTL <= 0 {
		return nil, errors.New("assistantctx: NewIdleSweepTicker requires a positive idleTTL")
	}
	if interval <= 0 {
		return nil, errors.New("assistantctx: NewIdleSweepTicker requires a positive interval")
	}
	if logger == nil {
		return nil, errors.New("assistantctx: NewIdleSweepTicker requires a non-nil logger")
	}
	return &IdleSweepTicker{
		store:    store,
		idleTTL:  idleTTL,
		interval: interval,
		logger:   logger,
	}, nil
}

// Run loops on the configured interval until ctx is cancelled. Each
// tick invokes SweepIdle exactly once; errors are logged and the loop
// continues (do NOT exit on transient DB errors — the next tick may
// succeed and a panic-on-error stance would take down the assistant).
func (t *IdleSweepTicker) Run(ctx context.Context) {
	tk := time.NewTicker(t.interval)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			removed, err := t.store.SweepIdle(ctx, t.idleTTL)
			if err != nil {
				t.logger.Error("assistant idle sweep failed",
					slog.String("err", err.Error()),
					slog.Duration("idle_ttl", t.idleTTL),
				)
				continue
			}
			if removed > 0 {
				t.logger.Info("assistant idle sweep removed rows",
					slog.Int64("removed", removed),
					slog.Duration("idle_ttl", t.idleTTL),
				)
			}
		}
	}
}
