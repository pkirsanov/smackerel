package policy

import "context"

// Decision records a policy guard result that must be persisted before any
// recommendation can render.
type Decision struct {
	Kind    string
	Outcome string
	Reason  string
}

// Guard enforces consent, sponsorship, restricted-category, safety, and hard
// constraint policy.
type Guard interface {
	Evaluate(ctx context.Context, candidateID string) ([]Decision, error)
}
