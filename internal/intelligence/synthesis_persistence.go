package intelligence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// BUG-004-004 SCOPE-01. RunSynthesis built insights and returned them; the
// scheduler logged a count and dropped them, while health read MAX(created_at)
// from a table nothing wrote. This file is the durable half: one logical run,
// one atomic aggregate, and an attempt record that outlives a rollback.

// SynthesisCadence is the run rhythm. It is part of the logical key, so a daily
// and a weekly run over the same window are different runs.
type SynthesisCadence string

const (
	CadenceDaily  SynthesisCadence = "daily"
	CadenceWeekly SynthesisCadence = "weekly"
)

// SynthesisAttemptOutcome is the closed set of attempt outcomes. Any value
// outside it is rejected by a CHECK constraint rather than silently stored.
type SynthesisAttemptOutcome string

const (
	AttemptSucceeded          SynthesisAttemptOutcome = "succeeded"
	AttemptFailed             SynthesisAttemptOutcome = "failed"
	AttemptIdempotentNoChange SynthesisAttemptOutcome = "idempotent_no_change"
)

// ErrSynthesisRunExists reports that the logical run was already committed. It
// is not a failure: the caller records an idempotent no-change attempt and
// keeps the existing output.
var ErrSynthesisRunExists = errors.New("synthesis logical run already committed")

// SynthesisRunKey identifies a LOGICAL run. Two triggers carrying the same key
// are the same run no matter how far apart they execute.
type SynthesisRunKey struct {
	Cadence       SynthesisCadence
	Principal     string
	WindowStart   time.Time
	WindowEnd     time.Time
	PolicyVersion string
	SourceIDs     []string
}

// LogicalKey derives the deterministic idempotency anchor.
//
// Determinism here is load-bearing: it is what makes a restart mid-run resolve
// to the same row rather than a second output. Two properties give it that.
// The window is normalized to UTC, so a scheduler that computes the same
// instant in a different location produces the same key. And SourceIDs are
// sorted and de-duplicated before hashing, so the eligible source SET is what
// identifies the run, not the order the query happened to return it in.
func (k SynthesisRunKey) LogicalKey() string {
	seen := make(map[string]struct{}, len(k.SourceIDs))
	ids := make([]string, 0, len(k.SourceIDs))
	for _, id := range k.SourceIDs {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	h := sha256.New()
	// Length-prefix every field. Without it, cadence "a" + principal "bc" and
	// cadence "ab" + principal "c" would hash identically.
	for _, part := range []string{
		string(k.Cadence),
		k.Principal,
		k.WindowStart.UTC().Format(time.RFC3339Nano),
		k.WindowEnd.UTC().Format(time.RFC3339Nano),
		k.PolicyVersion,
		strings.Join(ids, ","),
	} {
		fmt.Fprintf(h, "%d:%s|", len(part), part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// SourceSetDigest is the canonical digest of the eligible source set, stored
// alongside the run so a later reader can tell which inputs produced it.
func (k SynthesisRunKey) SourceSetDigest() string {
	ids := append([]string(nil), k.SourceIDs...)
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, ",")))
	return hex.EncodeToString(sum[:])
}

// SynthesisAggregate is one committed output read back as a unit.
type SynthesisAggregate struct {
	OutputID      string
	RunID         string
	LogicalKey    string
	InsightCount  int
	CitationCount int
	CreatedAt     time.Time
	Insights      []SynthesisInsight
}

// SynthesisPersistence commits synthesis aggregates durably.
type SynthesisPersistence struct {
	pool *pgxpool.Pool
}

// NewSynthesisPersistence builds the persistence layer. A nil pool is rejected
// here rather than at the first query, so a misconfigured caller fails at wiring
// time instead of at the first scheduled run.
func NewSynthesisPersistence(pool *pgxpool.Pool) (*SynthesisPersistence, error) {
	if pool == nil {
		return nil, fmt.Errorf("synthesis persistence requires a database pool")
	}
	return &SynthesisPersistence{pool: pool}, nil
}

// Commit writes one logical run and its complete aggregate in a single
// serializable transaction, then reads the aggregate back before reporting
// success.
//
// The read-back is a MANDATORY post-commit gate, not a diagnostic. The bug this
// repairs was a producer that reported success having written nothing, so
// "COMMIT returned no error" is not sufficient evidence that the output is
// readable. Commit reports success only after the production reader has
// returned the same output identity and matching counts.
//
// Returns ErrSynthesisRunExists when the logical run is already committed. The
// caller records an idempotent no-change attempt; it must NOT treat that as an
// error, and must NOT write a second output.
func (p *SynthesisPersistence) Commit(
	ctx context.Context,
	key SynthesisRunKey,
	insights []SynthesisInsight,
	now time.Time,
) (*SynthesisAggregate, error) {
	if key.Cadence != CadenceDaily && key.Cadence != CadenceWeekly {
		return nil, fmt.Errorf("unknown synthesis cadence %q", key.Cadence)
	}
	if key.Principal == "" {
		return nil, fmt.Errorf("synthesis run requires a principal")
	}
	if key.PolicyVersion == "" {
		return nil, fmt.Errorf("synthesis run requires a policy version")
	}
	if !key.WindowEnd.After(key.WindowStart) {
		return nil, fmt.Errorf("synthesis window end must be after start")
	}

	logicalKey := key.LogicalKey()
	runID := ulid.Make().String()
	outputID := ulid.Make().String()

	// Serializable, because the idempotence claim is about concurrent runs. A
	// weaker level would let two transactions both observe "no run yet".
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin synthesis transaction: %w", err)
	}
	// Rollback is unconditional on every path that does not reach Commit; after
	// a successful Commit it is a no-op.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	citationCount := 0
	for _, in := range insights {
		citationCount += len(dedupeStrings(in.SourceArtifactIDs))
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO synthesis_runs
			(id, logical_key, cadence, principal, window_start, window_end,
			 policy_version, source_set_digest, state, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'succeeded', $9, $9)
	`, runID, logicalKey, string(key.Cadence), key.Principal,
		key.WindowStart.UTC(), key.WindowEnd.UTC(),
		key.PolicyVersion, key.SourceSetDigest(), now.UTC())
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrSynthesisRunExists
		}
		return nil, fmt.Errorf("insert synthesis run: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO synthesis_outputs
			(id, run_id, insight_count, citation_count, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, outputID, runID, len(insights), citationCount, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("insert synthesis output: %w", err)
	}

	for ordinal, in := range insights {
		insightID := in.ID
		if insightID == "" {
			insightID = ulid.Make().String()
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO synthesis_output_insights
				(id, output_id, ordinal, insight_type, through_line,
				 key_tension, suggested_action, confidence, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, insightID, outputID, ordinal, string(in.InsightType), in.ThroughLine,
			nullIfEmpty(in.KeyTension), nullIfEmpty(in.SuggestedAction),
			in.Confidence, now.UTC())
		if err != nil {
			return nil, fmt.Errorf("insert synthesis insight %d: %w", ordinal, err)
		}

		for cOrdinal, artifactID := range dedupeStrings(in.SourceArtifactIDs) {
			_, err = tx.Exec(ctx, `
				INSERT INTO synthesis_citations (insight_id, artifact_id, ordinal)
				VALUES ($1, $2, $3)
			`, insightID, artifactID, cOrdinal)
			if err != nil {
				return nil, fmt.Errorf("insert synthesis citation %d/%d: %w", ordinal, cOrdinal, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrSynthesisRunExists
		}
		return nil, fmt.Errorf("commit synthesis transaction: %w", err)
	}

	// Post-commit read-back gate. See the doc comment: a producer that reported
	// success while writing nothing is the defect being repaired.
	agg, err := p.ReadAggregate(ctx, outputID)
	if err != nil {
		return nil, fmt.Errorf("post-commit read-back failed for output %s: %w", outputID, err)
	}
	if agg.InsightCount != len(insights) || agg.CitationCount != citationCount {
		return nil, fmt.Errorf(
			"post-commit read-back mismatch for output %s: stored insights=%d citations=%d, expected insights=%d citations=%d",
			outputID, agg.InsightCount, agg.CitationCount, len(insights), citationCount)
	}
	return agg, nil
}

// ReadAggregate is the PRODUCTION aggregate reader. Commit calls this same
// function for its read-back rather than a test-only query, so the gate proves
// the path real consumers use.
func (p *SynthesisPersistence) ReadAggregate(ctx context.Context, outputID string) (*SynthesisAggregate, error) {
	agg := &SynthesisAggregate{OutputID: outputID}
	err := p.pool.QueryRow(ctx, `
		SELECT o.run_id, r.logical_key, o.insight_count, o.citation_count, o.created_at
		FROM synthesis_outputs o
		JOIN synthesis_runs r ON r.id = o.run_id
		WHERE o.id = $1
	`, outputID).Scan(&agg.RunID, &agg.LogicalKey, &agg.InsightCount, &agg.CitationCount, &agg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("read synthesis output %s: %w", outputID, err)
	}

	rows, err := p.pool.Query(ctx, `
		SELECT i.id, i.insight_type, i.through_line,
		       COALESCE(i.key_tension, ''), COALESCE(i.suggested_action, ''),
		       i.confidence, i.created_at,
		       COALESCE(ARRAY_AGG(c.artifact_id ORDER BY c.ordinal)
		                FILTER (WHERE c.artifact_id IS NOT NULL), '{}')
		FROM synthesis_output_insights i
		LEFT JOIN synthesis_citations c ON c.insight_id = i.id
		WHERE i.output_id = $1
		GROUP BY i.id, i.insight_type, i.through_line, i.key_tension,
		         i.suggested_action, i.confidence, i.created_at, i.ordinal
		ORDER BY i.ordinal
	`, outputID)
	if err != nil {
		return nil, fmt.Errorf("read synthesis insights for %s: %w", outputID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var in SynthesisInsight
		var insightType string
		if err := rows.Scan(&in.ID, &insightType, &in.ThroughLine, &in.KeyTension,
			&in.SuggestedAction, &in.Confidence, &in.CreatedAt, &in.SourceArtifactIDs); err != nil {
			return nil, fmt.Errorf("scan synthesis insight: %w", err)
		}
		in.InsightType = InsightType(insightType)
		agg.Insights = append(agg.Insights, in)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate synthesis insights: %w", err)
	}
	return agg, nil
}

// FindOutputByLogicalKey resolves the committed output for a logical run, which
// is how an idempotent retry returns the existing aggregate instead of writing
// a second one.
func (p *SynthesisPersistence) FindOutputByLogicalKey(ctx context.Context, logicalKey string) (string, error) {
	var outputID string
	err := p.pool.QueryRow(ctx, `
		SELECT o.id
		FROM synthesis_outputs o
		JOIN synthesis_runs r ON r.id = o.run_id
		WHERE r.logical_key = $1
	`, logicalKey).Scan(&outputID)
	if err != nil {
		return "", fmt.Errorf("find synthesis output for logical key: %w", err)
	}
	return outputID, nil
}

// RecordAttempt appends the attempt audit row.
//
// This runs in its OWN transaction, deliberately. SCN-004-004-03 requires a
// failed attempt to be visible while none of the content it tried to write
// survives; an attempt written inside the content transaction would roll back
// with it and the failure would leave no trace. failureMessage carries a class
// detail only — passing candidate text or source ids here would leak exactly
// the uncommitted content the scenario forbids.
func (p *SynthesisPersistence) RecordAttempt(
	ctx context.Context,
	logicalKey string,
	outcome SynthesisAttemptOutcome,
	failureClass string,
	failureMessage string,
) error {
	switch outcome {
	case AttemptSucceeded, AttemptIdempotentNoChange:
		if failureClass != "" {
			return fmt.Errorf("outcome %q must not carry a failure class", outcome)
		}
	case AttemptFailed:
		if failureClass == "" {
			return fmt.Errorf("failed attempt requires a failure class")
		}
	default:
		return fmt.Errorf("unknown synthesis attempt outcome %q", outcome)
	}

	_, err := p.pool.Exec(ctx, `
		INSERT INTO synthesis_run_attempts
			(logical_key, outcome, failure_class, failure_message)
		VALUES ($1, $2, $3, $4)
	`, logicalKey, string(outcome), nullIfEmpty(failureClass), nullIfEmpty(failureMessage))
	if err != nil {
		return fmt.Errorf("record synthesis attempt: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
