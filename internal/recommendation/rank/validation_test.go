package rank

import "testing"

func TestValidateProviderBackedRankingsRejectsInjectedCandidate(t *testing.T) {
	rankings := []RankedCandidate{
		{CandidateID: "cand-provider-backed", Rank: 1},
		{CandidateID: "cand-injected", Rank: 2},
	}

	err := ValidateProviderBackedRankings(rankings, []string{"cand-provider-backed"})
	if err == nil {
		t.Fatal("ValidateProviderBackedRankings succeeded; want injected-candidate rejection")
	}
}

func TestValidateProviderBackedRankingsAcceptsProviderBackedCandidates(t *testing.T) {
	rankings := []RankedCandidate{
		{CandidateID: "cand-a", Rank: 1},
		{CandidateID: "cand-b", Rank: 2},
	}

	if err := ValidateProviderBackedRankings(rankings, []string{"cand-b", "cand-a"}); err != nil {
		t.Fatalf("ValidateProviderBackedRankings returned error: %v", err)
	}
}
