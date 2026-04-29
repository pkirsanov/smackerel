package provider

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/recommendation"
)

type testProvider struct {
	id      string
	display string
}

func (p *testProvider) ID() string { return p.id }

func (p *testProvider) DisplayName() string { return p.display }

func (p *testProvider) Categories() []recommendation.Category {
	return []recommendation.Category{recommendation.CategoryPlace}
}

func (p *testProvider) Fetch(_ context.Context, _ ReducedQuery) (FactsBundle, error) {
	return FactsBundle{ProviderID: p.id}, nil
}

func (p *testProvider) Health(_ context.Context) RuntimeHealth {
	return RuntimeHealth{
		ProviderID:  p.id,
		DisplayName: p.display,
		Status:      StatusHealthy,
		ObservedAt:  time.Now().UTC(),
	}
}

func TestDefaultRegistryIsEmptyByDefault(t *testing.T) {
	if got := DefaultRegistry.Len(); got != 0 {
		t.Fatalf("DefaultRegistry.Len() = %d, want 0", got)
	}
}

func TestRegistryListsProvidersInStableOrder(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&testProvider{id: "zeta", display: "Zeta"})
	reg.Register(&testProvider{id: "alpha", display: "Alpha"})

	providers := reg.List()
	if len(providers) != 2 {
		t.Fatalf("List returned %d providers, want 2", len(providers))
	}
	if providers[0].ID() != "alpha" || providers[1].ID() != "zeta" {
		t.Fatalf("List order = %q, %q; want alpha, zeta", providers[0].ID(), providers[1].ID())
	}
}

func TestRegistryDuplicateRegistrationPanics(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&testProvider{id: "same", display: "Same"})
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected duplicate provider panic")
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, "same") || !strings.Contains(msg, "already registered") {
			t.Fatalf("panic = %q, want duplicate provider detail", msg)
		}
	}()
	reg.Register(&testProvider{id: "same", display: "Same Again"})
}
