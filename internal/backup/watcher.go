// Spec 048 backup-status metrics watcher.
//
// The watcher polls the status file written by `scripts/commands/backup.sh`
// and republishes the most recent outcome to Prometheus. It is structured
// as a stateful struct so the unit test can drive the poll loop one tick
// at a time without sleeping.
//
// Failure modes:
//   - Status file missing → leave the gauge at 0. SmackerelBackupStale
//     fires after the configured stale window because (time() - 0) > window.
//   - Status file malformed → log a warning, do not crash, leave the gauge
//     at the previous value.
//
// The watcher emits exactly three metrics (defined in
// internal/metrics/backup.go):
//
//   - smackerel_backup_last_success_unixtime  (Gauge)
//   - smackerel_backup_size_bytes              (Gauge)
//   - smackerel_backup_runs_total{status}      (Counter, status in {success, failed})
//
// The counter is incremented only when last_run_unixtime advances, so the
// watcher does NOT inflate counts by re-reading an unchanged status file.

package backup

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"sync"
	"time"
)

// MetricSink is the small surface the watcher needs from the runtime
// metrics package. Using an interface keeps the watcher unit-testable
// without depending on the prometheus client_golang DefaultRegisterer.
type MetricSink interface {
	SetLastSuccessUnixtime(v float64)
	SetLastSizeBytes(v float64)
	IncRun(status string)
}

// Watcher polls the backup status file and republishes metrics.
//
// Zero-value Watcher is NOT usable; construct via NewWatcher.
type Watcher struct {
	path     string
	interval time.Duration
	sink     MetricSink

	// lastSeenRun is the last_run_unixtime value the watcher has already
	// reflected into the run counter. Used to gate IncRun so we do not
	// double-count a stable status file.
	mu          sync.Mutex
	lastSeenRun int64
}

// NewWatcher constructs a Watcher.
//
// `path` is the absolute (or repo-relative) BACKUP_STATUS_FILE path from
// SST. `interval` is the poll cadence; 60s is the recommended default.
// `sink` MUST NOT be nil — the watcher panics at construction time if it
// is, because a silent metrics watcher would defeat the alert contract.
func NewWatcher(path string, interval time.Duration, sink MetricSink) *Watcher {
	if sink == nil {
		panic("backup.NewWatcher: sink MUST NOT be nil")
	}
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Watcher{
		path:     path,
		interval: interval,
		sink:     sink,
	}
}

// Poll performs exactly one read of the status file and updates metrics.
//
// Returns the loaded Status (may be nil if the file does not exist) and
// any non-fatal error suitable for a slog.Warn call. The watcher loop
// uses Poll under the hood.
//
// Adversarial test note: Poll is idempotent for the gauge fields and
// strictly monotonic for the counter — calling Poll twice on the same
// unchanged status file increments smackerel_backup_runs_total exactly
// once.
func (w *Watcher) Poll() (*Status, error) {
	s, err := LoadStatus(w.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	w.sink.SetLastSuccessUnixtime(float64(s.LastSuccessUnixtime))
	w.sink.SetLastSizeBytes(float64(s.LastSizeBytes))

	w.mu.Lock()
	defer w.mu.Unlock()
	if s.LastRunUnixtime > w.lastSeenRun {
		if s.LastStatus != "" {
			w.sink.IncRun(s.LastStatus)
		}
		w.lastSeenRun = s.LastRunUnixtime
	}
	return s, nil
}

// Run starts the watcher's poll loop until ctx is canceled. The first
// poll runs immediately so the gauge is up-to-date the moment /metrics
// is first scraped after startup.
func (w *Watcher) Run(ctx context.Context) {
	if _, err := w.Poll(); err != nil {
		slog.Warn("backup watcher initial poll failed", "path", w.path, "error", err)
	}
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := w.Poll(); err != nil {
				slog.Warn("backup watcher poll failed", "path", w.path, "error", err)
			}
		}
	}
}

// LastSeenRun returns the most recent last_run_unixtime the watcher has
// reflected to metrics. Exposed for the watcher unit test only.
func (w *Watcher) LastSeenRun() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastSeenRun
}
