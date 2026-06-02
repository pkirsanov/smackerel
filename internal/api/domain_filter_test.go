package api

import (
	"encoding/json"
	"testing"
)

// TestSearchFilters_DomainFieldSerialization verifies that domain-specific filters
// serialize correctly in the SearchRequest JSON round-trip. (Scope 8, T8-06/T8-07)
func TestSearchFilters_DomainFieldSerialization(t *testing.T) {
	req := SearchRequest{
		Query: "recipes with chicken",
		Filters: SearchFilters{
			Domain:     "recipe",
			Ingredient: "chicken",
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded SearchRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Filters.Domain != "recipe" {
		t.Errorf("expected domain=recipe, got %q", decoded.Filters.Domain)
	}
	if decoded.Filters.Ingredient != "chicken" {
		t.Errorf("expected ingredient=chicken, got %q", decoded.Filters.Ingredient)
	}
}

// TestSearchFilters_PriceMaxSerialization verifies that PriceMax serializes
// correctly in SearchRequest JSON. (Scope 8, T8-07)
func TestSearchFilters_PriceMaxSerialization(t *testing.T) {
	req := SearchRequest{
		Query: "headphones under $200",
		Filters: SearchFilters{
			Domain:   "product",
			PriceMax: 200.0,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded SearchRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Filters.Domain != "product" {
		t.Errorf("expected domain=product, got %q", decoded.Filters.Domain)
	}
	if decoded.Filters.PriceMax != 200.0 {
		t.Errorf("expected price_max=200, got %f", decoded.Filters.PriceMax)
	}
}

// TestSearchFilters_DomainOmittedWhenEmpty verifies that empty domain filters
// are omitted from JSON output (omitempty). (Scope 8, T8-06)
func TestSearchFilters_DomainOmittedWhenEmpty(t *testing.T) {
	req := SearchRequest{
		Query:   "some query",
		Filters: SearchFilters{},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

	filters, ok := raw["filters"].(map[string]interface{})
	if !ok {
		// No filters key is fine — omitempty
		return
	}

	if _, exists := filters["domain"]; exists {
		t.Error("expected domain to be omitted when empty")
	}
	if _, exists := filters["ingredient"]; exists {
		t.Error("expected ingredient to be omitted when empty")
	}
	if _, exists := filters["price_max"]; exists {
		t.Error("expected price_max to be omitted when zero")
	}
}

// TestSearchResult_DomainDataSerialization verifies that SearchResult includes
// domain_data when present and omits it when absent. (Scope 8)
func TestSearchResult_DomainDataSerialization(t *testing.T) {
	t.Run("with domain_data", func(t *testing.T) {
		result := SearchResult{
			ArtifactID:   "art-001",
			Title:        "Pasta Carbonara",
			ArtifactType: "recipe",
			DomainData:   json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"eggs"}]}`),
		}

		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		dd, ok := raw["domain_data"]
		if !ok {
			t.Fatal("expected domain_data to be present")
		}
		ddMap, ok := dd.(map[string]interface{})
		if !ok {
			t.Fatalf("expected domain_data to be object, got %T", dd)
		}
		if ddMap["domain"] != "recipe" {
			t.Errorf("expected domain=recipe in domain_data, got %v", ddMap["domain"])
		}
	})

	t.Run("without domain_data", func(t *testing.T) {
		result := SearchResult{
			ArtifactID:   "art-002",
			Title:        "News Article",
			ArtifactType: "article",
		}

		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if _, ok := raw["domain_data"]; ok {
			t.Error("expected domain_data to be omitted when nil")
		}
	})
}

// Spec 066 SCOPE-4: the legacy regex domain-intent helper was removed
// along with its dedicated tests. Explicit-filter serialization remains
// covered by the JSON round-trip tests above; domain/entity resolution now
// flows through spec 068 compiled intents and spec 065 entity_resolve.

