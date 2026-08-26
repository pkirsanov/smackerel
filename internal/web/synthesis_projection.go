package web

import (
	"time"

	"github.com/smackerel/smackerel/internal/intelligence"
)

// BUG-004-004 SCOPE-05 — the Today/Status synthesis projection.
//
// Modelled on DigestPageModel deliberately: one pure classifier, a closed state
// vocabulary, and every content-bearing field cleared on any state that is not
// content. That shape is what makes "a read error can never present stored or
// substituted content" checkable rather than aspirational, and the same
// property is needed here for a stricter reason -- synthesis prose is derived
// from the user's corpus.

// SynthesisViewState is the closed set of states Today and Status can render.
// Exactly one is set. They are mutually exclusive by construction rather than
// by convention, so a template cannot show two at once.
type SynthesisViewState string

const (
	// SynthesisViewNeverRun — no attempt and no output has ever existed.
	// Distinct from quiet: quiet means a window was evaluated and produced
	// nothing, never-run means nothing was ever evaluated.
	SynthesisViewNeverRun SynthesisViewState = "never_run"
	// SynthesisViewCurrent — a verified output inside its freshness budget.
	SynthesisViewCurrent SynthesisViewState = "current"
	// SynthesisViewQuiet — a durable, explicitly-empty output.
	SynthesisViewQuiet SynthesisViewState = "quiet"
	// SynthesisViewStale — a verified output past its freshness budget.
	SynthesisViewStale SynthesisViewState = "stale"
	// SynthesisViewPartial — a policy-approved partial output that names its
	// omissions. Durable and honest, never full health.
	SynthesisViewPartial SynthesisViewState = "partial"
	// SynthesisViewFailedNoOutput — the latest attempt failed and nothing
	// verified has ever been stored.
	SynthesisViewFailedNoOutput SynthesisViewState = "failed_without_output"
	// SynthesisViewFailedPriorOutput — the latest attempt failed but an older
	// verified output exists. Kept separate so the reader is never shown a
	// stale answer as though it were the current one.
	SynthesisViewFailedPriorOutput SynthesisViewState = "failed_with_prior_output"
	// SynthesisViewUnavailable — the durable state could not be read. Not a
	// failure of synthesis and emphatically not an empty result.
	SynthesisViewUnavailable SynthesisViewState = "unavailable"
	// SynthesisViewAuthRequired — the reader is not authorized. Every
	// synthesis-derived field is cleared in this state.
	SynthesisViewAuthRequired SynthesisViewState = "auth_required"
)

// SynthesisPageModel is the single typed projection rendered by the Today and
// Status synthesis sections.
//
// Content-bearing fields are populated ONLY in states that legitimately carry
// content. Everything else is zero, so a template that forgets to branch shows
// nothing rather than showing the previous reader's synthesis.
type SynthesisPageModel struct {
	State SynthesisViewState

	// Identity and provenance. Present for any state backed by a real output.
	OutputID       string
	Cadence        string
	WindowStart    string
	WindowEnd      string
	PersistedAtUTC string
	LifecycleState string

	// Counts. Safe to render anywhere an output exists; they carry no text.
	InsightCount           int
	CitationCount          int
	EvaluatedArtifactCount int

	// AgeHours is how old the output is, for the stale explanation.
	AgeHours int

	// HasPriorVerifiedOutput distinguishes the two failure states for a reader
	// who needs to know whether anything trustworthy exists at all.
	HasPriorVerifiedOutput bool
}

// HasContent reports whether this state may render synthesis prose.
//
// It exists so a template asks one question instead of enumerating states, and
// so adding a state without deciding its content policy is a compile-time
// prompt rather than a silent leak.
func (m SynthesisPageModel) HasContent() bool {
	return m.State == SynthesisViewCurrent ||
		m.State == SynthesisViewStale ||
		m.State == SynthesisViewPartial
}

// ClassifySynthesisView maps one durable read to exactly one view state.
//
// Pure and total: every input produces exactly one state, and no input produces
// two. `authorized` is the first branch because an unauthorized reader must not
// reach any code path that copies a stored field into the model at all -- the
// clearing is structural, not a later pass that could be skipped.
func ClassifySynthesisView(
	authorized bool,
	outcome intelligence.SynthesisPersistenceOutcome,
	latest intelligence.SynthesisLatest,
	found bool,
	now time.Time,
) SynthesisPageModel {
	if !authorized {
		// Nothing is copied in. Not "copied then blanked" -- an unauthorized
		// model has never held the data, so no later change can leak it.
		return SynthesisPageModel{State: SynthesisViewAuthRequired}
	}

	switch outcome.Phase {
	case intelligence.PhaseProbeError:
		return SynthesisPageModel{State: SynthesisViewUnavailable}

	case intelligence.PhaseNoRun:
		return SynthesisPageModel{State: SynthesisViewNeverRun}

	case intelligence.PhaseWriteFailed:
		// A failure never renders the older output as though it were current.
		// The prior output is acknowledged as a fact about history, not shown as
		// the answer to "what is the latest synthesis".
		if outcome.HasPriorVerifiedOutput && found {
			return SynthesisPageModel{
				State:                  SynthesisViewFailedPriorOutput,
				HasPriorVerifiedOutput: true,
				OutputID:               latest.OutputID,
				PersistedAtUTC:         latest.CreatedAt.UTC().Format(time.RFC3339),
			}
		}
		return SynthesisPageModel{State: SynthesisViewFailedNoOutput}
	}

	// Committed but not read-back-verified is not a success. Rendering it would
	// show a reader prose that was never proven to be in the database.
	if outcome.Phase != intelligence.PhaseCommitted || outcome.ReadBack != intelligence.ReadBackOK || !found {
		return SynthesisPageModel{State: SynthesisViewFailedNoOutput}
	}

	m := SynthesisPageModel{
		OutputID:               latest.OutputID,
		Cadence:                latest.Cadence,
		WindowStart:            latest.WindowStart.UTC().Format(time.RFC3339),
		WindowEnd:              latest.WindowEnd.UTC().Format(time.RFC3339),
		PersistedAtUTC:         latest.CreatedAt.UTC().Format(time.RFC3339),
		LifecycleState:         latest.LifecycleState,
		InsightCount:           latest.InsightCount,
		CitationCount:          latest.CitationCount,
		EvaluatedArtifactCount: latest.EvaluatedArtifactCount,
		AgeHours:               int(now.Sub(latest.CreatedAt).Hours()),
		HasPriorVerifiedOutput: true,
	}

	switch {
	case outcome.Stale:
		m.State = SynthesisViewStale
	case latest.Kind == intelligence.OutputKindQuiet:
		m.State = SynthesisViewQuiet
	case latest.Kind == intelligence.OutputKindPartial:
		m.State = SynthesisViewPartial
	default:
		m.State = SynthesisViewCurrent
	}
	return m
}
