// residualstore.go — spec 075 SCOPE-3 Postgres-backed residual usage
// store. Backs the rolling 7-day report (ResidualReportQuery) and
// implements ResidualTelemetry so the policy's Record() call writes
// both Prometheus and the durable per-day roll-up.
//
// UPSERT semantics: one row per (window_id, command, user_bucket,
// day). Repeat observations on the same day bump count and refresh
// last_seen_at; the day boundary is UTC midnight.
package legacyretirement

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgxResidualQuerier is the SQL boundary the store uses; the
// concrete implementation is *pgxpool.Pool, but a stub is allowed
// for unit tests that do not need a live database.
type pgxResidualQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// SQLResidualStore persists residual-usage observations into
// assistant_legacy_retirement_residual and answers the rolling
// 7-day report query.
type SQLResidualStore struct {
	db       pgxResidualQuerier
	clock    func() time.Time
	windowID string
}

// SQLResidualStoreConfig wires the SQL pool, window id, and clock.
type SQLResidualStoreConfig struct {
	Pool     *pgxpool.Pool
	WindowID string
	Clock    func() time.Time
}

// NewSQLResidualStore constructs a SQLResidualStore. Returns an
// error on nil pool, empty WindowID, or nil Clock (callers may pass
// time.Now explicitly — there is no hidden fallback per repo
// no-defaults policy).
func NewSQLResidualStore(cfg SQLResidualStoreConfig) (*SQLResidualStore, error) {
	if cfg.Pool == nil {
		return nil, fmt.Errorf("legacyretirement: NewSQLResidualStore: Pool is nil")
	}
	if cfg.WindowID == "" {
		return nil, fmt.Errorf("legacyretirement: NewSQLResidualStore: WindowID is empty")
	}
	if cfg.Clock == nil {
		return nil, fmt.Errorf("legacyretirement: NewSQLResidualStore: Clock is nil")
	}
	return &SQLResidualStore{db: cfg.Pool, clock: cfg.Clock, windowID: cfg.WindowID}, nil
}

// Record implements ResidualTelemetry. Writes one observation into
// the per-day roll-up. Bucket label normalisation matches the
// Prometheus path so the SQL store and /metrics agree.
//
// Errors are logged-via-return on the caller side; since
// ResidualTelemetry.Record has no error channel, persistence
// failures here are deliberately swallowed (the Prometheus path
// in the MultiResidualTelemetry fan-out remains authoritative for
// live dashboards). Callers that need persistence success guarantees
// should invoke RecordWithError directly.
func (s *SQLResidualStore) Record(command, userBucket string, outcome RetirementOutcome) {
	_ = s.RecordWithError(context.Background(), command, userBucket, outcome)
}

// RecordWithError is the error-returning variant used by tests and
// any caller that needs to surface persistence failures.
func (s *SQLResidualStore) RecordWithError(ctx context.Context, command, userBucket string, _ RetirementOutcome) error {
	if command == "" {
		return nil
	}
	bucket := normaliseBucketLabel(userBucket)
	now := s.clock().UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	const q = `
		INSERT INTO assistant_legacy_retirement_residual
		    (window_id, command, user_bucket, day, count, last_seen_at)
		VALUES ($1, $2, $3, $4, 1, $5)
		ON CONFLICT (window_id, command, user_bucket, day) DO UPDATE
		   SET count        = assistant_legacy_retirement_residual.count + 1,
		       last_seen_at = EXCLUDED.last_seen_at`
	if _, err := s.db.Exec(ctx, q, s.windowID, command, bucket, day, now); err != nil {
		return fmt.Errorf("legacyretirement: residual upsert: %w", err)
	}
	return nil
}

// RollingSevenDay implements ResidualReportQuery. Returns the
// per-day and per-command roll-up over [now-6, now] (UTC dates).
func (s *SQLResidualStore) RollingSevenDay(ctx context.Context, windowID string, now time.Time) (ResidualReport, error) {
	if windowID == "" {
		return ResidualReport{}, fmt.Errorf("legacyretirement: RollingSevenDay: windowID is empty")
	}
	nowUTC := now.UTC()
	end := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)
	start := end.AddDate(0, 0, -6)

	perDay, err := s.queryPerDay(ctx, windowID, start, end)
	if err != nil {
		return ResidualReport{}, err
	}
	perCommand, err := s.queryPerCommand(ctx, windowID, start, end)
	if err != nil {
		return ResidualReport{}, err
	}
	return ResidualReport{
		WindowID:   windowID,
		StartDay:   start,
		EndDay:     end,
		PerDay:     perDay,
		PerCommand: perCommand,
	}, nil
}

func (s *SQLResidualStore) queryPerDay(ctx context.Context, windowID string, start, end time.Time) ([]ResidualPerDayRow, error) {
	const q = `
		SELECT command,
		       day,
		       SUM(count)::bigint                  AS invocations,
		       COUNT(DISTINCT user_bucket)::bigint AS distinct_users
		  FROM assistant_legacy_retirement_residual
		 WHERE window_id = $1
		   AND day BETWEEN $2 AND $3
		 GROUP BY command, day
		 ORDER BY command, day`
	rows, err := s.db.Query(ctx, q, windowID, start, end)
	if err != nil {
		return nil, fmt.Errorf("legacyretirement: per-day query: %w", err)
	}
	defer rows.Close()
	var out []ResidualPerDayRow
	for rows.Next() {
		var r ResidualPerDayRow
		if err := rows.Scan(&r.Command, &r.Day, &r.Invocations, &r.DistinctUsers); err != nil {
			return nil, fmt.Errorf("legacyretirement: per-day scan: %w", err)
		}
		r.Day = r.Day.UTC()
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("legacyretirement: per-day rows: %w", err)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Command != out[j].Command {
			return out[i].Command < out[j].Command
		}
		return out[i].Day.Before(out[j].Day)
	})
	return out, nil
}

func (s *SQLResidualStore) queryPerCommand(ctx context.Context, windowID string, start, end time.Time) ([]ResidualPerCommandRow, error) {
	const q = `
		SELECT command,
		       SUM(count)::bigint                  AS invocations,
		       COUNT(DISTINCT user_bucket)::bigint AS distinct_users
		  FROM assistant_legacy_retirement_residual
		 WHERE window_id = $1
		   AND day BETWEEN $2 AND $3
		 GROUP BY command
		 ORDER BY command`
	rows, err := s.db.Query(ctx, q, windowID, start, end)
	if err != nil {
		return nil, fmt.Errorf("legacyretirement: per-command query: %w", err)
	}
	defer rows.Close()
	var out []ResidualPerCommandRow
	for rows.Next() {
		var r ResidualPerCommandRow
		if err := rows.Scan(&r.Command, &r.Invocations, &r.DistinctUsers); err != nil {
			return nil, fmt.Errorf("legacyretirement: per-command scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("legacyretirement: per-command rows: %w", err)
	}
	return out, nil
}
