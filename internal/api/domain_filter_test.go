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

// TestDomainIntentToSearchFilters verifies that parseDomainIntent output maps
// correctly to SearchFilters fields. (Scope 8, T8-06/T8-07)
func TestDomainIntentToSearchFilters(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectDomain   string
		expectIngr     string
		expectPriceMax float64
	}{
		{
			name:         "recipe with ingredient",
			query:        "recipes with chicken",
			expectDomain: "recipe",
			expectIngr:   "chicken",
		},
		{
			name:         "recipe with multiple ingredients picks first",
			query:        "recipes with lemon and garlic",
			expectDomain: "recipe",
			expectIngr:   "lemon",
		},
		{
			name:           "product with price ceiling",
			query:          "cameras under $500",
			expectDomain:   "product",
			expectPriceMax: 500,
		},
		{
			name:         "product without price",
			query:        "best headphones",
			expectDomain: "product",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			intent := parseDomainIntent(tc.query)
			if intent == nil {
				t.Fatal("expected non-nil intent")
			}

			// Simulate how SearchHandler maps intent to filters
			var filters SearchFilters
			if filters.Domain == "" {
				filters.Domain = intent.Domain
			}
			if len(intent.Attributes) > 0 && filters.Ingredient == "" {
				filters.Ingredient = intent.Attributes[0]
			}
			if intent.PriceMax > 0 && filters.PriceMax == 0 {
				filters.PriceMax = intent.PriceMax
			}

			if filters.Domain != tc.expectDomain {
				t.Errorf("domain: expected %q, got %q", tc.expectDomain, filters.Domain)
			}
			if filters.Ingredient != tc.expectIngr {
				t.Errorf("ingredient: expected %q, got %q", tc.expectIngr, filters.Ingredient)
			}
			if filters.PriceMax != tc.expectPriceMax {
				t.Errorf("price_max: expected %f, got %f", tc.expectPriceMax, filters.PriceMax)
			}
		})
	}
}

// TestDomainIntentDoesNotOverrideExplicitFilters verifies that explicit filters
// take precedence over domain intent parsing. (Scope 8)
func TestDomainIntentDoesNotOverrideExplicitFilters(t *testing.T) {
	intent := parseDomainIntent("recipes with chicken")
	if intent == nil {
		t.Fatal("expected non-nil intent")
	}

	// Simulate explicit filter already set
	filters := SearchFilters{
		Domain:     "product",
		Ingredient: "tofu",
	}

	// Only set from intent if not already set (replicating search.go logic)
	if filters.Domain == "" {
		filters.Domain = intent.Domain
	}
	if len(intent.Attributes) > 0 && filters.Ingredient == "" {
		filters.Ingredient = intent.Attributes[0]
	}

	if filters.Domain != "product" {
		t.Errorf("explicit domain should not be overridden, got %q", filters.Domain)
	}
	if filters.Ingredient != "tofu" {
		t.Errorf("explicit ingredient should not be overridden, got %q", filters.Ingredient)
	}
}
