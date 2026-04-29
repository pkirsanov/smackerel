//go:build integration || e2e

package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/recommendation"
)

// FixtureProvider is available only to integration/e2e builds. Scope 1 keeps
// the production registry empty; later tests can opt in by registering this
// provider against an isolated registry.
type FixtureProvider struct {
	mu         sync.Mutex
	id         string
	display    string
	categories []recommendation.Category
	health     RuntimeHealth
	observed   []ReducedQuery
}

// NewFixtureProvider returns a deterministic in-process provider for live-stack
// tests that must avoid external network calls.
func NewFixtureProvider(id, display string, categories []recommendation.Category) *FixtureProvider {
	return &FixtureProvider{
		id:         id,
		display:    display,
		categories: append([]recommendation.Category(nil), categories...),
		health: RuntimeHealth{
			ProviderID:   id,
			DisplayName:  display,
			Status:       StatusHealthy,
			ObservedAt:   time.Now().UTC(),
			CategoryList: append([]recommendation.Category(nil), categories...),
		},
	}
}

func (p *FixtureProvider) ID() string { return p.id }

func (p *FixtureProvider) DisplayName() string { return p.display }

func (p *FixtureProvider) Categories() []recommendation.Category {
	return append([]recommendation.Category(nil), p.categories...)
}

func (p *FixtureProvider) Fetch(_ context.Context, query ReducedQuery) (FactsBundle, error) {
	p.mu.Lock()
	p.observed = append(p.observed, query)
	p.mu.Unlock()

	if p.health.Status == StatusFailing {
		return FactsBundle{ProviderID: p.id}, fmt.Errorf("fixture provider %s outage", p.id)
	}
	if query.Category != recommendation.CategoryPlace {
		return FactsBundle{ProviderID: p.id}, nil
	}

	facts := p.placeFacts(query)
	if query.Limit > 0 && len(facts) > query.Limit {
		facts = facts[:query.Limit]
	}
	return FactsBundle{ProviderID: p.id, Facts: facts}, nil
}

func (p *FixtureProvider) Health(_ context.Context) RuntimeHealth {
	health := p.health
	health.ObservedAt = time.Now().UTC()
	return health
}

// ObservedQueries returns provider-facing queries captured by this fixture.
func (p *FixtureProvider) ObservedQueries() []ReducedQuery {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]ReducedQuery(nil), p.observed...)
}

// SetHealth lets integration tests exercise provider degradation paths.
func (p *FixtureProvider) SetHealth(status RuntimeStatus, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.health.Status = status
	p.health.Reason = reason
}

func (p *FixtureProvider) placeFacts(query ReducedQuery) []Fact {
	now := time.Now().UTC()
	lowerQuery := strings.ToLower(query.Query)
	providerName := p.display
	if providerName == "" {
		providerName = p.id
	}

	var rows []struct {
		id          string
		title       string
		url         string
		score       float64
		quiet       bool
		vegetarian  bool
		hours       string
		conflicting bool
	}
	switch {
	case strings.Contains(lowerQuery, "coffee"):
		rows = []struct {
			id          string
			title       string
			url         string
			score       float64
			quiet       bool
			vegetarian  bool
			hours       string
			conflicting bool
		}{
			{id: "coffee-fogline", title: "Fogline Coffee", url: "https://fixture.example/coffee/fogline", score: 0.82, quiet: true, hours: "07:00-16:00"},
			{id: "coffee-mission-bean", title: "Mission Bean", url: "https://fixture.example/coffee/mission-bean", score: 0.74, quiet: true, hours: "08:00-15:00"},
			{id: "coffee-corner", title: "Corner Espresso", url: "https://fixture.example/coffee/corner", score: 0.68, quiet: false, hours: "07:30-17:00"},
		}
	case strings.Contains(lowerQuery, "conflict"):
		rows = []struct {
			id          string
			title       string
			url         string
			score       float64
			quiet       bool
			vegetarian  bool
			hours       string
			conflicting bool
		}{
			{id: "ramen-late-lab", title: "Late Ramen Lab", url: "https://fixture.example/ramen/late-lab", score: 0.79, quiet: true, hours: p.conflictHours(), conflicting: true},
		}
	default:
		rows = []struct {
			id          string
			title       string
			url         string
			score       float64
			quiet       bool
			vegetarian  bool
			hours       string
			conflicting bool
		}{
			{id: "ramen-tonkotsu", title: "Tonkotsu Workshop", url: "https://fixture.example/ramen/tonkotsu", score: 0.91, quiet: true, hours: "11:00-21:00"},
			{id: "ramen-quiet-noodle", title: "Quiet Noodle Bar", url: "https://fixture.example/ramen/quiet-noodle", score: 0.83, quiet: true, vegetarian: true, hours: "12:00-20:00"},
			{id: "ramen-mission-shoyu", title: "Mission Shoyu", url: "https://fixture.example/ramen/mission-shoyu", score: 0.76, quiet: false, vegetarian: true, hours: "11:30-22:00"},
			{id: "ramen-pork-broth", title: "Pork Broth Ramen", url: "https://fixture.example/ramen/pork-broth", score: 0.72, quiet: false, vegetarian: false, hours: "10:00-22:30"},
		}
	}

	facts := make([]Fact, 0, len(rows))
	for _, row := range rows {
		providerCandidateID := row.id
		if strings.Contains(p.id, "yelp") {
			providerCandidateID += "-yelp"
		}
		fact := Fact{
			ProviderID:          p.id,
			ProviderCandidateID: providerCandidateID,
			Category:            recommendation.CategoryPlace,
			Title:               row.title,
			RetrievedAt:         now,
			SourceUpdatedAt:     &now,
			NormalizedFact: map[string]any{
				"title":           row.title,
				"canonical_key":   canonicalKey(row.title),
				"canonical_url":   row.url,
				"provider_score":  row.score,
				"quiet":           row.quiet,
				"vegetarian":      row.vegetarian,
				"opening_hours":   row.hours,
				"source_conflict": row.conflicting,
				"location_cell":   query.Geometry.CellID,
			},
			Attribution: map[string]any{
				"label": providerName,
				"url":   row.url,
			},
			SponsoredState:  "none",
			RestrictedFlags: map[string]any{},
		}
		facts = append(facts, fact)
	}
	return facts
}

func (p *FixtureProvider) conflictHours() string {
	if strings.Contains(p.id, "yelp") {
		return "closed today"
	}
	return "11:00-23:00"
}

func canonicalKey(title string) string {
	value := strings.ToLower(strings.TrimSpace(title))
	value = strings.NewReplacer(" ", "-", "'", "", "&", "and").Replace(value)
	return value
}
