package intelligence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BUG-004-004 SCOPE-04 — the canonical read model.
//
// Health previously derived its verdict from GetLastSynthesisTime, which reads
// MAX(created_at) FROM synthesis_insights -- the LEGACY table. Nothing writes
// that table any more, so health would report never-run forever while durable
// output accumulated in synthesis_outputs. This model reads the tables the
// producer actually commits to, so the health verdict and the stored truth
// cannot drift apart.
//
// It is deliberately the ONLY place that assembles a SynthesisPersistenceOutcome
// from the database. A second assembler would be a second answer to "what is
// the state of synthesis", which is the class of divergence this packet exists
// to remove.
type SynthesisReadModel struct {
	pool *pgxpool.Pool
}

// NewSynthesisReadModel constructs the read model. A nil pool is refused rather
// than deferred to first query, so a misconfigured wiring fails at startup.
func NewSynthesisReadModel(pool *pgxpool.Pool) (*SynthesisReadModel, error) {
	if pool == nil {
		return nil, errors.New("synthesis read model requires a database pool")
	}
	return &SynthesisReadModel{pool: pool}, nil
}

// LatestOutcome returns the durable facts DeriveSynthesisHealth maps to a
// verdict: the latest attempt's phase, the read-back result, the committed
// output kind, staleness against the freshness budget, and whether any prior
// verified output exists.
//
// A query error surfaces as PhaseProbeError rather than as a Go error, because
// the health mapping already treats a probe failure as "cannot claim healthy" --
// which is the honest answer. Returning an error instead would tempt a caller
// into treating an unreadable database as fine.
func (m *SynthesisReadModel) LatestOutcome(
	ctx context.Context,
	freshnessBudget time.Duration,
	now time.Time,
) SynthesisPersistenceOutcome {
	// The latest COMMITTED output. Read-back is implied OK for a row that exists,
	// because Commit gates on read-back before the transaction is allowed to
	// stand -- an output that failed read-back never became a row.
	var outputKind string
	var createdAt time.Time
	err := m.pool.QueryRow(ctx, `
		SELECT o.output_kind, o.created_at
		FROM synthesis_outputs o
		JOIN synthesis_runs r ON r.id = o.run_id
		WHERE r.state = 'succeeded'
		ORDER BY o.created_at DESC
		LIMIT 1`).Scan(&outputKind, &createdAt)

	hasOutput := true
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		hasOutput = false
	case err != nil:
		return SynthesisPersistenceOutcome{Phase: PhaseProbeError}
	}

	// The latest attempt, which is what distinguishes never-ran from failed.
	// Without it a failed run and a fresh install would read identically.
	var outcome string
	attemptErr := m.pool.QueryRow(ctx, `
		SELECT outcome FROM synthesis_run_attempts
		ORDER BY recorded_at DESC, id DESC
		LIMIT 1`).Scan(&outcome)

	hasAttempt := true
	switch {
	case errors.Is(attemptErr, pgx.ErrNoRows):
		hasAttempt = false
	case attemptErr != nil:
		return SynthesisPersistenceOutcome{Phase: PhaseProbeError}
	}

	switch {
	case !hasAttempt && !hasOutput:
		// Nothing has ever run. Never-run is its own state, not a degraded
		// success and not a failure.
		return SynthesisPersistenceOutcome{Phase: PhaseNoRun}

	case outcome == string(AttemptFailed):
		// A failure does not become healthy because an older output survives, so
		// the prior output is reported alongside the failure rather than instead
		// of it. The mapping decides what that combination means.
		return SynthesisPersistenceOutcome{
			Phase:                  PhaseWriteFailed,
			ReadBack:               ReadBackNotAttempted,
			HasPriorVerifiedOutput: hasOutput,
		}

	case !hasOutput:
		// Attempts exist but nothing committed. Treating this as never-run would
		// erase the fact that work was tried and produced nothing durable.
		return SynthesisPersistenceOutcome{
			Phase:    PhaseWriteFailed,
			ReadBack: ReadBackNotAttempted,
		}
	}

	return SynthesisPersistenceOutcome{
		Phase:                  PhaseCommitted,
		ReadBack:               ReadBackOK,
		Output:                 SynthesisOutputKind(outputKind),
		Stale:                  now.Sub(createdAt) > freshnessBudget,
		HasPriorVerifiedOutput: true,
	}
}

// SynthesisLatest is the safe, content-free description of the newest durable
// output. It carries counts, identity and window -- never through-lines, source
// titles or artifact text, so it is safe to log, label a metric with, or return
// to a reader who may not see synthesis content.
type SynthesisLatest struct {
	OutputID               string
	LogicalKey             string
	Cadence                string
	Kind                   SynthesisOutputKind
	InsightCount           int
	CitationCount          int
	EvaluatedArtifactCount int
	WindowStart            time.Time
	WindowEnd              time.Time
	LifecycleState         string
	CreatedAt              time.Time
}

// Latest returns the newest verified output, or false when none exists.
//
// The false is the point: a caller must be able to tell "no synthesis yet" from
// "synthesis with zero insights". Returning a zero-valued struct for both would
// collapse never-run into quiet, which is exactly the confusion SCN-05 forbids.
func (m *SynthesisReadModel) Latest(ctx context.Context) (SynthesisLatest, bool, error) {
	var l SynthesisLatest
	err := m.pool.QueryRow(ctx, `
		SELECT o.id, r.logical_key, r.cadence, o.output_kind,
		       o.insight_count, o.citation_count, o.evaluated_artifact_count,
		       r.window_start, r.window_end, r.lifecycle_state, o.created_at
		FROM synthesis_outputs o
		JOIN synthesis_runs r ON r.id = o.run_id
		WHERE r.state = 'succeeded'
		ORDER BY o.created_at DESC
		LIMIT 1`).Scan(
		&l.OutputID, &l.LogicalKey, &l.Cadence, &l.Kind,
		&l.InsightCount, &l.CitationCount, &l.EvaluatedArtifactCount,
		&l.WindowStart, &l.WindowEnd, &l.LifecycleState, &l.CreatedAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return SynthesisLatest{}, false, nil
	case err != nil:
		return SynthesisLatest{}, false, fmt.Errorf("read latest synthesis output: %w", err)
	}
	return l, true, nil
}

// SynthesisHistoryEntry is one row of run history. Like SynthesisLatest it
// carries counts and identity but no synthesis text, so a history listing can
// be rendered, logged or paged without deciding who may read content.
type SynthesisHistoryEntry struct {
	OutputID       string
	LogicalKey     string
	Cadence        string
	Kind           SynthesisOutputKind
	InsightCount   int
	CitationCount  int
	WindowStart    time.Time
	WindowEnd      time.Time
	LifecycleState string
	CreatedAt      time.Time
}

// maxSynthesisHistoryLimit bounds a page. An unbounded listing would let one
// request pull the entire history into memory, so the cap is enforced here
// rather than trusted from the caller.
const maxSynthesisHistoryLimit = 100

// History returns verified outputs newest-first, capped at limit.
//
// A limit at or below zero is corrected to the cap rather than rejected: the
// caller asking for "everything" should get a bounded page, not an error, and
// definitely not everything.
func (m *SynthesisReadModel) History(ctx context.Context, limit int) ([]SynthesisHistoryEntry, error) {
	if limit <= 0 || limit > maxSynthesisHistoryLimit {
		limit = maxSynthesisHistoryLimit
	}

	rows, err := m.pool.Query(ctx, `
		SELECT o.id, r.logical_key, r.cadence, o.output_kind,
		       o.insight_count, o.citation_count,
		       r.window_start, r.window_end, r.lifecycle_state, o.created_at
		FROM synthesis_outputs o
		JOIN synthesis_runs r ON r.id = o.run_id
		WHERE r.state = 'succeeded'
		ORDER BY o.created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("read synthesis history: %w", err)
	}
	defer rows.Close()

	var out []SynthesisHistoryEntry
	for rows.Next() {
		var e SynthesisHistoryEntry
		if err := rows.Scan(&e.OutputID, &e.LogicalKey, &e.Cadence, &e.Kind,
			&e.InsightCount, &e.CitationCount,
			&e.WindowStart, &e.WindowEnd, &e.LifecycleState, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan synthesis history: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate synthesis history: %w", err)
	}
	return out, nil
}

// SynthesisMetricState is the exclusive numeric state a scraper sees. The
// mapping lives here, next to the read model that produces the outcome, so the
// metric and the /api/health verdict cannot drift apart -- they are two
// renderings of one derivation, not two derivations.
//
// Values mirror internal/metrics.SynthesisState*; they are ints rather than a
// dependency on that package because internal/metrics already imports nothing
// from intelligence and reversing that would create a cycle.
const (
	SynthesisMetricNeverRun   = 0
	SynthesisMetricUp         = 1
	SynthesisMetricStale      = 2
	SynthesisMetricPartial    = 3
	SynthesisMetricFailed     = 4
	SynthesisMetricProbeError = 5
)

// MetricStateFor maps a durable outcome to its exclusive scrape value.
//
// Order matters and is not arbitrary. Probe-error outranks everything because
// an unreadable database means the other fields describe nothing. Failure
// outranks staleness because a current failure is the more urgent fact. Partial
// outranks up because a partial output is durable and honest but never full
// health.
func MetricStateFor(outcome SynthesisPersistenceOutcome) int {
	switch {
	case outcome.Phase == PhaseProbeError:
		return SynthesisMetricProbeError
	case outcome.Phase == PhaseNoRun:
		return SynthesisMetricNeverRun
	case outcome.Phase == PhaseWriteFailed:
		return SynthesisMetricFailed
	case outcome.Phase != PhaseCommitted || outcome.ReadBack != ReadBackOK:
		// Committed-but-unverified is not a success. Treating it as one is the
		// class of claim this packet exists to remove.
		return SynthesisMetricFailed
	case outcome.Stale:
		return SynthesisMetricStale
	case outcome.Output == OutputKindPartial:
		return SynthesisMetricPartial
	default:
		return SynthesisMetricUp
	}
}
