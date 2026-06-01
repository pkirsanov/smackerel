// observationreport.go — spec 075 SCOPE-5 observation report
// producer that gates spec 066 final retired-handler deletion.
//
// SCN-075-A08: after window_state=closed for at least
// legacy_retirement.post_window_observation_days, the operator
// generates an observation snapshot. The snapshot records the
// retired-handler invocation count over the observation interval
// (sourced from the closed-state safety counter,
// RetiredHandlerInvocationCounter) and is persisted to
// assistant_legacy_retirement_observations. EligibleForFinalDeletion
// returns true only when the snapshot count is zero AND the
// observation interval is at least the configured minimum.
//
// Scope boundary: SCOPE-1 declares the seam (ObservationReport
// interface + ObservationSnapshot.EligibleForFinalDeletion). This
// file wires the SQL persister and the prometheus-backed
// invocation count source. The deletion itself is owned by spec 066.
package legacyretirement

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// observationSchemaVersion pins the schema_version written to
// assistant_legacy_retirement_observations.
const observationSchemaVersion = 1

// RetiredHandlerInvocationSource returns the total number of
// retired-handler invocations observed during interval [start, end].
// The interval-aware contract lets future implementations source
// from a durable log; the prometheus-backed default returns the
// monotonic counter total at call time (sufficient when the closed
// guard is in place and the process has not been restarted within
// the observation window — operators document any restart that
// resets the counter).
type RetiredHandlerInvocationSource interface {
	CountInvocations(ctx context.Context, start, end time.Time) (int, error)
}

// PrometheusInvocationSource is the default
// RetiredHandlerInvocationSource. It sums the RetiredHandlerInvocationCounter
// metric family from the supplied Gatherer (defaults to the
// process-global registry).
type PrometheusInvocationSource struct {
	Gatherer prometheus.Gatherer
}

// CountInvocations implements RetiredHandlerInvocationSource.
func (s *PrometheusInvocationSource) CountInvocations(ctx context.Context, _, _ time.Time) (int, error) {
	g := s.Gatherer
	if g == nil {
		g = prometheus.DefaultGatherer
	}
	families, err := g.Gather()
	if err != nil {
		return 0, fmt.Errorf("legacyretirement: gather metric families: %w", err)
	}
	var total float64
	for _, mf := range families {
		if mf.GetName() != MetricNameRetiredHandlerInvocation {
			continue
		}
		for _, m := range mf.GetMetric() {
			if c := m.GetCounter(); c != nil {
				total += c.GetValue()
			}
		}
	}
	if total < 0 {
		return 0, fmt.Errorf("legacyretirement: negative invocation total %f for %s", total, MetricNameRetiredHandlerInvocation)
	}
	if total > float64(int(^uint(0)>>1)) {
		return 0, fmt.Errorf("legacyretirement: invocation total %f exceeds int range", total)
	}
	return int(total), nil
}

var _ RetiredHandlerInvocationSource = (*PrometheusInvocationSource)(nil)
var _ = (*dto.Metric)(nil) // keep dto import referenced for future extensions

// pgxObservationQuerier is the SQL boundary the SQLObservationReport
// uses; *pgxpool.Pool satisfies it and unit tests can stub it.
type pgxObservationQuerier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// SQLObservationReport persists ObservationSnapshot rows into
// assistant_legacy_retirement_observations.
type SQLObservationReport struct {
	db     pgxObservationQuerier
	source RetiredHandlerInvocationSource
	clock  func() time.Time
	newID  func() string
}

// SQLObservationReportConfig wires the SQL pool, invocation source,
// and clock. All fields are required — no in-source defaults
// (Gate G028).
type SQLObservationReportConfig struct {
	Pool   *pgxpool.Pool
	Source RetiredHandlerInvocationSource
	Clock  func() time.Time
}

// NewSQLObservationReport constructs a SQLObservationReport.
func NewSQLObservationReport(cfg SQLObservationReportConfig) (*SQLObservationReport, error) {
	if cfg.Pool == nil {
		return nil, fmt.Errorf("legacyretirement: NewSQLObservationReport: Pool is nil")
	}
	if cfg.Source == nil {
		return nil, fmt.Errorf("legacyretirement: NewSQLObservationReport: Source is nil (use &PrometheusInvocationSource{} for the default)")
	}
	if cfg.Clock == nil {
		return nil, fmt.Errorf("legacyretirement: NewSQLObservationReport: Clock is nil")
	}
	return &SQLObservationReport{
		db:     cfg.Pool,
		source: cfg.Source,
		clock:  cfg.Clock,
		newID:  randomReportID,
	}, nil
}

// Generate implements ObservationReport. Builds and persists a
// fresh snapshot for windowID covering [now-observationDuration, now].
func (r *SQLObservationReport) Generate(ctx context.Context, windowID string, observationDuration time.Duration) (ObservationSnapshot, error) {
	if windowID == "" {
		return ObservationSnapshot{}, fmt.Errorf("legacyretirement: Generate: windowID empty")
	}
	if observationDuration <= 0 {
		return ObservationSnapshot{}, fmt.Errorf("legacyretirement: Generate: observationDuration must be positive, got %s", observationDuration)
	}
	now := r.clock().UTC()
	start := now.Add(-observationDuration)

	count, err := r.source.CountInvocations(ctx, start, now)
	if err != nil {
		return ObservationSnapshot{}, fmt.Errorf("legacyretirement: count invocations for window %q: %w", windowID, err)
	}
	if count < 0 {
		return ObservationSnapshot{}, fmt.Errorf("legacyretirement: negative invocation count %d for window %q", count, windowID)
	}

	snap := ObservationSnapshot{
		ReportID:                  r.newID(),
		WindowID:                  windowID,
		ObservationStartedAt:      start,
		ObservationEndedAt:        now,
		RetiredHandlerInvocations: count,
		GeneratedAt:               now,
		SchemaVersion:             observationSchemaVersion,
	}

	const q = `
		INSERT INTO assistant_legacy_retirement_observations
		    (report_id, window_id, observation_started_at,
		     observation_ended_at, retired_handler_invocations,
		     generated_at, schema_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	if _, err := r.db.Exec(ctx, q,
		snap.ReportID, snap.WindowID, snap.ObservationStartedAt,
		snap.ObservationEndedAt, snap.RetiredHandlerInvocations,
		snap.GeneratedAt, snap.SchemaVersion); err != nil {
		return ObservationSnapshot{}, fmt.Errorf("legacyretirement: observation insert for window %q: %w", windowID, err)
	}
	return snap, nil
}

// LatestSnapshot returns the most recent persisted ObservationSnapshot
// for windowID, or (zero, false, nil) when no snapshot exists. Used
// by the deletion gate to look up the current observation result
// without forcing a fresh Generate.
func (r *SQLObservationReport) LatestSnapshot(ctx context.Context, windowID string) (ObservationSnapshot, bool, error) {
	if windowID == "" {
		return ObservationSnapshot{}, false, fmt.Errorf("legacyretirement: LatestSnapshot: windowID empty")
	}
	const q = `
		SELECT report_id, window_id, observation_started_at,
		       observation_ended_at, retired_handler_invocations,
		       generated_at, schema_version
		  FROM assistant_legacy_retirement_observations
		 WHERE window_id = $1
		 ORDER BY generated_at DESC
		 LIMIT 1`
	var s ObservationSnapshot
	err := r.db.QueryRow(ctx, q, windowID).Scan(
		&s.ReportID, &s.WindowID, &s.ObservationStartedAt,
		&s.ObservationEndedAt, &s.RetiredHandlerInvocations,
		&s.GeneratedAt, &s.SchemaVersion,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ObservationSnapshot{}, false, nil
		}
		return ObservationSnapshot{}, false, fmt.Errorf("legacyretirement: LatestSnapshot select: %w", err)
	}
	s.ObservationStartedAt = s.ObservationStartedAt.UTC()
	s.ObservationEndedAt = s.ObservationEndedAt.UTC()
	s.GeneratedAt = s.GeneratedAt.UTC()
	return s, true, nil
}

// DeletionGateResult is the structured answer to "is the spec 066
// final deletion eligible to proceed?"
type DeletionGateResult struct {
	Eligible        bool
	Reason          string
	Snapshot        ObservationSnapshot
	HasSnapshot     bool
	MinObservation  time.Duration
	StaleReferences []string
}

// EvaluateDeletionGate combines the latest snapshot's
// EligibleForFinalDeletion check with the supplied stale-reference
// findings. Eligibility requires: a persisted snapshot exists, its
// retired_handler_invocations is zero, the observation interval is
// at least minObservation, and the staleReferences slice is empty.
func EvaluateDeletionGate(snapshot ObservationSnapshot, hasSnapshot bool, minObservation time.Duration, staleReferences []string) DeletionGateResult {
	result := DeletionGateResult{
		Snapshot:        snapshot,
		HasSnapshot:     hasSnapshot,
		MinObservation:  minObservation,
		StaleReferences: staleReferences,
	}
	if !hasSnapshot {
		result.Reason = "no observation snapshot persisted for window"
		return result
	}
	if !snapshot.EligibleForFinalDeletion(minObservation) {
		if snapshot.RetiredHandlerInvocations != 0 {
			result.Reason = fmt.Sprintf("observation snapshot reports %d retired-handler invocations; must be 0", snapshot.RetiredHandlerInvocations)
		} else {
			result.Reason = fmt.Sprintf("observation interval %s shorter than required %s", snapshot.ObservationEndedAt.Sub(snapshot.ObservationStartedAt), minObservation)
		}
		return result
	}
	if len(staleReferences) > 0 {
		result.Reason = fmt.Sprintf("%d stale first-party reference(s) to retired commands remain", len(staleReferences))
		return result
	}
	result.Eligible = true
	result.Reason = "snapshot count=0 over required observation window; no stale references"
	return result
}

// randomReportID returns a 16-byte hex report id. Crypto/rand is
// used so concurrent calls cannot collide on the primary key.
func randomReportID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read on Linux backed by getrandom never fails;
		// degrade to a clock-derived id only if it ever does.
		return fmt.Sprintf("obs-%d", time.Now().UTC().UnixNano())
	}
	return "obs-" + hex.EncodeToString(b[:])
}
