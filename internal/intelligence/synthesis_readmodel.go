package intelligence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

// SynthesisReadQuery identifies one causal health history. Every field is
// required so callers cannot accidentally fall back to a global latest row.
type SynthesisReadQuery struct {
	Principal       string
	Cadence         SynthesisCadence
	FreshnessBudget time.Duration
	ObservedAt      time.Time
}

// SynthesisAttemptSnapshot is the latest health-relevant immutable event for
// the requested actor and cadence.
type SynthesisAttemptSnapshot struct {
	RunID     string
	AttemptNo int
	EventType SynthesisEventType
	OutputID  string
	StartedAt time.Time
	EventAt   time.Time
}

// SynthesisOutputSnapshot identifies the current output and the immutable
// event that verified it through the production read-back boundary.
type SynthesisOutputSnapshot struct {
	Latest            SynthesisLatest
	RunID             string
	VerifiedAttemptNo int
	VerifiedEvent     SynthesisEventType
	VerifiedAt        time.Time
}

// SynthesisCausalRelation states how the latest attempt relates to the current
// verified output without requiring consumers to infer it from timestamps.
type SynthesisCausalRelation string

const (
	SynthesisCausalNoHistory            SynthesisCausalRelation = "no-history"
	SynthesisCausalAttemptWithoutOutput SynthesisCausalRelation = "attempt-without-output"
	SynthesisCausalSameAttemptOutput    SynthesisCausalRelation = "same-attempt-output"
	SynthesisCausalPriorAttemptOutput   SynthesisCausalRelation = "prior-attempt-output"
)

// SynthesisReadSnapshot is one actor/cadence observation assembled by one SQL
// statement. Outcome is derived only from the identities and ordered events in
// this snapshot.
type SynthesisReadSnapshot struct {
	Query               SynthesisReadQuery
	LatestAttempt       *SynthesisAttemptSnapshot
	CurrentOutput       *SynthesisOutputSnapshot
	CausalRelation      SynthesisCausalRelation
	CurrentOutputExists bool
	Outcome             SynthesisPersistenceOutcome
}

func validateSynthesisReadQuery(query SynthesisReadQuery) error {
	if err := validateSynthesisReadScope(query.Principal, query.Cadence); err != nil {
		return err
	}
	if query.FreshnessBudget <= 0 {
		return errors.New("synthesis read query requires a positive freshness budget")
	}
	if query.ObservedAt.IsZero() {
		return errors.New("synthesis read query requires an observation time")
	}
	return nil
}

func validateSynthesisReadScope(principal string, cadence SynthesisCadence) error {
	if principal == "" {
		return errors.New("synthesis read query requires a principal")
	}
	if cadence != CadenceDaily && cadence != CadenceWeekly {
		return fmt.Errorf("synthesis read query has unknown cadence %q", cadence)
	}
	return nil
}

// ReadSnapshot returns the latest event and verified current output from one
// actor/cadence history. An output row without a verifying terminal event is
// represented as committed-unverified and never promoted to healthy.
func (m *SynthesisReadModel) ReadSnapshot(
	ctx context.Context,
	query SynthesisReadQuery,
) (SynthesisReadSnapshot, error) {
	if err := validateSynthesisReadQuery(query); err != nil {
		return SynthesisReadSnapshot{}, err
	}

	var eventRunID, eventType, eventOutputID pgtype.Text
	var eventAttemptNo pgtype.Int4
	var eventAt, attemptStartedAt pgtype.Timestamptz
	var outputID, outputRunID, logicalKey, outputCadence, outputKind pgtype.Text
	var outputLifecycle, verifiedEvent pgtype.Text
	var insightCount, citationCount, evaluatedArtifactCount pgtype.Int4
	var windowStart, windowEnd, outputCreatedAt, verifiedAt pgtype.Timestamptz
	var verifiedAttemptNo pgtype.Int4
	var currentOutputExists bool

	err := m.pool.QueryRow(ctx, `
		WITH latest_event AS (
			SELECT e.run_id, e.attempt_no, e.event_type, e.output_id,
			       e.created_at, a.started_at
			FROM synthesis_run_events e
			JOIN synthesis_runs r ON r.id = e.run_id
			JOIN synthesis_run_attempts a
			  ON a.run_id = e.run_id AND a.attempt_no = e.attempt_no
			WHERE r.principal = $1 AND r.cadence = $2
			  AND e.event_type IN (
				'attempt_started', 'idempotent', 'persisted', 'quiet',
				'partial', 'rolled_back', 'retryable_failure', 'failed',
				'readback_failed', 'recovered'
			  )
			ORDER BY e.created_at DESC, e.id DESC
			LIMIT 1
		), verified_output AS (
			SELECT o.id, o.run_id, r.logical_key, r.cadence, o.output_kind,
			       o.insight_count, o.citation_count, o.evaluated_artifact_count,
			       r.window_start, r.window_end, o.lifecycle_state, o.created_at,
			       proof.attempt_no, proof.event_type, proof.created_at AS verified_at
			FROM synthesis_outputs o
			JOIN synthesis_runs r ON r.id = o.run_id
			JOIN LATERAL (
				SELECT e.attempt_no, e.event_type, e.created_at
				FROM synthesis_run_events e
				WHERE e.run_id = o.run_id AND e.output_id = o.id
				  AND e.event_type IN (
					'idempotent', 'persisted', 'quiet', 'partial',
					'readback_failed', 'recovered'
				  )
				ORDER BY e.created_at DESC, e.id DESC
				LIMIT 1
			) proof ON proof.event_type IN (
				'idempotent', 'persisted', 'quiet', 'partial', 'recovered'
			)
			WHERE o.principal = $1 AND o.cadence = $2
			  AND o.lifecycle_state = 'current'
			ORDER BY proof.created_at DESC, o.created_at DESC, o.id DESC
			LIMIT 1
		), output_presence AS (
			SELECT EXISTS (
				SELECT 1 FROM synthesis_outputs o
				WHERE o.principal = $1 AND o.cadence = $2
				  AND o.lifecycle_state = 'current'
			) AS present
		)
		SELECT le.run_id, le.attempt_no, le.event_type, le.output_id,
		       le.created_at, le.started_at,
		       vo.id, vo.run_id, vo.logical_key, vo.cadence, vo.output_kind,
		       vo.insight_count, vo.citation_count, vo.evaluated_artifact_count,
		       vo.window_start, vo.window_end, vo.lifecycle_state, vo.created_at,
		       vo.attempt_no, vo.event_type, vo.verified_at, op.present
		FROM output_presence op
		LEFT JOIN latest_event le ON TRUE
		LEFT JOIN verified_output vo ON TRUE
	`, query.Principal, string(query.Cadence)).Scan(
		&eventRunID, &eventAttemptNo, &eventType, &eventOutputID,
		&eventAt, &attemptStartedAt,
		&outputID, &outputRunID, &logicalKey, &outputCadence, &outputKind,
		&insightCount, &citationCount, &evaluatedArtifactCount,
		&windowStart, &windowEnd, &outputLifecycle, &outputCreatedAt,
		&verifiedAttemptNo, &verifiedEvent, &verifiedAt, &currentOutputExists,
	)
	if err != nil {
		return SynthesisReadSnapshot{}, fmt.Errorf("read causal synthesis snapshot: %w", err)
	}

	snapshot := SynthesisReadSnapshot{
		Query:               query,
		CausalRelation:      SynthesisCausalNoHistory,
		CurrentOutputExists: currentOutputExists,
	}
	if eventRunID.Valid {
		snapshot.LatestAttempt = &SynthesisAttemptSnapshot{
			RunID: eventRunID.String, AttemptNo: int(eventAttemptNo.Int32),
			EventType: SynthesisEventType(eventType.String), OutputID: eventOutputID.String,
			StartedAt: attemptStartedAt.Time, EventAt: eventAt.Time,
		}
		snapshot.CausalRelation = SynthesisCausalAttemptWithoutOutput
	}
	if outputID.Valid {
		snapshot.CurrentOutput = &SynthesisOutputSnapshot{
			Latest: SynthesisLatest{
				OutputID: outputID.String, LogicalKey: logicalKey.String,
				Cadence: outputCadence.String, Kind: SynthesisOutputKind(outputKind.String),
				InsightCount: int(insightCount.Int32), CitationCount: int(citationCount.Int32),
				EvaluatedArtifactCount: int(evaluatedArtifactCount.Int32),
				WindowStart:            windowStart.Time, WindowEnd: windowEnd.Time,
				LifecycleState: outputLifecycle.String, CreatedAt: outputCreatedAt.Time,
			},
			RunID: outputRunID.String, VerifiedAttemptNo: int(verifiedAttemptNo.Int32),
			VerifiedEvent: SynthesisEventType(verifiedEvent.String), VerifiedAt: verifiedAt.Time,
		}
		snapshot.CausalRelation = SynthesisCausalPriorAttemptOutput
		if snapshot.LatestAttempt != nil &&
			snapshot.LatestAttempt.RunID == snapshot.CurrentOutput.RunID &&
			snapshot.LatestAttempt.AttemptNo == snapshot.CurrentOutput.VerifiedAttemptNo {
			snapshot.CausalRelation = SynthesisCausalSameAttemptOutput
		}
	}
	snapshot.Outcome = deriveSynthesisSnapshotOutcome(snapshot)
	return snapshot, nil
}

func deriveSynthesisSnapshotOutcome(snapshot SynthesisReadSnapshot) SynthesisPersistenceOutcome {
	if snapshot.LatestAttempt == nil {
		if snapshot.CurrentOutputExists {
			return SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackNotAttempted}
		}
		return SynthesisPersistenceOutcome{Phase: PhaseNoRun}
	}

	hasVerifiedOutput := snapshot.CurrentOutput != nil
	switch snapshot.LatestAttempt.EventType {
	case EventAttemptStarted, EventClaimed:
		return SynthesisPersistenceOutcome{Phase: PhaseRunning, HasPriorVerifiedOutput: hasVerifiedOutput}
	case EventRolledBack, EventRetryableFailure, EventFailed:
		return SynthesisPersistenceOutcome{Phase: PhaseWriteFailed, ReadBack: ReadBackNotAttempted, HasPriorVerifiedOutput: hasVerifiedOutput}
	case EventReadbackFailed:
		return SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackError, HasPriorVerifiedOutput: hasVerifiedOutput}
	case EventPersisted, EventQuiet, EventPartial, EventIdempotent, EventRecovered:
		if snapshot.CurrentOutput == nil || snapshot.LatestAttempt.OutputID != snapshot.CurrentOutput.Latest.OutputID ||
			snapshot.CausalRelation != SynthesisCausalSameAttemptOutput {
			return SynthesisPersistenceOutcome{Phase: PhaseProbeError}
		}
		return SynthesisPersistenceOutcome{
			Phase: PhaseCommitted, ReadBack: ReadBackOK,
			Output:                 snapshot.CurrentOutput.Latest.Kind,
			Stale:                  snapshot.Query.ObservedAt.Sub(snapshot.CurrentOutput.Latest.CreatedAt) > snapshot.Query.FreshnessBudget,
			HasPriorVerifiedOutput: true,
		}
	default:
		return SynthesisPersistenceOutcome{Phase: PhaseProbeError}
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

// LatestFor returns the newest verified output for one actor and cadence, or
// false when that causal history has none.
//
// The false is the point: a caller must be able to tell "no synthesis yet" from
// "synthesis with zero insights". Returning a zero-valued struct for both would
// collapse never-run into quiet, which is exactly the confusion SCN-05 forbids.
func (m *SynthesisReadModel) LatestFor(
	ctx context.Context,
	principal string,
	cadence SynthesisCadence,
) (SynthesisLatest, bool, error) {
	if err := validateSynthesisReadScope(principal, cadence); err != nil {
		return SynthesisLatest{}, false, err
	}

	var l SynthesisLatest
	err := m.pool.QueryRow(ctx, `
		SELECT o.id, r.logical_key, r.cadence, o.output_kind,
		       o.insight_count, o.citation_count, o.evaluated_artifact_count,
		       r.window_start, r.window_end, r.lifecycle_state, o.created_at
		FROM synthesis_outputs o
		JOIN synthesis_runs r ON r.id = o.run_id
		WHERE r.state = 'succeeded'
		  AND r.principal = $1 AND r.cadence = $2
		ORDER BY o.created_at DESC
		LIMIT 1`, principal, string(cadence)).Scan(
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
