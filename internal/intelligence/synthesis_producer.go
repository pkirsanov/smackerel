package intelligence

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// BUG-004-004 SCOPE-02 — the producer that closes the loop.
//
// Before this file, RunSynthesis returned insights to a caller that logged
// len(insights) and dropped them. The count was true and the durable record was
// empty, which is the exact shape of the defect: a success report about work
// that left no trace. RunAndPersist is the only synthesis entry point that ends
// in a durable, read-back-verified row.

// synthesisPolicyVersion identifies the producer contract that shaped an
// output. It participates in the logical key, so changing it deliberately makes
// a re-run under new rules a NEW run rather than a duplicate of the old one.
const synthesisPolicyVersion = "synthesis/v1"

// synthesisSourceClass is the single source class today's cluster producer
// draws from. It is required, so a window that cannot read the canonical graph
// fails rather than quietly reporting a partial output.
const synthesisSourceClass = "canonical-graph"

// SynthesisProducer runs a cadence window end to end: read the authorized
// corpus, build candidate insights, validate, persist, read back.
type SynthesisProducer struct {
	engine      *Engine
	persistence *SynthesisPersistence
}

// NewSynthesisProducer wires a producer to its engine and durable store. Both
// are required; a producer without persistence is the defect being repaired, so
// it cannot be constructed.
func NewSynthesisProducer(engine *Engine, persistence *SynthesisPersistence) (*SynthesisProducer, error) {
	if engine == nil {
		return nil, errors.New("synthesis producer requires an engine")
	}
	if persistence == nil {
		return nil, errors.New("synthesis producer requires persistence; a producer that cannot store its output is the defect this repairs")
	}
	return &SynthesisProducer{engine: engine, persistence: persistence}, nil
}

// SynthesisWindow is the half-open interval [Start, End) a run covers.
type SynthesisWindow struct {
	Start time.Time
	End   time.Time
}

// WindowFor derives the window a cadence covers when run at now. The window is
// snapped to a UTC day boundary so two runs on the same day resolve to the same
// logical key and the second is recognised as a duplicate rather than stored
// twice.
func WindowFor(cadence SynthesisCadence, now time.Time) (SynthesisWindow, error) {
	end := now.UTC().Truncate(24 * time.Hour)
	switch cadence {
	case CadenceDaily:
		return SynthesisWindow{Start: end.AddDate(0, 0, -1), End: end}, nil
	case CadenceWeekly:
		return SynthesisWindow{Start: end.AddDate(0, 0, -7), End: end}, nil
	default:
		return SynthesisWindow{}, fmt.Errorf("unknown synthesis cadence %q", cadence)
	}
}

// authorizedCorpus returns the artifact ids the producer is permitted to cite.
//
// HONEST SCOPE. Today's cluster query is not window-filtered, so the authorized
// set is every live artifact, not the window's slice. That is a truthful
// description of what the producer may read right now, and the check it buys is
// real: an insight citing a deleted or non-existent artifact is a dangling
// citation and is refused. It is NOT a claim that content is window-scoped.
// Narrowing the corpus to the window requires narrowing the cluster query in the
// same change, or every insight would be refused as unauthorized.
func (p *SynthesisProducer) authorizedCorpus(ctx context.Context) ([]string, error) {
	rows, err := p.engine.Pool.Query(ctx, `SELECT id FROM artifacts`)
	if err != nil {
		return nil, fmt.Errorf("read authorized corpus: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan authorized artifact: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate authorized corpus: %w", err)
	}
	return ids, nil
}

// RunAndPersist executes one cadence window and returns the durable aggregate.
//
// Every terminal path records an attempt, including the failing ones, because
// the health mapping in SCOPE-04 distinguishes never-ran from failed and cannot
// do that if failures leave no row. An already-committed window resolves to the
// existing output rather than erroring: a restart mid-schedule must not turn a
// stored success into a reported failure.
func (p *SynthesisProducer) RunAndPersist(ctx context.Context, cadence SynthesisCadence, principal string, now time.Time) (*SynthesisAggregate, error) {
	window, err := WindowFor(cadence, now)
	if err != nil {
		return nil, err
	}

	authorized, err := p.authorizedCorpus(ctx)
	if err != nil {
		return nil, err
	}

	key := SynthesisRunKey{
		Cadence:       cadence,
		Principal:     principal,
		WindowStart:   window.Start,
		WindowEnd:     window.End,
		PolicyVersion: synthesisPolicyVersion,
		SourceIDs:     authorized,
	}
	policy := SourceClassPolicy{Required: []string{synthesisSourceClass}}

	insights, err := p.engine.RunSynthesis(ctx)
	if err != nil {
		// The corpus could not be read, so this window has no verdict. Recording
		// the failure is what keeps it distinguishable from a quiet window, which
		// is a genuine "evaluated, found nothing".
		p.recordAttempt(ctx, key, AttemptFailed, string(FailureInvalidPayload), "cluster query failed")
		return nil, fmt.Errorf("run synthesis: %w", err)
	}

	// A window that evaluated its corpus and surfaced nothing is a quiet output,
	// not an absence. Storing it is what lets an operator tell "nothing to say"
	// apart from "never ran" a day later.
	kind := OutputKindFull
	if len(insights) == 0 {
		kind = OutputKindQuiet
	}

	candidate := SynthesisCandidate{
		Key:                    key,
		Kind:                   kind,
		Insights:               insights,
		EvaluatedArtifactCount: len(authorized),
		IncludedClasses:        []string{synthesisSourceClass},
	}

	aggregate, err := p.persistence.Commit(ctx, candidate, policy, authorized, now)
	switch {
	case err == nil:
		p.recordAttempt(ctx, key, AttemptSucceeded, "", "")
		return aggregate, nil

	case errors.Is(err, ErrSynthesisRunExists):
		// Restart or a second scheduler reached the same window. The stored
		// output is the answer; re-deriving it would be a duplicate.
		outputID, readErr := p.persistence.FindOutputByLogicalKey(ctx, key.LogicalKey())
		if readErr != nil {
			p.recordAttempt(ctx, key, AttemptFailed, string(FailureInvalidPayload), "duplicate run could not be resolved")
			return nil, fmt.Errorf("resolve existing synthesis run: %w", readErr)
		}
		// Read the stored aggregate rather than returning the id alone, so the
		// duplicate path and the first-commit path hand back the same shape and a
		// caller cannot accidentally treat one as emptier than the other.
		existing, readErr := p.persistence.ReadAggregate(ctx, outputID)
		if readErr != nil {
			p.recordAttempt(ctx, key, AttemptFailed, string(FailureInvalidPayload), "existing output could not be read back")
			return nil, fmt.Errorf("read existing synthesis output: %w", readErr)
		}
		p.recordAttempt(ctx, key, AttemptIdempotentNoChange, "", "")
		return existing, nil

	default:
		failureClass := string(FailureInvalidPayload)
		var ve *SynthesisValidationError
		if errors.As(err, &ve) {
			failureClass = string(ve.Code)
		}
		p.recordAttempt(ctx, key, AttemptFailed, failureClass, "persistence rejected the candidate")
		return nil, err
	}
}

// recordAttempt writes the audit row. Its own error is logged by the caller's
// telemetry rather than returned, because losing an audit row must not convert a
// committed output into a reported failure -- that would be the same class of
// lie in the opposite direction.
func (p *SynthesisProducer) recordAttempt(ctx context.Context, key SynthesisRunKey, outcome SynthesisAttemptOutcome, failureClass, message string) {
	// The attempt is deliberately written outside any content transaction so it
	// survives that transaction's rollback.
	_ = p.persistence.RecordAttempt(ctx, key.LogicalKey(), outcome, failureClass, message)
}
