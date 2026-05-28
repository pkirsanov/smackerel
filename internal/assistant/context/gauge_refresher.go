// Spec 061 SCOPE-09 — active-threads gauge refresher.
//
// The smackerel_assistant_active_threads gauge (defined in
// internal/assistant/metrics/metrics.go) records the number of live
// assistant_conversations rows per transport. Because the gauge is a
// snapshot rather than a counter, it MUST be re-Set on a periodic
// cadence so dashboards reflect "rows currently in the store" rather
// than "rows seen at the last persist".
//
// ActiveThreadsRefresher samples Store.CountActiveByTransport on a
// fixed interval and pushes per-transport counts through the supplied
// setGauge callback. The callback indirection keeps this package free
// of an import on internal/assistant/metrics — the cmd/core wiring
// closes over assistantmetrics.ActiveThreadsGauge.
//
// Transports with zero rows MUST still receive a Set(0) so the gauge
// drops when a transport empties out. The caller supplies the closed
// transport vocabulary (typically assistantmetrics.AllTransports).

package assistantctx

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// ActiveThreadsRefresher periodically refreshes the per-transport
// active-threads gauge. Run blocks until ctx is cancelled.
type ActiveThreadsRefresher struct {
	store      Store
	transports []string
	interval   time.Duration
	setGauge   func(transport string, count float64)
	logger     *slog.Logger
}

// NewActiveThreadsRefresher constructs a refresher.
//
// transports is the closed transport vocabulary the refresher will
// emit a Set() for on every tick (zero-fill for absent transports).
// setGauge is invoked once per transport per tick. logger receives
// per-error and per-warning lines.
//
// All parameters are required.
func NewActiveThreadsRefresher(
	store Store,
	transports []string,
	interval time.Duration,
	setGauge func(transport string, count float64),
	logger *slog.Logger,
) (*ActiveThreadsRefresher, error) {
	if store == nil {
		return nil, errors.New("assistantctx: NewActiveThreadsRefresher requires a non-nil Store")
	}
	if len(transports) == 0 {
		return nil, errors.New("assistantctx: NewActiveThreadsRefresher requires a non-empty transports vocabulary")
	}
	if interval <= 0 {
		return nil, errors.New("assistantctx: NewActiveThreadsRefresher requires a positive interval")
	}
	if setGauge == nil {
		return nil, errors.New("assistantctx: NewActiveThreadsRefresher requires a non-nil setGauge callback")
	}
	if logger == nil {
		return nil, errors.New("assistantctx: NewActiveThreadsRefresher requires a non-nil logger")
	}
	transportsCopy := make([]string, len(transports))
	copy(transportsCopy, transports)
	return &ActiveThreadsRefresher{
		store:      store,
		transports: transportsCopy,
		interval:   interval,
		setGauge:   setGauge,
		logger:     logger,
	}, nil
}

// Run refreshes the gauge once immediately, then on the configured
// interval until ctx is cancelled. Errors are logged and the loop
// continues (next tick may succeed; failing the loop would freeze the
// gauge at a stale value, which is worse than briefly missing a
// sample).
func (r *ActiveThreadsRefresher) Run(ctx context.Context) {
	r.Refresh(ctx)
	tk := time.NewTicker(r.interval)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			r.Refresh(ctx)
		}
	}
}

// Refresh performs one sampling pass. Exposed for tests that drive
// the refresher synchronously without spinning the Run goroutine.
func (r *ActiveThreadsRefresher) Refresh(ctx context.Context) {
	counts, err := r.store.CountActiveByTransport(ctx)
	if err != nil {
		r.logger.Error("assistant active-threads refresh failed",
			slog.String("err", err.Error()),
		)
		return
	}
	for _, t := range r.transports {
		r.setGauge(t, float64(counts[t]))
	}
}
