// Spec 048 — backup metrics adapter.
//
// The Go core wires this adapter into internal/backup.Watcher so the
// watcher can update Prometheus metrics through the small MetricSink
// interface without importing the prometheus client_golang package
// (keeps internal/backup unit-testable in isolation).

package metrics

// BackupMetricsSink is the production implementation of the
// internal/backup.MetricSink interface. It bridges Watcher.Poll calls
// to the registered Prometheus collectors defined in backup.go.
type BackupMetricsSink struct{}

// NewBackupMetricsSink constructs a sink that updates the registered
// backup metrics. The sink is a value type with no state — multiple
// instances are interchangeable.
func NewBackupMetricsSink() BackupMetricsSink { return BackupMetricsSink{} }

// SetLastSuccessUnixtime implements internal/backup.MetricSink.
func (BackupMetricsSink) SetLastSuccessUnixtime(v float64) {
	BackupLastSuccessUnixtime.Set(v)
}

// SetLastSizeBytes implements internal/backup.MetricSink.
func (BackupMetricsSink) SetLastSizeBytes(v float64) {
	BackupSizeBytes.Set(v)
}

// IncRun implements internal/backup.MetricSink. `status` is one of
// {"success", "failed"} — enforced by the Watcher before this method
// is called (see internal/backup.AllowedStatuses).
func (BackupMetricsSink) IncRun(status string) {
	BackupRunsTotal.WithLabelValues(status).Inc()
}
