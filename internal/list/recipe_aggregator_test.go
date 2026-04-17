package list

import (
	"encoding/json"
	"testing"
)

func TestRecipeAggregator_MergeDuplicates(t *testing.T) {
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"garlic","quantity":"2","unit":"cloves"}]}`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"garlic","quantity":"3","unit":"cloves"}]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	if len(seeds) != 1 {
		t.Fatalf("expected 1 merged item, got %d", len(seeds))
	}

	if seeds[0].Quantity == nil || *seeds[0].Quantity != 5 {
		t.Fatalf("expected quantity 5, got %v", seeds[0].Quantity)
	}

	if len(seeds[0].SourceArtifactIDs) != 2 {
		t.Fatalf("expected 2 source artifacts, got %d", len(seeds[0].SourceArtifactIDs))
	}
}

func TestRecipeAggregator_DifferentIngredients(t *testing.T) {
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[
			{"name":"chicken","quantity":"2","unit":"lbs"},
			{"name":"garlic","quantity":"3","unit":"cloves"}
		]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	if len(seeds) != 2 {
		t.Fatalf("expected 2 items, got %d", len(seeds))
	}
}

func TestRecipeAggregator_CategoriesAssigned(t *testing.T) {
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[
			{"name":"chicken breast","quantity":"2","unit":"lbs"},
			{"name":"garlic","quantity":"3","unit":"cloves"},
			{"name":"olive oil","quantity":"2","unit":"tbsp"},
			{"name":"salt","quantity":"1","unit":"tsp"},
			{"name":"flour","quantity":"1","unit":"cup"}
		]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	categories := make(map[string]bool)
	for _, s := range seeds {
		categories[s.Category] = true
	}

	for _, expected := range []string{"proteins", "produce", "pantry", "spices", "baking"} {
		if !categories[expected] {
			t.Errorf("expected category %s, not found", expected)
		}
	}
}

func TestRecipeAggregator_SkipsEmptyIngredients(t *testing.T) {
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[
			{"name":"","quantity":"2","unit":"cups"},
			{"name":"flour","quantity":"1","unit":"cup"}
		]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	if len(seeds) != 1 {
		t.Fatalf("expected 1 item (empty name skipped), got %d", len(seeds))
	}
}

func TestRecipeAggregator_HandlesInvalidJSON(t *testing.T) {
	a := &RecipeAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`not json`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"flour","quantity":"1","unit":"cup"}]}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	// Should still get items from the valid source
	if len(seeds) != 1 {
		t.Fatalf("expected 1 item from valid source, got %d", len(seeds))
	}
}

func TestParseQuantity_Integer(t *testing.T) {
	qty, _ := ParseQuantity("3", "cups")
	if qty != 3 {
		t.Fatalf("expected 3, got %f", qty)
	}
}

func TestParseQuantity_Decimal(t *testing.T) {
	qty, _ := ParseQuantity("2.5", "tbsp")
	if qty != 2.5 {
		t.Fatalf("expected 2.5, got %f", qty)
	}
}

func TestParseQuantity_MixedFraction(t *testing.T) {
	qty, _ := ParseQuantity("2 1/2", "cups")
	if qty != 2.5 {
		t.Fatalf("expected 2.5, got %f", qty)
	}
}

func TestParseQuantity_SimpleFraction(t *testing.T) {
	qty, _ := ParseQuantity("1/4", "tsp")
	if qty != 0.25 {
		t.Fatalf("expected 0.25, got %f", qty)
	}
}

func TestParseQuantity_Empty(t *testing.T) {
	qty, _ := ParseQuantity("", "pinch")
	if qty != 0 {
		t.Fatalf("expected 0 for empty quantity, got %f", qty)
	}
}

func TestNormalizeUnit(t *testing.T) {
	cases := map[string]string{
		"tablespoon":  "tbsp",
		"tablespoons": "tbsp",
		"teaspoon":    "tsp",
		"cups":        "cup",
		"ounces":      "oz",
		"pounds":      "lb",
		"grams":       "g",
		"tbsp":        "tbsp",
		"":            "",
	}
	for input, expected := range cases {
		got := NormalizeUnit(input)
		if got != expected {
			t.Errorf("NormalizeUnit(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestNormalizeIngredientName(t *testing.T) {
	cases := map[string]string{
		"Chicken Breasts": "chicken breast",
		"garlic":          "garlic",
		"Tomatoes":        "tomato",
		"hummus":          "hummus", // don't strip 's' from words ending in 'ss'
	}
	for input, expected := range cases {
		got := NormalizeIngredientName(input)
		if got != expected {
			t.Errorf("NormalizeIngredientName(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestCategorizeIngredient(t *testing.T) {
	cases := map[string]string{
		"chicken breast": "proteins",
		"garlic":         "produce",
		"butter":         "dairy",
		"olive oil":      "pantry",
		"cumin":          "spices",
		"flour":          "baking",
		"water":          "beverages",
		"mystery item":   "other",
	}
	for input, expected := range cases {
		got := CategorizeIngredient(input)
		if got != expected {
			t.Errorf("CategorizeIngredient(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestFormatIngredient(t *testing.T) {
	cases := []struct {
		name, unit, prep string
		qty              float64
		expected         string
	}{
		{"garlic", "cloves", "", 5, "5 cloves garlic"},
		{"flour", "cup", "", 2, "2 cup flour"},
		{"salt", "tsp", "", 0.5, "0.5 tsp salt"},
		{"oil", "", "", 0, "oil"},
		{"chicken", "lbs", "diced", 2, "2 lbs chicken (diced)"},
	}
	for _, tc := range cases {
		got := FormatIngredient(tc.name, tc.qty, tc.unit, tc.prep)
		if got != tc.expected {
			t.Errorf("FormatIngredient(%q, %f, %q, %q) = %q, want %q",
				tc.name, tc.qty, tc.unit, tc.prep, got, tc.expected)
		}
	}
}
