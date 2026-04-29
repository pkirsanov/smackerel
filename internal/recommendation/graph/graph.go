package graph

import "context"

// SignalRef is a bounded personal-knowledge reference used by rank and why
// rendering. It carries IDs, not unsourced free text.
type SignalRef struct {
	ArtifactID string
	Kind       string
	Weight     float64
}

// Snapshot is the bounded graph context for one recommendation run.
type Snapshot struct {
	ActorUserID string
	Signals     []SignalRef
}

// Reader loads graph signals for recommendation scenarios.
type Reader interface {
	Snapshot(ctx context.Context, actorUserID string, candidateIDs []string) (Snapshot, error)
}
