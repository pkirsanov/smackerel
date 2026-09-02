package intelligence

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/config"
)

// BUG-004-004 SCOPE-02 — the producer that closes the loop.
//
// Before this file, RunSynthesis returned insights to a caller that logged
// len(insights) and dropped them. The count was true and the durable record was
// empty, which is the exact shape of the defect: a success report about work
// that left no trace. RunAndPersist is the only synthesis entry point that ends
// in a durable, read-back-verified row.

const canonicalSynthesisSourceClass = "canonical-graph"

// SynthesisRunPolicy is the fail-loud runtime contract shared by daily,
// weekly, scheduled, and operator-triggered synthesis.
type SynthesisRunPolicy struct {
	Actor                 string
	PolicyVersion         string
	RequiredSourceClasses []string
	OptionalSourceClasses []string
	Retention             time.Duration
}

func (p SynthesisRunPolicy) Validate() error {
	if strings.TrimSpace(p.Actor) == "" {
		return errors.New("synthesis run policy requires an actor")
	}
	if strings.TrimSpace(p.PolicyVersion) == "" {
		return errors.New("synthesis run policy requires a policy version")
	}
	if p.Retention <= 0 {
		return errors.New("synthesis run policy requires positive retention")
	}
	if len(p.RequiredSourceClasses) != 1 || p.RequiredSourceClasses[0] != canonicalSynthesisSourceClass {
		return fmt.Errorf("synthesis run policy requires exactly source class %q", canonicalSynthesisSourceClass)
	}
	if len(p.OptionalSourceClasses) != 0 {
		classes := append([]string(nil), p.OptionalSourceClasses...)
		sort.Strings(classes)
		return fmt.Errorf("synthesis run policy contains unsupported optional source classes: %s", strings.Join(classes, ","))
	}
	return nil
}

// SynthesisRunPolicyFromConfig maps the typed, fail-loud configuration into
// the production policy shared by daily, weekly, and operator-triggered runs.
func SynthesisRunPolicyFromConfig(cfg config.SynthesisConfig) SynthesisRunPolicy {
	return SynthesisRunPolicy{
		Actor:                 cfg.ActorUserID,
		PolicyVersion:         cfg.PolicyVersion,
		RequiredSourceClasses: append([]string{}, cfg.RequiredSourceClasses...),
		OptionalSourceClasses: append([]string{}, cfg.OptionalSourceClasses...),
		Retention:             cfg.Retention,
	}
}

// SynthesisRetryPolicyFromConfig maps the validated attempt count and typed
// durations into the coordinator policy used by the production runtime.
func SynthesisRetryPolicyFromConfig(cfg config.SynthesisConfig, maxAttempts int) SynthesisRetryPolicy {
	return SynthesisRetryPolicy{
		MaxAttempts:  maxAttempts,
		InitialDelay: cfg.RetryBackoff,
		MaxDelay:     cfg.RetryMaxBackoff,
		LeaseTTL:     cfg.LeaseTTL,
	}
}

// SynthesisProducer runs a cadence window end to end: read the authorized
// corpus, build candidate insights, validate, persist, read back.
type SynthesisProducer struct {
	engine      *Engine
	persistence *SynthesisPersistence
	policy      SynthesisRunPolicy
	// coordinator is optional. When set, a window is claimed before any work
	// starts and the commit runs under bounded retry, so a second scheduler or
	// an operator retry cannot duplicate the run. Nil keeps the single-process
	// path working unchanged.
	coordinator *SynthesisCoordinator
}

// WithCoordinator routes this producer's runs through cross-process claiming
// and bounded retry.
func (p *SynthesisProducer) WithCoordinator(c *SynthesisCoordinator) *SynthesisProducer {
	p.coordinator = c
	return p
}

// NewSynthesisProducer wires a producer to its engine and durable store. Both
// are required; a producer without persistence is the defect being repaired, so
// it cannot be constructed.
func NewSynthesisProducer(engine *Engine, persistence *SynthesisPersistence, policy SynthesisRunPolicy) (*SynthesisProducer, error) {
	if engine == nil {
		return nil, errors.New("synthesis producer requires an engine")
	}
	if persistence == nil {
		return nil, errors.New("synthesis producer requires persistence; a producer that cannot store its output is the defect this repairs")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &SynthesisProducer{engine: engine, persistence: persistence, policy: policy}, nil
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
func (p *SynthesisProducer) RunAndPersist(ctx context.Context, cadence SynthesisCadence, trigger SynthesisTriggerKind, now time.Time) (*SynthesisAggregate, error) {
	if p.coordinator == nil {
		return nil, errors.New("synthesis producer requires a coordinator; uncoordinated production writes are forbidden")
	}
	if err := validateTriggerKind(trigger); err != nil {
		return nil, err
	}
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
		Principal:     p.policy.Actor,
		WindowStart:   window.Start,
		WindowEnd:     window.End,
		PolicyVersion: p.policy.PolicyVersion,
		SourceIDs:     authorized,
	}
	policy := SourceClassPolicy{
		Required: append([]string(nil), p.policy.RequiredSourceClasses...),
		Optional: append([]string(nil), p.policy.OptionalSourceClasses...),
	}

	var lastErr error
	for retryIndex := 1; retryIndex <= p.coordinator.policy.MaxAttempts; retryIndex++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		attempt, startErr := p.persistence.StartAttempt(ctx, key, trigger,
			p.coordinator.holder, p.coordinator.policy.LeaseTTL, now)
		if startErr != nil {
			return nil, startErr
		}

		insights, buildErr := p.buildCadenceInsights(ctx, cadence)
		if buildErr != nil {
			kind := ClassifySynthesisFailure(buildErr)
			eventType := EventFailed
			if kind == FailureTransient {
				eventType = EventRetryableFailure
			}
			if auditErr := p.persistence.FinishAttemptFailure(context.WithoutCancel(ctx), attempt,
				eventType, string(FailureInvalidPayload), kind, time.Now().UTC()); auditErr != nil {
				return nil, auditErr
			}
			lastErr = fmt.Errorf("run synthesis: %w", buildErr)
		} else {
			kind := OutputKindFull
			if len(insights) == 0 {
				kind = OutputKindQuiet
			}
			candidate := SynthesisCandidate{
				Key: key, Kind: kind, Insights: insights,
				EvaluatedArtifactCount: len(authorized),
				IncludedClasses:        append([]string(nil), p.policy.RequiredSourceClasses...),
			}
			aggregate, commitErr := p.persistence.CommitAttempt(ctx, attempt, candidate,
				policy, authorized, time.Now().UTC())
			if commitErr == nil {
				if _, archiveErr := p.coordinator.ArchiveOlderThan(ctx, now.Add(-p.policy.Retention)); archiveErr != nil {
					return nil, fmt.Errorf("apply synthesis retention: %w", archiveErr)
				}
				return aggregate, nil
			}
			var auditErr *SynthesisAuditPersistenceError
			if errors.As(commitErr, &auditErr) {
				return nil, commitErr
			}
			lastErr = commitErr
		}

		if ClassifySynthesisFailure(lastErr) != FailureTransient || retryIndex == p.coordinator.policy.MaxAttempts {
			return nil, lastErr
		}
		if sleepErr := p.coordinator.sleep(ctx, p.coordinator.policy.backoffFor(retryIndex)); sleepErr != nil {
			return nil, sleepErr
		}
	}
	return nil, lastErr
}

func (p *SynthesisProducer) buildCadenceInsights(ctx context.Context, cadence SynthesisCadence) ([]SynthesisInsight, error) {
	switch cadence {
	case CadenceDaily:
		return p.engine.RunSynthesis(ctx)
	case CadenceWeekly:
		weekly, err := p.engine.GenerateWeeklySynthesis(ctx)
		if err != nil {
			return nil, err
		}
		return weekly.Insights, nil
	default:
		return nil, fmt.Errorf("unknown synthesis cadence %q", cadence)
	}
}
