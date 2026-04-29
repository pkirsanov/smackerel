package provider

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/recommendation"
	"github.com/smackerel/smackerel/internal/recommendation/location"
)

// RuntimeStatus is the bounded provider health state surfaced to operators.
type RuntimeStatus string

const (
	StatusHealthy  RuntimeStatus = "healthy"
	StatusDegraded RuntimeStatus = "degraded"
	StatusFailing  RuntimeStatus = "failing"
	StatusDisabled RuntimeStatus = "disabled"
)

// RuntimeHealth is the provider-neutral health snapshot for status pages and
// provider runtime-state persistence.
type RuntimeHealth struct {
	ProviderID   string
	DisplayName  string
	Status       RuntimeStatus
	Reason       string
	ObservedAt   time.Time
	CategoryList []recommendation.Category
}

// ReducedQuery is the provider-facing query shape after location and policy
// reduction. Later scopes fill this out without changing the provider boundary.
type ReducedQuery struct {
	Category        recommendation.Category
	Query           string
	PrecisionPolicy recommendation.PrecisionPolicy
	Geometry        location.ReducedGeometry
	Limit           int
}

// Fact is a normalized read-only provider result. Raw payloads are not part of
// this contract; providers must reduce payloads before returning facts.
type Fact struct {
	ProviderID          string
	ProviderCandidateID string
	Category            recommendation.Category
	Title               string
	NormalizedFact      map[string]any
	RetrievedAt         time.Time
	SourceUpdatedAt     *time.Time
	Attribution         map[string]any
	SponsoredState      string
	RestrictedFlags     map[string]any
}

// FactsBundle is the provider-neutral response from Fetch.
type FactsBundle struct {
	ProviderID string
	Facts      []Fact
}

// Provider is the read-only recommendation provider contract. It is not a
// connector and does not own periodic Sync behavior.
type Provider interface {
	ID() string
	DisplayName() string
	Categories() []recommendation.Category
	Fetch(ctx context.Context, query ReducedQuery) (FactsBundle, error)
	Health(ctx context.Context) RuntimeHealth
}

// Registry holds the configured recommendation providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// DefaultRegistry is intentionally empty in Scope 1. Production providers are
// registered in later scopes after their external boundaries exist.
var DefaultRegistry = NewRegistry()

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a provider and fails loudly on invalid or duplicate IDs.
func (r *Registry) Register(p Provider) {
	if p == nil {
		panic("recommendation/provider: Register called with nil provider")
	}
	id := p.ID()
	if id == "" {
		panic("recommendation/provider: Register called with empty provider ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.providers[id]; ok {
		panic(fmt.Sprintf("recommendation/provider: provider %q already registered (existing %T, attempted %T)", id, existing, p))
	}
	r.providers[id] = p
}

// Get resolves a provider by ID.
func (r *Registry) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// List returns providers in stable ID order.
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	providers := make([]Provider, 0, len(ids))
	for _, id := range ids {
		providers = append(providers, r.providers[id])
	}
	return providers
}

// Len returns the current provider count.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}
