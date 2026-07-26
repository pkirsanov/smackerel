package acceptance

import (
	"testing"
)

func compiledCanonicalPolicy(t *testing.T) CompiledAcceptancePolicy {
	t.Helper()
	p, err := CanonicalProductJourneyManifest().Compile(DefaultPolicyConfig(), nil)
	if err != nil {
		t.Fatalf("Compile(canonical) error = %v; want nil", err)
	}
	return p
}

// requiredResultsAll returns one result per REQUIRED journey with the given
// outcome, satisfying the reducer's required-journey completeness rule.
func requiredResultsAll(p CompiledAcceptancePolicy, outcome JourneyOutcome) []JourneyResult {
	var rs []JourneyResult
	for _, cj := range p.Journeys {
		if cj.Requiredness == RequirednessRequired {
			rs = append(rs, JourneyResult{JourneyID: cj.ID, Outcome: outcome})
		}
	}
	return rs
}

// withOutcome returns a copy of rs with the row for id set to outcome.
func withOutcome(rs []JourneyResult, id string, outcome JourneyOutcome) []JourneyResult {
	out := append([]JourneyResult(nil), rs...)
	for i := range out {
		if out[i].JourneyID == id {
			out[i].Outcome = outcome
		}
	}
	return out
}

// TestAllowedEmptyQuietOptionalAndDegradedRequireExactPolicy is TP-102-01-03. It
// proves each allowed-* outcome is honored ONLY when the compiled policy for
// that journey permits it, that an absent or ambiguous policy fails closed, and
// that a health-only / not-evaluated required journey never yields `accepted`. A
// permissive reducer (one that accepted any allowed-* outcome or promoted a
// not-evaluated required journey) would fail the adversarial subtests, so the
// canaries are real.
func TestAllowedEmptyQuietOptionalAndDegradedRequireExactPolicy(t *testing.T) {
	var reducer VerdictReducer
	policy := compiledCanonicalPolicy(t)

	t.Run("every required journey passing yields accepted", func(t *testing.T) {
		got := reducer.Reduce(policy, nil, requiredResultsAll(policy, OutcomePassed))
		if got.Verdict != VerdictAccepted {
			t.Fatalf("Reduce(all-required-pass).Verdict = %q (code %q); want %q", got.Verdict, got.Code, VerdictAccepted)
		}
	})

	t.Run("allowed-empty is honored only when policy permits it", func(t *testing.T) {
		// search.read explicitly allows allowed-empty -> honored (accepted-degraded).
		if _, ok := policy.Journey("search.read"); !ok {
			t.Fatal("search.read missing from compiled policy")
		}
		got := reducer.Reduce(policy, nil, withOutcome(requiredResultsAll(policy, OutcomePassed), "search.read", OutcomeAllowedEmpty))
		if got.Verdict != VerdictAcceptedDegraded {
			t.Fatalf("policy-permitted allowed-empty verdict = %q (code %q); want %q", got.Verdict, got.Code, VerdictAcceptedDegraded)
		}
		// session.login-reuse permits ONLY passed -> allowed-empty fails closed.
		bad := reducer.Reduce(policy, nil, withOutcome(requiredResultsAll(policy, OutcomePassed), "session.login-reuse", OutcomeAllowedEmpty))
		if bad.Verdict != VerdictContractInvalid {
			t.Fatalf("un-permitted allowed-empty verdict = %q; want %q", bad.Verdict, VerdictContractInvalid)
		}
	})

	t.Run("allowed-degraded on a journey that does not permit it fails closed", func(t *testing.T) {
		bad := reducer.Reduce(policy, nil, withOutcome(requiredResultsAll(policy, OutcomePassed), "session.login-reuse", OutcomeAllowedDegraded))
		if bad.Verdict != VerdictContractInvalid {
			t.Fatalf("un-permitted allowed-degraded verdict = %q; want %q", bad.Verdict, VerdictContractInvalid)
		}
	})

	t.Run("an explicit optional allowed-optional degrades but is accepted", func(t *testing.T) {
		results := append(requiredResultsAll(policy, OutcomePassed),
			JourneyResult{JourneyID: "cards.representative-read", Outcome: OutcomeAllowedOptional})
		got := reducer.Reduce(policy, nil, results)
		if got.Verdict != VerdictAcceptedDegraded {
			t.Fatalf("optional allowed-optional verdict = %q (code %q); want %q", got.Verdict, got.Code, VerdictAcceptedDegraded)
		}
		if got.OptionalLimited != 1 {
			t.Errorf("OptionalLimited = %d; want 1", got.OptionalLimited)
		}
	})

	t.Run("a health-only (not-evaluated) required journey never yields accepted", func(t *testing.T) {
		results := withOutcome(requiredResultsAll(policy, OutcomePassed), "capability-status.read", OutcomeNotEvaluated)
		got := reducer.Reduce(policy, nil, results)
		if got.Verdict == VerdictAccepted || got.Verdict == VerdictAcceptedDegraded {
			t.Fatalf("not-evaluated required journey verdict = %q; want a non-accepted verdict", got.Verdict)
		}
		if got.Verdict != VerdictRejected {
			t.Fatalf("not-evaluated required journey verdict = %q; want %q", got.Verdict, VerdictRejected)
		}
	})

	t.Run("a required journey that failed yields rejected", func(t *testing.T) {
		results := withOutcome(requiredResultsAll(policy, OutcomePassed), "search.read", OutcomeFailed)
		got := reducer.Reduce(policy, nil, results)
		if got.Verdict != VerdictRejected {
			t.Fatalf("required-failed verdict = %q; want %q", got.Verdict, VerdictRejected)
		}
	})

	t.Run("an absent policy fails closed", func(t *testing.T) {
		got := reducer.Reduce(CompiledAcceptancePolicy{}, nil, nil)
		if got.Verdict != VerdictContractInvalid {
			t.Fatalf("empty-policy verdict = %q; want %q", got.Verdict, VerdictContractInvalid)
		}
	})

	t.Run("a result for a journey the policy does not know fails closed", func(t *testing.T) {
		results := append(requiredResultsAll(policy, OutcomePassed),
			JourneyResult{JourneyID: "ghost.journey", Outcome: OutcomePassed})
		got := reducer.Reduce(policy, nil, results)
		if got.Verdict != VerdictContractInvalid {
			t.Fatalf("unknown-journey verdict = %q; want %q", got.Verdict, VerdictContractInvalid)
		}
	})

	t.Run("a blocked required prerequisite yields blocked-prerequisite", func(t *testing.T) {
		results := withOutcome(requiredResultsAll(policy, OutcomePassed), "wiki.browse", OutcomeNotEvaluated)
		prereqs := []PrerequisiteResult{{
			Ref: "specs/080-knowledge-graph-public-api/bugs/BUG-080-001", Ready: false, Current: false, GatesJourney: "wiki.browse",
		}}
		got := reducer.Reduce(policy, prereqs, results)
		if got.Verdict != VerdictBlockedPrerequisite {
			t.Fatalf("blocked-prerequisite verdict = %q; want %q", got.Verdict, VerdictBlockedPrerequisite)
		}
	})
}
