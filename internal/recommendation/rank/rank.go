package rank

import (
	"context"
	"fmt"
)

// RankedCandidate is a structured ranking result with source refs retained for
// renderer validation.
type RankedCandidate struct {
	CandidateID     string
	Rank            int
	ScoreBreakdown  map[string]float64
	GraphSignalRefs []string
	Confidence      string
}

// PreferenceCorrection is the ranking-facing shape of an active user correction.
type PreferenceCorrection struct {
	ID             string
	PreferenceKey  string
	CorrectionKind string
}

// Ranker orders canonical candidates against bounded graph context.
type Ranker interface {
	Rank(ctx context.Context, candidateIDs []string, graphSignalRefs []string) ([]RankedCandidate, error)
}

// ValidateProviderBackedRankings rejects rankings for candidates that did not
// come from provider facts in the current run.
func ValidateProviderBackedRankings(rankings []RankedCandidate, providerBackedCandidateIDs []string) error {
	allowed := make(map[string]struct{}, len(providerBackedCandidateIDs))
	for _, id := range providerBackedCandidateIDs {
		if id != "" {
			allowed[id] = struct{}{}
		}
	}
	for _, ranking := range rankings {
		if ranking.CandidateID == "" {
			return fmt.Errorf("ranked candidate missing candidate_id")
		}
		if _, ok := allowed[ranking.CandidateID]; !ok {
			return fmt.Errorf("ranked candidate %q was not returned by a provider", ranking.CandidateID)
		}
	}
	return nil
}

// ActiveCorrectionForPreference returns the active correction that blocks a
// positive boost for preferenceKey, if one exists.
func ActiveCorrectionForPreference(preferenceKey string, corrections []PreferenceCorrection) (PreferenceCorrection, bool) {
	if preferenceKey == "" {
		return PreferenceCorrection{}, false
	}
	for _, correction := range corrections {
		if correction.PreferenceKey != preferenceKey {
			continue
		}
		switch correction.CorrectionKind {
		case "remove", "invert", "set_weight", "block_category":
			return correction, true
		}
	}
	return PreferenceCorrection{}, false
}

// PositiveBoostAllowed reports whether preferenceKey may still act as a
// positive ranking signal after active user corrections are applied.
func PositiveBoostAllowed(preferenceKey string, corrections []PreferenceCorrection) bool {
	_, blocked := ActiveCorrectionForPreference(preferenceKey, corrections)
	return !blocked
}
