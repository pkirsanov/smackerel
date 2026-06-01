package web

import "context"

// Brave is a v1 stub for the Brave Search API. The operator selects
// a provider via assistant.open_knowledge.provider; until a real
// adapter ships, selecting "brave" surfaces ErrProviderNotConfigured
// from the agent loop instead of silently falling back.
type Brave struct{}

// NewBrave constructs the Brave stub. It never returns an error; the
// stub exists so the registry wiring compiles end-to-end before the
// real adapter lands.
func NewBrave() *Brave { return &Brave{} }

// Name implements WebSearchProvider.
func (*Brave) Name() string { return "brave" }

// Search implements WebSearchProvider. Always returns
// ErrProviderNotConfigured; never makes a network call.
func (*Brave) Search(_ context.Context, _ string, _ int) ([]WebSnippet, error) {
	return nil, ErrProviderNotConfigured
}
