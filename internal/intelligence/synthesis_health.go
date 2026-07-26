package intelligence

// synthesis_health.go — the pure, database-free HEALTH-TRUTH mapping for
// durable synthesis (BUG-004-004 SCOPE-04 slice: "Canonical Read Health Alert
// And API Truth").
//
// Bug being fixed: synthesis did not persist durable insights, and health/status
// reported HEALTHY even when synthesis had actually FAILED to persist.
// internal/api/health.go::getCachedIntelligenceHealth maps a never-run sentinel
// to "up" and a freshness-probe error to "up", so a system that never produced
// (or failed to persist) any synthesis still reports green.
//
// design.md defines the fix as a `SynthesisHealthPolicy` derived from the durable
// latest attempt, the latest persisted output, the freshness budget, output
// completeness, and — decisively — the mandatory post-commit read-back gate
// ("only that successful read-back yields `persisted`"). This file implements
// exactly that policy as a single pure function.
//
// The DB-backed derivation of the outcome (the live PostgreSQL run ledger, the
// atomic-commit transaction, and wiring this verdict into the /api/health
// handler) is the live-stack slice and is DEFERRED. This file delivers only the
// disjoint, unit-verifiable determination: given a persistence OUTCOME, produce
// the truthful health verdict. It performs NO I/O, reads NO clock, and touches
// NO database — every input is carried by the injected
// SynthesisPersistenceOutcome seam, which makes the mapping unit-testable
// without a database.

// SynthesisHealthState is the closed, truthful vocabulary a synthesis health
// verdict may take (design.md SCOPE-04). Only SynthesisReadyCurrent and
// SynthesisReadyQuiet are "green" states; every other value is a NON-green
// state that can never be reported as strict "up" readiness and can never be
// reported as `persisted` for the latest attempt.
type SynthesisHealthState string

const (
	// SynthesisNeverRun — no synthesis attempt or output has ever been recorded.
	SynthesisNeverRun SynthesisHealthState = "never-run"
	// SynthesisRunning — the latest attempt is claimed and in flight; it has not
	// yet produced a read-back-verified output.
	SynthesisRunning SynthesisHealthState = "running"
	// SynthesisReadyCurrent — the latest attempt committed a full output that
	// passed read-back and is within the freshness budget. The only "green"
	// state with insights.
	SynthesisReadyCurrent SynthesisHealthState = "ready-current"
	// SynthesisReadyQuiet — the latest attempt committed an explicit no-insight
	// (quiet) output that passed read-back and is fresh. A durable, healthy
	// outcome distinct from never-run and failure.
	SynthesisReadyQuiet SynthesisHealthState = "ready-quiet"
	// SynthesisDegradedPartial — a permitted partial output was committed and
	// read back, but it is incomplete; durable, yet never healthy or persisted.
	SynthesisDegradedPartial SynthesisHealthState = "degraded-partial"
	// SynthesisDegradedStale — a read-back-verified complete/quiet output exists
	// but is older than the freshness budget.
	SynthesisDegradedStale SynthesisHealthState = "degraded-stale"
	// SynthesisFailedWithoutOutput — the latest attempt failed to persist and no
	// prior verified output exists.
	SynthesisFailedWithoutOutput SynthesisHealthState = "failed-without-output"
	// SynthesisFailedWithPriorOutput — the latest attempt failed to persist but a
	// prior verified output remains on record.
	SynthesisFailedWithPriorOutput SynthesisHealthState = "failed-with-prior-output"
	// SynthesisReadDegraded — durability is UNPROVEN: a commit whose post-commit
	// read-back gate did not verify a coherent aggregate (orphaned citations,
	// missing insight/output rows, a failed read-back, or an unreadable probe).
	SynthesisReadDegraded SynthesisHealthState = "read-degraded"
)

// PersistencePhase is the durable outcome of the LATEST synthesis attempt's
// atomic persistence transaction, as observed by the coordinator (design.md
// "Single persistence transaction" + mandatory "post-commit read-back gate").
// It is an INJECTED fact — the caller derives it from the durable run ledger —
// which is the seam that makes DeriveSynthesisHealth testable without a database.
type PersistencePhase string

const (
	// PhaseNoRun — no synthesis attempt or output has ever been recorded.
	PhaseNoRun PersistencePhase = "no-run"
	// PhaseRunning — the latest attempt is claimed and in flight; no committed-
	// or-rolled-back verdict yet.
	PhaseRunning PersistencePhase = "running"
	// PhaseWriteFailed — the latest attempt's transaction rolled back; nothing
	// from this attempt is committed (atomic rollback, no partial rows).
	PhaseWriteFailed PersistencePhase = "write-failed"
	// PhaseCommitted — the latest attempt's transaction committed. Commit ALONE
	// is not success: the mandatory post-commit read-back gate (ReadBack) still
	// decides whether the aggregate is durable and coherent.
	PhaseCommitted PersistencePhase = "committed"
	// PhaseProbeError — the durable persistence state could not be evaluated (the
	// health probe/read itself failed). Truth is unknown, so it is never "up".
	PhaseProbeError PersistencePhase = "probe-error"
)

// ReadBackResult is the outcome of the MANDATORY post-commit read-back gate:
// after the transaction commits, the coordinator re-reads the full aggregate
// (run + output + insights + citations) with the same query the APIs use.
// design.md: "only that successful read-back yields `persisted`."
type ReadBackResult string

const (
	// ReadBackNotAttempted — no read-back was performed. A commit without a
	// verifying read-back is not `persisted`.
	ReadBackNotAttempted ReadBackResult = "not-attempted"
	// ReadBackOK — the committed aggregate read back whole and internally
	// consistent.
	ReadBackOK ReadBackResult = "ok"
	// ReadBackMismatch — the read-back returned an incomplete or inconsistent
	// aggregate (orphaned citations, a missing insight, a missing output row).
	// The commit is not trustworthy as durable success.
	ReadBackMismatch ReadBackResult = "mismatch"
	// ReadBackError — the read-back query itself failed; durability is unproven.
	ReadBackError ReadBackResult = "error"
)

// SynthesisOutputKind classifies a committed output that passed read-back.
type SynthesisOutputKind string

const (
	// OutputKindNone — no output kind (only valid before commit / on failure).
	OutputKindNone SynthesisOutputKind = "none"
	// OutputKindFull — a complete output with one or more insights.
	OutputKindFull SynthesisOutputKind = "full"
	// OutputKindQuiet — an explicit no-insight output for a fully-evaluated
	// window (a durable, healthy "nothing to surface" result).
	OutputKindQuiet SynthesisOutputKind = "quiet"
	// OutputKindPartial — a policy-approved partial output that names its
	// omitted source classes; durable but incomplete, never healthy.
	OutputKindPartial SynthesisOutputKind = "partial"
)

// SynthesisPersistenceOutcome is the injected, database-free description of the
// durable facts the health-truth mapping derives from. It captures exactly the
// inputs SCOPE-04 names: the latest attempt's persistence phase, the mandatory
// post-commit read-back result, the committed output kind, whether the latest
// verified output is stale against the freshness budget, and whether a prior
// read-back-verified output exists.
type SynthesisPersistenceOutcome struct {
	// Phase is the durable outcome of the latest attempt's persistence
	// transaction.
	Phase PersistencePhase
	// ReadBack is the mandatory post-commit read-back gate result. It is only
	// meaningful when Phase == PhaseCommitted.
	ReadBack ReadBackResult
	// Output is the committed output kind; meaningful only when Phase ==
	// PhaseCommitted and ReadBack == ReadBackOK.
	Output SynthesisOutputKind
	// Stale is true when the latest read-back-verified output is older than the
	// freshness budget (an SST value evaluated by the caller, not here).
	Stale bool
	// HasPriorVerifiedOutput is true when a previous read-back-verified output
	// exists. It distinguishes failed-with-prior-output from
	// failed-without-output. It never upgrades the latest attempt's verdict:
	// a failed or in-flight latest attempt is never healthy or `persisted`
	// on the strength of an older output.
	HasPriorVerifiedOutput bool
}

// SynthesisHealth is the truthful derived verdict produced by
// DeriveSynthesisHealth.
type SynthesisHealth struct {
	// State is the closed-vocabulary health state.
	State SynthesisHealthState
	// Persisted is true ONLY when the LATEST synthesis attempt committed a
	// complete-or-quiet aggregate that passed the mandatory post-commit
	// read-back gate. A write failure, a partial output, or any non-OK
	// read-back can never be Persisted. (A stale verified output is still
	// Persisted — persistence succeeded; it is merely old.)
	Persisted bool
	// Healthy is strict "green" readiness: a durable, read-back-verified,
	// complete-or-quiet output that is not stale. Healthy always implies
	// Persisted.
	Healthy bool
	// IntelligenceStatus is the coarse /api/health "intelligence" service value
	// ("up" | "stale" | "down"). The pre-fix bug mapped never-run and
	// probe-error to "up"; this mapping never does. It is "up" if and only if
	// Healthy is true.
	IntelligenceStatus string
}

// Coarse /api/health "intelligence" service status values. Kept unexported;
// consumers should prefer the SynthesisHealth.Healthy / .Persisted booleans and
// assign IntelligenceStatus verbatim to the existing health snapshot field.
const (
	synthesisStatusUp    = "up"
	synthesisStatusStale = "stale"
	synthesisStatusDown  = "down"
)

// DeriveSynthesisHealth is the single, pure, database-free HEALTH-TRUTH mapping
// for durable synthesis. It is the SCOPE-04 authority that replaces the
// epoch-sentinel / probe-error-is-up logic in internal/api/health.go: a
// synthesis attempt that failed to persist, only partially persisted, or failed
// its mandatory post-commit read-back gate can NEVER be reported as Healthy or
// Persisted, and never maps to intelligence status "up".
//
// It performs no I/O, reads no clock, and touches no database — every input is
// carried by the injected SynthesisPersistenceOutcome, which the caller derives
// from the durable run ledger.
func DeriveSynthesisHealth(o SynthesisPersistenceOutcome) SynthesisHealth {
	switch o.Phase {
	case PhaseProbeError:
		// Durable state is UNKNOWN — never assert health from an unreadable
		// probe. This closes the "freshness-probe error maps to up" bug.
		return notHealthy(SynthesisReadDegraded)

	case PhaseNoRun:
		// No attempt or output ever. This closes the "never-run maps to up" bug.
		return notHealthy(SynthesisNeverRun)

	case PhaseWriteFailed:
		// The atomic transaction rolled back; the latest attempt persisted
		// nothing. A prior verified output (if any) only changes which failure
		// state is reported, never the not-healthy / not-persisted verdict.
		if o.HasPriorVerifiedOutput {
			return notHealthy(SynthesisFailedWithPriorOutput)
		}
		return notHealthy(SynthesisFailedWithoutOutput)

	case PhaseRunning:
		// An in-flight run is not itself a durable success — "scheduler
		// acceptance ... is never success". The latest attempt has produced no
		// read-back-verified output, so it is never healthy or persisted.
		return notHealthy(SynthesisRunning)

	case PhaseCommitted:
		return deriveCommittedHealth(o)

	default:
		// Unknown phase — fail closed, never green.
		return notHealthy(SynthesisReadDegraded)
	}
}

// deriveCommittedHealth resolves the verdict for a committed transaction, where
// the mandatory post-commit read-back gate is the decider.
func deriveCommittedHealth(o SynthesisPersistenceOutcome) SynthesisHealth {
	if o.ReadBack != ReadBackOK {
		// Committed, but the mandatory read-back gate did not verify a coherent
		// aggregate (mismatch/orphan, read-back error, or no read-back at all).
		// "only that successful read-back yields `persisted`" — so this is
		// never persisted and never healthy.
		return notHealthy(SynthesisReadDegraded)
	}

	switch o.Output {
	case OutputKindFull, OutputKindQuiet:
		if o.Stale {
			// Persistence succeeded and read back cleanly, but the output is
			// older than the freshness budget: durable (Persisted) yet not
			// green.
			return SynthesisHealth{
				State:              SynthesisDegradedStale,
				Persisted:          true,
				Healthy:            false,
				IntelligenceStatus: synthesisStatusStale,
			}
		}
		state := SynthesisReadyCurrent
		if o.Output == OutputKindQuiet {
			state = SynthesisReadyQuiet
		}
		return SynthesisHealth{
			State:              state,
			Persisted:          true,
			Healthy:            true,
			IntelligenceStatus: synthesisStatusUp,
		}

	case OutputKindPartial:
		// A permitted partial output is durable but incomplete: it can never be
		// reported healthy or persisted (a fully-persisted complete success).
		return notHealthy(SynthesisDegradedPartial)

	default:
		// OutputKindNone (or any unknown kind) after a supposedly-OK read-back is
		// itself an inconsistency: fail closed.
		return notHealthy(SynthesisReadDegraded)
	}
}

// notHealthy builds a verdict for any NON-green state: never Persisted, never
// Healthy, and coarse status "down" (readiness not met and not a mere staleness
// condition). SynthesisDegradedStale is the only non-green state that maps to
// "stale", and it is constructed directly in deriveCommittedHealth.
func notHealthy(state SynthesisHealthState) SynthesisHealth {
	return SynthesisHealth{
		State:              state,
		Persisted:          false,
		Healthy:            false,
		IntelligenceStatus: synthesisStatusDown,
	}
}
