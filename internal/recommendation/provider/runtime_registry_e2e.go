//go:build e2e

package provider

import "github.com/smackerel/smackerel/internal/recommendation"

// RuntimeRegistry returns deterministic fixture providers for the disposable
// e2e stack. DefaultRegistry stays empty so production and operator-status
// zero-provider behavior remain unchanged.
func RuntimeRegistry() *Registry {
	registry := NewRegistry()
	registry.Register(NewFixtureProvider("fixture_google_places", "Fixture Google Places", []recommendation.Category{recommendation.CategoryPlace}))
	registry.Register(NewFixtureProvider("fixture_yelp", "Fixture Yelp", []recommendation.Category{recommendation.CategoryPlace}))
	return registry
}
