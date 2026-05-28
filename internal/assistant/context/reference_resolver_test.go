// Spec 061 SCOPE-04 — reference resolver unit tests.

package assistantctx

import (
	"testing"
	"time"
)

func TestResolveReference(t *testing.T) {
	t.Parallel()

	threeSources := WorkingContext{Turns: []ContextTurn{
		{UserText: "what did paul say", Body: "Paul said ...", SourceIDs: []string{"art-A", "art-B", "art-C"}, EmittedAt: time.Unix(1, 0)},
	}}
	twoSources := WorkingContext{Turns: []ContextTurn{
		{UserText: "ping", Body: "pong", SourceIDs: []string{"art-X", "art-Y"}, EmittedAt: time.Unix(1, 0)},
	}}
	zeroSources := WorkingContext{Turns: []ContextTurn{
		{UserText: "ping", Body: "pong", SourceIDs: nil, EmittedAt: time.Unix(1, 0)},
	}}
	empty := WorkingContext{}

	cases := []struct {
		name             string
		userText         string
		wc               WorkingContext
		wantOutcome      ResolveOutcome
		wantSourceID     string
		wantAvailableSrc int
	}{
		// --- resolved: "that one" / "it" → first source ---
		{name: "that one — first source", userText: "that one", wc: threeSources, wantOutcome: ResolveOutcomeResolved, wantSourceID: "art-A", wantAvailableSrc: 3},
		{name: "that — first source", userText: "that", wc: threeSources, wantOutcome: ResolveOutcomeResolved, wantSourceID: "art-A", wantAvailableSrc: 3},
		{name: "it — first source (single-word)", userText: "it", wc: threeSources, wantOutcome: ResolveOutcomeResolved, wantSourceID: "art-A", wantAvailableSrc: 3},

		// --- resolved: numeric ---
		{name: "open 2 — second source", userText: "open 2", wc: threeSources, wantOutcome: ResolveOutcomeResolved, wantSourceID: "art-B", wantAvailableSrc: 3},
		{name: "show 3 — third source", userText: "show 3", wc: threeSources, wantOutcome: ResolveOutcomeResolved, wantSourceID: "art-C", wantAvailableSrc: 3},
		{name: "show me 1 — first source", userText: "show me 1", wc: threeSources, wantOutcome: ResolveOutcomeResolved, wantSourceID: "art-A", wantAvailableSrc: 3},
		{name: "bare digit — first source", userText: "1", wc: threeSources, wantOutcome: ResolveOutcomeResolved, wantSourceID: "art-A", wantAvailableSrc: 3},
		{name: "bare digit 2 — second source", userText: "2", wc: threeSources, wantOutcome: ResolveOutcomeResolved, wantSourceID: "art-B", wantAvailableSrc: 3},

		// --- missing: numeric out-of-range ---
		{name: "open 5 — out-of-range (3 sources)", userText: "open 5", wc: threeSources, wantOutcome: ResolveOutcomeMissing, wantAvailableSrc: 3},
		{name: "open 0 — out-of-range (lower)", userText: "open 0", wc: threeSources, wantOutcome: ResolveOutcomeMissing, wantAvailableSrc: 3},
		{name: "open -1 — out-of-range (negative)", userText: "open -1", wc: threeSources, wantOutcome: ResolveOutcomeMissing, wantAvailableSrc: 3},
		{name: "open 3 against 2 sources — out-of-range", userText: "open 3", wc: twoSources, wantOutcome: ResolveOutcomeMissing, wantAvailableSrc: 2},

		// --- missing: no prior context ---
		{name: "that one with empty context", userText: "that one", wc: empty, wantOutcome: ResolveOutcomeMissing},
		{name: "open 2 with empty context", userText: "open 2", wc: empty, wantOutcome: ResolveOutcomeMissing},

		// --- missing: prior turn had zero sources ---
		{name: "that with zero-source prior turn", userText: "that", wc: zeroSources, wantOutcome: ResolveOutcomeMissing},

		// --- none: no reference detected ---
		{name: "plain text — no reference", userText: "what time is it in barcelona", wc: threeSources, wantOutcome: ResolveOutcomeNone},
		{name: "open the door — no reference (no numeric)", userText: "open the door", wc: threeSources, wantOutcome: ResolveOutcomeNone},
		{name: "show me the recipe — no reference (no numeric)", userText: "show me the recipe", wc: threeSources, wantOutcome: ResolveOutcomeNone},
		{name: "two-word it phrase — no reference", userText: "it works", wc: threeSources, wantOutcome: ResolveOutcomeNone},
		{name: "empty string — no reference", userText: "", wc: threeSources, wantOutcome: ResolveOutcomeNone},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveReference(tc.userText, tc.wc)
			if got.Outcome != tc.wantOutcome {
				t.Fatalf("Outcome = %q; want %q", got.Outcome, tc.wantOutcome)
			}
			if got.SourceID != tc.wantSourceID {
				t.Errorf("SourceID = %q; want %q", got.SourceID, tc.wantSourceID)
			}
			if got.AvailableSources != tc.wantAvailableSrc {
				t.Errorf("AvailableSources = %d; want %d", got.AvailableSources, tc.wantAvailableSrc)
			}
		})
	}
}
