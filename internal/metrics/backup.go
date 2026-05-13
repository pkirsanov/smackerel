// Spec 048 — Backup and Restore Automation metrics.
//
// Three Prometheus metrics expose the most recent backup outcome so
// spec 049 alert rules can fire on missed/failed backups:
//
//   smackerel_backup_last_success_unixtime  (Gauge)
//   smackerel_backup_size_bytes              (Gauge)
//   smackerel_backup_runs_total{status}      (Counter)
//
// The Go core's backup watcher (internal/backup) reads the status file
// produced by scripts/commands/backup.sh and republishes these values.
//
// SmackerelBackupStale alert (config/prometheus/alerts.yml) consumes
// smackerel_backup_last_success_unixtime directly:
//
//   up{job="smackerel-core"} == 1
//   and
//   (time() - smackerel_backup_last_success_unixtime) > <stale_window_seconds>
//
// Label cardinality is bounded — `status` has exactly two values:
// "success" and "failed" (enforced by internal/backup.AllowedStatuses).

package metrics

import "github.com/prometheus/client_golang/prometheus"

// BackupLastSuccessUnixtime reports the Unix seconds timestamp of the
// most recent SUCCESSFUL backup. Zero means "no backup has succeeded
// since this process started" — the SmackerelBackupStale rule fires on
// `time() - 0 > stale_window`, which is always true. Operators learn
// about a missing backup pipeline within one scrape interval after the
// stale window elapses.
var BackupLastSuccessUnixtime = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "smackerel_backup_last_success_unixtime",
		Help: "Unix timestamp (seconds) of the most recent successful Smackerel backup, as reported by scripts/commands/backup.sh via the BACKUP_STATUS_FILE",
	},
)

// BackupSizeBytes reports the size in bytes of the most recent backup
// artifact (whether the run succeeded or failed; a failed run reports 0).
var BackupSizeBytes = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "smackerel_backup_size_bytes",
		Help: "Size in bytes of the most recent Smackerel backup artifact (0 if the most recent run failed)",
	},
)

// BackupRunsTotal counts backup runs by status. `status` is one of
// {"success", "failed"} — the closed set enforced by
// internal/backup.AllowedStatuses. The watcher gates the increment on
// last_run_unixtime advancing so a stable status file never inflates
// the counter.
var BackupRunsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_backup_runs_total",
		Help: "Total Smackerel backup runs by terminal status (success | failed)",
	},
	[]string{"status"},
)
