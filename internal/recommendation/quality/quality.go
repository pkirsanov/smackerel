package quality

import "context"

// Decision records a quality guard result such as stale facts, diversity, or
// repeat cooldown.
type Decision struct {
	Kind    string
	Outcome string
	Reason  string
}

// Guard applies quality constraints before delivery.
type Guard interface {
	Evaluate(ctx context.Context, candidateID string) ([]Decision, error)
}
