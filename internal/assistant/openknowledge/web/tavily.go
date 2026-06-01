package web

import "context"

// Tavily is a v1 stub for the Tavily search API. See brave.go for the
// rationale: stub today, real adapter later, never a silent fallback.
type Tavily struct{}

// NewTavily constructs the Tavily stub.
func NewTavily() *Tavily { return &Tavily{} }

// Name implements WebSearchProvider.
func (*Tavily) Name() string { return "tavily" }

// Search implements WebSearchProvider. Always returns
// ErrProviderNotConfigured.
func (*Tavily) Search(_ context.Context, _ string, _ int) ([]WebSnippet, error) {
	return nil, ErrProviderNotConfigured
}
