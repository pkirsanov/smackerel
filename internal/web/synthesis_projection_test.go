package web

import (
	"reflect"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence"
)

// BUG-004-004 SCOPE-05 — T004-UI-UNIT.

func synthNow() time.Time { return time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC) }

func synthLatest(kind intelligence.SynthesisOutputKind, age time.Duration) intelligence.SynthesisLatest {
	return intelligence.SynthesisLatest{
		OutputID:               "out-1",
		LogicalKey:             "key-1",
		Cadence:                "daily",
		Kind:                   kind,
		InsightCount:           3,
		CitationCount:          7,
		EvaluatedArtifactCount: 42,
		WindowStart:            synthNow().Add(-24 * time.Hour),
		WindowEnd:              synthNow(),
		LifecycleState:         "current",
		CreatedAt:              synthNow().Add(-age),
	}
}

func committed(kind intelligence.SynthesisOutputKind, stale bool) intelligence.SynthesisPersistenceOutcome {
	return intelligence.SynthesisPersistenceOutcome{
		Phase:                  intelligence.PhaseCommitted,
		ReadBack:               intelligence.ReadBackOK,
		Output:                 kind,
		Stale:                  stale,
		HasPriorVerifiedOutput: true,
	}
}

func TestSynthesisProjection_ClosedStatesAreExclusive(t *testing.T) {
	for name, tc := range map[string]struct {
		authorized bool
		outcome    intelligence.SynthesisPersistenceOutcome
		latest     intelligence.SynthesisLatest
		found      bool
		want       SynthesisViewState
	}{
		"unauthorized": {
			false, committed(intelligence.OutputKindFull, false), synthLatest(intelligence.OutputKindFull, time.Hour), true,
			SynthesisViewAuthRequired,
		},
		"probe error": {
			true, intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseProbeError}, intelligence.SynthesisLatest{}, false,
			SynthesisViewUnavailable,
		},
		"never run": {
			true, intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseNoRun}, intelligence.SynthesisLatest{}, false,
			SynthesisViewNeverRun,
		},
		"failed with no output": {
			true, intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseWriteFailed}, intelligence.SynthesisLatest{}, false,
			SynthesisViewFailedNoOutput,
		},
		"failed with prior output": {
			true,
			intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseWriteFailed, HasPriorVerifiedOutput: true},
			synthLatest(intelligence.OutputKindFull, 2*time.Hour), true,
			SynthesisViewFailedPriorOutput,
		},
		"current": {
			true, committed(intelligence.OutputKindFull, false), synthLatest(intelligence.OutputKindFull, time.Hour), true,
			SynthesisViewCurrent,
		},
		"quiet": {
			true, committed(intelligence.OutputKindQuiet, false), synthLatest(intelligence.OutputKindQuiet, time.Hour), true,
			SynthesisViewQuiet,
		},
		"partial": {
			true, committed(intelligence.OutputKindPartial, false), synthLatest(intelligence.OutputKindPartial, time.Hour), true,
			SynthesisViewPartial,
		},
		"stale outranks kind": {
			true, committed(intelligence.OutputKindPartial, true), synthLatest(intelligence.OutputKindPartial, 72*time.Hour), true,
			SynthesisViewStale,
		},
		// Committed-but-unverified must not render. Showing it would put prose in
		// front of a reader that was never proven to be in the database.
		"committed but read-back failed": {
			true,
			intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseCommitted, ReadBack: intelligence.ReadBackMismatch},
			synthLatest(intelligence.OutputKindFull, time.Hour), true,
			SynthesisViewFailedNoOutput,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := ClassifySynthesisView(tc.authorized, tc.outcome, tc.latest, tc.found, synthNow())
			if got.State != tc.want {
				t.Fatalf("state %q, want %q", got.State, tc.want)
			}
		})
	}
}

// Every state must be reachable and distinct, or one branch of the UI is dead
// code that no test can ever exercise.
func TestSynthesisProjection_EveryStateIsReachable(t *testing.T) {
	reached := map[SynthesisViewState]bool{}
	for _, tc := range []struct {
		authorized bool
		outcome    intelligence.SynthesisPersistenceOutcome
		latest     intelligence.SynthesisLatest
		found      bool
	}{
		{false, committed(intelligence.OutputKindFull, false), synthLatest(intelligence.OutputKindFull, time.Hour), true},
		{true, intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseProbeError}, intelligence.SynthesisLatest{}, false},
		{true, intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseNoRun}, intelligence.SynthesisLatest{}, false},
		{true, intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseWriteFailed}, intelligence.SynthesisLatest{}, false},
		{true, intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseWriteFailed, HasPriorVerifiedOutput: true}, synthLatest(intelligence.OutputKindFull, time.Hour), true},
		{true, committed(intelligence.OutputKindFull, false), synthLatest(intelligence.OutputKindFull, time.Hour), true},
		{true, committed(intelligence.OutputKindQuiet, false), synthLatest(intelligence.OutputKindQuiet, time.Hour), true},
		{true, committed(intelligence.OutputKindPartial, false), synthLatest(intelligence.OutputKindPartial, time.Hour), true},
		{true, committed(intelligence.OutputKindFull, true), synthLatest(intelligence.OutputKindFull, 72*time.Hour), true},
	} {
		reached[ClassifySynthesisView(tc.authorized, tc.outcome, tc.latest, tc.found, synthNow()).State] = true
	}

	for _, state := range []SynthesisViewState{
		SynthesisViewAuthRequired, SynthesisViewUnavailable, SynthesisViewNeverRun,
		SynthesisViewFailedNoOutput, SynthesisViewFailedPriorOutput,
		SynthesisViewCurrent, SynthesisViewQuiet, SynthesisViewPartial, SynthesisViewStale,
	} {
		if !reached[state] {
			t.Fatalf("state %q is unreachable; the UI branch for it is dead code", state)
		}
	}
}

// SCN-004-004-09. An unauthorized reader's model must hold NOTHING derived from
// synthesis -- not blanked afterwards, never populated. Reflection is used so a
// field added later is covered without anyone remembering to extend this list.
func TestSynthesisProjection_UnauthorizedModelIsStructurallyEmpty(t *testing.T) {
	populated := ClassifySynthesisView(true, committed(intelligence.OutputKindFull, false),
		synthLatest(intelligence.OutputKindFull, time.Hour), true, synthNow())
	// Control: the authorized model must actually carry data, or the emptiness
	// assertion below would hold on a classifier that returns nothing at all.
	if populated.OutputID == "" || populated.InsightCount == 0 || populated.CitationCount == 0 {
		t.Fatalf("authorized model is not populated: %+v", populated)
	}

	cleared := ClassifySynthesisView(false, committed(intelligence.OutputKindFull, false),
		synthLatest(intelligence.OutputKindFull, time.Hour), true, synthNow())
	if cleared.State != SynthesisViewAuthRequired {
		t.Fatalf("state %q, want auth_required", cleared.State)
	}

	v := reflect.ValueOf(cleared)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if typ.Field(i).Name == "State" {
			continue
		}
		if !v.Field(i).IsZero() {
			t.Fatalf("unauthorized model carries %s = %v; synthesis-derived fields must never be populated for an unauthorized reader",
				typ.Field(i).Name, v.Field(i).Interface())
		}
	}
}

// HasContent is what a template asks instead of enumerating states. It must be
// true for exactly the states that legitimately carry prose.
func TestSynthesisProjection_HasContentIsTrueForExactlyTheContentStates(t *testing.T) {
	want := map[SynthesisViewState]bool{
		SynthesisViewCurrent:           true,
		SynthesisViewStale:             true,
		SynthesisViewPartial:           true,
		SynthesisViewQuiet:             false, // quiet HAS no prose by definition
		SynthesisViewNeverRun:          false,
		SynthesisViewFailedNoOutput:    false,
		SynthesisViewFailedPriorOutput: false,
		SynthesisViewUnavailable:       false,
		SynthesisViewAuthRequired:      false,
	}
	for state, expected := range want {
		if got := (SynthesisPageModel{State: state}).HasContent(); got != expected {
			t.Fatalf("HasContent for %q is %v, want %v", state, got, expected)
		}
	}
}

// A failure must not present the older output as the current answer. The
// distinction is the whole reason failed-with-prior-output is its own state.
func TestSynthesisProjection_FailureWithPriorOutputRendersNoContent(t *testing.T) {
	m := ClassifySynthesisView(true,
		intelligence.SynthesisPersistenceOutcome{Phase: intelligence.PhaseWriteFailed, HasPriorVerifiedOutput: true},
		synthLatest(intelligence.OutputKindFull, 2*time.Hour), true, synthNow())

	if m.HasContent() {
		t.Fatal("failed-with-prior-output claims content; the older output must not be shown as the current answer")
	}
	if m.InsightCount != 0 || m.CitationCount != 0 {
		t.Fatalf("failure state carries counts (%d insights, %d citations) that describe an output it is not presenting",
			m.InsightCount, m.CitationCount)
	}
	// The prior output is acknowledged as history, which is what lets a reader
	// tell "nothing has ever worked" from "the latest run failed".
	if !m.HasPriorVerifiedOutput || m.OutputID == "" {
		t.Fatal("failed-with-prior-output must still name that a prior output exists")
	}
}
