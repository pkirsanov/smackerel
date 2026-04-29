package dedupe

import "context"

// CandidateGroup preserves the provider fact refs that formed one canonical
// recommendation candidate.
type CandidateGroup struct {
	CandidateID     string
	ProviderFactIDs []string
	MergeReason     string
}

// Engine groups provider facts without discarding source conflicts.
type Engine interface {
	Group(ctx context.Context, providerFactIDs []string) ([]CandidateGroup, error)
}
