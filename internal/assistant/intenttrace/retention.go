// Spec 071 SCOPE-02 — Retention sweep runner (SCN-071-A09).
//
// RunRetentionSweep ticks at the SST-configured interval and asks the
// store to delete rows whose expires_at <= now. The sweep itself is
// observable via a structured log entry + Prometheus counter so
// operators can detect a stuck sweep (counts only — raw row content
// never leaves the store).

package intenttrace

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// RunRetentionSweep blocks until ctx is cancelled. It is intended to
// be invoked in its own goroutine from cmd/core wiring.
func RunRetentionSweep(ctx context.Context, store IntentTraceStore, interval time.Duration, now func() time.Time) error {
	if store == nil {
		return errors.New("intenttrace: RunRetentionSweep requires a non-nil store")
	}
	if interval <= 0 {
		return errors.New("intenttrace: RunRetentionSweep requires interval > 0")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	tk := time.NewTicker(interval)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tk.C:
			res, err := store.SweepExpired(ctx, now())
			if err != nil {
				slog.WarnContext(ctx, "assistant_intent_trace_retention_sweep_failed", "error", err)
				IntentTraceRetentionSweepRowsTotal.WithLabelValues("error").Inc()
				continue
			}
			slog.InfoContext(ctx, "assistant_intent_trace_retention_sweep",
				"deleted", res.Deleted,
				"swept_at", res.SweptAt.Format("2006-01-02T15:04:05.000Z07:00"),
			)
			if res.Deleted > 0 {
				IntentTraceRetentionSweepRowsTotal.WithLabelValues("deleted").Add(float64(res.Deleted))
			} else {
				IntentTraceRetentionSweepRowsTotal.WithLabelValues("noop").Inc()
			}
		}
	}
}
